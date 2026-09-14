package controllers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oracle/oci-go-sdk/v65/core"
)

// oci_controller_config.go — OCI 配置（账号凭据）的列表/增删改与 API 私钥上传。

type UserPageRequest struct {
	Page     int    `json:"page" binding:"required,min=1"`
	PageSize int    `json:"pageSize" binding:"required,min=1,max=100"`
	Username string `json:"username"`
}

type UserPageResponse struct {
	List     []models.OciUserListResponse `json:"list"`
	Total    int64                        `json:"total"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"pageSize"`
}

func (oc *OciController) UserPage(c *gin.Context) {
	var req UserPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	db := database.GetDB()
	var users []models.OciUser
	var total int64

	query := db.Model(&models.OciUser{})
	if req.Username != "" {
		query = query.Where("username LIKE ?", "%"+req.Username+"%")
	}

	query.Count(&total)
	offset := (req.Page - 1) * req.PageSize
	query.Order("create_time DESC").Limit(req.PageSize).Offset(offset).Find(&users)

	responseList := make([]models.OciUserListResponse, len(users))
	cacheEnabled := oc.schedulerService.IsCacheEnabled()

	if cacheEnabled {
		// 从数据库缓存读取
		for i, user := range users {
			tenantCreateTime := ""
			if user.TenantCreateTime != nil {
				tenantCreateTime = user.TenantCreateTime.Format("2006-01-02 15:04:05")
			}
			responseList[i] = models.OciUserListResponse{
				ID:               user.ID,
				Username:         user.Username,
				TenantName:       user.TenantName,
				TenantCreateTime: tenantCreateTime,
				OciTenantID:      user.OciTenantID,
				OciRegion:        user.OciRegion,
				CreateTime:       user.CreateTime.Format("2006-01-02 15:04:05"),
				InstanceCount:    0,
				RunningInstances: 0,
			}

			cache, err := oc.schedulerService.GetConfigCache(user.ID)
			if err == nil {
				responseList[i].InstanceCount = cache.InstanceCount
				responseList[i].RunningInstances = cache.RunningInstances
			}
		}
	} else {
		// 实时获取（并发）
		type instanceCountResult struct {
			index            int
			instanceCount    int
			runningInstances int
		}
		resultChan := make(chan instanceCountResult, len(users))

		// 实时统计：并发查询每个配置的实例数，但限制并发上限并加超时，
		// 避免配置很多时对 OCI 瞬间发起大量请求或因单个 ListInstances 挂起而长时间阻塞。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		sem := make(chan struct{}, 8)
		for i, user := range users {
			idx, u := i, user
			sem <- struct{}{}
			util.Go("UserPage.count", func() {
				defer func() { <-sem }()
				result := instanceCountResult{index: idx}
				// 保证无论是否 panic 都向 channel 发送一个结果，否则下方按数量读取的循环会死锁。
				defer func() { resultChan <- result }()
				instances, err := oc.ociService.ListInstances(ctx, &u, u.OciTenantID)
				if err == nil {
					result.instanceCount = len(instances)
					for _, inst := range instances {
						if inst.LifecycleState == core.InstanceLifecycleStateRunning {
							result.runningInstances++
						}
					}
				}
			})
		}

		for i, user := range users {
			tenantCreateTime := ""
			if user.TenantCreateTime != nil {
				tenantCreateTime = user.TenantCreateTime.Format("2006-01-02 15:04:05")
			}
			responseList[i] = models.OciUserListResponse{
				ID:               user.ID,
				Username:         user.Username,
				TenantName:       user.TenantName,
				TenantCreateTime: tenantCreateTime,
				OciTenantID:      user.OciTenantID,
				OciRegion:        user.OciRegion,
				CreateTime:       user.CreateTime.Format("2006-01-02 15:04:05"),
				InstanceCount:    0,
				RunningInstances: 0,
			}
		}

		for i := 0; i < len(users); i++ {
			result := <-resultChan
			responseList[result.index].InstanceCount = result.instanceCount
			responseList[result.index].RunningInstances = result.runningInstances
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(UserPageResponse{
		List:     responseList,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, "success"))
}

type AddCfgRequest struct {
	Username       string `json:"username" binding:"required"`
	TenantName     string `json:"tenantName" binding:"required"`
	OciTenantID    string `json:"ociTenantId" binding:"required"`
	OciUserID      string `json:"ociUserId" binding:"required"`
	OciFingerprint string `json:"ociFingerprint" binding:"required"`
	OciRegion      string `json:"ociRegion" binding:"required"`
	OciKeyPath     string `json:"ociKeyPath" binding:"required"`
}

func (oc *OciController) AddCfg(c *gin.Context) {
	var req AddCfgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	if _, err := util.KeyFilePath(req.OciKeyPath); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Invalid private key filename"))
		return
	}

	user := models.OciUser{
		ID:             uuid.New().String(),
		Username:       req.Username,
		TenantName:     req.TenantName,
		OciTenantID:    req.OciTenantID,
		OciUserID:      req.OciUserID,
		OciFingerprint: req.OciFingerprint,
		OciRegion:      req.OciRegion,
		OciKeyPath:     req.OciKeyPath,
		CreateTime:     time.Now(),
	}

	// 获取真正的租户名称和创建时间
	ctx := context.Background()
	tenantInfo, err := oc.ociService.GetTenantInfo(ctx, &user)
	if err == nil && tenantInfo != nil {
		if tenantInfo.Name != "" {
			user.TenantName = tenantInfo.Name
		}
		if tenantInfo.CreateTime != "" {
			if parsedTime, err := time.Parse("2006-01-02 15:04:05", tenantInfo.CreateTime); err == nil {
				user.TenantCreateTime = &parsedTime
			}
		}
	}

	if err := database.GetDB().Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to create user"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Configuration added successfully"))
}

type UpdateCfgNameRequest struct {
	ID         string `json:"id" binding:"required"`
	Username   string `json:"username" binding:"required"`
	OciKeyPath string `json:"ociKeyPath"`
}

func (oc *OciController) UpdateCfgName(c *gin.Context) {
	var req UpdateCfgNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	updates := map[string]interface{}{
		"username": req.Username,
	}

	if req.OciKeyPath != "" {
		if _, err := util.KeyFilePath(req.OciKeyPath); err != nil {
			c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Invalid private key filename"))
			return
		}
		updates["oci_key_path"] = req.OciKeyPath
	}

	if err := database.GetDB().Model(&models.OciUser{}).Where("id = ?", req.ID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to update"))
		return
	}
	oc.ociService.InvalidateClientCache(req.ID)

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Updated successfully"))
}

type RemoveCfgRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

func (oc *OciController) RemoveCfg(c *gin.Context) {
	var req RemoveCfgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var users []models.OciUser
	if err := database.GetDB().Where("id IN ?", req.IDs).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to query users"))
		return
	}

	for _, user := range users {
		if user.OciKeyPath != "" {
			if keyPath, err := util.KeyFilePath(user.OciKeyPath); err == nil {
				_ = os.Remove(keyPath)
			}
		}
	}

	if err := database.GetDB().Where("id IN ?", req.IDs).Delete(&models.OciUser{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to delete"))
		return
	}
	for _, user := range users {
		oc.ociService.InvalidateClientCache(user.ID)
	}

	// 配置删除后，其私钥可能已无任何引用（例如同一密钥被多次上传）。
	// 兜底清理一次孤儿密钥，避免敏感文件长期留在磁盘上。
	go services.CleanupOrphanKeys()

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Deleted successfully"))
}

func (oc *OciController) UploadKey(c *gin.Context) {
	const maxPrivateKeySize = 1 << 20
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPrivateKeySize)

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "No file uploaded"))
		return
	}

	if file.Size <= 0 || file.Size > maxPrivateKeySize {
		c.JSON(http.StatusRequestEntityTooLarge, models.ErrorResponse(413, "Private key file is too large"))
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pem" && ext != ".key" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Only .pem or .key files are allowed"))
		return
	}

	keysDir := "./keys"
	if _, err := os.Stat(keysDir); os.IsNotExist(err) {
		if err := os.MkdirAll(keysDir, 0700); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to create keys directory"))
			return
		}
	}
	if err := os.Chmod(keysDir, 0700); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to secure keys directory"))
		return
	}

	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath := filepath.Join(keysDir, filename)

	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to save file"))
		return
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		_ = os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to secure uploaded file"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(filename, "File uploaded successfully"))
}
