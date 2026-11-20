package identity

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"luckytv/backend/internal/cloud/store"
)

type fakeVerificationMailer struct{ Messages []verificationMessage }

type verificationMessage struct{ Email, Code string }

func (m *fakeVerificationMailer) SendVerificationCode(_ context.Context, email, code string) error {
	m.Messages = append(m.Messages, verificationMessage{Email: email, Code: code})
	return nil
}

func newEmailOTPTestService(t *testing.T) (*EmailOTPService, *fakeVerificationMailer, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "otp.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	mailer := &fakeVerificationMailer{}
	identities := New(db, time.Hour, nil)
	service := NewEmailOTPService(db, identities, mailer, []byte("01234567890123456789012345678901"))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return service, mailer, db
}

func TestEmailOTPConsumesOnceAndStoresOnlyHash(t *testing.T) {
	service, mailer, db := newEmailOTPTestService(t)
	if err := service.RequestCode(context.Background(), "Creator@Example.com"); err != nil {
		t.Fatal(err)
	}
	if len(mailer.Messages) != 1 || mailer.Messages[0].Email != "creator@example.com" {
		t.Fatalf("unexpected delivery: %+v", mailer.Messages)
	}
	var record store.EmailVerificationCode
	if err := db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.CodeHash == mailer.Messages[0].Code || len(record.CodeHash) != 64 {
		t.Fatalf("verification code was not stored as a keyed digest: %+v", record)
	}
	login, err := service.VerifyCode("creator@example.com", mailer.Messages[0].Code)
	if err != nil || login.Token == "" {
		t.Fatalf("valid code rejected: login=%+v err=%v", login, err)
	}
	if _, err := service.VerifyCode("creator@example.com", mailer.Messages[0].Code); !errors.Is(err, ErrEmailOTPInvalid) {
		t.Fatalf("verification code replay accepted: %v", err)
	}
	if _, err := service.identity.Authenticate(login.Token); err != nil {
		t.Fatalf("email session rejected: %v", err)
	}
}

func TestEmailOTPWrongCodeAttemptsAndExpiry(t *testing.T) {
	service, mailer, db := newEmailOTPTestService(t)
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	if err := service.RequestCode(context.Background(), "creator@example.com"); err != nil {
		t.Fatal(err)
	}
	for range verificationMaxAttempts {
		if _, err := service.VerifyCode("creator@example.com", "000000"); !errors.Is(err, ErrEmailOTPInvalid) {
			t.Fatalf("wrong code result: %v", err)
		}
	}
	var record store.EmailVerificationCode
	if err := db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Attempts != verificationMaxAttempts {
		t.Fatalf("wrong attempts not capped: %d", record.Attempts)
	}
	if _, err := service.VerifyCode("creator@example.com", mailer.Messages[0].Code); !errors.Is(err, ErrEmailOTPInvalid) {
		t.Fatalf("code became valid after max attempts: %v", err)
	}
}

func TestEmailOTPRequestIsRateLimitedAndInvalidEmailDoesNotSend(t *testing.T) {
	service, mailer, _ := newEmailOTPTestService(t)
	if err := service.RequestCode(context.Background(), "not-an-email"); !errors.Is(err, ErrEmailOTPInvalid) {
		t.Fatalf("invalid email result: %v", err)
	}
	if len(mailer.Messages) != 0 {
		t.Fatal("invalid email was sent")
	}
	if err := service.RequestCode(context.Background(), "creator@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := service.RequestCode(context.Background(), "creator@example.com"); !errors.Is(err, ErrEmailOTPRateLimited) {
		t.Fatalf("repeat request was not rate limited: %v", err)
	}
}
