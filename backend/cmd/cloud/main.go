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
	storage, err := store.Open(cfg.DataDir)
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
	services, err := cloudapp.New(storage.DB(), 24*time.Hour, cfg.SandboxEnabled, mailer, otpSecret, gateway, models, cfg.AdminBootstrapSecret, cfg.AdminBootstrapEmail, cfg.AdminInitialSecret)
	if err != nil {
		return err
	}
	handler := httpapi.NewRouter(cfg, services.Identity, services.Admin, services.EmailOTP, services.Catalog, services.Ledger, services.Payment, services.Tasks)
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
