package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LegacySQLiteFile struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modified_at"`
}

type LegacySQLiteBackupInfo struct {
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Size      int64              `json:"size"`
	FileCount int                `json:"file_count"`
	CreatedAt int64              `json:"created_at"`
	Files     []LegacySQLiteFile `json:"files"`
}

type LegacySQLiteReport struct {
	Detected         bool               `json:"detected"`
	SQLiteAvailable  bool               `json:"sqlite_available"`
	DatabaseDir      string             `json:"database_dir"`
	FileCount        int                `json:"file_count"`
	TotalSize        int64              `json:"total_size"`
	Files            []LegacySQLiteFile `json:"files"`
	TableCounts      map[string]int     `json:"table_counts,omitempty"`
	ActiveAdminCount int                `json:"active_admin_count,omitempty"`
	Warnings         []string           `json:"warnings,omitempty"`
}

func InspectLegacySQLite(ctx context.Context, databaseDir string) LegacySQLiteReport {
	files, err := ListLegacySQLiteFiles(databaseDir)
	report := LegacySQLiteReport{
		Detected:    len(files) > 0,
		DatabaseDir: firstNonEmptyStore(databaseDir, "db"),
		FileCount:   len(files),
		Files:       files,
	}
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
	}
	for _, file := range files {
		report.TotalSize += file.Size
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		if report.Detected {
			report.Warnings = append(report.Warnings, "sqlite3 command is unavailable; table counts cannot be inspected")
		}
		return report
	}
	report.SQLiteAvailable = true
	counts := map[string]int{}
	for _, file := range files {
		if !strings.HasSuffix(strings.ToLower(file.Name), ".db") {
			continue
		}
		tableNames, err := legacySQLiteTables(ctx, sqliteBin, file.Path)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %v", file.Name, err))
			continue
		}
		for _, table := range tableNames {
			count, err := legacySQLiteTableCount(ctx, sqliteBin, file.Path, table)
			if err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s.%s: %v", file.Name, table, err))
				continue
			}
			key := strings.TrimSuffix(file.Name, ".db") + "." + table
			counts[key] = count
			if file.Name == "users.db" && table == "users" {
				report.ActiveAdminCount = legacySQLiteActiveAdminCount(ctx, sqliteBin, file.Path)
			}
		}
	}
	if len(counts) > 0 {
		report.TableCounts = counts
	}
	return report
}

func ListLegacySQLiteFiles(databaseDir string) ([]LegacySQLiteFile, error) {
	databaseDir = firstNonEmptyStore(strings.TrimSpace(databaseDir), "db")
	entries, err := os.ReadDir(databaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]LegacySQLiteFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !isLegacySQLiteFilename(lower) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, LegacySQLiteFile{
			Name:       name,
			Path:       filepath.Join(databaseDir, name),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Unix(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func BackupLegacySQLite(databaseDir, backupDir string) (LegacySQLiteBackupInfo, bool, error) {
	files, err := ListLegacySQLiteFiles(databaseDir)
	if err != nil || len(files) == 0 {
		return LegacySQLiteBackupInfo{}, false, err
	}
	backupDir = firstNonEmptyStore(strings.TrimSpace(backupDir), filepath.Join("db", "backups"))
	now := time.Now().UTC()
	name := "legacy_sqlite_" + now.Format("20060102_150405") + "_" + strconv.FormatInt(now.UnixNano()%1e9, 10)
	targetDir := filepath.Join(backupDir, name)
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return LegacySQLiteBackupInfo{}, true, err
	}
	info := LegacySQLiteBackupInfo{Name: name, Path: targetDir, CreatedAt: now.Unix(), Files: make([]LegacySQLiteFile, 0, len(files))}
	for _, file := range files {
		target := filepath.Join(targetDir, file.Name)
		if err := copyRegularFile(file.Path, target); err != nil {
			return LegacySQLiteBackupInfo{}, true, err
		}
		copied, err := os.Stat(target)
		if err != nil {
			return LegacySQLiteBackupInfo{}, true, err
		}
		item := LegacySQLiteFile{Name: file.Name, Path: target, Size: copied.Size(), ModifiedAt: copied.ModTime().Unix()}
		info.Files = append(info.Files, item)
		info.Size += item.Size
	}
	info.FileCount = len(info.Files)
	return info, true, nil
}

func copyRegularFile(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrNotFound
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func isLegacySQLiteFilename(name string) bool {
	return strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm")
}

func legacySQLiteTables(ctx context.Context, sqliteBin, path string) ([]string, error) {
	out, err := exec.CommandContext(ctx, sqliteBin, "-readonly", "-noheader", "-batch", path, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;`).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tables := make([]string, 0, len(lines))
	for _, line := range lines {
		table := strings.TrimSpace(line)
		if table != "" {
			tables = append(tables, table)
		}
	}
	return tables, nil
}

func legacySQLiteTableCount(ctx context.Context, sqliteBin, path, table string) (int, error) {
	query := `SELECT COUNT(*) FROM ` + quoteSQLiteIdentifier(table) + `;`
	out, err := exec.CommandContext(ctx, sqliteBin, "-readonly", "-noheader", "-batch", path, query).Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

func legacySQLiteActiveAdminCount(ctx context.Context, sqliteBin, path string) int {
	out, err := exec.CommandContext(ctx, sqliteBin, "-readonly", "-noheader", "-batch", path, `SELECT COUNT(*) FROM users WHERE ROLE=0 AND COALESCE(ACTIVE_STATUS, 0)=1;`).Output()
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return count
}

func quoteSQLiteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func firstNonEmptyStore(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
