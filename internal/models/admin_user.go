package models

import "time"

// AdminUser 是面板管理员账户。凭据持久化在数据库（而非仅 config.toml），
// 以支持运行时改密与（后续）邮箱重置，无需重启服务。
type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Account      string    `gorm:"uniqueIndex;size:64" json:"account"`
	PasswordHash string    `json:"-"`
	Email        string    `gorm:"size:255" json:"email"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
