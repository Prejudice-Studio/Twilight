package api

import (
	"net/http"
	"strings"
)

func (a *App) handleV2UserDevices(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	items := a.security().listDevices(uid)
	ok(w, "OK", items)
}

func (a *App) handleV2AdminUserDevices(w http.ResponseWriter, r *http.Request, params Params) {
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	items := a.security().listDevices(uid)
	ok(w, "OK", items)
}

func (a *App) handleV2BlockDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.security().blockDevice(uid, deviceID); statusFromError(w, err) {
		return
	}
	a.audit(r, "block_device", "admin", uid, map[string]any{"device_id": deviceID})
	ok(w, "device blocked", nil)
}

func (a *App) handleV2TrustDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.security().trustDevice(uid, deviceID); statusFromError(w, err) {
		return
	}
	a.audit(r, "trust_device", "user", uid, map[string]any{"device_id": deviceID})
	ok(w, "device trusted", nil)
}

func (a *App) handleV2DeleteDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.security().deleteDevice(uid, deviceID); statusFromError(w, err) {
		return
	}
	a.audit(r, "delete_device", "user", uid, map[string]any{"device_id": deviceID})
	ok(w, "device removed", nil)
}

func (a *App) handleV2UserLoginHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	uid := current(r).User.UID
	limit := clamp(queryInt(r, "limit", 50), 1, 100)
	items := a.security().loginHistory(uid, limit)
	ok(w, "OK", map[string]any{"records": items, "total": len(items)})
}

func (a *App) handleV2AdminLoginHistory(w http.ResponseWriter, r *http.Request, params Params) {
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 100)
	items := a.security().loginHistory(uid, limit)
	ok(w, "OK", map[string]any{"records": items, "total": len(items)})
}

func (a *App) handleV2IPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.security().listIPBlacklist())
}

func (a *App) handleV2AddIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ip := strings.TrimSpace(stringValue(payload, "ip"))
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}

	const maxBlacklistHours = 24 * 365 * 10
	hours := intValue(payload, "hours", -1)
	if hours > maxBlacklistHours {
		failWithCode(w, http.StatusBadRequest, ErrIPBlacklistDurationInvalid, "封禁时长超出允许范围")
		return
	}
	if hours == 0 || (hours < 0 && hours != -1) {
		failWithCode(w, http.StatusBadRequest, ErrIPBlacklistDurationInvalid, "封禁时长非法")
		return
	}

	reason := stringValue(payload, "reason")
	if err := a.security().addIPBlacklist(ip, reason, hours); statusFromError(w, err) {
		return
	}

	expireAt := int64(-1)
	if hours > 0 {
		expireAt = int64(hours)
	}
	a.audit(r, "add_ip_blacklist", "admin", 0, map[string]any{"ip": ip, "expire_at": expireAt, "reason": reason})
	ok(w, "IP 已加入黑名单", nil)
}

func (a *App) handleV2DeleteIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := strings.TrimSpace(stringValue(decodeMap(r), "ip"))
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}
	if err := a.security().removeIPBlacklist(ip); statusFromError(w, err) {
		return
	}
	a.audit(r, "delete_ip_blacklist", "admin", 0, map[string]any{"ip": ip})
	ok(w, "IP 已移出黑名单", nil)
}

func (a *App) handleV2Suspicious(w http.ResponseWriter, r *http.Request, _ Params) {
	hours := queryInt(r, "hours", 24)
	items := a.security().suspiciousActivity(hours)
	ok(w, "OK", items)
}
