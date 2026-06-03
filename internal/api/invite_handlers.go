package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleInviteConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"enabled": a.cfg().InviteEnabled, "max_depth": a.cfg().InviteMaxDepth, "invite_limit": a.cfg().InviteLimit, "invite_root_user_limit": a.cfg().InviteRootUserLimit, "require_emby": a.cfg().InviteRequireEmby, "default_days": a.cfg().InviteDefaultDays, "code_format": a.inviteCodeFormat(""), "permanent_invite_max_days": a.cfg().PermanentInviteMaxDays})
}

func (a *App) handleInviteMe(w http.ResponseWriter, r *http.Request, _ Params) {
	user := current(r).User
	codes := a.store().ListInviteCodes(user.UID)
	codeItems := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		codeItems = append(codeItems, a.inviteCodeDTO(code))
	}
	parent := any(nil)
	if rel, okRel := a.store().ParentOf(user.UID); okRel {
		if u, okUser := a.store().User(rel.ParentUID); okUser {
			parent = publicUser(u)
		}
	}
	children := []map[string]any{}
	maxDays, maxReason := a.maxCodeDays(user)
	now := time.Now().Unix()
	for _, rel := range a.store().ChildrenOf(user.UID) {
		if u, okUser := a.store().User(rel.ChildUID); okUser {
			item := publicUser(u)
			item["has_emby"] = u.EmbyID != ""
			item["emby_expired"] = inviteChildEmbyExpired(u, now)
			item["can_generate_renew_code"] = a.canGenerateInviteRenewCodeForChild(user, u, maxDays)
			item["can_delete_emby_and_detach"] = inviteChildCanDeleteEmbyAndDetach(u, now)
			children = append(children, item)
		}
	}
	canInvite, reason := a.canInvite(user)
	ok(w, "OK", map[string]any{"enabled": a.cfg().InviteEnabled, "is_root": parent == nil, "parent": parent, "children": children, "tree": a.inviteTreeFor(user), "depth": a.inviteDepth(user.UID), "max_depth": a.cfg().InviteMaxDepth, "can_invite": canInvite, "invite_block_reason": reason, "max_code_days": maxDays, "max_code_days_reason": maxReason, "codes": codeItems, "total": len(codeItems)})
}

func (a *App) handleCreateInviteCode(w http.ResponseWriter, r *http.Request, _ Params) {
	user := current(r).User
	payload := decodeMap(r)
	if strings.HasSuffix(r.URL.Path, "/renew-codes") {
		a.handleCreateInviteRenewCode(w, r, user, payload)
		return
	}
	if !a.cfg().InviteEnabled {
		failWithCode(w, http.StatusForbidden, ErrInviteDisabled, "邀请功能未开启")
		return
	}
	// 按 UID 限速生成邀请码，防止恶意账号一次性吃满 invite_limit 配额，让
	// 邀请树受 invite_root_user_limit 控制后还是被噪声码塞满列表。
	if !a.allowRate(r.Context(), rateKey("invite-mint:", user.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	canInvite, reason := a.canInvite(user)
	if !canInvite {
		failWithCode(w, http.StatusForbidden, ErrInviteCannotInvite, reason)
		return
	}
	days := intValue(payload, "days", a.cfg().InviteDefaultDays)
	maxDays, _ := a.maxCodeDays(user)
	if days <= 0 || days > maxDays {
		failWithCode(w, http.StatusBadRequest, ErrInviteDaysOutOfRange, "邀请码天数超出允许范围")
		return
	}
	expiresAt := int64(intValue(payload, "expires_at", -1))
	if expiresAt > 0 && expiresAt <= time.Now().Unix() {
		failWithCode(w, http.StatusBadRequest, ErrInviteExpiresBeforeNow, "邀请码过期时间必须晚于当前时间")
		return
	}
	targetUsername := strings.TrimSpace(stringValue(payload, "target_username"))
	if targetUsername != "" && !validRegcodeTargetUsername(targetUsername) {
		failWithCode(w, http.StatusBadRequest, ErrInviteTargetUsernameBad, "目标用户名长度需为 3-32 个字符，且不能包含特殊路径或注入字符")
		return
	}
	code := ""
	format := a.inviteCodeFormat(stringValue(payload, "format"))
	algorithm := firstNonEmpty(stringValue(payload, "random_algorithm"), a.cfg().InviteCodeRandomAlgorithm, "hex10")
	for attempt := 0; attempt < 20; attempt++ {
		candidate := generateInviteCode(format, algorithm, days, 1)
		if _, exists := a.store().InviteCode(candidate); exists {
			continue
		}
		if _, exists := a.store().RegCode(candidate); exists {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		failWithCode(w, http.StatusConflict, ErrInviteGenerationConflict, "邀请码生成冲突，请重试")
		return
	}
	invite := store.InviteCode{Code: code, UID: user.UID, InviterUID: user.UID, Days: days, UseCountLimit: 1, Active: true, Note: truncateString(stringValue(payload, "note"), 255), TargetUsername: targetUsername, CreatedAt: time.Now().Unix(), ExpiredAt: expiresAt}
	if err := a.store().UpsertInviteCode(invite); statusFromError(w, err) {
		return
	}
	created(w, "invite code created", a.inviteCodeDTO(invite))
}

func (a *App) handleCreateInviteRenewCode(w http.ResponseWriter, r *http.Request, user store.User, payload map[string]any) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	if !user.Active {
		failWithCode(w, http.StatusForbidden, ErrInviteRenewUserDisabled, "账号已被禁用，无法生成续期码")
		return
	}
	// 与 canInvite 同口径：entitlement 已过期的账号不允许给下级 mint 续期码，
	// 哪怕 Active=true。R62-4 防御纵深，注意 maxCodeDays 后面也会再挡一次，但
	// 让"已过期"提前给出明确错误对前端 UX 更好（避免 maxDays 兜底的"剩余天数
	// 不足"误导文案）。
	if !userEntitlementOK(user) {
		failWithCode(w, http.StatusForbidden, ErrInviterDaysShort, "账号有效期已到期，无法生成续期码")
		return
	}
	if a.cfg().InviteRequireEmby && user.EmbyID == "" {
		failWithCode(w, http.StatusForbidden, ErrInviteRenewRequiresEmby, "请先绑定 Emby 账号后再生成续期码")
		return
	}
	targetUID := int64(intValue(payload, "target_uid", 0))
	if targetUID <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInviteRenewBadTarget, "目标用户无效")
		return
	}
	rel, okRel := a.store().ParentOf(targetUID)
	if !okRel || rel.ParentUID != user.UID {
		failWithCode(w, http.StatusForbidden, ErrInviteRenewNotDirectChild, "只能给自己的直属下级生成续期码")
		return
	}
	child, okChild := a.store().User(targetUID)
	if !okChild {
		// 历史上这里返回 ErrInviteRenewTargetMissing："目标用户不存在"——但这是
		// 对 ErrUserNotFound 的本地分叉，前端要为 USER_NOT_FOUND 与
		// INVITE_RENEW_TARGET_MISSING 写两份一模一样的"用户不存在"分支。
		// R64-7 把所有结构上的"按 uid 找不到用户"统一回 ErrUserNotFound，
		// 文案统一为 userNotFoundMessage；renew code 特有的真错误（剩余天数
		// 不够、非直属下级等）继续走各自的专门 code。
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	if !child.Active {
		failWithCode(w, http.StatusForbidden, ErrInviteRenewBadTarget, "下级 Web 账号已被禁用，无法使用续期码；请删除其 Emby 账号并断开关系")
		return
	}
	maxDays, reason := a.maxCodeDays(user)
	if maxDays <= 0 {
		failWithCode(w, http.StatusForbidden, ErrInviterDaysShort, firstNonEmpty(reason, "当前账号有效期不足，无法生成续期码"))
		return
	}
	days := intValue(payload, "days", minInt(30, maxDays))
	if days <= 0 || days > maxDays {
		failWithCode(w, http.StatusBadRequest, ErrInviteRenewDaysOutOfRange, "续期天数超出允许范围")
		return
	}
	validityHours := clamp(intValue(payload, "validity_hours", 72), 1, 720)
	format := firstNonEmpty(stringValue(payload, "format"), a.cfg().RenewCodeFormat, "REN-{random}")
	algorithm := firstNonEmpty(stringValue(payload, "random_algorithm"), a.cfg().RegCodeRandomAlgorithm, "base32-20")
	code := ""
	for attempt := 0; attempt < 20; attempt++ {
		candidate := generateRegCode(format, 2, algorithm, days, 1, int64(validityHours), 1)
		if _, exists := a.store().RegCode(candidate); exists {
			continue
		}
		if _, exists := a.store().InviteCode(candidate); exists {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		failWithCode(w, http.StatusConflict, ErrInviteGenerationConflict, "续期码生成冲突，请重试")
		return
	}
	reg := store.RegCode{Code: code, Type: 2, ValidityTime: int64(validityHours), UseCountLimit: 1, Days: days, Note: truncateString(stringValue(payload, "note"), 120), TargetUsername: child.Username, Active: true}
	if err := a.store().UpsertRegCode(reg); statusFromError(w, err) {
		return
	}
	created(w, "renew code created", map[string]any{"code": code, "target_uid": child.UID, "target_username": child.Username, "days": days, "validity_hours": validityHours, "max_code_days": maxDays})
}

func (a *App) handleInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store().ListInviteCodes(current(r).User.UID)
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		items = append(items, a.inviteCodeDTO(code))
	}
	ok(w, "OK", map[string]any{"codes": items, "total": len(items)})
}

func (a *App) handleDeleteInviteCode(w http.ResponseWriter, r *http.Request, params Params) {
	if statusFromError(w, a.store().DeleteInviteCode(current(r).User.UID, params["code"])) {
		return
	}
	ok(w, "invite code deleted", nil)
}

func (a *App) handleDetachExpiredInviteChild(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	user := current(r).User
	rel, okRel := a.store().ParentOf(uid)
	if !okRel || rel.ParentUID != user.UID {
		failWithCode(w, http.StatusForbidden, ErrInviteDetachNotDirect, "只能断开自己的直属下级")
		return
	}
	child, okChild := a.store().User(uid)
	if !okChild {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	now := time.Now().Unix()
	if !inviteChildCanDeleteEmbyAndDetach(child, now) {
		failWithCode(w, http.StatusBadRequest, ErrInviteDetachNotExpired, "只能断开 Emby 已到期或 Web 已禁用且仍绑定 Emby 的直属下级")
		return
	}
	deletedEmby := false
	if child.EmbyID != "" {
		if !a.embyConfigured() {
			failWithCode(w, http.StatusBadGateway, ErrEmbyDeleteFailed, "Emby 未配置，无法删除下级 Emby 账号")
			return
		}
		if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(child.EmbyID)); err != nil {
			zap.L().Warn("detach invite child: emby delete failed", zap.Int64("uid", uid), zap.Error(err))
			failWithCode(w, http.StatusBadGateway, ErrEmbyDeleteFailed, "删除下级 Emby 账号失败，请稍后重试或联系管理员")
			return
		}
		deletedEmby = true
	}
	updated, err := a.store().UpdateUser(uid, func(u *store.User) error {
		u.EmbyID = ""
		u.EmbyUsername = ""
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	if err := a.store().DetachInvite(uid); statusFromError(w, err) {
		return
	}
	ok(w, "已断开下级关系", map[string]any{"uid": uid, "detached": true, "deleted_emby": deletedEmby, "user": publicUser(updated)})
}

func inviteChildEmbyExpired(child store.User, now int64) bool {
	return child.EmbyID != "" && child.ExpiredAt > 0 && child.ExpiredAt < now
}

func inviteChildCanDeleteEmbyAndDetach(child store.User, now int64) bool {
	return child.EmbyID != "" && (inviteChildEmbyExpired(child, now) || !child.Active)
}

func (a *App) canGenerateInviteRenewCodeForChild(parent, child store.User, maxDays int) bool {
	if maxDays <= 0 || !child.Active || !userEntitlementOK(parent) {
		return false
	}
	if a.cfg().InviteRequireEmby && parent.EmbyID == "" {
		return false
	}
	return true
}

func (a *App) handleInviteCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().InviteEnabled {
		failWithCode(w, http.StatusForbidden, ErrInviteDisabled, "邀请功能未开启")
		return
	}
	// /invite/check 是 AuthPublic：未鉴权也能查询。如果不限速，攻击者可以
	// 按邀请码空间扫描，命中后拿到邀请人用户名（信息泄露）。按 IP 限到
	// 每分钟 20 次足够正常用户多次粘贴尝试，又能让扫描者明显受阻。
	if !a.allowRate(r.Context(), rateKey("invite-check:", a.clientIP(r)), 20, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	code := stringValue(decodeMap(r), "code")
	invite, okInvite := a.store().InviteCode(code)
	if !okInvite || !invite.Active || (invite.ExpiredAt > 0 && invite.ExpiredAt <= time.Now().Unix()) {
		failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
		return
	}
	inviter := ""
	if u, okUser := a.store().User(invite.InviterUID); okUser {
		if !u.Active {
			failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
			return
		}
		if maxDays, _ := a.maxCodeDays(u); maxDays <= 0 {
			failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
			return
		}
		inviter = u.Username
	} else {
		failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
		return
	}
	ok(w, "OK", map[string]any{"days": invite.Days, "inviter": inviter})
}

func (a *App) handleInviteUse(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().InviteEnabled {
		failWithCode(w, http.StatusForbidden, ErrInviteDisabled, "邀请功能未开启")
		return
	}
	// 按 UID 限速：登录用户每分钟最多 10 次使用尝试，避免被盗号后或恶意
	// 用户脚本尝试所有可能的邀请码进入邀请树。
	if !a.allowRate(r.Context(), rateKey("invite-use:", current(r).User.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	code := stringValue(payload, "code")
	if code == "" {
		failWithCode(w, http.StatusBadRequest, ErrCodeEmpty, "邀请码不能为空")
		return
	}
	user := current(r).User
	if user.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrInviteEmbyBound, "当前账号已绑定 Emby")
		return
	}
	invite, okInvite := a.store().InviteCode(code)
	if !okInvite || !invite.Active || (invite.ExpiredAt > 0 && invite.ExpiredAt <= time.Now().Unix()) {
		failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
		return
	}
	if invite.TargetUsername != "" && !strings.EqualFold(invite.TargetUsername, user.Username) {
		failWithCode(w, http.StatusForbidden, ErrInviteTargetMismatch, "此邀请码仅限指定用户使用")
		return
	}
	if invite.InviterUID == user.UID {
		failWithCode(w, http.StatusBadRequest, ErrInviteSelfGenerate, "不能使用自己生成的邀请码")
		return
	}
	if _, hasParent := a.store().ParentOf(user.UID); hasParent {
		failWithCode(w, http.StatusBadRequest, ErrInviteAlreadyHasParent, "当前账号已存在邀请上级，不能重复加入邀请树")
		return
	}
	if !user.PendingEmby && a.userHasEmbyGrantHistory(user) {
		failWithCode(w, http.StatusBadRequest, ErrCodeRegistrationGrantAlreadyUsed, "当前账号已经使用过 Emby 注册资格，不能重复使用邀请码")
		return
	}
	inviter, okInviter := a.store().User(invite.InviterUID)
	if !okInviter || !inviter.Active {
		failWithCode(w, http.StatusForbidden, ErrInviterUnavailable, "邀请人状态不可用")
		return
	}
	if a.inviteDepth(inviter.UID) >= a.cfg().InviteMaxDepth {
		failWithCode(w, http.StatusForbidden, ErrInviteDepthExceeded, "邀请树层级已达上限")
		return
	}
	if a.cfg().InviteRootUserLimit > 0 {
		rootUID := a.inviteRootUID(inviter.UID)
		if a.inviteDescendantCount(rootUID) >= a.cfg().InviteRootUserLimit {
			failWithCode(w, http.StatusForbidden, ErrInviteRootFull, "邀请树人数已达上限")
			return
		}
	}
	maxDays, reason := a.maxCodeDays(inviter)
	if maxDays <= 0 {
		failWithCode(w, http.StatusForbidden, ErrInviterDaysShort, firstNonEmpty(reason, "邀请人有效期不足"))
		return
	}
	effectiveDays := invite.Days
	if effectiveDays <= 0 || effectiveDays > maxDays {
		effectiveDays = maxDays
	}
	if reached, current, limit := a.embyCapacityReachedExcluding(user.UID, "", code); reached {
		failWithCode(w, http.StatusConflict, ErrEmbyCapacityReached, fmt.Sprintf("Emby 用户数量已达上限 %d/%d", current, limit))
		return
	}
	u, _, err := a.store().ConsumeInviteCodeAndUpdateUser(code, user.UID, func(u *store.User, _ store.InviteCode) error {
		if u.EmbyID == "" && u.EmbyGrantLocked && !user.PendingEmby {
			return store.ErrGrantLocked
		}
		u.EmbyUsername = firstNonEmpty(stringValue(payload, "emby_username"), u.Username)
		u.PendingEmby = true
		u.PendingEmbyDays = &effectiveDays
		markRegistrationGrant(u, registrationSourceInvite, code)
		u.ExpiredAt = boundedInviteExpiry(addDaysToExpiry(u.ExpiredAt, effectiveDays, time.Now()), inviter.ExpiredAt)
		return nil
	})
	if errors.Is(err, store.ErrGrantLocked) {
		failWithCode(w, http.StatusBadRequest, ErrCodeRegistrationGrantAlreadyUsed, "当前账号已经使用过 Emby 注册资格，不能重复使用邀请码")
		return
	}
	if statusFromError(w, err) {
		return
	}
	ok(w, "invite code used", map[string]any{"user": publicUser(u), "days": effectiveDays, "inviter_uid": invite.InviterUID})
}

func boundedInviteExpiry(candidate, inviterExpiredAt int64) int64 {
	if inviterExpiredAt > 0 && inviterExpiredAt < permanentExpiryUnix && (candidate < 0 || candidate > inviterExpiredAt) {
		return inviterExpiredAt
	}
	return candidate
}
