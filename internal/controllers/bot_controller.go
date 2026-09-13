package controllers

import (
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type BotController struct {
	instanceService *services.InstanceService
}

func NewBotController(instanceService *services.InstanceService) *BotController {
	return &BotController{instanceService: instanceService}
}

// BotInstance 是 bot 摘要里单个实例的紧凑表示。
type BotInstance struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Shape     string   `json:"shape"`
	PublicIPs []string `json:"publicIps"`
}

// BotConfigSummary 是单个 OCI 配置（租户）的紧凑摘要。
type BotConfigSummary struct {
	ID            string        `json:"id"`
	Username      string        `json:"username"`
	Region        string        `json:"region"`
	InstanceCount int           `json:"instanceCount"`
	RunningCount  int           `json:"runningCount"`
	Instances     []BotInstance `json:"instances"`
}

// BotSummaryResponse 是 /api/bot/summary 的完整响应，便于机器人直接消费。
type BotSummaryResponse struct {
	Time             string             `json:"time"`
	TotalInstances   int                `json:"totalInstances"`
	RunningInstances int                `json:"runningInstances"`
	Configs          []BotConfigSummary `json:"configs"`
}

// Summary 返回所有配置的紧凑只读摘要，供机器人通过 API Token 拉取。
func (bc *BotController) Summary(c *gin.Context) {
	db := database.GetDB()
	var users []models.OciUser
	if err := db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "failed to load configs"))
		return
	}

	resp := BotSummaryResponse{
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Configs: make([]BotConfigSummary, 0, len(users)),
	}

	for _, user := range users {
		cfg := BotConfigSummary{
			ID:        user.ID,
			Username:  user.Username,
			Region:    user.OciRegion,
			Instances: make([]BotInstance, 0),
		}
		instances, err := bc.instanceService.ListInstances(user.ID, user.OciTenantID)
		if err != nil {
			// 单个配置查询失败不影响其余配置，记录为空实例列表
			resp.Configs = append(resp.Configs, cfg)
			continue
		}
		for _, inst := range instances {
			cfg.Instances = append(cfg.Instances, BotInstance{
				ID:        inst.ID,
				Name:      inst.DisplayName,
				State:     inst.State,
				Shape:     inst.Shape,
				PublicIPs: inst.PublicIPs,
			})
			resp.TotalInstances++
			cfg.InstanceCount++
			if inst.State == "RUNNING" {
				resp.RunningInstances++
				cfg.RunningCount++
			}
		}
		resp.Configs = append(resp.Configs, cfg)
	}

	c.JSON(http.StatusOK, models.SuccessResponse(resp, "success"))
}
