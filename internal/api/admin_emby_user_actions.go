package api

import (
	"context"
	"net/http"
	"strings"
)

// kickEmbySessions 踢出某 Emby 用户当前的全部在线会话（逐个 /Sessions/{id}/Logout），
// 返回成功登出的会话数。Emby 未配置或 embyID 为空时返回 0。供按 uid / 按 emby_id 两条
// 踢人入口共用，避免重复遍历 /Sessions 的逻辑。
func (a *App) kickEmbySessions(ctx context.Context, embyID string) int {
	if !a.embyConfigured() || strings.TrimSpace(embyID) == "" {
		return 0
	}
	var sessions []map[string]any
	if err := a.embyGet(ctx, "/Sessions", &sessions); err != nil {
		return 0
	}
	kicked := 0
	for _, session := range sessions {
		if asString(session["UserId"]) != embyID {
			continue
		}
		if sid := asString(session["Id"]); sid != "" {
			var ignored map[string]any
			if err := a.embyPost(ctx, "/Sessions/"+urlPathEscape(sid)+"/Logout", nil, &ignored); err == nil {
				kicked++
			}
		}
	}
	return kicked
}

// handleAdminEmbyUserToggle 按 Emby 用户 ID 单独启停 Emby 账号，服务设备/IP 审查页对
// 用户的快速处置（含未关联本地账号的 Emby 账号）。守卫：
//   - Emby 未配置 → 拒绝；远端账号不存在 → 404；
//   - 远端是 Emby 管理员 → 拒绝（避免锁死服务器管理员）；
//   - 若该 Emby 已关联本地用户：受保护账号拒绝、启用方向必须满足 embyShouldEnableUser
//     （不得绕过 Web 禁用/有效期），并同步本地 EmbyDisabled 镜像。
func (a *App) handleAdminEmbyUserToggle(w http.ResponseWriter, r *http.Request, params Params) {
	enable := strings.HasSuffix(r.URL.Path, "/enable")
	embyID := strings.TrimSpace(params["embyId"])
	if embyID == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "缺少 Emby 用户 ID")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby URL 或 API Token 未配置")
		return
	}
	remote, found, err := a.embyUserByID(r.Context(), embyID)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyUserLookupFailed, "读取 Emby 用户失败")
		return
	}
	if !found {
		failWithCode(w, http.StatusNotFound, ErrEmbyUserNotFound, "Emby 用户不存在")
		return
	}
	if policy, okPolicy := remote["Policy"].(map[string]any); okPolicy && boolish(policy["IsAdministrator"]) {
		failWithCode(w, http.StatusForbidden, ErrEmbyAdminBlocked, "禁止操作 Emby 管理员账号")
		return
	}
	// 已关联本地用户：沿用本地侧的保护与有效期约束，并维护 EmbyDisabled 镜像。
	if linked, okLinked := a.store().FindUserByEmbyID(embyID); okLinked {
		if a.userIsProtected(linked) {
			failWithCode(w, http.StatusForbidden, ErrUserProtected, "受保护账号禁止单独修改 Emby 状态")
			return
		}
		if enable && !a.embyShouldEnableUser(linked) {
			failWithCode(w, http.StatusConflict, ErrConflict, "Web 账号已禁用或已过期，禁止绕过有效期直接启用 Emby")
			return
		}
		if err := a.embyApplyEnabledState(r.Context(), linked.UID, embyID, enable); err != nil {
			failWithCode(w, http.StatusBadGateway, ErrEmbyDisableFailed, "Emby 状态更新失败")
			return
		}
	} else if err := a.embySetUserEnabled(r.Context(), embyID, enable); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyDisableFailed, "Emby 状态更新失败")
		return
	}
	ok(w, "Emby 状态已更新", map[string]any{"emby_user_id": embyID, "emby_enabled": enable})
}

// handleAdminEmbyUserKick 按 Emby 用户 ID 踢出其全部在线会话，用于审查页快速断连。
func (a *App) handleAdminEmbyUserKick(w http.ResponseWriter, r *http.Request, params Params) {
	embyID := strings.TrimSpace(params["embyId"])
	if embyID == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "缺少 Emby 用户 ID")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby URL 或 API Token 未配置")
		return
	}
	kicked := a.kickEmbySessions(r.Context(), embyID)
	ok(w, "会话踢出完成", map[string]any{"emby_user_id": embyID, "kicked_count": kicked})
}

// handleAdminEmbyKickAll 终止所有当前在线 Emby 会话，保留离线设备记录不被清除。
func (a *App) handleAdminEmbyKickAll(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "Emby 未配置", map[string]any{"kicked": 0})
		return
	}
	var sessResp struct {
		Items []map[string]any `json:"Items"`
	}
	if err := a.embyGet(r.Context(), "/Sessions", &sessResp); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "获取 Emby 在线会话失败")
		return
	}
	kicked := 0
	failed := 0
	for _, sess := range sessResp.Items {
		sid := asString(sess["Id"])
		if sid == "" {
			continue
		}
		if err := a.embyPost(r.Context(), "/Sessions/"+urlPathEscape(sid)+"/Terminate", nil, nil); err != nil {
			failed++
		} else {
			kicked++
		}
	}
	a.audit(r, "emby_kick_all_sessions", "admin", 0, map[string]any{"kicked": kicked, "failed": failed})
	ok(w, "在线会话已终止", map[string]any{"kicked": kicked, "failed": failed})
}
