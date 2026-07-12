package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// kickEmbySessions stops active playback and revokes the devices backing the
// user's current Emby sessions. Emby does not expose /Sessions/{id}/Logout;
// device deletion is the administrator API that also clears offline access.
func (a *App) kickEmbySessions(ctx context.Context, embyID string) int {
	if !a.embyConfigured() || strings.TrimSpace(embyID) == "" {
		return 0
	}
	sessions, err := a.embySessionsSnapshot(ctx, true)
	if err != nil {
		return 0
	}
	deviceIDs := map[string]struct{}{}
	for _, session := range sessions {
		if asString(session["UserId"]) != embyID {
			continue
		}
		if sid := asString(session["Id"]); sid != "" {
			if _, playing := embySessionNowPlaying(session); playing {
				var ignored map[string]any
				_ = a.embyPost(ctx, "/Sessions/"+urlPathEscape(sid)+"/Playing/Stop", map[string]any{"Command": "Stop"}, &ignored)
			}
		}
		if deviceID := strings.TrimSpace(asString(session["DeviceId"])); deviceID != "" {
			deviceIDs[deviceID] = struct{}{}
		}
	}
	kicked := 0
	for deviceID := range deviceIDs {
		if err := a.embyDelete(ctx, "/Devices?Id="+url.QueryEscape(deviceID)); err == nil {
			kicked++
		}
	}
	if kicked > 0 {
		a.invalidateEmbySessionsSnapshot()
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
