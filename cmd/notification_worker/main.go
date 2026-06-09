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
	"github.com/yota/ohey/backend/internal/supabase"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}
	usesPostgresStore := cfg.DataStore == "postgres" || cfg.DataStore == "neon"
	if !usesPostgresStore && strings.TrimSpace(cfg.SupabaseServiceRoleKey) == "" {
		logger.Error("SUPABASE_SERVICE_ROLE_KEY is required for notification worker")
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
	var supabaseClient *supabase.Client
	var adminSupabaseClient *supabase.Client
	if !usesPostgresStore {
		supabaseClient = supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseAnonKey, httpClient)
		adminSupabaseClient = supabase.NewClient(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, httpClient)
	}
	var postgresDB *postgres.DB
	if usesPostgresStore {
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
	fcm, err := httpapi.NewFCMSender(cfg.FCMServiceAccountJSON, httpClient)
	if err != nil {
		logger.Error("fcm configuration error", "error", err)
		os.Exit(1)
	}

	result, err := httpapi.ProcessNotificationOutboxOnce(context.Background(), httpapi.Dependencies{
		Config:        cfg,
		Logger:        logger,
		Supabase:      supabaseClient,
		AdminSupabase: adminSupabaseClient,
		Postgres:      postgresDB,
		FCM:           fcm,
	}, limit)
	if err != nil {
		logger.Error("notification outbox worker failed", "error", err)
		os.Exit(1)
	}
	logger.Info("notification outbox worker completed", "processed", result.ProcessedCount, "failed", result.FailedCount, "skipped", result.SkippedCount)
	fmt.Printf("processed=%d failed=%d skipped=%d\n", result.ProcessedCount, result.FailedCount, result.SkippedCount)
}
