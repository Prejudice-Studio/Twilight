package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) twoFactorError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, security.ErrTwoFactorKey):
		failWithCode(w, 503, ErrTwoFactorUnavailable, "双重验证暂不可用，请联系管理员检查密钥配置")
	case errors.Is(err, security.ErrTwoFactorCode):
		failWithCode(w, 401, ErrTwoFactorCode, "验证码无效或已使用")
	case errors.Is(err, store.ErrTwoFactorInvalid), errors.Is(err, store.ErrTwoFactorRequired):
		failWithCode(w, 409, ErrTwoFactorInvalid, "验证请求已失效，请重新开始")
	case errors.Is(err, store.ErrTelegramLinkCapacity):
		failWithCode(w, 429, ErrRateLimited, "请求过多，请稍后重试")
	default:
		failWithCode(w, 500, ErrInternal, "双重验证操作失败，请稍后重试")
	}
}

func (a *App) handleTwoFactorStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	factor, err := a.store().TwoFactorAccount(r.Context(), current(r).User.UID)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	_, keyErr := security.TwoFactorKey(a.cfg().TwoFactorKey)
	ok(w, "OK", map[string]any{"enabled": factor.EnabledAt > 0, "enabled_at": factor.EnabledAt, "recovery_remaining": len(factor.Recovery), "enrollment_allowed": a.cfg().TwoFactorEnrollmentEnabled, "key_ready": keyErr == nil})
}

func (a *App) twoFactorPassword(w http.ResponseWriter, r *http.Request, payload map[string]any) bool {
	uid := current(r).User.UID
	if !a.allowRate(r.Context(), rateKey("2fa:manage:uid:", uid), 5, 5*time.Minute) || !a.allowRate(r.Context(), rateKey("2fa:manage:ip:", a.clientIP(r)), 20, 5*time.Minute) {
		failWithCode(w, 429, ErrRateLimited, "验证过于频繁，请稍后再试")
		return false
	}
	if !verifyPasswordThrottled(stringValue(payload, "password"), current(r).User.PasswordHash) {
		failWithCode(w, 403, ErrPasswordOldMismatch, "当前密码不正确")
		return false
	}
	return true
}

func (a *App) handleTwoFactorSetup(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !a.cfg().TwoFactorEnrollmentEnabled {
		_ = a.store().CancelTwoFactorSetups(r.Context())
		failWithCode(w, 403, ErrTwoFactorUnavailable, "管理员尚未开放双重验证设置")
		return
	}
	payload := decodeMap(r)
	if !a.twoFactorPassword(w, r, payload) {
		return
	}
	key, err := security.TwoFactorKey(a.cfg().TwoFactorKey)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	secret, err := security.NewTOTPSecret()
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	token, err := security.RandomHex(32)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	p := current(r)
	sealed, err := security.SealTOTP(key, p.User.UID, secret)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	expiry := time.Now().Add(10 * time.Minute).Unix()
	device := loginDeviceID(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	err = a.store().BeginTwoFactorSetup(r.Context(), p.User, store.TwoFactorRequest{Hash: security.TwoFactorDigest("request", token), Secret: sealed, SessionHash: sessionTokenDigest(p.Token), DeviceID: device, ExpiresAt: expiry})
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	ok(w, "扫描二维码后输入验证码", map[string]any{"request": token, "secret": secret, "uri": security.TOTPUri(secret, firstNonEmpty(a.cfg().AppName, "Twilight"), p.User.Username), "expires_at": expiry})
}

func (a *App) handleTwoFactorEnable(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !a.cfg().TwoFactorEnrollmentEnabled {
		_ = a.store().CancelTwoFactorSetups(r.Context())
		failWithCode(w, 403, ErrTwoFactorUnavailable, "管理员已关闭新的双重验证设置")
		return
	}
	p := current(r)
	payload := decodeMap(r)
	if !a.allowRate(r.Context(), rateKey("2fa:enable:", p.User.UID), 10, 5*time.Minute) {
		failWithCode(w, 429, ErrRateLimited, "验证过于频繁")
		return
	}
	key, err := security.TwoFactorKey(a.cfg().TwoFactorKey)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	codes, hashes, err := security.NewRecoveryCodes()
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	err = a.store().EnableTwoFactor(r.Context(), p.User.UID, security.TwoFactorDigest("request", stringValue(payload, "request")), sessionTokenDigest(p.Token), strings.TrimSpace(stringValue(payload, "code")), key, hashes)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	a.audit(r, "two_factor_enable", "user", p.User.UID, nil)
	a.clearSessionCookie(w)
	ok(w, "双重验证已启用，请保存备用码并重新登录", map[string]any{"recovery_codes": codes, "reauthenticate": true})
}

func (a *App) handleTwoFactorChange(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	payload := decodeMap(r)
	if !a.twoFactorPassword(w, r, payload) {
		return
	}
	disable := r.Method == http.MethodDelete
	key, err := security.TwoFactorKey(a.cfg().TwoFactorKey)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	var codes, hashes []string
	if !disable {
		codes, hashes, err = security.NewRecoveryCodes()
		if err != nil {
			a.twoFactorError(w, err)
			return
		}
	}
	p := current(r)
	err = a.store().ChangeTwoFactor(r.Context(), p.User, sessionTokenDigest(p.Token), strings.TrimSpace(stringValue(payload, "code")), boolValue(payload, "recovery", false), disable, key, hashes)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	action := "two_factor_recovery_regenerate"
	if disable {
		action = "two_factor_disable"
	}
	a.audit(r, action, "user", p.User.UID, nil)
	a.clearSessionCookie(w)
	ok(w, "设置已更新，请重新登录", map[string]any{"recovery_codes": codes, "reauthenticate": true})
}

// Returns true when a second-step response (or failure) has been written.
func (a *App) beginTwoFactorLogin(w http.ResponseWriter, r *http.Request, input loginInput, user store.User, method, tgRequest string) bool {
	deviceID := loginDeviceID(input.DeviceID, input.UserAgent, input.IP)
	if device, found := a.store().Device(user.UID, deviceID); found && device.Blocked {
		a.revokeDeviceSessions(r.Context(), user.UID, deviceID)
		failWithCode(w, http.StatusForbidden, ErrDeviceBlocked, "该设备已被管理员封禁，无法登录")
		return true
	}
	token, err := security.RandomHex(32)
	if err != nil {
		a.twoFactorError(w, err)
		return true
	}
	expiry := time.Now().Add(3 * time.Minute).Unix()
	needed, err := a.store().BeginTwoFactorLogin(r.Context(), user, store.TwoFactorRequest{Hash: security.TwoFactorDigest("request", token), DeviceID: loginDeviceID(input.DeviceID, input.UserAgent, input.IP), ExpiresAt: expiry, Method: method, TelegramRequest: tgRequest})
	if err != nil {
		a.twoFactorError(w, err)
		return true
	}
	if !needed {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	ok(w, "请输入双重验证码", map[string]any{"two_factor_required": true, "request": token, "expires_at": expiry})
	return true
}

func (a *App) handleTwoFactorLogin(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Cache-Control", "no-store")
	payload := decodeMap(r)
	token := stringValue(payload, "request")
	if len(token) != 64 {
		a.twoFactorError(w, store.ErrTwoFactorInvalid)
		return
	}
	hash := security.TwoFactorDigest("request", token)
	if r.Method == http.MethodDelete {
		if err := a.store().CancelTwoFactorRequest(r.Context(), hash); err != nil {
			a.twoFactorError(w, err)
			return
		}
		ok(w, "已取消", nil)
		return
	}
	if !a.allowRate(r.Context(), rateKey("2fa:login:ip:", a.clientIP(r)), 30, 5*time.Minute) {
		failWithCode(w, 429, ErrRateLimited, "验证过于频繁")
		return
	}
	uid, method, err := a.store().TwoFactorLoginOwner(r.Context(), hash)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	if !a.allowRate(r.Context(), rateKey("2fa:login:uid:", uid), 10, 5*time.Minute) {
		failWithCode(w, 429, ErrRateLimited, "验证过于频繁")
		return
	}
	if method == "telegram" && !a.telegramLoginEnabled() {
		_ = a.store().CancelTwoFactorRequest(r.Context(), hash)
		a.twoFactorError(w, store.ErrTwoFactorInvalid)
		return
	}
	key, err := security.TwoFactorKey(a.cfg().TwoFactorKey)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	input := loginInput{DeviceID: r.Header.Get("X-Twilight-Device"), UserAgent: r.UserAgent(), IP: a.clientIP(r)}
	grant, err := a.store().ConsumeTwoFactorLogin(r.Context(), hash, loginDeviceID(input.DeviceID, input.UserAgent, input.IP), strings.TrimSpace(stringValue(payload, "code")), boolValue(payload, "recovery", false), key)
	if err != nil {
		a.auditWithUser(r, uid, "", "two_factor_login_failed", "user", uid, nil)
		a.twoFactorError(w, err)
		return
	}
	result, err := a.completeVerifiedLogin(r, input, grant.User, grant.Version, grant.RequestHash)
	if err != nil {
		a.twoFactorError(w, err)
		return
	}
	if _, valid := a.sessions().GetRecord(r.Context(), result.Token); !valid || (method == "telegram" && !a.telegramLoginEnabled()) {
		a.sessions().Delete(r.Context(), result.Token)
		a.twoFactorError(w, store.ErrTwoFactorInvalid)
		return
	}
	a.auditWithUser(r, uid, grant.User.Username, "two_factor_login", "user", uid, map[string]any{"method": method, "recovery": boolValue(payload, "recovery", false)})
	a.issueSessionCookies(w, result.Token, result.Expiry)
	ok(w, "登录成功", map[string]any{"token": result.Token, "user": publicUser(result.User)})
}
