package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/store"
)

func TestAPITokenRoutesKeepManagementAndLedgerWebOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-api-token-route?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{DB: db, SessionTTL: time.Hour, Sandbox: true})
	if err != nil {
		t.Fatal(err)
	}
	login, err := services.Identity.SandboxLogin(nil, "api-route-user")
	if err != nil {
		t.Fatal(err)
	}
	token, err := services.Identity.CreateAPIToken(login.Account.ID, "route-test", 90, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(Options{SandboxEnabled: true, Identity: services.Identity, Catalog: services.Catalog, Ledger: services.Ledger, Payment: services.Payment})

	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token.Token)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	if result := request(http.MethodGet, "/api/v1/account"); result.Code != http.StatusOK {
		t.Fatalf("API token account access status=%d body=%s", result.Code, result.Body.String())
	}
	if result := request(http.MethodGet, "/api/v1/ledger"); result.Code != http.StatusForbidden {
		t.Fatalf("API token ledger access status=%d body=%s", result.Code, result.Body.String())
	}
	if result := request(http.MethodGet, "/api/v1/tokens"); result.Code != http.StatusForbidden {
		t.Fatalf("API token management access status=%d body=%s", result.Code, result.Body.String())
	}

	body, _ := json.Marshal(map[string]any{"name": "second", "expiryDays": 7})
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", bytes.NewReader(body))
	createRequest.Header.Set("Authorization", "Bearer "+login.Token)
	createResult := httptest.NewRecorder()
	handler.ServeHTTP(createResult, createRequest)
	if createResult.Code != http.StatusCreated {
		t.Fatalf("Web Session token create status=%d body=%s", createResult.Code, createResult.Body.String())
	}
}
