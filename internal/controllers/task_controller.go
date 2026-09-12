package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TaskController struct {
	taskService *services.TaskService
}

func NewTaskController(taskService *services.TaskService) *TaskController {
	return &TaskController{
		taskService: taskService,
	}
}

type CreateTaskRequest struct {
	UserID          string  `json:"userId" binding:"required"`
	OciRegion       string  `json:"ociRegion" binding:"required"`
	Ocpus           float64 `json:"ocpus"`
	Memory          float64 `json:"memory"`
	Disk            int     `json:"disk"`
	BootVolumeVpu   int64   `json:"bootVolumeVpu"`
	Architecture    string  `json:"architecture"`
	OperationSystem string  `json:"operationSystem"`
	ImageId         string  `json:"imageId"`
	SSHKeyID        string  `json:"sshKeyId" binding:"required"`
	Interval        int     `json:"interval"`
	ExecuteOnce     bool    `json:"executeOnce"`
}

func (tc *TaskController) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var sshKey models.SSHKey
	if err := database.GetDB().First(&sshKey, "id = ?", req.SSHKeyID).Error; err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "SSH密钥不存在"))
		return
	}

	var user models.OciUser
	if err := database.GetDB().First(&user, "id = ?", req.UserID).Error; err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "配置不存在"))
		return
	}

	if req.Interval < 10 {
		req.Interval = 60
	}
	if req.Ocpus <= 0 {
		req.Ocpus = 1
	}
	if req.Memory <= 0 {
		req.Memory = 6
	}
	if req.Disk <= 0 {
		req.Disk = 50
	}
	if req.BootVolumeVpu <= 0 {
		req.BootVolumeVpu = 10
	}
	if req.Architecture == "" {
		req.Architecture = "ARM"
	}
	if req.OperationSystem == "" {
		req.OperationSystem = "Ubuntu"
	}

	// 如果是只执行一次，状态设置为 pending，执行后变为 completed 或 error
	status := "running"
	if req.ExecuteOnce {
		status = "pending"
	}

	task := &models.OciCreateTask{
		ID:              uuid.New().String(),
		UserID:          req.UserID,
		Username:        user.Username,
		OciRegion:       req.OciRegion,
		Ocpus:           req.Ocpus,
		Memory:          req.Memory,
		Disk:            req.Disk,
		BootVolumeVpu:   req.BootVolumeVpu,
		Architecture:    req.Architecture,
		OperationSystem: req.OperationSystem,
		ImageId:         req.ImageId,
		SSHKeyID:        req.SSHKeyID,
		Interval:        req.Interval,
		Status:          status,
		CreateTime:      time.Now(),
	}

	if err := tc.taskService.AddTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "创建任务失败"))
		return
	}

	if req.ExecuteOnce {
		// 立即执行一次
		if err := tc.taskService.ExecuteTaskOnce(c.Request.Context(), task.ID); err != nil {
			c.JSON(http.StatusOK, models.ErrorResponse(500, err.Error()))
			return
		}
		if err := database.GetDB().First(task, "id = ?", task.ID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "读取任务结果失败"))
			return
		}
		c.JSON(http.StatusOK, models.SuccessResponse(task, "实例创建成功"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(task, "任务创建成功"))
}

type TaskPageRequest struct {
	Page     int    `json:"page" binding:"required,min=1"`
	PageSize int    `json:"pageSize" binding:"required,min=1,max=100"`
	Status   string `json:"status"`
}

type TaskPageResponse struct {
	List     []models.TaskListResponse `json:"list"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
}

func (tc *TaskController) TaskList(c *gin.Context) {
	var req TaskPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	db := database.GetDB()
	var tasks []models.OciCreateTask
	var total int64

	query := db.Model(&models.OciCreateTask{})
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "查询任务失败"))
		return
	}
	offset := (req.Page - 1) * req.PageSize
	if err := query.Order("create_time DESC").Limit(req.PageSize).Offset(offset).Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "查询任务失败"))
		return
	}

	list := make([]models.TaskListResponse, len(tasks))
	for i, t := range tasks {
		lastExecuteTime := ""
		if t.LastExecuteTime != nil {
			lastExecuteTime = t.LastExecuteTime.Format("2006-01-02 15:04:05")
		}
		list[i] = models.TaskListResponse{
			ID:              t.ID,
			UserID:          t.UserID,
			Username:        t.Username,
			OciRegion:       t.OciRegion,
			Ocpus:           t.Ocpus,
			Memory:          t.Memory,
			Disk:            t.Disk,
			Architecture:    t.Architecture,
			Interval:        t.Interval,
			OperationSystem: t.OperationSystem,
			Status:          t.Status,
			ExecuteCount:    t.ExecuteCount,
			SuccessCount:    t.SuccessCount,
			LastExecuteTime: lastExecuteTime,
			LastMessage:     t.LastMessage,
			CreateTime:      t.CreateTime.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(TaskPageResponse{
		List:     list,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, "success"))
}

type TaskActionRequest struct {
	TaskID string `json:"taskId" binding:"required"`
}

func (tc *TaskController) StartTask(c *gin.Context) {
	var req TaskActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := tc.taskService.StartTask(req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "任务已启动"))
}

func (tc *TaskController) StopTask(c *gin.Context) {
	var req TaskActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := tc.taskService.StopTask(req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "任务已停止"))
}

func (tc *TaskController) DeleteTask(c *gin.Context) {
	var req TaskActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := tc.taskService.DeleteTask(req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "任务已删除"))
}

type BatchDeleteTaskRequest struct {
	TaskIDs []string `json:"taskIds" binding:"required"`
}

func (tc *TaskController) BatchDeleteTask(c *gin.Context) {
	var req BatchDeleteTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var failedCount int
	for _, taskID := range req.TaskIDs {
		if err := tc.taskService.DeleteTask(taskID); err != nil {
			failedCount++
		}
	}

	if failedCount > 0 {
		c.JSON(http.StatusOK, models.SuccessResponse(nil, fmt.Sprintf("删除完成，%d个失败", failedCount)))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "批量删除成功"))
}

type TaskLogsRequest struct {
	TaskID   string `json:"taskId" binding:"required"`
	Page     int    `json:"page" binding:"required,min=1"`
	PageSize int    `json:"pageSize" binding:"required,min=1,max=100"`
}

type TaskLogsResponse struct {
	List     []models.TaskLog `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}

func (tc *TaskController) TaskLogs(c *gin.Context) {
	var req TaskLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	logs, total, err := tc.taskService.GetTaskLogs(req.TaskID, req.Page, req.PageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(TaskLogsResponse{
		List:     logs,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, "success"))
}

func (tc *TaskController) ClearTaskLogs(c *gin.Context) {
	var req TaskActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := tc.taskService.ClearTaskLogs(req.TaskID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "日志已清空"))
}
