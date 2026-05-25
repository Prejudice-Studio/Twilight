package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	databaseRestoreConfirmPhrase = "RESTORE_DATABASE_BACKUP"
	databaseMigrateConfirmPhrase = "MIGRATE_DATABASE"
)

func (a *App) handleDatabaseStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, _ := store.ListBackups(a.cfg().DatabaseBackupDir)
	ok(w, "OK", map[string]any{
		"active_driver":     a.store().Backend(),
		"configured_driver": a.cfg().DatabaseDriver,
		"active_label":      databaseDriverLabel(a.store().Backend()),
		"configured_label":  databaseDriverLabel(a.cfg().DatabaseDriver),
		"supported_drivers": []map[string]string{
			{"driver": "postgres", "label": "postgresql", "role": "runtime"},
			{"driver": "json", "label": "gojson", "role": "runtime"},
		},
		"state_file":              a.cfg().StateFile,
		"backup_dir":              a.cfg().DatabaseBackupDir,
		"backup_count":            len(backups),
		"storage_mismatch":        a.runtimeDatabaseMismatch(),
		"storage_warning":         a.databaseMismatchWarning(),
		"migration_panel_enabled": a.cfg().DatabaseMigrationPanelEnabled,
		"postgres_configured":     a.cfg().PostgresDSN() != "",
		"redis_enabled":           a.redis() != nil,
		"user_count":              a.store().UserCount(),
		"legacy_sqlite_detected":  false,
	})
}

func databaseDriverLabel(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case store.BackendPostgres, "postgresql":
		return "postgresql"
	case store.BackendJSON, "file", "":
		return "gojson"
	default:
		return driver
	}
}

func (a *App) handleDatabaseBackups(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, err := store.ListBackups(a.cfg().DatabaseBackupDir)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBBackupListFailed, "读取数据库备份列表失败")
		return
	}
	ok(w, "OK", map[string]any{"backups": backups})
}

func (a *App) handleDatabaseBackupInspect(w http.ResponseWriter, r *http.Request, params Params) {
	name := strings.TrimSpace(params["name"])
	target, err := store.ResolveBackupPath(a.cfg().DatabaseBackupDir, name)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupReadFailed, "读取备份失败")
		return
	}
	var state store.State
	if err := json.Unmarshal(data, &state); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupSnapshotBad, "备份内容不是有效的 Twilight 状态快照")
		return
	}
	state.EnsureForMigration()
	info, err := databaseBackupInfo(target)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	counts := databaseStateCounts(state)
	ok(w, "OK", map[string]any{
		"backup":         info,
		"snapshot_bytes": len(data),
		"counts":         counts,
		"users":          counts["users"],
		"api_keys":       counts["api_keys"],
		"regcodes":       counts["regcodes"],
		"invite_codes":   counts["invite_codes"],
		"media_requests": counts["media_requests"],
		"announcements":  counts["announcements"],
	})
}

func (a *App) handleDatabaseBackupDelete(w http.ResponseWriter, r *http.Request, params Params) {
	name := strings.TrimSpace(params["name"])
	target, err := store.ResolveBackupPath(a.cfg().DatabaseBackupDir, name)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	info, err := databaseBackupInfo(target)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	if err := os.Remove(target); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBBackupDeleteFailed, "删除数据库备份失败")
		return
	}
	_ = os.Remove(store.BackupMetaPath(target))
	ok(w, "数据库备份已删除", map[string]any{"backup": info})
}

func (a *App) handleDatabaseBackup(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	note := firstNonEmpty(stringValue(payload, "note"), stringValue(payload, "remark"))
	info, err := a.store().BackupWithNote(a.cfg().DatabaseBackupDir, note)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBBackupCreateFailed, "数据库备份失败")
		return
	}
	ok(w, "数据库备份已创建", map[string]any{"backup": info})
}

func (a *App) handleDatabaseRestore(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	name := firstNonEmpty(stringValue(payload, "name"), stringValue(payload, "backup"))
	target, err := store.ResolveBackupPath(a.cfg().DatabaseBackupDir, name)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupReadFailed, "读取备份失败")
		return
	}
	var targetState store.State
	if err := json.Unmarshal(targetData, &targetState); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupSnapshotBad, "备份内容不是有效的 Twilight 状态快照")
		return
	}
	targetState.EnsureForMigration()
	currentSnapshot, err := a.store().Snapshot()
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBSnapshotFailed, "生成当前数据库快照失败")
		return
	}
	var currentState store.State
	if err := json.Unmarshal(currentSnapshot, &currentState); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBSnapshotVerifyBad, "当前数据库快照校验失败")
		return
	}
	currentState.EnsureForMigration()
	backupInfo, err := databaseBackupInfo(target)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBBackupInvalid, "备份文件无效")
		return
	}
	result := map[string]any{
		"operation":              "restore",
		"dry_run":                true,
		"requires_confirmation":  true,
		"confirm":                databaseRestoreConfirmPhrase,
		"restored":               filepath.Base(target),
		"backup":                 backupInfo,
		"target_snapshot_bytes":  len(targetData),
		"current_snapshot_bytes": len(currentSnapshot),
		"counts":                 databaseStateCounts(targetState),
		"current_counts":         databaseStateCounts(currentState),
		"users":                  len(targetState.Users),
		"api_keys":               len(targetState.APIKeys),
		"regcodes":               len(targetState.RegCodes),
		"invite_codes":           len(targetState.InviteCodes),
		"media_requests":         len(targetState.MediaRequests),
		"announcements":          len(targetState.Announcements),
		"warnings": []string{
			"restore will replace the active database state",
			"the server will create a protective backup before applying this restore",
		},
	}
	if boolValue(payload, "dry_run", false) || boolValue(payload, "preview", false) || stringValue(payload, "confirm") != databaseRestoreConfirmPhrase {
		ok(w, "恢复预览已生成", result)
		return
	}
	preRestore, backupErr := a.store().BackupWithNote(a.cfg().DatabaseBackupDir, "数据库恢复前保护性备份")
	if backupErr != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBRestoreBackupFail, "恢复前备份失败")
		return
	}
	if err := a.store().LoadSnapshot(targetData); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrDBRestoreFailed, "备份恢复失败")
		return
	}
	result["dry_run"] = false
	result["requires_confirmation"] = false
	result["pre_restore_backup"] = preRestore
	result["pre_operation_backup"] = preRestore
	ok(w, "数据库已恢复", result)
}

func (a *App) handleDatabaseMigrate(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().DatabaseMigrationPanelEnabled {
		failWithCode(w, http.StatusForbidden, ErrDBMigrationDisabled, "数据库迁移功能未开启，请先在配置文件中启用 Database.migration_panel_enabled")
		return
	}
	payload := decodeMap(r)
	sourceDriver := strings.ToLower(firstNonEmpty(stringValue(payload, "source_driver"), stringValue(payload, "source"), a.store().Backend()))
	if sourceDriver == "sqlite" || sourceDriver == "legacy_sqlite" || sourceDriver == "legacy-sqlite" {
		failWithCode(w, http.StatusForbidden, ErrDBSQLiteDisabled, "SQLite 数据源已禁用；请使用当前运行状态或 PostgreSQL")
		return
	}
	defer runtime.GC()
	targetDriver := strings.ToLower(firstNonEmpty(stringValue(payload, "target_driver"), stringValue(payload, "driver"), a.cfg().DatabaseDriver))
	if targetDriver == "" {
		targetDriver = store.BackendJSON
	}
	confirmed := stringValue(payload, "confirm") == databaseMigrateConfirmPhrase
	dryRun := boolValue(payload, "dry_run", false) || boolValue(payload, "preview", false) || !confirmed
	snapshot, err := a.store().Snapshot()
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBSnapshotFailed, "生成迁移快照失败")
		return
	}
	snapshotBytes := len(snapshot)
	var state store.State
	if err := json.Unmarshal(snapshot, &state); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBSnapshotVerifyBad, "迁移快照校验失败")
		return
	}
	state.EnsureForMigration()

	switch targetDriver {
	case store.BackendPostgres, "postgresql":
		targetDriver = store.BackendPostgres
		dsn := firstNonEmpty(stringValue(payload, "database_url"), stringValue(payload, "postgres_dsn"), a.cfg().PostgresDSN())
		if dsn == "" {
			failWithCode(w, http.StatusBadRequest, ErrDBPostgresMissing, "未配置 PostgreSQL 连接信息")
			return
		}
		targetReady := map[string]any{"driver": targetDriver, "configured": true, "connected": false, "schema_ready": false}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if dryRun {
			status, err := store.CheckPostgresTarget(ctx, dsn)
			if err != nil {
				failWithCode(w, http.StatusBadRequest, ErrDBPostgresConnect, databasePostgresErrorMessage("连接 PostgreSQL 失败", err))
				return
			}
			targetReady = postgresTargetReadyMap(targetDriver, status)
			ok(w, "迁移预检通过", a.databaseMigrationSummary(targetDriver, state, dryRun, snapshotBytes, targetReady))
			return
		}
		preMigration, backupErr := a.store().BackupWithNote(a.cfg().DatabaseBackupDir, "数据库迁移前保护性备份")
		if backupErr != nil {
			failWithCode(w, http.StatusInternalServerError, ErrDBRestoreBackupFail, "迁移前备份失败")
			return
		}
		targetStore, err := store.OpenPostgres(ctx, dsn)
		if err != nil {
			failWithCode(w, http.StatusBadRequest, ErrDBPostgresConnect, databasePostgresErrorMessage("连接 PostgreSQL 失败", err))
			return
		}
		defer targetStore.Close()
		targetStore.ConfigurePostgres(a.cfg().PostgresMaxOpenConns, a.cfg().PostgresMaxIdleConns)
		targetReady["connected"] = true
		targetReady["schema_ready"] = true
		if err := targetStore.LoadSnapshot(snapshot); err != nil {
			failWithCode(w, http.StatusInternalServerError, ErrDBPostgresWriteFail, "写入 PostgreSQL 失败")
			return
		}
		summary := a.databaseMigrationSummary(targetDriver, state, dryRun, snapshotBytes, targetReady)
		summary["pre_migration_backup"] = preMigration
		summary["pre_operation_backup"] = preMigration
		ok(w, "数据库已迁移到 PostgreSQL", summary)
	case store.BackendJSON, "file":
		targetDriver = store.BackendJSON
		targetPath := strings.TrimSpace(stringValue(payload, "state_file"))
		if targetPath == "" {
			targetPath = a.cfg().StateFile
		} else {
			targetPath, err = resolveStateFileTarget(a.cfg().DatabaseDir, targetPath)
			if err != nil {
				failWithCode(w, http.StatusBadRequest, ErrDBStateFileBadPath, "目标状态文件路径无效")
				return
			}
		}
		targetReady := map[string]any{"driver": targetDriver, "configured": targetPath != "", "path": targetPath, "parent_dir": filepath.Dir(targetPath)}
		if dryRun {
			summary := a.databaseMigrationSummary(store.BackendJSON, state, dryRun, snapshotBytes, targetReady)
			summary["state_file"] = targetPath
			ok(w, "迁移预检通过", summary)
			return
		}
		preMigration, backupErr := a.store().BackupWithNote(a.cfg().DatabaseBackupDir, "数据库迁移前保护性备份")
		if backupErr != nil {
			failWithCode(w, http.StatusInternalServerError, ErrDBRestoreBackupFail, "迁移前备份失败")
			return
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			failWithCode(w, http.StatusInternalServerError, ErrDBStateFileMkdirBad, "创建数据库目录失败")
			return
		}
		// 走 store.WriteFileAtomicSync：tmp + fsync(file) + rename + fsync(dir)，
		// 与 saveLocked / BackupWithNote 共用一份原子写盘语义，掉电也不会留下
		// 半截 state 文件让前端显示"迁移成功"但磁盘上根本没有内容。
		if err := store.WriteFileAtomicSync(targetPath, snapshot, 0o600); err != nil {
			failWithCode(w, http.StatusInternalServerError, ErrDBStateFileWriteBad, "写入状态文件失败")
			return
		}
		summary := a.databaseMigrationSummary(store.BackendJSON, state, dryRun, snapshotBytes, targetReady)
		summary["state_file"] = targetPath
		summary["pre_migration_backup"] = preMigration
		summary["pre_operation_backup"] = preMigration
		ok(w, "数据库已迁移到 JSON 状态文件", summary)
	default:
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "不支持的数据库目标")
	}
}

func databasePostgresErrorMessage(prefix string, err error) string {
	// lib/pq 原始错误会带 host/port/dbname/user/SQLSTATE 等连接拓扑信息，
	// 直接拼到响应里给前端会让任何拿到 admin token 的攻击者通过 422 文案
	// 反推出 PG 部署细节。这里只回固定语义错，原文走 zap 仅落本地日志。
	if err != nil {
		zap.L().Warn("postgres connect", zap.String("stage", prefix), zap.Error(err))
	}
	return prefix + "，请检查数据库连接配置"
}

func postgresTargetReadyMap(driver string, status store.PostgresTargetStatus) map[string]any {
	return map[string]any{
		"driver":           driver,
		"configured":       true,
		"connected":        status.Connected,
		"schema_ready":     status.SchemaReady,
		"database_created": status.DatabaseCreated,
		"host":             status.Host,
		"user":             status.User,
		"database":         status.Database,
	}
}

func (a *App) databaseMigrationSummary(driver string, state store.State, dryRun bool, snapshotBytes int, targetReady map[string]any) map[string]any {
	counts := databaseStateCounts(state)
	warnings := []string{}
	if a.store().Backend() != driver {
		warnings = append(warnings, "active database backend will not change until the service restarts with the target driver")
	}
	if strings.ToLower(a.cfg().DatabaseDriver) != driver {
		warnings = append(warnings, "configured database.driver differs from migration target; update config before restart")
	}
	return map[string]any{
		"source_driver":         a.store().Backend(),
		"configured_driver":     strings.ToLower(a.cfg().DatabaseDriver),
		"target_driver":         driver,
		"dry_run":               dryRun,
		"operation":             "migrate",
		"requires_confirmation": dryRun,
		"confirm":               databaseMigrateConfirmPhrase,
		"snapshot_bytes":        snapshotBytes,
		"source_ready": map[string]any{
			"driver":   a.store().Backend(),
			"snapshot": true,
			"counts":   counts,
		},
		"target_ready": targetReady,
		"backup_ready": map[string]any{
			"automatic":     true,
			"current_state": true,
			"legacy_sqlite": false,
			"backup_dir":    a.cfg().DatabaseBackupDir,
		},
		"warnings":       warnings,
		"counts":         counts,
		"users":          counts["users"],
		"api_keys":       counts["api_keys"],
		"regcodes":       counts["regcodes"],
		"invite_codes":   counts["invite_codes"],
		"media_requests": counts["media_requests"],
		"announcements":  counts["announcements"],
	}
}

func databaseStateCounts(state store.State) map[string]int {
	return map[string]int{
		"users":               len(state.Users),
		"api_keys":            len(state.APIKeys),
		"regcodes":            len(state.RegCodes),
		"invite_codes":        len(state.InviteCodes),
		"invite_relations":    len(state.InviteRelations),
		"media_requests":      len(state.MediaRequests),
		"announcements":       len(state.Announcements),
		"bind_codes":          len(state.BindCodes),
		"signin":              len(state.Signin),
		"scheduler_runs":      len(state.SchedulerRuns),
		"scheduler_schedules": len(state.SchedulerSchedules),
		"devices":             len(state.Devices),
		"login_logs":          len(state.LoginLogs),
		"ip_blacklist":        len(state.IPBlacklist),
		"playback_records":    len(state.PlaybackRecords),
		"rebind_requests":     len(state.RebindRequests),
		"telegram_roster":     len(state.TelegramRoster),
	}
}

func databaseBackupInfo(path string) (store.BackupInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return store.BackupInfo{}, err
	}
	if !info.Mode().IsRegular() {
		return store.BackupInfo{}, store.ErrNotFound
	}
	return store.BackupInfo{
		Name:      filepath.Base(path),
		Path:      path,
		Size:      info.Size(),
		CreatedAt: info.ModTime().Unix(),
		Note:      store.ReadBackupNote(path),
	}, nil
}

func resolveStateFileTarget(databaseDir, target string) (string, error) {
	joined, err := ResolveWithinRoot(firstNonEmpty(databaseDir, "db"), target)
	if err != nil {
		return "", store.ErrNotFound
	}
	if strings.ToLower(filepath.Ext(joined)) != ".json" {
		return "", store.ErrNotFound
	}
	return joined, nil
}
