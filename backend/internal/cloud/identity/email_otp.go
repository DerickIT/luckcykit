package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/store"
)

const (
	verificationCodeTTL      = 10 * time.Minute
	verificationResendWindow = time.Minute
	verificationMaxAttempts  = 5
)

var (
	ErrEmailOTPUnavailable = errors.New("email verification is unavailable")
	ErrEmailOTPInvalid     = errors.New("invalid or expired verification code")
	ErrEmailOTPRateLimited = errors.New("verification code request rate limited")
)

type VerificationMailer interface {
	SendVerificationCode(context.Context, string, string) error
}

type EmailOTPService struct {
	db        *gorm.DB
	identity  *Service
	mailer    VerificationMailer
	secret    []byte
	now       func() time.Time
	requestMu sync.Mutex
}

func NewEmailOTPService(db *gorm.DB, identities *Service, mailer VerificationMailer, secret []byte) *EmailOTPService {
	return &EmailOTPService{
		db: db, identity: identities, mailer: mailer,
		secret: append([]byte(nil), secret...),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// RequestCode never checks whether an account exists; delivery behavior is
// independent of account state to avoid an email-address enumeration oracle.
func (s *EmailOTPService) RequestCode(ctx context.Context, rawEmail string) error {
	if len(s.secret) < 32 || s.mailer == nil {
		return ErrEmailOTPUnavailable
	}
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	now := s.now()
	var recent store.EmailVerificationCode
	err = s.db.Where("email = ? AND created_at > ?", email, now.Add(-verificationResendWindow)).
		Order("created_at DESC").First(&recent).Error
	if err == nil {
		return ErrEmailOTPRateLimited
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check email verification rate: %w", err)
	}

	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Errorf("generate verification code: %w", err)
	}
	code := fmt.Sprintf("%06d", binary.BigEndian.Uint32(random[:])%1_000_000)
	record := store.EmailVerificationCode{
		ID: uuid.NewString(), Email: email, CodeHash: s.codeHash(email, code),
		ExpiresAt: now.Add(verificationCodeTTL), CreatedAt: now,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.EmailVerificationCode{}).
			Where("email = ? AND used_at IS NULL", email).
			Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Create(&record).Error
	}); err != nil {
		return fmt.Errorf("store email verification code: %w", err)
	}
	if err := s.mailer.SendVerificationCode(ctx, email, code); err != nil {
		_ = s.db.Model(&store.EmailVerificationCode{}).
			Where("id = ? AND used_at IS NULL", record.ID).
			Update("used_at", s.now()).Error
		return fmt.Errorf("send email verification code: %w", err)
	}
	return nil
}

func (s *EmailOTPService) VerifyCode(rawEmail, rawCode string) (LoginResult, error) {
	if len(s.secret) < 32 || s.identity == nil {
		return LoginResult{}, ErrEmailOTPUnavailable
	}
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return LoginResult{}, ErrEmailOTPInvalid
	}
	code := strings.TrimSpace(rawCode)
	if len(code) != 6 || !digitsOnly(code) {
		return LoginResult{}, ErrEmailOTPInvalid
	}
	now := s.now()
	var result LoginResult
	invalidCode := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var record store.EmailVerificationCode
		if err := tx.Where("email = ? AND used_at IS NULL AND expires_at > ? AND attempts < ?", email, now, verificationMaxAttempts).
			Order("created_at DESC").First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEmailOTPInvalid
			}
			return fmt.Errorf("read email verification code: %w", err)
		}
		if !hmac.Equal([]byte(record.CodeHash), []byte(s.codeHash(email, code))) {
			updated := tx.Model(&store.EmailVerificationCode{}).
				Where("id = ? AND used_at IS NULL AND attempts < ?", record.ID, verificationMaxAttempts).
				UpdateColumn("attempts", gorm.Expr("attempts + 1"))
			if updated.Error != nil {
				return fmt.Errorf("record verification attempt: %w", updated.Error)
			}
			invalidCode = true
			return nil
		}
		consumed := tx.Model(&store.EmailVerificationCode{}).
			Where("id = ? AND used_at IS NULL AND expires_at > ? AND attempts < ?", record.ID, now, verificationMaxAttempts).
			Update("used_at", now)
		if consumed.Error != nil {
			return fmt.Errorf("consume email verification code: %w", consumed.Error)
		}
		if consumed.RowsAffected != 1 {
			return ErrEmailOTPInvalid
		}
		account, err := findOrCreateAccount(tx, ProviderIdentity{Provider: "email", Subject: email}, now)
		if err != nil {
			return fmt.Errorf("resolve email account: %w", err)
		}
		return s.identity.createSessionIn(tx, account, now, &result)
	})
	if err != nil {
		return LoginResult{}, err
	}
	if invalidCode {
		return LoginResult{}, ErrEmailOTPInvalid
	}
	return result, nil
}

func (s *EmailOTPService) codeHash(email, code string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(email))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) > 320 {
		return "", ErrEmailOTPInvalid
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return "", ErrEmailOTPInvalid
	}
	return email, nil
}

func digitsOnly(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
