package api

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/prejudice-studio/twilight/internal/migration"
)

func TestCollectMigrationResourcesUsesLogicalPaths(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"avatar/user.jpg":                 "avatar",
		"background/dark.webp":            "background",
		"tickets/42/reply/image.png":      "ticket",
		"server-icon/site.png":            "icon",
		"auth-background/background.jpeg": "auth",
		"bangumi/123.jpg":                 "cover",
	}
	for relative, content := range files {
		filename := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := collectMigrationResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(files) {
		t.Fatalf("resource count = %d, want %d", len(got), len(files))
	}
	seen := make(map[string]string, len(got))
	for _, file := range got {
		seen[file.Path] = string(file.Data)
		if filepath.IsAbs(file.Path) || file.Kind != "resource" {
			t.Fatalf("unsafe migration resource entry: %#v", file)
		}
	}
	want := map[string]string{
		"resources/avatars/user.jpg":                "avatar",
		"resources/backgrounds/dark.webp":           "background",
		"resources/tickets/42/reply/image.png":      "ticket",
		"resources/server-icon/site.png":            "icon",
		"resources/auth-background/background.jpeg": "auth",
		"resources/bangumi/123.jpg":                 "cover",
	}
	for path, content := range want {
		if seen[path] != content {
			t.Fatalf("resource %q = %q, want %q; all=%v", path, seen[path], content, seen)
		}
	}
}

func TestCollectMigrationResourcesRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires elevated privileges on some Windows hosts")
	}
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "avatar"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "avatar", "linked.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectMigrationResources(root); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestCollectMigrationResourcesHonorsBounds(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "avatar"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "avatar", "a.jpg"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	limits := migration.Limits{MaxFiles: 2, MaxFileBytes: 4, MaxTotalBytes: 8, MaxManifest: 1}
	if _, err := collectMigrationResourcesWithLimits(root, limits); !errors.Is(err, migration.ErrArchiveLimit) {
		t.Fatalf("error = %v, want ErrArchiveLimit", err)
	}
}
