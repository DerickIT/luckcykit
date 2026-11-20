package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/config"
	"luckytv/backend/internal/cloud/store"
)

func TestAdminSessionIsSeparateFromUserSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "admin-http.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	services, err := cloudapp.New(db, time.Hour, true, nil, nil, nil, nil, "bootstrap-secret-012345678901234567890123", "admin@example.com", "admin-secret-0123456789012345678901234567")
	if err != nil {
		t.Fatal(err)
	}
	var adminAccount store.AdminAccount
	if err := db.Where("email = ?", "admin@example.com").First(&adminAccount).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(config.Config{SandboxEnabled: true}, services.Identity, services.Admin, nil, services.Catalog, services.Ledger, services.Payment, nil)
	login := request(handler, http.MethodPost, "/api/v1/admin/login", "", "127.0.0.1:50000", `{"email":"admin@example.com","secret":"admin-secret-0123456789012345678901234567"}`)
	if login.Code != http.StatusCreated {
		t.Fatalf("admin login status=%d body=%s", login.Code, login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("missing admin token: %v %s", err, login.Body.String())
	}
	adminRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/account", nil)
	adminRequest.Header.Set("Admin-Authorization", "Bearer "+session.Token)
	adminResponse := httptest.NewRecorder()
	handler.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK || !contains(adminResponse.Body.String(), adminAccount.ID) {
		t.Fatalf("admin session failed: %d %s", adminResponse.Code, adminResponse.Body.String())
	}
	wrongHeader := request(handler, http.MethodGet, "/api/v1/admin/account", session.Token, "127.0.0.1:50000", "")
	if wrongHeader.Code != http.StatusUnauthorized {
		t.Fatalf("user bearer was accepted as admin: %d %s", wrongHeader.Code, wrongHeader.Body.String())
	}
	badLogin := request(handler, http.MethodPost, "/api/v1/admin/login", "", "127.0.0.1:50000", `{"email":"missing@example.com","secret":"admin-secret-0123456789012345678901234567"}`)
	if badLogin.Code != http.StatusUnauthorized || badLogin.Body.String() != `{"error":"invalid_admin_credentials"}` {
		t.Fatalf("admin login leaked account state: %d %s", badLogin.Code, badLogin.Body.String())
	}
}

func contains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
