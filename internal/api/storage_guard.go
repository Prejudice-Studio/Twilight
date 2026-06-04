package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

const regcodeStorageMismatchMessage = "当前运行数据库与配置数据库或状态文件不一致，注册码写入已暂停。请先在数据库迁移页完成迁移并重启，确认当前运行后端与配置后端一致后再生成或使用注册码。"

func normalizedRuntimeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", store.BackendJSON, "file":
		return store.BackendJSON
	case store.BackendPostgres, "postgresql":
		return store.BackendPostgres
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

func (a *App) runtimeDatabaseMismatch() bool {
	if a == nil || a.store() == nil {
		return false
	}
	configuredDriver := normalizedRuntimeDriver(a.cfg().DatabaseDriver)
	runtimeDriver := normalizedRuntimeDriver(a.store().Backend())
	if configuredDriver != runtimeDriver {
		return true
	}
	if configuredDriver == store.BackendJSON {
		return normalizedRuntimeStateFile(a.cfg().StateFile) != normalizedRuntimeStateFile(a.store().Path())
	}
	return false
}

func normalizedRuntimeStateFile(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = filepath.Join("db", "twilight_go_state.json")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func (a *App) rejectRegcodeWriteIfStorageMismatch(w http.ResponseWriter) bool {
	if !a.runtimeDatabaseMismatch() {
		return false
	}
	failWithCode(w, http.StatusConflict, ErrRegcodeStorageMismatch, regcodeStorageMismatchMessage)
	return true
}

func (a *App) databaseMismatchWarning() string {
	if !a.runtimeDatabaseMismatch() {
		return ""
	}
	return regcodeStorageMismatchMessage
}
