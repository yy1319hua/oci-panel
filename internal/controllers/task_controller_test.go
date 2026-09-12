package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/adiecho/oci-panel/internal/services"
	"github.com/gin-gonic/gin"
)

type successfulInstanceCreator struct{}

func (successfulInstanceCreator) CreateInstance(context.Context, *models.OciUser, string, string, string, float64, float64, int, int64, string, string) error {
	return nil
}

func TestCreateOneOffTaskReturnsPersistedResult(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "tasks.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.Create(&models.OciUser{ID: "config", Username: "test"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SSHKey{ID: "key", Name: "test", PublicKey: "public", KeyType: "standalone"}).Error; err != nil {
		t.Fatal(err)
	}
	s := services.NewTaskService(db, successfulInstanceCreator{})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	r := gin.New()
	r.POST("/create", NewTaskController(s).CreateTask)
	req := httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(`{"userId":"config","ociRegion":"region","sshKeyId":"key","executeOnce":true}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	var body struct {
		Code int                  `json:"code"`
		Data models.OciCreateTask `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.Code != 200 || body.Data.Status != "completed" || body.Data.SuccessCount != 1 || body.Data.ExecuteCount != 1 {
		t.Fatalf("response contains a stale task snapshot: %s", response.Body.String())
	}
}
