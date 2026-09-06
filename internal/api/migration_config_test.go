package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/config"
)

func TestCollectMigrationConfigOmitsSecretsFromPlaintextArchive(t *testing.T) {
	cfg := config.Config{
		AppName:              "Twilight Test",
		DatabaseURL:          "postgres://user:password@example.invalid/db",
		EmbyToken:            "emby-secret-token",
		TelegramBotToken:     "telegram-secret-token",
		SMTPPassword:         "smtp-secret-password",
		TMDBAPIKey:           "tmdb-secret-key",
		BangumiToken:         "bangumi-secret-token",
		BotInternalSecret:    "bot-secret",
		BangumiWebhookSecret: "webhook-secret",
	}
	files, err := collectMigrationConfig(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("config file count = %d, want 2", len(files))
	}
	var toml string
	var policy migrationConfigPolicy
	for _, file := range files {
		switch file.Path {
		case "config/effective.toml":
			toml = string(file.Data)
		case "config/policy.json":
			if err := json.Unmarshal(file.Data, &policy); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, secret := range []string{"password@example.invalid", "emby-secret-token", "telegram-secret-token", "smtp-secret-password", "tmdb-secret-key", "bangumi-secret-token", "bot-secret", "webhook-secret"} {
		if strings.Contains(toml, secret) {
			t.Fatalf("plaintext migration config contains secret %q", secret)
		}
	}
	if !strings.Contains(toml, secretMaskValue) {
		t.Fatalf("plaintext migration config does not contain secret sentinel")
	}
	if policy.FormatVersion != "twilight-config/v1" || policy.SecretsIncluded {
		t.Fatalf("unexpected plaintext policy: %#v", policy)
	}
	if migrationConfigContainsSecret(toml, cfg) {
		t.Fatal("secret detector reported a secret in plaintext config")
	}
}

func TestCollectMigrationConfigIncludesSecretsOnlyWhenProtected(t *testing.T) {
	cfg := config.Config{EmbyToken: "emby-secret-token", SMTPPassword: "smtp-secret-password"}
	files, err := collectMigrationConfig(cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	var toml string
	for _, file := range files {
		if file.Path == "config/effective.toml" {
			toml = string(file.Data)
		}
	}
	for _, secret := range []string{"emby-secret-token", "smtp-secret-password"} {
		if !strings.Contains(toml, secret) {
			t.Fatalf("protected migration config does not contain %q", secret)
		}
	}
	if !migrationConfigContainsSecret(toml, cfg) {
		t.Fatal("secret detector did not find protected config secret")
	}
}
