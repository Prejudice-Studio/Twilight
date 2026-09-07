package api

import (
	"net/http"

	"github.com/prejudice-studio/twilight/internal/store"
)

func v2DatabaseBackupDTO(info store.BackupInfo) map[string]any {
	item := map[string]any{
		"name":       info.Name,
		"size":       info.Size,
		"created_at": info.CreatedAt,
	}
	if info.Note != "" {
		item["note"] = info.Note
	}
	return item
}

func (a *App) handleV2DatabaseStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, _ := store.ListBackups(a.cfg().DatabaseBackupDir)
	ok(w, "OK", map[string]any{
		"active_driver":           a.store().Backend(),
		"configured_driver":       a.cfg().DatabaseDriver,
		"active_label":            databaseDriverLabel(a.store().Backend()),
		"configured_label":        databaseDriverLabel(a.cfg().DatabaseDriver),
		"supported_drivers":       []map[string]string{{"driver": "postgres", "label": "postgresql", "role": "runtime"}, {"driver": "json", "label": "gojson", "role": "export"}},
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

func (a *App) handleV2DatabaseBackups(w http.ResponseWriter, r *http.Request, _ Params) {
	backups, err := store.ListBackups(a.cfg().DatabaseBackupDir)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrDBBackupListFailed, "读取数据库备份列表失败")
		return
	}
	items := make([]map[string]any, 0, len(backups))
	for _, backup := range backups {
		items = append(items, v2DatabaseBackupDTO(backup))
	}
	ok(w, "OK", map[string]any{"backups": items})
}

var v2DatabasePrivateFields = map[string]struct{}{
	"path": {}, "parent_dir": {}, "backup_dir": {}, "backup_path": {}, "state_file": {},
	"host": {}, "user": {}, "database": {}, "database_url": {}, "dsn": {}, "postgres_dsn": {},
}

func (a *App) delegateV2DatabaseHandler(w http.ResponseWriter, r *http.Request, p Params, handler func(http.ResponseWriter, *http.Request, Params)) {
	a.delegateV2SafeHandler(w, r, p, handler, v2DatabasePrivateFields)
}

func (a *App) handleV2DatabaseBackupInspect(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2DatabaseHandler(w, r, p, a.handleDatabaseBackupInspect)
}

func (a *App) handleV2DatabaseBackupDelete(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2DatabaseHandler(w, r, p, a.handleDatabaseBackupDelete)
}

func (a *App) handleV2DatabaseBackup(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2DatabaseHandler(w, r, p, a.handleDatabaseBackup)
}

func (a *App) handleV2DatabaseRestore(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2DatabaseHandler(w, r, p, a.handleDatabaseRestore)
}

func (a *App) handleV2DatabaseMigrate(w http.ResponseWriter, r *http.Request, p Params) {
	a.delegateV2DatabaseHandler(w, r, p, a.handleDatabaseMigrate)
}
