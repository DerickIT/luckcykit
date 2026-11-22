package task

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/store"
)

func newTaskTest(t *testing.T, gateway *modelgateway.Client) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "task.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&store.Account{ID: "account-task", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.Order{ID: "order-task", AccountID: "account-task", ProductID: "product", ProductName: "test", Points: 100, Status: "paid", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	points := ledger.New(db)
	if err := points.CreditInTx(db, "account-task", "order-task", 100); err != nil {
		t.Fatal(err)
	}
	models := []store.HostedModel{{ID: "text-fast", UpstreamID: "provider-text", PriceVersion: "price-v1", MaxInputTokens: 50, MaxOutputTokens: 50, InputPointsPerMillion: 1_000_000, OutputPointsPerMillion: 1_000_000, Enabled: true}}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	service := New(db, points, gateway, models)
	service.Start(context.Background())
	t.Cleanup(service.Close)
	return db, service
}

func waitForTask(t *testing.T, service *Service, accountID, taskID string) store.CloudTask {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := service.Get(accountID, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "completed" || got.Status == "failed" || got.Status == "pending-reconciliation" {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("task %s did not finish", taskID)
	return store.CloudTask{}
}

func TestSubmitSettlesUsageAndIdempotencyDoesNotCallUpstreamAgain(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer oneapi-key" {
			t.Errorf("unexpected gateway request: path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": "generated"}}},
			"usage":   map[string]int64{"prompt_tokens": 2, "completion_tokens": 3, "total_tokens": 5},
		})
	}))
	defer server.Close()
	gateway, err := modelgateway.New(server.URL, "oneapi-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	db, service := newTaskTest(t, gateway)
	input := SubmitRequest{AccountID: "account-task", IdempotencyKey: "task-request-1", ModelID: "text-fast", Request: modelgateway.Request{Messages: []modelgateway.Message{{Role: "user", Content: "hello"}}, MaxOutputTokens: 20}}
	first, err := service.Submit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	first.Task = waitForTask(t, service, input.AccountID, first.Task.ID)
	if first.Replayed || first.Task.Status != "completed" || first.Task.ChargedPoints != 5 {
		t.Fatalf("unexpected first task: %+v", first)
	}
	second, err := service.Submit(context.Background(), input)
	if err != nil || !second.Replayed || calls.Load() != 1 {
		t.Fatalf("idempotent replay called upstream: result=%+v calls=%d err=%v", second, calls.Load(), err)
	}
	balance, err := ledger.New(db).Balance("account-task")
	if err != nil || balance.Points != 95 || balance.ReservedPoints != 0 || balance.AvailablePoints != 95 {
		t.Fatalf("unexpected settled balance: %+v err=%v", balance, err)
	}
}

func TestSubmitTimeoutLeavesPendingReconciliationAndReservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer server.Close()
	gateway, err := modelgateway.New(server.URL, "oneapi-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	db, service := newTaskTest(t, gateway)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, err := service.Submit(ctx, SubmitRequest{AccountID: "account-task", IdempotencyKey: "task-timeout", ModelID: "text-fast", Request: modelgateway.Request{Messages: []modelgateway.Message{{Role: "user", Content: "hello"}}}})
	if err != nil {
		t.Fatal(err)
	}
	result.Task = waitForTask(t, service, result.Task.AccountID, result.Task.ID)
	if result.Task.Status != "pending-reconciliation" || result.Task.ErrorCode != "submission_unknown" {
		t.Fatalf("timeout did not enter reconciliation: %+v", result.Task)
	}
	balance, err := ledger.New(db).Balance("account-task")
	if err != nil || balance.ReservedPoints != 100 || balance.AvailablePoints != 0 {
		t.Fatalf("timeout reservation was not retained: %+v err=%v", balance, err)
	}
}

func TestSubmitRejectsClientModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "unused"}}}, "usage": map[string]int64{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
	}))
	defer server.Close()
	gateway, err := modelgateway.New(server.URL, "oneapi-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, service := newTaskTest(t, gateway)
	if _, err := service.Submit(context.Background(), SubmitRequest{AccountID: "account-task", IdempotencyKey: "client-model", ModelID: "text-fast", Request: modelgateway.Request{Model: "provider-text", Messages: []modelgateway.Message{{Role: "user", Content: "hello"}}}}); !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("client supplied upstream model was accepted: %v", err)
	}
}

func TestSuccessfulResponseWithoutUsageKeepsReservationForReconciliation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "generated"}}}})
	}))
	defer server.Close()
	gateway, err := modelgateway.New(server.URL, "oneapi-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	db, service := newTaskTest(t, gateway)
	result, err := service.Submit(context.Background(), SubmitRequest{AccountID: "account-task", IdempotencyKey: "task-no-usage", ModelID: "text-fast", Request: modelgateway.Request{Messages: []modelgateway.Message{{Role: "user", Content: "hello"}}}})
	if err != nil {
		t.Fatal(err)
	}
	result.Task = waitForTask(t, service, result.Task.AccountID, result.Task.ID)
	if result.Task.Status != "pending-reconciliation" || result.Task.ErrorCode != "usage_invalid" {
		t.Fatalf("missing usage was treated as a completed task: %+v", result.Task)
	}
	balance, err := ledger.New(db).Balance("account-task")
	if err != nil || balance.ReservedPoints != 100 || balance.AvailablePoints != 0 {
		t.Fatalf("missing usage released reservation: %+v err=%v", balance, err)
	}
}
