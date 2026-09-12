package controllers

import (
	"context"
	"net/http"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/util"
	"github.com/gin-gonic/gin"
)

// oci_controller_iam.go — 租户 IAM 用户管理：密码过期策略、用户信息、删除用户、重置密码、清除 MFA/API 密钥。

type UpdatePasswordExpiryRequest struct {
	CfgID                string `json:"cfgId" binding:"required"`
	PasswordExpiresAfter int    `json:"passwordExpiresAfter"`
}

type UserManagementRequest struct {
	OciCfgID string `json:"ociCfgId" binding:"required"`
	UserID   string `json:"userId" binding:"required"`
}

type UpdateUserInfoRequest struct {
	OciCfgID    string `json:"ociCfgId" binding:"required"`
	UserID      string `json:"userId" binding:"required"`
	Email       string `json:"email" binding:"required"`
	DbUserName  string `json:"dbUserName" binding:"required"`
	Description string `json:"description"`
}

// UpdatePasswordExpiry 更新密码过期策略
func (oc *OciController) UpdatePasswordExpiry(c *gin.Context) {
	var req UpdatePasswordExpiryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.CfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.UpdatePasswordExpiresAfter(ctx, &user, req.PasswordExpiresAfter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	// 更新缓存
	util.Go("UpdateConfigCache", func() { oc.schedulerService.UpdateConfigCache(req.CfgID) })

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "密码过期策略已更新"))
}

// UpdateUserInfo 更新用户信息
func (oc *OciController) UpdateUserInfo(c *gin.Context) {
	var req UpdateUserInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.OciCfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.UpdateUserInfo(ctx, &user, req.UserID, req.Email, req.DbUserName, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	util.Go("UpdateConfigCache", func() { oc.schedulerService.UpdateConfigCache(req.OciCfgID) })

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "用户信息更新成功"))
}

// DeleteUser 删除用户
func (oc *OciController) DeleteUser(c *gin.Context) {
	var req UserManagementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.OciCfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.DeleteUser(ctx, &user, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	util.Go("UpdateConfigCache", func() { oc.schedulerService.UpdateConfigCache(req.OciCfgID) })

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "用户删除成功"))
}

// ResetPassword 重置用户密码
func (oc *OciController) ResetPassword(c *gin.Context) {
	var req UserManagementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.OciCfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.ResetUserPassword(ctx, &user, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "密码重置成功"))
}

// DeleteMfaDevice 清除用户MFA设备
func (oc *OciController) DeleteMfaDevice(c *gin.Context) {
	var req UserManagementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.OciCfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.DeleteUserMfaDevices(ctx, &user, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	util.Go("UpdateConfigCache", func() { oc.schedulerService.UpdateConfigCache(req.OciCfgID) })

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "MFA设备清除成功"))
}

// DeleteApiKey 清除用户API密钥
func (oc *OciController) DeleteApiKey(c *gin.Context) {
	var req UserManagementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	user, ok := oc.loadUser(c, req.OciCfgID)
	if !ok {
		return
	}

	ctx := context.Background()
	err := oc.ociService.DeleteUserApiKeys(ctx, &user, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "API密钥清除成功"))
}
