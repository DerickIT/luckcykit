package app

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/store"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cloud-app.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	return db
}

func TestNewOptionsSelectOptionalServicesAndSandboxCatalog(t *testing.T) {
	db := newTestDB(t)
	app, err := New(Options{DB: db, SessionTTL: time.Hour, Sandbox: true})
	if err != nil {
		t.Fatal(err)
	}
	if app.EmailOTP != nil || app.Tasks != nil {
		t.Fatalf("optional services should be disabled without adapters: otp=%v tasks=%v", app.EmailOTP, app.Tasks)
	}
	products, err := app.Catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 2 {
		t.Fatalf("sandbox catalog product count = %d, want 2", len(products))
	}
}

func TestNewOptionsBootstrapCreatesInitialAdmin(t *testing.T) {
	db := newTestDB(t)
	_, err := New(Options{
		DB: db, SessionTTL: time.Hour,
		AdminBootstrapSecret: "bootstrap-secret-012345678901234567890123",
		AdminBootstrapEmail:  "admin@example.com",
		InitialAdminSecret:   "admin-secret-0123456789012345678901234567",
	})
	if err != nil {
		t.Fatal(err)
	}
	var account store.AdminAccount
	if err := db.Where("email = ?", "admin@example.com").First(&account).Error; err != nil {
		t.Fatalf("initial admin was not created: %v", err)
	}
}
