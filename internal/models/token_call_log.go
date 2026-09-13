package models

import "time"

// TokenCallLog 记录一次 API 令牌调用明细，供面板「调用记录」查看（参考青龙面板）。
// 只保留成功通过鉴权的调用；按令牌分页查询，过量由清理逻辑裁剪。
type TokenCallLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TokenID    uint      `gorm:"index;not null" json:"tokenId"`
	Method     string    `gorm:"size:16" json:"method"`
	Path       string    `gorm:"size:255" json:"path"`
	StatusCode int       `json:"statusCode"`
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func (TokenCallLog) TableName() string {
	return "token_call_log"
}
