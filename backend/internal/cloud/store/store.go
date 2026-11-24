package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Account struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type AccountIdentity struct {
	ID        string    `gorm:"primaryKey;size:36" json:"-"`
	AccountID string    `gorm:"index;size:36;not null" json:"-"`
	Provider  string    `gorm:"uniqueIndex:idx_cloud_identity,priority:1;size:32;not null" json:"provider"`
	Subject   string    `gorm:"uniqueIndex:idx_cloud_identity,priority:2;size:120;not null" json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

type Session struct {
	ID        string     `gorm:"primaryKey;size:36" json:"id"`
	AccountID string     `gorm:"index;size:36;not null" json:"accountId"`
	TokenHash string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"index;not null" json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type OAuthState struct {
	ID          string     `gorm:"primaryKey;size:64" json:"-"`
	HandoffURL  string     `gorm:"size:512" json:"-"`
	ClientState string     `gorm:"size:160" json:"-"`
	ExpiresAt   time.Time  `gorm:"index;not null" json:"-"`
	ConsumedAt  *time.Time `json:"-"`
	CreatedAt   time.Time  `json:"-"`
}

type HandoffCode struct {
	ID          string     `gorm:"primaryKey;size:64" json:"-"`
	AccountID   string     `gorm:"index;size:36;not null" json:"-"`
	CodeHash    string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ClientState string     `gorm:"size:160;not null" json:"-"`
	ExpiresAt   time.Time  `gorm:"index;not null" json:"-"`
	ConsumedAt  *time.Time `json:"-"`
	CreatedAt   time.Time  `json:"-"`
}

type APIToken struct {
	ID         string     `gorm:"primaryKey;size:36" json:"-"`
	AccountID  string     `gorm:"index;size:36;not null" json:"-"`
	Name       string     `gorm:"size:120;not null" json:"name"`
	TokenHash  string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Prefix     string     `gorm:"size:16;not null" json:"prefix"`
	Suffix     string     `gorm:"size:8;not null" json:"suffix"`
	ScopesJSON string     `gorm:"type:text;not null" json:"-"`
	ExpiresAt  time.Time  `gorm:"index;not null" json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	UseCount   int64      `gorm:"not null" json:"useCount"`
	CreatedAt  time.Time  `json:"createdAt"`
}
type EmailVerificationCode struct {
	ID        string     `gorm:"primaryKey;size:36" json:"-"`
	Email     string     `gorm:"index;size:320;not null" json:"-"`
	CodeHash  string     `gorm:"size:64;not null" json:"-"`
	Attempts  int        `gorm:"not null" json:"-"`
	ExpiresAt time.Time  `gorm:"index;not null" json:"-"`
	UsedAt    *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

type AdminAccount struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	Email      string    `gorm:"uniqueIndex;size:320;not null" json:"email"`
	SecretHash string    `gorm:"size:255;not null" json:"-"`
	Role       string    `gorm:"size:32;not null" json:"role"`
	Active     bool      `gorm:"not null" json:"active"`
	CreatedAt  time.Time `json:"createdAt"`
}

type AdminSession struct {
	ID        string     `gorm:"primaryKey;size:36" json:"-"`
	AdminID   string     `gorm:"index;size:36;not null" json:"adminId"`
	TokenHash string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ExpiresAt time.Time  `gorm:"index;not null" json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type AdminBootstrap struct {
	ID         string     `gorm:"primaryKey;size:16" json:"-"`
	SecretHash string     `gorm:"size:64;not null" json:"-"`
	UsedAt     *time.Time `json:"-"`
	CreatedAt  time.Time  `json:"-"`
}

type AuditEvent struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	AdminID      string    `gorm:"index;size:36;not null" json:"adminId"`
	Action       string    `gorm:"size:80;not null" json:"action"`
	ResourceType string    `gorm:"size:40;not null" json:"resourceType"`
	ResourceID   string    `gorm:"size:120" json:"resourceId,omitempty"`
	Outcome      string    `gorm:"size:24;not null" json:"outcome"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Product struct {
	ID        string `gorm:"primaryKey;size:64" json:"id"`
	Name      string `gorm:"size:120;not null" json:"name"`
	Currency  string `gorm:"size:3;not null" json:"currency"`
	AmountCNY int64  `gorm:"not null" json:"amountCny"`
	Points    int64  `gorm:"not null" json:"points"`
	Enabled   bool   `gorm:"not null" json:"enabled"`
}

type Order struct {
	ID          string     `gorm:"primaryKey;size:36" json:"id"`
	AccountID   string     `gorm:"index;size:36;not null" json:"accountId"`
	ProductID   string     `gorm:"size:64;not null" json:"productId"`
	ProductName string     `gorm:"size:120;not null" json:"productName"`
	Currency    string     `gorm:"size:3;not null" json:"currency"`
	AmountCNY   int64      `gorm:"not null" json:"amountCny"`
	Points      int64      `gorm:"not null" json:"points"`
	Status      string     `gorm:"index;size:24;not null" json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
}

type PaymentEvent struct {
	ID                string    `gorm:"primaryKey;size:36" json:"id"`
	OrderID           string    `gorm:"uniqueIndex;size:36;not null" json:"orderId"`
	AccountID         string    `gorm:"uniqueIndex:idx_cloud_payment_idempotency,priority:1;size:36;not null" json:"accountId"`
	IdempotencyKey    string    `gorm:"uniqueIndex:idx_cloud_payment_idempotency,priority:2;size:160;not null" json:"-"`
	Provider          string    `gorm:"size:24;not null" json:"provider"`
	ProviderEventID   string    `gorm:"uniqueIndex:idx_cloud_payment_provider_event,priority:1;size:160;not null" json:"-"`
	ProviderAccountID string    `gorm:"uniqueIndex:idx_cloud_payment_provider_event,priority:2;size:160;not null" json:"-"`
	Currency          string    `gorm:"size:3;not null" json:"currency"`
	AmountCNY         int64     `gorm:"not null" json:"amountCny"`
	CreatedAt         time.Time `json:"createdAt"`
}

type LedgerEntry struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	AccountID  string    `gorm:"index;size:36;not null" json:"accountId"`
	OrderID    string    `gorm:"index;size:36;not null" json:"orderId,omitempty"`
	SourceType string    `gorm:"uniqueIndex:idx_cloud_ledger_source,priority:1;size:32;not null" json:"sourceType"`
	SourceID   string    `gorm:"uniqueIndex:idx_cloud_ledger_source,priority:2;size:120;not null" json:"sourceId"`
	Kind       string    `gorm:"size:24;not null" json:"kind"`
	Points     int64     `gorm:"not null" json:"points"`
	CreatedAt  time.Time `json:"createdAt"`
}

type HostedModel struct {
	ID                     string    `gorm:"primaryKey;size:80" json:"id"`
	UpstreamID             string    `gorm:"size:160;not null" json:"-"`
	PriceVersion           string    `gorm:"size:80;not null" json:"priceVersion"`
	MaxInputTokens         int64     `gorm:"not null" json:"maxInputTokens"`
	MaxOutputTokens        int64     `gorm:"not null" json:"maxOutputTokens"`
	InputPointsPerMillion  int64     `gorm:"not null" json:"inputPointsPerMillion"`
	OutputPointsPerMillion int64     `gorm:"not null" json:"outputPointsPerMillion"`
	Enabled                bool      `gorm:"not null" json:"enabled"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type CloudTask struct {
	ID                     string    `gorm:"primaryKey;size:36" json:"id"`
	AccountID              string    `gorm:"uniqueIndex:idx_cloud_task_idempotency,priority:1;size:36;not null" json:"accountId"`
	IdempotencyKey         string    `gorm:"uniqueIndex:idx_cloud_task_idempotency,priority:2;size:160;not null" json:"-"`
	ModelID                string    `gorm:"size:80;not null" json:"modelId"`
	UpstreamID             string    `gorm:"size:160;not null" json:"-"`
	PriceVersion           string    `gorm:"size:80;not null" json:"priceVersion"`
	MaxInputTokens         int64     `gorm:"not null" json:"-"`
	MaxOutputTokens        int64     `gorm:"not null" json:"-"`
	InputPointsPerMillion  int64     `gorm:"not null" json:"-"`
	OutputPointsPerMillion int64     `gorm:"not null" json:"-"`
	RequestJSON            string    `gorm:"type:text;not null" json:"-"`
	ResponseText           string    `gorm:"type:text" json:"responseText,omitempty"`
	Status                 string    `gorm:"index;size:32;not null" json:"status"`
	ReservedPoints         int64     `gorm:"not null" json:"reservedPoints"`
	InputTokens            int64     `gorm:"not null" json:"inputTokens"`
	OutputTokens           int64     `gorm:"not null" json:"outputTokens"`
	ChargedPoints          int64     `gorm:"not null" json:"chargedPoints"`
	ErrorCode              string    `gorm:"size:64" json:"errorCode,omitempty"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type HostedUsage struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	TaskID       string    `gorm:"uniqueIndex;size:36;not null" json:"taskId"`
	InputTokens  int64     `gorm:"not null" json:"inputTokens"`
	OutputTokens int64     `gorm:"not null" json:"outputTokens"`
	TotalTokens  int64     `gorm:"not null" json:"totalTokens"`
	CreatedAt    time.Time `json:"createdAt"`
}

type PointReservation struct {
	ID                     string    `gorm:"primaryKey;size:36" json:"id"`
	TaskID                 string    `gorm:"uniqueIndex;size:36;not null" json:"taskId"`
	AccountID              string    `gorm:"index;size:36;not null" json:"accountId"`
	Points                 int64     `gorm:"not null" json:"points"`
	MaxInputTokens         int64     `gorm:"not null" json:"maxInputTokens"`
	MaxOutputTokens        int64     `gorm:"not null" json:"maxOutputTokens"`
	InputPointsPerMillion  int64     `gorm:"not null" json:"inputPointsPerMillion"`
	OutputPointsPerMillion int64     `gorm:"not null" json:"outputPointsPerMillion"`
	Status                 string    `gorm:"index;size:32;not null" json:"status"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type SchemaMigration struct {
	Version   int       `gorm:"primaryKey"`
	AppliedAt time.Time `gorm:"not null"`
}

type Store struct{ db *gorm.DB }

func Open(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" || filepath.Clean(dataDir) == "." {
		return nil, fmt.Errorf("cloud data directory must be isolated")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create cloud data directory: %w", err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(dataDir, "cloud-sandbox.db")+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open cloud sandbox database: %w", err)
	}
	if err := Migrate(db); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("migrate cloud sandbox database: %w", err)
	}
	return &Store{db: db}, nil
}

func OpenProduction(databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("cloud PostgreSQL URL is required")
	}
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open cloud PostgreSQL database: %w", err)
	}
	if err := Migrate(db); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("migrate cloud PostgreSQL database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

const (
	schemaVersionBase         = 1
	schemaVersionEmailAdmin   = 2
	schemaVersionTaskSnapshot = 3
	schemaVersionOAuthState   = 4
	schemaVersionHandoff      = 5
	schemaVersionAPITokens    = 6
	schemaVersionPaymentEvent = 7
)

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&SchemaMigration{}); err != nil {
		return err
	}
	applied, err := appliedSchemaVersions(db)
	if err != nil {
		return err
	}
	// Released versions are immutable: new tables always arrive as a new version,
	// so existing sandbox databases keep upgrading instead of silently missing them.
	if !applied[schemaVersionBase] {
		if err := applyMigration(db, schemaVersionBase, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Account{}, &AccountIdentity{}, &Session{}, &Product{}, &Order{}, &PaymentEvent{}, &LedgerEntry{}, &HostedModel{}, &CloudTask{}, &HostedUsage{}, &PointReservation{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionEmailAdmin] {
		if err := applyMigration(db, schemaVersionEmailAdmin, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&EmailVerificationCode{}, &AdminAccount{}, &AdminSession{}, &AdminBootstrap{}, &AuditEvent{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionTaskSnapshot] {
		if err := applyMigration(db, schemaVersionTaskSnapshot, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&CloudTask{}, &AdminAccount{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionOAuthState] {
		if err := applyMigration(db, schemaVersionOAuthState, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&OAuthState{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionHandoff] {
		if err := applyMigration(db, schemaVersionHandoff, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&OAuthState{}, &HandoffCode{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionAPITokens] {
		if err := applyMigration(db, schemaVersionAPITokens, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&APIToken{})
		}); err != nil {
			return err
		}
	}
	if !applied[schemaVersionPaymentEvent] {
		if err := applyMigration(db, schemaVersionPaymentEvent, func(tx *gorm.DB) error {
			return tx.AutoMigrate(&PaymentEvent{})
		}); err != nil {
			return err
		}
	}
	return installConstraints(db)
}

func applyMigration(db *gorm.DB, version int, migrate func(*gorm.DB) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := migrate(tx); err != nil {
			return err
		}
		return tx.Create(&SchemaMigration{Version: version, AppliedAt: time.Now().UTC()}).Error
	})
}

func appliedSchemaVersions(db *gorm.DB) (map[int]bool, error) {
	var versions []int
	if err := db.Model(&SchemaMigration{}).Pluck("version", &versions).Error; err != nil {
		return nil, fmt.Errorf("read cloud schema versions: %w", err)
	}
	applied := make(map[int]bool, len(versions))
	for _, version := range versions {
		applied[version] = true
	}
	return applied, nil
}

func installConstraints(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	for _, statement := range []string{
		`DROP TRIGGER IF EXISTS cloud_ledger_points_positive`,
		`DROP TRIGGER IF EXISTS cloud_ledger_order_exists`,
		`CREATE TRIGGER IF NOT EXISTS cloud_identity_account_exists BEFORE INSERT ON account_identities WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'identity account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_session_account_exists BEFORE INSERT ON sessions WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'session account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_order_account_exists BEFORE INSERT ON orders WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'order account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_order_points_positive BEFORE INSERT ON orders WHEN NEW.points <= 0 BEGIN SELECT RAISE(ABORT, 'order points must be positive'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_order_status_valid BEFORE INSERT ON orders WHEN NEW.status NOT IN ('pending', 'paid') BEGIN SELECT RAISE(ABORT, 'invalid order status'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_payment_order_account_exists BEFORE INSERT ON payment_events WHEN NOT EXISTS (SELECT 1 FROM orders WHERE id = NEW.order_id AND account_id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'payment order does not belong to account'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_ledger_nonzero BEFORE INSERT ON ledger_entries WHEN NEW.points = 0 BEGIN SELECT RAISE(ABORT, 'ledger points cannot be zero'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_ledger_account_exists BEFORE INSERT ON ledger_entries WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'ledger account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_admin_session_admin_exists BEFORE INSERT ON admin_sessions WHEN NOT EXISTS (SELECT 1 FROM admin_accounts WHERE id = NEW.admin_id AND active = 1) BEGIN SELECT RAISE(ABORT, 'admin session requires active admin'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_audit_admin_exists BEFORE INSERT ON audit_events WHEN NOT EXISTS (SELECT 1 FROM admin_accounts WHERE id = NEW.admin_id) BEGIN SELECT RAISE(ABORT, 'audit admin does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_audit_no_update BEFORE UPDATE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit events are append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_audit_no_delete BEFORE DELETE ON audit_events BEGIN SELECT RAISE(ABORT, 'audit events are append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_ledger_order_exists BEFORE INSERT ON ledger_entries WHEN NEW.order_id <> '' AND NOT EXISTS (SELECT 1 FROM orders WHERE id = NEW.order_id AND account_id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'ledger order does not belong to account'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_task_account_exists BEFORE INSERT ON cloud_tasks WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'task account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_reservation_account_exists BEFORE INSERT ON point_reservations WHEN NOT EXISTS (SELECT 1 FROM accounts WHERE id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'reservation account does not exist'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_reservation_task_exists BEFORE INSERT ON point_reservations WHEN NOT EXISTS (SELECT 1 FROM cloud_tasks WHERE id = NEW.task_id AND account_id = NEW.account_id) BEGIN SELECT RAISE(ABORT, 'reservation task does not belong to account'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_order_points_positive_update BEFORE UPDATE OF points ON orders WHEN NEW.points <= 0 BEGIN SELECT RAISE(ABORT, 'order points must be positive'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_order_status_valid_update BEFORE UPDATE OF status ON orders WHEN NEW.status NOT IN ('pending', 'paid') BEGIN SELECT RAISE(ABORT, 'invalid order status'); END`,

		`CREATE TRIGGER IF NOT EXISTS cloud_ledger_no_update BEFORE UPDATE ON ledger_entries BEGIN SELECT RAISE(ABORT, 'ledger entries are append-only'); END`,
		`CREATE TRIGGER IF NOT EXISTS cloud_ledger_no_delete BEFORE DELETE ON ledger_entries BEGIN SELECT RAISE(ABORT, 'ledger entries are append-only'); END`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("install cloud data constraint: %w", err)
		}
	}
	return nil
}

func IsSQLiteConflict(err error) bool {
	return strings.Contains(strings.ToLower(fmt.Sprint(err)), "unique constraint failed")
}

func IsSQLiteRetryable(err error) bool {
	message := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is busy")
}
