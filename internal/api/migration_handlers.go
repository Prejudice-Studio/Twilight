package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/migration"
	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	migrationImportConfirmPhrase  = "IMPORT_TWILIGHT_DATA"
	migrationResourceModePreserve = "preserve"
	migrationResourceModeReplace  = "replace"
	migrationMultipartMemory      = 8 << 20
	migrationDatabaseSchema       = "postgres-state-v1"
)

var migrationDataFileNames = map[string]struct{}{
	"data/state.json":            {},
	"data/runtime-logs.json":     {},
	"data/audit-logs.json":       {},
	"data/telegram-roster.json":  {},
	"data/telegram-runtime.json": {},
	"data/playback-records.json": {},
}

var errMigrationResourceConflict = errors.New("migration resource conflict")

type migrationExportRequest struct {
	Password string `json:"password"`
}

type migrationImportOptions struct {
	Password     string
	Confirm      string
	Preview      bool
	ApplyConfig  bool
	ResourceMode string
}

type migrationResourcePlanEntry struct {
	LogicalPath string
	TargetPath  string
	Data        []byte
	Exists      bool
	Same        bool
}

type migrationResourcePlan struct {
	Entries   []migrationResourcePlanEntry
	Conflicts []string
}

type migrationResourceRollbackEntry struct {
	target  string
	backup  string
	existed bool
}

func (a *App) handleMigrationStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.migrationEnabled(w) {
		return
	}
	ok(w, "OK", map[string]any{
		"enabled":                 true,
		"format_version":          migration.FormatVersion,
		"database_schema_version": migrationDatabaseSchema,
		"max_archive_bytes":       migration.MaxArchiveBytes,
		"resource_namespaces": []string{
			"resources/avatars/",
			"resources/backgrounds/",
			"resources/tickets/",
			"resources/server-icon/",
			"resources/auth-background/",
			"resources/bangumi/",
		},
	})
}

func (a *App) handleMigrationExport(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.migrationEnabled(w) {
		return
	}
	var request migrationExportRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSON(r, &request); err != nil {
			failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "导出参数无效")
			return
		}
	}
	if len([]byte(request.Password)) > 1024 || (request.Password != "" && len([]byte(request.Password)) < 8) {
		failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "迁移密码长度无效")
		return
	}

	a.migrationMu.Lock()
	defer a.migrationMu.Unlock()
	archive, manifest, fileCount, err := a.createMigrationArchive(r, request.Password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrMigrationExportFail, "生成迁移包失败")
		return
	}
	filename := "twilight-migration-" + time.Now().UTC().Format("20060102-150405") + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Twilight-Migration-Format", manifest.FormatVersion)
	w.Header().Set("X-Twilight-Migration-Files", fmt.Sprintf("%d", fileCount))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
	a.audit(r, "export_migration_archive", "admin", 0, map[string]any{
		"files": fileCount, "bytes": len(archive), "encrypted": manifest.Encrypted,
	})
}

func (a *App) handleMigrationImport(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.migrationEnabled(w) {
		return
	}
	archiveBytes, options, err := readMigrationMultipart(r)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "迁移包上传无效")
		return
	}
	if options.ResourceMode == "" {
		options.ResourceMode = migrationResourceModePreserve
	}
	if options.ResourceMode != migrationResourceModePreserve && options.ResourceMode != migrationResourceModeReplace {
		failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "资源覆盖模式无效")
		return
	}

	a.migrationMu.Lock()
	defer a.migrationMu.Unlock()
	archive, err := migration.Open(archiveBytes, options.Password, migration.DefaultLimits())
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationArchiveBad, "迁移包校验失败或密码不正确")
		return
	}
	if err := validateMigrationArchiveFiles(archive); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationArchiveBad, "迁移包内容不受支持")
		return
	}
	plan, err := a.planMigrationResources(archive)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationArchiveBad, "迁移资源校验失败")
		return
	}
	configContent, hasConfig, err := a.prepareMigrationConfig(archive)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationArchiveBad, "迁移配置校验失败")
		return
	}
	summary := migrationArchiveSummary(archive, plan, hasConfig)
	summary["resource_mode"] = options.ResourceMode
	summary["apply_config_requested"] = options.ApplyConfig
	if len(plan.Conflicts) > 0 {
		summary["resource_conflicts"] = plan.Conflicts
		summary["resource_conflict_count"] = len(plan.Conflicts)
	}
	if options.Preview || options.Confirm != migrationImportConfirmPhrase {
		summary["dry_run"] = true
		summary["requires_confirmation"] = true
		ok(w, "迁移导入预览已生成", summary)
		return
	}
	if len(plan.Conflicts) > 0 && options.ResourceMode != migrationResourceModeReplace {
		failWithCode(w, http.StatusConflict, ErrMigrationConflict, "迁移资源存在冲突，请确认覆盖模式")
		return
	}
	if options.ApplyConfig && !hasConfig {
		failWithCode(w, http.StatusBadRequest, ErrMigrationArchiveBad, "迁移包不包含可应用的配置")
		return
	}

	previousConfig, hadConfig := readOptionalFile(a.configFilePath())
	if options.ApplyConfig {
		info, status, message := a.saveConfigContent(configContent)
		if status != http.StatusOK {
			failWithCode(w, status, ErrMigrationImportFail, message)
			return
		}
		_ = info
	}
	rollbackResources, err := a.applyMigrationResources(plan, options.ResourceMode == migrationResourceModeReplace)
	if err != nil {
		rollbackMigrationConfig(a, previousConfig, hadConfig)
		failWithCode(w, http.StatusInternalServerError, ErrMigrationImportFail, "迁移资源写入失败")
		return
	}
	if _, err := a.store().ImportMigrationArchive(r.Context(), archive); err != nil {
		rollbackResources()
		rollbackMigrationConfig(a, previousConfig, hadConfig)
		failWithCode(w, http.StatusInternalServerError, ErrMigrationImportFail, "迁移数据库导入失败")
		return
	}
	rollbackResources = func() {}
	summary["dry_run"] = false
	summary["requires_confirmation"] = false
	summary["config_applied"] = options.ApplyConfig
	summary["resources_written"] = len(plan.Entries)
	a.audit(r, "import_migration_archive", "admin", 0, map[string]any{
		"files": len(archive.Files), "resources": len(plan.Entries), "config_applied": options.ApplyConfig,
	})
	ok(w, "迁移数据已导入", summary)
}

func (a *App) migrationEnabled(w http.ResponseWriter) bool {
	if a.cfg().DatabaseMigrationPanelEnabled {
		return true
	}
	failWithCode(w, http.StatusForbidden, ErrMigrationDisabled, "数据库迁移功能未开启")
	return false
}

func (a *App) createMigrationArchive(r *http.Request, password string) ([]byte, migration.Manifest, int, error) {
	files, err := a.store().ExportMigrationFiles(r.Context())
	if err != nil {
		return nil, migration.Manifest{}, 0, err
	}
	resources, err := collectMigrationResources(a.cfg().UploadDir)
	if err != nil {
		return nil, migration.Manifest{}, 0, err
	}
	configFiles, err := collectMigrationConfig(*a.cfg(), password != "")
	if err != nil {
		return nil, migration.Manifest{}, 0, err
	}
	files = append(files, configFiles...)
	files = append(files, resources...)
	archive, manifest, err := migration.Create(migration.Input{
		TwilightVersion:       firstNonEmpty(a.cfg().Version, "unknown"),
		DatabaseSchemaVersion: migrationDatabaseSchema,
		Files:                 files,
		Password:              password,
	})
	return archive, manifest, len(files), err
}

func readMigrationMultipart(r *http.Request) ([]byte, migrationImportOptions, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data;") {
		return nil, migrationImportOptions{}, errors.New("multipart form required")
	}
	if err := r.ParseMultipartForm(migrationMultipartMemory); err != nil {
		return nil, migrationImportOptions{}, err
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		return nil, migrationImportOptions{}, err
	}
	defer file.Close()
	if header.Size > migration.MaxArchiveBytes {
		return nil, migrationImportOptions{}, migration.ErrArchiveLimit
	}
	data, err := io.ReadAll(io.LimitReader(file, migration.MaxArchiveBytes+1))
	if err != nil || int64(len(data)) > migration.MaxArchiveBytes {
		return nil, migrationImportOptions{}, migration.ErrArchiveLimit
	}
	options := migrationImportOptions{
		Password:     r.FormValue("password"),
		Confirm:      strings.TrimSpace(r.FormValue("confirm")),
		Preview:      boolFormValue(r.FormValue("preview")) || boolFormValue(r.FormValue("dry_run")),
		ApplyConfig:  boolFormValue(r.FormValue("apply_config")),
		ResourceMode: strings.ToLower(strings.TrimSpace(r.FormValue("resource_mode"))),
	}
	return data, options, nil
}

func boolFormValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func validateMigrationArchiveFiles(archive migration.Archive) error {
	for _, entry := range archive.Manifest.Files {
		switch {
		case entry.Kind == "data":
			if _, ok := migrationDataFileNames[entry.Path]; !ok {
				return fmt.Errorf("unsupported data file %q", entry.Path)
			}
		case entry.Kind == "config":
			if entry.Path != "config/effective.toml" && entry.Path != "config/policy.json" {
				return fmt.Errorf("unsupported config file %q", entry.Path)
			}
		case entry.Kind == "resource":
			if _, _, err := migrationResourcePathParts(entry.Path); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported migration file kind")
		}
	}
	for path := range migrationDataFileNames {
		if _, ok := archive.Files[path]; !ok {
			return fmt.Errorf("missing migration data file %q", path)
		}
	}
	if _, policy := archive.Files["config/policy.json"]; policy {
		if _, configFile := archive.Files["config/effective.toml"]; !configFile {
			return errors.New("config policy has no config content")
		}
	}
	return nil
}

func migrationResourcePathParts(logical string) (string, string, error) {
	if logical == "" || strings.Contains(logical, "\\") || path.IsAbs(logical) || path.Clean(logical) != logical {
		return "", "", migration.ErrPathRejected
	}
	for _, prefix := range []struct{ prefix, source string }{
		{"resources/avatars/", "avatar"},
		{"resources/backgrounds/", "background"},
		{"resources/tickets/", "tickets"},
		{"resources/server-icon/", "server-icon"},
		{"resources/auth-background/", "auth-background"},
		{"resources/bangumi/", "bangumi"},
	} {
		if strings.HasPrefix(logical, prefix.prefix) {
			rest := strings.TrimPrefix(logical, prefix.prefix)
			if rest == "" || rest == "." || strings.Contains(rest, "\\") {
				return "", "", migration.ErrPathRejected
			}
			return prefix.source, rest, nil
		}
	}
	return "", "", fmt.Errorf("unsupported migration resource %q", logical)
}

func (a *App) planMigrationResources(archive migration.Archive) (migrationResourcePlan, error) {
	plan := migrationResourcePlan{Entries: make([]migrationResourcePlanEntry, 0)}
	root := firstNonEmpty(a.cfg().UploadDir, "uploads")
	if err := validateMigrationUploadRoot(root); err != nil {
		return plan, err
	}
	for _, manifestFile := range archive.Manifest.Files {
		if manifestFile.Kind != "resource" {
			continue
		}
		sourceDir, relative, err := migrationResourcePathParts(manifestFile.Path)
		if err != nil {
			return plan, err
		}
		target, err := ResolveWithinRoot(root, filepath.Join(sourceDir, filepath.FromSlash(relative)))
		if err != nil {
			return plan, err
		}
		data := archive.Files[manifestFile.Path]
		entry := migrationResourcePlanEntry{LogicalPath: manifestFile.Path, TargetPath: target, Data: data}
		info, statErr := os.Lstat(target)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return plan, fmt.Errorf("resource target is not a regular file")
			}
			entry.Exists = true
			existing, readErr := os.ReadFile(target)
			if readErr != nil {
				return plan, readErr
			}
			entry.Same = bytes.Equal(existing, data)
			if !entry.Same {
				plan.Conflicts = append(plan.Conflicts, manifestFile.Path)
			}
		} else if !os.IsNotExist(statErr) {
			return plan, statErr
		}
		plan.Entries = append(plan.Entries, entry)
	}
	return plan, nil
}

func validateMigrationUploadRoot(root string) error {
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("upload root is not a real directory")
	}
	return nil
}

func (a *App) applyMigrationResources(plan migrationResourcePlan, replace bool) (func(), error) {
	if len(plan.Entries) == 0 {
		return func() {}, nil
	}
	root, err := filepath.Abs(firstNonEmpty(a.cfg().UploadDir, "uploads"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := validateMigrationUploadRoot(root); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(root), ".twilight-migration-")
	if err != nil {
		return nil, err
	}
	changes := make([]migrationResourceRollbackEntry, 0, len(plan.Entries))
	rollback := func() {
		for i := len(changes) - 1; i >= 0; i-- {
			change := changes[i]
			if change.existed {
				if data, readErr := os.ReadFile(change.backup); readErr == nil {
					_ = store.WriteFileAtomicSync(change.target, data, 0o600)
				}
			} else {
				_ = os.Remove(change.target)
			}
		}
		_ = os.RemoveAll(tmp)
	}
	for _, entry := range plan.Entries {
		if entry.Same {
			continue
		}
		if entry.Exists && !replace {
			rollback()
			return nil, errMigrationResourceConflict
		}
		if err := ensureMigrationParent(root, entry.TargetPath); err != nil {
			rollback()
			return nil, err
		}
		backup := ""
		if entry.Exists {
			backup = filepath.Join(tmp, fmt.Sprintf("backup-%d", len(changes)))
			if err := copyMigrationFile(entry.TargetPath, backup); err != nil {
				rollback()
				return nil, err
			}
		}
		if err := store.WriteFileAtomicSync(entry.TargetPath, entry.Data, 0o600); err != nil {
			rollback()
			return nil, err
		}
		changes = append(changes, migrationResourceRollbackEntry{target: entry.TargetPath, backup: backup, existed: entry.Exists})
	}
	return func() { _ = os.RemoveAll(tmp) }, nil
}

func ensureMigrationParent(root, target string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for current := dir; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("migration resource parent is not a real directory")
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current || !isWithinPath(root, parent) {
			return errors.New("migration resource parent escaped upload root")
		}
	}
	return nil
}

func isWithinPath(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func copyMigrationFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}

func (a *App) prepareMigrationConfig(archive migration.Archive) (string, bool, error) {
	content, ok := archive.Files["config/effective.toml"]
	if !ok {
		return "", false, nil
	}
	if len(content) == 0 {
		return "", false, errors.New("empty migration config")
	}
	tmp, err := os.CreateTemp("", ".twilight-migration-config-*.toml")
	if err != nil {
		return "", false, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		return "", false, err
	}
	imported, err := config.Load(name)
	if err != nil {
		return "", false, err
	}
	current := configValues(*a.cfg())
	source := configValues(imported)
	for section, fields := range source {
		if current[section] == nil {
			current[section] = map[string]any{}
		}
		for field, value := range fields {
			if !migrationConfigFieldPortable(section, field) {
				continue
			}
			if isSecretField(section, field) && value == secretMaskValue {
				continue
			}
			current[section][field] = value
		}
	}
	return renderConfigTOML(current), true, nil
}

func migrationConfigFieldPortable(section, field string) bool {
	switch section {
	case "Global":
		return field != "databases_dir"
	case "Database":
		return field == "migration_panel_enabled"
	case "API":
		return field == "max_upload_size"
	case "SystemUpdate":
		return false
	default:
		return true
	}
}

func readOptionalFile(filename string) ([]byte, bool) {
	data, err := os.ReadFile(filename)
	return data, err == nil
}

func rollbackMigrationConfig(a *App, data []byte, existed bool) {
	if existed {
		if err := store.WriteFileAtomicSync(a.configFilePath(), data, 0o600); err == nil {
			_, _ = a.reloadConfig()
		}
		return
	}
	_ = os.Remove(a.configFilePath())
	_, _ = a.reloadConfig()
}

func migrationArchiveSummary(archive migration.Archive, plan migrationResourcePlan, hasConfig bool) map[string]any {
	var total int64
	resources := 0
	for _, entry := range archive.Manifest.Files {
		total += entry.Size
		if entry.Kind == "resource" {
			resources++
		}
	}
	return map[string]any{
		"format_version":           archive.Manifest.FormatVersion,
		"twilight_version":         archive.Manifest.TwilightVersion,
		"database_schema_version":  archive.Manifest.DatabaseSchemaVersion,
		"exported_at":              archive.Manifest.ExportedAt,
		"encrypted":                archive.Manifest.Encrypted,
		"file_count":               len(archive.Manifest.Files),
		"total_uncompressed_bytes": total,
		"resource_count":           resources,
		"resource_conflict_count":  len(plan.Conflicts),
		"has_config":               hasConfig,
	}
}
