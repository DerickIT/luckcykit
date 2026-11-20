package payment

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/store"
)

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderPaid           = errors.New("order already paid")
	ErrIdempotency         = errors.New("idempotency key is required and must be at most 160 characters")
	ErrIdempotencyConflict = errors.New("idempotency key was used for another order")
)

type Service struct {
	db      *gorm.DB
	catalog *catalog.Service
	ledger  *ledger.Service
	now     func() time.Time
}

func New(db *gorm.DB, products *catalog.Service, points *ledger.Service) *Service {
	return &Service{db: db, catalog: products, ledger: points, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) CreateOrder(accountID, productID string) (store.Order, error) {
	var order store.Order
	err := s.db.Transaction(func(tx *gorm.DB) error {
		product, err := s.catalog.FindInTx(tx, strings.TrimSpace(productID))
		if err != nil {
			return err
		}
		order = store.Order{ID: uuid.NewString(), AccountID: accountID, ProductID: product.ID,
			ProductName: product.Name, Points: product.Points, Status: "pending", CreatedAt: s.now()}
		return tx.Create(&order).Error
	})
	if err != nil {
		return store.Order{}, fmt.Errorf("create sandbox order: %w", err)
	}
	return order, nil
}

func (s *Service) SimulatePayment(accountID, orderID, idempotencyKey string) (store.Order, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 160 {
		return store.Order{}, false, ErrIdempotency
	}
	order, replayed, err := s.simulatePaymentOnce(accountID, orderID, idempotencyKey)
	if err == nil {
		return order, replayed, nil
	}
	if store.IsSQLiteConflict(err) || store.IsSQLiteRetryable(err) {
		if resolvedOrder, resolved, resolveErr := s.resolveCommittedPayment(accountID, orderID, idempotencyKey); resolveErr != nil {
			return store.Order{}, false, fmt.Errorf("simulate sandbox payment: %w", resolveErr)
		} else if resolved {
			return resolvedOrder, true, nil
		}
		if store.IsSQLiteRetryable(err) {
			retryOrder, retryReplay, retryErr := s.simulatePaymentOnce(accountID, orderID, idempotencyKey)
			if retryErr == nil {
				return retryOrder, retryReplay, nil
			}
			if resolvedOrder, resolved, resolveErr := s.resolveCommittedPayment(accountID, orderID, idempotencyKey); resolveErr != nil {
				return store.Order{}, false, fmt.Errorf("simulate sandbox payment: %w", resolveErr)
			} else if resolved {
				return resolvedOrder, true, nil
			}
			return store.Order{}, false, fmt.Errorf("simulate sandbox payment: %w", retryErr)
		}
	}
	return store.Order{}, false, fmt.Errorf("simulate sandbox payment: %w", err)
}

func (s *Service) simulatePaymentOnce(accountID, orderID, idempotencyKey string) (store.Order, bool, error) {
	var order store.Order
	replayed := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var eventForKey store.PaymentEvent
		keyErr := tx.Where("account_id = ? AND idempotency_key = ?", accountID, idempotencyKey).First(&eventForKey).Error
		if keyErr == nil && eventForKey.OrderID != orderID {
			return ErrIdempotencyConflict
		}
		if keyErr != nil && !errors.Is(keyErr, gorm.ErrRecordNotFound) {
			return keyErr
		}
		if err := tx.Where("id = ? AND account_id = ?", orderID, accountID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		var existing store.PaymentEvent
		err := tx.Where("order_id = ?", order.ID).First(&existing).Error
		if err == nil {
			if existing.IdempotencyKey != idempotencyKey {
				return ErrOrderPaid
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if order.Status != "pending" {
			return ErrOrderPaid
		}
		now := s.now()
		event := store.PaymentEvent{ID: uuid.NewString(), OrderID: order.ID, AccountID: accountID,
			IdempotencyKey: idempotencyKey, Provider: "sandbox", CreatedAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := s.ledger.CreditInTx(tx, accountID, order.ID, order.Points); err != nil {
			return err
		}
		updated := tx.Model(&store.Order{}).Where("id = ? AND account_id = ? AND status = ?", order.ID, accountID, "pending").
			Updates(map[string]any{"status": "paid", "paid_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrOrderPaid
		}
		order.Status = "paid"
		order.PaidAt = &now
		return nil
	})
	if err != nil {
		return store.Order{}, false, err
	}
	return order, replayed, nil
}

func (s *Service) resolveCommittedPayment(accountID, orderID, idempotencyKey string) (store.Order, bool, error) {
	var event store.PaymentEvent
	err := s.db.Where("account_id = ? AND idempotency_key = ?", accountID, idempotencyKey).First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = s.db.Where("account_id = ? AND order_id = ?", accountID, orderID).First(&event).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Order{}, false, nil
		}
		if err != nil {
			return store.Order{}, false, err
		}
		if event.IdempotencyKey != idempotencyKey {
			return store.Order{}, false, ErrOrderPaid
		}
	} else if err != nil {
		return store.Order{}, false, err
	} else if event.OrderID != orderID {
		return store.Order{}, false, ErrIdempotencyConflict
	}
	var order store.Order
	if err := s.db.Where("id = ? AND account_id = ?", orderID, accountID).First(&order).Error; err != nil {
		return store.Order{}, false, err
	}
	return order, true, nil
}

func (s *Service) GetOrder(accountID, orderID string) (store.Order, error) {
	var order store.Order
	err := s.db.Where("id = ? AND account_id = ?", orderID, accountID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Order{}, ErrOrderNotFound
	}
	if err != nil {
		return store.Order{}, fmt.Errorf("read sandbox order: %w", err)
	}
	return order, nil
}

func (s *Service) ListOrders(accountID string) ([]store.Order, error) {
	orders := make([]store.Order, 0)
	if err := s.db.Where("account_id = ?", accountID).Order("created_at DESC").Order("id DESC").Limit(100).Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("list sandbox orders: %w", err)
	}
	return orders, nil
}
