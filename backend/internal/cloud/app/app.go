package app

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/admin"
	"luckytv/backend/internal/cloud/catalog"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/ledger"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/payment"
	"luckytv/backend/internal/cloud/store"
	"luckytv/backend/internal/cloud/task"
)

type App struct {
	Identity *identity.Service
	Admin    *admin.Service
	Catalog  *catalog.Service
	Ledger   *ledger.Service
	Payment  *payment.Service
	EmailOTP *identity.EmailOTPService
	Tasks    *task.Service
}

type Options struct {
	DB                   *gorm.DB
	SessionTTL           time.Duration
	Sandbox              bool
	Mailer               identity.VerificationMailer
	OTPSecret            []byte
	Gateway              *modelgateway.Client
	Models               []store.HostedModel
	AdminBootstrapSecret string
	AdminBootstrapEmail  string
	InitialAdminSecret   string
}

func New(options Options) (*App, error) {
	db := options.DB
	products := catalog.New(db)
	points := ledger.New(db)
	var provider identity.Provider
	if options.Sandbox {
		provider = identity.SandboxProvider{}
	}
	identities := identity.New(db, options.SessionTTL, provider)
	administrators := admin.New(db)
	if options.AdminBootstrapSecret != "" {
		if err := administrators.SeedBootstrap(options.AdminBootstrapSecret); err != nil {
			return nil, fmt.Errorf("seed admin bootstrap: %w", err)
		}
		if _, err := administrators.Bootstrap(options.AdminBootstrapSecret, options.AdminBootstrapEmail, options.InitialAdminSecret); err != nil && err != admin.ErrBootstrapUsed {
			return nil, fmt.Errorf("consume initial admin bootstrap: %w", err)
		}
	}
	orders := payment.New(db, products, points)
	if options.Sandbox {
		if err := products.EnsureSandboxProducts(); err != nil {
			return nil, fmt.Errorf("initialize sandbox catalog: %w", err)
		}
	}
	var emailOTP *identity.EmailOTPService
	if options.Mailer != nil && len(options.OTPSecret) >= 32 {
		emailOTP = identity.NewEmailOTPService(db, identities, options.Mailer, options.OTPSecret)
	}
	var tasks *task.Service
	if options.Gateway != nil && len(options.Models) > 0 {
		tasks = task.New(db, points, options.Gateway, options.Models)
	}
	return &App{Identity: identities, Admin: administrators, Catalog: products, Ledger: points, Payment: orders, EmailOTP: emailOTP, Tasks: tasks}, nil
}
