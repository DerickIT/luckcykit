package ledger

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/store"
)

var (
	ErrInsufficientPoints = errors.New("insufficient available points")
	ErrInvalidReservation = errors.New("invalid point reservation")
	ErrInvalidUsage       = errors.New("invalid token usage")
)

type Service struct{ db *gorm.DB }

type Balance struct {
	AccountID       string `json:"accountId"`
	Points          int64  `json:"points"`
	ReservedPoints  int64  `json:"reservedPoints"`
	AvailablePoints int64  `json:"availablePoints"`
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// CreditInTx is the sole ledger write path; callers must include it in their
// order/payment transaction. Existing entries are never updated or deleted.
func (s *Service) CreditInTx(tx *gorm.DB, accountID, orderID string, points int64) error {
	if points <= 0 {
		return fmt.Errorf("ledger credit must be a positive integer")
	}
	return tx.Create(&store.LedgerEntry{
		ID: uuid.NewString(), AccountID: accountID, OrderID: orderID,
		SourceType: "sandbox_payment", SourceID: orderID,
		Kind: "sandbox_purchase", Points: points, CreatedAt: time.Now().UTC(),
	}).Error
}

func (s *Service) Reserve(tx *gorm.DB, taskID, accountID string, points, maxInput, maxOutput, inputRate, outputRate int64) error {
	if taskID == "" || accountID == "" || points <= 0 || maxInput <= 0 || maxOutput <= 0 || inputRate < 0 || outputRate < 0 || inputRate == 0 && outputRate == 0 {
		return ErrInvalidReservation
	}
	var balance int64
	if err := tx.Model(&store.LedgerEntry{}).Where("account_id = ?", accountID).Select("COALESCE(SUM(points), 0)").Scan(&balance).Error; err != nil {
		return fmt.Errorf("read ledger balance for reservation: %w", err)
	}
	var reserved int64
	if err := tx.Model(&store.PointReservation{}).Where("account_id = ? AND status IN ?", accountID, []string{"active", "pending-reconciliation"}).Select("COALESCE(SUM(points), 0)").Scan(&reserved).Error; err != nil {
		return fmt.Errorf("read active reservations: %w", err)
	}
	if balance < reserved || points > balance-reserved {
		return ErrInsufficientPoints
	}
	now := time.Now().UTC()
	return tx.Create(&store.PointReservation{
		ID: uuid.NewString(), TaskID: taskID, AccountID: accountID, Points: points,
		MaxInputTokens: maxInput, MaxOutputTokens: maxOutput,
		InputPointsPerMillion: inputRate, OutputPointsPerMillion: outputRate,
		Status: "active", CreatedAt: now, UpdatedAt: now,
	}).Error
}

func (s *Service) Settle(tx *gorm.DB, taskID, accountID string, inputTokens, outputTokens int64) (int64, error) {
	var reservation store.PointReservation
	if err := tx.Where("task_id = ? AND account_id = ?", taskID, accountID).First(&reservation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrInvalidReservation
		}
		return 0, fmt.Errorf("read point reservation: %w", err)
	}
	var usage store.HostedUsage
	if err := tx.Where("task_id = ?", taskID).First(&usage).Error; err == nil {
		var previous store.LedgerEntry
		if err := tx.Where("source_type = ? AND source_id = ?", "hosted_usage", taskID).First(&previous).Error; err == nil {
			return -previous.Points, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("read existing usage charge: %w", err)
		}
		return 0, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("read existing usage: %w", err)
	}
	if reservation.Status != "active" || inputTokens < 0 || outputTokens < 0 || inputTokens > reservation.MaxInputTokens || outputTokens > reservation.MaxOutputTokens {
		return 0, ErrInvalidUsage
	}
	charged, err := PointsForUsage(inputTokens, outputTokens, reservation.InputPointsPerMillion, reservation.OutputPointsPerMillion)
	if err != nil || charged > reservation.Points {
		return 0, ErrInvalidUsage
	}
	now := time.Now().UTC()
	if charged > 0 {
		entry := store.LedgerEntry{ID: uuid.NewString(), AccountID: accountID, SourceType: "hosted_usage", SourceID: taskID, Kind: "model_usage", Points: -charged, CreatedAt: now}
		if err := tx.Create(&entry).Error; err != nil {
			return 0, fmt.Errorf("append model usage charge: %w", err)
		}
	}
	usage = store.HostedUsage{ID: uuid.NewString(), TaskID: taskID, InputTokens: inputTokens, OutputTokens: outputTokens, TotalTokens: inputTokens + outputTokens, CreatedAt: now}
	if err := tx.Create(&usage).Error; err != nil {
		return 0, fmt.Errorf("store model usage: %w", err)
	}
	updated := tx.Model(&store.PointReservation{}).Where("id = ? AND account_id = ? AND status = ?", reservation.ID, accountID, "active").Updates(map[string]any{"status": "settled", "updated_at": now})
	if updated.Error != nil {
		return 0, fmt.Errorf("settle point reservation: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return 0, ErrInvalidReservation
	}
	return charged, nil
}

func (s *Service) Release(tx *gorm.DB, taskID, accountID, status string) error {
	if status != "released" && status != "pending-reconciliation" {
		return ErrInvalidReservation
	}
	updated := tx.Model(&store.PointReservation{}).Where("task_id = ? AND account_id = ? AND status = ?", taskID, accountID, "active").Update("status", status)
	if updated.Error != nil {
		return fmt.Errorf("update point reservation: %w", updated.Error)
	}
	if updated.RowsAffected == 0 {
		var reservation store.PointReservation
		if err := tx.Where("task_id = ? AND account_id = ? AND status = ?", taskID, accountID, status).First(&reservation).Error; err == nil {
			return nil
		}
		return ErrInvalidReservation
	}
	return nil
}

func PointsForUsage(inputTokens, outputTokens, inputRate, outputRate int64) (int64, error) {
	if inputTokens < 0 || outputTokens < 0 || inputRate < 0 || outputRate < 0 || inputRate == 0 && outputRate == 0 {
		return 0, ErrInvalidUsage
	}
	input, err := ceilRate(inputTokens, inputRate)
	if err != nil {
		return 0, err
	}
	output, err := ceilRate(outputTokens, outputRate)
	if err != nil || input > math.MaxInt64-output {
		return 0, ErrInvalidUsage
	}
	return input + output, nil
}

func ceilRate(tokens, rate int64) (int64, error) {
	if tokens == 0 || rate == 0 {
		return 0, nil
	}
	if tokens > math.MaxInt64/rate {
		return 0, ErrInvalidUsage
	}
	value := tokens * rate
	return value/1_000_000 + boolToInt(value%1_000_000 != 0), nil
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (s *Service) Balance(accountID string) (Balance, error) {
	var total int64
	if err := s.db.Model(&store.LedgerEntry{}).Where("account_id = ?", accountID).
		Select("COALESCE(SUM(points), 0)").Scan(&total).Error; err != nil {
		return Balance{}, fmt.Errorf("read ledger balance: %w", err)
	}
	var reserved int64
	if err := s.db.Model(&store.PointReservation{}).Where("account_id = ? AND status IN ?", accountID, []string{"active", "pending-reconciliation"}).Select("COALESCE(SUM(points), 0)").Scan(&reserved).Error; err != nil {
		return Balance{}, fmt.Errorf("read point reservations: %w", err)
	}
	return Balance{AccountID: accountID, Points: total, ReservedPoints: reserved, AvailablePoints: total - reserved}, nil
}

func (s *Service) Entries(accountID string) ([]store.LedgerEntry, error) {
	entries := make([]store.LedgerEntry, 0)
	if err := s.db.Where("account_id = ?", accountID).Order("created_at ASC").Order("id ASC").Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("read ledger entries: %w", err)
	}
	return entries, nil
}
