package api

import (
	"context"
	"net/http"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
)

const regcodeStorageMismatchMessage = "当前运行数据库与配置数据库不一致，注册码写入已暂停。请确认 database.driver 为 postgres 且 PostgreSQL 连接配置正确后重启服务。"

type requestStoreRefreshKey struct{}

type requestStoreRefreshState struct {
	completed bool
}

func withRequestStoreRefreshState(r *http.Request) *http.Request {
	if r == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), requestStoreRefreshKey{}, &requestStoreRefreshState{}))
}

// runtimeDatabaseMismatch keeps a final runtime guard for a configuration that
// no longer points at the mandatory PostgreSQL backend.
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

// refreshStoreForRequest makes the PostgreSQL snapshot current once for every
// routed HTTP request. This covers authentication and public read routes as
// well as handlers, while the request-local marker keeps legacy handler guards
// from issuing a second version probe.
func (a *App) refreshStoreForRequest(w http.ResponseWriter, r *http.Request) bool {
	if a == nil || a.store() == nil {
		return false
	}
	var state *requestStoreRefreshState
	if r != nil {
		state, _ = r.Context().Value(requestStoreRefreshKey{}).(*requestStoreRefreshState)
	}
	if state != nil && state.completed {
		return false
	}
	if err := a.store().Refresh(); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "读取最新数据库状态失败")
		return true
	}
	if state != nil {
		state.completed = true
	}
	return false
}

func (a *App) refreshCurrentUserForRequest(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	if a.refreshStoreForRequest(w, r) {
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
