package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/store"
)

var (
	ErrTaskNotFound     = errors.New("task not found")
	ErrTaskIdempotency  = errors.New("task idempotency key conflict")
	ErrModelUnavailable = errors.New("hosted model unavailable")
)

type Service struct {
	db      *gorm.DB
	ledger  *ledger.Service
	gateway *modelgateway.Client
	models  map[string]store.HostedModel
	now     func() time.Time
}

type SubmitRequest struct {
	AccountID      string
	IdempotencyKey string
	ModelID        string
	Request        modelgateway.Request
}

type SubmitResult struct {
	Task     store.CloudTask
	Replayed bool
}

func New(db *gorm.DB, points *ledger.Service, gateway *modelgateway.Client, models []store.HostedModel) *Service {
	modelMap := make(map[string]store.HostedModel, len(models))
	for _, model := range models {
		modelMap[model.ID] = model
	}
	return &Service{db: db, ledger: points, gateway: gateway, models: modelMap, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Submit(ctx context.Context, input SubmitRequest) (SubmitResult, error) {
	if strings.TrimSpace(input.AccountID) == "" || len(strings.TrimSpace(input.IdempotencyKey)) == 0 || len(input.IdempotencyKey) > 160 {
		return SubmitResult{}, ErrTaskIdempotency
	}
	model, ok := s.models[input.ModelID]
	if !ok || !model.Enabled || model.MaxInputTokens <= 0 || model.MaxOutputTokens <= 0 || input.Request.Model != "" || input.Request.MaxOutputTokens > model.MaxOutputTokens {
		return SubmitResult{}, ErrModelUnavailable
	}
	requestJSON, err := json.Marshal(input.Request)
	if err != nil {
		return SubmitResult{}, fmt.Errorf("encode model task request: %w", err)
	}
	var result SubmitResult
	now := s.now()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing store.CloudTask
		if err := tx.Where("account_id = ? AND idempotency_key = ?", input.AccountID, input.IdempotencyKey).First(&existing).Error; err == nil {
			if existing.ModelID != input.ModelID || existing.RequestJSON != string(requestJSON) {
				return ErrTaskIdempotency
			}
			result = SubmitResult{Task: existing, Replayed: true}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("read existing model task: %w", err)
		}
		task := store.CloudTask{ID: uuid.NewString(), AccountID: input.AccountID, IdempotencyKey: input.IdempotencyKey, ModelID: model.ID, PriceVersion: model.PriceVersion, RequestJSON: string(requestJSON), Status: "reserved", CreatedAt: now, UpdatedAt: now}
		reservationPoints, err := ledger.PointsForUsage(model.MaxInputTokens, model.MaxOutputTokens, model.InputPointsPerMillion, model.OutputPointsPerMillion)
		if err != nil {
			return fmt.Errorf("calculate task reservation: %w", err)
		}
		task.ReservedPoints = reservationPoints
		if err := tx.Create(&task).Error; err != nil {
			if store.IsSQLiteConflict(err) {
				return ErrTaskIdempotency
			}
			return fmt.Errorf("create model task: %w", err)
		}
		if err := s.ledger.Reserve(tx, task.ID, input.AccountID, reservationPoints, model.MaxInputTokens, model.MaxOutputTokens, model.InputPointsPerMillion, model.OutputPointsPerMillion); err != nil {
			return fmt.Errorf("reserve task points: %w", err)
		}
		result = SubmitResult{Task: task}
		return nil
	}); err != nil {
		return SubmitResult{}, err
	}
	if result.Replayed {
		return result, nil
	}
	if s.gateway == nil {
		return result, nil
	}
	if err := s.execute(ctx, result.Task); err != nil {
		return SubmitResult{}, err
	}
	updated, err := s.Get(input.AccountID, result.Task.ID)
	if err != nil {
		return SubmitResult{}, err
	}
	result.Task = updated
	return result, nil
}

func (s *Service) execute(ctx context.Context, task store.CloudTask) error {
	var request modelgateway.Request
	if err := json.Unmarshal([]byte(task.RequestJSON), &request); err != nil {
		return s.fail(task, "invalid_request", false)
	}
	model, ok := s.models[task.ModelID]
	if !ok || !model.Enabled || model.UpstreamID == "" {
		return s.fail(task, "model_unavailable", true)
	}
	response, err := s.gateway.Call(ctx, model.UpstreamID, request)
	if errors.Is(err, modelgateway.ErrUnknownOutcome) {
		return s.markUnknown(task, "submission_unknown")
	}
	if errors.Is(err, modelgateway.ErrInvalidResponse) {
		return s.markUnknown(task, "usage_invalid")
	}
	if err != nil {
		return s.fail(task, "upstream_failed", true)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		charged, err := s.ledger.Settle(tx, task.ID, task.AccountID, response.Usage.PromptTokens, response.Usage.CompletionTokens)
		if err != nil {
			return s.updateTask(tx, task.ID, "pending-reconciliation", "usage_invalid", task.ResponseText, task.InputTokens, task.OutputTokens, 0)
		}
		return s.updateTask(tx, task.ID, "completed", "", response.Text, response.Usage.PromptTokens, response.Usage.CompletionTokens, charged)
	})
}

func (s *Service) fail(task store.CloudTask, code string, release bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if release {
			if err := s.ledger.Release(tx, task.ID, task.AccountID, "released"); err != nil {
				return err
			}
		}
		return s.updateTask(tx, task.ID, "failed", code, "", 0, 0, 0)
	})
}

func (s *Service) markUnknown(task store.CloudTask, code string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.PointReservation{}).Where("task_id = ? AND account_id = ? AND status = ?", task.ID, task.AccountID, "active").Update("status", "pending-reconciliation").Error; err != nil {
			return err
		}
		return s.updateTask(tx, task.ID, "pending-reconciliation", code, "", 0, 0, 0)
	})
}

func (s *Service) updateTask(tx *gorm.DB, taskID, status, errorCode, response string, inputTokens, outputTokens, charged int64) error {
	updates := map[string]any{"status": status, "error_code": errorCode, "response_text": response, "input_tokens": inputTokens, "output_tokens": outputTokens, "charged_points": charged, "updated_at": s.now()}
	if err := tx.Model(&store.CloudTask{}).Where("id = ?", taskID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update model task: %w", err)
	}
	return nil
}

func (s *Service) Get(accountID, taskID string) (store.CloudTask, error) {
	var task store.CloudTask
	if err := s.db.Where("id = ? AND account_id = ?", taskID, accountID).First(&task).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return store.CloudTask{}, ErrTaskNotFound
	} else if err != nil {
		return store.CloudTask{}, fmt.Errorf("read model task: %w", err)
	}
	return task, nil
}
