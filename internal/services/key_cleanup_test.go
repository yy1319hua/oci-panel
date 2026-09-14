package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupKeyCleanupEnv 把进程工作目录切到一个临时目录，让 CleanupOrphanKeys 里的
// 相对路径 "./keys" 落在沙箱内，避免污染真实仓库；同时返回该临时目录。
func setupKeyCleanupEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取工作目录失败：%v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("切换工作目录失败：%v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

// setupKeyCleanupDB 用内存 SQLite 替换全局 DB，并写入指定配置。
func setupKeyCleanupDB(t *testing.T, keyPaths ...string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存数据库失败：%v", err)
	}
	if err := db.AutoMigrate(&models.OciUser{}); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	for i, p := range keyPaths {
		u := models.OciUser{OciKeyPath: p}
		u.ID = "cfg-" + string(rune('a'+i))
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("写入配置失败：%v", err)
		}
	}
	prev := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = prev })
}

func writeKeyFile(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(keysDir, 0700); err != nil {
		t.Fatalf("创建 keys 目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(keysDir, name), []byte("dummy"), 0600); err != nil {
		t.Fatalf("写入 %s 失败：%v", name, err)
	}
}

func exists(name string) bool {
	_, err := os.Stat(filepath.Join(keysDir, name))
	return err == nil
}

// 核心行为：只删没人引用的 .pem/.key，在用的一律保留。
func TestCleanupOrphanKeysRemovesOnlyUnreferenced(t *testing.T) {
	setupKeyCleanupEnv(t)
	setupKeyCleanupDB(t, "in-use.pem")

	writeKeyFile(t, "in-use.pem")
	writeKeyFile(t, "orphan1.pem")
	writeKeyFile(t, "orphan2.key")

	removed := CleanupOrphanKeys()

	if !exists("in-use.pem") {
		t.Fatal("在用的密钥被误删了")
	}
	if exists("orphan1.pem") || exists("orphan2.key") {
		t.Fatal("孤儿密钥未被清理")
	}
	if len(removed) != 2 {
		t.Fatalf("期望清理 2 个，实际 %d 个：%v", len(removed), removed)
	}
}

// 非私钥文件（如 .txt/.bak/无扩展名）绝不能被碰。
func TestCleanupOrphanKeysIgnoresNonKeyFiles(t *testing.T) {
	setupKeyCleanupEnv(t)
	setupKeyCleanupDB(t)

	for _, name := range []string{"notes.txt", "backup.bak", "README", "id_rsa"} {
		writeKeyFile(t, name)
	}

	removed := CleanupOrphanKeys()

	if len(removed) != 0 {
		t.Fatalf("不应删除任何非私钥文件，实际删除了：%v", removed)
	}
	for _, name := range []string{"notes.txt", "backup.bak", "README", "id_rsa"} {
		if !exists(name) {
			t.Fatalf("%s 被误删", name)
		}
	}
}

// 查库失败时必须放弃清理，绝不能基于不完整数据删密钥。
func TestCleanupOrphanKeysSkipsWhenDBUnavailable(t *testing.T) {
	setupKeyCleanupEnv(t)
	prev := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = prev })

	writeKeyFile(t, "whatever.pem")

	removed := CleanupOrphanKeys()

	if len(removed) != 0 {
		t.Fatalf("数据库不可用时不应删除任何文件，实际删除了：%v", removed)
	}
	if !exists("whatever.pem") {
		t.Fatal("数据库不可用时文件被误删")
	}
}

// keys 目录不存在时应安静返回，不应 panic。
func TestCleanupOrphanKeysHandlesMissingDir(t *testing.T) {
	setupKeyCleanupEnv(t)
	setupKeyCleanupDB(t)

	if removed := CleanupOrphanKeys(); len(removed) != 0 {
		t.Fatalf("目录不存在时应返回空，实际：%v", removed)
	}
}

// 数据库里存的是完整路径时，取 basename 后仍应正确识别为「在用」。
func TestCleanupOrphanKeysMatchesBasename(t *testing.T) {
	setupKeyCleanupEnv(t)
	setupKeyCleanupDB(t, "/app/keys/nested-name.pem")

	writeKeyFile(t, "nested-name.pem")

	if removed := CleanupOrphanKeys(); len(removed) != 0 {
		t.Fatalf("带路径的在用密钥被误判为孤儿：%v", removed)
	}
	if !exists("nested-name.pem") {
		t.Fatal("带路径的在用密钥被误删")
	}
}
