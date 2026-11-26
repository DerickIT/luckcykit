package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestSandboxPurchaseFlowAndLoopbackBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-http-flow?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{DB: db, SessionTTL: 24 * time.Hour, Sandbox: true})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(Options{SandboxEnabled: true, Identity: services.Identity, Catalog: services.Catalog, Ledger: services.Ledger, Payment: services.Payment})

	remoteLogin := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "198.51.100.12:50000", `{"subject":"remote"}`)
	if remoteLogin.Code != http.StatusForbidden {
		t.Fatalf("non-loopback sandbox login status = %d", remoteLogin.Code)
	}
	login := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "127.0.0.1:50000", `{"subject":"creator"}`)
	if login.Code != http.StatusCreated {
		t.Fatalf("sandbox login status = %d body=%s", login.Code, login.Body.String())
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("invalid login response: %v %s", err, login.Body.String())
	}

	unauth := request(handler, http.MethodPost, "/api/v1/orders", "", "127.0.0.1:50000", `{"productId":"sandbox-points-100"}`)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated order status = %d", unauth.Code)
	}
	created := request(handler, http.MethodPost, "/api/v1/orders", session.Token, "127.0.0.1:50000", `{"productId":"sandbox-points-100","points":999999}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("order creation status = %d body=%s", created.Code, created.Body.String())
	}
	var orderResponse struct {
		Order store.Order `json:"order"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &orderResponse); err != nil {
		t.Fatal(err)
	}
	if orderResponse.Order.Points != 100 {
		t.Fatalf("client-selected points were accepted: %+v", orderResponse.Order)
	}
	otherLogin := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "127.0.0.1:50000", `{"subject":"other-creator"}`)
	var otherSession struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(otherLogin.Body.Bytes(), &otherSession); err != nil || otherSession.Token == "" {
		t.Fatalf("invalid second login response: %v %s", err, otherLogin.Body.String())
	}
	foreignOrder := request(handler, http.MethodGet, "/api/v1/orders/"+orderResponse.Order.ID, otherSession.Token, "127.0.0.1:50000", "")
	if foreignOrder.Code != http.StatusNotFound {
		t.Fatalf("cross-account order lookup status = %d", foreignOrder.Code)
	}
	path := "/api/v1/orders/" + orderResponse.Order.ID + "/sandbox-payment"
	paid := requestWithIdempotency(handler, path, session.Token, "127.0.0.1:50000", "pay-1")
	if paid.Code != http.StatusOK {
		t.Fatalf("sandbox payment status = %d body=%s", paid.Code, paid.Body.String())
	}
	replay := requestWithIdempotency(handler, path, session.Token, "127.0.0.1:50000", "pay-1")
	if replay.Code != http.StatusOK || !bytes.Contains(replay.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("replayed payment response = %d %s", replay.Code, replay.Body.String())
	}
	account := request(handler, http.MethodGet, "/api/v1/account", session.Token, "127.0.0.1:50000", "")
	if account.Code != http.StatusOK || !bytes.Contains(account.Body.Bytes(), []byte(`"points":100`)) {
		t.Fatalf("ledger balance response = %d %s", account.Code, account.Body.String())
	}
	var balance struct {
		Points          int64 `json:"points"`
		ReservedPoints  int64 `json:"reservedPoints"`
		AvailablePoints int64 `json:"availablePoints"`
	}
	if err := json.Unmarshal(account.Body.Bytes(), &balance); err != nil {
		t.Fatal(err)
	}
	if balance.Points != 100 || balance.ReservedPoints != 0 || balance.AvailablePoints != 100 {
		t.Fatalf("account balance fields = %+v", balance)
	}
	remotePayment := requestWithIdempotency(handler, path, session.Token, "198.51.100.12:50000", "pay-1")
	if remotePayment.Code != http.StatusForbidden {
		t.Fatalf("non-loopback sandbox payment status = %d", remotePayment.Code)
	}

	revoked := request(handler, http.MethodPost, "/api/v1/sessions/revoke", session.Token, "127.0.0.1:50000", "")
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("session revoke status = %d body=%s", revoked.Code, revoked.Body.String())
	}
	denied := request(handler, http.MethodGet, "/api/v1/account", session.Token, "127.0.0.1:50000", "")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", denied.Code)
	}
}

func TestSandboxRoutesAreNotMountedWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-disabled-flow?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{DB: db, SessionTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewRouter(Options{Identity: services.Identity, Catalog: services.Catalog, Ledger: services.Ledger, Payment: services.Payment})
	response := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "127.0.0.1:50000", `{"subject":"creator"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled sandbox route status = %d", response.Code)
	}
	products := request(handler, http.MethodGet, "/api/v1/products", "", "127.0.0.1:50000", "")
	if products.Code != http.StatusNotFound {
		t.Fatalf("disabled sandbox catalog status = %d", products.Code)
	}
}

func request(handler http.Handler, method, path, token, remoteAddr, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = remoteAddr
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func requestWithIdempotency(handler http.Handler, path, token, remoteAddr, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestRouterCORSAllowsConfiguredOriginAndRejectsUnknownOrigin(t *testing.T) {
	handler := NewRouter(Options{AllowedOrigins: []string{"https://cloud.example.com", "wails://wails.localhost"}})
	allowed := requestWithOrigin(handler, http.MethodOptions, "/api/v1/auth/wechat/start", "https://cloud.example.com")
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "https://cloud.example.com" || allowed.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("allowed CORS response = %d headers=%v", allowed.Code, allowed.Header())
	}
	unknown := requestWithOrigin(handler, http.MethodOptions, "/api/v1/auth/wechat/start", "https://evil.example.com")
	if unknown.Code != http.StatusForbidden {
		t.Fatalf("unknown CORS origin status = %d", unknown.Code)
	}
}

func requestWithOrigin(handler http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Origin", origin)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
