package api

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
	"github.com/prejudice-studio/twilight/internal/validate"
)

var telegramPublicUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

// generatedPasswordHexLen 是自动生成密码主体段（"Twilight-" 之后）的 hex 长度。
// 32 hex char = 128 bit 熵；攻击者即使拿到响应模板也无法在合理时间内枚举。
// 历史值为 12 (=48 bit)，对在线 + 离线攻击均偏弱。
const generatedPasswordHexLen = 32

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "Twilight API", map[string]any{"name": a.cfg().AppName, "version": a.cfg().Version, "docs": "/api/v1/docs"})
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request, _ Params) {
	// openapi.json 是 AuthPublic（无需登录即可读）。只输出 AuthPublic 路由，
	// 不暴露 admin / 鉴权端点的 pattern——未鉴权枚举 admin/* 等于给攻击者一份
	// 完整攻击面地图。admin 端点本身仍受 AuthAdmin 保护，这里仅去除信息披露。
	// 需要完整路由清单的运维场景走 AuthAdmin 的 /system/admin/apis。
	paths := map[string]map[string]any{}
	for _, route := range a.routes {
		if route.Auth != AuthPublic {
			continue
		}
		pathItem := paths[route.Pattern]
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[route.Pattern] = pathItem
		}
		pathItem[strings.ToLower(route.Method)] = map[string]any{"responses": map[string]any{"200": map[string]string{"description": "OK"}}}
	}
	ok(w, "OK", map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "Twilight Go API", "version": a.cfg().Version}, "paths": paths})
}

func (a *App) handleDocs(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Twilight Go API</title><h1>Twilight Go API</h1><p>OpenAPI JSON: <a href="/api/v1/openapi.json">/api/v1/openapi.json</a></p>`))
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	if !a.cfg().BangumiEnabled {
		if _, ok := payload["bgm_mode"]; ok {
			failWithCode(w, http.StatusForbidden, ErrBangumiSyncDisabled, "Bangumi 同步未开启")
			return
		}
	}
	if !a.cfg().BangumiManageEnabled {
		if _, ok := payload["bgm_manage_mode"]; ok {
			failWithCode(w, http.StatusForbidden, ErrBangumiManageDisabled, "Bangumi 管理功能未开启")
			return
		}
	}
	if token := stringValue(payload, "bgm_token"); token != "" && !a.cfg().BangumiEnabled && !a.cfg().BangumiManageEnabled {
		failWithCode(w, http.StatusForbidden, ErrBangumiSyncDisabled, "Bangumi 功能未开启")
		return
	}
	if token := stringValue(payload, "bgm_token"); len(token) > 4096 {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenTooLong, "Bangumi Token 过长")
		return
	}
	nextToken := p.User.BGMToken
	if _, ok := payload["bgm_token"]; ok {
		nextToken = stringValue(payload, "bgm_token")
	}
	if a.cfg().BangumiEnabled && boolValue(payload, "bgm_mode", false) && nextToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "启用 Bangumi 同步前请先填写个人 Token")
		return
	}
	if a.cfg().BangumiManageEnabled && boolValue(payload, "bgm_manage_mode", false) && nextToken == "" {
		failWithCode(w, http.StatusBadRequest, ErrBangumiTokenMissing, "启用 Bangumi 管理前请先填写个人 Token")
		return
	}
	bgmTokenChanged := false
	if _, ok := payload["bgm_token"]; ok {
		bgmTokenChanged = nextToken != p.User.BGMToken
	}
	// 强制邮箱验证开启时，普通/白名单用户不能再走通用资料更新随意改邮箱（那会绕过
	// 验证码校验），必须到邮箱验证区用验证码绑定 / 换绑。
	if email := strings.TrimSpace(stringValue(payload, "email")); email != "" && !strings.EqualFold(email, p.User.Email) && a.emailGateActive(p.User) {
		failWithCode(w, http.StatusForbidden, ErrEmailVerificationRequired, "已开启强制邮箱验证，请在邮箱验证区通过验证码绑定或更换邮箱")
		return
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		if email := stringValue(payload, "email"); email != "" {
			if err := validate.ValidateEmailFormat(email); err != nil {
				return err
			}
			email = strings.TrimSpace(email)
			if len(a.cfg().EmailBlacklist) > 0 && validate.CheckEmailBlacklist(email, a.cfg().EmailBlacklist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			if len(a.cfg().EmailWhitelist) > 0 && !validate.CheckEmailWhitelist(email, a.cfg().EmailWhitelist) {
				return fmt.Errorf("该邮箱域名不在允许范围内")
			}
			// 邮箱发生变化即视为未验证：通用资料更新不经过验证码，不能保留旧的
			// 已验证状态，否则用户改成任意邮箱后仍显示"已验证"。
			if !strings.EqualFold(email, u.Email) {
				u.EmailVerified = false
				u.EmailVerifiedAt = 0
			}
			u.Email = email
		}
		if username := stringValue(payload, "username"); username != "" {
			if err := validate.ValidateUsername(username); err != nil {
				return err
			}
			u.Username = username
		}
		if _, ok := payload["bgm_mode"]; ok {
			u.BGMMode = boolValue(payload, "bgm_mode", u.BGMMode)
		}
		if _, ok := payload["bgm_manage_mode"]; ok {
			u.BGMManageMode = boolValue(payload, "bgm_manage_mode", u.BGMManageMode)
		}
		if _, ok := payload["bgm_token"]; ok {
			token := stringValue(payload, "bgm_token")
			u.BGMToken = token
			if token == "" {
				u.BGMMode = false
				u.BGMManageMode = false
			}
		}
		if _, ok := payload["notify_on_login_telegram"]; ok {
			u.NotifyOnLoginTelegram = boolValue(payload, "notify_on_login_telegram", u.NotifyOnLoginTelegram)
		}
		if _, ok := payload["notify_on_login_email"]; ok {
			u.NotifyOnLoginEmail = boolValue(payload, "notify_on_login_email", u.NotifyOnLoginEmail)
		}
		if _, ok := payload["notify_on_ticket_telegram"]; ok {
			u.NotifyOnTicketTelegram = boolValue(payload, "notify_on_ticket_telegram", u.NotifyOnTicketTelegram)
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	if bgmTokenChanged {
		_ = a.store().DeleteBangumiCollectionCache(u.UID, 0)
	}
	a.audit(r, "update_profile", "user", 0, nil)
	ok(w, "更新成功", publicUser(u))
}

func (a *App) handleUpdateUsername(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "new_username")
	if username == "" {
		failWithCode(w, http.StatusBadRequest, ErrUserNewUsernameRequired, "请填写新用户名")
		return
	}
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.Username = username
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "用户名已更新", publicUser(u))
}

// rotateSessionsAfterPasswordChange 在用户自助修改密码后吊销其全部会话（驱逐任何
// 被盗 token），再为当前调用方签发一份新会话，使其在本设备保持登录：cookie 客户端
// 透明续期（重写 session cookie），Bearer 客户端用返回体里的新 token 续用。
// 与 handleAdminResetPassword / handleForgotPassword 的「改密即吊销旧会话」口径一致。
// 失败时已写响应，返回 ok=false，调用方直接 return。
func (a *App) rotateSessionsAfterPasswordChange(w http.ResponseWriter, r *http.Request, uid int64) (string, bool) {
	a.sessions().DeleteUser(r.Context(), uid)
	token, expires, err := a.sessions().Create(r.Context(), uid)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrSessionCreateFailed, "创建会话失败")
		return "", false
	}
	a.issueSessionCookies(w, token, expires)
	return token, true
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("change-pwd:", p.User.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	oldPassword := stringValue(payload, "old_password")
	newPassword := stringValue(payload, "new_password")
	if !security.VerifyPassword(oldPassword, p.User.PasswordHash) {
		failWithCode(w, http.StatusForbidden, ErrPasswordOldMismatch, "原密码不正确")
		return
	}
	// 强制邮箱验证开启时，改系统密码需先通过邮箱验证码（未强制则直接放行）。
	if !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangePass) {
		return
	}
	if err := validate.ValidatePasswordStrength(newPassword); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	_, err = a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	token, rotated := a.rotateSessionsAfterPasswordChange(w, r, p.User.UID)
	if !rotated {
		return
	}
	a.audit(r, "change_password", "user", 0, nil)
	ok(w, "password updated", map[string]any{"token": token})
}

func (a *App) handleGeneratedPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("gen-pwd:", p.User.UID), 5, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	// 自动生成密码至少 128 bit 熵：32 hex chars。
	// 旧实现使用 randomCode(12) = 48 bit，对在线/离线攻击都过弱。
	password := "Twilight-" + randomCode(generatedPasswordHexLen)
	hash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	_, err = a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	token, rotated := a.rotateSessionsAfterPasswordChange(w, r, p.User.UID)
	if !rotated {
		return
	}
	a.audit(r, "generate_password", "user", 0, nil)
	ok(w, "password reset", map[string]any{"new_password": password, "token": token})
}

func (a *App) handleChangeEmbyPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if p.User.EmbyID == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAccountUnlinked, "当前账号未关联 Emby")
		return
	}
	payload := decodeMap(r)
	newPassword := stringValue(payload, "new_password")
	if okPass, msg := validateStrongPassword(newPassword, "Emby password"); !okPass {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, msg)
		return
	}
	// 强制邮箱验证开启时，改 Emby 密码需先通过邮箱验证码（未强制则直接放行）。
	if !a.consumePasswordChangeEmailCode(w, payload, p.User, emailPurposeChangeEmby) {
		return
	}
	if err := a.embySetPassword(r.Context(), p.User.EmbyID, newPassword); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyPasswordUpdateFailed, "更新 Emby 密码失败，请稍后重试")
		return
	}
	a.audit(r, "change_emby_password", "user", 0, nil)
	ok(w, "Emby password updated", nil)
}

func (a *App) handleBindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	if p.User.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyMissingCreds, "请填写 Emby 用户名")
		return
	}
	embyUser, okAuth, err := a.embyAuthenticateByName(r.Context(), embyUsername, embyPassword)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyAuthFailed, "Emby 鉴权失败，请稍后重试或检查上游 Emby 状态")
		return
	}
	if !okAuth {
		failWithCode(w, http.StatusUnauthorized, ErrEmbyAuthFailed, "Emby 用户名或密码错误")
		return
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	if embyID == "" {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateNoID, "Emby 未返回用户 ID")
		return
	}
	// Security: prevent non-admin users from binding Emby administrator accounts
	if p.User.Role != store.RoleAdmin {
		policy := embyPolicy(embyUser)
		if boolish(policy["IsAdministrator"]) {
			failWithCode(w, http.StatusForbidden, ErrEmbyAdminLinkForbidden, "安全限制：不允许绑定 Emby 管理员账号。如需绑定，请联系系统管理员。")
			return
		}
	}
	u, _, err := a.store().BindUserEmbyAtomic(p.User.UID, embyID, firstNonEmpty(asString(embyUser["Name"]), embyUsername), false)
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrEmbyLinkedOtherUser, "该 Emby 账号已关联其他用户")
		return
	}
	if statusFromError(w, err) {
		return
	}
	_ = a.embyApplyEnabledState(r.Context(), u.UID, u.EmbyID, a.embyShouldEnableUser(u))
	a.audit(r, "bind_emby", "user", 0, nil)
	ok(w, "Emby account linked", map[string]any{"emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "user": publicUser(u)})
}
func (a *App) handleRegisterEmby(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	if p.User.EmbyID != "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAlreadyLinked, "当前账号已关联 Emby 账号")
		return
	}
	if !p.User.PendingEmby && !a.cfg().EmbyDirectRegisterEnabled {
		failWithCode(w, http.StatusBadRequest, ErrEmbyNoRegistrationGrant, "当前账号没有 Emby 注册资格")
		return
	}
	if !p.User.PendingEmby && a.userHasEmbyGrantHistory(p.User) {
		failWithCode(w, http.StatusBadRequest, ErrCodeRegistrationGrantAlreadyUsed, "当前账号已经使用过 Emby 注册资格，不能重复自助开通 Emby")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyMissingCreds, "请填写 Emby 用户名")
		return
	}
	if okPass, msg := validateStrongPassword(embyPassword, "Emby password"); !okPass {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, msg)
		return
	}
	if existing, exists, err := a.embyUserByName(r.Context(), embyUsername); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyUsernameLookupFailed, "查询 Emby 用户名失败，请稍后重试")
		return
	} else if exists {
		failWithCode(w, http.StatusConflict, ErrEmbyUsernameTaken, "Emby 用户名已存在："+asString(existing["Name"]))
		return
	}
	if reached, current, limit := a.embyCapacityReached(p.User.UID); reached {
		failWithCode(w, http.StatusConflict, ErrEmbyCapacityReached, fmt.Sprintf("Emby 用户数量已达上限 %d/%d", current, limit))
		return
	}
	createdUser, err := a.embyCreateUser(r.Context(), embyUsername, embyPassword)
	if err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyCreateFailed, "创建 Emby 用户失败，请稍后重试")
		return
	}
	embyID := asString(createdUser["Id"])
	days := a.cfg().EmbyDirectRegisterDays
	if p.User.PendingEmbyDays != nil {
		days = *p.User.PendingEmbyDays
	}
	if days == 0 {
		days = 30
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.EmbyID = embyID
		u.EmbyUsername = firstNonEmpty(asString(createdUser["Name"]), embyUsername)
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		if p.User.PendingEmby {
			markRegistrationGrant(u, firstNonEmpty(u.RegistrationSource, registrationSourceRegCode), u.RegistrationCode)
		} else {
			markRegistrationGrant(u, registrationSourceRegCode, "")
		}
		if u.Role == store.RoleUnrecognized {
			u.Role = store.RoleNormal
		}
		if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || days < 0 {
			u.ExpiredAt = permanentExpiryUnix
		} else {
			u.ExpiredAt = expiryFromDays(days, time.Now())
		}
		return nil
	})
	if statusFromError(w, err) {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		return
	}
	_ = a.embyApplyEnabledState(r.Context(), u.UID, u.EmbyID, a.embyShouldEnableUser(u))
	a.audit(r, "register_emby", "user", 0, nil)
	ok(w, "Emby account created", map[string]any{"user": publicUser(u), "emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "request_id": ""})
}

func (a *App) handleUnbindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	user, okUser := a.store().User(p.User.UID)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	if a.requireNonEmbyAdmin(w, r, user) {
		return
	}
	if !a.userCanSelfUnbindEmby(user) {
		failWithCode(w, http.StatusForbidden, ErrEmbyUnbindForbidden, "当前账号的 Emby 注册资格来自注册码、邀请码或管理员授予，不能自助解绑后重复注册")
		return
	}
	embyID := user.EmbyID
	remoteDisabled, proceed := a.disableRemoteEmbyForUnbind(w, r, embyID)
	if !proceed {
		return
	}
	u, err := a.store().UpdateUser(user.UID, func(u *store.User) error {
		if embyID != "" && u.EmbyID != "" && u.EmbyID != embyID {
			return store.ErrConflict
		}
		u.EmbyID = ""
		u.EmbyUsername = ""
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := publicUser(u)
	data["remote_emby_disabled"] = remoteDisabled
	data["old_emby_id"] = embyID
	a.audit(r, "unbind_emby", "user", 0, nil)
	ok(w, "远端 Emby 账号已禁用，并已解除本地绑定", data)
}

func (a *App) handleRenew(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	// Self-service renewal requires a reg_code; bare renewal without code is forbidden
	payload := decodeMap(r)
	regCode := stringValue(payload, "reg_code")
	if regCode == "" {
		failWithCode(w, http.StatusBadRequest, ErrRenewCodeRequired, "续期需要提供注册码")
		return
	}
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if !a.allowRate(r.Context(), rateKey("renew:", p.User.UID), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "操作过于频繁，请稍后再试")
		return
	}
	preview, source, okPreview := a.previewCode(r.Context(), regCode, p.User)
	if !okPreview || source != "regcode" || int(numeric(preview["type"])) != 2 {
		failWithCode(w, http.StatusBadRequest, ErrRenewCodeInvalid, "续期码无效、已用完、已过期或不属于当前用户")
		return
	}
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	u, _, err := a.store().ConsumeRegCodeAndUpdateUser(regCode, p.User.UID, p.User.TelegramID, func(u *store.User, code store.RegCode) error {
		days := normalizeRegCodeDays(code.Days)
		// 用 renewExpiryAndReactivate 而不是裸 ExpiredAt = ...：自助续费会
		// 把曾被 check_expired 设成 Active=false 的非邀请账号同步解禁，避免
		// "续完仍登不上"的死循环。
		renewExpiryAndReactivate(u, addDaysToExpiry(u.ExpiredAt, days, time.Now()))
		return nil
	})
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, store.ErrExpired) && !errors.Is(err, store.ErrConflict) {
			statusFromError(w, err)
			return
		}
		failWithCode(w, http.StatusBadRequest, ErrRegcodeInvalid, "注册码无效、已用完或已过期")
		return
	}
	a.audit(r, "renew_account", "user", 0, map[string]any{"code": regCode})
	ok(w, "续期成功", map[string]any{"expire_status": expireStatus(u.ExpiredAt), "expired_at": publicExpiryUnix(u.ExpiredAt), "user": publicUser(u)})
}

func (a *App) handleQueueStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"status": "success", "pending": false, "terminal": true, "result": map[string]any{}})
}

func (a *App) handleRegisterBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCreateBindCode) {
		return
	}
	if !a.telegramAvailable() {
		failWithCode(w, http.StatusServiceUnavailable, ErrTGNotConfigured, "Telegram Bot 未配置")
		return
	}
	if !a.allowRate(r.Context(), rateKey("register-bind-code:", a.clientIP(r)), a.cfg().RateLimitRegisterPer10m, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrBindCodeRateLimited, "绑定码请求过于频繁")
		return
	}
	a.createBindCode(w, 0, "register")
}

func (a *App) handleUserBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	if !requireWebUIIntent(w, r, twilightIntentCreateBindCode) {
		return
	}
	if !a.telegramAvailable() {
		failWithCode(w, http.StatusServiceUnavailable, ErrTGNotConfigured, "Telegram Bot 未配置")
		return
	}
	if !a.allowRate(r.Context(), rateKey("user-bind-code:", current(r).User.UID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrBindCodeRateLimited, "绑定码请求过于频繁")
		return
	}
	a.createBindCode(w, current(r).User.UID, "user")
}

func (a *App) createBindCode(w http.ResponseWriter, uid int64, scene string) {
	a.cleanupExpiredBindCodes(time.Now().Unix())
	code := ""
	for attempt := 0; attempt < 20; attempt++ {
		candidate := strings.ToUpper(randomCode(6))
		if _, exists := a.bindCode(candidate); exists {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		failWithCode(w, http.StatusConflict, ErrBindCodeConflict, "绑定码生成冲突，请重试")
		return
	}
	now := time.Now().Unix()
	if err := a.upsertBindCode(store.BindCode{Code: code, Scene: scene, UID: uid, CreatedAt: now, ExpiresAt: now + 300}); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrBindCodeSaveFailed, "绑定码保存失败，请稍后重试")
		return
	}
	ok(w, "OK", map[string]any{"bind_code": code, "expires_in": 300})
}

func (a *App) handleBindCodeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	// Long-poll 支持：客户端传 wait=N（秒）表示愿意等待最多 N 秒。
	// 在等待期间每 500ms 检查一次 bind code 状态，一旦变为终态立即返回。
	// 不传 wait 或 wait<=0 时退化为即时响应（兼容旧客户端）。
	waitSec := clamp(queryInt(r, "wait", 0), 0, 60)

	respond := func() {
		state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
		writeTelegramBindCodeState(w, state)
	}

	// 即时模式
	if waitSec <= 0 {
		respond()
		return
	}

	// Long-poll 模式：先检查一次，如果已经是终态直接返回
	state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
	if state.Terminal {
		respond()
		return
	}

	// 挂起等待，每 500ms 轮询 store 直到终态或超时
	deadline := time.After(time.Duration(waitSec) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			respond()
			return
		case <-ticker.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if state.Terminal {
				respond()
				return
			}
		}
	}
}

func (a *App) handleBindCodeStatusWS(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("register-bind-status-ws:", a.clientIP(r)), max(30, a.cfg().RateLimitLoginPerMinute), time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	sendState := func(state telegramBindCodeState) bool {
		data := state.response()
		data["type"] = "status"
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		return writeWebSocketText(conn, payload) == nil
	}

	updates, unsubscribe := a.bindStatus.subscribe(code)
	defer unsubscribe()

	state := a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
	if !sendState(state) || state.Terminal {
		writeWebSocketClose(conn)
		return
	}

	expiryTimer := time.NewTimer(bindStateExpiryWait(state))
	defer expiryTimer.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	resetExpiryTimer := func(next telegramBindCodeState) {
		if !expiryTimer.Stop() {
			select {
			case <-expiryTimer.C:
			default:
			}
		}
		expiryTimer.Reset(bindStateExpiryWait(next))
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-expiryTimer.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-heartbeat.C:
			state = a.telegramBindCodeState(code, 0, "register", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
		}
	}
}

func (a *App) handleUserBindCodeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	uid := current(r).User.UID
	waitSec := clamp(queryInt(r, "wait", 0), 0, 60)

	respond := func() {
		state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
		writeTelegramBindCodeState(w, state)
	}

	if waitSec <= 0 {
		respond()
		return
	}

	state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
	if state.Terminal {
		respond()
		return
	}

	deadline := time.After(time.Duration(waitSec) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			respond()
			return
		case <-ticker.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if state.Terminal {
				respond()
				return
			}
		}
	}
}

func (a *App) handleUserBindCodeStatusWS(w http.ResponseWriter, r *http.Request, _ Params) {
	uid := current(r).User.UID
	if !a.allowRate(r.Context(), rateKey("user-bind-status-ws:", uid), 10, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁，请稍后再试")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if !telegramBindCodePattern.MatchString(code) {
		failWithCode(w, http.StatusBadRequest, ErrTGBindCodeFormat, "Telegram 绑定码格式不正确")
		return
	}
	conn, err := acceptWebSocket(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	sendState := func(state telegramBindCodeState) bool {
		data := state.response()
		data["type"] = "status"
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		return writeWebSocketText(conn, payload) == nil
	}

	updates, unsubscribe := a.bindStatus.subscribe(code)
	defer unsubscribe()

	state := a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
	if !sendState(state) || state.Terminal {
		writeWebSocketClose(conn)
		return
	}

	expiryTimer := time.NewTimer(bindStateExpiryWait(state))
	defer expiryTimer.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	resetExpiryTimer := func(next telegramBindCodeState) {
		if !expiryTimer.Stop() {
			select {
			case <-expiryTimer.C:
			default:
			}
		}
		expiryTimer.Reset(bindStateExpiryWait(next))
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-expiryTimer.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
			resetExpiryTimer(state)
		case <-heartbeat.C:
			state = a.telegramBindCodeState(code, uid, "", time.Now().Unix(), true)
			if !sendState(state) || state.Terminal {
				writeWebSocketClose(conn)
				return
			}
		}
	}
}

func bindStateExpiryWait(state telegramBindCodeState) time.Duration {
	if state.ExpiresIn <= 0 {
		return time.Second
	}
	return time.Duration(state.ExpiresIn)*time.Second + time.Second
}

func (a *App) handleRebindComplete(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if !p.User.RebindingInProgress {
		ok(w, "not in rebinding", nil)
		return
	}
	if p.User.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGNotBound, "尚未完成 Telegram 绑定")
		return
	}
	// 换绑完成后检查新 Telegram 账号是否在要求的群组/频道中
	if missing, err := a.telegramBindRequirementMissing(r.Context(), p.User.TelegramID); err != nil {
		_, _ = a.store().UpdateUser(p.User.UID, func(u *store.User) error {
			u.RebindingInProgress = true
			u.RebindingSince = time.Now().Unix()
			return nil
		})
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupCheckFailed, "Telegram 账号未加入要求的群组/频道，换绑失败")
		return
	} else if len(missing) > 0 {
		_, _ = a.store().UpdateUser(p.User.UID, func(u *store.User) error {
			u.RebindingInProgress = true
			u.RebindingSince = time.Now().Unix()
			return nil
		})
		failWithCode(w, http.StatusForbidden, ErrTGBindGroupCheckFailed, "Telegram 账号未加入要求的群组/频道："+strings.Join(missing, "、"))
		return
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error {
		u.RebindingInProgress = false
		u.RebindingSince = 0
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	// 换绑完成后同步恢复 Emby 账号
	if u.EmbyID != "" {
		sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
		if a.embyShouldEnableUser(u) {
			_ = a.embyApplyEnabledState(sideCtx, u.UID, u.EmbyID, true)
		}
		sideCancel()
	}
	ok(w, "rebinding complete", publicUser(u))
}

// telegramStatusFields 统一计算用户 Telegram 绑定 / 换绑状态，供
// handleTelegramStatus（/users/me/telegram，dashboard 用）与 handleUserSettings
// （/users/me/settings，设置页用）复用。历史上两处各自手算，导致
// can_change / can_unbind 在 "used"（换绑已消费）与管理员场景下口径不一致。
// can_unbind 口径必须与 handleUnbindTelegram 的服务端校验完全一致：换绑一律走
// 审批，非管理员仅在存在 approved 换绑请求时才可解绑（与 force_bind 无关），
// 否则只能走 can_change 提交换绑申请。管理员不受限。
func (a *App) telegramStatusFields(u store.User) map[string]any {
	forceBind := a.cfg().ForceBindTelegram
	admin := u.Role == store.RoleAdmin
	canUnbind := admin
	canChange := true
	pendingRebind := false
	rebindApproved := false
	var rebindStatus any
	var rebindID any
	if latestReq, hasReq := a.store().UserLatestRebindRequest(u.UID); hasReq {
		rebindStatus = latestReq.Status
		rebindID = latestReq.ID
		switch latestReq.Status {
		case "pending":
			pendingRebind = true
			if !admin {
				canChange = false
			}
		case "approved":
			rebindApproved = true
			canUnbind = true
		}
		// "used"（解绑已消费）/ "rejected"（被驳回）不再限制 canChange：用户应能
		// 重新生成绑定码 / 重新发起换绑请求。
	}
	return map[string]any{
		"bound":                  u.TelegramID != 0,
		"telegram_id":            nullableInt(u.TelegramID),
		"telegram_id_full":       nullableInt(u.TelegramID),
		"telegram_username":      u.TelegramUsername,
		"force_bind":             forceBind,
		"can_unbind":             canUnbind,
		"can_change":             canChange,
		"rebind_approved":        rebindApproved,
		"pending_rebind_request": pendingRebind,
		"rebind_request_status":  rebindStatus,
		"rebind_request_id":      rebindID,
		"rebinding_in_progress":  u.RebindingInProgress,
	}
}

func (a *App) handleTelegramStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.telegramStatusFields(current(r).User))
}

func (a *App) handleUnbindTelegram(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	// 换绑一律走审批：非管理员解绑必须先有一条 approved 的换绑申请，解绑时消费掉
	// （approved→used）。这条门禁与 force_bind_telegram 无关——即使关闭强制绑定，
	// 也不允许用户自助无限解绑/重绑（每次更换都需管理员批准一次，用过即作废）。
	// 管理员自身不受限。
	var consumeRebindID int64
	if p.User.Role != store.RoleAdmin {
		latestReq, hasReq := a.store().UserLatestRebindRequest(p.User.UID)
		if !hasReq || latestReq.Status != "approved" {
			failWithCode(w, http.StatusForbidden, ErrTGUnbindForbidden, "更换 Telegram 需要先提交换绑申请并经管理员批准")
			return
		}
		consumeRebindID = latestReq.ID
	}
	u, err := a.store().UpdateUser(p.User.UID, func(u *store.User) error { u.TelegramID = 0; u.TelegramUsername = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	// Mark approved rebind request as consumed so it cannot be reused.
	// 走 ConsumeRebindRequest 而非 ReviewRebindRequest：后者会把 ReviewerUID
	// 覆盖成 0、用 "auto-consumed" 抹掉管理员原始审核备注、并把 ReviewedAt
	// 重置为 now，等于销毁了"哪位管理员、何时、为何批准"的审计痕迹。
	// ConsumeRebindRequest 只把 Status 由 approved 翻成 used，保留审核元数据。
	if consumeRebindID > 0 {
		_ = a.store().ConsumeRebindRequest(consumeRebindID)
	}
	_, _ = a.store().UpdateUser(p.User.UID, func(u2 *store.User) error {
		if u2.Role != store.RoleAdmin {
			u2.RebindingInProgress = true
			u2.RebindingSince = time.Now().Unix()
		}
		return nil
	})
	// 解绑时同步禁用 Emby 账号，防止用户通过旧 TG 关联的 Emby 继续使用
	if u.EmbyID != "" {
		sideCtx, sideCancel := schedulerSideEffectContext(r.Context())
		_, _ = a.disableRemoteEmbyForWebState(sideCtx, u)
		sideCancel()
	}
	a.audit(r, "unbind_telegram", "user", 0, nil)
	ok(w, "Telegram unbound. rebinding required", publicUser(u))
}

func (a *App) handleTelegramRebindRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	if u.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGNotBound, "当前账号未绑定 Telegram")
		return
	}
	req, err := a.store().CreateRebindRequest(store.RebindRequest{UID: u.UID, Username: u.Username, OldTelegramID: u.TelegramID, Reason: truncateString(stringValue(decodeMap(r), "reason"), 500)})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram rebind request submitted", req)
}

func (a *App) handleUserSettings(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User

	ok(w, "OK", map[string]any{
		"bgm_mode": u.BGMMode, "bgm_token_set": u.BGMToken != "", "api_key_enabled": u.LegacyAPIKeyStatus,
		"notify_on_login_telegram": u.NotifyOnLoginTelegram, "notify_on_login_email": u.NotifyOnLoginEmail, "notify_on_ticket_telegram": u.NotifyOnTicketTelegram,
		// 与 /users/me/telegram 共用 telegramStatusFields，避免两个端点对换绑状态
		// 给出互相矛盾的 can_change / can_unbind。
		"telegram":      a.telegramStatusFields(u),
		"emby_status":   map[string]any{"is_synced": u.EmbyID != "", "is_active": u.Active, "can_unbind": a.userCanSelfUnbindEmby(u), "active_sessions": 0, "message": "OK"},
		"system_config": map[string]any{"device_limit_enabled": a.cfg().DeviceLimitEnabled, "max_devices": a.cfg().MaxDevices, "max_streams": a.cfg().MaxStreams, "bangumi_sync_enabled": a.cfg().BangumiEnabled, "bangumi_manage_enabled": a.cfg().BangumiManageEnabled},
	})
}

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request, params Params) {
	// :uid 仅出现在 AuthAdmin 路由（/api/v1/security/users/:uid/devices）。
	// 走 requireAdminForUIDParam 把"caller 不是 admin 但路径带 :uid"路由错配
	// 的情况打回 403，避免 user A 走错路由就拿 user B 的设备列表。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	items := []map[string]any{}
	for _, d := range a.store().ListDevices(uid) {
		items = append(items, map[string]any{"device_id": d.DeviceID, "device_name": d.DeviceName, "client": d.Client, "last_ip": d.LastIP, "first_seen": d.FirstSeen, "last_seen": d.LastSeen, "is_trusted": d.Trusted})
	}
	ok(w, "OK", items)
}

func (a *App) handleSessions(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", []any{})
		return
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Sessions", &remote); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "failed to read Emby sessions")
		return
	}
	user := current(r).User
	adminView := strings.Contains(r.URL.Path, "/admin/")
	items := []map[string]any{}
	for _, session := range remote {
		// 非 admin 视图只能看到自己 Emby 账号的会话。注意必须用「EmbyID 不匹配」
		// 直接过滤，而不能加 `user.EmbyID != ""` 短路：未绑定 Emby 的普通用户
		// EmbyID 为空，一旦短路成立整段过滤失效，会把全站正在播放的用户名册和
		// telegram_id（敏感信息）泄露给任意登录用户。EmbyID 为空时下面的相等判断
		// 永不成立，自然返回空列表，正是期望行为。
		if !adminView && asString(session["UserId"]) != user.EmbyID {
			continue
		}
		nowPlaying := any(nil)
		if item, ok := session["NowPlayingItem"].(map[string]any); ok {
			nowPlaying = map[string]any{"id": item["Id"], "name": item["Name"], "type": item["Type"]}
		}
		local := any(nil)
		if u, okUser := a.store().FindUserByEmbyID(asString(session["UserId"])); okUser {
			local = map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID)}
		}
		items = append(items, map[string]any{
			"session_id":          asString(session["Id"]),
			"user_id":             asString(session["UserId"]),
			"user_name":           firstNonEmpty(asString(session["UserName"]), asString(session["UserName"])),
			"client":              asString(session["Client"]),
			"device_name":         asString(session["DeviceName"]),
			"device_id":           asString(session["DeviceId"]),
			"remote_endpoint":     asString(session["RemoteEndPoint"]),
			"application_version": asString(session["ApplicationVersion"]),
			"is_active":           boolish(session["IsActive"]),
			"now_playing":         nowPlaying,
			"local_user":          local,
		})
	}
	ok(w, "OK", items)
}

func (a *App) handleLoginHistory(w http.ResponseWriter, r *http.Request, params Params) {
	// 路由表把 /security/login-history（AuthUser）和 /security/login-history/:uid
	// （AuthAdmin）挂到同 handler。原实现完全靠 splitPath + Contains 推断 uid，
	// 字符串变化或路由重排都会让鉴权静默失效。改为读 params["uid"]，并显式断
	// 言"路径带 :uid 必须是 admin"。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 100)
	logs := a.store().LoginHistory(uid, false, 0, limit)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"id": log.ID, "ip": log.IP, "device": log.DeviceName, "client": log.Client, "time": log.Time, "blocked": log.Blocked, "country": log.Country, "city": log.City})
	}
	ok(w, "OK", map[string]any{"records": items, "total": len(items)})
}

func (a *App) handleBlockDevice(w http.ResponseWriter, r *http.Request, params Params) {
	// :uid 只能来自 AuthAdmin 路由；若 AuthUser 路由表手抖加上 :uid，下面这条
	// 断言会立刻 403，避免 user A 用伪造路径 block user B 的设备。
	uid, written := requireAdminForUIDParam(w, r, params)
	if written {
		return
	}
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.store().UpdateDevice(uid, deviceID, func(d *store.Device) { d.Blocked = true; d.Trusted = false }); statusFromError(w, err) {
		return
	}
	ok(w, "device blocked", nil)
}

func (a *App) handleTrustDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	deviceID := params["device_id"]
	if err := a.store().UpdateDevice(uid, deviceID, func(d *store.Device) { d.Trusted = true; d.Blocked = false }); statusFromError(w, err) {
		return
	}
	ok(w, "device trusted", nil)
}

func (a *App) handleDeleteDevice(w http.ResponseWriter, r *http.Request, params Params) {
	deviceID := params["device_id"]
	if deviceID == "" {
		failWithCode(w, http.StatusBadRequest, ErrDeviceIDRequired, "设备 ID 不能为空")
		return
	}
	if err := a.store().DeleteDevice(current(r).User.UID, deviceID); statusFromError(w, err) {
		return
	}
	ok(w, "device removed", nil)
}

func (a *App) handleIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.store().ListIPBlacklist())
}

func (a *App) handleAddIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ip := stringValue(payload, "ip")
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}
	// hours 上限：10 年。time.Duration 是 int64 纳秒，hours * time.Hour 在 hours
	// 接近 math.MaxInt32 时会整数溢出，得到一个绕到过去的 expireAt（负数）。
	// admin 误填或 admin 凭据被盗时可借此构造"永久封禁"或"立即解封"的歧义状态，
	// 把 IP 黑名单做成不可信视图。-1 仍按"永久"语义保留，超过 87600 直接拒。
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
	expireAt := int64(-1)
	if hours > 0 {
		expireAt = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
	}
	if err := a.store().AddIPBlacklist(ip, stringValue(payload, "reason"), expireAt); statusFromError(w, err) {
		return
	}
	ok(w, "IP 已加入黑名单", nil)
}

func (a *App) handleDeleteIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := stringValue(decodeMap(r), "ip")
	if ip == "" {
		failWithCode(w, http.StatusBadRequest, ErrIPRequired, "IP 不能为空")
		return
	}
	if err := a.store().RemoveIPBlacklist(ip); statusFromError(w, err) {
		return
	}
	ok(w, "IP 已移出黑名单", nil)
}

func (a *App) handleSuspicious(w http.ResponseWriter, r *http.Request, _ Params) {
	hours := queryInt(r, "hours", 24)
	logs := a.store().LoginHistory(0, true, time.Now().Add(-time.Duration(hours)*time.Hour).Unix(), 100)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"uid": log.UID, "ip": log.IP, "device": log.DeviceName, "time": log.Time, "reason": firstNonEmpty(log.Reason, "blocked")})
	}
	ok(w, "OK", items)
}

func (a *App) handleSystemInfo(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !a.allowRate(r.Context(), rateKey("system-info:", a.clientIP(r)), 30, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁")
		return
	}
	ok(w, "OK", map[string]any{
		"name":                cfg.AppName,
		"icon":                a.publicServerIconURL(),
		"version":             cfg.Version,
		"api_version":         "v1",
		"session_cookie_name": cfg.SessionCookie,
		"features": map[string]any{
			"register":                      cfg.RegisterEnabled,
			"emby_direct_register":          cfg.EmbyDirectRegisterEnabled,
			"telegram":                      cfg.TelegramMode,
			"force_bind_telegram":           cfg.ForceBindTelegram,
			"force_bind_group":              cfg.TelegramForceBindGroup,
			"force_bind_channel":            cfg.TelegramForceBindChannel,
			"bangumi_sync":                  cfg.BangumiEnabled,
			"bangumi_manage":                cfg.BangumiManageEnabled,
			"media_request":                 cfg.MediaRequestEnabled,
			"signin":                        cfg.SigninEnabled,
			"invite":                        cfg.InviteEnabled,
			"email_enabled":                 emailConfigured(cfg),
			"force_bind_email":              cfg.EmailForceBind,
			"forgot_password_enabled":       cfg.ForgotPasswordEnabled,
			"forgot_password_emby_enabled":  cfg.ForgotPasswordEmbyEnabled,
			"forgot_password_email_enabled": cfg.ForgotPasswordEmailEnabled,
			"ticket_system":                 cfg.TicketSystemEnabled,
			"developer_mode":                a.store().DeveloperModeEnabled(),
			"emby_stats":                    cfg.EmbyStatsEnabled,
		},
		"auth_background_url": cfg.AuthBackgroundURL,
		"limits":              map[string]any{"user_limit": cfg.UserLimit, "stream_limit": cfg.MaxStreams, "ticket_image_max_size": cfg.TicketImageMaxSize, "ticket_image_max_count": cfg.TicketImageMaxCount},
		"telegram_bot":        a.publicTelegramBotInfo(r.Context()),
		"setup":               a.setupStatusData(),
		"telegram_links":      publicTelegramLinks(cfg.TelegramGroupIDs, cfg.TelegramChannelIDs),
		"required_telegram_links": publicTelegramLinks(
			requiredTelegramLinkIDs(cfg.TelegramGroupIDs, cfg.TelegramForceBindGroup),
			requiredTelegramLinkIDs(cfg.TelegramChannelIDs, cfg.TelegramForceBindChannel),
		),
		"telegram_mode":        cfg.TelegramMode,
		"bangumi_sync_enabled": cfg.BangumiEnabled,
	})
}

func (a *App) publicTelegramBotInfo(ctx context.Context) map[string]any {
	empty := map[string]any{"username": nil, "url": nil, "enabled": a.cfg().TelegramMode, "configured": strings.TrimSpace(a.cfg().TelegramBotToken) != "", "ok": false, "error": ""}
	if !a.telegramAvailable() {
		empty["error"] = "Telegram 未启用或未配置 Bot Token"
		return empty
	}
	token := strings.TrimSpace(a.cfg().TelegramBotToken)
	now := time.Now()
	a.telegramBotMu.Lock()
	if a.telegramBotCacheToken == token && now.Before(a.telegramBotCacheUntil) && a.telegramBotCache != nil {
		cached := cloneMap(a.telegramBotCache)
		a.telegramBotMu.Unlock()
		return cached
	}
	a.telegramBotMu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	me, err := a.telegramGetMe(lookupCtx)
	bot := empty
	if err == nil {
		username := strings.TrimPrefix(asString(me["username"]), "@")
		if telegramPublicUsernamePattern.MatchString(username) {
			bot = map[string]any{"username": username, "url": "https://t.me/" + username, "enabled": a.cfg().TelegramMode, "configured": true, "ok": true, "error": ""}
		}
	} else {
		bot["error"] = err.Error()
	}

	a.telegramBotMu.Lock()
	a.telegramBotCacheToken = token
	if err == nil {
		a.telegramBotCacheUntil = now.Add(10 * time.Minute)
	} else {
		a.telegramBotCacheUntil = now.Add(30 * time.Second)
	}
	a.telegramBotCache = cloneMap(bot)
	a.telegramBotMu.Unlock()
	return bot
}

func publicTelegramLinks(groupIDs, channelIDs []string) map[string]any {
	return map[string]any{
		"groups":   publicTelegramLinkList(groupIDs),
		"channels": publicTelegramLinkList(channelIDs),
	}
}

func requiredTelegramLinkIDs(ids []string, required bool) []string {
	if !required {
		return nil
	}
	return ids
}

func publicTelegramLinkList(values []string) []map[string]string {
	out := []map[string]string{}
	seen := map[string]bool{}
	for _, value := range values {
		item, ok := publicTelegramLink(value)
		if !ok || seen[item["url"]] {
			continue
		}
		seen[item["url"]] = true
		out = append(out, item)
	}
	return out
}

func publicTelegramLink(raw string) (map[string]string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "\x00\r\n\t ") {
		return nil, false
	}
	if strings.HasPrefix(value, "@") {
		username := strings.TrimPrefix(value, "@")
		if telegramPublicUsernamePattern.MatchString(username) {
			return map[string]string{"label": "@" + username, "url": "https://t.me/" + username}, true
		}
		return nil, false
	}
	if strings.HasPrefix(strings.ToLower(value), "t.me/") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "https" {
			return nil, false
		}
		host := strings.ToLower(parsed.Hostname())
		if host != "t.me" && host != "telegram.me" {
			return nil, false
		}
		username := strings.Trim(strings.TrimPrefix(parsed.Path, "/"), "/")
		if !telegramPublicUsernamePattern.MatchString(username) {
			return nil, false
		}
		cleanURL := "https://t.me/" + username
		return map[string]string{"label": "@" + username, "url": cleanURL}, true
	}
	if telegramPublicUsernamePattern.MatchString(value) {
		return map[string]string{"label": "@" + value, "url": "https://t.me/" + value}, true
	}
	return nil, false
}

func (a *App) handleServerIcon(w http.ResponseWriter, r *http.Request, _ Params) {
	iconPath, contentType, okIcon := a.configuredServerIconPath()
	if okIcon {
		file, err := os.Open(iconPath)
		if err == nil {
			defer file.Close()
			if info, statErr := file.Stat(); statErr == nil {
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeContent(w, r, info.Name(), info.ModTime(), file)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			_, _ = io.Copy(w, file)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(serverIconPNG)
}

func (a *App) publicServerIconURL() string {
	value := strings.TrimSpace(a.cfg().ServerIcon)
	if value == "" {
		return "/api/v1/system/server-icon"
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		if u.Scheme == "https" && u.User == nil && u.Hostname() != "" {
			return value
		}
		return "/api/v1/system/server-icon"
	}
	if iconPath, _, okIcon := a.configuredServerIconPath(); okIcon {
		if info, err := os.Stat(iconPath); err == nil {
			return "/api/v1/system/server-icon?v=" + strconv.FormatInt(info.ModTime().UnixNano(), 36) + "-" + strconv.FormatInt(info.Size(), 36)
		}
	}
	return "/api/v1/system/server-icon"
}

func (a *App) configuredServerIconPath() (string, string, bool) {
	value := strings.TrimSpace(a.cfg().ServerIcon)
	if value == "" {
		return "", "", false
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" {
		return "", "", false
	}
	ext := strings.ToLower(filepath.Ext(value))
	contentTypes := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".ico":  "image/x-icon",
	}
	contentType, ok := contentTypes[ext]
	if !ok {
		return "", "", false
	}
	// 必须经过 ResolveWithinRoot 约束在上传目录内：server_icon 是管理员可写的
	// 配置项，而 /api/v1/system/server-icon 是 AuthPublic。若直接接受绝对路径或
	// 含 ".." 的相对路径，一次"管理员写配置"就会变成"任意人读主机任意图片扩展名
	// 文件"（也可经 handleConfigRestore 用构造的备份触发）。绝对路径不再被接受。
	path, err := ResolveWithinRoot(firstNonEmpty(a.cfg().UploadDir, "uploads"), value)
	if err != nil {
		return "", "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return "", "", false
	}
	return path, contentType, true
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.allowRate(r.Context(), rateKey("health:", a.clientIP(r)), 60, time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrRateLimited, "请求过于频繁")
		return
	}
	isAuth := current(r).User.UID != 0
	data := map[string]any{
		"status": "healthy",
		"time":   time.Now().Unix(),
		"api":    true,
	}
	if isAuth {
		emby := a.embyOverview(r.Context())
		sessionFallback := a.sessions().FallbackCount()
		rateFallback := a.limiter().FallbackCount()
		data["database"] = a.store() != nil
		data["emby"] = boolValue(emby, "online", false)
		data["emby_configured"] = a.cfg().EmbyURL != ""
		data["redis"] = a.redis() != nil
		data["redis_degraded"] = a.redis() != nil && (sessionFallback > 0 || rateFallback > 0)
		data["storage"] = a.store().Backend()
		data["active_database"] = a.store().Backend()
		data["config_database"] = strings.ToLower(a.cfg().DatabaseDriver)
		data["storage_mismatch"] = a.runtimeDatabaseMismatch()
		data["storage_warning"] = a.databaseMismatchWarning()
	}
	ok(w, "OK", data)
}

func (a *App) handleSystemStats(w http.ResponseWriter, r *http.Request, _ Params) {
	users := a.store().ListUsers()
	activeUsers := countActive(users)
	regcodes := a.store().ListRegCodes()
	activeRegcodes := 0
	for _, code := range regcodes {
		if code.Active {
			activeRegcodes++
		}
	}
	usage := 0
	if a.cfg().UserLimit > 0 {
		usage = int(float64(len(users)) / float64(a.cfg().UserLimit) * 100)
	}
	ok(w, "OK", map[string]any{
		"timestamp":     time.Now().Unix(),
		"cpu_count":     nil,
		"users":         map[string]any{"active": activeUsers, "total": len(users), "limit": zeroNil(int64(a.cfg().UserLimit)), "usage_percent": usage},
		"regcodes":      map[string]any{"active": activeRegcodes, "total": len(regcodes)},
		"emby":          a.embyOverview(r.Context()),
		"total_users":   len(users),
		"active_users":  activeUsers,
		"redis_enabled": a.redis() != nil,
		"redis_fallback": map[string]any{
			"session": a.sessions().FallbackCount(),
			"rate":    a.limiter().FallbackCount(),
		},
		"routes": len(a.routes),
		"uptime": int64(time.Since(runtimeStartedAt).Seconds()),
	})
}

func (a *App) embyOverview(ctx context.Context) map[string]any {
	// 系统首页摘要：1.5s 快速探活，避免 emby 故障时阻塞用户响应。
	info, online := a.embyHealthFast(ctx)
	if info == nil {
		info = map[string]any{}
	}
	sessions := []map[string]any{}
	if online {
		embyCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
		_ = a.embyGet(embyCtx, "/Sessions", &sessions)
	}
	return map[string]any{
		"online":           online,
		"configured":       a.cfg().EmbyURL != "",
		"server":           a.cfg().EmbyURL,
		"server_name":      firstNonEmpty(asString(info["ServerName"]), asString(info["Name"])),
		"version":          firstNonEmpty(asString(info["Version"]), "unknown"),
		"operating_system": asString(info["OperatingSystem"]),
		"active_sessions":  len(sessions),
		"total_sessions":   len(sessions),
	}
}

func (a *App) handleEmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	// No Emby account and not pending: hide URLs
	if u.Role == store.RoleNormal && u.EmbyID == "" && !u.PendingEmby {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": true, "requires_renewal": false, "emby_disabled_by_expiry": false})
		return
	}
	// Expired normal user: hide URLs regardless of whether account is active
	if u.Role == store.RoleNormal && u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix() {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": false, "requires_renewal": true, "emby_disabled_by_expiry": true})
		return
	}
	// Disabled account (non-admin): hide URLs
	if u.Role == store.RoleNormal && !u.Active {
		ok(w, "OK", map[string]any{"lines": []any{}, "whitelist_lines": []any{}, "requires_emby_account": false, "requires_renewal": false, "emby_disabled_by_expiry": false})
		return
	}
	lines := []map[string]string{}
	for _, line := range a.cfg().EmbyURLList {
		lines = append(lines, map[string]string{"name": line.Name, "url": line.URL})
	}
	if a.cfg().EmbyPublicURL != "" {
		lines = append(lines, map[string]string{"name": "默认线路", "url": a.cfg().EmbyPublicURL})
	}
	whitelist := []map[string]string{}
	if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist {
		for _, line := range a.cfg().EmbyWhitelistURLList {
			whitelist = append(whitelist, map[string]string{"name": line.Name, "url": line.URL})
		}
		if a.cfg().EmbyWhitelistURL != "" {
			whitelist = append(whitelist, map[string]string{"name": "whitelist route", "url": a.cfg().EmbyWhitelistURL})
		}
	}
	ok(w, "OK", map[string]any{"lines": lines, "whitelist_lines": whitelist, "requires_emby_account": false, "requires_renewal": false, "emby_disabled_by_expiry": false})
}

func (a *App) handlePublicConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{
		"upload_limit":          a.cfg().MaxUploadSize,
		"bangumi_sync_enabled":  a.cfg().BangumiEnabled,
		"telegram_mode":         a.cfg().TelegramMode,
		"media_request_enabled": a.cfg().MediaRequestEnabled,
		"signin_enabled":        a.cfg().SigninEnabled,
		"invite_enabled":        a.cfg().InviteEnabled,
		"device_limit":          map[string]any{"enabled": a.cfg().DeviceLimitEnabled, "max_devices": a.cfg().MaxDevices, "max_streams": a.cfg().MaxStreams},
		"bangumi_sync":          map[string]any{"enabled": a.cfg().BangumiEnabled},
		"media_request": map[string]any{
			"enabled":                 a.cfg().MediaRequestEnabled,
			"max_concurrent_per_user": a.cfg().MaxConcurrentRequestsPerUser,
			"max_concurrent_global":   a.cfg().MaxConcurrentRequestsGlobal,
		},
		"signin": signinConfigPayload(*a.cfg()),
		"invite": map[string]any{"enabled": a.cfg().InviteEnabled},
		"email":  map[string]any{"enabled": emailConfigured(a.cfg()), "force_bind": a.cfg().EmailForceBind},
	})
}

func (a *App) handleAdminConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"host": a.cfg().Host, "port": a.cfg().Port, "redis_enabled": a.redis() != nil, "state_file": a.cfg().StateFile, "upload_dir": a.cfg().UploadDir})
}

func (a *App) handleConfigTOMLGet(w http.ResponseWriter, r *http.Request, _ Params) {
	path := a.configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrConfigFileNotFound, "config file not found")
		return
	}
	// 密钥遮蔽：raw TOML GET 与 schema GET 必须同口径屏蔽真实密钥，否则本接口
	// 会成为绕过 schema 遮蔽、把全部密钥（Postgres DSN、Emby Token、Bot Token、
	// BotInternalSecret、Webhook Secret 等）泄露到浏览器 DOM/缓存/历史的旁路。
	//   - content（规范化渲染）：先在 values 上 maskConfigSecrets 再 render；
	//   - raw_content（磁盘原文）：按 section 上下文做行级 maskTOMLSecrets。
	// 两侧用同一哨兵，completed 比较仍对非密钥字段有效。PUT 路径
	// （handleConfigTOMLPutSafe）会把回传的哨兵还原为真实值，避免写盘覆盖。
	maskedValues := configValues(*a.cfg())
	maskConfigSecrets(maskedValues)
	normalizedContent := stripProtectedAdminConfig(renderConfigTOML(maskedValues))
	rawContent := stripProtectedAdminConfig(maskTOMLSecrets(string(data)))
	ok(w, "OK", map[string]any{"content": normalizedContent, "raw_content": rawContent, "path": path, "completed": normalizedContent != rawContent})
}

func (a *App) handleConfigTOMLPut(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigTOMLPutSafe(w, r, nil)
}

func (a *App) handleConfigSchema(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigSchemaFull(w, r, nil)
}

func (a *App) handleConfigSchemaUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleConfigSchemaUpdateSafe(w, r, nil)
}

func (a *App) handleConfigSweep(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "config check completed", map[string]any{"changed": false, "config_file": a.cfg().ConfigFile})
}

func (a *App) handleAPIRoutes(w http.ResponseWriter, r *http.Request, _ Params) {
	items := make([]map[string]string, 0, len(a.routes))
	for _, route := range a.routes {
		items = append(items, map[string]string{"method": route.Method, "path": strings.TrimPrefix(route.Pattern, "/api/v1"), "endpoint": route.Pattern, "full_path": route.Pattern})
	}
	ok(w, "OK", map[string]any{"apis": items, "total": len(items)})
}

func (a *App) handleBotTest(w http.ResponseWriter, r *http.Request, _ Params) {
	results := []map[string]any{}
	if !a.cfg().TelegramMode {
		results = append(results, map[string]any{"target": "配置", "success": false, "error": "telegram_mode 未启用"})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	if strings.TrimSpace(a.cfg().TelegramBotToken) == "" {
		results = append(results, map[string]any{"target": "Bot Token", "success": false, "error": "未配置 Telegram Bot Token"})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	testCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	me, err := a.telegramGetMe(testCtx)
	if err != nil {
		results = append(results, map[string]any{"target": "Bot getMe", "success": false, "error": err.Error()})
		ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
		return
	}
	botID := int64(numeric(me["id"]))
	username := strings.TrimPrefix(asString(me["username"]), "@")
	results = append(results, map[string]any{"target": "Bot getMe", "success": true, "username": username, "bot_id": zeroNil(botID)})
	for _, chatID := range telegramChatIDs(a.cfg().TelegramGroupIDs) {
		var chat map[string]any
		err := a.telegramPost(testCtx, "getChat", map[string]any{"chat_id": chatID}, &chat)
		item := map[string]any{"target": " 群组 " + chatID, "success": err == nil}
		if err != nil {
			item["error"] = err.Error()
		} else {
			item["title"] = firstNonEmpty(asString(chat["title"]), asString(chat["username"]))
			if botID != 0 {
				if member, memberErr := a.telegramGetChatMember(testCtx, chatID, botID); memberErr != nil {
					item["success"] = false
					item["error"] = memberErr.Error()
				} else {
					item["bot_status"] = asString(member["status"])
				}
			}
		}
		results = append(results, item)
	}
	for _, chatID := range telegramChatIDs(a.cfg().TelegramChannelIDs) {
		var chat map[string]any
		err := a.telegramPost(testCtx, "getChat", map[string]any{"chat_id": chatID}, &chat)
		item := map[string]any{"target": "频道 " + chatID, "success": err == nil}
		if err != nil {
			item["error"] = err.Error()
		} else {
			item["title"] = firstNonEmpty(asString(chat["title"]), asString(chat["username"]))
		}
		results = append(results, item)
	}
	ok(w, "测试完成", map[string]any{"results": results, "runtime": a.telegramRuntimeStatus()})
}

func (a *App) handleEmbyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	// 默认 5s ctx 走 embyHealth；调用方未设置 deadline 时由 doJSONRequestWithTimeout 兜底。
	info, online := a.embyHealth(r.Context())
	if info == nil {
		info = map[string]any{}
	}
	sessions := []map[string]any{}
	if online {
		_ = a.embyGet(r.Context(), "/Sessions", &sessions)
	}
	u := current(r).User
	ok(w, "OK", map[string]any{"online": online, "server_name": firstNonEmpty(asString(info["ServerName"]), asString(info["Name"])), "version": firstNonEmpty(asString(info["Version"]), "unknown"), "operating_system": asString(info["OperatingSystem"]), "active_sessions": len(sessions), "total_sessions": len(sessions), "is_synced": u.EmbyID != "", "is_active": u.Active, "can_unbind": a.userCanSelfUnbindEmby(u), "message": "OK"})
}

func (a *App) handleDeprecatedEmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	failWithCode(w, http.StatusGone, ErrAPIDeprecated, "该接口已废弃，请使用 /api/v1/system/emby-urls")
}

func (a *App) handleEmbyLatest(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"items": []any{}, "total": 0})
		return
	}
	limit := clamp(queryInt(r, "limit", 20), 1, 100)
	itemTypes := firstNonEmpty(r.URL.Query().Get("item_types"), "Movie,Series")
	query := embyItemQuery(map[string]string{
		"IncludeItemTypes": itemTypes,
		"Recursive":        "true",
		"SortBy":           "DateCreated",
		"SortOrder":        "Descending",
		"Limit":            strconv.Itoa(limit),
		"Fields":           "Overview,ProviderIds,DateCreated,ProductionYear,PremiereDate",
	})
	var payload map[string]any
	if err := a.embyGet(r.Context(), "/Items"+query, &payload); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyLatestFailed, "failed to read latest Emby media")
		return
	}
	items, _ := payload["Items"].([]any)
	ok(w, "OK", map[string]any{"items": items, "total": int(numeric(payload["TotalRecordCount"]))})
}

func (a *App) handleSessionCount(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.embyConfigured() {
		ok(w, "OK", map[string]any{"active": 0, "total": 0})
		return
	}
	var sessions []map[string]any
	if err := a.embyGet(r.Context(), "/Sessions", &sessions); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyRemoteSessionsFail, "failed to read Emby sessions")
		return
	}
	active := 0
	for _, session := range sessions {
		if boolish(session["IsActive"]) || session["NowPlayingItem"] != nil {
			active++
		}
	}
	ok(w, "OK", map[string]any{"active": active, "total": len(sessions)})
}

func (a *App) handleAdminUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	users := a.store().ListUsers()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	roleFilter := r.URL.Query().Get("role")
	activeFilter := r.URL.Query().Get("active")
	embyFilter := r.URL.Query().Get("emby")
	embyStatusFilter := r.URL.Query().Get("emby_status")
	emailFilter := r.URL.Query().Get("email_status")
	items := make([]map[string]any, 0, len(users))
	for _, u := range users {
		if roleFilter != "" && strconv.Itoa(u.Role) != roleFilter {
			continue
		}
		if activeFilter == "true" && !u.Active {
			continue
		}
		if activeFilter == "false" && u.Active {
			continue
		}
		if embyFilter == "bound" && u.EmbyID == "" {
			continue
		}
		if embyFilter == "unbound" && u.EmbyID != "" {
			continue
		}
		// emby_status 筛选 Emby 账号的启停状态（与 Web 账号 active 独立）：
		// active  = Emby 已绑定且未被禁用（远端 + 到期均未禁用）
		// disabled = Emby 已绑定但被禁用（远端禁用 或 到期自动禁用）
		switch embyStatusFilter {
		case "active":
			if u.EmbyID == "" || u.EmbyDisabled {
				continue
			}
			if u.Role == store.RoleNormal && u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix() {
				continue
			}
		case "disabled":
			if u.EmbyID == "" {
				continue
			}
			disabled := u.EmbyDisabled
			if !disabled && u.Role == store.RoleNormal && u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix() {
				disabled = true
			}
			if !disabled {
				continue
			}
		}
		// 邮箱验证管理区筛选：verified=已验证；unverified=已填邮箱但未验证；
		// bound=已填邮箱（不论验证）；none=未填邮箱。
		switch emailFilter {
		case "verified":
			if !u.EmailVerified {
				continue
			}
		case "unverified":
			if u.EmailVerified || strings.TrimSpace(u.Email) == "" {
				continue
			}
		case "bound":
			if strings.TrimSpace(u.Email) == "" {
				continue
			}
		case "none":
			if strings.TrimSpace(u.Email) != "" {
				continue
			}
		}
		if search != "" && !strings.Contains(strings.ToLower(u.Username+" "+u.Email+" "+u.EmbyID+" "+strconv.FormatInt(u.UID, 10)+" "+strconv.FormatInt(u.TelegramID, 10)), search) {
			continue
		}
		items = append(items, publicUser(u))
	}
	sortUsers(items, r.URL.Query().Get("sort"))
	total := len(items)
	items = paginate(items, page, perPage)
	ok(w, "OK", map[string]any{"users": items, "total": total, "page": page, "per_page": perPage, "pages": pages(total, perPage)})
}

func (a *App) handleAdminCreateUser(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	username := strings.TrimSpace(stringValue(payload, "username"))
	if err := validate.ValidateUsername(username); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
		return
	}
	if _, exists := a.store().FindUserByUsername(username); exists {
		failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名已被占用，请换一个用户名")
		return
	}
	role := store.RoleNormal
	if rawRole, ok := payload["role"]; ok {
		parsed, valid := normalizeRoleValue(rawRole)
		if !valid || parsed == store.RoleUnrecognized {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "role 取值非法")
			return
		}
		role = parsed
	}
	telegramID := int64(intValue(payload, "telegram_id", 0))
	if telegramID < 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGBindTGIDInvalid, "Telegram ID 无效")
		return
	}
	if telegramID != 0 {
		if existing, okUser := a.store().FindUserByTelegramID(telegramID); okUser {
			failWithCode(w, http.StatusConflict, ErrTGAlreadyBound, "该 Telegram 已绑定到账号 "+existing.Username)
			return
		}
	}
	password := stringValue(payload, "password")
	autoGenerated := password == ""
	if autoGenerated {
		password = "Twilight-" + randomCode(generatedPasswordHexLen)
	} else if err := validate.ValidatePasswordStrength(password); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, err.Error())
		return
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "密码处理失败")
		return
	}
	expiredAt := numeric(payload["expired_at"])
	if _, ok := payload["days"]; ok {
		days := intValue(payload, "days", -1)
		if days < 0 {
			expiredAt = permanentExpiryUnix
		} else {
			expiredAt = time.Now().AddDate(0, 0, days).Unix()
		}
	}
	if expiryIsPermanent(expiredAt) {
		expiredAt = permanentExpiryUnix
	}
	if email := stringValue(payload, "email"); email != "" {
		if err := validate.ValidateEmailFormat(email); err != nil {
			failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
			return
		}
	}
	createdUser, err := a.store().CreateUser(store.User{
		Username:         username,
		Email:            stringValue(payload, "email"),
		PasswordHash:     hash,
		Role:             role,
		ExpiredAt:        expiredAt,
		TelegramID:       telegramID,
		TelegramUsername: strings.TrimPrefix(strings.TrimSpace(stringValue(payload, "telegram_username")), "@"),
	})
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrUsernameTaken, "用户名或 Telegram 已被占用")
		return
	}
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "admin_create_user", "admin", createdUser.UID, map[string]any{"username": createdUser.Username})
	created(w, "Web account created", map[string]any{"user": publicUser(createdUser), "password": password, "auto_generated": autoGenerated})
}

func (a *App) handleAdminUser(w http.ResponseWriter, r *http.Request, params Params) {
	u, okUser := a.userFromPath(w, params, "uid")
	if !okUser {
		return
	}
	ok(w, "OK", publicUser(u))
}

func (a *App) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	// 在进入 store.UpdateUser 之前先把字段校验跑完：role 必须在已知集合内，
	// username 必须通过 validate.ValidateUsername，否则会出现：
	//   - admin 给用户写入未知 role（如 99），前端 roleName 落到"未识别"，
	//     该用户无法被任何鉴权分支处理；
	//   - admin 改写的 username 包含禁止字符 / 长度越界，导致用户后续无法登录。
	var (
		hasRole     bool
		desiredRole int
	)
	if rawRole, ok := payload["role"]; ok {
		role, valid := normalizeRoleValue(rawRole)
		if !valid {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "role 取值非法")
			return
		}
		hasRole = true
		desiredRole = role
	}
	if rawUsername, ok := payload["username"]; ok {
		if username, isStr := rawUsername.(string); isStr && username != "" {
			if err := validate.ValidateUsername(username); err != nil {
				failWithCode(w, http.StatusBadRequest, ErrUsernameInvalid, err.Error())
				return
			}
		}
	}
	if rawEmail, ok := payload["email"]; ok {
		if email, isStr := rawEmail.(string); isStr && email != "" {
			if err := validate.ValidateEmailFormat(email); err != nil {
				failWithCode(w, http.StatusBadRequest, ErrEmailInvalid, err.Error())
				return
			}
		}
	}
	// active 字段必须走 SetUserActiveAtomic：直接在 UpdateUser 闭包里
	// 写 u.Active = false 会绕过 last-admin 守卫，最后一名 active admin 会被
	// 任何带 active=false 的 PATCH 静默禁用，等价于 self-disable 自残。
	currentUID := current(r).User.UID
	hasActive := false
	desiredActive := true
	if raw, ok := payload["active"]; ok {
		hasActive = true
		switch v := raw.(type) {
		case bool:
			desiredActive = v
		case string:
			desiredActive = strings.EqualFold(strings.TrimSpace(v), "true")
		case float64:
			desiredActive = v != 0
		default:
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "active 取值非法")
			return
		}
		if !desiredActive && uid == currentUID {
			failWithCode(w, http.StatusForbidden, ErrUserProtected, "无法禁用自己的账号")
			return
		}
	}
	// role 写入走 store.SetUserRoleAtomic：在同一把锁里做"读取目标当前 role +
	// 计数 active admin + 写入"，避免两个 admin 并发降级两个不同 admin 时
	// 同时通过 last-admin 校验导致 0 admin。其它字段再走 UpdateUser 闭包。
	if hasRole {
		if _, err := a.store().SetUserRoleAtomic(uid, desiredRole); err != nil {
			if errors.Is(err, store.ErrLastAdmin) {
				failWithCode(w, http.StatusConflict, ErrAdminLastAdminProtected, "无法移除最后一个管理员的权限，系统至少需要一个管理员")
				return
			}
			if statusFromError(w, err) {
				return
			}
		}
	}
	embySyncFailed := false
	if hasActive {
		updated, err := a.store().SetUserActiveAtomic(uid, desiredActive)
		if err != nil {
			if errors.Is(err, store.ErrLastAdmin) {
				failWithCode(w, http.StatusConflict, ErrAdminLastAdminProtected, "无法禁用最后一个管理员，系统至少需要一个管理员")
				return
			}
			if statusFromError(w, err) {
				return
			}
		}
		if !desiredActive {
			if _, syncErr := a.disableRemoteEmbyForWebState(r.Context(), updated); syncErr != nil {
				embySyncFailed = true
			}
		}
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if username := stringValue(payload, "username"); username != "" {
			u.Username = username
		}
		if email := stringValue(payload, "email"); email != "" {
			u.Email = email
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := publicUser(u)
	if embySyncFailed {
		data["emby_sync_failed"] = true
	}
	a.audit(r, "update_user", "admin", uid, map[string]any{
		"has_role": hasRole, "has_active": hasActive,
	})
	ok(w, "user updated", data)
}

// normalizeRoleValue 把任意 JSON 数值收成已知 role 整数。
// 仅 RoleAdmin / RoleNormal / RoleWhitelist / RoleUnrecognized 视为合法。
// 用于 admin update / set-role 等所有写 user.Role 的入口，避免持久化未知值。
func normalizeRoleValue(v any) (int, bool) {
	var role int
	switch n := v.(type) {
	case float64:
		role = int(n)
	case int:
		role = n
	case int64:
		role = int(n)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		role = parsed
	default:
		return 0, false
	}
	switch role {
	case store.RoleAdmin, store.RoleNormal, store.RoleWhitelist, store.RoleUnrecognized:
		return role, true
	default:
		return 0, false
	}
}

func (a *App) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	if uid == current(r).User.UID {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "cannot delete current admin")
		return
	}
	payload := decodeMap(r)
	mode := firstNonEmpty(stringValue(payload, "mode"), "local_only")
	if mode != "local_only" && mode != "with_emby" && mode != "emby_only" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "mode 必须是 local_only、with_emby 或 emby_only")
		return
	}
	depth := intValue(payload, "cascade_depth", queryInt(r, "cascade_depth", 1))
	deleted := []int64{}
	skipped := []map[string]any{}
	failed := []map[string]any{}
	for _, targetUID := range a.collectCascadeUIDs(uid, depth) {
		target, okUser := a.store().User(targetUID)
		if !okUser {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": "not_found"})
			continue
		}
		if a.userIsProtected(target) {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": a.protectedUserReason(target)})
			continue
		}
		if mode == "emby_only" {
			if target.EmbyID != "" {
				if !a.embyConfigured() {
					failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: emby not configured"})
					continue
				}
				if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil && !strings.Contains(err.Error(), "remote status 404") {
					failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: " + err.Error()})
					continue
				}
			}
			_, err := a.store().UpdateUser(targetUID, func(u *store.User) error { u.EmbyID = ""; u.EmbyUsername = ""; return nil })
			if err != nil {
				failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			} else {
				a.store().ClearEmbyGrantForUnboundUsers([]int64{targetUID})
				deleted = append(deleted, targetUID)
			}
			continue
		}
		if mode == "with_emby" && target.EmbyID != "" {
			if !a.embyConfigured() {
				failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: emby not configured"})
				continue
			}
			if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil && !strings.Contains(err.Error(), "remote status 404") {
				failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: " + err.Error()})
				continue
			}
		}
		if err := a.store().DeleteUser(targetUID); err != nil {
			failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			continue
		}
		deleted = append(deleted, targetUID)
	}
	// 审计日志
	if len(deleted) > 0 {
		a.audit(r, "delete_user", "admin", uid, map[string]any{"deleted": deleted, "mode": mode, "cascade_depth": depth})
	}
	ok(w, "users deleted", map[string]any{"deleted": deleted, "skipped": skipped, "failed": failed, "mode": mode, "cascade_depth": depth})
}

func (a *App) handleAdminToggleUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	enable := strings.HasSuffix(r.URL.Path, "/enable")
	currentUID := current(r).User.UID
	// admin 不允许 disable 自己：自残会让 admin 立即丢失 active 状态、踢自己下线，
	// 同时绕开 store.SetUserActiveAtomic 的"剩余 active admin >= 1" 兜底。
	if !enable && uid == currentUID {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "无法禁用自己的账号")
		return
	}
	if target, okUser := a.store().User(uid); okUser && a.userIsProtected(target) && target.UID != currentUID {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "cannot operate on protected user")
		return
	}
	payload := decodeMap(r)
	depth := intValue(payload, "cascade_depth", queryInt(r, "cascade_depth", 1))
	// 禁用理由可选，仅在禁用方向有意义，超长截断防止审计 detail 膨胀。
	reason := strings.TrimSpace(stringValue(payload, "reason"))
	if len(reason) > 200 {
		reason = reason[:200]
	}
	affected := []int64{}
	skipped := []map[string]any{}
	failed := []map[string]any{}
	embyFailed := []map[string]any{}
	for _, targetUID := range a.collectCascadeUIDs(uid, depth) {
		if target, okUser := a.store().User(targetUID); okUser && a.userIsProtected(target) && target.UID != currentUID {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": a.protectedUserReason(target)})
			continue
		}
		// 走 SetUserActiveAtomic：禁用最后一个 active admin 时返回 ErrLastAdmin，
		// 在级联场景下被记入 skipped 而不是悄悄通过。
		updated, err := a.store().SetUserActiveAtomic(targetUID, enable)
		if err != nil {
			if errors.Is(err, store.ErrLastAdmin) {
				skipped = append(skipped, map[string]any{"uid": targetUID, "reason": "last_admin"})
				continue
			}
			failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			continue
		}
		if !enable {
			// Emby 同步失败不阻断本地禁用，但记录到响应让管理员知晓。
			if _, syncErr := a.disableRemoteEmbyForWebState(r.Context(), updated); syncErr != nil {
				embyFailed = append(embyFailed, map[string]any{"uid": targetUID, "reason": syncErr.Error()})
			}
		}
		affected = append(affected, targetUID)
	}
	u, _ := a.store().User(uid)
	resp := map[string]any{"user": publicUser(u), "active": enable, "affected": affected, "skipped": skipped, "failed": failed, "cascade_depth": depth, "enable": enable}
	if len(embyFailed) > 0 {
		resp["emby_failed"] = embyFailed
	}
	// 审计日志
	if len(affected) > 0 {
		action := "disable_user"
		if enable {
			action = "enable_user"
		}
		detail := map[string]any{"affected": affected, "cascade_depth": depth}
		// 仅禁用方向记录理由，避免给启用操作附带无意义的旧理由。
		if !enable && reason != "" {
			detail["reason"] = reason
		}
		a.audit(r, action, "admin", uid, detail)
	}
	ok(w, "用户状态已更新", resp)
}

// handleAdminToggleEmby 单独启停用户的 Emby 账号，不改动其 Web 账号状态。
// 与「禁用 Web 账号顺带关停 Emby」相反：管理员有时只想停掉某人的 Emby 观看权限，
// 但保留其网页端登录（看公告、改资料、续费）。守卫与 Telegram 面板的同名操作对齐：
// 受保护账号不可操作；未绑定 Emby / 未配置 Emby 直接拒绝；启用方向必须满足
// embyShouldEnableUser（Web 处于 active 且未过期），不得绕过有效期把 Emby 重新放开。
func (a *App) handleAdminToggleEmby(w http.ResponseWriter, r *http.Request, params Params) {
	enable := strings.HasSuffix(r.URL.Path, "/enable")
	target, okUser := a.userFromPath(w, params, "uid")
	if !okUser {
		return
	}
	if a.userIsProtected(target) {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "受保护账号禁止单独修改 Emby 状态")
		return
	}
	if target.EmbyID == "" {
		failWithCode(w, http.StatusBadRequest, ErrEmbyAccountUnlinked, "目标用户未绑定 Emby 账号")
		return
	}
	if !a.embyConfigured() {
		failWithCode(w, http.StatusBadGateway, ErrEmbyNotConfigured, "Emby URL 或 API Token 未配置")
		return
	}
	if enable && !a.embyShouldEnableUser(target) {
		failWithCode(w, http.StatusConflict, ErrConflict, "Web 账号已禁用或已过期，禁止绕过有效期直接启用 Emby")
		return
	}
	// 禁用理由可选，仅在禁用方向有意义，超长截断防止审计 detail 膨胀。
	payload := decodeMap(r)
	reason := strings.TrimSpace(stringValue(payload, "reason"))
	if len(reason) > 200 {
		reason = reason[:200]
	}
	if err := a.embyApplyEnabledState(r.Context(), target.UID, target.EmbyID, enable); err != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyDisableFailed, "Emby 状态更新失败")
		return
	}
	action := "admin_disable_emby"
	if enable {
		action = "admin_enable_emby"
	}
	var detail map[string]any
	// 仅禁用方向记录理由，避免给启用操作附带无意义的旧理由。
	if !enable && reason != "" {
		detail = map[string]any{"reason": reason}
	}
	a.audit(r, action, "admin", target.UID, detail)
	ok(w, "Emby 状态已更新", map[string]any{"uid": target.UID, "emby_enabled": enable})
}

func (a *App) handleAdminUnbindEmby(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	target, okUser := a.store().User(uid)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	embyID := target.EmbyID
	remoteDisabled, proceed := a.disableRemoteEmbyForUnbind(w, r, embyID)
	if !proceed {
		return
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if embyID != "" && u.EmbyID != "" && u.EmbyID != embyID {
			return store.ErrConflict
		}
		u.EmbyID = ""
		u.EmbyUsername = ""
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	data := publicUser(u)
	data["remote_emby_disabled"] = remoteDisabled
	data["old_emby_id"] = embyID
	ok(w, "Emby unbound", data)
}

func (a *App) handleAdminForceUnbind(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	target, okUser := a.store().User(uid)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	scope := strings.ToLower(stringValue(decodeMap(r), "scope"))
	if scope == "" {
		scope = "both"
	}
	if scope != "telegram" && scope != "emby" && scope != "both" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "scope 必须是 telegram、emby 或 both")
		return
	}
	unbindTelegram := scope == "telegram" || scope == "both"
	unbindEmby := scope == "emby" || scope == "both"
	oldTelegramID := target.TelegramID
	oldEmbyID := target.EmbyID
	remoteDisabled := false
	if unbindEmby {
		var proceed bool
		remoteDisabled, proceed = a.disableRemoteEmbyForUnbind(w, r, oldEmbyID)
		if !proceed {
			return
		}
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if unbindTelegram {
			u.TelegramID = 0
			u.TelegramUsername = ""
		}
		if unbindEmby {
			if oldEmbyID != "" && u.EmbyID != "" && u.EmbyID != oldEmbyID {
				return store.ErrConflict
			}
			u.EmbyID = ""
			u.EmbyUsername = ""
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	changed := []string{}
	if unbindTelegram && oldTelegramID != 0 {
		changed = append(changed, "telegram")
	}
	if unbindEmby && oldEmbyID != "" {
		changed = append(changed, "emby")
	}
	a.audit(r, "admin_force_unbind", "admin", uid, map[string]any{"changed": changed})
	ok(w, "unbound", map[string]any{
		"changed":              changed,
		"changed_any":          len(changed) > 0,
		"old":                  map[string]any{"telegram_id": nullableInt(oldTelegramID), "emby_id": emptyNil(oldEmbyID)},
		"remote_emby_disabled": remoteDisabled,
		"user":                 publicUser(u),
	})
}

func (a *App) handleRegistrationQueueClear(w http.ResponseWriter, r *http.Request, params Params) {
	if params["uid"] != "" {
		uid, _ := int64Param(params, "uid")
		u, err := a.store().UpdateUser(uid, func(u *store.User) error { u.PendingEmby = false; u.PendingEmbyDays = nil; return nil })
		if statusFromError(w, err) {
			return
		}
		ok(w, "注册队列状态已清理", map[string]any{"uid": uid, "user": publicUser(u)})
		return
	}
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", true)
	confirm := stringValue(payload, "confirm")
	candidates := []int64{}
	for _, u := range a.store().ListUsers() {
		if u.PendingEmby {
			candidates = append(candidates, u.UID)
		}
	}
	if dryRun || confirm != "CLEAR_REGISTRATION_QUEUE" {
		ok(w, "dry-run", map[string]any{"dry_run": true, "candidates": candidates, "count": len(candidates), "confirm_required": "CLEAR_REGISTRATION_QUEUE"})
		return
	}
	updated := 0
	failed := []int64{}
	for _, uid := range candidates {
		if _, err := a.store().UpdateUser(uid, func(u *store.User) error { u.PendingEmby = false; u.PendingEmbyDays = nil; return nil }); err != nil {
			failed = append(failed, uid)
			continue
		}
		updated++
	}
	if len(failed) > 0 {
		failWithCode(w, http.StatusInternalServerError, ErrAdminQueueClearPartial, "部分注册队列状态清理失败")
		return
	}
	ok(w, "registration queue cleaned", map[string]any{"updated": updated, "uids": candidates})
}

func (a *App) handleRegistrationEntitlement(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	days := normalizeRegCodeDays(intValue(decodeMap(r), "days", a.cfg().InviteDefaultDays))
	if days < -1 || days > 3650 {
		failWithCode(w, http.StatusBadRequest, ErrAdminDaysOutOfRange, "days 超出允许范围")
		return
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if u.EmbyID != "" {
			return store.ErrConflict
		}
		u.PendingEmby = true
		u.PendingEmbyDays = &days
		markRegistrationGrant(u, registrationSourceAdminGrant, "")
		if u.Role == store.RoleUnrecognized {
			u.Role = store.RoleNormal
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Emby access granted", map[string]any{"uid": uid, "days": days, "user": publicUser(u)})
}

func (a *App) handleRegistrationEntitlementBulk(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	dryRun := boolValue(payload, "dry_run", true)
	confirm := stringValue(payload, "confirm")
	days := normalizeRegCodeDays(intValue(payload, "days", a.cfg().InviteDefaultDays))
	if days < -1 || days > 3650 {
		failWithCode(w, http.StatusBadRequest, ErrAdminDaysOutOfRange, "days 超出允许范围")
		return
	}
	candidates := []int64{}
	for _, u := range a.store().ListUsers() {
		if u.PendingEmby && u.EmbyID == "" && u.Active {
			candidates = append(candidates, u.UID)
		}
	}
	if dryRun || confirm != "GRANT_AND_CLEAR_REGISTRATION_QUEUE" {
		ok(w, "dry-run", map[string]any{"dry_run": true, "candidates": candidates, "count": len(candidates), "confirm_required": "GRANT_AND_CLEAR_REGISTRATION_QUEUE"})
		return
	}
	updated := 0
	failed := []int64{}
	for _, uid := range candidates {
		if _, err := a.store().UpdateUser(uid, func(u *store.User) error {
			u.PendingEmby = true
			u.PendingEmbyDays = &days
			markRegistrationGrant(u, registrationSourceAdminGrant, "")
			if u.Role == store.RoleUnrecognized {
				u.Role = store.RoleNormal
			}
			return nil
		}); err != nil {
			failed = append(failed, uid)
			continue
		}
		updated++
	}
	if len(failed) > 0 {
		failWithCode(w, http.StatusInternalServerError, ErrAdminEntitlementPartial, "部分 Emby 注册资格发放失败")
		return
	}
	ok(w, "batch access granted", map[string]any{"updated": updated, "uids": candidates, "days": days})
}

func (a *App) handleSyncBindings(w http.ResponseWriter, r *http.Request, _ Params) {
	seenTG := map[int64]int64{}
	duplicates := []map[string]any{}
	missingEmby := []int64{}
	checked := 0
	for _, u := range a.store().ListUsers() {
		checked++
		if u.TelegramID != 0 {
			if previous, ok := seenTG[u.TelegramID]; ok {
				duplicates = append(duplicates, map[string]any{"telegram_id": u.TelegramID, "uids": []int64{previous, u.UID}})
			} else {
				seenTG[u.TelegramID] = u.UID
			}
		}
		if u.EmbyID != "" && a.cfg().EmbyURL != "" {
			var remote map[string]any
			if err := a.embyGet(r.Context(), "/Users/"+urlPathEscape(u.EmbyID), &remote); err != nil {
				missingEmby = append(missingEmby, u.UID)
			}
		}
	}
	ok(w, "binding sync checked", map[string]any{"matched": checked, "telegram_duplicates": duplicates, "emby_missing": missingEmby, "repaired": 0, "synced": checked - len(missingEmby), "failed": len(missingEmby)})
}

func (a *App) handleKickUser(w http.ResponseWriter, r *http.Request, params Params) {
	u, okUser := a.userFromPath(w, params, "uid")
	if !okUser {
		return
	}
	kicked := a.kickEmbySessions(r.Context(), u.EmbyID)
	ok(w, "会话踢出完成", map[string]any{"kicked_count": kicked})
}

func (a *App) handleAdminRenewUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	days := int64(normalizeRegCodeDays(intValue(payload, "days", 30)))
	if expiredAt := numeric(payload["expired_at"]); expiredAt < 0 || expiryIsPermanent(expiredAt) {
		days = -1
	}
	// 与 handleAdminSetUserExpiry / handleBatchRenewUsers / handleAdminBulkExpire
	// 对齐：正天数必须有上限。少了这道闸，base+days*86400 既可能因 days 巨大直接
	// 落进 permanentExpiryUnix 区间（静默把账号变永久），更大的值还会让 int64
	// 乘法溢出回绕成负数 → expiryIsPermanent 也判真。days<0 是显式永久路径，放行。
	if days > 36500 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "days 不能超过 36500")
		return
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if days < 0 {
			renewExpiryAndReactivate(u, permanentExpiryUnix)
			return nil
		}
		base := time.Now().Unix()
		if u.ExpiredAt > base {
			base = u.ExpiredAt
		}
		// admin renew 走统一助手：被 check_expired 自动禁用的账号在续费成功
		// 后必须自动恢复登录，否则 admin 续完看到用户报"账号被禁"再来一次
		// 手动 enable，这一步漏掉就违反 R62 不变量。
		renewExpiryAndReactivate(u, base+days*86400)
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "admin_renew_user", "admin", uid, map[string]any{"days": days})
	ok(w, "续期成功", publicUser(u))
}

// handleAdminSetUserExpiry 绝对式设置用户到期时间，区别于 handleAdminRenewUser 的
// 叠加续期：这里直接把 ExpiredAt 覆盖为「从现在起 N 天后」或「永久」，不读取旧值做
// base。它服务两条路由：
//   - /cancel-permanent：把永久号改成 N 天后到期。旧实现复用叠加式 renew，base 取
//     max(now, ExpiredAt)，而永久号的 ExpiredAt == permanentExpiryUnix（2534 年），
//     于是 base+days 依旧 >= permanentExpiryUnix → expiryIsPermanent 仍为真，"取消
//     永久"实际什么都没改。绝对式设置修掉这个静默失败。
//   - /set-expiry：管理员手动把任意用户的剩余到期时间设为精确值（覆盖，不叠加）。
//
// days < 0 或 permanent=true 或 expired_at 永久哨兵 → 设永久；否则 days 必须为正整数，
// 上限 36500（与批量续期对齐），避免溢出或意外落进永久区间。
func (a *App) handleAdminSetUserExpiry(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	permanent := boolValue(payload, "permanent", false)
	if expiredAt := numeric(payload["expired_at"]); expiredAt < 0 || expiryIsPermanent(expiredAt) {
		permanent = true
	}
	rawDays := intValue(payload, "days", 0)
	if rawDays < 0 {
		permanent = true
	}
	if !permanent {
		if rawDays <= 0 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "days 必须为正整数，或选择永久")
			return
		}
		if rawDays > 36500 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "days 不能超过 36500")
			return
		}
	}
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if permanent {
			renewExpiryAndReactivate(u, permanentExpiryUnix)
			return nil
		}
		// 绝对式：始终以 now 为基准，覆盖旧到期时间（含永久哨兵）。
		renewExpiryAndReactivate(u, time.Now().Unix()+int64(rawDays)*86400)
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "admin_set_expiry", "admin", uid, map[string]any{"days": rawDays, "permanent": permanent})
	ok(w, "到期时间已更新", publicUser(u))
}

func (a *App) handleAdminResetPassword(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	if uid == 0 {
		uid = current(r).User.UID
	}
	body := decodeMap(r)
	scope := strings.ToLower(firstNonEmpty(stringValue(body, "scope"), "both"))
	if scope != "system" && scope != "emby" && scope != "both" {
		failWithCode(w, http.StatusBadRequest, ErrAdminPasswordResetScope, "invalid password reset scope")
		return
	}
	targetUser, okUser := a.store().User(uid)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return
	}
	newPassword := stringValue(body, "password")
	autoGenerated := newPassword == ""
	if autoGenerated {
		// 与 handleGeneratedPassword 保持一致：≥128 bit 熵。
		newPassword = "Twilight-" + randomCode(generatedPasswordHexLen)
	} else if okPass, msg := validateStrongPassword(newPassword, "password"); !okPass {
		failWithCode(w, http.StatusBadRequest, ErrPasswordWeak, msg)
		return
	}
	if (scope == "emby" || scope == "both") && targetUser.EmbyID == "" {
		// 旧实现里 scope=both 在 EmbyID 空时会"静默降级为 system"，前端无法察觉
		// 这次写入并未触达 Emby。改为返回 409 让前端显式选择"仅系统密码"或先绑 Emby。
		failWithCode(w, http.StatusConflict, ErrEmbyAccountUnlinked, "user has no linked Emby account; choose scope=system to reset only the system password")
		return
	}
	if scope == "emby" || scope == "both" {
		if err := a.embySetPassword(r.Context(), targetUser.EmbyID, newPassword); err != nil {
			failWithCode(w, http.StatusBadGateway, ErrAdminEmbyPasswordReset, "failed to reset Emby password")
			return
		}
	}
	if scope == "system" || scope == "both" {
		hashValue, err := security.HashPassword(newPassword)
		if err != nil {
			failWithCode(w, http.StatusInternalServerError, ErrPasswordHashFailed, "password processing failed")
			return
		}
		if _, updateErr := a.store().UpdateUser(uid, func(u *store.User) error { u.PasswordHash = hashValue; return nil }); updateErr != nil {
			statusFromError(w, updateErr)
			return
		}
		// 写完新密码必须立即吊销目标用户的所有会话：admin "重置密码" 的核心
		// 用途就是止血账号接管，旧实现却没踢 cookie，被盗 token 在 SessionTTL
		// 内仍然有效；与 handleAdminLogoutUser 保持一致，写入即吊销。
		a.sessions().DeleteUser(r.Context(), uid)
	}
	a.audit(r, "admin_reset_password", "admin", uid, map[string]any{"scope": scope, "auto_generated": autoGenerated})
	ok(w, "password reset", map[string]any{"scope": scope, "new_password": newPassword, "auto_generated": autoGenerated})
}

func (a *App) handleAdminSetRole(w http.ResponseWriter, r *http.Request, params Params) {
	if requireAdmin(w, r) {
		return
	}
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	role := store.RoleNormal
	if boolValue(payload, "is_admin", boolValue(payload, "admin", false)) {
		role = store.RoleAdmin
	}
	// SetUserRoleAtomic 在同一把写锁内做 last-admin 计数 + 写入。
	// 旧实现把 ListUsers 计数与 UpdateUser 闭包分两段执行，并发降级两个不同 admin
	// 时各自看到 adminCount=2 都通过校验，事后剩 0 admin。
	u, err := a.store().SetUserRoleAtomic(uid, role)
	if err != nil {
		if errors.Is(err, store.ErrLastAdmin) {
			failWithCode(w, http.StatusConflict, ErrAdminLastAdminProtected, "无法移除最后一个管理员的权限，系统至少需要一个管理员")
			return
		}
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "set_role", "admin", uid, map[string]any{
		"new_role": role,
	})
	ok(w, "role updated", publicUser(u))
}

func (a *App) handleAdminUnbindTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	old := int64(0)
	u, err := a.store().UpdateUser(uid, func(u *store.User) error {
		if u.Role == store.RoleAdmin && u.UID != current(r).User.UID {
			return store.ErrConflict
		}
		old = u.TelegramID
		u.TelegramID = 0
		u.TelegramUsername = ""
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "admin_unbind_telegram", "admin", uid, map[string]any{"old_telegram_id": old})
	ok(w, "Telegram unbound", map[string]any{"uid": u.UID, "username": u.Username, "old_telegram_id": old})
}

func (a *App) handleAdminBindTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	tgid := int64(intValue(payload, "telegram_id", 0))
	if tgid <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrTGIDInvalid, "telegram_id 无效")
		return
	}
	// BindUserTelegramAtomic 在同一把写锁里做唯一性 + admin 自保 + 写入，
	// 替换旧实现里 FindUserByTelegramID(RLock) → UpdateUser(Lock) 两段独立锁，
	// 关闭并发同 telegram_id 绑定到两个 UID 的 TOCTOU 窗口。
	u, old, err := a.store().BindUserTelegramAtomic(uid, tgid, current(r).User.UID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			// store.ErrConflict 同时承载两类语义：目标是其他 admin 不允许改写，
			// 或 telegram_id 已被占用。优先识别后者（语义更精确）。
			if existing, okUser := a.store().FindUserByTelegramID(tgid); okUser && existing.UID != uid {
				failWithCode(w, http.StatusConflict, ErrTGIDTaken, "Telegram ID is already bound to another user")
				return
			}
		}
		if statusFromError(w, err) {
			return
		}
	}
	ok(w, "Telegram bound", map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": u.TelegramID, "old_telegram_id": zeroNil(old)})
}

func (a *App) handleUserByTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	tgid, _ := int64Param(params, "telegram_id")
	for _, u := range a.store().ListUsers() {
		if u.TelegramID == tgid {
			ok(w, "OK", publicUser(u))
			return
		}
	}
	failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
}

func (a *App) handleEmbySync(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleEmbySyncV2(w, r, nil)
}
func (a *App) handleAdminEmbyUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleAdminEmbyUsersV2(w, r, nil)
}
func (a *App) handleEmbyTest(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleEmbyTestV2(w, r, nil)
}
func (a *App) handleCountZero(w http.ResponseWriter, r *http.Request, _ Params) {
	result := a.cleanupUnusedUploadAssets(24 * time.Hour)
	ok(w, "OK", map[string]any{"count": result["deleted"], "cleaned": result["deleted"], "scanned": result["scanned"], "failed": result["failed"]})
}

func (a *App) handleCreateStandaloneEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleCreateStandaloneEmbyV2(w, r, nil)
}

func (a *App) handleAdminBindEmby(w http.ResponseWriter, r *http.Request, params Params) {
	// 显式 admin 断言。AuthAdmin 路由（routes.go:177）已挡住非 admin，但 admin
	// 强力 mutating handler 必须自己再确认一次：未来若有人把这个 handler 同
	// 时挂到 AuthUser 路由（手抖 / 复制粘贴），dispatcher 不会拦下，handler
	// 这一行会。
	if requireAdmin(w, r) {
		return
	}
	targetUID, _ := int64Param(params, "uid")
	body := decodeMap(r)
	embyIDInput := stringValue(body, "emby_id")
	embyNameInput := stringValue(body, "emby_username")
	force := boolValue(body, "force", false)
	var remoteUser map[string]any
	var found bool
	var lookupErr error
	if embyIDInput != "" {
		remoteUser, found, lookupErr = a.embyUserByID(r.Context(), embyIDInput)
	} else {
		remoteUser, found, lookupErr = a.embyUserByName(r.Context(), embyNameInput)
	}
	if lookupErr != nil {
		failWithCode(w, http.StatusBadGateway, ErrEmbyUserLookupFailed, "failed to read Emby user")
		return
	}
	if !found {
		failWithCode(w, http.StatusNotFound, ErrEmbyUserNotFound, "Emby user not found")
		return
	}
	embyID := asString(remoteUser["Id"])
	embyName := firstNonEmpty(asString(remoteUser["Name"]), embyNameInput, embyID)
	updatedUser, displacedUID, updateErr := a.store().BindUserEmbyAtomic(targetUID, embyID, embyName, force)
	if errors.Is(updateErr, store.ErrConflict) {
		existing, _ := a.store().FindUserByEmbyID(embyID)
		// 旧实现走 ok() 200 + {"conflict": true, ...}：前端只能依赖隐式
		// 约定查 data.conflict，与其他 admin handler 的"重名/已存在"统一
		// 走 409 + ErrCode 不一致。改为 failWithCodeData：envelope
		// success=false + code=EMBY_ACCOUNT_CONFLICT + 同样的 conflict 元
		// 数据。前端用 errorCode 判定即可，无需再读 data.conflict。
		failWithCodeData(w, http.StatusConflict, ErrEmbyAccountConflict, "Emby account already linked", map[string]any{
			"conflict_uid":      existing.UID,
			"conflict_username": existing.Username,
			"emby_id":           embyID,
			"emby_username":     embyName,
		})
		return
	}
	if statusFromError(w, updateErr) {
		return
	}
	_ = a.embyApplyEnabledState(r.Context(), updatedUser.UID, updatedUser.EmbyID, a.embyShouldEnableUser(updatedUser))
	previousUID := any(nil)
	if displacedUID != 0 {
		previousUID = displacedUID
	}
	ok(w, "Emby account linked", map[string]any{"uid": updatedUser.UID, "emby_id": updatedUser.EmbyID, "emby_username": updatedUser.EmbyUsername, "force_taken": displacedUID != 0, "previous_uid": previousUID, "user": publicUser(updatedUser)})
}

func (a *App) handleTelegramRosterStats(w http.ResponseWriter, r *http.Request, _ Params) {
	chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
	chatID := ""
	if len(chats) > 0 {
		chatID = chats[0]
	}
	stats := a.store().TelegramRosterStats(chatID)
	entries := a.store().TelegramRoster(chatID, true)
	bound := 0
	unbound := 0
	if len(entries) > 0 {
		for _, entry := range entries {
			if entry.IsBot {
				continue
			}
			if _, okUser := a.store().FindUserByTelegramID(entry.TelegramID); okUser {
				bound++
			} else {
				unbound++
			}
		}
	} else {
		for _, u := range a.store().ListUsers() {
			if u.TelegramID == 0 {
				continue
			}
			bound++
		}
		stats["total"] = bound
		stats["active"] = bound
		stats["known_only"] = true
	}
	stats["bound"] = bound
	stats["unbound"] = unbound
	ok(w, "OK", stats)
}

// csvSafe 中和 CSV 公式注入：Excel / WPS / LibreOffice 会把以 = + - @ 开头
// （或以 Tab / CR / LF 起始）的单元格当作公式求值，恶意用户名 / 媒体标题
// 形如 "=HYPERLINK(...)"、"=1+2"、"+cmd" 在管理员打开导出文件时会被执行或
// 用于钓鱼。前置一个单引号 ' 让电子表格按文本处理，不影响 CSV 数据语义。
// 空串与普通文本原样返回。
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + value
	}
	return value
}

func (a *App) handleExportUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "email", "role", "active"})
	for _, u := range a.store().ListUsers() {
		_ = cw.Write([]string{strconv.FormatInt(u.UID, 10), csvSafe(u.Username), csvSafe(u.Email), strconv.Itoa(u.Role), strconv.FormatBool(u.Active)})
	}
	cw.Flush()
}

func (a *App) handleExportPlayback(w http.ResponseWriter, r *http.Request, _ Params) {
	days := queryInt(r, "days", 30)
	since := int64(0)
	if days > 0 {
		since = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}
	records := a.store().PlaybackRecords(0, since, 10000)
	usernameByUID := map[int64]string{}
	for _, u := range a.store().ListUsers() {
		usernameByUID[u.UID] = u.Username
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=playback_stats.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "item_id", "title", "media_type", "duration_seconds", "played_at"})
	for _, record := range records {
		_ = cw.Write([]string{
			strconv.FormatInt(record.UID, 10),
			csvSafe(usernameByUID[record.UID]),
			csvSafe(record.ItemID),
			csvSafe(record.Title),
			csvSafe(record.MediaType),
			strconv.FormatInt(record.Duration, 10),
			strconv.FormatInt(record.PlayedAt, 10),
		})
	}
	cw.Flush()
}

func (a *App) handleWatchStats(w http.ResponseWriter, r *http.Request, params Params) {
	caller := current(r).User
	uid := caller.UID
	global := false
	// 路由表把 /stats/me、/stats/user/:uid（AuthUser）、/batch/watch-stats、
	// /batch/watch-stats/:uid（AuthAdmin）、/batch/watch-stats/global（AuthAdmin）
	// 都映射到本 handler。原实现 splitPath + Contains 推断既不直观也容易在
	// 增减路由时漏判。改为：global 走最后一段是 "global" 的硬约定，:uid 走
	// params；最后用 caller.Role == admin || uid == self 做统一鉴权。
	if parts := splitPath(r.URL.Path); len(parts) > 0 && parts[len(parts)-1] == "global" {
		global = true
		uid = 0
	} else if params["uid"] != "" {
		paramUID, err := int64Param(params, "uid")
		if err == nil && paramUID > 0 {
			uid = paramUID
		}
	}
	// belt-and-suspenders：global / 跨用户视图必须是 admin。AuthAdmin 已挡住
	// 主流量，但 path-string 之前是真实存在的鉴权依据，整改后保留显式断言
	// 防止路由表手抖。
	if global && caller.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "权限不足")
		return
	}
	if uid != caller.UID && caller.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrWatchStatsForbidden, "cannot view another user's watch stats")
		return
	}
	days := queryInt(r, "days", 0)
	if global && days <= 0 {
		days = 7
	}
	since := int64(0)
	if days > 0 {
		since = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}
	records := a.store().PlaybackRecords(uid, since, 1000)
	totalDuration := int64(0)
	typeStats := map[string]map[string]any{}
	recent := []map[string]any{}
	activeUsers := map[int64]bool{}
	for _, record := range records {
		totalDuration += record.Duration
		activeUsers[record.UID] = true
		mediaType := firstNonEmpty(record.MediaType, "unknown")
		stat := typeStats[mediaType]
		if stat == nil {
			stat = map[string]any{"count": 0, "duration": int64(0)}
			typeStats[mediaType] = stat
		}
		stat["count"] = stat["count"].(int) + 1
		stat["duration"] = stat["duration"].(int64) + record.Duration
		if len(recent) < 10 {
			recent = append(recent, map[string]any{"item_id": record.ItemID, "item_name": record.Title, "item_type": record.MediaType, "duration": record.Duration, "duration_str": formatSeconds(record.Duration), "start_time": record.PlayedAt})
		}
	}
	ok(w, "OK", map[string]any{"period_days": days, "total_play_count": len(records), "play_count": len(records), "plays": len(records), "total_duration": totalDuration, "duration": totalDuration, "total_duration_str": formatSeconds(totalDuration), "active_user_count": len(activeUsers), "type_stats": typeStats, "recent_plays": recent, "items": recent})
}

func (a *App) handleExpiringUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	days := queryInt(r, "days", 3)
	deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := time.Now().Unix()
	items := []map[string]any{}
	for _, u := range a.store().ListUsers() {
		if u.ExpiredAt > now && u.ExpiredAt <= deadline {
			remaining := u.ExpiredAt - now
			items = append(items, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "expired_at": u.ExpiredAt, "remaining_seconds": remaining, "remaining_str": formatSeconds(remaining)})
		}
	}
	ok(w, "OK", map[string]any{"days": days, "count": len(items), "users": items})
}

// randomCode 生成 hex 编码的随机字符串，用于 API key、绑定码、上传文件名、
// 临时密码等所有"必须不可预测"的场景。
// 安全性约束：crypto/rand 故障时**绝不**回退到
// time.Now().UnixNano() — 那会让攻击者用本机时钟近似猜出 token，等同于
// 给 API key / 密码出一个可预测后门。这里直接 panic：HTTP 路径会被 app.go
// 的 recover 中间件兜成 500（更符合 fail-closed 原则）；telegram bot 和
// scheduler daemon 的入口都已加 defer recover，单条任务失败不会拖垮进程。
// crypto/rand 在 Linux/macOS/Windows 现代内核上从不返回 error；这里 panic
// 仅在熵源完全坏掉时触发（容器无 /dev/urandom、自定义 sandbox 等），属于
// 真正应该让上层感知的故障。
func randomCode(length int) string {
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand failure (cannot generate secure random): %v", err))
	}
	return hex.EncodeToString(buf)[:length]
}

func stringSlice(v any) []string {
	switch typed := v.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		return strings.Split(typed, ",")
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func zeroNil(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func countActive(users []store.User) int {
	count := 0
	for _, u := range users {
		if u.Active {
			count++
		}
	}
	return count
}
