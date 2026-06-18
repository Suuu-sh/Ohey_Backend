package config

import (
	"strings"
	"testing"
)

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
	t.Setenv(EnvAppEnvironment, "production")
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvAuthProvider, "clerk")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")
	t.Setenv(EnvClerkAudience, "ohey-mobile")
	t.Setenv(EnvAllowedOrigins, "*")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAllowedOrigins) {
		t.Fatalf("Load() error = %v, want %s production validation", err, EnvAllowedOrigins)
	}
}

func TestLoadRequiresClerkAudienceInProduction(t *testing.T) {
	t.Setenv(EnvAppEnvironment, "production")
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvAuthProvider, "clerk")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")
	t.Setenv(EnvAllowedOrigins, "https://oheyapp.com")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvClerkAudience) {
		t.Fatalf("Load() error = %v, want %s production validation", err, EnvClerkAudience)
	}
}
