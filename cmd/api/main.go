package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/yota/ohey/backend/internal/config"
	"github.com/yota/ohey/backend/internal/httpapi"
	"github.com/yota/ohey/backend/internal/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	var postgresDB *postgres.DB
	if cfg.DataStore == "postgres" || cfg.DataStore == "neon" {
		postgresDB, err = postgres.Open(context.Background(), postgres.Config{
			DatabaseURL: cfg.DatabaseURL,
			MaxConns:    cfg.DatabaseMaxConns,
		})
		if err != nil {
			logger.Error("postgres configuration error", "error", err)
			os.Exit(1)
		}
		defer postgresDB.Close()
	}
	clerkAPI := httpapi.NewClerkAPIClientForDependencies(cfg.ClerkSecretKey, httpClient)
	fcm, err := httpapi.NewFCMSender(cfg.FCMServiceAccountJSON, httpClient)
	if err != nil {
		logger.Error("fcm configuration error", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(httpapi.Dependencies{Config: cfg, Logger: logger, Postgres: postgresDB, FCM: fcm, ClerkAPI: clerkAPI}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	logger.Info("starting ohey backend", "port", cfg.Port, "env", cfg.Environment, "data_store", cfg.DataStore)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
