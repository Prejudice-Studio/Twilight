package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleUseCode(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	// 限速必须在任何 preview / 消费之前、且对 check_only 同样生效：否则
	// check_only=true 会把 /users/me/use-code 变成一个不消费的"卡码有效性 + 类型 +
	// 天数"预言机，吞吐量是全局 IP 桶（最高 1200/min）而非 regcode-check/invite-check
	// 专用的 10~20/min，等于绕过了那两个端点刻意设置的枚举防护。
	// 双桶：per-IP 挡单机扫描与"多账号并发刷同一码空间"，per-UID 挡单账号轮换 IP。
	if !a.allowRate(r.Context(), rateKey("use-code:ip:", a.clientIP(r)), 10, time.Minute) ||
		!a.allowRate(r.Context(), rateKey("use-code:uid:", strconv.FormatInt(p.User.UID, 10)), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	code := firstNonEmpty(stringValue(payload, "reg_code"), stringValue(payload, "code"))
	if code == "" {
		failWithCode(w, http.StatusBadRequest, ErrCodeEmpty, "卡码不能为空")
		return
	}
	preview, source, okPreview := a.previewCode(r.Context(), code, p.User)
	if !okPreview {
		failWithCode(w, http.StatusBadRequest, ErrCodeInvalid, "卡码无效或已过期")
		return
	}
	days := 30
	if _, ok := preview["days"]; ok {
		days = int(numeric(preview["days"]))
	}
	codeType := int(numeric(preview["type"]))
	grantsEmby := codeGrantsEmbyRegistration(source, codeType)
	if grantsEmby && p.User.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrCodeAlreadyEmbyBound, "当前账号已绑定 Emby，请使用续期码")
		return
	}
	if boolValue(payload, "check_only", false) {
		ok(w, "OK", preview)
		return
	}
	if source == "regcode" && a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	replacesPendingEntitlement := source == "regcode" && p.User.EmbyID == "" && p.User.PendingEmby && codeGrantsEmbyRegistration(source, codeType)
	var inviteForUse store.InviteCode
	var inviterForUse store.User
	if grantsEmby && p.User.EmbyID == "" && !replacesPendingEntitlement {
		excludeRegCode := ""
		excludeInviteCode := ""
		if source == "regcode" {
			excludeRegCode = code
		} else if source == "invite" {
			excludeInviteCode = code
		}
		if reached, current, limit := a.embyCapacityReachedExcluding(p.User.UID, excludeRegCode, excludeInviteCode); reached {
			failWithCode(w, http.StatusConflict, ErrEmbyCapacityReached, "Emby 用户数量已达上限 "+strconv.Itoa(current)+"/"+strconv.Itoa(limit))
			return
		}
	}

	if source == "invite" {
		invite, okInvite := a.store().InviteCode(code)
		if !okInvite || !invite.Active || (invite.ExpiredAt > 0 && invite.ExpiredAt <= time.Now().Unix()) {
			failWithCode(w, http.StatusNotFound, ErrInviteNotFound, "邀请码无效或已停用")
			return
		}
		inviteForUse = invite
		if inviteForUse.InviterUID == p.User.UID {
			failWithCode(w, http.StatusBadRequest, ErrInviteSelfGenerate, "不能使用自己生成的邀请码")
			return
		}
		if _, hasParent := a.store().ParentOf(p.User.UID); hasParent {
			failWithCode(w, http.StatusBadRequest, ErrInviteAlreadyHasParent, "当前账号已存在邀请上级，不能重复加入邀请树")
			return
		}
		if inviteForUse.TargetUsername != "" && !strings.EqualFold(inviteForUse.TargetUsername, p.User.Username) {
			failWithCode(w, http.StatusForbidden, ErrInviteTargetMismatch, "此邀请码仅限指定用户使用")
			return
		}
		inviter, okInviter := a.store().User(inviteForUse.InviterUID)
		if !okInviter || !inviter.Active {
			failWithCode(w, http.StatusForbidden, ErrInviterUnavailable, "邀请人状态不可用")
			return
		}
		inviterForUse = inviter
		if a.inviteDepth(inviterForUse.UID) >= a.cfg().InviteMaxDepth {
			failWithCode(w, http.StatusForbidden, ErrInviteDepthExceeded, "邀请树层级已达上限")
			return
		}
		if a.cfg().InviteRootUserLimit > 0 {
			rootUID := a.inviteRootUID(inviterForUse.UID)
			if a.inviteDescendantCount(rootUID) >= a.cfg().InviteRootUserLimit {
				failWithCode(w, http.StatusForbidden, ErrInviteRootFull, "邀请树人数已达上限")
				return
			}
		}
		maxDays, reason := a.maxCodeDays(inviterForUse)
		if maxDays <= 0 {
			failWithCode(w, http.StatusForbidden, ErrInviterDaysShort, firstNonEmpty(reason, "邀请人有效期不足"))
			return
		}
		if days <= 0 || days > maxDays {
			days = maxDays
		}
		if _, err := a.store().ConsumeInviteCode(code, p.User.UID); statusFromError(w, err) {
			return
		}
	}
	var reg store.RegCode
	if source == "regcode" {
		var err error
		reg, err = a.store().ConsumeRegCode(code, p.User.UID, p.User.TelegramID)
		if statusFromError(w, err) {
			return
		}
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		if replacesPendingEntitlement {
			u.PendingEmby = false
			u.PendingEmbyDays = nil
		}
		if source == "regcode" {
			switch reg.Type {
			case 1:
				u.Role = store.RoleNormal
				u.PendingEmby = u.EmbyID == ""
				u.PendingEmbyDays = &days
			case 3:
				u.Role = store.RoleWhitelist
				u.Active = true
				u.ExpiredAt = permanentExpiryUnix
				if u.EmbyID == "" {
					permanentDays := -1
					u.PendingEmby = true
					u.PendingEmbyDays = &permanentDays
				}
			}
		}
		if source == "invite" || (source == "regcode" && reg.Type == 1) {
			u.EmbyUsername = firstNonEmpty(stringValue(payload, "emby_username"), u.Username)
			if u.EmbyID == "" {
				u.PendingEmby = true
				u.PendingEmbyDays = &days
			}
		}
		if source == "invite" {
			// invite 续期：上限 = 邀请人剩余天数。统一走 renewExpiryAndReactivate
			// 让"过期 invitee 重新使用 invite 码"路径自动重新激活账号。
			renewExpiryAndReactivate(u, boundedInviteExpiry(addDaysToExpiry(u.ExpiredAt, days, time.Now()), inviterForUse.ExpiredAt))
		} else if u.Role != store.RoleWhitelist {
			renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, time.Now()))
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := preview
	data["pending"] = u.PendingEmby && u.EmbyID == ""
	data["user"] = publicUser(u)
	data["expire_status"] = expireStatus(u.ExpiredAt)
	data["expired_at"] = u.ExpiredAt
	data["role"] = u.Role
	data["role_name"] = roleName(u.Role)
	ok(w, "使用成功", data)
}

func codeGrantsEmbyRegistration(source string, codeType int) bool {
	return source == "invite" || codeType == 1 || codeType == 3
}

func codePreview(source string, codeType int, days int, inviter string) map[string]any {
	typeName := map[int]string{1: "注册码", 2: "续期码", 3: "白名单码"}[codeType]
	if source == "invite" {
		typeName = "邀请码"
	}
	duration := "永久"
	if days > 0 {
		duration = strconv.Itoa(days) + " 天"
	}
	return map[string]any{"source": source, "type": codeType, "type_name": typeName, "days": days, "valid": true, "inviter": inviter, "requires_emby_credentials": source == "invite" || codeType == 1 || codeType == 3, "confirm_title": "确认使用" + typeName, "description": "使用后将获得 " + duration + " 权益", "duration_label": duration, "submit_label": "确认使用"}
}

func (a *App) handleRegcodeCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("regcode-check:", a.clientIP(r)), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	code := stringValue(payload, "reg_code")
	if code != "" {
		if reg, okReg := a.store().RegCode(code); okReg {
			if reg.IsDecoy || reg.TargetUsername != "" {
				failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
				return
			}
			status := regcodeStatus(reg)
			ok(w, "OK", map[string]any{"type": reg.Type, "type_name": regcodeTypeName(reg.Type), "days": normalizeRegCodeDays(reg.Days), "valid": status == "available"})
			return
		}
	}
	failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
}
