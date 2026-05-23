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
	payload := decodeMap(r)
	code := firstNonEmpty(stringValue(payload, "reg_code"), stringValue(payload, "code"))
	if code == "" {
		fail(w, http.StatusBadRequest, "卡码不能为空")
		return
	}
	preview, source, okPreview := a.previewCode(code, p.User)
	if !okPreview {
		fail(w, http.StatusBadRequest, "卡码无效或已过期")
		return
	}
	if boolValue(payload, "check_only", false) {
		ok(w, "OK", preview)
		return
	}
	if source == "invite" {
		invite, _ := a.store.InviteCode(code)
		if invite.InviterUID == p.User.UID {
			fail(w, http.StatusBadRequest, "不能使用自己生成的邀请码")
			return
		}
		if invite.TargetUsername != "" && !strings.EqualFold(invite.TargetUsername, p.User.Username) {
			fail(w, http.StatusForbidden, "此邀请码仅限指定用户使用")
			return
		}
		if p.User.EmbyID != "" {
			fail(w, http.StatusBadRequest, "当前账号已绑定 Emby，不能使用邀请码")
			return
		}
		if _, err := a.store.ConsumeInviteCode(code, p.User.UID); statusFromError(w, err) {
			return
		}
	}
	var reg store.RegCode
	if source == "regcode" {
		var err error
		reg, err = a.store.ConsumeRegCode(code, p.User.UID, p.User.TelegramID)
		if statusFromError(w, err) {
			return
		}
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		days := intValue(preview, "days", 30)
		if source == "regcode" {
			switch reg.Type {
			case 1:
				u.Role = store.RoleNormal
				u.PendingEmby = false
			case 3:
				u.Role = store.RoleWhitelist
				u.ExpiredAt = 253402214400
			}
		}
		if source == "invite" || (source == "regcode" && reg.Type == 1) {
			u.EmbyUsername = firstNonEmpty(stringValue(payload, "emby_username"), u.Username)
			u.EmbyID = firstNonEmpty(u.EmbyID, "emby_"+strconv.FormatInt(u.UID, 10))
		}
		if u.Role != store.RoleWhitelist {
			if days <= 0 {
				u.ExpiredAt = -1
			} else {
				base := time.Now().Unix()
				if u.ExpiredAt > base {
					base = u.ExpiredAt
				}
				u.ExpiredAt = base + int64(days)*86400
			}
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := preview
	data["pending"] = false
	data["user"] = publicUser(u)
	data["expire_status"] = expireStatus(u.ExpiredAt)
	data["expired_at"] = u.ExpiredAt
	data["role"] = u.Role
	data["role_name"] = roleName(u.Role)
	ok(w, "浣跨敤鎴愬姛", data)
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
	payload := decodeMap(r)
	code := stringValue(payload, "reg_code")
	if code != "" {
		if reg, okReg := a.store.RegCode(code); okReg {
			if reg.IsDecoy {
				fail(w, http.StatusNotFound, "娉ㄥ唽鐮佷笉瀛樺湪")
				return
			}
			status := regcodeStatus(reg)
			ok(w, "OK", map[string]any{"type": reg.Type, "type_name": regcodeTypeName(reg.Type), "days": reg.Days, "valid": status == "available"})
			return
		}
	}
	fail(w, http.StatusNotFound, "娉ㄥ唽鐮佷笉瀛樺湪")
}
