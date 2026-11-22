package admin

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

func newAdminTest(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "admin.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

func TestBootstrapIsSeededAndConsumedOnce(t *testing.T) {
	db, service := newAdminTest(t)
	bootstrapSecret := "bootstrap-secret-012345678901234567890123"
	adminSecret := "admin-secret-0123456789012345678901234567"
	if err := service.SeedBootstrap(bootstrapSecret); err != nil {
		t.Fatal(err)
	}
	account, err := service.Bootstrap(bootstrapSecret, "admin@example.com", adminSecret)
	if err != nil || account.Role != string(RoleSystem) || account.SecretHash == adminSecret {
		t.Fatalf("bootstrap result: %+v err=%v", account, err)
	}
	if _, err := service.Bootstrap(bootstrapSecret, "second@example.com", adminSecret); !errors.Is(err, ErrBootstrapUsed) {
		t.Fatalf("bootstrap replay accepted: %v", err)
	}
	var bootstrap store.AdminBootstrap
	if err := db.First(&bootstrap, "id = ?", bootstrapID).Error; err != nil || bootstrap.UsedAt == nil {
		t.Fatalf("bootstrap was not consumed: %+v err=%v", bootstrap, err)
	}
}

func TestAdminSessionAndRoleAuthorization(t *testing.T) {
	_, service := newAdminTest(t)
	if err := service.SeedBootstrap("bootstrap-secret-012345678901234567890123"); err != nil {
		t.Fatal(err)
	}
	account, err := service.Bootstrap("bootstrap-secret-012345678901234567890123", "admin@example.com", "admin-secret-0123456789012345678901234567")
	if err != nil {
		t.Fatal(err)
	}
	login, err := service.Login(account.Email, "admin-secret-0123456789012345678901234567")
	if err != nil || login.Token == "" {
		t.Fatalf("admin login failed: %+v err=%v", login, err)
	}
	got, err := service.Authenticate(login.Token)
	if err != nil || got.ID != account.ID {
		t.Fatalf("admin session failed: %+v err=%v", got, err)
	}
	if err := service.RequireRole(got, "pricing.write"); err != nil {
		t.Fatalf("system admin denied pricing permission: %v", err)
	}
	readOnly := got
	readOnly.Role = string(RoleReadOnly)
	if err := service.RequireRole(readOnly, "pricing.write"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("read-only admin allowed pricing write: %v", err)
	}
	if err := service.RequireRole(readOnly, "accounts.read"); err != nil {
		t.Fatalf("read-only admin denied read permission: %v", err)
	}
}

func TestAuditIsAppendOnlyAndScopedToAdmin(t *testing.T) {
	db, service := newAdminTest(t)
	adminHash, err := hashAdminSecret("admin-secret-0123456789012345678901234567")
	if err != nil {
		t.Fatal(err)
	}
	adminAccount := store.AdminAccount{ID: "admin-audit", Email: "audit@example.com", SecretHash: adminHash, Role: string(RoleSystem), Active: true, CreatedAt: time.Now().UTC()}
	if err := db.Create(&adminAccount).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Audit(db, adminAccount.ID, "pricing.write", "model", "model-1", "success"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&store.AuditEvent{}).Where("admin_id = ?", adminAccount.ID).Update("outcome", "tampered").Error; err == nil {
		t.Fatal("audit event update unexpectedly succeeded")
	}
	if err := db.Where("admin_id = ?", adminAccount.ID).Delete(&store.AuditEvent{}).Error; err == nil {
		t.Fatal("audit event delete unexpectedly succeeded")
	}
	if err := service.Audit(db, "missing-admin", "pricing.write", "model", "model-2", "success"); err == nil {
		t.Fatal("audit event for missing admin unexpectedly succeeded")
	}
}
