package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/models"
)

const (
	resetTokenTTL = 30 * time.Minute
	resendAPIURL  = "https://api.resend.com/emails"
)

// SendMail 通过 Resend 发送 HTML 邮件。
func SendMail(cfg *config.Config, to, subject, html string) error {
	if strings.TrimSpace(cfg.Email.ResendAPIKey) == "" {
		return errors.New("邮件服务未配置：缺少 resend_api_key")
	}
	if strings.TrimSpace(cfg.Email.From) == "" {
		return errors.New("邮件服务未配置：缺少 from 发件人")
	}

	payload := map[string]interface{}{
		"from":    cfg.Email.From,
		"to":      []string{to},
		"subject": subject,
		"html":    html,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, resendAPIURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Email.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("邮件服务返回错误 %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// publicBaseURL 返回用于拼接重置链接的面板地址。
func publicBaseURL(cfg *config.Config) string {
	if strings.TrimSpace(cfg.Email.PublicURL) != "" {
		return strings.TrimRight(strings.TrimSpace(cfg.Email.PublicURL), "/")
	}
	return "http://localhost:8999"
}

// RequestPasswordReset 为指定邮箱创建一次性重置令牌并发送邮件。
// 出于安全考虑，无论邮箱是否存在，调用方都返回成功（避免账号枚举）。
func RequestPasswordReset(cfg *config.Config, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("邮箱不能为空")
	}
	db := database.GetDB()
	var admin models.AdminUser
	if err := db.Where("email = ?", email).First(&admin).Error; err != nil {
		// 邮箱未绑定任何账号：静默返回，不泄露信息
		return nil
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	// 清理该账号旧的未用令牌
	db.Where("account = ?", admin.Account).Delete(&models.PasswordResetToken{})
	record := models.PasswordResetToken{
		Account:   admin.Account,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(resetTokenTTL),
	}
	if err := db.Create(&record).Error; err != nil {
		return err
	}

	resetURL := fmt.Sprintf("%s/reset-password?token=%s", publicBaseURL(cfg), token)
	html := buildResetEmailHTML(admin.Account, resetURL, int(resetTokenTTL.Minutes()))
	return SendMail(cfg, email, "OCI Panel 密码重置", html)
}

// ResetPassword 校验一次性令牌并设置新密码。
func ResetPassword(token, newPassword string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("重置令牌无效")
	}
	if len(newPassword) < 6 {
		return errors.New("新密码长度至少 6 位")
	}
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])

	db := database.GetDB()
	var record models.PasswordResetToken
	if err := db.Where("token_hash = ?", tokenHash).First(&record).Error; err != nil {
		return errors.New("重置令牌无效或已使用")
	}
	if record.Used || time.Now().After(record.ExpiresAt) {
		return errors.New("重置令牌已过期")
	}

	hash, err := middleware.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := db.Model(&models.AdminUser{}).Where("account = ?", record.Account).Update("password_hash", hash).Error; err != nil {
		return err
	}
	// 一次性使用：直接删除
	db.Delete(&models.PasswordResetToken{}, record.ID)
	return nil
}

func buildResetEmailHTML(account, resetURL string, minutes int) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN"><body style="font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;background:#f6f7f9;padding:24px;">
  <div style="max-width:520px;margin:0 auto;background:#fff;border-radius:12px;padding:32px;border:1px solid #e5e7eb;">
    <h2 style="margin:0 0 16px;color:#111827;">OCI Panel 密码重置</h2>
    <p style="color:#374151;line-height:1.6;">账号 <b>%s</b> 请求重置密码。点击下方按钮设置新密码：</p>
    <p style="text-align:center;margin:28px 0;">
      <a href="%s" style="display:inline-block;background:#2563eb;color:#fff;text-decoration:none;padding:12px 24px;border-radius:8px;font-weight:600;">重置密码</a>
    </p>
    <p style="color:#6b7280;font-size:13px;line-height:1.6;">链接 %d 分钟内有效，且只能使用一次。若非本人操作，请忽略此邮件，账号密码不会被更改。</p>
    <p style="color:#9ca3af;font-size:12px;word-break:break-all;">%s</p>
  </div>
</body></html>`, account, resetURL, minutes, resetURL)
}
