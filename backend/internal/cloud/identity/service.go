package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"net"
	"net/url"
	"strings"
	"time"

	"luckytv/backend/internal/cloud/store"
)

var (
	ErrInvalidSession    = errors.New("invalid or expired session")
	ErrInvalidSubject    = errors.New("sandbox subject must contain 1 to 80 characters")
	ErrInvalidOAuthState = errors.New("invalid or expired OAuth state")
	ErrNoDesktopHandoff  = errors.New("OAuth state has no desktop handoff")
)

type Service struct {
	db         *gorm.DB
	sessionTTL time.Duration
	provider   Provider
	now        func() time.Time
}

type LoginResult struct {
	Account store.Account `json:"account"`
	Token   string        `json:"token"`
	Expires time.Time     `json:"expiresAt"`
}

func New(db *gorm.DB, sessionTTL time.Duration, provider Provider) *Service {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return &Service{db: db, sessionTTL: sessionTTL, provider: provider, now: func() time.Time { return time.Now().UTC() }}
}

// SandboxLogin resolves an identity only through the injected provider. The
// sandbox provider and route are mounted only by an explicitly enabled
// loopback development runtime.
func (s *Service) SandboxLogin(ctx context.Context, subject string) (LoginResult, error) {
	if s.provider == nil {
		return LoginResult{}, errors.New("identity provider is unavailable")
	}
	identity, err := s.provider.Resolve(ctx, subject)
	if err != nil {
		return LoginResult{}, err
	}
	account, err := s.resolveAccount(identity)
	if err != nil {
		return LoginResult{}, fmt.Errorf("create sandbox account: %w", err)
	}
	return s.issueSession(account)
}

func (s *Service) LoginWithIdentity(identity ProviderIdentity) (LoginResult, error) {
	if strings.TrimSpace(identity.Provider) == "" || strings.TrimSpace(identity.Subject) == "" {
		return LoginResult{}, ErrInvalidSubject
	}
	account, err := s.resolveAccount(identity)
	if err != nil {
		return LoginResult{}, fmt.Errorf("resolve cloud identity: %w", err)
	}
	return s.issueSession(account)
}

func (s *Service) CreateOAuthStateWithHandoff(ttl time.Duration, handoffURL, clientState string) (string, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	handoffURL = strings.TrimSpace(handoffURL)
	clientState = strings.TrimSpace(clientState)
	if handoffURL != "" || clientState != "" {
		parsed, err := url.Parse(handoffURL)
		if err != nil || parsed.Scheme != "http" || !isLoopbackHandoffHost(parsed.Hostname()) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || clientState == "" || len(handoffURL) > 512 || len(clientState) > 160 {
			return "", ErrInvalidOAuthState
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	state := hex.EncodeToString(raw)
	now := s.now()
	if err := s.db.Create(&store.OAuthState{ID: state, HandoffURL: handoffURL, ClientState: clientState, ExpiresAt: now.Add(ttl), CreatedAt: now}).Error; err != nil {
		return "", fmt.Errorf("store OAuth state: %w", err)
	}
	return state, nil
}

func (s *Service) ConsumeOAuthState(state string) error {
	state = strings.TrimSpace(state)
	if state == "" {
		return ErrInvalidOAuthState
	}
	now := s.now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&store.OAuthState{}).Where("id = ? AND expires_at > ? AND consumed_at IS NULL", state, now).Update("consumed_at", now)
		if updated.Error != nil {
			return fmt.Errorf("consume OAuth state: %w", updated.Error)
		}
		if updated.RowsAffected != 1 {
			return ErrInvalidOAuthState
		}
		return nil
	})
}

type HandoffResult struct {
	URL   string
	State string
}

func isLoopbackHandoffHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) IssueHandoffCode(accountID, state string) (HandoffResult, error) {
	var oauthState store.OAuthState
	if err := s.db.Where("id = ? AND consumed_at IS NOT NULL", strings.TrimSpace(state)).First(&oauthState).Error; err != nil {
		return HandoffResult{}, ErrInvalidOAuthState
	}
	if strings.TrimSpace(accountID) == "" || oauthState.ExpiresAt.Before(s.now()) {
		return HandoffResult{}, ErrInvalidOAuthState
	}
	if strings.TrimSpace(oauthState.HandoffURL) == "" {
		return HandoffResult{}, ErrNoDesktopHandoff
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return HandoffResult{}, fmt.Errorf("generate handoff code: %w", err)
	}
	code := hex.EncodeToString(raw)
	now := s.now()
	handoff := store.HandoffCode{ID: uuid.NewString(), AccountID: accountID, CodeHash: hashToken(code), ClientState: oauthState.ClientState, ExpiresAt: now.Add(2 * time.Minute), CreatedAt: now}
	if err := s.db.Create(&handoff).Error; err != nil {
		return HandoffResult{}, fmt.Errorf("store handoff code: %w", err)
	}
	parsed, err := url.Parse(oauthState.HandoffURL)
	if err != nil {
		return HandoffResult{}, ErrInvalidOAuthState
	}
	query := parsed.Query()
	query.Set("code", code)
	query.Set("state", oauthState.ClientState)
	parsed.RawQuery = query.Encode()
	return HandoffResult{URL: parsed.String(), State: oauthState.ClientState}, nil
}

func (s *Service) ExchangeHandoffCodeWithState(code, clientState string) (LoginResult, error) {
	return s.exchangeHandoffCode(code, strings.TrimSpace(clientState))
}

func (s *Service) exchangeHandoffCode(code, clientState string) (LoginResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return LoginResult{}, ErrInvalidOAuthState
	}
	now := s.now()
	var result LoginResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var handoff store.HandoffCode
		if err := tx.Where("code_hash = ? AND expires_at > ? AND consumed_at IS NULL", hashToken(code), now).First(&handoff).Error; err != nil {
			return ErrInvalidOAuthState
		}
		if clientState != "" && handoff.ClientState != clientState {
			return ErrInvalidOAuthState
		}
		consumed := tx.Model(&store.HandoffCode{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", handoff.ID, now).Update("consumed_at", now)
		if consumed.Error != nil {
			return consumed.Error
		}
		if consumed.RowsAffected != 1 {
			return ErrInvalidOAuthState
		}
		var account store.Account
		if err := tx.First(&account, "id = ?", handoff.AccountID).Error; err != nil {
			return ErrInvalidOAuthState
		}
		return s.createSessionIn(tx, account, now, &result)
	})
	if err != nil {
		return LoginResult{}, err
	}
	return result, nil
}

func (s *Service) issueSession(account store.Account) (LoginResult, error) {
	now := s.now()
	var result LoginResult
	if err := s.createSessionIn(s.db, account, now, &result); err != nil {
		return LoginResult{}, fmt.Errorf("create cloud session: %w", err)
	}
	return result, nil
}

func (s *Service) createSessionIn(db *gorm.DB, account store.Account, now time.Time, result *LoginResult) error {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(rawToken)
	session := store.Session{
		ID: uuid.NewString(), AccountID: account.ID,
		TokenHash: hashToken(token), ExpiresAt: now.Add(s.sessionTTL), CreatedAt: now,
	}
	if err := db.Create(&session).Error; err != nil {
		return err
	}
	*result = LoginResult{Account: account, Token: token, Expires: session.ExpiresAt}
	return nil
}

func (s *Service) resolveAccount(identity ProviderIdentity) (store.Account, error) {
	for attempt := 0; attempt < 5; attempt++ {
		account, err := s.resolveAccountOnce(identity)
		if err == nil {
			return account, nil
		}
		if !store.IsSQLiteConflict(err) && !store.IsSQLiteRetryable(err) {
			return store.Account{}, err
		}
		if account, lookupErr := s.findAccount(identity); lookupErr == nil {
			return account, nil
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) && !store.IsSQLiteRetryable(lookupErr) {
			return store.Account{}, lookupErr
		}
		if attempt == 4 {
			return store.Account{}, err
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return store.Account{}, errors.New("unable to resolve identity")
}

func (s *Service) resolveAccountOnce(identity ProviderIdentity) (store.Account, error) {
	var account store.Account
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		account, err = findOrCreateAccount(tx, identity, s.now())
		return err
	})
	return account, err
}

// findOrCreateAccount is the only place that links a provider identity to an
// account; concurrent first logins rely on its unique-index conflict handling.
func findOrCreateAccount(tx *gorm.DB, identity ProviderIdentity, now time.Time) (store.Account, error) {
	var linked store.AccountIdentity
	err := tx.Where("provider = ? AND subject = ?", identity.Provider, identity.Subject).First(&linked).Error
	if err == nil {
		var account store.Account
		if err := tx.First(&account, "id = ?", linked.AccountID).Error; err != nil {
			return store.Account{}, err
		}
		return account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Account{}, err
	}
	account := store.Account{ID: uuid.NewString(), CreatedAt: now}
	if err := tx.Create(&account).Error; err != nil {
		return store.Account{}, err
	}
	linked = store.AccountIdentity{ID: uuid.NewString(), AccountID: account.ID, Provider: identity.Provider, Subject: identity.Subject, CreatedAt: now}
	if err := tx.Create(&linked).Error; err != nil {
		return store.Account{}, err
	}
	return account, nil
}

func (s *Service) findAccount(identity ProviderIdentity) (store.Account, error) {
	return s.findAccountIn(s.db, identity)
}

func (s *Service) findAccountIn(db *gorm.DB, identity ProviderIdentity) (store.Account, error) {
	var linked store.AccountIdentity
	if err := db.Where("provider = ? AND subject = ?", identity.Provider, identity.Subject).First(&linked).Error; err != nil {
		return store.Account{}, err
	}
	var account store.Account
	err := db.First(&account, "id = ?", linked.AccountID).Error
	return account, err
}

func (s *Service) Authenticate(token string) (store.Account, error) {
	if strings.TrimSpace(token) == "" {
		return store.Account{}, ErrInvalidSession
	}
	var session store.Session
	err := s.db.Where("token_hash = ? AND expires_at > ? AND revoked_at IS NULL", hashToken(token), s.now()).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Account{}, ErrInvalidSession
	}
	if err != nil {
		return store.Account{}, fmt.Errorf("read cloud session: %w", err)
	}
	var account store.Account
	if err := s.db.First(&account, "id = ?", session.AccountID).Error; err != nil {
		return store.Account{}, ErrInvalidSession
	}
	return account, nil
}

func (s *Service) Revoke(token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidSession
	}
	now := s.now()
	result := s.db.Model(&store.Session{}).
		Where("token_hash = ? AND revoked_at IS NULL", hashToken(token)).
		Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("revoke cloud session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrInvalidSession
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
