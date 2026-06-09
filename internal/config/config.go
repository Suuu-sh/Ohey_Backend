package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment           string
	Port                  string
	DataStore             string
	DatabaseURL           string
	DatabaseMaxConns      int32
	AuthProvider          string
	ClerkIssuer           string
	ClerkJWKSURL          string
	ClerkAudience         string
	ClerkSecretKey        string
	AllowedOrigins        []string
	FCMServiceAccountJSON string
	AdminEmails           []string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:           getEnv(EnvOheyEnvironment, "production"),
		Port:                  getEnv(EnvPort, "8080"),
		DataStore:             strings.ToLower(getEnv(EnvDataStore, "neon")),
		DatabaseURL:           strings.TrimSpace(os.Getenv(EnvDatabaseURL)),
		DatabaseMaxConns:      int32FromEnv(EnvDatabaseMaxConns, 10),
		AuthProvider:          strings.ToLower(getEnv(EnvAuthProvider, "clerk")),
		ClerkIssuer:           strings.TrimRight(os.Getenv(EnvClerkIssuer), "/"),
		ClerkJWKSURL:          strings.TrimSpace(os.Getenv(EnvClerkJWKSURL)),
		ClerkAudience:         strings.TrimSpace(os.Getenv(EnvClerkAudience)),
		ClerkSecretKey:        strings.TrimSpace(os.Getenv(EnvClerkSecretKey)),
		AllowedOrigins:        splitCSV(getEnv(EnvAllowedOrigins, "*")),
		FCMServiceAccountJSON: strings.TrimSpace(os.Getenv(EnvFCMServiceAccountJSON)),
		AdminEmails:           splitCSV(os.Getenv(EnvOheyAdminEmails)),
	}
	if cfg.DataStore == "" {
		cfg.DataStore = "neon"
	}
	switch cfg.DataStore {
	case "postgres", "neon":
		if cfg.DatabaseURL == "" {
			return cfg, errors.New(EnvDatabaseURL + " is required when " + EnvDataStore + "=" + cfg.DataStore)
		}
	default:
		return cfg, errors.New(EnvDataStore + " must be postgres or neon")
	}
	if cfg.AuthProvider == "" {
		cfg.AuthProvider = "clerk"
	}
	if cfg.AuthProvider != "clerk" {
		return cfg, errors.New(EnvAuthProvider + " must be clerk")
	}
	if cfg.ClerkIssuer == "" {
		return cfg, errors.New(EnvClerkIssuer + " is required when " + EnvAuthProvider + "=clerk")
	}
	if cfg.ClerkJWKSURL == "" {
		cfg.ClerkJWKSURL = cfg.ClerkIssuer + "/.well-known/jwks.json"
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

func int32FromEnv(key string, fallback int32) int32 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return int32(parsed)
}
