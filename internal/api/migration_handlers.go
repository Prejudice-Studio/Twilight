package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
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
	"data/state.json":                     {},
	"data/runtime-logs.json":              {},
	"data/audit-logs.json":                {},
	"data/telegram-roster.json":           {},
	"data/telegram-runtime.json":          {},
	"data/playback-records.json":          {},
	"data/trusted-playback-events.json":   {},
	"data/trusted-playback-segments.json": {},
	"data/trusted-playback-daily.json":    {},
}

var migrationRequiredDataFileNames = map[string]struct{}{
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
	// Skipped 是文件名不符合该命名空间白名单、因此不会写盘的资源逻辑路径。
	Skipped []string
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
	archiveFile, archiveSize, options, err := readMigrationMultipart(r)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "迁移包上传无效")
		return
	}
	defer archiveFile.Close()
	if options.ResourceMode == "" {
		options.ResourceMode = migrationResourceModePreserve
	}
	if options.ResourceMode != migrationResourceModePreserve && options.ResourceMode != migrationResourceModeReplace {
		failWithCode(w, http.StatusBadRequest, ErrMigrationUploadBad, "资源覆盖模式无效")
		return
	}

	a.migrationMu.Lock()
	defer a.migrationMu.Unlock()
	archive, err := migration.OpenReaderAt(archiveFile, archiveSize, options.Password, migration.DefaultLimits())
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
	_, hasConfig, err := a.prepareMigrationConfig(archive)
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
	if len(plan.Skipped) > 0 {
		summary["resource_skipped"] = plan.Skipped
		summary["resource_skipped_count"] = len(plan.Skipped)
	}
	if options.Preview || options.Confirm != migrationImportConfirmPhrase {
		summary["dry_run"] = true
		summary["requires_confirmation"] = true
		skipAuditForDryRun(r)
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

	status, message := a.applyMigrationImport(r, archive, plan, options)
	if status != http.StatusOK {
		failWithCode(w, status, ErrMigrationImportFail, message)
		return
	}
	summary["dry_run"] = false
	summary["requires_confirmation"] = false
	summary["config_applied"] = options.ApplyConfig
	summary["resources_written"] = len(plan.Entries)
	a.audit(r, "import_migration_archive", "admin", 0, map[string]any{
		"files": len(archive.Files), "resources": len(plan.Entries), "resources_skipped": len(plan.Skipped), "config_applied": options.ApplyConfig,
	})
	ok(w, "迁移数据已导入", summary)
}

// Keep configuration application and failure rollback in one serialized scope.
// A resource/database failure must never roll back another editor's newer file.
func (a *App) applyMigrationImport(r *http.Request, archive migration.Archive, plan migrationResourcePlan, options migrationImportOptions) (int, string) {
	status, message := http.StatusInternalServerError, "迁移导入失败"
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	err := config.WithWriteLock(ctx, a.configFilePath(), func() error {
		a.runtimeMu.Lock()
		defer a.runtimeMu.Unlock()
		var previous, candidate []byte
		var existed bool
		if options.ApplyConfig {
			snapshot, err := a.configEditSnapshot()
			if err != nil {
				message = "读取当前配置失败"
				return nil
			}
			previous = []byte(snapshot.content)
			_, statErr := os.Stat(a.configFilePath())
			existed = statErr == nil
			content, _, err := a.prepareMigrationConfigFromSnapshot(archive, snapshot)
			if err != nil {
				message = "迁移配置校验失败"
				return nil
			}
			_, status, message = a.saveConfigContentLocked(content, snapshot.revision)
			if status != http.StatusOK {
				return nil
			}
			candidate, err = os.ReadFile(a.configFilePath())
			if err != nil {
				status, message = http.StatusInternalServerError, "读取已应用配置失败，请检查配置文件"
				return nil
			}
		}
		rollbackConfig := func() {
			if !options.ApplyConfig {
				return
			}
			if err := rollbackConfigCandidate(a.configFilePath(), string(candidate), previous, existed); err != nil {
				message += "；配置回滚失败，请检查配置文件"
				return
			}
			if _, err := a.reloadConfigLocked(); err != nil {
				message += "；配置回滚后重载失败"
			}
		}
		rollbackResources, commitResources, err := a.stageMigrationResources(plan, options.ResourceMode == migrationResourceModeReplace)
		if err != nil {
			status, message = http.StatusInternalServerError, "迁移资源写入失败"
			rollbackConfig()
			return nil
		}
		a.sessions().DeleteAll(r.Context())
		if _, err := a.store().ImportMigrationArchive(r.Context(), archive, a.cfg().TwoFactorKey); err != nil {
			rollbackResources()
			status, message = http.StatusInternalServerError, "迁移数据库导入失败"
			rollbackConfig()
			return nil
		}
		commitResources()
		status, message = http.StatusOK, ""
		return nil
	})
	if err != nil {
		return http.StatusServiceUnavailable, "迁移配置锁暂不可用，请稍后重试"
	}
	return status, message
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

// readMigrationMultipart 返回上传的封包文件句柄而不是整份字节：ParseMultipartForm
// 超过 migrationMultipartMemory 的部分已落到临时文件，multipart.File 实现了
// io.ReaderAt，交给 migration.OpenReaderAt 直接读取，避免再复制一份到内存。
// 调用方负责 Close；临时文件由 net/http 在请求结束后清理。
func readMigrationMultipart(r *http.Request) (multipart.File, int64, migrationImportOptions, error) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data;") {
		return nil, 0, migrationImportOptions{}, errors.New("multipart form required")
	}
	if err := r.ParseMultipartForm(migrationMultipartMemory); err != nil {
		return nil, 0, migrationImportOptions{}, err
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		return nil, 0, migrationImportOptions{}, err
	}
	if header.Size <= 0 || header.Size > migration.MaxArchiveBytes {
		_ = file.Close()
		return nil, 0, migrationImportOptions{}, migration.ErrArchiveLimit
	}
	options := migrationImportOptions{
		Password:     r.FormValue("password"),
		Confirm:      strings.TrimSpace(r.FormValue("confirm")),
		Preview:      boolFormValue(r.FormValue("preview")) || boolFormValue(r.FormValue("dry_run")),
		ApplyConfig:  boolFormValue(r.FormValue("apply_config")),
		ResourceMode: strings.ToLower(strings.TrimSpace(r.FormValue("resource_mode"))),
	}
	return file, header.Size, options, nil
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
	for path := range migrationRequiredDataFileNames {
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

var (
	migrationAuthBackgroundNamePattern = regexp.MustCompile(`^background\.(jpg|png|gif|webp|bmp)$`)
	migrationBangumiCoverNamePattern   = regexp.MustCompile(`^[0-9]+\.(jpg|png|gif|webp|bmp)$`)
	migrationTicketImageNamePattern    = regexp.MustCompile(`^[0-9]+/[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$`)
)

// migrationResourceNameAllowed 按命名空间校验导入资源的相对路径：
//   - avatar / background / server-icon：随机 16 hex + 图片扩展名（uploadFilenamePattern）；
//   - auth-background：background.<图片扩展名>，或旧版随机 hex 文件名；
//   - tickets：<工单 ID>/<随机 16 hex>.<图片扩展名>；
//   - bangumi：<条目 ID>.<图片扩展名>。
func migrationResourceNameAllowed(source, relative string) bool {
	switch source {
	case "avatar", "background", "server-icon":
		return uploadFilenamePattern.MatchString(relative)
	case "auth-background":
		return migrationAuthBackgroundNamePattern.MatchString(relative) || uploadFilenamePattern.MatchString(relative)
	case "tickets":
		return migrationTicketImageNamePattern.MatchString(relative)
	case "bangumi":
		return migrationBangumiCoverNamePattern.MatchString(relative)
	default:
		return false
	}
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
		// 资源文件名必须符合本系统各命名空间真实会产生的格式，否则不写盘：
		// 例如 auth-background 目录下的 zzz.html 会被公开背景端点当成背景回传。
		if !migrationResourceNameAllowed(sourceDir, relative) {
			plan.Skipped = append(plan.Skipped, manifestFile.Path)
			continue
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
	_, commit, err := a.stageMigrationResources(plan, replace)
	return commit, err
}

func (a *App) stageMigrationResources(plan migrationResourcePlan, replace bool) (func(), func(), error) {
	if len(plan.Entries) == 0 {
		return func() {}, func() {}, nil
	}
	root, err := filepath.Abs(firstNonEmpty(a.cfg().UploadDir, "uploads"))
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, nil, err
	}
	if err := validateMigrationUploadRoot(root); err != nil {
		return nil, nil, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(root), ".twilight-migration-")
	if err != nil {
		return nil, nil, err
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
			return nil, nil, errMigrationResourceConflict
		}
		if err := ensureMigrationParent(root, entry.TargetPath); err != nil {
			rollback()
			return nil, nil, err
		}
		backup := ""
		if entry.Exists {
			backup = filepath.Join(tmp, fmt.Sprintf("backup-%d", len(changes)))
			if err := copyMigrationFile(entry.TargetPath, backup); err != nil {
				rollback()
				return nil, nil, err
			}
		}
		if err := store.WriteFileAtomicSync(entry.TargetPath, entry.Data, 0o600); err != nil {
			rollback()
			return nil, nil, err
		}
		changes = append(changes, migrationResourceRollbackEntry{target: entry.TargetPath, backup: backup, existed: entry.Exists})
	}
	return rollback, func() { _ = os.RemoveAll(tmp) }, nil
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
	if _, ok := archive.Files["config/effective.toml"]; !ok {
		return "", false, nil
	}
	snapshot, err := a.configEditSnapshot()
	if err != nil {
		return "", false, err
	}
	return a.prepareMigrationConfigFromSnapshot(archive, snapshot)
}

func (a *App) prepareMigrationConfigFromSnapshot(archive migration.Archive, snapshot configEditSnapshot) (string, bool, error) {
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
	imported, err := config.LoadFileOnly(name)
	if err != nil {
		return "", false, err
	}
	current := configValues(snapshot.file)
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
	merged, err := mergeConfigTOML(snapshot.content, current)
	return merged, true, err
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
		"two_factor_notice":        "Import replaces two-factor settings. Backups without two-factor data clear those settings. The matching deployment encryption key is required.",
	}
}
