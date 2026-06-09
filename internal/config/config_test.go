package config

import (
	"strings"
	"testing"
)

func TestLoadAllowsSupabaseStoreByDefault(t *testing.T) {
	t.Setenv(EnvSupabaseURL, "https://example.supabase.co")
	t.Setenv(EnvSupabaseAnonKey, "anon")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DataStore != "supabase" {
		t.Fatalf("DataStore = %q, want supabase", cfg.DataStore)
	}
	if cfg.DatabaseMaxConns != 10 {
		t.Fatalf("DatabaseMaxConns = %d, want 10", cfg.DatabaseMaxConns)
	}
}

func TestLoadRequiresDatabaseURLForNeonStore(t *testing.T) {
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvAuthProvider, "clerk")
	t.Setenv(EnvClerkIssuer, "https://clerk.example")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvDatabaseURL) {
		t.Fatalf("Load() error = %v, want %s requirement", err, EnvDatabaseURL)
	}
}

func TestLoadRequiresClerkAuthForNeonStore(t *testing.T) {
	t.Setenv(EnvDataStore, "neon")
	t.Setenv(EnvDatabaseURL, "postgres://user:pass@example.neon.tech/db?sslmode=require")
	t.Setenv(EnvAuthProvider, "supabase")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), EnvAuthProvider) {
		t.Fatalf("Load() error = %v, want %s requirement", err, EnvAuthProvider)
	}
}

func TestLoadNeonStoreParsesDatabaseMaxConns(t *testing.T) {
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
