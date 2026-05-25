package api

// 媒体库（Library）自助域 handler。从 handlers.go 抽出来的目的：
//   - handlers.go 长期聚合 9+ 业务域 2000+ 行；媒体库相关端点散在三处
//     （handleLibraries / handleBulkEnableLibrarySelfService /
//     handleAdminLibrarySelfService），新人接手时无法快速定位"媒体库自
//     助 / 批量开关"在哪条链路；
//   - 三个 handler 共享 `LibrarySelfService` user 字段、Emby 远端库读写以
//     及"非 admin 走 EmbySelfServiceLibraries 白名单"的安全约束，集中到
//     一处后单测可针对自助开关 / 白名单校验 / 批量幂等性 写而不必跨整
//     个 handlers 文件；
//   - handleLibraries 同时承担 `/emby/libraries`（全局列表）/ `/users/:uid/
//     libraries`（GET 当前 / PUT 修改）两条路由分支，保留原有路径嗅探不
//     改语义，避免拆函数后路由注册被迫改动。
//
// 修改时务必保持与原有契约一致：
//   - 非 admin 用户必须先经 requireNonEmbyAdmin 拦截 Emby 管理员账号；
//   - 自助库白名单走 EmbySelfServiceLibraries（不区分大小写），调用前必须
//     normalizeLibraryNames；
//   - failWithCode 走 errcode.go，不在这里临时新增。

import (
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleLibraries(w http.ResponseWriter, r *http.Request, params Params) {
	// Route: list all Emby libraries (no user context needed)
	if strings.Contains(r.URL.Path, "/emby/libraries") || strings.Contains(r.URL.Path, "/admin/emby/libraries") {
		remoteLibraries, err := a.embyLibraries(r.Context())
		if err != nil {
			failWithCode(w, http.StatusBadGateway, ErrEmbyConnectFailed, "无法连接 Emby 服务器，请检查 Emby 是否在线")
			return
		}
		ok(w, "OK", remoteLibraries)
		return
	}

	// Pre-check: Emby must be configured
	if a.cfg().EmbyURL == "" {
		failWithCode(w, http.StatusServiceUnavailable, ErrEmbyNotConfigured, "Emby 未配置，请先在系统配置中填写 Emby 服务地址")
		return
	}

	// Resolve target user: admin routes pass :uid, user routes use session
	targetUser := current(r).User
	if params["uid"] != "" {
		uid, _ := int64Param(params, "uid")
		if u, okUser := a.store().User(uid); okUser {
			targetUser = u
		} else {
			failWithCode(w, http.StatusNotFound, ErrUserNotFound, "用户不存在")
			return
		}
	}

	// PUT: modify library visibility
	if r.Method == http.MethodPut {
		if targetUser.EmbyID == "" {
			failWithCode(w, http.StatusBadRequest, ErrEmbyAccountUnlinked, "目标用户未关联 Emby 账号")
			return
		}
		body := decodeMap(r)
		action := firstNonEmpty(stringValue(body, "action"), "set")
		names := normalizeLibraryNames(stringSlice(body["library_names"]))
		ids := stringSlice(body["library_ids"])

		// Non-admin users: enforce self-service restrictions
		if current(r).User.Role != store.RoleAdmin {
			// Security: block non-admin users with Emby admin accounts
			if a.requireNonEmbyAdmin(w, r, current(r).User) {
				return
			}
			if !targetUser.LibrarySelfService {
				failWithCode(w, http.StatusForbidden, ErrLibrarySelfServiceDisabled, "library self-service is not enabled")
				return
			}
			if action != "show" && action != "hide" {
				failWithCode(w, http.StatusForbidden, ErrLibrarySelfServiceAction, "unsupported self-service action")
				return
			}
			allowed := map[string]bool{}
			for _, name := range normalizeLibraryNames(a.cfg().EmbySelfServiceLibraries) {
				allowed[strings.ToLower(name)] = true
			}
			for _, name := range names {
				if !allowed[strings.ToLower(name)] {
					failWithCode(w, http.StatusForbidden, ErrLibraryNotSelfService, "library is not self-service enabled")
					return
				}
			}
		}
		if err := a.embySetLibrariesByAction(r.Context(), targetUser, action, ids, names, boolValue(body, "enable_all", false)); err != nil {
			failWithCode(w, http.StatusBadGateway, ErrEmbyConnectFailed, err.Error())
			return
		}
	}

	// GET or after successful PUT: return current library access state
	ok(w, "OK", a.embyLibraryAccess(r.Context(), targetUser, current(r).User.Role == store.RoleAdmin))
}

func (a *App) handleBulkEnableLibrarySelfService(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != "ENABLE_LIBRARY_SELF_SERVICE" {
		failWithCode(w, http.StatusBadRequest, ErrAdminBulkLibraryConfirm, "缺少确认标记")
		return
	}
	updated := 0
	for _, u := range a.store().ListUsers() {
		if u.Role == store.RoleUnrecognized {
			continue
		}
		if _, err := a.store().UpdateUser(u.UID, func(u *store.User) error { u.LibrarySelfService = true; return nil }); err == nil {
			updated++
		}
	}
	ok(w, "batch enabled", map[string]any{"updated": updated})
}

func (a *App) handleAdminLibrarySelfService(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	enabled := boolValue(decodeMap(r), "enabled", true)
	u, err := a.store().UpdateUser(uid, func(u *store.User) error { u.LibrarySelfService = enabled; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "updated", map[string]any{"uid": uid, "library_self_service": u.LibrarySelfService})
}
