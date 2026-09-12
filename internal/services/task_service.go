package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const taskExecutionTimeout = 5 * time.Minute

var (
	ociErrorMessagePattern = regexp.MustCompile(`Message:\s*([^.]+)`)
	ErrTaskServiceStopped  = errors.New("任务服务未启动")
	ErrTaskBusy            = errors.New("任务仍在执行，请稍后重试")
)

func extractOCIErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if matches := ociErrorMessagePattern.FindStringSubmatch(err.Error()); len(matches) > 1 {
		return matches[1]
	}
	return err.Error()
}

type instanceCreator interface {
	CreateInstance(context.Context, *models.OciUser, string, string, string, float64, float64, int, int64, string, string) error
}

type taskExecution struct {
	ctx    context.Context
	cancel context.CancelFunc
	timer  *time.Timer
	active bool
}

type TaskService struct {
	db         *gorm.DB
	ociService instanceCreator

	// Start cannot race with Stop while it waits for workers.
	lifecycleMu sync.Mutex
	mu          sync.Mutex
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
	tasks       map[string]*taskExecution
	workers     sync.WaitGroup
	slots       chan struct{}
}

func NewTaskService(db *gorm.DB, ociService instanceCreator) *TaskService {
	return &TaskService{
		db: db, ociService: ociService,
		tasks: make(map[string]*taskExecution),
		slots: make(chan struct{}, 5),
	}
}

func (s *TaskService) Start() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	var tasks []models.OciCreateTask
	if err := s.db.Where("status = ?", "running").Find(&tasks).Error; err != nil {
		return err
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.running = true
	for _, task := range tasks {
		s.scheduleTaskLocked(task)
	}
	log.Println("Task service started")
	return nil
}

func (s *TaskService) Stop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.cancel()
	for id := range s.tasks {
		s.cancelTaskLocked(id)
	}
	s.mu.Unlock()

	s.workers.Wait()
	log.Println("Task service stopped")
}

// All admissions and state changes share mu. An active execution remains in
// the map until it exits, even after cancellation, so it cannot overlap a restart.
func (s *TaskService) scheduleTaskLocked(task models.OciCreateTask) {
	if !s.running || task.Status != "running" || s.tasks[task.ID] != nil {
		return
	}
	interval := time.Duration(task.Interval) * time.Second
	if interval < 10*time.Second {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(s.ctx)
	execution := &taskExecution{ctx: ctx, cancel: cancel}
	s.tasks[task.ID] = execution
	execution.timer = time.AfterFunc(interval, func() {
		util.Go("execute task "+task.ID, func() { s.executeScheduled(task.ID, execution) })
	})
}

func (s *TaskService) executeScheduled(taskID string, execution *taskExecution) {
	s.mu.Lock()
	if !s.running || s.tasks[taskID] != execution || execution.active || execution.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	execution.active = true
	execution.timer = nil
	s.workers.Add(1)
	s.mu.Unlock()

	if err := s.executeTask(taskID, execution, false); err != nil {
		log.Printf("Task %s: %v", taskID, err)
	}
}

func (s *TaskService) finishExecution(taskID string, execution *taskExecution, once bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.workers.Done()
	defer execution.cancel()
	if s.tasks[taskID] != execution {
		return
	}
	delete(s.tasks, taskID)
	if s.running && !once && execution.ctx.Err() == nil {
		var task models.OciCreateTask
		if err := s.db.First(&task, "id = ?", taskID).Error; err == nil {
			s.scheduleTaskLocked(task)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Failed to reschedule task %s: %v", taskID, err)
		}
	}
}

func (s *TaskService) executeTask(taskID string, execution *taskExecution, once bool) error {
	defer s.finishExecution(taskID, execution, once)
	ctx, cancel := context.WithTimeout(execution.ctx, taskExecutionTimeout)
	defer cancel()

	var task models.OciCreateTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return err
	}
	expectedStatus := "running"
	if once {
		expectedStatus = "pending"
	}
	if task.Status != expectedStatus {
		return fmt.Errorf("任务状态为 %s，无法执行", task.Status)
	}

	var executionErr error
	terminal := once
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
		var invalidConfig bool
		executionErr, invalidConfig = s.createInstance(ctx, task)
		terminal = terminal || invalidConfig
	case <-ctx.Done():
		executionErr = ctx.Err()
	}

	if err := s.recordResult(task, executionErr, terminal); err != nil {
		// A successful OCI launch with a failed database write must not be
		// retried automatically in this process.
		execution.cancel()
		return fmt.Errorf("保存任务执行结果失败: %w", errors.Join(executionErr, err))
	}
	return executionErr
}

func (s *TaskService) createInstance(ctx context.Context, task models.OciCreateTask) (error, bool) {
	var user models.OciUser
	if err := s.db.First(&user, "id = ?", task.UserID).Error; err != nil {
		return fmt.Errorf("配置不存在或无法读取: %w", err), true
	}
	var sshKey models.SSHKey
	if err := s.db.First(&sshKey, "id = ?", task.SSHKeyID).Error; err != nil {
		return fmt.Errorf("SSH密钥不存在或无法读取: %w", err), true
	}
	if err := ctx.Err(); err != nil {
		return err, false
	}
	return s.ociService.CreateInstance(ctx, &user, task.OciRegion, task.Architecture, task.OperationSystem,
		task.Ocpus, task.Memory, task.Disk, task.BootVolumeVpu, sshKey.PublicKey, task.ImageId), false
}

// Persist only execution fields. Save could overwrite a concurrent stop or
// reinsert a deleted task. Status changes are conditional, while a known OCI
// success is still recorded after cancellation. Logs and counters commit together.
func (s *TaskService) recordResult(task models.OciCreateTask, executionErr error, terminal bool) error {
	status, logStatus, message := "completed", "success", "实例创建成功"
	if executionErr != nil {
		status, logStatus, message = "running", "error", extractOCIErrorMessage(executionErr)
		if terminal {
			status = "error"
		}
	}
	now := time.Now()
	updates := map[string]interface{}{
		"execute_count":     gorm.Expr("execute_count + 1"),
		"last_execute_time": now,
		"last_message":      message,
		"status":            gorm.Expr("CASE WHEN status = ? THEN ? ELSE status END", task.Status, status),
	}
	if executionErr == nil {
		updates["success_count"] = gorm.Expr("success_count + 1")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.OciCreateTask{}).Where("id = ?", task.ID).Updates(updates)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return tx.Create(&models.TaskLog{
			ID: uuid.New().String(), TaskID: task.ID, Status: logStatus, Message: message, ExecuteTime: now,
		}).Error
	})
}

func (s *TaskService) cancelTaskLocked(taskID string) {
	if execution := s.tasks[taskID]; execution != nil {
		execution.cancel()
		if execution.timer != nil {
			execution.timer.Stop()
		}
		if !execution.active {
			delete(s.tasks, taskID)
		}
	}
}

func (s *TaskService) AddTask(task *models.OciCreateTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return ErrTaskServiceStopped
	}
	if s.tasks[task.ID] != nil {
		return ErrTaskBusy
	}
	if err := s.db.Create(task).Error; err != nil {
		return err
	}
	s.scheduleTaskLocked(*task)
	return nil
}

func (s *TaskService) StartTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return ErrTaskServiceStopped
	}
	if execution := s.tasks[taskID]; execution != nil {
		if execution.ctx.Err() != nil || execution.active {
			return ErrTaskBusy
		}
		return nil
	}
	var task models.OciCreateTask
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return err
	}
	if err := s.db.Model(&task).Update("status", "running").Error; err != nil {
		return err
	}
	task.Status = "running"
	s.scheduleTaskLocked(task)
	return nil
}

func (s *TaskService) StopTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&models.OciCreateTask{}).Where("id = ?", taskID).Update("status", "stopped")
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	s.cancelTaskLocked(taskID)
	return nil
}

func (s *TaskService) DeleteTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", taskID).Delete(&models.TaskLog{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", taskID).Delete(&models.OciCreateTask{}).Error
	})
	if err == nil {
		s.cancelTaskLocked(taskID)
	}
	return err
}

func (s *TaskService) GetTaskLogs(taskID string, page, pageSize int) ([]models.TaskLog, int64, error) {
	var logs []models.TaskLog
	var total int64
	query := s.db.Model(&models.TaskLog{}).Where("task_id = ?", taskID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("execute_time DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *TaskService) ClearTaskLogs(taskID string) error {
	return s.db.Where("task_id = ?", taskID).Delete(&models.TaskLog{}).Error
}

// ExecuteTaskOnce shares admission, cancellation and persistence with scheduled
// work. HTTP request cancellation also reaches the OCI SDK.
func (s *TaskService) ExecuteTaskOnce(ctx context.Context, taskID string) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return ErrTaskServiceStopped
	}
	if s.tasks[taskID] != nil {
		s.mu.Unlock()
		return ErrTaskBusy
	}
	executionCtx, cancel := context.WithCancel(s.ctx)
	stopCancellation := context.AfterFunc(ctx, cancel)
	defer stopCancellation()
	if ctx.Err() != nil {
		cancel()
	}
	execution := &taskExecution{ctx: executionCtx, cancel: cancel, active: true}
	s.tasks[taskID] = execution
	s.workers.Add(1)
	s.mu.Unlock()
	return s.executeTask(taskID, execution, true)
}
