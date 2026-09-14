package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type BotController struct {
	ociService *services.OCIService
}

func NewBotController(ociService *services.OCIService) *BotController {
	return &BotController{ociService: ociService}
}

// BotInstance 是 bot 摘要里单个实例的紧凑表示。
type BotInstance struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Shape     string   `json:"shape"`
	PublicIPs []string `json:"publicIps"`
	IPv6      string   `json:"ipv6"`
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

		// 走与面板一致的链路：先列实例，再并发补详情（含 VNIC 查询）。
		//
		// 此前这里只调 instanceService.ListInstances，它构造 InstanceInfo 时
		// 不填 PublicIPs，导致摘要里公网 IP 恒为空 —— 表现就是机器人在有公网 IP
		// 的实例上仍显示「无公网IP」，而面板同一个实例却能正常显示。
		// 公网 IP / IPv6 只存在于 VNIC 详情里，必须经 GetInstancesDetailsConcurrent
		// 逐个实例查询才能拿到。
		instanceList, err := bc.ociService.ListInstances(context.Background(), &user, user.OciTenantID)
		if err != nil {
			// 单个配置查询失败不影响其余配置，记录为空实例列表
			resp.Configs = append(resp.Configs, cfg)
			continue
		}
		instances := bc.ociService.GetInstancesDetailsConcurrent(context.Background(), &user, instanceList)

		cfg.Instances = buildBotInstances(instances)
		cfg.InstanceCount = len(cfg.Instances)
		for _, inst := range cfg.Instances {
			resp.TotalInstances++
			if inst.State == "RUNNING" {
				resp.RunningInstances++
				cfg.RunningCount++
			}
		}
		resp.Configs = append(resp.Configs, cfg)
	}

	c.JSON(http.StatusOK, models.SuccessResponse(resp, "success"))
}

// buildBotInstances 把内部实例详情裁剪成 bot 摘要所需的紧凑结构。
//
// 单独抽出是为了可测：尤其是 PublicIPs / IPv6 这两个字段 —— 它们只存在于
// VNIC 详情里，历史上曾因直接使用不含 VNIC 信息的精简实例列表而恒为空，
// 导致机器人在有公网 IP 的实例上误报「无公网IP」。
func buildBotInstances(instances []models.InstanceInfo) []BotInstance {
	out := make([]BotInstance, 0, len(instances))
	for _, inst := range instances {
		out = append(out, BotInstance{
			ID:        inst.ID,
			Name:      inst.DisplayName,
			State:     inst.State,
			Shape:     inst.Shape,
			PublicIPs: inst.PublicIPs,
			IPv6:      inst.IPv6,
		})
	}
	return out
}
