package ledger

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/store"
)

func newLedgerTest(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ledger.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, New(db)
}

func TestReserveSettleAndReleaseAreAccountScopedAndIdempotent(t *testing.T) {
	db, service := newLedgerTest(t)
	now := time.Now().UTC()
	if err := db.Create(&store.Account{ID: "account-ledger", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.Order{ID: "order-ledger", AccountID: "account-ledger", ProductID: "product", ProductName: "test", Points: 100, Status: "paid", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.CreditInTx(db, "account-ledger", "order-ledger", 100); err != nil {
		t.Fatal(err)
	}
	task := store.CloudTask{ID: "task-ledger", AccountID: "account-ledger", IdempotencyKey: "request-ledger", ModelID: "model", PriceVersion: "price-v1", RequestJSON: "{}", Status: "submitted", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Reserve(db, task.ID, task.AccountID, 50, 1000000, 1000000, 1000000, 1000000); err != nil {
		t.Fatal(err)
	}
	balance, err := service.Balance(task.AccountID)
	if err != nil || balance.AvailablePoints != 50 || balance.ReservedPoints != 50 {
		t.Fatalf("unexpected reserved balance: %+v err=%v", balance, err)
	}
	if err := service.Reserve(db, "task-too-many", task.AccountID, 51, 1, 1, 1000000, 1000000); !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("overdraft reservation accepted: %v", err)
	}
	charged, err := service.Settle(db, task.ID, task.AccountID, 2, 3)
	if err != nil || charged != 5 {
		t.Fatalf("settle result: charged=%d err=%v", charged, err)
	}
	charged, err = service.Settle(db, task.ID, task.AccountID, 2, 3)
	if err != nil || charged != 5 {
		t.Fatalf("settle replay result: charged=%d err=%v", charged, err)
	}
	balance, err = service.Balance(task.AccountID)
	if err != nil || balance.Points != 95 || balance.AvailablePoints != 95 || balance.ReservedPoints != 0 {
		t.Fatalf("unexpected settled balance: %+v err=%v", balance, err)
	}
}

func TestReserveUnknownTaskKeepsPointsUnavailable(t *testing.T) {
	db, service := newLedgerTest(t)
	now := time.Now().UTC()
	if err := db.Create(&store.Account{ID: "account-unknown", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.Order{ID: "order-unknown", AccountID: "account-unknown", ProductID: "product", ProductName: "test", Points: 20, Status: "paid", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.CreditInTx(db, "account-unknown", "order-unknown", 20); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.CloudTask{ID: "task-unknown", AccountID: "account-unknown", IdempotencyKey: "request-unknown", ModelID: "model", PriceVersion: "price-v1", RequestJSON: "{}", Status: "submitted", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Reserve(db, "task-unknown", "account-unknown", 20, 100, 100, 1000000, 1000000); err != nil {
		t.Fatal(err)
	}
	if err := service.Release(db, "task-unknown", "account-unknown", "pending-reconciliation"); err != nil {
		t.Fatal(err)
	}
	balance, err := service.Balance("account-unknown")
	if err != nil || balance.ReservedPoints != 20 || balance.AvailablePoints != 0 {
		t.Fatalf("unknown task reservation was released unexpectedly: %+v err=%v", balance, err)
	}
	if err := service.Release(db, "task-unknown", "account-unknown", "pending-reconciliation"); err != nil {
		t.Fatalf("idempotent reconciliation marker failed: %v", err)
	}
}

func TestPointsForUsageRoundsUpAndRejectsOverflow(t *testing.T) {
	points, err := PointsForUsage(1, 1, 1, 1)
	if err != nil || points != 2 {
		t.Fatalf("expected each partial million to round up: points=%d err=%v", points, err)
	}
	if _, err := PointsForUsage(-1, 0, 1, 1); !errors.Is(err, ErrInvalidUsage) {
		t.Fatalf("negative usage accepted: %v", err)
	}
}
