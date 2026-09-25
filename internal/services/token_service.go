package services

import (
        "crypto/rand"
        "encoding/hex"
        "fmt"
        "time"

        "github.com/adiecho/oci-panel/internal/database"
        "github.com/adiecho/oci-panel/internal/middleware"
        "github.com/adiecho/oci-panel/internal/models"
)

// apiTokenPrefix 是生成的 API Token 的肉眼可辨识前缀，便于与 JWT 区分。
const apiTokenPrefix = "opat_"

// GenerateApiToken 在数据库中新建一个 API Token 并返回明文（仅此一次可见）。
// expiresInDays <= 0 表示永不过期；否则从当前时间起算相应天数后过期。
// scope 合法值：full / readonly / instance / traffic；空串按 full 处理，非法值报错。
// 返回的 plaintext 形如 "opat_<64hex>"，长度 69，远低于 bcrypt 的 72 字节上限。
func GenerateApiToken(name string, expiresInDays int, scope string) (plaintext string, token *models.ApiToken, err error) {
        switch scope {
        case "":
                scope = "full"
        case "full", "readonly", "instance", "traffic":
        default:
                return "", nil, fmt.Errorf("无效的 scope: %s（可选 full/readonly/instance/traffic）", scope)
        }
        buf := make([]byte, 32)
        if _, err := rand.Read(buf); err != nil {
                return "", nil, fmt.Errorf("failed to generate random token: %w", err)
        }
        plaintext = apiTokenPrefix + hex.EncodeToString(buf)

        hash, err := middleware.HashPassword(plaintext)
        if err != nil {
                return "", nil, fmt.Errorf("failed to hash token: %w", err)
        }

        token = &models.ApiToken{
                Name:      name,
                TokenHash: hash,
                Prefix:    plaintext[:12],
                Scope:     scope,
        }
        if expiresInDays > 0 {
                exp := time.Now().AddDate(0, 0, expiresInDays)
                token.ExpiresAt = &exp
        }

        if err := database.GetDB().Create(token).Error; err != nil {
                return "", nil, fmt.Errorf("failed to persist token: %w", err)
        }
        return plaintext, token, nil
}

// ListApiTokens 返回所有令牌（不含哈希），用于管理界面展示。
func ListApiTokens() ([]models.ApiToken, error) {
        var tokens []models.ApiToken
        if err := database.GetDB().Order("created_at desc").Find(&tokens).Error; err != nil {
                return nil, fmt.Errorf("failed to list tokens: %w", err)
        }
        return tokens, nil
}

// RevokeApiToken 按 ID 删除（吊销）一个令牌，同时清理其调用记录。
func RevokeApiToken(id uint) error {
        db := database.GetDB()
        if err := db.Delete(&models.ApiToken{}, id).Error; err != nil {
                return fmt.Errorf("failed to revoke token: %w", err)
        }
        // 顺带删除该令牌的调用记录，避免残留孤儿数据。
        _ = db.Where("token_id = ?", id).Delete(&models.TokenCallLog{}).Error
        return nil
}

// ListTokenCalls 返回指定令牌最近的调用记录（倒序，最多 200 条）。
func ListTokenCalls(tokenID uint) ([]models.TokenCallLog, error) {
        var calls []models.TokenCallLog
        if err := database.GetDB().
                Where("token_id = ?", tokenID).
                Order("created_at desc").
                Limit(200).
                Find(&calls).Error; err != nil {
                return nil, fmt.Errorf("failed to list token calls: %w", err)
        }
        return calls, nil
}
