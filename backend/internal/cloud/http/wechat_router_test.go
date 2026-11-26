package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/store"
)

func TestWeChatOAuthCallbackConsumesStateAndSetsSecureSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-wechat-router?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{DB: db, SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"upstream-access","openid":"openid","unionid":"union-123"}`))
	}))
	defer tokenServer.Close()

	const callbackURL = "https://cloud.example.com/api/v1/auth/wechat/callback"
	provider := &identity.WeChatProvider{AppID: "app-id", AppSecret: "app-secret", RedirectURI: callbackURL, TokenURL: tokenServer.URL}
	handler := NewRouter(Options{Identity: services.Identity, WeChat: provider, WeChatCallbackURL: callbackURL, Ledger: services.Ledger})
	start := httptest.NewRequest(http.MethodGet, "/api/v1/auth/wechat/start", nil)
	startResult := httptest.NewRecorder()
	handler.ServeHTTP(startResult, start)
	if startResult.Code != http.StatusOK {
		t.Fatalf("start OAuth status=%d body=%s", startResult.Code, startResult.Body.String())
	}
	var begin struct {
		AuthorizationURL string `json:"authorizationURL"`
	}
	if err := json.Unmarshal(startResult.Body.Bytes(), &begin); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(begin.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callbackURLWithCode := "/api/v1/auth/wechat/callback?" + url.Values{"state": {authorize.Query().Get("state")}, "code": {"test-code"}}.Encode()
	callback := httptest.NewRequest(http.MethodGet, callbackURLWithCode, nil)
	callbackResult := httptest.NewRecorder()
	handler.ServeHTTP(callbackResult, callback)
	if callbackResult.Code != http.StatusOK {
		t.Fatalf("callback status=%d body=%s", callbackResult.Code, callbackResult.Body.String())
	}
	cookie := callbackResult.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != "luckytv_cloud_session" || !cookie[0].Secure || !cookie[0].HttpOnly || cookie[0].SameSite != http.SameSiteLaxMode || cookie[0].Value == "" {
		t.Fatalf("session cookie has unsafe attributes: %+v", cookie)
	}

	accountRequest := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	accountRequest.AddCookie(cookie[0])
	accountResult := httptest.NewRecorder()
	handler.ServeHTTP(accountResult, accountRequest)
	if accountResult.Code != http.StatusOK {
		t.Fatalf("cookie-authenticated account request status=%d body=%s", accountResult.Code, accountResult.Body.String())
	}

	replay := httptest.NewRequest(http.MethodGet, callbackURLWithCode, nil)
	replayResult := httptest.NewRecorder()
	handler.ServeHTTP(replayResult, replay)
	if replayResult.Code != http.StatusBadRequest {
		t.Fatalf("replayed OAuth state status=%d body=%s", replayResult.Code, replayResult.Body.String())
	}
}

func TestWeChatOAuthDesktopHandoffRedirectsAndExchangesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:cloud-wechat-handoff?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	services, err := cloudapp.New(cloudapp.Options{DB: db, SessionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"upstream-access","openid":"openid","unionid":"union-handoff"}`))
	}))
	defer tokenServer.Close()
	const callbackURL = "https://cloud.example.com/api/v1/auth/wechat/callback"
	provider := &identity.WeChatProvider{AppID: "app-id", AppSecret: "app-secret", RedirectURI: callbackURL, TokenURL: tokenServer.URL}
	handler := NewRouter(Options{Identity: services.Identity, WeChat: provider, WeChatCallbackURL: callbackURL, Ledger: services.Ledger})
	handoffURL := "http://127.0.0.1:45678/cloud/callback"
	start := httptest.NewRequest(http.MethodGet, "/api/v1/auth/wechat/start?"+url.Values{"handoffURL": {handoffURL}, "clientState": {"desktop-state"}}.Encode(), nil)
	startResult := httptest.NewRecorder()
	handler.ServeHTTP(startResult, start)
	if startResult.Code != http.StatusOK {
		t.Fatalf("start OAuth status=%d body=%s", startResult.Code, startResult.Body.String())
	}
	var begin struct {
		AuthorizationURL string `json:"authorizationURL"`
	}
	if err := json.Unmarshal(startResult.Body.Bytes(), &begin); err != nil {
		t.Fatal(err)
	}
	authorize, err := url.Parse(begin.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	callbackPath := "/api/v1/auth/wechat/callback?" + url.Values{"state": {authorize.Query().Get("state")}, "code": {"test-code"}}.Encode()
	callbackResult := httptest.NewRecorder()
	handler.ServeHTTP(callbackResult, httptest.NewRequest(http.MethodGet, callbackPath, nil))
	if callbackResult.Code != http.StatusFound || !strings.Contains(callbackResult.Header().Get("Location"), "code=") || strings.Contains(callbackResult.Header().Get("Location"), "token") {
		t.Fatalf("desktop callback redirect = %d %q", callbackResult.Code, callbackResult.Header().Get("Location"))
	}
	location, err := url.Parse(callbackResult.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var handoff struct{ Code, State string }
	values := location.Query()
	handoff.Code, handoff.State = values.Get("code"), values.Get("state")
	body, _ := json.Marshal(handoff)
	exchange := httptest.NewRecorder()
	handler.ServeHTTP(exchange, httptest.NewRequest(http.MethodPost, "/api/v1/auth/wechat/handoff", strings.NewReader(string(body))))
	if exchange.Code != http.StatusOK || strings.Contains(exchange.Body.String(), "union-handoff") {
		t.Fatalf("handoff exchange = %d %s", exchange.Code, exchange.Body.String())
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, httptest.NewRequest(http.MethodPost, "/api/v1/auth/wechat/handoff", strings.NewReader(string(body))))
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("handoff replay status = %d", replay.Code)
	}
}
