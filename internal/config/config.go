package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Environment            string
	Port                   string
	SupabaseURL            string
	SupabaseAnonKey        string
	SupabaseServiceRoleKey string
	AllowedOrigins         []string
	FCMServiceAccountJSON  string
	AdminEmails            []string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:            getEnv(EnvOheyEnvironment, "production"),
		Port:                   getEnv(EnvPort, "8080"),
		SupabaseURL:            strings.TrimRight(os.Getenv(EnvSupabaseURL), "/"),
		SupabaseAnonKey:        os.Getenv(EnvSupabaseAnonKey),
		SupabaseServiceRoleKey: strings.TrimSpace(os.Getenv(EnvSupabaseServiceRoleKey)),
		AllowedOrigins:         splitCSV(getEnv(EnvAllowedOrigins, "*")),
		FCMServiceAccountJSON:  strings.TrimSpace(os.Getenv(EnvFCMServiceAccountJSON)),
		AdminEmails:            splitCSV(os.Getenv(EnvOheyAdminEmails)),
	}
	if cfg.SupabaseURL == "" {
		return cfg, errors.New(EnvSupabaseURL + " is required")
	}
	if cfg.SupabaseAnonKey == "" {
		return cfg, errors.New(EnvSupabaseAnonKey + " is required")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
