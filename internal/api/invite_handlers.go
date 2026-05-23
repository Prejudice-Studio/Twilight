package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleInviteConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"enabled": a.cfg.InviteEnabled, "max_depth": a.cfg.InviteMaxDepth, "invite_limit": a.cfg.InviteLimit, "invite_root_user_limit": a.cfg.InviteRootUserLimit, "require_emby": a.cfg.InviteRequireEmby, "default_days": a.cfg.InviteDefaultDays, "code_format": "INV-{random}", "permanent_invite_max_days": a.cfg.PermanentInviteMaxDays})
}
func (a *App) handleInviteMe(w http.ResponseWriter, r *http.Request, _ Params) {
	user := current(r).User
	codes := a.store.ListInviteCodes(user.UID)
	codeItems := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		codeItems = append(codeItems, inviteCodeDTO(code))
	}
	parent := any(nil)
	if rel, okRel := a.store.ParentOf(user.UID); okRel {
		if u, okUser := a.store.User(rel.ParentUID); okUser {
			parent = publicUser(u)
		}
	}
	children := []map[string]any{}
	for _, rel := range a.store.ChildrenOf(user.UID) {
		if u, okUser := a.store.User(rel.ChildUID); okUser {
			children = append(children, publicUser(u))
		}
	}
	canInvite, reason := a.canInvite(user)
	maxDays, maxReason := a.maxCodeDays(user)
	ok(w, "OK", map[string]any{"parent": parent, "children": children, "tree": children, "depth": a.inviteDepth(user.UID), "max_depth": a.cfg.InviteMaxDepth, "can_invite": canInvite, "invite_block_reason": reason, "max_code_days": maxDays, "max_code_days_reason": maxReason, "codes": codeItems, "total": len(codeItems)})
}

func (a *App) handleCreateInviteCode(w http.ResponseWriter, r *http.Request, _ Params) {
	user := current(r).User
	canInvite, reason := a.canInvite(user)
	if !canInvite {
		fail(w, http.StatusForbidden, reason)
		return
	}
	payload := decodeMap(r)
	days := intValue(payload, "days", a.cfg.InviteDefaultDays)
	maxDays, _ := a.maxCodeDays(user)
	if days <= 0 || days > maxDays {
		fail(w, http.StatusBadRequest, "邀请码天数超出允许范围")
		return
	}
	code := strings.ToUpper("INV" + randomCode(10))
	expiresAt := int64(intValue(payload, "expires_at", -1))
	targetUsername := strings.TrimSpace(stringValue(payload, "target_username"))
	if targetUsername != "" && (len(targetUsername) < 3 || len(targetUsername) > 32) {
		fail(w, http.StatusBadRequest, "目标用户名长度需在 3-32 字符之间")
		return
	}
	invite := store.InviteCode{Code: code, UID: user.UID, InviterUID: user.UID, Days: days, UseCountLimit: 1, Active: true, Note: truncateString(stringValue(payload, "note"), 255), TargetUsername: targetUsername, CreatedAt: time.Now().Unix(), ExpiredAt: expiresAt}
	_ = a.store.UpsertInviteCode(invite)
	created(w, "invite code created", inviteCodeDTO(invite))
}

func (a *App) handleInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store.ListInviteCodes(current(r).User.UID)
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		items = append(items, inviteCodeDTO(code))
	}
	ok(w, "OK", map[string]any{"codes": items, "total": len(items)})
}
func (a *App) handleDeleteInviteCode(w http.ResponseWriter, r *http.Request, params Params) {
	if statusFromError(w, a.store.DeleteInviteCode(current(r).User.UID, params["code"])) {
		return
	}
	ok(w, "invite code deleted", nil)
}
func (a *App) handleInviteCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	code := stringValue(decodeMap(r), "code")
	invite, okInvite := a.store.InviteCode(code)
	if !okInvite || !invite.Active || (invite.ExpiredAt > 0 && invite.ExpiredAt < time.Now().Unix()) {
		fail(w, http.StatusNotFound, "邀请码无效或已停用")
		return
	}
	inviter := ""
	if u, okUser := a.store.User(invite.InviterUID); okUser {
		inviter = u.Username
	}
	ok(w, "OK", map[string]any{"days": invite.Days, "inviter": inviter})
}
func (a *App) handleInviteUse(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	code := stringValue(payload, "code")
	if code == "" {
		fail(w, http.StatusBadRequest, "邀请码不能为空")
		return
	}
	user := current(r).User
	if user.EmbyID != "" {
		fail(w, http.StatusBadRequest, "当前账号已绑定 Emby")
		return
	}
	// Validate target_username restriction if set on the invite code
	invite, okInvite := a.store.InviteCode(code)
	if !okInvite || !invite.Active {
		fail(w, http.StatusNotFound, "邀请码无效或已停用")
		return
	}
	if invite.TargetUsername != "" && !strings.EqualFold(invite.TargetUsername, user.Username) {
		fail(w, http.StatusForbidden, "此邀请码仅限指定用户使用")
		return
	}
	if _, err := a.store.ConsumeInviteCode(code, user.UID); statusFromError(w, err) {
		return
	}
	u, err := a.store.UpdateUser(user.UID, func(u *store.User) error {
		u.EmbyUsername = firstNonEmpty(stringValue(payload, "emby_username"), u.Username)
		u.EmbyID = "emby_" + strconv.FormatInt(u.UID, 10)
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "invite code used", map[string]any{"user": publicUser(u)})
}
