package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/migration"
)

type migrationConfigPolicy struct {
	FormatVersion   string `json:"format_version"`
	SecretsIncluded bool   `json:"secrets_included"`
	Source          string `json:"source"`
}

// collectMigrationConfig exports the effective, schema-known configuration.
// A plaintext archive contains only the secret sentinel, so importing it can
// preserve the destination instance's credentials. Password-protected archives
// may carry the effective secrets because the whole payload is authenticated
// and encrypted by migration.Create.
func collectMigrationConfig(cfg config.Config, passwordProtected bool) ([]migration.InputFile, error) {
	values := configValues(cfg)
	content := renderConfigTOML(values)
	if !passwordProtected {
		content = maskTOMLSecrets(content)
	}
	policy, err := json.Marshal(migrationConfigPolicy{
		FormatVersion:   "twilight-config/v1",
		SecretsIncluded: passwordProtected,
		Source:          "effective-runtime-config",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal migration config policy: %w", err)
	}
	return []migration.InputFile{
		{
			Path:        "config/effective.toml",
			Kind:        "config",
			ContentType: "application/toml",
			Data:        []byte(content),
		},
		{
			Path:        "config/policy.json",
			Kind:        "config",
			ContentType: "application/json",
			Data:        policy,
		},
	}, nil
}

func migrationConfigContainsSecret(content string, cfg config.Config) bool {
	if strings.TrimSpace(content) == "" {
		return false
	}
	values := configValues(cfg)
	for section, fields := range values {
		for field, value := range fields {
			if !isSecretField(section, field) {
				continue
			}
			if text, ok := value.(string); ok && text != "" && text != secretMaskValue && strings.Contains(content, text) {
				return true
			}
		}
	}
	return false
}
