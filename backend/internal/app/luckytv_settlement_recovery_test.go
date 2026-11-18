package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"luckytv/backend/internal/generation"
	"luckytv/backend/internal/model"
)

func TestLuckyTVVideoSettlementPendingThenFailed(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	svc, db := openDBRuntimeMatrix(t, t.TempDir())
	defer closeDBRuntimeMatrix(t, svc, db)

	var polls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/videos/vid-settle-1" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/video/generations/vid-settle-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		call := polls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                 "vid-settle-1",
				"status":             "FAILURE",
				"usage_state":        "pending",
				"reconcile_deadline": "2026-04-20T12:00:00Z",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "vid-settle-1",
			"status":      "FAILURE",
			"usage_state": "failed",
		})
	}))
	defer gateway.Close()

	inputJSON, err := json.Marshal(canvasGenerationInput{
		Mode:   "video",
		Prompt: "test video",
		Config: providerConfig{
			BaseURL:       gateway.URL,
			APIKey:        "test-key",
			Model:         "seedance-2.0",
			InterfaceType: "newapi",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	task := model.Task{
		ID:                "lucky-video-settle",
		UserID:            "local",
		Type:              "canvas_video",
		Status:            model.TaskStatusFailed,
		ProviderRequestID: "vid-settle-1",
		InputJSON:         string(inputJSON),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	ctx := generation.WithEndpoints(context.Background(), generation.Endpoints{LuckyTVVideoBaseURL: gateway.URL})
	_ = svc.recoverFailedVideo(ctx, task)

	first, err := svc.Task("local", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.LuckyUsageState == nil || *first.LuckyUsageState != "pending" {
		t.Fatalf("LuckyUsageState = %v, want pending", first.LuckyUsageState)
	}
	if first.NextPollAt == nil {
		t.Fatalf("expected NextPollAt to be scheduled while usage_state is pending")
	}
	if first.Outcome == nil || first.Outcome.Phase != "failed" || first.Outcome.Regenerate != "blocked" || first.Outcome.Reason != "settlement_pending" || first.Outcome.ReconcileDeadline != "2026-04-20T12:00:00Z" {
		t.Fatalf("first outcome = %+v", first.Outcome)
	}

	_ = svc.recoverFailedVideo(ctx, *first)

	second, err := svc.Task("local", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.LuckyUsageState == nil || *second.LuckyUsageState != "failed" {
		t.Fatalf("LuckyUsageState = %v, want failed", second.LuckyUsageState)
	}
	if second.LuckyReconcileDeadline != nil {
		t.Fatalf("expected LuckyReconcileDeadline cleared, got %v", *second.LuckyReconcileDeadline)
	}
	if second.NextPollAt != nil {
		t.Fatalf("expected NextPollAt cleared once usage_state settled, got %v", second.NextPollAt)
	}
	if second.Outcome == nil || second.Outcome.Phase != "failed" || second.Outcome.Regenerate != "allowed" || second.Outcome.Reason != "no_result" {
		t.Fatalf("second outcome = %+v", second.Outcome)
	}
}

func TestLuckyTVImageJobSettlementPendingRefundedAndAbsent(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")

	t.Run("pending-to-refunded", func(t *testing.T) {
		s, db, _, _ := creationTestService(t)
		var polls atomic.Int32
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || r.URL.Path != "/v1/images/jobs/job-img-1" {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			call := polls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":                 "job-img-1",
					"status":             "failed",
					"billing_state":      "",
					"reconcile_deadline": "2026-04-20T15:00:00Z",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "job-img-1",
				"status":        "failed",
				"billing_state": "refunded",
			})
		}))
		defer gateway.Close()

		inputJSON, _ := json.Marshal(canvasGenerationInput{
			Mode: "image",
			Config: providerConfig{
				BaseURL: gateway.URL,
				APIKey:  "test-key",
				Model:   "gpt-image-1",
			},
		})
		task := model.Task{
			ID:         "lucky-img-pending",
			UserID:     "user",
			Type:       "canvas_image",
			Status:     model.TaskStatusFailed,
			Stage:      "submission_unknown",
			ImageJobID: "job-img-1",
			InputJSON:  string(inputJSON),
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}

		ctx := generation.WithEndpoints(context.Background(), generation.Endpoints{LuckyTVVideoBaseURL: gateway.URL})
		if err := s.recoverFailedImage(ctx, task); err != nil {
			t.Fatal(err)
		}
		first, err := s.Task("user", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if first.LuckyBillingState == nil || *first.LuckyBillingState != "" {
			t.Fatalf("LuckyBillingState = %v, want empty string", first.LuckyBillingState)
		}
		if first.NextPollAt == nil {
			t.Fatalf("expected NextPollAt scheduled while billing_state is empty string")
		}
		if first.Outcome == nil || first.Outcome.Phase != "failed" || first.Outcome.Regenerate != "blocked" || first.Outcome.Reason != "settlement_pending" || first.Outcome.ReconcileDeadline != "2026-04-20T15:00:00Z" {
			t.Fatalf("first outcome = %+v", first.Outcome)
		}

		if err := s.recoverFailedImage(ctx, *first); err != nil {
			t.Fatal(err)
		}
		second, err := s.Task("user", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if second.LuckyBillingState == nil || *second.LuckyBillingState != "refunded" {
			t.Fatalf("LuckyBillingState = %v, want refunded", second.LuckyBillingState)
		}
		if second.NextPollAt != nil {
			t.Fatalf("expected NextPollAt cleared once billing_state settled, got %v", second.NextPollAt)
		}
		if second.Outcome == nil || second.Outcome.Phase != "failed" || second.Outcome.Regenerate != "allowed" || second.Outcome.Reason != "result_unrecoverable" {
			t.Fatalf("second outcome = %+v", second.Outcome)
		}
	})

	t.Run("settled-unrecoverable", func(t *testing.T) {
		s, db, _, _ := creationTestService(t)
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":            "job-img-2",
				"status":        "completed",
				"billing_state": "settled",
			})
		}))
		defer gateway.Close()

		inputJSON, _ := json.Marshal(canvasGenerationInput{
			Mode: "image",
			Config: providerConfig{
				BaseURL: gateway.URL,
				APIKey:  "test-key",
				Model:   "gpt-image-1",
			},
		})
		task := model.Task{
			ID:         "lucky-img-settled",
			UserID:     "user",
			Type:       "canvas_image",
			Status:     model.TaskStatusFailed,
			Stage:      "submission_unknown",
			ImageJobID: "job-img-2",
			InputJSON:  string(inputJSON),
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}

		ctx := generation.WithEndpoints(context.Background(), generation.Endpoints{LuckyTVVideoBaseURL: gateway.URL})
		if err := s.recoverFailedImage(ctx, task); err != nil {
			t.Fatal(err)
		}
		got, err := s.Task("user", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Outcome == nil || got.Outcome.Phase != "failed" || got.Outcome.Regenerate != "confirm" || got.Outcome.Reason != "charged_result_unrecoverable" {
			t.Fatalf("settled unrecoverable outcome = %+v", got.Outcome)
		}
	})

	t.Run("billing-state-absent-fallback", func(t *testing.T) {
		s, db, _, _ := creationTestService(t)
		gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "job-img-3",
				"status": "failed",
			})
		}))
		defer gateway.Close()

		inputJSON, _ := json.Marshal(canvasGenerationInput{
			Mode: "image",
			Config: providerConfig{
				BaseURL: gateway.URL,
				APIKey:  "test-key",
				Model:   "gpt-image-1",
			},
		})
		task := model.Task{
			ID:         "lucky-img-absent",
			UserID:     "user",
			Type:       "canvas_image",
			Status:     model.TaskStatusFailed,
			Stage:      "submission_unknown",
			ImageJobID: "job-img-3",
			InputJSON:  string(inputJSON),
		}
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}

		ctx := generation.WithEndpoints(context.Background(), generation.Endpoints{LuckyTVVideoBaseURL: gateway.URL})
		if err := s.recoverFailedImage(ctx, task); err != nil {
			t.Fatal(err)
		}
		got, err := s.Task("user", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.LuckyBillingState != nil {
			t.Fatalf("expected LuckyBillingState to remain nil when billing_state key is absent, got %q", *got.LuckyBillingState)
		}
		if got.NextPollAt != nil {
			t.Fatalf("expected NextPollAt to be nil when billing_state key is absent")
		}
		if got.Outcome == nil || got.Outcome.Phase != "failed" || got.Outcome.Regenerate != "confirm" {
			t.Fatalf("absent billing_state fallback outcome = %+v", got.Outcome)
		}
	})
}
