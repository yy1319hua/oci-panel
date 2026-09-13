package models

import "time"

// ApiToken 表示由后台生成、可轮换/吊销的 API 访问令牌（等价于管理员私钥）。
// 设计参考青龙面板的 OpenAPI Token：明文仅创建时返回一次，之后只存储 bcrypt 哈希；
// 调用方通过 `Authorization: Bearer <token>` 访问 /api/* 接口。
type ApiToken struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:64;not null" json:"name"`
	TokenHash  string     `gorm:"size:255;not null" json:"-"` // bcrypt 哈希，永不明文存储
	Prefix     string     `gorm:"size:16" json:"prefix"`      // 明文前若干字符，仅用于列表中辨识
	Scope      string     `gorm:"size:16;default:'full'" json:"scope"` // full=完整API权限；readonly=仅GET（预留）
	ExpiresAt  *time.Time `json:"expiresAt"`                  // 为空表示永不过期
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CallCount  int64      `gorm:"default:0" json:"callCount"`       // 累计调用次数
	LastUsedIP string     `gorm:"size:64" json:"lastUsedIp"`        // 最近一次调用来源 IP
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

func (ApiToken) TableName() string {
	return "api_token"
}
