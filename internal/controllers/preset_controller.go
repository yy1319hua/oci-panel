package controllers

import (
	"net/http"
	"time"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PresetController struct{}

func NewPresetController() *PresetController {
	return &PresetController{}
}

type CreatePresetRequest struct {
	Name            string  `json:"name" binding:"required"`
	Ocpus           float64 `json:"ocpus"`
	Memory          float64 `json:"memory"`
	Disk            int     `json:"disk"`
	BootVolumeVpu   int64   `json:"bootVolumeVpu"`
	Architecture    string  `json:"architecture"`
	OperationSystem string  `json:"operationSystem"`
	ImageID         string  `json:"imageId"`
	SSHKeyID        string  `json:"sshKeyId"`
	Description     string  `json:"description"`
}

func (pc *PresetController) CreatePreset(c *gin.Context) {
	var req CreatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if req.Ocpus <= 0 {
		req.Ocpus = 1
	}
	if req.Memory <= 0 {
		req.Memory = 6
	}
	if req.Disk <= 0 {
		req.Disk = 50
	}
	if req.BootVolumeVpu <= 0 {
		req.BootVolumeVpu = 10
	}
	if req.Architecture == "" {
		req.Architecture = "ARM"
	}
	if req.OperationSystem == "" {
		req.OperationSystem = "Ubuntu"
	}

	preset := &models.InstancePreset{
		ID:              uuid.New().String(),
		Name:            req.Name,
		Ocpus:           req.Ocpus,
		Memory:          req.Memory,
		Disk:            req.Disk,
		BootVolumeVpu:   req.BootVolumeVpu,
		Architecture:    req.Architecture,
		OperationSystem: req.OperationSystem,
		ImageID:         req.ImageID,
		SSHKeyID:        req.SSHKeyID,
		Description:     req.Description,
		CreateTime:      time.Now(),
	}

	if err := database.GetDB().Create(preset).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "创建预设失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(preset, "创建成功"))
}

type UpdatePresetRequest struct {
	ID              string  `json:"id" binding:"required"`
	Name            string  `json:"name" binding:"required"`
	Ocpus           float64 `json:"ocpus"`
	Memory          float64 `json:"memory"`
	Disk            int     `json:"disk"`
	BootVolumeVpu   int64   `json:"bootVolumeVpu"`
	Architecture    string  `json:"architecture"`
	OperationSystem string  `json:"operationSystem"`
	ImageID         string  `json:"imageId"`
	SSHKeyID        string  `json:"sshKeyId"`
	Description     string  `json:"description"`
}

func (pc *PresetController) UpdatePreset(c *gin.Context) {
	var req UpdatePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	var preset models.InstancePreset
	if err := database.GetDB().First(&preset, "id = ?", req.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse(404, "预设不存在"))
		return
	}

	updates := map[string]interface{}{
		"name":             req.Name,
		"ocpus":            req.Ocpus,
		"memory":           req.Memory,
		"disk":             req.Disk,
		"boot_volume_vpu":  req.BootVolumeVpu,
		"architecture":     req.Architecture,
		"operation_system": req.OperationSystem,
		"image_id":         req.ImageID,
		"ssh_key_id":       req.SSHKeyID,
		"description":      req.Description,
	}

	if err := database.GetDB().Model(&preset).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "更新失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "更新成功"))
}

type DeletePresetRequest struct {
	ID string `json:"id" binding:"required"`
}

func (pc *PresetController) DeletePreset(c *gin.Context) {
	var req DeletePresetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := database.GetDB().Delete(&models.InstancePreset{}, "id = ?", req.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "删除失败"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "删除成功"))
}

func (pc *PresetController) ListPresets(c *gin.Context) {
	var presets []models.InstancePreset
	if err := database.GetDB().Order("create_time DESC").Find(&presets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "获取列表失败"))
		return
	}

	keyMap := make(map[string]string)
	var keys []models.SSHKey
	database.GetDB().Find(&keys)
	for _, key := range keys {
		keyMap[key.ID] = key.Name
	}

	list := make([]models.InstancePresetResponse, len(presets))
	for i, p := range presets {
		list[i] = models.InstancePresetResponse{
			ID:              p.ID,
			Name:            p.Name,
			Ocpus:           p.Ocpus,
			Memory:          p.Memory,
			Disk:            p.Disk,
			BootVolumeVpu:   p.BootVolumeVpu,
			Architecture:    p.Architecture,
			OperationSystem: p.OperationSystem,
			ImageID:         p.ImageID,
			SSHKeyID:        p.SSHKeyID,
			SSHKeyName:      keyMap[p.SSHKeyID],
			Description:     p.Description,
			CreateTime:      p.CreateTime.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(list, "success"))
}

func (pc *PresetController) GetPreset(c *gin.Context) {
	id := c.Query("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "缺少ID参数"))
		return
	}

	var preset models.InstancePreset
	if err := database.GetDB().First(&preset, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse(404, "预设不存在"))
		return
	}

	var sshKeyName string
	var key models.SSHKey
	if database.GetDB().First(&key, "id = ?", preset.SSHKeyID).Error == nil {
		sshKeyName = key.Name
	}

	resp := models.InstancePresetResponse{
		ID:              preset.ID,
		Name:            preset.Name,
		Ocpus:           preset.Ocpus,
		Memory:          preset.Memory,
		Disk:            preset.Disk,
		BootVolumeVpu:   preset.BootVolumeVpu,
		Architecture:    preset.Architecture,
		OperationSystem: preset.OperationSystem,
		ImageID:         preset.ImageID,
		SSHKeyID:        preset.SSHKeyID,
		SSHKeyName:      sshKeyName,
		Description:     preset.Description,
		CreateTime:      preset.CreateTime.Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusOK, models.SuccessResponse(resp, "success"))
}
