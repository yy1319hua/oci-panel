package controllers

import (
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type KeyController struct{}

func NewKeyController() *KeyController {
	return &KeyController{}
}

type CreateKeyRequest struct {
	Name      string `json:"name" binding:"required"`
	PublicKey string `json:"publicKey" binding:"required"`
}

func (kc *KeyController) CreateKey(c *gin.Context) {
	var req CreateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	key := models.SSHKey{
		ID:         uuid.New().String(),
		Name:       req.Name,
		PublicKey:  req.PublicKey,
		KeyType:    "standalone",
		CreateTime: time.Now(),
	}

	if err := database.GetDB().Create(&key).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "创建密钥失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(key, "创建成功"))
}

type KeyPageRequest struct {
	Page     int    `json:"page" form:"page" binding:"required,min=1"`
	PageSize int    `json:"pageSize" form:"pageSize" binding:"required,min=1,max=100"`
	KeyType  string `json:"keyType" form:"keyType"`
	Name     string `json:"name" form:"name"`
}

func (kc *KeyController) ListKeys(c *gin.Context) {
	var req KeyPageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var keys []models.SSHKey
	var total int64

	db := database.GetDB().Model(&models.SSHKey{})

	if req.KeyType != "" {
		db = db.Where("key_type = ?", req.KeyType)
	}
	if req.Name != "" {
		db = db.Where("name LIKE ?", "%"+req.Name+"%")
	}

	db.Count(&total)

	offset := (req.Page - 1) * req.PageSize
	if err := db.Order("create_time DESC").Offset(offset).Limit(req.PageSize).Find(&keys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "查询失败"))
		return
	}

	var responses []models.SSHKeyResponse
	for _, key := range keys {
		resp := models.SSHKeyResponse{
			ID:         key.ID,
			Name:       key.Name,
			PublicKey:  key.PublicKey,
			KeyType:    key.KeyType,
			ConfigID:   key.ConfigID,
			CreateTime: key.CreateTime.Format("2006-01-02 15:04:05"),
		}

		if key.ConfigID != "" {
			var config models.OciUser
			if err := database.GetDB().First(&config, "id = ?", key.ConfigID).Error; err == nil {
				resp.ConfigName = config.Username
			}
		}

		responses = append(responses, resp)
	}

	c.JSON(http.StatusOK, models.SuccessResponse(gin.H{
		"list":  responses,
		"total": total,
		"page":  req.Page,
	}, ""))
}

func (kc *KeyController) GetAllStandaloneKeys(c *gin.Context) {
	var keys []models.SSHKey

	if err := database.GetDB().Where("key_type = ?", "standalone").Order("create_time DESC").Find(&keys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "查询失败"))
		return
	}

	var responses []models.SSHKeyResponse
	for _, key := range keys {
		responses = append(responses, models.SSHKeyResponse{
			ID:         key.ID,
			Name:       key.Name,
			PublicKey:  key.PublicKey,
			KeyType:    key.KeyType,
			CreateTime: key.CreateTime.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, models.SuccessResponse(responses, ""))
}

type UpdateKeyRequest struct {
	ID        string `json:"id" binding:"required"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
}

func (kc *KeyController) UpdateKey(c *gin.Context) {
	var req UpdateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var key models.SSHKey
	if err := database.GetDB().First(&key, "id = ?", req.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse(404, "密钥不存在"))
		return
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.PublicKey != "" {
		updates["public_key"] = req.PublicKey
	}

	if len(updates) > 0 {
		if err := database.GetDB().Model(&key).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "更新失败"))
			return
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "更新成功"))
}

type DeleteKeyRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

func (kc *KeyController) DeleteKey(c *gin.Context) {
	var req DeleteKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := database.GetDB().Where("id IN ? AND key_type = ?", req.IDs, "standalone").Delete(&models.SSHKey{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "删除失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "删除成功"))
}

func (kc *KeyController) GetKeyByID(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "缺少ID参数"))
		return
	}

	var key models.SSHKey
	if err := database.GetDB().First(&key, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse(404, "密钥不存在"))
		return
	}

	resp := models.SSHKeyResponse{
		ID:         key.ID,
		Name:       key.Name,
		PublicKey:  key.PublicKey,
		KeyType:    key.KeyType,
		ConfigID:   key.ConfigID,
		CreateTime: key.CreateTime.Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusOK, models.SuccessResponse(resp, ""))
}
