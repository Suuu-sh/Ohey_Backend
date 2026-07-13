package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/config"
	"github.com/Suuu-sh/Ohey_Backend/internal/httpapi"
	"github.com/Suuu-sh/Ohey_Backend/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("ohey backend stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	var postgresDB *postgres.DB
	if cfg.DataStore == "postgres" || cfg.DataStore == "neon" {
		postgresDB, err = postgres.Open(context.Background(), postgres.Config{
			DatabaseURL: cfg.DatabaseURL,
			MaxConns:    cfg.DatabaseMaxConns,
		})
		if err != nil {
			return fmt.Errorf("postgres configuration error: %w", err)
		}
		defer postgresDB.Close()
	}
	clerkAPI := httpapi.NewClerkAPIClientForDependencies(cfg.ClerkSecretKey, httpClient)
	fcm, err := httpapi.NewFCMSender(cfg.FCMServiceAccountJSON, httpClient)
	if err != nil {
		return fmt.Errorf("fcm configuration error: %w", err)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(httpapi.Dependencies{Config: cfg, Logger: logger, Postgres: postgresDB, FCM: fcm, ClerkAPI: clerkAPI}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("starting ohey backend", "port", cfg.Port, "env", cfg.Environment, "data_store", cfg.DataStore)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve http: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down ohey backend")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve http during shutdown: %w", err)
		}
		return nil
	}
}
