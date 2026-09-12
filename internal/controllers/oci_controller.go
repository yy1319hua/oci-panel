package controllers

import (
	"net/http"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

// oci_controller.go — OciController 的核心：依赖装配与共享辅助。
// 各业务域的 handler 按域拆分到 oci_controller_<domain>.go（同一 package，方法集不变）。

type OciController struct {
	ociService       *services.OCIService
	schedulerService *services.SchedulerService
}

func NewOciController(ociService *services.OCIService, schedulerService *services.SchedulerService) *OciController {
	return &OciController{
		ociService:       ociService,
		schedulerService: schedulerService,
	}
}

// loadUser 按配置 ID 查询 OciUser；未找到时写入 404 响应并返回 ok=false。
// 用于消除各 handler 中重复的「取 DB → 查 user → 404」前导（原 ~18 处复制粘贴）。
func (oc *OciController) loadUser(c *gin.Context, configID any) (models.OciUser, bool) {
	var user models.OciUser
	if err := database.GetDB().Where("id = ?", configID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse(404, "Configuration not found"))
		return user, false
	}
	return user, true
}
