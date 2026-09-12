package database

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/adiecho/oci-panel/internal/models"
)

// TestInitDBEnablesWALAndConcurrency 验证 B2：WAL 已启用，且放开单连接后并发写入不会失败。
func TestInitDBEnablesWALAndConcurrency(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "test.db")
	if err := InitDB(dsn); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	var mode string
	if err := DB.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("query journal_mode failed: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(k int) {
			defer wg.Done()
			id := "key-" + strconv.Itoa(k)
			if err := DB.Create(&models.SysSetting{ID: id, Key: id, Value: "v"}).Error; err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent write failed (would have serialized/locked under MaxOpenConns(1)): %v", err)
	}

	var count int64
	DB.Model(&models.SysSetting{}).Count(&count)
	if count != n {
		t.Errorf("row count = %d, want %d", count, n)
	}
}

func TestWithSQLitePragmas(t *testing.T) {
	got := withSQLitePragmas("db/oci.db")
	if !contains(got, "journal_mode(WAL)") || !contains(got, "busy_timeout(5000)") {
		t.Errorf("pragmas not appended: %q", got)
	}
	// 已含 _pragma 时不覆盖
	custom := "db/oci.db?_pragma=busy_timeout(1)"
	if withSQLitePragmas(custom) != custom {
		t.Errorf("should not override caller-provided pragmas")
	}
	// 已有查询参数时用 & 连接
	if got := withSQLitePragmas("db/oci.db?cache=shared"); !contains(got, "?cache=shared&_pragma=") {
		t.Errorf("should append with & when query already present: %q", got)
	}
}

func TestConcurrentUpsertSysSetting(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "settings.db")); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			<-start
			if err := UpsertSysSetting("shared", strconv.Itoa(value)); err != nil {
				t.Errorf("concurrent upsert: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	var settings []models.SysSetting
	if err := DB.Where("key = ?", "shared").Find(&settings).Error; err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 {
		t.Fatalf("got %d settings, want one", len(settings))
	}
	if err := UpsertSysSetting("shared", ""); err != nil {
		t.Fatal(err)
	}
	var updated models.SysSetting
	if err := DB.First(&updated, "key = ?", "shared").Error; err != nil {
		t.Fatal(err)
	}
	if updated.ID != settings[0].ID || updated.Value != "" {
		t.Fatalf("upsert changed the ID or ignored an empty value: %+v", updated)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
