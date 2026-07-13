package config

import (
	"strings"
	"testing"
)

func setValidProductionEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvAppEnvironment, "production")
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvAuthProvider, "clerk")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")
	t.Setenv(EnvClerkAudience, "ohey-mobile")
	t.Setenv(EnvClerkSecretKey, "sk_live_example")
	t.Setenv(EnvAllowedOrigins, "https://oheyapp.com")
	t.Setenv(EnvOriginVerifySecret, strings.Repeat("a", minimumOriginVerifySecretBytes))
	t.Setenv(EnvOheyAdminEmails, "admin@oheyapp.com")
}

func TestLoadRequiresDatabaseURLByDefault(t *testing.T) {
	t.Setenv(EnvClerkIssuer, "https://clerk.example")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvDatabaseURL) {
		t.Fatalf("Load() error = %v, want %s requirement", err, EnvDatabaseURL)
	}
}

func TestLoadRejectsNonClerkAuth(t *testing.T) {
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvAuthProvider, "legacy")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAuthProvider) {
		t.Fatalf("Load() error = %v, want %s requirement", err, EnvAuthProvider)
	}
}

func TestLoadNeonStoreParsesDatabaseMaxConns(t *testing.T) {
	t.Setenv(EnvAppEnvironment, "development")
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvDatabaseMaxConns, "4")
	t.Setenv(EnvAuthProvider, "clerk")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DataStore != "neon" {
		t.Fatalf("DataStore = %q, want neon", cfg.DataStore)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL is empty")
	}
	if cfg.DatabaseMaxConns != 4 {
		t.Fatalf("DatabaseMaxConns = %d, want 4", cfg.DatabaseMaxConns)
	}
}

func TestLoadRejectsWildcardOriginInProduction(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvAllowedOrigins, "*")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAllowedOrigins) {
		t.Fatalf("Load() error = %v, want %s production validation", err, EnvAllowedOrigins)
	}
}

func TestLoadRequiresClerkAudienceInProduction(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvClerkAudience, "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvClerkAudience) {
		t.Fatalf("Load() error = %v, want %s production validation", err, EnvClerkAudience)
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	t.Setenv(EnvAppEnvironment, "prodution")
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAppEnvironment) {
		t.Fatalf("Load() error = %v, want %s validation", err, EnvAppEnvironment)
	}
}

func TestLoadRequiresProductionOriginSecret(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvOriginVerifySecret, "short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvOriginVerifySecret) {
		t.Fatalf("Load() error = %v, want %s validation", err, EnvOriginVerifySecret)
	}
}

func TestLoadRequiresProductionClerkSecret(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvClerkSecretKey, "sk_test_example")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvClerkSecretKey) {
		t.Fatalf("Load() error = %v, want %s validation", err, EnvClerkSecretKey)
	}
}

func TestLoadRequiresTLSForProductionDatabase(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=disable")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "sslmode") {
		t.Fatalf("Load() error = %v, want database TLS validation", err)
	}
}

func TestLoadRejectsProductionOriginWithPath(t *testing.T) {
	setValidProductionEnv(t)
	t.Setenv(EnvAllowedOrigins, "https://oheyapp.com/app")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAllowedOrigins) {
		t.Fatalf("Load() error = %v, want origin validation", err)
	}
}

func TestLoadAcceptsHardenedProductionConfig(t *testing.T) {
	setValidProductionEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q, want production", cfg.Environment)
	}
}
