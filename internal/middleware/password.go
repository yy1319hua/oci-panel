package middleware

import (
	"crypto/subtle"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// VerifyPassword 校验明文密码 provided 是否与配置中存储的 stored 匹配。
//
// 若 stored 是 bcrypt 哈希（以 $2a$/$2b$/$2y$ 开头），用 bcrypt 比对；
// 否则按明文做常量时间比较——既向后兼容 config.toml 里既有的明文密码，
// 又消除了此前 "req.Password != cfg.Web.Password" 直接比较带来的时序侧信道。
// 推荐在 config.toml 中存放 bcrypt 哈希而非明文（迁移说明见 README）。
func VerifyPassword(stored, provided string) bool {
	if stored == "" {
		return false
	}
	if IsBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(provided)) == nil
	}
	return SecureCompareString(stored, provided)
}

// IsBcryptHash 粗略判断字符串是否为 bcrypt 哈希。
func IsBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")
}

// HashPassword 对明文密码做 bcrypt 哈希，用于安全地存储管理员密码。
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SecureCompareString 以常量时间比较两个字符串，避免时序攻击导致的账号/密码枚举。
func SecureCompareString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
