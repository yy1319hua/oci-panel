package models

import "time"

// PasswordResetToken 邮箱重置密码的一次性令牌。
// 只存哈希，明文仅通过邮件链接发送；使用后立即删除，并带过期时间。
type PasswordResetToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Account   string    `gorm:"index;size:64" json:"account"`
	TokenHash string    `gorm:"uniqueIndex;size:128" json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
	Used      bool      `gorm:"default:false" json:"used"`
	CreatedAt time.Time `json:"createdAt"`
}
