package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	limit := 50
	if raw := strings.TrimSpace(os.Getenv("NOTIFICATION_OUTBOX_WORKER_LIMIT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			logger.Error("NOTIFICATION_OUTBOX_WORKER_LIMIT must be a positive integer", "value", raw)
			os.Exit(1)
		}
		limit = min(parsed, 200)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	var postgresDB *postgres.DB
	postgresDB, err = postgres.Open(context.Background(), postgres.Config{
		DatabaseURL: cfg.DatabaseURL,
		MaxConns:    cfg.DatabaseMaxConns,
	})
	if err != nil {
		logger.Error("postgres configuration error", "error", err)
		os.Exit(1)
	}
	defer postgresDB.Close()
	fcm, err := httpapi.NewFCMSender(cfg.FCMServiceAccountJSON, httpClient)
	if err != nil {
		logger.Error("fcm configuration error", "error", err)
		os.Exit(1)
	}

	result, err := httpapi.ProcessNotificationOutboxOnce(context.Background(), httpapi.Dependencies{
		Config:   cfg,
		Logger:   logger,
		Postgres: postgresDB,
		FCM:      fcm,
	}, limit)
	if err != nil {
		logger.Error("notification outbox worker failed", "error", err)
		os.Exit(1)
	}
	logger.Info("notification outbox worker completed", "processed", result.ProcessedCount, "failed", result.FailedCount, "skipped", result.SkippedCount)
	fmt.Printf("processed=%d failed=%d skipped=%d\n", result.ProcessedCount, result.FailedCount, result.SkippedCount)
}
