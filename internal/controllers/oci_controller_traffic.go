package controllers

import (
	"context"
	"net/http"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/gin-gonic/gin"
)

// oci_controller_traffic.go — 流量统计查询及其查询条件（区域/实例/VNIC 选项）。

type GetTrafficDataRequest struct {
	ConfigID   string `json:"configId" binding:"required"`
	InstanceID string `json:"instanceId" binding:"required"`
	VnicID     string `json:"vnicId" binding:"required"`
	StartTime  string `json:"startTime" binding:"required"`
	EndTime    string `json:"endTime" binding:"required"`
}

// GetTrafficData 获取流量统计数据
func (oc *OciController) GetTrafficData(c *gin.Context) {
	var req GetTrafficDataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	trafficData, err := oc.ociService.GetTrafficData(ctx, &user, req.VnicID, req.StartTime, req.EndTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(trafficData, "Success"))
}

// GetTrafficCondition 获取流量查询条件（区域和实例列表）
func (oc *OciController) GetTrafficCondition(c *gin.Context) {
	configId := c.Query("configId")
	if configId == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "configId is required"))
		return
	}

	user, ok := oc.loadUser(c, configId)
	if !ok {
		return
	}

	ctx := context.Background()
	condition := models.TrafficCondition{
		Regions:   []models.ValueLabel{},
		Instances: []models.ValueLabel{},
	}

	// 获取租户区域
	tenantInfo, err := oc.ociService.GetTenantInfo(ctx, &user)
	if err == nil {
		for _, region := range tenantInfo.Regions {
			condition.Regions = append(condition.Regions, models.ValueLabel{
				Value: region,
				Label: region,
			})
		}
	}

	// 获取当前区域的实例
	compartmentId := user.OciTenantID
	instances, err := oc.ociService.ListInstances(ctx, &user, compartmentId)
	if err == nil {
		for _, inst := range instances {
			if inst.Id != nil && inst.DisplayName != nil {
				condition.Instances = append(condition.Instances, models.ValueLabel{
					Value: *inst.Id,
					Label: *inst.DisplayName,
				})
			}
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(condition, "Success"))
}

// GetInstanceVnics 获取实例的VNIC列表
func (oc *OciController) GetInstanceVnics(c *gin.Context) {
	configId := c.Query("configId")
	instanceId := c.Query("instanceId")
	if configId == "" || instanceId == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "configId and instanceId are required"))
		return
	}

	user, ok := oc.loadUser(c, configId)
	if !ok {
		return
	}

	ctx := context.Background()
	instanceDetail, err := oc.ociService.GetInstanceDetails(ctx, &user, instanceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	vnics := []models.ValueLabel{}
	for _, vnic := range instanceDetail.VnicList {
		label := vnic.Name
		if label == "" {
			label = vnic.VnicID
		}
		vnics = append(vnics, models.ValueLabel{
			Value: vnic.VnicID,
			Label: label,
		})
	}

	c.JSON(http.StatusOK, models.SuccessResponse(vnics, "Success"))
}

// GetMonthlyTraffic 获取账号级月度流量（总流量 + 每实例明细 + 实际/计费区分）。
// 甲骨文按账号计费，故汇总 compartment 下全部实例；同时返回每实例拆分与计费出站字节。
func (oc *OciController) GetMonthlyTraffic(c *gin.Context) {
	var req struct {
		ConfigID string `json:"configId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	stats, err := oc.ociService.GetMonthlyTrafficStats(ctx, &user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(stats, "Success"))
}

// GetDailyCost 获取账号每日成本（近 N 天，默认 30 天）。
// 用于第一时间发现超免费额度产生的扣费；依赖 USAGE_REPORT_READ 权限。
func (oc *OciController) GetDailyCost(c *gin.Context) {
	var req struct {
		ConfigID string `json:"configId" binding:"required"`
		Days     int    `json:"days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	stats, err := oc.ociService.GetDailyCost(ctx, &user, req.Days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(stats, "Success"))
}
