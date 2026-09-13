package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

// trafficCacheTTL 是流量缓存的兜底有效期。
//
// 为什么不复用「缓存间隔」设置：月度流量要按实例逐个查询 VNIC 并打 3 次
// Monitoring API，单次成本远高于实例/卷列表，因此允许更长的保鲜期。
//
// 这个值与调度器的刷新节奏必须满足：trafficCacheTTL >= 缓存间隔 × TrafficRefreshFactor，
// 否则会出现「调度器还没到该刷新的时间，控制器却已判定缓存过期」的错配 ——
// 结果是每个请求都穿透到实时查询（约 10s），首页慢的问题原地复现。
// 默认配置（间隔 30 分钟 × 6 = 180 分钟）下取 4 小时，留出足够余量。
// 详见 scheduler_service.go 的 TrafficRefreshFactor。
const trafficCacheTTL = 4 * time.Hour

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
//
// 缓存策略（修复首页「每次打开都要等很久」）：该接口每次调用会对每个实例
// 发起 5+ 次 OCI 网络请求，单次耗时约 10 秒。因此：
//   - 缓存开启且命中未过期缓存 → 直接返回，耗时 ~2ms；
//   - 未命中 → 实时查询，并回写缓存（下次即可命中）；
//   - 用户显式传 forceRefresh=true（首页「刷新」按钮）→ 跳过缓存强制实时查询。
//
// 注意：缓存命中时也**不要**在响应路径里同步回写，避免把慢查询重新拉回请求链路。
func (oc *OciController) GetMonthlyTraffic(c *gin.Context) {
	var req struct {
		ConfigID     string `json:"configId" binding:"required"`
		ForceRefresh bool   `json:"forceRefresh"`
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
	cacheEnabled := oc.schedulerService != nil && oc.schedulerService.IsCacheEnabled()

	// 1) 缓存优先：命中且未过期则直接返回。
	if cacheEnabled && !req.ForceRefresh {
		if cache, err := oc.schedulerService.GetConfigCache(req.ConfigID); err == nil &&
			cache.TrafficData != "" &&
			time.Since(cache.TrafficUpdateTime) < trafficCacheTTL {
			var cached services.MonthlyTrafficStats
			if json.Unmarshal([]byte(cache.TrafficData), &cached) == nil {
				c.JSON(http.StatusOK, models.SuccessResponse(cached, "Success (cached)"))
				return
			}
			// 缓存内容损坏：继续走实时查询，避免把坏数据一直透给前端。
		}
	}

	// 2) 实时查询。
	stats, err := oc.ociService.GetMonthlyTrafficStats(ctx, &user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	// 3) 回写缓存，让下一次打开首页直接命中。
	//
	// 这里必须用**列级更新**（UpdateColumn）而不是读出整行再 Save：
	// 调度器的 UpdateConfigCache 走的是「读出 → 改内存 → 整行 Save」，
	// 若此处也用整行 Save，两个写入方并发时会互相覆盖对方刚写入的字段
	// （经典 lost update：调度器把我刚写的流量覆盖回旧值，或我把它的实例
	// 列表覆盖回旧值）。列级更新只触碰 traffic_* 两列，天然互不干扰。
	if cacheEnabled {
		oc.schedulerService.SaveTrafficCache(req.ConfigID, stats)
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
