package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTOMLAndEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[Global]
server_name = "Test Twilight"
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
	if cfg.AppName != "Test Twilight" {
		t.Fatalf("server_name was not loaded: %q", cfg.AppName)
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
state_file = "db/existing_state.json"
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
	if cfg.StateFile != "db/existing_state.json" {
		t.Fatalf("state_file was not loaded: %q", cfg.StateFile)
	}
	if cfg.PostgresDSN() == "" || cfg.PostgresSSLMode != "require" {
		t.Fatalf("expected postgres dsn, got %q", cfg.PostgresDSN())
	}
	if len(cfg.EmbyURLList) != 2 || cfg.EmbyURLList[0].Name != "Direct" {
		t.Fatalf("unexpected emby lines: %#v", cfg.EmbyURLList)
	}
}

func TestLoadDefaultPathPrefersConfigTomlOverEnv(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := os.WriteFile("config.toml", []byte("[API]\nport = 5051\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(dir, "env.toml")
	if err := os.WriteFile(envPath, []byte("[API]\nport = 6061\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWILIGHT_CONFIG_FILE", envPath)

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != "config.toml" || cfg.Port != 5051 {
		t.Fatalf("expected local config.toml to win, file=%q port=%d", cfg.ConfigFile, cfg.Port)
	}
}

func TestLoadDefaultPathIgnoresEnvWhenConfigTomlMissing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	envPath := filepath.Join(dir, "env.toml")
	if err := os.WriteFile(envPath, []byte("[API]\nport = 6062\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWILIGHT_CONFIG_FILE", envPath)

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != "config.toml" || cfg.Port != 5000 {
		t.Fatalf("expected default config.toml and built-in defaults, file=%q port=%d", cfg.ConfigFile, cfg.Port)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
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
	if cfg.DatabaseDriver != "postgres" {
		t.Fatalf("database defaults should use postgres, got %q", cfg.DatabaseDriver)
	}
	if got := cfg.PostgresDSN(); !strings.Contains(got, "://twilight@127.0.0.1:5432/twilight") {
		t.Fatalf("postgres defaults should produce a usable local dsn, got %q", got)
	}
}

func TestLogConfigSupportsLegacyNumericLevels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[Global]\nlog_level = \"WARNING\"\nruntime_log_limit = 9000\n\n[Database]\nmigration_panel_enabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "warn" || cfg.RuntimeLogLimit != 9000 {
		t.Fatalf("unexpected log config: level=%q limit=%d", cfg.LogLevel, cfg.RuntimeLogLimit)
	}
	if !cfg.DatabaseMigrationPanelEnabled {
		t.Fatal("expected database migration panel to be enabled")
	}
	if cfg.SlogLevel() != slog.LevelWarn {
		t.Fatalf("expected warn slog level, got %v", cfg.SlogLevel())
	}

	if err := os.WriteFile(path, []byte("[Global]\nlog_level = 10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "debug" || cfg.SlogLevel() != slog.LevelDebug {
		t.Fatalf("expected legacy numeric debug level, got %q/%v", cfg.LogLevel, cfg.SlogLevel())
	}
}

func TestTelegramCustomCommandsParseReplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `[Telegram]
bot_custom_commands = [
  "/hello = 第一行\n第二行",
  "ping = pong",
  "bad command = ignored",
]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TelegramCustomCommands) != 2 {
		t.Fatalf("unexpected custom commands: %#v", cfg.TelegramCustomCommands)
	}
	if cfg.TelegramCustomCommands[0].Command != "/hello" || cfg.TelegramCustomCommands[0].Reply != "第一行\n第二行" {
		t.Fatalf("custom command did not parse newline reply: %#v", cfg.TelegramCustomCommands[0])
	}
	if cfg.TelegramCustomCommands[1].Command != "/ping" || cfg.TelegramCustomCommands[1].Reply != "pong" {
		t.Fatalf("custom command normalization failed: %#v", cfg.TelegramCustomCommands[1])
	}
}

func TestProductionTemplateIncludesPostgresDatabaseSection(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.production.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseDriver != "postgres" {
		t.Fatalf("production template should default to postgres, got %q", cfg.DatabaseDriver)
	}
	if cfg.PostgresUser != "twilight" || cfg.PostgresDatabase != "twilight" || cfg.PostgresMaxOpenConns != 8 || cfg.PostgresMaxIdleConns != 4 {
		t.Fatalf("production template postgres fields were not loaded: %#v", cfg)
	}
}
