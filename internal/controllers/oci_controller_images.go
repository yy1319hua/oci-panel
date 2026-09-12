package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

// oci_controller_images.go — 可用镜像列表查询（带按 区域+架构 维度的数据库缓存）。

// ListImagesRequest 获取镜像列表请求
type ListImagesRequest struct {
	ConfigID     string `json:"configId" binding:"required"`
	Region       string `json:"region" binding:"required"`
	Architecture string `json:"architecture" binding:"required"`
	ClearCache   bool   `json:"clearCache"`
}

// ListImages 获取可用镜像列表
func (oc *OciController) ListImages(c *gin.Context) {
	var req ListImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.ConfigID)
	if !ok {
		return
	}
	db := database.GetDB()

	cacheKey := req.Region + "_" + req.Architecture

	// 尝试从缓存获取
	if !req.ClearCache {
		var cache models.OciImageCache
		if err := db.Where("region = ? AND architecture = ?", req.Region, req.Architecture).First(&cache).Error; err == nil {
			if cache.ImagesData != "" {
				var images []services.ImageInfo
				if json.Unmarshal([]byte(cache.ImagesData), &images) == nil {
					c.JSON(http.StatusOK, models.SuccessResponse(images, "获取镜像列表成功(缓存)"))
					return
				}
			}
		}
	}

	ctx := context.Background()
	images, err := oc.ociService.ListImages(ctx, &user, req.Region, req.Architecture)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	// 保存到缓存
	imagesJson, _ := json.Marshal(images)
	var cache models.OciImageCache
	result := db.Where("region = ? AND architecture = ?", req.Region, req.Architecture).First(&cache)
	if result.Error != nil {
		cache = models.OciImageCache{
			ID:           cacheKey,
			Region:       req.Region,
			Architecture: req.Architecture,
		}
	}
	cache.ImagesData = string(imagesJson)
	cache.UpdateTime = time.Now()
	db.Save(&cache)

	c.JSON(http.StatusOK, models.SuccessResponse(images, "获取镜像列表成功"))
}
