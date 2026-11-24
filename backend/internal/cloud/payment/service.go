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
	ErrPaymentMismatch     = errors.New("payment confirmation does not match order")
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
			ProductName: product.Name, Currency: product.Currency, AmountCNY: product.AmountCNY, Points: product.Points, Status: "pending", CreatedAt: s.now()}
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
	var order store.Order
	if err := s.db.Where("id = ? AND account_id = ?", orderID, accountID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.Order{}, false, ErrOrderNotFound
		}
		return store.Order{}, false, err
	}
	return s.ConfirmPayment(PaymentConfirmation{AccountID: accountID, OrderID: orderID, IdempotencyKey: idempotencyKey, Provider: "sandbox", ProviderEventID: idempotencyKey, ProviderAccountID: accountID, Currency: order.Currency, AmountCNY: order.AmountCNY})
}

type PaymentConfirmation struct {
	AccountID         string
	OrderID           string
	IdempotencyKey    string
	Provider          string
	ProviderEventID   string
	ProviderAccountID string
	Currency          string
	AmountCNY         int64
}

func (s *Service) ConfirmPayment(input PaymentConfirmation) (store.Order, bool, error) {
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.OrderID = strings.TrimSpace(input.OrderID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Provider = strings.TrimSpace(input.Provider)
	input.ProviderEventID = strings.TrimSpace(input.ProviderEventID)
	input.ProviderAccountID = strings.TrimSpace(input.ProviderAccountID)
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.AccountID == "" || input.OrderID == "" || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 160 || input.Provider == "" || input.ProviderEventID == "" || input.ProviderAccountID == "" || input.Currency == "" || input.AmountCNY <= 0 {
		return store.Order{}, false, ErrPaymentMismatch
	}
	var order store.Order
	replayed := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var eventForKey store.PaymentEvent
		keyErr := tx.Where("account_id = ? AND idempotency_key = ?", input.AccountID, input.IdempotencyKey).First(&eventForKey).Error
		if keyErr == nil {
			if eventForKey.OrderID != input.OrderID {
				return ErrIdempotencyConflict
			}
			replayed = true
			return tx.Where("id = ? AND account_id = ?", input.OrderID, input.AccountID).First(&order).Error
		}
		if !errors.Is(keyErr, gorm.ErrRecordNotFound) {
			return keyErr
		}
		if err := tx.Where("id = ? AND account_id = ?", input.OrderID, input.AccountID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if order.Status != "pending" {
			return ErrOrderPaid
		}
		if order.Currency != input.Currency || order.AmountCNY != input.AmountCNY || input.ProviderAccountID != input.AccountID {
			return ErrPaymentMismatch
		}
		var providerEvent store.PaymentEvent
		if err := tx.Where("provider = ? AND provider_event_id = ? AND provider_account_id = ?", input.Provider, input.ProviderEventID, input.ProviderAccountID).First(&providerEvent).Error; err == nil {
			return ErrIdempotencyConflict
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := s.now()
		event := store.PaymentEvent{ID: uuid.NewString(), OrderID: order.ID, AccountID: input.AccountID, IdempotencyKey: input.IdempotencyKey, Provider: input.Provider, ProviderEventID: input.ProviderEventID, ProviderAccountID: input.ProviderAccountID, Currency: input.Currency, AmountCNY: input.AmountCNY, CreatedAt: now}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := s.ledger.CreditInTx(tx, input.AccountID, order.ID, order.Points); err != nil {
			return err
		}
		updated := tx.Model(&store.Order{}).Where("id = ? AND account_id = ? AND status = ?", order.ID, input.AccountID, "pending").Updates(map[string]any{"status": "paid", "paid_at": now})
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
