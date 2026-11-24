package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	cloudapp "luckytv/backend/internal/cloud/app"
	"luckytv/backend/internal/cloud/config"
	"luckytv/backend/internal/cloud/emailconfig"
	httpapi "luckytv/backend/internal/cloud/http"
	"luckytv/backend/internal/cloud/identity"
	"luckytv/backend/internal/cloud/modelgateway"
	"luckytv/backend/internal/cloud/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var storage *store.Store
	if cfg.Environment == "production" {
		storage, err = store.OpenProduction(cfg.DatabaseURL)
	} else {
		storage, err = store.Open(cfg.DataDir)
	}
	if err != nil {
		return err
	}
	defer func() {
		if err := storage.Close(); err != nil {
			log.Printf("close cloud database: %v", err)
		}
	}()
	mailer, otpSecret, err := emailconfig.Build(cfg)
	if err != nil {
		return fmt.Errorf("configure email verification: %w", err)
	}
	var gateway *modelgateway.Client
	if cfg.HostedModelsEnabled {
		gateway, err = modelgateway.New(cfg.OneAPIBaseURL, cfg.OneAPIAPIKey, nil)
		if err != nil {
			return fmt.Errorf("configure OneAPI gateway: %w", err)
		}
	}
	models := make([]store.HostedModel, 0, len(cfg.HostedModels))
	for _, model := range cfg.HostedModels {
		models = append(models, store.HostedModel{
			ID: model.ID, UpstreamID: model.UpstreamID, PriceVersion: model.PriceVersion,
			MaxInputTokens: model.MaxInputTokens, MaxOutputTokens: model.MaxOutputTokens,
			InputPointsPerMillion: model.InputPointsPerMillion, OutputPointsPerMillion: model.OutputPointsPerMillion,
			Enabled: true,
		})
	}
	var wechat *identity.WeChatProvider
	if cfg.WeChatAppID != "" && cfg.WeChatAppSecret != "" {
		wechat = &identity.WeChatProvider{AppID: cfg.WeChatAppID, AppSecret: cfg.WeChatAppSecret, RedirectURI: cfg.WeChatCallbackURL}
	}
	services, err := cloudapp.New(cloudapp.Options{
		DB: storage.DB(), SessionTTL: 24 * time.Hour, Sandbox: cfg.SandboxEnabled, Provider: wechat,
		Mailer: mailer, OTPSecret: otpSecret, Gateway: gateway, Models: models,
		AdminBootstrapSecret: cfg.AdminBootstrapSecret, AdminBootstrapEmail: cfg.AdminBootstrapEmail,
		InitialAdminSecret: cfg.AdminInitialSecret,
	})
	if err != nil {
		return err
	}
	services.Start(ctx)
	defer services.Close()
	var emailOTP *identity.EmailOTPService
	if cfg.Environment != "production" {
		emailOTP = services.EmailOTP
	}
	handler := httpapi.NewRouter(httpapi.Options{
		SandboxEnabled: cfg.SandboxEnabled, Identity: services.Identity, Admin: services.Admin,
		EmailOTP: emailOTP, WeChat: wechat, WeChatCallbackURL: cfg.WeChatCallbackURL,
		Catalog: services.Catalog, HostedModels: services.HostedModels,
		Ledger: services.Ledger, Payment: services.Payment, Tasks: services.Tasks,
	})
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen for cloud sandbox: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	log.Printf("cloud sandbox listening on %s (environment=%s, sandbox=%t)", cfg.ListenAddr, cfg.Environment, cfg.SandboxEnabled)
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("cloud HTTP server: %w", err)
		}
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown cloud HTTP server: %w", err)
	}
	if err := <-serveErr; err != nil {
		return fmt.Errorf("cloud HTTP server: %w", err)
	}
	log.Printf("cloud sandbox stopped")
	return nil
}
