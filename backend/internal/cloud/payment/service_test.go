package payment

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/store"
)

func newTestServices(t *testing.T) (*gorm.DB, *catalog.Service, *ledger.Service, *Service) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "cloud-test.db") + "?_busy_timeout=5000&_journal_mode=WAL"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	products := catalog.New(db)
	if err := products.EnsureSandboxProducts(); err != nil {
		t.Fatal(err)
	}
	points := ledger.New(db)
	orders := New(db, products, points)
	return db, products, points, orders
}

func TestCreateOrderSnapshotsCatalogAndPaymentIsIdempotent(t *testing.T) {
	db, products, points, orders := newTestServices(t)
	var account store.Account
	if err := db.Create(&store.Account{ID: "account-1", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	account.ID = "account-1"

	order, err := orders.CreateOrder(account.ID, "sandbox-points-100")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&store.Product{}).Where("id = ?", order.ProductID).Updates(map[string]any{"name": "changed", "points": 900}).Error; err != nil {
		t.Fatal(err)
	}
	if err := products.EnsureSandboxProducts(); err != nil {
		t.Fatal(err)
	}
	if order.ProductName != "沙箱点数 100" || order.Points != 100 {
		t.Fatalf("order did not preserve product snapshot: %+v", order)
	}

	paid, replayed, err := orders.SimulatePayment(account.ID, order.ID, "payment-1")
	if err != nil || replayed || paid.Status != "paid" {
		t.Fatalf("first payment: order=%+v replayed=%t err=%v", paid, replayed, err)
	}
	_, replayed, err = orders.SimulatePayment(account.ID, order.ID, "payment-1")
	if err != nil || !replayed {
		t.Fatalf("repeated payment should replay: replayed=%t err=%v", replayed, err)
	}
	if _, _, err := orders.SimulatePayment(account.ID, order.ID, "payment-2"); !errors.Is(err, ErrOrderPaid) {
		t.Fatalf("different payment event must be rejected, got %v", err)
	}
	secondOrder, err := orders.CreateOrder(account.ID, "sandbox-points-500")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := orders.SimulatePayment(account.ID, secondOrder.ID, "payment-1"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency key reuse across orders must be rejected, got %v", err)
	}
	balance, err := points.Balance(account.ID)
	if err != nil || balance.Points != 100 {
		t.Fatalf("ledger-derived balance: %+v err=%v", balance, err)
	}
	entries, err := points.Entries(account.ID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one ledger entry, got %d err=%v", len(entries), err)
	}
	if err := db.Model(&store.LedgerEntry{}).Where("id = ?", entries[0].ID).Update("points", 999).Error; err == nil {
		t.Fatal("ledger update unexpectedly succeeded")
	}
	if err := db.Where("id = ?", entries[0].ID).Delete(&store.LedgerEntry{}).Error; err == nil {
		t.Fatal("ledger delete unexpectedly succeeded")
	}
	balance, err = points.Balance(account.ID)
	if err != nil || balance.Points != 100 {
		t.Fatalf("ledger balance changed after rejected mutation: %+v err=%v", balance, err)
	}
}

func TestConcurrentSandboxPaymentCreditsOnce(t *testing.T) {
	db, _, points, orders := newTestServices(t)
	if err := db.Create(&store.Account{ID: "account-concurrent", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	order, err := orders.CreateOrder("account-concurrent", "sandbox-points-100")
	if err != nil {
		t.Fatal(err)
	}

	type paymentResult struct {
		replayed bool
		err      error
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan paymentResult, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, replayed, err := orders.SimulatePayment("account-concurrent", order.ID, "same-payment")
			results <- paymentResult{replayed: replayed, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, replays := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("same-key concurrent retry failed: %v", result.err)
		}
		if result.replayed {
			replays++
		} else {
			successes++
		}
	}
	if successes != 1 || replays != 1 {
		t.Fatalf("expected one payment and one replay, got successes=%d replays=%d", successes, replays)
	}
	balance, err := points.Balance("account-concurrent")
	if err != nil || balance.Points != 100 {
		t.Fatalf("concurrent payment credited more than once: %+v err=%v", balance, err)
	}
	entries, err := points.Entries("account-concurrent")
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one ledger entry, got %d err=%v", len(entries), err)
	}
}

func TestConcurrentIdempotencyKeyAcrossOrdersIsRejected(t *testing.T) {
	db, _, points, orders := newTestServices(t)
	if err := db.Create(&store.Account{ID: "account-cross", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	first, err := orders.CreateOrder("account-cross", "sandbox-points-100")
	if err != nil {
		t.Fatal(err)
	}
	second, err := orders.CreateOrder("account-cross", "sandbox-points-500")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, orderID := range []string{first.ID, second.ID} {
		wg.Add(1)
		go func(orderID string) {
			defer wg.Done()
			<-start
			_, _, err := orders.SimulatePayment("account-cross", orderID, "shared-key")
			errs <- err
		}(orderID)
	}
	close(start)
	wg.Wait()
	close(errs)
	conflicts := 0
	for err := range errs {
		if errors.Is(err, ErrIdempotencyConflict) {
			conflicts++
		} else if err != nil {
			t.Fatalf("unexpected concurrent cross-order result: %v", err)
		}
	}
	if conflicts != 1 {
		t.Fatalf("expected exactly one conflicting order request, got %d", conflicts)
	}
	balance, err := points.Balance("account-cross")
	if err != nil || balance.Points != 100 && balance.Points != 500 {
		t.Fatalf("expected exactly one order credit, got %+v err=%v", balance, err)
	}
	var events int64
	if err := db.Model(&store.PaymentEvent{}).Where("account_id = ? AND idempotency_key = ?", "account-cross", "shared-key").Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("expected one winning event, count=%d err=%v", events, err)
	}
}

func TestPaymentFailureRollsBackEventAndLedger(t *testing.T) {
	db, _, points, orders := newTestServices(t)
	if err := db.Create(&store.Account{ID: "account-2", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	order, err := orders.CreateOrder("account-2", "sandbox-points-100")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE ledger_entries").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := orders.SimulatePayment("account-2", order.ID, "payment-rollback"); err == nil {
		t.Fatal("payment unexpectedly succeeded with missing ledger table")
	}
	var count int64
	if err := db.Model(&store.PaymentEvent{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("payment event should rollback: count=%d err=%v", count, err)
	}
	if err := db.Table("orders").Where("id = ? AND status = ?", order.ID, "pending").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("order should remain pending: count=%d err=%v", count, err)
	}
	if _, err := points.Balance("account-2"); err == nil {
		t.Fatal("missing ledger table should be observable")
	}
}
