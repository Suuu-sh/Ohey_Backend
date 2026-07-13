package config

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const minimumOriginVerifySecretBytes = 32

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
	ResendAPIKey          string
	ResendFromEmail       string
	ResendReplyToEmail    string
	ClerkWebhookSecret    string
	R2AccountID           string
	R2AccessKeyID         string
	R2SecretAccessKey     string
	R2Bucket              string
	R2PublicURL           string
	FCMServiceAccountJSON string
	AdminEmails           []string
	OriginVerifySecret    string
}

func Load() (Config, error) {
	cfg := Config{
		Environment:           strings.ToLower(getEnv(EnvAppEnvironment, "production")),
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
		ResendAPIKey:          strings.TrimSpace(os.Getenv(EnvResendAPIKey)),
		ResendFromEmail:       strings.TrimSpace(os.Getenv(EnvResendFromEmail)),
		ResendReplyToEmail:    strings.TrimSpace(os.Getenv(EnvResendReplyToEmail)),
		ClerkWebhookSecret:    strings.TrimSpace(os.Getenv(EnvClerkWebhookSecret)),
		R2AccountID:           strings.TrimSpace(os.Getenv(EnvR2AccountID)),
		R2AccessKeyID:         strings.TrimSpace(os.Getenv(EnvR2AccessKeyID)),
		R2SecretAccessKey:     strings.TrimSpace(os.Getenv(EnvR2SecretAccessKey)),
		R2Bucket:              getEnv(EnvR2Bucket, "ohey-public"),
		R2PublicURL:           strings.TrimRight(strings.TrimSpace(os.Getenv(EnvR2PublicURL)), "/"),
		FCMServiceAccountJSON: strings.TrimSpace(os.Getenv(EnvFCMServiceAccountJSON)),
		AdminEmails:           splitCSV(os.Getenv(EnvOheyAdminEmails)),
		OriginVerifySecret:    strings.TrimSpace(os.Getenv(EnvOriginVerifySecret)),
	}
	if err := validateEnvironment(cfg.Environment); err != nil {
		return cfg, err
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
	if err := validateHTTPSURL(EnvClerkIssuer, cfg.ClerkIssuer, false); err != nil {
		return cfg, err
	}
	if cfg.ClerkJWKSURL == "" {
		cfg.ClerkJWKSURL = cfg.ClerkIssuer + "/.well-known/jwks.json"
	}
	if parsed, err := url.Parse(cfg.ClerkJWKSURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return cfg, errors.New(EnvClerkJWKSURL + " must be an https URL")
	}
	if isProduction(cfg.Environment) {
		if err := validateProduction(cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

func validateEnvironment(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "dev", "development", "production":
		return nil
	default:
		return errors.New(EnvAppEnvironment + " must be local, dev, development, or production")
	}
}

func validateProduction(cfg Config) error {
	if len(cfg.AllowedOrigins) == 0 {
		return errors.New(EnvAllowedOrigins + " is required in production")
	}
	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			return errors.New(EnvAllowedOrigins + " cannot include * in production")
		}
		if err := validateHTTPSURL(EnvAllowedOrigins, origin, true); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.ClerkAudience) == "" {
		return errors.New(EnvClerkAudience + " is required in production")
	}
	if !strings.HasPrefix(cfg.ClerkSecretKey, "sk_live_") {
		return errors.New(EnvClerkSecretKey + " must be a Clerk production secret key in production")
	}
	if len(cfg.OriginVerifySecret) < minimumOriginVerifySecretBytes {
		return fmt.Errorf("%s must contain at least %d bytes in production", EnvOriginVerifySecret, minimumOriginVerifySecretBytes)
	}
	if len(cfg.AdminEmails) == 0 {
		return errors.New(EnvOheyAdminEmails + " must include at least one moderation administrator in production")
	}
	for _, email := range cfg.AdminEmails {
		parsed, err := mail.ParseAddress(email)
		if err != nil || !strings.EqualFold(parsed.Address, strings.TrimSpace(email)) {
			return errors.New(EnvOheyAdminEmails + " must contain valid email addresses")
		}
	}
	if err := validateProductionDatabaseURL(cfg.DatabaseURL); err != nil {
		return err
	}
	return nil
}

func validateHTTPSURL(key, value string, originOnly bool) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New(key + " must contain valid https URLs")
	}
	if originOnly && (parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "") {
		return errors.New(key + " entries must be HTTPS origins without paths, queries, or fragments")
	}
	return nil
}

func validateProductionDatabaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return errors.New(EnvDatabaseURL + " must be a valid Postgres URL in production")
	}
	switch strings.ToLower(parsed.Query().Get("sslmode")) {
	case "require", "verify-ca", "verify-full":
		return nil
	default:
		return errors.New(EnvDatabaseURL + " must require TLS with sslmode=require, verify-ca, or verify-full in production")
	}
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

func isProduction(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "production")
}
