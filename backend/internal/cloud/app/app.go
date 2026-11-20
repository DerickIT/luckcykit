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

func New(db *gorm.DB, sessionTTL time.Duration, sandbox bool, mailer identity.VerificationMailer, otpSecret []byte, gateway *modelgateway.Client, models []store.HostedModel, bootstrapSecret, bootstrapEmail, initialAdminSecret string) (*App, error) {
	products := catalog.New(db)
	points := ledger.New(db)
	var provider identity.Provider
	if sandbox {
		provider = identity.SandboxProvider{}
	}
	identities := identity.New(db, sessionTTL, provider)
	administrators := admin.New(db)
	if bootstrapSecret != "" {
		if err := administrators.SeedBootstrap(bootstrapSecret); err != nil {
			return nil, fmt.Errorf("seed admin bootstrap: %w", err)
		}
		if _, err := administrators.Bootstrap(bootstrapSecret, bootstrapEmail, initialAdminSecret); err != nil && err != admin.ErrBootstrapUsed {
			return nil, fmt.Errorf("consume initial admin bootstrap: %w", err)
		}
	}
	orders := payment.New(db, products, points)
	if sandbox {
		if err := products.EnsureSandboxProducts(); err != nil {
			return nil, fmt.Errorf("initialize sandbox catalog: %w", err)
		}
	}
	var emailOTP *identity.EmailOTPService
	if mailer != nil && len(otpSecret) >= 32 {
		emailOTP = identity.NewEmailOTPService(db, identities, mailer, otpSecret)
	}
	var tasks *task.Service
	if gateway != nil && len(models) > 0 {
		tasks = task.New(db, points, gateway, models)
	}
	return &App{Identity: identities, Admin: administrators, Catalog: products, Ledger: points, Payment: orders, EmailOTP: emailOTP, Tasks: tasks}, nil
}
