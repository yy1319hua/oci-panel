package controllers

import (
	"net/http"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/gin-gonic/gin"
)

// 旧创建入口 /oci/createInstance 由 TaskController.CreateTask 处理。

type CreateTaskPageRequest struct {
	Page     int    `json:"page" binding:"required,min=1"`
	PageSize int    `json:"pageSize" binding:"required,min=1,max=100"`
	UserID   string `json:"userId"`
}

type CreateTaskPageResponse struct {
	List     []models.OciCreateTask `json:"list"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"pageSize"`
}

func (oc *OciController) CreateTaskPage(c *gin.Context) {
	var req CreateTaskPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	db := database.GetDB()
	var tasks []models.OciCreateTask
	var total int64

	query := db.Model(&models.OciCreateTask{})
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
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

	c.JSON(http.StatusOK, models.SuccessResponse(CreateTaskPageResponse{
		List:     tasks,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, "success"))
}
