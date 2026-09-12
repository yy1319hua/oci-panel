package services

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeInstanceCreator struct{ create func(context.Context) error }

func (f fakeInstanceCreator) CreateInstance(ctx context.Context, _ *models.OciUser, _, _, _ string, _, _ float64, _ int, _ int64, _, _ string) error {
	return f.create(ctx)
}

func newTaskTestService(t *testing.T, create func(context.Context) error) *TaskService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tasks.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.OciUser{}, &models.SSHKey{}, &models.OciCreateTask{}, &models.TaskLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.OciUser{ID: "config"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SSHKey{ID: "key", Name: "test", PublicKey: "public-key", KeyType: "standalone"}).Error; err != nil {
		t.Fatal(err)
	}
	s := NewTaskService(db, fakeInstanceCreator{create: create})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s
}

func addTestTask(t *testing.T, s *TaskService, status string) models.OciCreateTask {
	t.Helper()
	task := models.OciCreateTask{ID: uuid.NewString(), UserID: "config", SSHKeyID: "key", Status: status, Interval: 3600}
	if err := s.AddTask(&task); err != nil {
		t.Fatal(err)
	}
	return task
}

// Trigger the real timer callback without waiting for the production interval.
func triggerTask(t *testing.T, s *TaskService, taskID string) <-chan struct{} {
	t.Helper()
	s.mu.Lock()
	execution := s.tasks[taskID]
	if execution == nil || execution.timer == nil {
		s.mu.Unlock()
		t.Fatal("task was not scheduled")
	}
	execution.timer.Stop()
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.executeScheduled(taskID, execution)
	}()
	return done
}

func awaitTask(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not finish")
	}
}

func readTestTask(t *testing.T, s *TaskService, id string) models.OciCreateTask {
	t.Helper()
	var task models.OciCreateTask
	if err := s.db.First(&task, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	return task
}

func TestTaskResultCannotUndoStopOrDelete(t *testing.T) {
	for _, mode := range []string{"running", "pending"} {
		for _, action := range []string{"stop", "delete"} {
			for _, succeeds := range []bool{false, true} {
				name := mode + "/" + action + "/failure"
				if succeeds {
					name = mode + "/" + action + "/success"
				}
				t.Run(name, func(t *testing.T) {
					started := make(chan struct{})
					release := make(chan struct{})
					unblock := sync.OnceFunc(func() { close(release) })
					defer unblock()
					s := newTaskTestService(t, func(ctx context.Context) error {
						close(started)
						// Model an OCI response arriving after a cancellation request.
						<-release
						if ctx.Err() == nil {
							t.Error("stop/delete did not cancel the OCI request")
						}
						if succeeds {
							return nil
						}
						return errors.New("capacity unavailable")
					})
					task := addTestTask(t, s, mode)
					var done <-chan struct{}
					if mode == "running" {
						done = triggerTask(t, s, task.ID)
					} else {
						finished := make(chan struct{})
						done = finished
						go func() {
							defer close(finished)
							s.ExecuteTaskOnce(context.Background(), task.ID)
						}()
					}
					awaitTask(t, started)
					if action == "stop" {
						if err := s.StopTask(task.ID); err != nil {
							t.Fatal(err)
						}
						if err := s.StartTask(task.ID); !errors.Is(err, ErrTaskBusy) {
							t.Fatalf("overlapping restart accepted: %v", err)
						}
					} else if err := s.DeleteTask(task.ID); err != nil {
						t.Fatal(err)
					}
					unblock()
					awaitTask(t, done)
					logs, total, err := s.GetTaskLogs(task.ID, 1, 10)
					if err != nil {
						t.Fatal(err)
					}
					if action == "delete" {
						var deleted models.OciCreateTask
						if err := s.db.First(&deleted, "id = ?", task.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
							t.Fatalf("deleted task was resurrected: %v", err)
						}
						if total != 0 {
							t.Fatalf("deleted task has orphan logs: %d", total)
						}
					} else {
						stored := readTestTask(t, s, task.ID)
						if stored.Status != "stopped" || stored.ExecuteCount != 1 || total != 1 || len(logs) != 1 {
							t.Fatalf("unexpected stopped task: %+v; logs=%d", stored, total)
						}
						if succeeds && stored.SuccessCount != 1 {
							t.Fatal("lost known OCI success after stop")
						}
					}
					s.mu.Lock()
					defer s.mu.Unlock()
					if s.tasks[task.ID] != nil {
						t.Fatal("canceled task was rescheduled")
					}
				})
			}
		}
	}
}

func TestTaskRetriesAndStopsOnSuccess(t *testing.T) {
	var calls atomic.Int32
	s := newTaskTestService(t, func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("capacity unavailable")
		}
		return nil
	})
	task := addTestTask(t, s, "running")
	awaitTask(t, triggerTask(t, s, task.ID))
	first := readTestTask(t, s, task.ID)
	if first.Status != "running" || first.ExecuteCount != 1 {
		t.Fatalf("failed attempt was not recorded: %+v", first)
	}
	awaitTask(t, triggerTask(t, s, task.ID))
	completed := readTestTask(t, s, task.ID)
	if completed.Status != "completed" || completed.ExecuteCount != 2 || completed.SuccessCount != 1 {
		t.Fatalf("unexpected completed task: %+v", completed)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[task.ID] != nil {
		t.Fatal("completed task was scheduled again")
	}
}

func TestStaleTimerCannotExecuteRestartedTask(t *testing.T) {
	var calls atomic.Int32
	s := newTaskTestService(t, func(context.Context) error { calls.Add(1); return nil })
	task := addTestTask(t, s, "running")
	s.mu.Lock()
	stale := s.tasks[task.ID]
	s.mu.Unlock()
	if err := s.StopTask(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.StartTask(task.ID); err != nil {
		t.Fatal(err)
	}
	s.executeScheduled(task.ID, stale)
	if calls.Load() != 0 {
		t.Fatal("old timer callback executed after restart")
	}
	awaitTask(t, triggerTask(t, s, task.ID))
	if calls.Load() != 1 {
		t.Fatal("restarted task did not execute exactly once")
	}
}

func TestTaskServiceStopDrainsWorkersAndCanRestart(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	s := newTaskTestService(t, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	})
	task := addTestTask(t, s, "running")
	done := triggerTask(t, s, task.ID)
	awaitTask(t, started)
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	awaitTask(t, canceled)
	select {
	case <-stopped:
		t.Fatal("Stop returned before its worker exited")
	default:
	}
	if err := s.StartTask(task.ID); !errors.Is(err, ErrTaskServiceStopped) {
		t.Fatalf("task admitted during shutdown: %v", err)
	}
	unblock()
	awaitTask(t, stopped)
	awaitTask(t, done)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) != 1 || s.tasks[task.ID] == nil {
		t.Fatal("restart did not reload the persisted running task")
	}
}

func TestConcurrentOneOffTaskIsRejected(t *testing.T) {
	started := make(chan struct{})
	s := newTaskTestService(t, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	task := addTestTask(t, s, "pending")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.ExecuteTaskOnce(ctx, task.ID)
	}()
	awaitTask(t, started)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.ExecuteTaskOnce(context.Background(), task.ID); !errors.Is(err, ErrTaskBusy) {
				t.Errorf("duplicate execution accepted: %v", err)
			}
		}()
	}
	wg.Wait()
	cancel()
	awaitTask(t, done)
	stored := readTestTask(t, s, task.ID)
	if stored.Status != "error" || stored.ExecuteCount != 1 {
		t.Fatalf("request cancellation left an unfinished one-off task: %+v", stored)
	}
}

func TestInvalidTaskConfigurationIsTerminal(t *testing.T) {
	for _, missing := range []string{"config", "key"} {
		t.Run(missing, func(t *testing.T) {
			s := newTaskTestService(t, func(context.Context) error { t.Error("OCI called with invalid configuration"); return nil })
			task := addTestTask(t, s, "running")
			var err error
			if missing == "config" {
				err = s.db.Delete(&models.OciUser{}, "id = ?", task.UserID).Error
			} else {
				err = s.db.Delete(&models.SSHKey{}, "id = ?", task.SSHKeyID).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			awaitTask(t, triggerTask(t, s, task.ID))
			if stored := readTestTask(t, s, task.ID); stored.Status != "error" || stored.LastMessage == "" {
				t.Fatalf("invalid configuration left a running task: %+v", stored)
			}
		})
	}
}

func TestTaskResultAndLogAreAtomic(t *testing.T) {
	for _, status := range []string{"pending", "running"} {
		t.Run(status, func(t *testing.T) {
			s := newTaskTestService(t, func(context.Context) error { return nil })
			task := addTestTask(t, s, status)
			if err := s.db.Exec("CREATE TRIGGER fail_log BEFORE INSERT ON task_log BEGIN SELECT RAISE(ABORT, 'log unavailable'); END").Error; err != nil {
				t.Fatal(err)
			}
			if status == "pending" {
				if err := s.ExecuteTaskOnce(context.Background(), task.ID); err == nil {
					t.Fatal("log persistence failure was ignored")
				}
			} else {
				awaitTask(t, triggerTask(t, s, task.ID))
			}
			stored := readTestTask(t, s, task.ID)
			if stored.Status != status || stored.ExecuteCount != 0 || stored.SuccessCount != 0 {
				t.Fatalf("partial result committed without a log: %+v", stored)
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.tasks[task.ID] != nil {
				t.Fatal("OCI success with a failed database write was rescheduled")
			}
		})
	}
}

func TestTaskConcurrencyIsBoundedAndQueuedWorkCanCancel(t *testing.T) {
	started := make(chan struct{}, 6)
	var calls atomic.Int32
	s := newTaskTestService(t, func(ctx context.Context) error {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	var tasks []models.OciCreateTask
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		task := addTestTask(t, s, "pending")
		tasks = append(tasks, task)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.ExecuteTaskOnce(context.Background(), task.ID)
		}()
	}
	for i := 0; i < 5; i++ {
		awaitTask(t, started)
	}
	s.Stop()
	wg.Wait()
	if calls.Load() != 5 {
		t.Fatalf("got %d OCI calls, want five active and one canceled in queue", calls.Load())
	}
	for _, task := range tasks {
		stored := readTestTask(t, s, task.ID)
		// A request rejected before admission has not started; admitted work must
		// leave pending state even when canceled while waiting for a slot.
		if stored.ExecuteCount > 0 && stored.Status != "error" {
			t.Fatalf("canceled admitted task stayed pending: %+v", stored)
		}
	}
}

func TestDeleteTaskRollsBackLogsOnFailure(t *testing.T) {
	s := newTaskTestService(t, func(context.Context) error { return nil })
	task := addTestTask(t, s, "running")
	if err := s.db.Create(&models.TaskLog{ID: "log", TaskID: task.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Exec("CREATE TRIGGER fail_delete BEFORE DELETE ON oci_create_task BEGIN SELECT RAISE(ABORT, 'delete unavailable'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTask(task.ID); err == nil {
		t.Fatal("delete failure was ignored")
	}
	_, total, err := s.GetTaskLogs(task.ID, 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("failed task deletion removed logs: total=%d, err=%v", total, err)
	}
	readTestTask(t, s, task.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tasks[task.ID] == nil || s.tasks[task.ID].ctx.Err() != nil {
		t.Fatal("failed deletion canceled a live task")
	}
}
