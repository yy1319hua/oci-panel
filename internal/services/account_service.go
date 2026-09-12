package services

import (
	"errors"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/models"
)

// SeedAdminFromConfig 在数据库为空时，用 config.toml 中的初始账号密码创建首个管理员。
// 若库里已有管理员则跳过，保证 config.toml 仅作为“首次种子”，后续以数据库为准。
// 密码存储：若 config 中已是 bcrypt 哈希则原样保存；否则对明文做 bcrypt 哈希。
func SeedAdminFromConfig(account, password string) error {
	db := database.GetDB()
	if db == nil {
		return errors.New("database not initialized")
	}
	var count int64
	if err := db.Model(&models.AdminUser{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash := password
	if !middleware.IsBcryptHash(password) {
		h, err := middleware.HashPassword(password)
		if err != nil {
			return err
		}
		hash = h
	}
	admin := models.AdminUser{Account: account, PasswordHash: hash}
	return db.Create(&admin).Error
}

// GetAdminByAccount 按账号查询管理员。
func GetAdminByAccount(account string) (*models.AdminUser, error) {
	db := database.GetDB()
	var u models.AdminUser
	if err := db.Where("account = ?", account).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ChangePassword 校验旧密码后更新为新密码（bcrypt 哈希）。
func ChangePassword(account, oldPlain, newPlain string) error {
	db := database.GetDB()
	var u models.AdminUser
	if err := db.Where("account = ?", account).First(&u).Error; err != nil {
		return errors.New("account not found")
	}
	if !middleware.VerifyPassword(u.PasswordHash, oldPlain) {
		return errors.New("current password incorrect")
	}
	hash, err := middleware.HashPassword(newPlain)
	if err != nil {
		return err
	}
	return db.Model(&u).Update("password_hash", hash).Error
}

// SetAdminEmail 更新管理员的邮箱（用于密码重置等通知）。
func SetAdminEmail(account, email string) error {
	db := database.GetDB()
	return db.Model(&models.AdminUser{}).Where("account = ?", account).Update("email", email).Error
}

// GetAdminEmail 返回管理员的邮箱（空串表示未设置）。
func GetAdminEmail(account string) (string, error) {
	u, err := GetAdminByAccount(account)
	if err != nil {
		return "", err
	}
	return u.Email, nil
}

// UpdateAccount 将指定旧账号改名为新账号。newAccount 不得与库内其它管理员冲突
// （同名即视为无变更，允许通过）。
func UpdateAccount(current, newAccount string) error {
	db := database.GetDB()
	return db.Model(&models.AdminUser{}).Where("account = ?", current).Update("account", newAccount).Error
}
