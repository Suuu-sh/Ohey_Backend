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
}

func Load() (Config, error) {
	cfg := Config{
		Environment:            getEnv("NOMO_ENV", "production"),
		Port:                   getEnv("PORT", "8080"),
		SupabaseURL:            strings.TrimRight(os.Getenv("SUPABASE_URL"), "/"),
		SupabaseAnonKey:        os.Getenv("SUPABASE_ANON_KEY"),
		SupabaseServiceRoleKey: strings.TrimSpace(os.Getenv("SUPABASE_SERVICE_ROLE_KEY")),
		AllowedOrigins:         splitCSV(getEnv("ALLOWED_ORIGINS", "*")),
	}
	if cfg.SupabaseURL == "" {
		return cfg, errors.New("SUPABASE_URL is required")
	}
	if cfg.SupabaseAnonKey == "" {
		return cfg, errors.New("SUPABASE_ANON_KEY is required")
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
