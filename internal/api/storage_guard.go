package api

import (
	"net/http"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const regcodeStorageMismatchMessage = "当前运行数据库与配置数据库不一致，注册码写入已暂停。请确认 database.driver 为 postgres 且 PostgreSQL 连接配置正确后重启服务。"

// runtimeDatabaseMismatch 判定运行期后端与配置后端是否不一致。后端已收敛为
// 单一 PostgreSQL：运行期 store 恒为 postgres，唯一的失配场景是配置里的
// driver 被改成了非 postgres 值（此时启动本应被拒，这里作为最后一道运行期
// 保险，避免误判把注册码写进错误后端）。不再比对 JSON 状态文件路径。
func (a *App) runtimeDatabaseMismatch() bool {
	if a == nil || a.store() == nil {
		return false
	}
	return !config.IsPostgresDriver(a.cfg().DatabaseDriver)
}

func (a *App) rejectRegcodeWriteIfStorageMismatch(w http.ResponseWriter) bool {
	if !a.runtimeDatabaseMismatch() {
		return false
	}
	failWithCode(w, http.StatusConflict, ErrRegcodeStorageMismatch, regcodeStorageMismatchMessage)
	return true
}

func (a *App) refreshStoreForRequest(w http.ResponseWriter) bool {
	if a == nil || a.store() == nil {
		return false
	}
	if err := a.store().Refresh(); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "读取最新数据库状态失败")
		return true
	}
	return false
}

func (a *App) refreshCurrentUserForRequest(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	if a.refreshStoreForRequest(w) {
		return store.User{}, false
	}
	p := current(r)
	if p.User.UID == 0 {
		return store.User{}, true
	}
	u, ok := a.store().User(p.User.UID)
	if !ok || !u.Active {
		failWithCode(w, http.StatusUnauthorized, ErrUnauthorized, "登录状态已失效，请重新登录")
		return store.User{}, false
	}
	return u, true
}

func (a *App) databaseMismatchWarning() string {
	if !a.runtimeDatabaseMismatch() {
		return ""
	}
	return regcodeStorageMismatchMessage
}
