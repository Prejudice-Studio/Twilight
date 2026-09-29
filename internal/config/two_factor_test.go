package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTwoFactorKeyConfigPrecedence(t *testing.T) {
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	local := filepath.Join(dir, "config.local.toml")
	t.Setenv("TWILIGHT_CONFIG_LOCAL_FILE", local)
	write := func(path, key string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("[Security]\ntwo_factor_key = \""+key+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	check := func(load func(string) (Config, error), want string) {
		t.Helper()
		cfg, err := load(path)
		if err != nil || cfg.TwoFactorKey != want {
			t.Fatalf("wrong key source: %v", err)
		}
	}
	write(path, "primary-key")
	check(Load, "primary-key")
	write(local, "local-key")
	check(Load, "local-key")
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "environment-key")
	check(Load, "environment-key")
	check(LoadFileOnly, "primary-key")
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	check(Load, "local-key")
}
