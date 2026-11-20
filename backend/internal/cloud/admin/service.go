package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/store"
)

const (
	bootstrapID = "initial"
	sessionTTL  = 8 * time.Hour
)

var (
	ErrBootstrapUnavailable = errors.New("admin bootstrap unavailable")
	ErrBootstrapUsed        = errors.New("admin bootstrap already used")
	ErrInvalidAdminSession  = errors.New("invalid or expired admin session")
	ErrAdminInactive        = errors.New("admin account inactive")
	ErrPermissionDenied     = errors.New("admin permission denied")
)

type Role string

const (
	RoleReadOnly Role = "read-only"
	RolePricing  Role = "pricing"
	RoleBilling  Role = "billing"
	RoleSystem   Role = "system"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

type LoginResult struct {
	Admin   store.AdminAccount `json:"admin"`
	Token   string             `json:"token"`
	Expires time.Time          `json:"expiresAt"`
}

func New(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SeedBootstrap(secret string) error {
	if len(secret) < 32 {
		return ErrBootstrapUnavailable
	}
	var existing store.AdminBootstrap
	if err := s.db.Where("id = ?", bootstrapID).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(&store.AdminBootstrap{ID: bootstrapID, SecretHash: secretHash(secret), CreatedAt: s.now()}).Error
}

func (s *Service) Bootstrap(secret, email, adminSecret string) (store.AdminAccount, error) {
	email, err := normalizeEmail(email)
	if err != nil || len(secret) < 32 || len(adminSecret) < 32 {
		return store.AdminAccount{}, ErrBootstrapUnavailable
	}
	now := s.now()
	var account store.AdminAccount
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var bootstrap store.AdminBootstrap
		if err := tx.Where("id = ?", bootstrapID).First(&bootstrap).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBootstrapUnavailable
		} else if err != nil {
			return err
		}
		if bootstrap.UsedAt != nil || !hmac.Equal([]byte(bootstrap.SecretHash), []byte(secretHash(secret))) {
			return ErrBootstrapUsed
		}
		account = store.AdminAccount{ID: uuid.NewString(), Email: email, SecretHash: secretHash(adminSecret), Role: string(RoleSystem), Active: true, CreatedAt: now}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		return tx.Model(&store.AdminBootstrap{}).Where("id = ? AND used_at IS NULL", bootstrapID).Update("used_at", now).Error
	})
	if err != nil {
		return store.AdminAccount{}, err
	}
	return account, nil
}

func (s *Service) Login(email, secret string) (LoginResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return LoginResult{}, ErrInvalidAdminSession
	}
	var account store.AdminAccount
	if err := s.db.Where("email = ? AND active = ?", email, true).First(&account).Error; err != nil {
		return LoginResult{}, ErrInvalidAdminSession
	}
	if !hmac.Equal([]byte(account.SecretHash), []byte(secretHash(secret))) {
		return LoginResult{}, ErrInvalidAdminSession
	}
	result, err := s.issueSession(account)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.Audit(s.db, account.ID, "admin.login", "admin", account.ID, "success"); err != nil {
		return LoginResult{}, fmt.Errorf("record admin login audit: %w", err)
	}
	return result, nil
}

func (s *Service) issueSession(account store.AdminAccount) (LoginResult, error) {
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return LoginResult{}, fmt.Errorf("generate admin session: %w", err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	now := s.now()
	session := store.AdminSession{ID: uuid.NewString(), AdminID: account.ID, TokenHash: tokenHash(token), ExpiresAt: now.Add(sessionTTL), CreatedAt: now}
	if err := s.db.Create(&session).Error; err != nil {
		return LoginResult{}, fmt.Errorf("create admin session: %w", err)
	}
	return LoginResult{Admin: account, Token: token, Expires: session.ExpiresAt}, nil
}

func (s *Service) Authenticate(token string) (store.AdminAccount, error) {
	var session store.AdminSession
	if strings.TrimSpace(token) == "" || s.db.Where("token_hash = ? AND expires_at > ? AND revoked_at IS NULL", tokenHash(token), s.now()).First(&session).Error != nil {
		return store.AdminAccount{}, ErrInvalidAdminSession
	}
	var account store.AdminAccount
	if err := s.db.Where("id = ? AND active = ?", session.AdminID, true).First(&account).Error; err != nil {
		return store.AdminAccount{}, ErrInvalidAdminSession
	}
	return account, nil
}

func (s *Service) RequireRole(account store.AdminAccount, action string) error {
	allowed := map[Role]map[string]bool{
		RoleReadOnly: {"accounts.read": true, "orders.read": true, "tasks.read": true},
		RolePricing:  {"accounts.read": true, "orders.read": true, "tasks.read": true, "pricing.write": true, "models.write": true},
		RoleBilling:  {"accounts.read": true, "orders.read": true, "tasks.read": true, "billing.write": true},
		RoleSystem:   {"accounts.read": true, "orders.read": true, "tasks.read": true, "pricing.write": true, "models.write": true, "billing.write": true, "admins.write": true},
	}
	if !account.Active || !allowed[Role(account.Role)][action] {
		return ErrPermissionDenied
	}
	return nil
}

func (s *Service) Audit(tx *gorm.DB, adminID, action, resourceType, resourceID, outcome string) error {
	if strings.TrimSpace(adminID) == "" || strings.TrimSpace(action) == "" || strings.TrimSpace(resourceType) == "" || strings.TrimSpace(outcome) == "" {
		return ErrPermissionDenied
	}
	return tx.Create(&store.AuditEvent{ID: uuid.NewString(), AdminID: adminID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome, CreatedAt: s.now()}).Error
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") || len(email) > 320 {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func secretHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func tokenHash(token string) string { return secretHash(token) }
