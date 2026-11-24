package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/store"
)

const (
	APITokenPrefix       = "lkt_"
	APITokenMaxActive    = 10
	ScopeAccountRead     = "account.read"
	ScopeHostedInvoke    = "hosted.invoke"
	ScopeHostedTasksRead = "hosted.tasks.read"
)

var (
	ErrInvalidAPIToken       = errors.New("invalid or expired API token")
	ErrAPITokenLimit         = errors.New("API token limit reached")
	ErrInvalidAPITokenName   = errors.New("invalid API token name")
	ErrInvalidAPITokenExpiry = errors.New("invalid API token expiry")
	ErrInvalidAPITokenScope  = errors.New("invalid API token scope")
)

type APITokenInfo struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Suffix     string     `json:"suffix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	UseCount   int64      `json:"useCount"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type APITokenCreateResult struct {
	Token string       `json:"token"`
	Info  APITokenInfo `json:"tokenInfo"`
}

type APITokenAuth struct {
	AccountID string
	TokenID   string
	Scopes    map[string]bool
}

var allowedAPITokenScopes = map[string]bool{
	ScopeAccountRead:     true,
	ScopeHostedInvoke:    true,
	ScopeHostedTasksRead: true,
}

func (s *Service) CreateAPIToken(accountID, name string, expiryDays int, scopes []string) (APITokenCreateResult, error) {
	name = strings.TrimSpace(name)
	if accountID == "" || name == "" || len(name) > 120 {
		return APITokenCreateResult{}, ErrInvalidAPITokenName
	}
	if expiryDays != 7 && expiryDays != 30 && expiryDays != 90 && expiryDays != 365 {
		return APITokenCreateResult{}, ErrInvalidAPITokenExpiry
	}
	normalizedScopes, err := normalizeAPITokenScopes(scopes)
	if err != nil {
		return APITokenCreateResult{}, err
	}
	encodedScopes, err := json.Marshal(normalizedScopes)
	if err != nil {
		return APITokenCreateResult{}, fmt.Errorf("encode API token scopes: %w", err)
	}
	var active int64
	if err := s.db.Model(&store.APIToken{}).Where("account_id = ? AND revoked_at IS NULL AND expires_at > ?", accountID, s.now()).Count(&active).Error; err != nil {
		return APITokenCreateResult{}, fmt.Errorf("count active API tokens: %w", err)
	}
	if active >= APITokenMaxActive {
		return APITokenCreateResult{}, ErrAPITokenLimit
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return APITokenCreateResult{}, fmt.Errorf("generate API token: %w", err)
	}
	token := APITokenPrefix + hex.EncodeToString(raw)
	now := s.now()
	record := store.APIToken{ID: uuid.NewString(), AccountID: accountID, Name: name, TokenHash: hashToken(token), Prefix: token[:len(APITokenPrefix)+8], Suffix: token[len(token)-4:], ScopesJSON: string(encodedScopes), ExpiresAt: now.AddDate(0, 0, expiryDays), CreatedAt: now}
	if err := s.db.Create(&record).Error; err != nil {
		return APITokenCreateResult{}, fmt.Errorf("store API token: %w", err)
	}
	return APITokenCreateResult{Token: token, Info: apiTokenInfo(record, normalizedScopes)}, nil
}

func (s *Service) ListAPITokens(accountID string) ([]APITokenInfo, error) {
	records := make([]store.APIToken, 0)
	if err := s.db.Where("account_id = ?", accountID).Order("created_at DESC").Order("id DESC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list API tokens: %w", err)
	}
	result := make([]APITokenInfo, 0, len(records))
	for _, record := range records {
		scopes, err := decodeAPITokenScopes(record.ScopesJSON)
		if err != nil {
			return nil, fmt.Errorf("decode API token scopes: %w", err)
		}
		result = append(result, apiTokenInfo(record, scopes))
	}
	return result, nil
}

func (s *Service) RevokeAPIToken(accountID, tokenID string) error {
	now := s.now()
	result := s.db.Model(&store.APIToken{}).Where("id = ? AND account_id = ? AND revoked_at IS NULL", strings.TrimSpace(tokenID), accountID).Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("revoke API token: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrInvalidAPIToken
	}
	return nil
}

func (s *Service) RevokeAllAPITokens(accountID string) error {
	if err := s.db.Model(&store.APIToken{}).Where("account_id = ? AND revoked_at IS NULL", accountID).Update("revoked_at", s.now()).Error; err != nil {
		return fmt.Errorf("revoke all API tokens: %w", err)
	}
	return nil
}

func (s *Service) AuthenticateAPIToken(token string, requiredScope string) (APITokenAuth, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, APITokenPrefix) {
		return APITokenAuth{}, ErrInvalidAPIToken
	}
	var record store.APIToken
	now := s.now()
	if err := s.db.Where("token_hash = ? AND expires_at > ? AND revoked_at IS NULL", hashToken(token), now).First(&record).Error; err != nil {
		return APITokenAuth{}, ErrInvalidAPIToken
	}
	scopes, err := decodeAPITokenScopes(record.ScopesJSON)
	if err != nil {
		return APITokenAuth{}, ErrInvalidAPITokenScope
	}
	scopeSet := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		scopeSet[scope] = true
	}
	if !scopeSet[requiredScope] {
		return APITokenAuth{}, ErrInvalidAPITokenScope
	}
	usedAt := now
	if err := s.db.Model(&store.APIToken{}).Where("id = ?", record.ID).Updates(map[string]any{"last_used_at": usedAt, "use_count": gorm.Expr("use_count + 1")}).Error; err != nil {
		return APITokenAuth{}, fmt.Errorf("record API token use: %w", err)
	}
	return APITokenAuth{AccountID: record.AccountID, TokenID: record.ID, Scopes: scopeSet}, nil
}

func normalizeAPITokenScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		scopes = []string{ScopeAccountRead, ScopeHostedInvoke, ScopeHostedTasksRead}
	}
	seen := make(map[string]bool, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if !allowedAPITokenScopes[scope] {
			return nil, ErrInvalidAPITokenScope
		}
		if seen[scope] {
			continue
		}
		seen[scope] = true
		normalized = append(normalized, scope)
	}
	if len(normalized) == 0 {
		return nil, ErrInvalidAPITokenScope
	}
	return normalized, nil
}

func decodeAPITokenScopes(raw string) ([]string, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil, err
	}
	return normalizeAPITokenScopes(scopes)
}

func apiTokenInfo(record store.APIToken, scopes []string) APITokenInfo {
	return APITokenInfo{ID: record.ID, Name: record.Name, Prefix: record.Prefix, Suffix: record.Suffix, Scopes: scopes, ExpiresAt: record.ExpiresAt, RevokedAt: record.RevokedAt, LastUsedAt: record.LastUsedAt, UseCount: record.UseCount, CreatedAt: record.CreatedAt}
}
