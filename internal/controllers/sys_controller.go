package controllers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/logger"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

type SysController struct {
	cfg              *config.Config
	schedulerService *services.SchedulerService
	mfaMu            sync.Mutex
	mfaChallenges    map[string]mfaChallenge
	loginMu          sync.Mutex
	loginFailures    map[string]loginFailure
}

const (
	mfaChallengeTTL        = 5 * time.Minute
	mfaChallengeTries      = 5
	mfaChallengeBytes      = 32
	maxMFAChallenges       = 4096
	loginFailureWindow     = time.Minute
	loginFailureLimit      = 10
	maxLoginFailureEntries = 4096
)

type mfaChallenge struct {
	account   string
	expiresAt time.Time
	attempts  int
}

type loginFailure struct {
	windowStart time.Time
	count       int
}

func NewSysController(cfg *config.Config, schedulerService *services.SchedulerService) *SysController {
	return &SysController{
		cfg:              cfg,
		schedulerService: schedulerService,
		mfaChallenges:    make(map[string]mfaChallenge),
		loginFailures:    make(map[string]loginFailure),
	}
}

type LoginRequest struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token          string `json:"token"`
	Username       string `json:"username"`
	NeedMFA        bool   `json:"needMfa"`
	NeedPasskey    bool   `json:"needPasskey"`
	PasskeyEnabled bool   `json:"passkeyEnabled"`
	MFATicket      string `json:"mfaTicket,omitempty"`
}

func (sc *SysController) issueMFAChallenge(account string) (string, error) {
	raw := make([]byte, mfaChallengeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(raw)
	now := time.Now()

	sc.mfaMu.Lock()
	defer sc.mfaMu.Unlock()
	for existing, challenge := range sc.mfaChallenges {
		if now.After(challenge.expiresAt) {
			delete(sc.mfaChallenges, existing)
		}
	}
	if len(sc.mfaChallenges) >= maxMFAChallenges {
		return "", fmt.Errorf("too many pending MFA challenges")
	}
	sc.mfaChallenges[ticket] = mfaChallenge{
		account:   account,
		expiresAt: now.Add(mfaChallengeTTL),
	}
	return ticket, nil
}

func (sc *SysController) consumeMFAChallenge(ticket, code, secret string) (string, bool) {
	now := time.Now()
	sc.mfaMu.Lock()
	defer sc.mfaMu.Unlock()

	challenge, ok := sc.mfaChallenges[ticket]
	if !ok || now.After(challenge.expiresAt) {
		delete(sc.mfaChallenges, ticket)
		return "", false
	}
	if challenge.attempts >= mfaChallengeTries {
		delete(sc.mfaChallenges, ticket)
		return "", false
	}

	if !totp.Validate(code, secret) {
		challenge.attempts++
		if challenge.attempts >= mfaChallengeTries {
			delete(sc.mfaChallenges, ticket)
		} else {
			sc.mfaChallenges[ticket] = challenge
		}
		return "", false
	}

	delete(sc.mfaChallenges, ticket)
	return challenge.account, true
}

func (sc *SysController) loginRateLimited(account string) bool {
	now := time.Now()
	sc.loginMu.Lock()
	defer sc.loginMu.Unlock()

	if sc.loginFailures == nil {
		sc.loginFailures = make(map[string]loginFailure)
	}
	for key, failure := range sc.loginFailures {
		if now.Sub(failure.windowStart) >= loginFailureWindow {
			delete(sc.loginFailures, key)
		}
	}
	failure, ok := sc.loginFailures[account]
	return ok && now.Sub(failure.windowStart) < loginFailureWindow && failure.count >= loginFailureLimit
}

func (sc *SysController) recordLoginFailure(account string) {
	now := time.Now()
	sc.loginMu.Lock()
	defer sc.loginMu.Unlock()

	if sc.loginFailures == nil {
		sc.loginFailures = make(map[string]loginFailure)
	}
	failure, ok := sc.loginFailures[account]
	if !ok || now.Sub(failure.windowStart) >= loginFailureWindow {
		if len(sc.loginFailures) >= maxLoginFailureEntries {
			return
		}
		sc.loginFailures[account] = loginFailure{windowStart: now, count: 1}
		return
	}
	failure.count++
	sc.loginFailures[account] = failure
}

func (sc *SysController) clearLoginFailures(account string) {
	sc.loginMu.Lock()
	delete(sc.loginFailures, account)
	sc.loginMu.Unlock()
}

func (sc *SysController) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	if sc.loginRateLimited(req.Account) {
		c.JSON(http.StatusTooManyRequests, models.ErrorResponse(429, "Too many login attempts; try again later"))
		return
	}

	// 常量时间比较账号 + 支持 bcrypt 的密码校验；两个判断都先求值再合并，
	// 避免账号错误时提前返回造成的时序侧信道。
	db := database.GetDB()
	var admin models.AdminUser
	if err := db.Where("account = ?", req.Account).First(&admin).Error; err != nil {
		sc.recordLoginFailure(req.Account)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid credentials"))
		return
	}
	if !middleware.VerifyPassword(admin.PasswordHash, req.Password) {
		sc.recordLoginFailure(req.Account)
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid credentials"))
		return
	}
	sc.clearLoginFailures(req.Account)

	mfaEnabled := false
	passkeyEnabled := false

	var mfaSetting models.SysSetting
	if err := db.Where("key = ?", "mfa_enabled").First(&mfaSetting).Error; err == nil {
		mfaEnabled = mfaSetting.Value == "true"
	}

	var passkeySetting models.SysSetting
	if err := db.Where("key = ?", "passkey_enabled").First(&passkeySetting).Error; err == nil {
		passkeyEnabled = passkeySetting.Value == "true"
	}

	if mfaEnabled || passkeyEnabled {
		mfaTicket := ""
		if mfaEnabled {
			var err error
			mfaTicket, err = sc.issueMFAChallenge(req.Account)
			if err != nil {
				c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to create MFA challenge"))
				return
			}
		}
		c.JSON(http.StatusOK, models.SuccessResponse(LoginResponse{
			Token:          "",
			Username:       req.Account,
			NeedMFA:        mfaEnabled,
			NeedPasskey:    passkeyEnabled,
			PasskeyEnabled: passkeyEnabled,
			MFATicket:      mfaTicket,
		}, "Additional verification required"))
		return
	}

	token, err := middleware.GenerateToken(req.Account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(LoginResponse{
		Token:    token,
		Username: req.Account,
		NeedMFA:  false,
	}, "Login successful"))
}

type GlanceResponse struct {
	TotalConfigs int64 `json:"totalConfigs"`
	TotalTasks   int64 `json:"totalTasks"`
}

func (sc *SysController) GetGlance(c *gin.Context) {
	db := database.GetDB()

	var totalConfigs int64
	db.Model(&models.OciUser{}).Count(&totalConfigs)

	var totalTasks int64
	db.Model(&models.OciCreateTask{}).Count(&totalTasks)

	c.JSON(http.StatusOK, models.SuccessResponse(GlanceResponse{
		TotalConfigs: totalConfigs,
		TotalTasks:   totalTasks,
	}, "success"))
}

type SysCfgResponse struct {
	LogLevel      string `json:"logLevel"`
	KeyDirPath    string `json:"keyDirPath"`
	CacheEnabled  bool   `json:"cacheEnabled"`
	CacheInterval int    `json:"cacheInterval"`
}

// resolveKeyDir 返回密钥目录的绝对路径：优先用配置项 [paths].key_dir，
// 未配置时回退到默认 "keys"。
func (sc *SysController) resolveKeyDir() string {
	dir := strings.TrimSpace(sc.cfg.Paths.KeyDir)
	if dir == "" {
		dir = "keys"
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func (sc *SysController) GetSysCfg(c *gin.Context) {
	c.JSON(http.StatusOK, models.SuccessResponse(SysCfgResponse{
		LogLevel:      sc.cfg.Logging.Level,
		KeyDirPath:    sc.resolveKeyDir(),
		CacheEnabled:  sc.schedulerService.IsCacheEnabled(),
		CacheInterval: sc.schedulerService.GetCacheInterval(),
	}, "success"))
}

type UpdateCacheCfgRequest struct {
	CacheEnabled  bool `json:"cacheEnabled"`
	CacheInterval int  `json:"cacheInterval"`
}

func (sc *SysController) UpdateCacheCfg(c *gin.Context) {
	var req UpdateCacheCfgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	if err := sc.schedulerService.SetCacheEnabled(req.CacheEnabled); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to update cache enabled"))
		return
	}

	if req.CacheInterval > 0 {
		if err := sc.schedulerService.SetCacheInterval(req.CacheInterval); err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to update cache interval"))
			return
		}
	}

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Cache configuration updated"))
}

func (sc *SysController) RefreshCache(c *gin.Context) {
	if !sc.schedulerService.IsCacheEnabled() {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Cache is not enabled"))
		return
	}

	sc.schedulerService.RefreshAllCaches()
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Cache refresh started"))
}

const (
	MfaEnabledKey = "mfa_enabled"
	MfaSecretKey  = "mfa_secret"
)

type AuthStatusResponse struct {
	MfaEnabled     bool `json:"mfaEnabled"`
	PasskeyEnabled bool `json:"passkeyEnabled"`
}

func (sc *SysController) GetAuthStatus(c *gin.Context) {
	db := database.GetDB()
	mfaEnabled := false
	passkeyEnabled := false

	var mfaSetting models.SysSetting
	if err := db.Where("key = ?", MfaEnabledKey).First(&mfaSetting).Error; err == nil {
		mfaEnabled = mfaSetting.Value == "true"
	}

	var passkeySetting models.SysSetting
	if err := db.Where("key = ?", "passkey_enabled").First(&passkeySetting).Error; err == nil {
		passkeyEnabled = passkeySetting.Value == "true"
	}

	c.JSON(http.StatusOK, models.SuccessResponse(AuthStatusResponse{
		MfaEnabled:     mfaEnabled,
		PasskeyEnabled: passkeyEnabled,
	}, "success"))
}

type GenerateMfaResponse struct {
	Secret string `json:"secret"`
	QrCode string `json:"qrCode"`
}

func (sc *SysController) GenerateMfaSecret(c *gin.Context) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OCI Panel",
		AccountName: c.GetString("username"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to generate MFA secret"))
		return
	}

	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to generate QR code"))
		return
	}
	if err := png.Encode(&buf, img); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to encode QR code"))
		return
	}
	qrCode := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	c.JSON(http.StatusOK, models.SuccessResponse(GenerateMfaResponse{
		Secret: key.Secret(),
		QrCode: qrCode,
	}, "success"))
}

type EnableMfaRequest struct {
	Secret string `json:"secret" binding:"required"`
	Code   string `json:"code" binding:"required"`
}

func (sc *SysController) EnableMfa(c *gin.Context) {
	var req EnableMfaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	valid := totp.Validate(req.Code, req.Secret)
	if !valid {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Invalid verification code"))
		return
	}

	// upsert：保持原有「失败静默」行为（原代码亦未检查写入错误）
	_ = database.UpsertSysSetting(MfaSecretKey, req.Secret)
	_ = database.UpsertSysSetting(MfaEnabledKey, "true")

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "MFA enabled successfully"))
}

func (sc *SysController) DisableMfa(c *gin.Context) {
	db := database.GetDB()
	db.Model(&models.SysSetting{}).Where("key = ?", MfaEnabledKey).Update("value", "false")
	db.Model(&models.SysSetting{}).Where("key = ?", MfaSecretKey).Update("value", "")
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "MFA disabled successfully"))
}

type CheckMfaCodeRequest struct {
	Ticket string `json:"ticket" binding:"required"`
	Code   string `json:"code" binding:"required,len=6"`
}

func (sc *SysController) CheckMfaCode(c *gin.Context) {
	var req CheckMfaCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	db := database.GetDB()
	var enabledSetting models.SysSetting
	if err := db.Where("key = ?", MfaEnabledKey).First(&enabledSetting).Error; err != nil || enabledSetting.Value != "true" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "MFA not enabled"))
		return
	}

	var secretSetting models.SysSetting
	if err := db.Where("key = ?", MfaSecretKey).First(&secretSetting).Error; err != nil || secretSetting.Value == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "MFA not configured"))
		return
	}

	account, ok := sc.consumeMFAChallenge(req.Ticket, req.Code, secretSetting.Value)
	if !ok {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid verification code"))
		return
	}

	token, err := middleware.GenerateToken(account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(LoginResponse{
		Token:    token,
		Username: account,
		NeedMFA:  false,
	}, "MFA verification successful"))
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

// ChangePassword 允许已登录管理员修改自己的密码（DB 持久化，无需重启）。
func (sc *SysController) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	account := c.GetString("username")
	if account == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	if err := services.ChangePassword(account, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "密码修改成功"))
}

type UpdateEmailRequest struct {
	Email string `json:"email"`
}

// UpdateEmail 更新管理员的邮箱（用于密码重置等通知）。
func (sc *SysController) UpdateEmail(c *gin.Context) {
	var req UpdateEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	account := c.GetString("username")
	if account == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	if err := services.SetAdminEmail(account, req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "邮箱更新成功"))
}

type ProfileResponse struct {
	Account string `json:"account"`
	Email   string `json:"email"`
}

type RequestPasswordResetRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// RequestPasswordReset 接收邮箱，发送一次性重置链接。无论邮箱是否存在都返回成功，避免账号枚举。
func (sc *SysController) RequestPasswordReset(c *gin.Context) {
	var req RequestPasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "请输入有效的邮箱地址"))
		return
	}
	if err := services.RequestPasswordReset(sc.cfg, req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "若该邮箱已绑定账号，重置邮件已发送"))
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

// ResetPassword 使用邮件中的一次性令牌设置新密码。
func (sc *SysController) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	if err := services.ResetPassword(req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "密码重置成功，请使用新密码登录"))
}

// GetProfile 返回当前登录管理员的基本资料。
func (sc *SysController) GetProfile(c *gin.Context) {
	account := c.GetString("username")
	if account == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	email, _ := services.GetAdminEmail(account)
	c.JSON(http.StatusOK, models.SuccessResponse(ProfileResponse{
		Account: account,
		Email:   email,
	}, "success"))
}

// 合法日志级别白名单（与 slog 一致）。
var validLogLevels = map[string]struct{}{
	"debug": {}, "info": {}, "warn": {}, "error": {},
}

type UpdateLogLevelRequest struct {
	Level string `json:"level" binding:"required"`
}

// UpdateLogLevel 运行时调整日志级别（即时生效），并写回 config.toml 持久化。
func (sc *SysController) UpdateLogLevel(c *gin.Context) {
	var req UpdateLogLevelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	level := strings.ToLower(strings.TrimSpace(req.Level))
	if _, ok := validLogLevels[level]; !ok {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "日志级别须为 debug/info/warn/error 之一"))
		return
	}
	if err := logger.SetLevel(level); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "设置日志级别失败: "+err.Error()))
		return
	}
	// 写回 config.toml，保证重启后仍保留。
	if err := sc.persistLogLevel(level); err != nil {
		// 运行时已生效，仅持久化失败则给出警告但不阻断。
		log.Printf("warning: 日志级别已生效，但写回 config.toml 失败: %v", err)
	} else {
		sc.cfg.Logging.Level = level
	}
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "日志级别已更新为 "+level))
}

// persistLogLevel 把日志级别写回 config.toml 的 [logging] 段（保留其余内容与注释）。
func (sc *SysController) persistLogLevel(level string) error {
	data, err := os.ReadFile("config.toml")
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inLogging := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[logging]" {
			inLogging = true
			continue
		}
		// 进入下一个 section 时退出 [logging] 上下文
		if inLogging && strings.HasPrefix(trimmed, "[") {
			inLogging = false
		}
		if inLogging && strings.HasPrefix(trimmed, "level") {
			lines[i] = `level = "` + level + `"`
			replaced = true
			inLogging = false
		}
	}
	if !replaced {
		// 极端情况：配置里没有 [logging].level，则追加一段
		lines = append(lines, "", "[logging]", `level = "`+level+`"`)
	}
	return os.WriteFile("config.toml", []byte(strings.Join(lines, "\n")), 0644)
}

type UpdateAccountRequest struct {
	Account string `json:"account" binding:"required"`
}

// UpdateAccount 修改当前管理员的登录账号，并重签 JWT 返回，避免旧 token 中的用户名失效。
func (sc *SysController) UpdateAccount(c *gin.Context) {
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}
	current := c.GetString("username")
	if current == "" {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Unauthorized"))
		return
	}
	newAccount := strings.TrimSpace(req.Account)
	if newAccount == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "账号不能为空"))
		return
	}

	// 检查新账号是否与其它管理员冲突
	var conflict models.AdminUser
	if err := database.GetDB().Where("account = ?", newAccount).First(&conflict).Error; err == nil && conflict.Account == newAccount {
		if conflict.Account != current {
			c.JSON(http.StatusConflict, models.ErrorResponse(409, "该账号已存在"))
			return
		}
	}

	if err := services.UpdateAccount(current, newAccount); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, err.Error()))
		return
	}

	newToken, err := middleware.GenerateToken(newAccount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "账号已更新，但重新签发令牌失败: "+err.Error()))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(map[string]string{
		"token":   newToken,
		"account": newAccount,
	}, "账号已更新"))
}
