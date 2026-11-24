package identity

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

func newAPITokenTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "api-tokens.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&store.Account{ID: "account-token", CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return New(db, time.Hour, nil)
}

func TestAPITokenIsOneTimeSecretAndScopeBound(t *testing.T) {
	service := newAPITokenTestService(t)
	result, err := service.CreateAPIToken("account-token", "desktop", 90, []string{ScopeAccountRead, ScopeHostedTasksRead})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.Info.Prefix == "" || result.Info.Suffix == "" || len(result.Info.Scopes) != 2 {
		t.Fatalf("incomplete token result: %+v", result)
	}
	records, err := service.ListAPITokens("account-token")
	if err != nil || len(records) != 1 || records[0].Name != "desktop" || records[0].Prefix == result.Token {
		t.Fatalf("token metadata leaked or missing: records=%+v err=%v", records, err)
	}
	auth, err := service.AuthenticateAPIToken(result.Token, ScopeAccountRead)
	if err != nil || auth.AccountID != "account-token" {
		t.Fatalf("account scope rejected: %+v %v", auth, err)
	}
	if _, err := service.AuthenticateAPIToken(result.Token, ScopeHostedInvoke); !errors.Is(err, ErrInvalidAPITokenScope) {
		t.Fatalf("ungranted scope accepted: %v", err)
	}
	if err := service.RevokeAPIToken("account-token", result.Info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(result.Token, ScopeAccountRead); !errors.Is(err, ErrInvalidAPIToken) {
		t.Fatalf("revoked token accepted: %v", err)
	}
}

func TestAPITokenLimitAndExpiry(t *testing.T) {
	service := newAPITokenTestService(t)
	for range APITokenMaxActive {
		if _, err := service.CreateAPIToken("account-token", "token", 7, nil); err != nil {
			t.Fatalf("create token: %v", err)
		}
	}
	if _, err := service.CreateAPIToken("account-token", "overflow", 7, nil); !errors.Is(err, ErrAPITokenLimit) {
		t.Fatalf("token limit not enforced: %v", err)
	}
}
