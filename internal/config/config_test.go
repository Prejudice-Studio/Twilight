package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTOMLAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[Global]
redis_url = "redis://localhost:6379/2"

[API]
host = "127.0.0.1"
port = 5050
cors_origins = ["http://localhost:3000", "https://example.com"]
max_upload_size = 1234
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWILIGHT_API_PORT", "6060")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisURL != "redis://localhost:6379/2" {
		t.Fatalf("unexpected redis url: %q", cfg.RedisURL)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 6060 {
		t.Fatalf("unexpected host/port: %s/%d", cfg.Host, cfg.Port)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.MaxUploadSize != 1234 {
		t.Fatalf("unexpected cors/upload config: %#v %d", cfg.CORSOrigins, cfg.MaxUploadSize)
	}
}

func TestLoadMultilineArraysAndPostgresConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[Global]
databases_dir = "db"

[Database]
driver = "postgres"
postgres_host = "db.local"
postgres_port = 5433
postgres_user = "twilight"
postgres_password = "secret"
postgres_database = "twilight_prod"
postgres_sslmode = "require"
postgres_max_open_conns = 16
postgres_max_idle_conns = 8

[Emby]
emby_url_list = [
  "Direct : http://127.0.0.1:8096/",
  "Relay : https://emby.example.com/",
]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "postgres" || cfg.PostgresPort != 5433 || cfg.PostgresMaxOpenConns != 16 || cfg.PostgresMaxIdleConns != 8 {
		t.Fatalf("unexpected database config: %#v", cfg)
	}
	if cfg.PostgresDSN() == "" || cfg.PostgresSSLMode != "require" {
		t.Fatalf("expected postgres dsn, got %q", cfg.PostgresDSN())
	}
	if len(cfg.EmbyURLList) != 2 || cfg.EmbyURLList[0].Name != "Direct" {
		t.Fatalf("unexpected emby lines: %#v", cfg.EmbyURLList)
	}
}

func TestPostgresEnvOverridesAndIPv6DSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[Database]
driver = "postgres"
postgres_host = "::1"
postgres_port = 5432
postgres_user = "twilight"
postgres_password = "secret"
postgres_database = "twilight"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWILIGHT_POSTGRES_MAX_OPEN_CONNS", "20")
	t.Setenv("TWILIGHT_POSTGRES_MAX_IDLE_CONNS", "10")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PostgresMaxOpenConns != 20 || cfg.PostgresMaxIdleConns != 10 {
		t.Fatalf("postgres pool env overrides failed: open=%d idle=%d", cfg.PostgresMaxOpenConns, cfg.PostgresMaxIdleConns)
	}
	if got := cfg.PostgresDSN(); !strings.Contains(got, "://twilight:secret@[::1]:5432/") {
		t.Fatalf("IPv6 DSN was not bracketed correctly: %s", got)
	}

	t.Setenv("TWILIGHT_POSTGRES_DSN", "postgres://env-user:env-pass@db.example/twilight?sslmode=require")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.PostgresDSN(); got != "postgres://env-user:env-pass@db.example/twilight?sslmode=require" {
		t.Fatalf("postgres dsn env alias was not honored: %s", got)
	}
}

func TestDefaultsIncludeUsablePostgresParts(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PostgresUser != "twilight" || cfg.PostgresDatabase != "twilight" {
		t.Fatalf("unexpected postgres defaults: user=%q database=%q", cfg.PostgresUser, cfg.PostgresDatabase)
	}
	if got := cfg.PostgresDSN(); !strings.Contains(got, "://twilight@127.0.0.1:5432/twilight") {
		t.Fatalf("postgres defaults should produce a usable local dsn, got %q", got)
	}
}

func TestProductionTemplateIncludesPostgresDatabaseSection(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.production.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "json" {
		t.Fatalf("production template should default to json, got %q", cfg.DatabaseDriver)
	}
	if cfg.PostgresUser != "twilight" || cfg.PostgresDatabase != "twilight" || cfg.PostgresMaxOpenConns != 8 || cfg.PostgresMaxIdleConns != 4 {
		t.Fatalf("production template postgres fields were not loaded: %#v", cfg)
	}
}
