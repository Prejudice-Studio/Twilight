package api

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/prejudice-studio/twilight/internal/migration"
)

// migrationResourceRoots is deliberately explicit. UploadDir may contain
// administrator-managed files that are not Twilight resources and must not be
// copied into a migration archive by accident.
var migrationResourceRoots = []struct {
	source string
	target string
}{
	{source: "avatar", target: "avatars"},
	{source: "background", target: "backgrounds"},
	{source: "tickets", target: "tickets"},
	{source: "server-icon", target: "server-icon"},
	{source: "auth-background", target: "auth-background"},
	{source: "bangumi", target: "bangumi"},
}

// collectMigrationResources reads only the approved UploadDir namespaces and
// returns logical resources/ paths. It never exposes the machine's absolute
// path in the archive manifest.
func collectMigrationResources(uploadDir string) ([]migration.InputFile, error) {
	return collectMigrationResourcesWithLimits(uploadDir, migration.DefaultLimits())
}

func collectMigrationResourcesWithLimits(uploadDir string, limits migration.Limits) ([]migration.InputFile, error) {
	if limits.MaxFiles <= 0 || limits.MaxFileBytes <= 0 || limits.MaxTotalBytes <= 0 {
		limits = migration.DefaultLimits()
	}
	root := firstNonEmpty(strings.TrimSpace(uploadDir), "uploads")
	rootInfo, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return []migration.InputFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect migration resource root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, fmt.Errorf("migration resource root is not a directory")
	}

	collector := migrationResourceCollector{limits: limits}
	for _, spec := range migrationResourceRoots {
		sourceDir, resolveErr := ResolveWithinRoot(root, spec.source)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve migration resource directory %q: %w", spec.source, resolveErr)
		}
		info, statErr := os.Lstat(sourceDir)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return nil, fmt.Errorf("inspect migration resource directory %q: %w", spec.source, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("migration resource directory %q is not a real directory", spec.source)
		}
		if err := collector.walk(sourceDir, "resources/"+spec.target); err != nil {
			return nil, err
		}
	}
	return collector.files, nil
}

type migrationResourceCollector struct {
	limits migration.Limits
	files  []migration.InputFile
	total  int64
}

func (c *migrationResourceCollector) walk(directory, logicalPrefix string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read migration resource directory: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") {
			return fmt.Errorf("invalid migration resource filename")
		}
		candidate, candidateErr := ResolveWithinRoot(directory, name)
		if candidateErr != nil {
			return fmt.Errorf("resolve migration resource %q: %w", name, candidateErr)
		}
		info, statErr := os.Lstat(candidate)
		if statErr != nil {
			return fmt.Errorf("inspect migration resource %q: %w", name, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("migration resource symlink rejected: %q", name)
		}
		logicalPath := path.Join(logicalPrefix, name)
		if info.IsDir() {
			if err := c.walk(candidate, logicalPath); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("migration resource is not a regular file: %q", name)
		}
		if len(c.files) >= c.limits.MaxFiles || info.Size() < 0 || info.Size() > c.limits.MaxFileBytes || c.total+info.Size() > c.limits.MaxTotalBytes {
			return migration.ErrArchiveLimit
		}
		data, readErr := readMigrationResource(candidate, info.Size(), c.limits.MaxFileBytes)
		if readErr != nil {
			return fmt.Errorf("read migration resource %q: %w", name, readErr)
		}
		c.files = append(c.files, migration.InputFile{
			Path:        filepath.ToSlash(logicalPath),
			Kind:        "resource",
			ContentType: mime.TypeByExtension(strings.ToLower(filepath.Ext(name))),
			Data:        data,
		})
		c.total += int64(len(data))
	}
	return nil
}

func readMigrationResource(filename string, expectedSize, limit int64) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit || int64(len(data)) != expectedSize {
		return nil, migration.ErrArchiveLimit
	}
	latest, err := os.Lstat(filename)
	if err != nil || latest.Mode()&os.ModeSymlink != 0 || !latest.Mode().IsRegular() || latest.Size() != expectedSize {
		return nil, fmt.Errorf("resource changed during collection")
	}
	return data, nil
}
