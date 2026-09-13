package controllers

import (
	"net/http"
	"strconv"

	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type TokenController struct{}

func NewTokenController() *TokenController {
	return &TokenController{}
}

type CreateTokenRequest struct {
	// Name 为令牌的易读名称，仅用于管理界面辨识。
	Name string `json:"name" binding:"required"`
	// ExpiresInDays <= 0 表示永不过期；否则从当前时间起算相应天数后过期。
	ExpiresInDays int `json:"expiresInDays"`
}

type CreateTokenResponse struct {
	// Token 是明文令牌，仅在此接口的返回中出现一次，请立即保存。
	Token string `json:"token"`
	Info  models.ApiToken `json:"info"`
}

// CreateToken 生成一个新的 API Token。仅管理员 JWT 可调用（由 RequireAdmin 守卫）。
func (tc *TokenController) CreateToken(c *gin.Context) {
	var req CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	plaintext, tok, err := services.GenerateApiToken(req.Name, req.ExpiresInDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(CreateTokenResponse{
		Token: plaintext,
		Info:  *tok,
	}, "API Token 创建成功，请立即复制保存（仅显示一次）"))
}

// ListTokens 列出所有令牌（不含哈希）。
func (tc *TokenController) ListTokens(c *gin.Context) {
	tokens, err := services.ListApiTokens()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(tokens, "success"))
}

type RevokeTokenRequest struct {
	ID uint `json:"id" binding:"required"`
}

// RevokeToken 吊销（删除）指定 ID 的令牌。
func (tc *TokenController) RevokeToken(c *gin.Context) {
	var req RevokeTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	if err := services.RevokeApiToken(req.ID); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "令牌已吊销"))
}

// ListTokenCalls 返回指定令牌的最近调用记录（参考青龙面板的调用日志）。
// 仅管理员 JWT 可调用（由 RequireAdmin 守卫），避免 API Token 窥探其它令牌的使用情况。
func (tc *TokenController) ListTokenCalls(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "invalid id"))
		return
	}
	calls, err := services.ListTokenCalls(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(calls, "success"))
}

// RevokeTokenByID 支持以路径参数形式吊销，便于 curl 直接调用。
func (tc *TokenController) RevokeTokenByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "invalid id"))
		return
	}
	if err := services.RevokeApiToken(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "令牌已吊销"))
}
