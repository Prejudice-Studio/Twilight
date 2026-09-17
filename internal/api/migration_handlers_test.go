package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/migration"
)

func TestMigrationResourcePathPartsRejectsUnsafeLogicalPaths(t *testing.T) {
	valid := []string{
		"resources/avatars/user.png",
		"resources/tickets/42/reply/image.webp",
		"resources/bangumi/123.jpg",
	}
	for _, value := range valid {
		if source, relative, err := migrationResourcePathParts(value); err != nil || source == "" || relative == "" {
			t.Fatalf("valid resource %q returned (%q, %q, %v)", value, source, relative, err)
		}
	}
	for _, value := range []string{
		"resources/avatars/",
		"resources/avatars/../secret",
		"resources/avatars/..\\secret",
		"resources/unknown/file.png",
		"/resources/avatars/file.png",
	} {
		if _, _, err := migrationResourcePathParts(value); err == nil {
			t.Fatalf("unsafe resource path %q was accepted", value)
		}
	}
}

func TestPlanMigrationResourcesUsesAbsoluteRootAndDetectsConflict(t *testing.T) {
	root := t.TempDir()
	resource := filepath.Join(root, "avatar", "user.png")
	if err := os.MkdirAll(filepath.Dir(resource), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resource, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	app.runtime.Store(&runtimeState{cfg: config.Config{UploadDir: root}})
	archive := migration.Archive{
		Manifest: migration.Manifest{Files: []migration.FileEntry{{Path: "resources/avatars/user.png", Kind: "resource", Size: 3}}},
		Files:    map[string][]byte{"resources/avatars/user.png": []byte("new")},
	}
	plan, err := app.planMigrationResources(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0] != "resources/avatars/user.png" {
		t.Fatalf("conflicts = %#v", plan.Conflicts)
	}
	if len(plan.Entries) != 1 || !filepath.IsAbs(plan.Entries[0].TargetPath) {
		t.Fatalf("target path was not normalized: %#v", plan.Entries)
	}
}

func TestApplyMigrationResourcesRollsBackOnConflict(t *testing.T) {
	root := t.TempDir()
	app := &App{}
	app.runtime.Store(&runtimeState{cfg: config.Config{UploadDir: root}})
	plan := migrationResourcePlan{Entries: []migrationResourcePlanEntry{
		{TargetPath: filepath.Join(root, "avatar", "new.png"), Data: []byte("new")},
		{TargetPath: filepath.Join(root, "avatar", "existing.png"), Data: []byte("replacement"), Exists: true},
	}}
	if err := os.MkdirAll(filepath.Dir(plan.Entries[1].TargetPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plan.Entries[1].TargetPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.applyMigrationResources(plan, false); !errors.Is(err, errMigrationResourceConflict) {
		t.Fatalf("error = %v, want resource conflict", err)
	}
	if _, err := os.Stat(plan.Entries[0].TargetPath); !os.IsNotExist(err) {
		t.Fatalf("new resource survived failed apply, stat error = %v", err)
	}
	data, err := os.ReadFile(plan.Entries[1].TargetPath)
	if err != nil || string(data) != "old" {
		t.Fatalf("existing resource changed after failed apply: %q, %v", data, err)
	}
}

func TestValidateMigrationArchiveFilesRequiresCoreData(t *testing.T) {
	archive := migration.Archive{Manifest: migration.Manifest{Files: []migration.FileEntry{{Path: "data/state.json", Kind: "data"}}}, Files: map[string][]byte{"data/state.json": []byte(`{}`)}}
	err := validateMigrationArchiveFiles(archive)
	if err == nil || !strings.Contains(err.Error(), "missing migration data file") {
		t.Fatalf("error = %v, want missing core data error", err)
	}
}
