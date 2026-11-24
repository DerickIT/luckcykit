package identity

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/store"
)

func TestOAuthStateIsSingleUseAndExpires(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-oauth-state-test?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	service := New(db, time.Hour, nil)
	state, err := service.CreateOAuthStateWithHandoff(time.Minute, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ConsumeOAuthState(state); err != nil {
		t.Fatalf("consume fresh OAuth state: %v", err)
	}
	if err := service.ConsumeOAuthState(state); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("replayed OAuth state accepted: %v", err)
	}
}
func TestDesktopHandoffCodeIsBoundToStateAndSingleUse(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cloud-handoff-test?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	service := New(db, time.Hour, nil)
	login, err := service.LoginWithIdentity(ProviderIdentity{Provider: "wechat_unionid", Subject: "union-handoff"})
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.CreateOAuthStateWithHandoff(time.Minute, "http://127.0.0.1:45678/callback", "desktop-state")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ConsumeOAuthState(state); err != nil {
		t.Fatal(err)
	}
	handoff, err := service.IssueHandoffCode(login.Account.ID, state)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(handoff.URL)
	if err != nil || parsed.Query().Get("state") != "desktop-state" || parsed.Query().Get("code") == "" {
		t.Fatalf("invalid handoff URL: %s", handoff.URL)
	}
	result, err := service.ExchangeHandoffCodeWithState(parsed.Query().Get("code"), "desktop-state")
	if err != nil || result.Token == "" {
		t.Fatalf("handoff exchange failed: %+v %v", result, err)
	}
	if _, err := service.ExchangeHandoffCodeWithState(parsed.Query().Get("code"), "desktop-state"); !errors.Is(err, ErrInvalidOAuthState) {
		t.Fatalf("handoff code replay accepted: %v", err)
	}
}

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
