package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCloudMigrationDoesNotCreateLocalWorkspaceTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-schema-boundary?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "account_identities", "sessions", "products", "orders", "payment_events", "ledger_entries"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("cloud table %q was not created", table)
		}
	}
	var versions int64
	if err := db.Model(&SchemaMigration{}).Where("version = ?", schemaVersionEmailAdmin).Count(&versions).Error; err != nil || versions != 1 {
		t.Fatalf("cloud schema version was not recorded exactly once: count=%d err=%v", versions, err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("repeat cloud migration: %v", err)
	}
	if err := db.Model(&SchemaMigration{}).Count(&versions).Error; err != nil || versions != int64(schemaVersionEmailAdmin) {
		t.Fatalf("repeat migration changed version history: count=%d err=%v", versions, err)
	}
	for _, table := range []string{"workspaces", "projects", "canvas_projects", "tasks", "resources"} {
		if db.Migrator().HasTable(table) {
			t.Errorf("local workspace table %q was created by cloud migration", table)
		}
	}
}

func TestCloudMigrationUpgradesExistingV1Database(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-schema-upgrade?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := db.AutoMigrate(&SchemaMigration{}, &Account{}, &AccountIdentity{}, &Session{}, &Product{}, &Order{}, &PaymentEvent{}, &LedgerEntry{}, &HostedModel{}, &CloudTask{}, &HostedUsage{}, &PointReservation{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&SchemaMigration{Version: schemaVersionBase, AppliedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("email_verification_codes") {
		t.Fatal("fixture unexpectedly already has the v2 table")
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("upgrade v1 database: %v", err)
	}
	for _, table := range []string{"email_verification_codes", "admin_accounts", "admin_sessions", "admin_bootstraps", "audit_events"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("v2 table %q was not created on upgrade", table)
		}
	}
	var version int64
	if err := db.Model(&SchemaMigration{}).Where("version = ?", schemaVersionEmailAdmin).Count(&version).Error; err != nil || version != 1 {
		t.Fatalf("v2 was not recorded exactly once: count=%d err=%v", version, err)
	}
}

func TestCloudConstraintsRejectInvalidOrdersAndLedgerEntries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-schema-constraints?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Create(&Account{ID: "account-1", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	validOrder := Order{ID: "order-1", AccountID: "account-1", ProductID: "product", ProductName: "test", Points: 10, Status: "pending", CreatedAt: now}
	if err := db.Create(&validOrder).Error; err != nil {
		t.Fatal(err)
	}
	invalidOrders := []Order{
		{ID: "order-zero", AccountID: "account-1", ProductID: "product", ProductName: "test", Points: 0, Status: "pending", CreatedAt: now},
		{ID: "order-status", AccountID: "account-1", ProductID: "product", ProductName: "test", Points: 1, Status: "refunded", CreatedAt: now},
	}
	for _, order := range invalidOrders {
		if err := db.Create(&order).Error; err == nil {
			t.Errorf("invalid order accepted: %+v", order)
		}
	}
	if err := db.Create(&LedgerEntry{ID: "ledger-orphan", AccountID: "account-missing", SourceType: "test", SourceID: "orphan", Kind: "test", Points: 1, CreatedAt: now}).Error; err == nil {
		t.Fatal("ledger entry for unknown account was accepted")
	}
	for _, entry := range []LedgerEntry{
		{ID: "ledger-zero", AccountID: "account-1", OrderID: "order-1", SourceType: "test", SourceID: "zero", Kind: "test", Points: 0, CreatedAt: now},
		{ID: "ledger-foreign", AccountID: "account-2", OrderID: "order-1", SourceType: "test", SourceID: "foreign", Kind: "test", Points: 10, CreatedAt: now},
	} {
		if err := db.Create(&entry).Error; err == nil {
			t.Errorf("invalid ledger entry accepted: %+v", entry)
		}
	}
}
