package controllers

import (
	"encoding/json"
	"errors"

	"github.com/adiecho/oci-panel/internal/database"
	"gorm.io/gorm"
)

// automation_controller.go 的辅助函数。

func db() *gorm.DB { return database.GetDB() }

func jsonUnmarshal(s string, v interface{}) error { return json.Unmarshal([]byte(s), v) }

func errFreeShape() error {
	return errors.New("仅允许 Always Free 资格内的形状（A1.Flex / E2.1.Micro），防止产生费用")
}

func errBackupLimit() error {
	return errors.New("备份保留份数上限为 5（每租户仅 5 个免费备份槽，超出将产生费用）")
}

func errInvalidThreshold() error {
	return errors.New("告警阈值必须在 1~100 之间")
}
