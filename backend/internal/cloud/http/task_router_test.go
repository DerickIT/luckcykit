package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/store"
)

func TestModelTaskRoutesRequireSessionAndSettleOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "generated"}}},
			"usage":   map[string]int64{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:cloud-task-routes?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	gateway, err := modelgateway.New(upstream.URL, "oneapi-key", upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	services, err := cloudapp.New(cloudapp.Options{
		DB: db, SessionTTL: time.Hour, Sandbox: true, Gateway: gateway,
		Models: []store.HostedModel{{ID: "text-fast", UpstreamID: "provider-text", PriceVersion: "price-v1",
			MaxInputTokens: 50, MaxOutputTokens: 50, InputPointsPerMillion: 1_000_000, OutputPointsPerMillion: 1_000_000, Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	services.Start(context.Background())
	defer services.Close()
	handler := NewRouter(Options{SandboxEnabled: true, Identity: services.Identity, Catalog: services.Catalog, HostedModels: services.HostedModels, Ledger: services.Ledger, Payment: services.Payment, Tasks: services.Tasks})

	login := request(handler, http.MethodPost, "/api/v1/sandbox/login", "", "127.0.0.1:50000", `{"subject":"task-creator"}`)
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil || session.Token == "" {
		t.Fatalf("login failed: %d %s", login.Code, login.Body.String())
	}
	// Fund the account through the same sandbox purchase path the UI uses.
	created := request(handler, http.MethodPost, "/api/v1/orders", session.Token, "127.0.0.1:50000", `{"productId":"sandbox-points-100"}`)
	var order struct {
		Order store.Order `json:"order"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil || order.Order.ID == "" {
		t.Fatalf("order creation failed: %d %s", created.Code, created.Body.String())
	}
	if paid := requestWithIdempotency(handler, "/api/v1/orders/"+order.Order.ID+"/sandbox-payment", session.Token, "127.0.0.1:50000", "pay-task"); paid.Code != http.StatusOK {
		t.Fatalf("payment failed: %d %s", paid.Code, paid.Body.String())
	}

	submit := func(token, body, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/model-tasks", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:50000"
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	const body = `{"modelId":"text-fast","messages":[{"role":"user","content":"hello"}],"maxOutputTokens":20}`

	if anon := submit("", body, "task-1"); anon.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated submit status = %d", anon.Code)
	}
	if unknown := submit(session.Token, strings.Replace(body, "text-fast", "unknown-model", 1), "task-unknown"); unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown model status = %d body=%s", unknown.Code, unknown.Body.String())
	}

	accepted := submit(session.Token, body, "task-1")
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d body=%s", accepted.Code, accepted.Body.String())
	}
	var acceptedBody struct {
		Task     store.CloudTask `json:"task"`
		Replayed bool            `json:"replayed"`
	}
	if err := json.Unmarshal(accepted.Body.Bytes(), &acceptedBody); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if acceptedBody.Task.Status == "completed" {
			break
		}
		time.Sleep(5 * time.Millisecond)
		accepted = request(handler, http.MethodGet, "/api/v1/model-tasks/"+acceptedBody.Task.ID, session.Token, "127.0.0.1:50000", "")
		var current struct {
			Task store.CloudTask `json:"task"`
		}
		if err := json.Unmarshal(accepted.Body.Bytes(), &current); err == nil {
			acceptedBody.Task = current.Task
		}
	}
	if acceptedBody.Replayed || acceptedBody.Task.Status != "completed" || acceptedBody.Task.ChargedPoints != 5 {
		t.Fatalf("unexpected task result: %+v", acceptedBody)
	}

	replay := submit(session.Token, body, "task-1")
	if replay.Code != http.StatusAccepted || !bytes.Contains(replay.Body.Bytes(), []byte(`"replayed":true`)) {
		t.Fatalf("replay response = %d %s", replay.Code, replay.Body.String())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("idempotent replay called upstream %d times", upstreamCalls.Load())
	}
	balance, err := ledger.New(db).Balance(acceptedBody.Task.AccountID)
	if err != nil || balance.Points != 95 || balance.ReservedPoints != 0 {
		t.Fatalf("unexpected settled balance: %+v err=%v", balance, err)
	}
}
