package api

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWithinRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveWithinRoot(root, "file.json"); err != nil {
		t.Errorf("expected ok, got %v", err)
	}
	if _, err := ResolveWithinRoot(root, "../etc/passwd"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath, got %v", err)
	}
	if _, err := ResolveWithinRoot(root, ""); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath for empty, got %v", err)
	}
	if _, err := ResolveWithinRoot(root, "/etc/passwd"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath for abs path, got %v", err)
	}
	// nested ok
	nested := filepath.Join(root, "sub", "a.json")
	if _, err := ResolveWithinRoot(root, "sub/a.json"); err != nil {
		t.Errorf("expected ok for nested, got %v (nested=%s)", err, nested)
	}
}

func TestResolveLeafFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ok.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveLeafFile(root, "ok.json", "json"); err != nil {
		t.Errorf("expected ok, got %v", err)
	}
	if _, err := ResolveLeafFile(root, "ok.json", "toml"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ext mismatch ErrUnsafePath, got %v", err)
	}
	if _, err := ResolveLeafFile(root, "../ok.json", "json"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath for parent ref, got %v", err)
	}
	if _, err := ResolveLeafFile(root, "sub/ok.json", "json"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath for nested name, got %v", err)
	}
	if _, err := ResolveLeafFile(root, "missing.json", "json"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath for missing file, got %v", err)
	}
}
