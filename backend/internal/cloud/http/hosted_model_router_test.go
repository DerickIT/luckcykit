package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/store"
)

func TestHostedModelRouteRequiresSessionAndReturnsPublicProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-hosted-model-route?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{
		DB: db, SessionTTL: time.Hour, Sandbox: true,
		Models: []store.HostedModel{
			{ID: "text-fast", UpstreamID: "internal-upstream", PriceVersion: "price-v1", MaxInputTokens: 100, MaxOutputTokens: 50, InputPointsPerMillion: 2, OutputPointsPerMillion: 3, Enabled: true},
			{ID: "disabled", UpstreamID: "hidden-upstream", PriceVersion: "price-v1", MaxInputTokens: 100, MaxOutputTokens: 50, Enabled: false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(Options{SandboxEnabled: true, Identity: services.Identity, Catalog: services.Catalog, HostedModels: services.HostedModels, Ledger: services.Ledger, Payment: services.Payment})

	if response := request(handler, http.MethodGet, "/api/v1/models", "", "127.0.0.1:50000", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated model catalog status = %d", response.Code)
	}
	login := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "127.0.0.1:50000", `{"subject":"catalog-reader"}`)
	var session struct{ Token string `json:"token"` }
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("invalid login response: %v %s", err, login.Body.String())
	}
	response := request(handler, http.MethodGet, "/api/v1/models", session.Token, "127.0.0.1:50000", "")
	if response.Code != http.StatusOK {
		t.Fatalf("model catalog status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !containsAll(body, `"id":"text-fast"`, `"capabilities":["text"]`, `"priceVersion":"price-v1"`) {
		t.Fatalf("public model projection missing fields: %s", body)
	}
	if containsAny(body, "internal-upstream", "hidden-upstream", "disabled") {
		t.Fatalf("model catalog leaked internal or disabled data: %s", body)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func containsAny(value string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(value, part) {
			return true
		}
	}
	return false
}
