package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/gin-gonic/gin"
)

// oci_controller_resource.go — 单个配置的资源读取：配置详情、租户信息、实例/卷/VCN 列表、缓存刷新。
// 这些 handler 共享 GetResourceRequest 与「先读数据库缓存、未命中再实时拉取」的统一模式。

type GetConfigDetailsRequest struct {
	ConfigID string `json:"configId" binding:"required"`
}

func (oc *OciController) GetConfigDetails(c *gin.Context) {
	var req GetConfigDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	details := models.OciConfigDetails{
		UserID:      user.ID,
		Username:    user.Username,
		TenantID:    user.OciTenantID,
		TenantName:  user.TenantName,
		Fingerprint: user.OciFingerprint,
		KeyPath:     filepath.Base(user.OciKeyPath),
		Region:      user.OciRegion,
		CreateTime:  user.CreateTime.Format("2006-01-02 15:04:05"),
		Instances:   []models.InstanceInfo{},
		Volumes:     []models.VolumeInfo{},
		VCNs:        []models.VCNInfo{},
	}

	c.JSON(http.StatusOK, models.SuccessResponse(details, "Success"))
}

type GetResourceRequest struct {
	ConfigID   string `json:"configId" binding:"required"`
	ClearCache bool   `json:"clearCache"`
}

// GetConfigInstances 获取配置的实例列表
func (oc *OciController) GetConfigInstances(c *gin.Context) {
	var req GetResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	// 如果需要刷新缓存，先更新
	if req.ClearCache {
		oc.schedulerService.UpdateConfigCache(req.ConfigID)
	}

	// 尝试从数据库缓存获取
	if oc.schedulerService.IsCacheEnabled() {
		cache, err := oc.schedulerService.GetConfigCache(req.ConfigID)
		if err == nil && cache.InstancesData != "" {
			var instances []models.InstanceInfo
			if json.Unmarshal([]byte(cache.InstancesData), &instances) == nil {
				c.JSON(http.StatusOK, models.SuccessResponse(instances, "Success (cached)"))
				return
			}
		}
	}

	// 实时获取
	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	compartmentId := user.OciTenantID

	instances := []models.InstanceInfo{}
	instanceList, err := oc.ociService.ListInstances(ctx, &user, compartmentId)
	if err == nil {
		// 并发获取各实例详情（带并发上限），替代此前逐实例串行的 N×(6~8) 往返。
		instances = oc.ociService.GetInstancesDetailsConcurrent(ctx, &user, instanceList)
	}

	c.JSON(http.StatusOK, models.SuccessResponse(instances, "Success"))
}

// GetConfigVolumes 获取配置的存储卷列表
func (oc *OciController) GetConfigVolumes(c *gin.Context) {
	var req GetResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if req.ClearCache {
		oc.schedulerService.UpdateConfigCache(req.ConfigID)
	}

	// 尝试从数据库缓存获取
	if oc.schedulerService.IsCacheEnabled() {
		cache, err := oc.schedulerService.GetConfigCache(req.ConfigID)
		if err == nil && cache.VolumesData != "" {
			var volumes []models.VolumeInfo
			if json.Unmarshal([]byte(cache.VolumesData), &volumes) == nil {
				c.JSON(http.StatusOK, models.SuccessResponse(volumes, "Success (cached)"))
				return
			}
		}
	}

	// 实时获取
	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	compartmentId := user.OciTenantID

	volumes, err := oc.ociService.ListBootVolumes(ctx, &user, compartmentId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(volumes, "Success"))
}

// GetConfigVCNs 获取配置的VCN列表
func (oc *OciController) GetConfigVCNs(c *gin.Context) {
	var req GetResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if req.ClearCache {
		oc.schedulerService.UpdateConfigCache(req.ConfigID)
	}

	// 尝试从数据库缓存获取
	if oc.schedulerService.IsCacheEnabled() {
		cache, err := oc.schedulerService.GetConfigCache(req.ConfigID)
		if err == nil && cache.VcnsData != "" {
			var vcns []models.VCNInfo
			if json.Unmarshal([]byte(cache.VcnsData), &vcns) == nil {
				c.JSON(http.StatusOK, models.SuccessResponse(vcns, "Success (cached)"))
				return
			}
		}
	}

	// 实时获取
	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	compartmentId := user.OciTenantID

	vcns, err := oc.ociService.ListVCNs(ctx, &user, compartmentId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(vcns, "Success"))
}

// ClearConfigCache 刷新配置的缓存
func (oc *OciController) ClearConfigCache(c *gin.Context) {
	var req GetConfigDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	// 重新获取并更新缓存
	util.Go("UpdateConfigCache", func() { oc.schedulerService.UpdateConfigCache(req.ConfigID) })

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Cache refresh started"))
}

// GetTenantInfo 获取租户详情
func (oc *OciController) GetTenantInfo(c *gin.Context) {
	var req GetResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if req.ClearCache {
		oc.schedulerService.UpdateConfigCache(req.ConfigID)
	}

	// 尝试从数据库缓存获取
	if oc.schedulerService.IsCacheEnabled() {
		cache, err := oc.schedulerService.GetConfigCache(req.ConfigID)
		if err == nil && cache.TenantData != "" {
			var tenantInfo models.TenantInfo
			if json.Unmarshal([]byte(cache.TenantData), &tenantInfo) == nil {
				c.JSON(http.StatusOK, models.SuccessResponse(tenantInfo, "Success (cached)"))
				return
			}
		}
	}

	// 实时获取
	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}

	ctx := context.Background()
	tenantInfo, err := oc.ociService.GetTenantInfo(ctx, &user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(tenantInfo, "Success"))
}
