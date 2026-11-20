package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/store"
)

func TestSandboxIdentitySessionLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-identity-test?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	service := New(db, time.Hour, SandboxProvider{})
	first, err := service.SandboxLogin(context.Background(), "creator-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.SandboxLogin(context.Background(), "creator-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Account.ID != second.Account.ID || first.Token == second.Token {
		t.Fatalf("logins should share account but issue distinct sessions: first=%+v second=%+v", first, second)
	}
	if _, err := service.Authenticate(first.Token); err != nil {
		t.Fatalf("fresh session rejected: %v", err)
	}
	if err := service.Revoke(first.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(first.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("revoked session accepted: %v", err)
	}
	if _, err := service.Authenticate(second.Token); err != nil {
		t.Fatalf("revoking one session affected another: %v", err)
	}
}

func TestConcurrentFirstLoginSharesAccountAndIssuesDistinctSessions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-identity-concurrent?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	service := New(db, time.Hour, SandboxProvider{})
	start := make(chan struct{})
	results := make(chan LoginResult, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, err := service.SandboxLogin(context.Background(), "same-first-login")
			results <- result
			errs <- err
		}()
	}
	close(start)
	first, second := <-results, <-results
	firstErr, secondErr := <-errs, <-errs
	if firstErr != nil || secondErr != nil {
		t.Fatalf("concurrent login failed: %v / %v", firstErr, secondErr)
	}
	if first.Account.ID != second.Account.ID || first.Token == second.Token {
		t.Fatalf("concurrent login should share account and issue distinct sessions: %+v / %+v", first, second)
	}
	if _, err := service.Authenticate(first.Token); err != nil {
		t.Fatalf("first concurrent session invalid: %v", err)
	}
	if _, err := service.Authenticate(second.Token); err != nil {
		t.Fatalf("second concurrent session invalid: %v", err)
	}
}

func TestSessionExpiration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-session-expiry?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	service := New(db, time.Hour, SandboxProvider{})
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	login, err := service.SandboxLogin(context.Background(), "creator-expiry")
	if err != nil {
		t.Fatal(err)
	}
	now = login.Expires.Add(time.Second)
	if _, err := service.Authenticate(login.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired session accepted: %v", err)
	}
}
