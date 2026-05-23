package api

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

var uploadFilenamePattern = regexp.MustCompile(`^[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$`)
var demoActionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
var backgroundGradientPattern = regexp.MustCompile(`(?i)^(linear-gradient|radial-gradient|conic-gradient|repeating-linear-gradient|repeating-radial-gradient)\s*\(`)
var telegramPublicUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "Twilight API", map[string]any{"name": a.cfg.AppName, "version": a.cfg.Version, "docs": "/api/v1/docs"})
}

func (a *App) handleOpenAPI(w http.ResponseWriter, r *http.Request, _ Params) {
	paths := map[string]map[string]any{}
	for _, route := range a.routes {
		pathItem := paths[route.Pattern]
		if pathItem == nil {
			pathItem = map[string]any{}
			paths[route.Pattern] = pathItem
		}
		pathItem[strings.ToLower(route.Method)] = map[string]any{"responses": map[string]any{"200": map[string]string{"description": "OK"}}}
	}
	ok(w, "OK", map[string]any{"openapi": "3.0.3", "info": map[string]string{"title": "Twilight Go API", "version": a.cfg.Version}, "paths": paths})
}

func (a *App) handleDocs(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Twilight Go API</title><h1>Twilight Go API</h1><p>OpenAPI JSON: <a href="/api/v1/openapi.json">/api/v1/openapi.json</a></p>`))
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.limiter.Allow(r.Context(), rateKey("login:", a.clientIP(r)), 10, time.Minute) {
		fail(w, http.StatusTooManyRequests, "登录过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	if username == "" || password == "" {
		fail(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}
	u, okUser := a.store.FindUserByUsername(username)
	if !okUser || !security.VerifyPassword(password, u.PasswordHash) {
		fail(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if !u.Active {
		fail(w, http.StatusForbidden, "账号已被禁用")
		return
	}
	token, expires, err := a.sessions.Create(r.Context(), u.UID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	a.setSessionCookie(w, token, expires)
	deviceID := firstNonEmpty(r.Header.Get("X-Twilight-Device"), r.UserAgent(), a.clientIP(r))
	_ = a.store.UpsertDevice(store.Device{UID: u.UID, DeviceID: deviceID, DeviceName: firstNonEmpty(r.UserAgent(), "unknown"), Client: "web", FirstSeen: time.Now().Unix(), LastSeen: time.Now().Unix()})
	_ = a.store.AddLoginLog(store.LoginLog{UID: u.UID, IP: a.clientIP(r), DeviceID: deviceID, DeviceName: firstNonEmpty(r.UserAgent(), "unknown"), Client: "web", Time: time.Now().Unix()})
	ok(w, "登录成功", map[string]any{"token": token, "user": publicUser(u)})
}

func (a *App) handleLoginByAPIKey(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	key := stringValue(payload, "apikey")
	if key == "" {
		fail(w, http.StatusBadRequest, "API Key 不能为空")
		return
	}
	_, u, okKey := a.store.FindAPIKeyByHash(hashAPIKey(key))
	if !okKey {
		fail(w, http.StatusUnauthorized, "API Key 无效")
		return
	}
	token, expires, err := a.sessions.Create(r.Context(), u.UID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "创建会话失败")
		return
	}
	a.setSessionCookie(w, token, expires)
	ok(w, "登录成功", map[string]any{"token": token, "user": publicUser(u)})
}

func (a *App) handleDirectLoginUnavailable(w http.ResponseWriter, r *http.Request, _ Params) {
	fail(w, http.StatusForbidden, "直接登录未启用")
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := a.clientIP(r)
	if !a.limiter.Allow(r.Context(), rateKey("forgot-password:ip:", ip), 5, 10*time.Minute) {
		fail(w, http.StatusTooManyRequests, "too many password reset attempts")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" || embyPassword == "" {
		fail(w, http.StatusBadRequest, "missing Emby username or password")
		return
	}
	if len(embyUsername) > 100 || len(embyPassword) > 200 {
		fail(w, http.StatusBadRequest, "input too long")
		return
	}
	if !a.limiter.Allow(r.Context(), rateKey("forgot-password:user:", strings.ToLower(embyUsername)), 5, 30*time.Minute) {
		fail(w, http.StatusTooManyRequests, "too many password reset attempts for this account")
		return
	}
	embyUser, okAuth, err := a.embyAuthenticateByName(r.Context(), embyUsername, embyPassword)
	if err != nil {
		fail(w, http.StatusUnauthorized, "Emby authentication failed")
		return
	}
	if !okAuth {
		fail(w, http.StatusUnauthorized, "invalid Emby username or password")
		return
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	u, okUser := a.store.FindUserByEmbyID(embyID)
	if !okUser {
		fail(w, http.StatusNotFound, "Emby account is not linked to a Web account")
		return
	}
	if !u.Active {
		fail(w, http.StatusForbidden, "account disabled")
		return
	}
	newPassword := "Twilight-" + randomCode(18)
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		fail(w, http.StatusInternalServerError, "password processing failed")
		return
	}
	u, err = a.store.UpdateUser(u.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	if u.EmbyID != "" && a.cfg.EmbyURL != "" {
		_ = a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u))
	}
	a.sessions.DeleteUser(r.Context(), u.UID)
	ok(w, "password reset", map[string]any{"username": u.Username, "new_password": newPassword})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions.Delete(r.Context(), p.Token)
	a.clearSessionCookie(w)
	ok(w, "logged out", nil)
}

func (a *App) handleLogoutAll(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions.DeleteUser(r.Context(), p.User.UID)
	a.clearSessionCookie(w)
	ok(w, "all sessions logged out", nil)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	a.sessions.Delete(r.Context(), p.Token)
	token, expires, err := a.sessions.Create(r.Context(), p.User.UID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "刷新会话失败")
		return
	}
	a.setSessionCookie(w, token, expires)
	ok(w, "刷新成功", map[string]any{"token": token, "user": publicUser(p.User)})
}

func (a *App) handleCurrentUser(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", publicUser(current(r).User))
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.limiter.Allow(r.Context(), rateKey("register:", a.clientIP(r)), 5, 10*time.Minute) {
		fail(w, http.StatusTooManyRequests, "注册过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "username")
	password := stringValue(payload, "password")
	if len(username) < 3 || len(username) > 32 || strings.ContainsAny(username, "/\\@:\x00<>\"'&") {
		fail(w, http.StatusBadRequest, "invalid username")
		return
	}
	if len(password) < 8 {
		fail(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		fail(w, http.StatusInternalServerError, "密码处理失败")
		return
	}
	role := store.RoleNormal
	if a.store.UserCount() == 0 {
		role = store.RoleAdmin
	}
	u, err := a.store.CreateUser(store.User{Username: username, Email: stringValue(payload, "email"), PasswordHash: passwordHash, Role: role})
	if statusFromError(w, err) {
		return
	}
	created(w, "注册成功", map[string]any{"user": publicUser(u), "first_admin": role == store.RoleAdmin})
}

func (a *App) handleRegisterAvailability(w http.ResponseWriter, r *http.Request, _ Params) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	available := true
	if username != "" {
		_, found := a.store.FindUserByUsername(username)
		available = !found
	}
	ok(w, "OK", map[string]any{"enabled": true, "can_register": true, "requires_reg_code": false, "available": available})
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	if !a.cfg.BangumiEnabled {
		if _, ok := payload["bgm_mode"]; ok {
			fail(w, http.StatusForbidden, "Bangumi sync is disabled")
			return
		}
		if token := stringValue(payload, "bgm_token"); token != "" {
			fail(w, http.StatusForbidden, "Bangumi sync is disabled")
			return
		}
	}
	if token := stringValue(payload, "bgm_token"); len(token) > 4096 {
		fail(w, http.StatusBadRequest, "Bangumi Token 过长")
		return
	}
	if boolValue(payload, "bgm_mode", false) && p.User.BGMToken == "" && stringValue(payload, "bgm_token") == "" {
		fail(w, http.StatusBadRequest, "启用 Bangumi 同步前请先填写个人 Token")
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		if email := stringValue(payload, "email"); email != "" {
			if len(email) > 256 || strings.ContainsAny(email, "<>\"'\x00") {
				return fmt.Errorf("invalid email format")
			}
			u.Email = email
		}
		if username := stringValue(payload, "username"); username != "" {
			if len(username) < 3 || len(username) > 32 || strings.ContainsAny(username, "/\\@:\x00<>\"'&") {
				return fmt.Errorf("invalid username")
			}
			u.Username = username
		}
		if _, ok := payload["bgm_mode"]; ok {
			u.BGMMode = boolValue(payload, "bgm_mode", u.BGMMode)
		}
		if token := stringValue(payload, "bgm_token"); token != "" {
			u.BGMToken = token
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "鏇存柊鎴愬姛", publicUser(u))
}

func (a *App) handleUpdateUsername(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	username := stringValue(payload, "new_username")
	if username == "" {
		fail(w, http.StatusBadRequest, "missing new_username")
		return
	}
	if len(username) < 3 || len(username) > 32 || strings.ContainsAny(username, "/\\@:\x00<>\"'&") {
		fail(w, http.StatusBadRequest, "invalid username")
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		u.Username = username
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "鐢ㄦ埛鍚嶅凡鏇存柊", publicUser(u))
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	payload := decodeMap(r)
	oldPassword := stringValue(payload, "old_password")
	newPassword := stringValue(payload, "new_password")
	if !security.VerifyPassword(oldPassword, p.User.PasswordHash) {
		fail(w, http.StatusForbidden, "old password is incorrect")
		return
	}
	if len(newPassword) < 8 {
		fail(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	hash, err := security.HashPassword(newPassword)
	if err != nil {
		fail(w, http.StatusInternalServerError, "瀵嗙爜澶勭悊澶辫触")
		return
	}
	_, err = a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "password updated", nil)
}

func (a *App) handleGeneratedPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	password := "Twilight-" + randomCode(12)
	hash, err := security.HashPassword(password)
	if err != nil {
		fail(w, http.StatusInternalServerError, "瀵嗙爜澶勭悊澶辫触")
		return
	}
	_, err = a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.PasswordHash = hash; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "password reset", map[string]any{"new_password": password})
}

func (a *App) handleChangeEmbyPassword(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	if p.User.EmbyID == "" {
		fail(w, http.StatusBadRequest, "user has no linked Emby account")
		return
	}
	payload := decodeMap(r)
	newPassword := stringValue(payload, "new_password")
	if okPass, msg := validateStrongPassword(newPassword, "Emby password"); !okPass {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if err := a.embySetPassword(r.Context(), p.User.EmbyID, newPassword); err != nil {
		fail(w, http.StatusBadGateway, "failed to update Emby password")
		return
	}
	ok(w, "Emby password updated", nil)
}

func (a *App) handleBindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.User.EmbyID != "" {
		fail(w, http.StatusBadRequest, "user already has linked Emby account")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		fail(w, http.StatusBadRequest, "missing Emby username")
		return
	}
	embyUser, okAuth, err := a.embyAuthenticateByName(r.Context(), embyUsername, embyPassword)
	if err != nil {
		fail(w, http.StatusBadGateway, "failed to authenticate with Emby")
		return
	}
	if !okAuth {
		fail(w, http.StatusUnauthorized, "invalid Emby username or password")
		return
	}
	embyID := firstNonEmpty(asString(embyUser["Id"]), asString(embyUser["ID"]), asString(embyUser["id"]))
	if embyID == "" {
		fail(w, http.StatusBadGateway, "Emby did not return a user id")
		return
	}
	// Security: prevent non-admin users from binding Emby administrator accounts
	if p.User.Role != store.RoleAdmin {
		policy := embyPolicy(embyUser)
		if boolish(policy["IsAdministrator"]) {
			fail(w, http.StatusForbidden, "安全限制：不允许绑定 Emby 管理员账号。如需绑定，请联系系统管理员。")
			return
		}
	}
	if existing, okExisting := a.store.FindUserByEmbyID(embyID); okExisting && existing.UID != p.User.UID {
		fail(w, http.StatusConflict, "Emby account is already linked to another user")
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		u.EmbyUsername = firstNonEmpty(asString(embyUser["Name"]), embyUsername)
		u.EmbyID = embyID
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	_ = a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u))
	// Apply default hidden libraries for non-admin/non-whitelist users on bind
	if len(a.cfg.EmbyDefaultHiddenLibraries) > 0 && u.Role != store.RoleAdmin && u.Role != store.RoleWhitelist {
		_ = a.embySetLibrariesByAction(r.Context(), u, "hide", nil, a.cfg.EmbyDefaultHiddenLibraries, false)
	}
	ok(w, "Emby account linked", map[string]any{"emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "user": publicUser(u)})
}
func (a *App) handleRegisterEmby(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	if p.User.EmbyID != "" {
		fail(w, http.StatusBadRequest, "user already has linked Emby account")
		return
	}
	if !p.User.PendingEmby && !a.cfg.EmbyDirectRegisterEnabled {
		fail(w, http.StatusBadRequest, "current account has no Emby registration entitlement")
		return
	}
	payload := decodeMap(r)
	embyUsername := stringValue(payload, "emby_username")
	embyPassword := stringValue(payload, "emby_password")
	if embyUsername == "" {
		fail(w, http.StatusBadRequest, "missing Emby username")
		return
	}
	if okPass, msg := validateStrongPassword(embyPassword, "Emby password"); !okPass {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if existing, exists, err := a.embyUserByName(r.Context(), embyUsername); err != nil {
		fail(w, http.StatusBadGateway, "failed to check Emby username")
		return
	} else if exists {
		fail(w, http.StatusConflict, "Emby username already exists: "+asString(existing["Name"]))
		return
	}
	if a.cfg.EmbyUserLimit > 0 {
		count := 0
		for _, u := range a.store.ListUsers() {
			// Count users who already have Emby or have pending entitlement
			if u.EmbyID != "" || (u.PendingEmby && u.UID != p.User.UID) {
				count++
			}
		}
		// Also count active unused invite codes as reserved slots
		for _, code := range a.store.ListAllInviteCodes() {
			if code.Active && code.UseCount < code.UseCountLimit {
				count++
			}
		}
		if count >= a.cfg.EmbyUserLimit {
			fail(w, http.StatusConflict, "Emby user limit reached")
			return
		}
	}
	createdUser, err := a.embyCreateUser(r.Context(), embyUsername, embyPassword)
	if err != nil {
		fail(w, http.StatusBadGateway, "failed to create Emby user")
		return
	}
	embyID := asString(createdUser["Id"])
	days := a.cfg.EmbyDirectRegisterDays
	if p.User.PendingEmbyDays != nil {
		days = *p.User.PendingEmbyDays
	}
	if days == 0 {
		days = 30
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		u.EmbyID = embyID
		u.EmbyUsername = firstNonEmpty(asString(createdUser["Name"]), embyUsername)
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		if u.Role == store.RoleUnrecognized {
			u.Role = store.RoleNormal
		}
		if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist || days < 0 {
			u.ExpiredAt = 253402214400
		} else {
			u.ExpiredAt = time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
		}
		return nil
	})
	if statusFromError(w, err) {
		_ = a.embyDelete(r.Context(), "/Users/"+urlPathEscape(embyID))
		return
	}
	_ = a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u))
	if len(a.cfg.EmbyDefaultHiddenLibraries) > 0 && u.Role != store.RoleAdmin && u.Role != store.RoleWhitelist {
		_ = a.embySetLibrariesByAction(r.Context(), u, "hide", nil, a.cfg.EmbyDefaultHiddenLibraries, false)
	}
	ok(w, "Emby account created", map[string]any{"user": publicUser(u), "emby_id": u.EmbyID, "emby_username": u.EmbyUsername, "request_id": ""})
}

func (a *App) handleUnbindEmby(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.EmbyID = ""; u.EmbyUsername = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "Emby account unbound", publicUser(u))
}

func (a *App) handleRenew(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	// Self-service renewal requires a reg_code; bare renewal without code is forbidden
	payload := decodeMap(r)
	regCode := stringValue(payload, "reg_code")
	if regCode == "" {
		fail(w, http.StatusBadRequest, "续期需要提供注册码")
		return
	}
	if a.requireNonEmbyAdmin(w, r, p.User) {
		return
	}
	// Consume the reg code (validates active, use count, expiry)
	code, err := a.store.ConsumeRegCode(regCode, p.User.UID, p.User.TelegramID)
	if err != nil {
		fail(w, http.StatusBadRequest, "注册码无效、已用完或已过期")
		return
	}
	days := int64(code.Days)
	if days <= 0 {
		days = 30
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		base := time.Now().Unix()
		if u.ExpiredAt > base {
			base = u.ExpiredAt
		}
		u.ExpiredAt = base + days*86400
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "续期成功", map[string]any{"expire_status": expireStatus(u.ExpiredAt), "expired_at": u.ExpiredAt, "user": publicUser(u)})
}

func (a *App) handleQueueStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"status": "success", "pending": false, "terminal": true, "result": map[string]any{}})
}

func (a *App) handleRegisterBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	a.createBindCode(w, 0, "register")
}

func (a *App) handleUserBindCode(w http.ResponseWriter, r *http.Request, _ Params) {
	a.createBindCode(w, current(r).User.UID, "user")
}

func (a *App) createBindCode(w http.ResponseWriter, uid int64, scene string) {
	code := strings.ToUpper(randomCode(8))
	now := time.Now().Unix()
	_ = a.store.UpsertBindCode(store.BindCode{Code: code, Scene: scene, UID: uid, CreatedAt: now, ExpiresAt: now + 600})
	ok(w, "OK", map[string]any{"bind_code": code, "expires_in": 600})
}

func (a *App) handleBindCodeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	bind, okBind := a.store.BindCode(code)
	if !okBind || bind.ExpiresAt < time.Now().Unix() {
		ok(w, "OK", map[string]any{"code": code, "confirmed": false, "invalid": true, "terminal": true})
		return
	}
	ok(w, "OK", map[string]any{"code": code, "confirmed": bind.Confirmed, "expires_in": bind.ExpiresAt - time.Now().Unix(), "invalid": false, "terminal": bind.Confirmed})
}

func (a *App) handleBindConfirm(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	code := strings.ToUpper(stringValue(payload, "code"))
	bind, okBind := a.store.BindCode(code)
	if !okBind {
		fail(w, http.StatusNotFound, "缁戝畾鐮佷笉瀛樺湪")
		return
	}
	bind.Confirmed = true
	bind.TelegramID = int64(intValue(payload, "telegram_id", 0))
	_ = a.store.UpsertBindCode(bind)
	ok(w, "bind confirmed", map[string]any{"code": code, "confirmed": true})
}

func (a *App) handleTelegramStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	canUnbind := !a.cfg.ForceBindTelegram || u.Role == store.RoleAdmin
	ok(w, "OK", map[string]any{"bound": u.TelegramID != 0, "telegram_id": nullableInt(u.TelegramID), "telegram_id_full": nullableInt(u.TelegramID), "telegram_username": u.TelegramUsername, "force_bind": a.cfg.ForceBindTelegram, "can_unbind": canUnbind, "can_change": canUnbind})
}

func (a *App) handleUnbindTelegram(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	// Enforce force-bind policy: non-admin users cannot unbind when force_bind_telegram is enabled
	if a.cfg.ForceBindTelegram && p.User.Role != store.RoleAdmin {
		fail(w, http.StatusForbidden, "当前系统要求强制绑定 Telegram，无法解绑")
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.TelegramID = 0; u.TelegramUsername = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram unbound", publicUser(u))
}

func (a *App) handleTelegramRebindRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	if u.TelegramID == 0 {
		fail(w, http.StatusBadRequest, "当前账号未绑定 Telegram")
		return
	}
	req, err := a.store.CreateRebindRequest(store.RebindRequest{UID: u.UID, Username: u.Username, OldTelegramID: u.TelegramID, Reason: truncateString(stringValue(decodeMap(r), "reason"), 500)})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram rebind request submitted", req)
}

func (a *App) handleUserSettings(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	ok(w, "OK", map[string]any{
		"bgm_mode": u.BGMMode, "bgm_token_set": u.BGMToken != "", "api_key_enabled": u.LegacyAPIKeyStatus,
		"telegram":      map[string]any{"bound": u.TelegramID != 0, "force_bind": a.cfg.ForceBindTelegram, "can_unbind": !a.cfg.ForceBindTelegram, "can_change": true, "pending_rebind_request": false, "rebind_request_status": nil, "rebind_request_id": nil},
		"emby_status":   map[string]any{"is_synced": u.EmbyID != "", "is_active": u.Active, "active_sessions": 0, "message": "OK"},
		"system_config": map[string]any{"device_limit_enabled": a.cfg.DeviceLimitEnabled, "max_devices": a.cfg.MaxDevices, "max_streams": a.cfg.MaxStreams, "bangumi_sync_enabled": a.cfg.BangumiEnabled},
	})
}

func (a *App) handleDevices(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	if params["uid"] != "" {
		paramUID, err := int64Param(params, "uid")
		if err == nil && paramUID > 0 {
			uid = paramUID
		}
	}
	items := []map[string]any{}
	for _, d := range a.store.ListDevices(uid) {
		items = append(items, map[string]any{"device_id": d.DeviceID, "device_name": d.DeviceName, "client": d.Client, "first_seen": d.FirstSeen, "last_seen": d.LastSeen, "is_trusted": d.Trusted})
	}
	ok(w, "OK", items)
}

func (a *App) handleLibraries(w http.ResponseWriter, r *http.Request, params Params) {
	// Route: list all Emby libraries (no user context needed)
	if strings.Contains(r.URL.Path, "/emby/libraries") || strings.Contains(r.URL.Path, "/admin/emby/libraries") {
		remoteLibraries, err := a.embyLibraries(r.Context())
		if err != nil {
			fail(w, http.StatusBadGateway, "failed to read Emby libraries")
			return
		}
		ok(w, "OK", remoteLibraries)
		return
	}

	// Resolve target user: admin routes pass :uid, user routes use session
	targetUser := current(r).User
	if params["uid"] != "" {
		uid, _ := int64Param(params, "uid")
		if u, okUser := a.store.User(uid); okUser {
			targetUser = u
		} else {
			fail(w, http.StatusNotFound, "user not found")
			return
		}
	}

	// PUT: modify library visibility
	if r.Method == http.MethodPut {
		if targetUser.EmbyID == "" {
			fail(w, http.StatusBadRequest, "user has no linked Emby account")
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
				fail(w, http.StatusForbidden, "library self-service is not enabled")
				return
			}
			if action != "show" && action != "hide" {
				fail(w, http.StatusForbidden, "unsupported self-service action")
				return
			}
			allowed := map[string]bool{}
			for _, name := range normalizeLibraryNames(a.cfg.EmbySelfServiceLibraries) {
				allowed[strings.ToLower(name)] = true
			}
			for _, name := range names {
				if !allowed[strings.ToLower(name)] {
					fail(w, http.StatusForbidden, "library is not self-service enabled")
					return
				}
			}
		}
		if err := a.embySetLibrariesByAction(r.Context(), targetUser, action, ids, names, boolValue(body, "enable_all", false)); err != nil {
			fail(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	// GET or after successful PUT: return current library access state
	ok(w, "OK", a.embyLibraryAccess(r.Context(), targetUser, current(r).User.Role == store.RoleAdmin))
}

func (a *App) handleSessions(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.cfg.EmbyURL == "" {
		ok(w, "OK", []any{})
		return
	}
	var remote []map[string]any
	if err := a.embyGet(r.Context(), "/Sessions", &remote); err != nil {
		fail(w, http.StatusBadGateway, "failed to read Emby sessions")
		return
	}
	user := current(r).User
	adminView := strings.Contains(r.URL.Path, "/admin/")
	items := []map[string]any{}
	for _, session := range remote {
		if !adminView && user.EmbyID != "" && asString(session["UserId"]) != user.EmbyID {
			continue
		}
		nowPlaying := any(nil)
		if item, ok := session["NowPlayingItem"].(map[string]any); ok {
			nowPlaying = map[string]any{"id": item["Id"], "name": item["Name"], "type": item["Type"]}
		}
		local := any(nil)
		if u, okUser := a.store.FindUserByEmbyID(asString(session["UserId"])); okUser {
			local = map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID)}
		}
		items = append(items, map[string]any{
			"session_id":          asString(session["Id"]),
			"user_id":             asString(session["UserId"]),
			"user_name":           firstNonEmpty(asString(session["UserName"]), asString(session["UserName"])),
			"client":              asString(session["Client"]),
			"device_name":         asString(session["DeviceName"]),
			"device_id":           asString(session["DeviceId"]),
			"application_version": asString(session["ApplicationVersion"]),
			"is_active":           boolish(session["IsActive"]),
			"now_playing":         nowPlaying,
			"local_user":          local,
		})
	}
	ok(w, "OK", items)
}

func (a *App) handleLoginHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	uid := current(r).User.UID
	parts := splitPath(r.URL.Path)
	if len(parts) > 0 {
		if last, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil && strings.Contains(r.URL.Path, "/security/login-history/") {
			uid = last
		}
	}
	limit := clamp(queryInt(r, "limit", 50), 1, 100)
	logs := a.store.LoginHistory(uid, false, 0, limit)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"id": log.ID, "ip": log.IP, "device": log.DeviceName, "client": log.Client, "time": log.Time, "blocked": log.Blocked, "country": log.Country, "city": log.City})
	}
	ok(w, "OK", map[string]any{"records": items, "total": len(items)})
}

func (a *App) handleBlockDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	if params["uid"] != "" {
		uid, _ = int64Param(params, "uid")
	}
	deviceID := params["device_id"]
	if deviceID == "" {
		fail(w, http.StatusBadRequest, "设备 ID 不能为空")
		return
	}
	if err := a.store.UpdateDevice(uid, deviceID, func(d *store.Device) { d.Blocked = true; d.Trusted = false }); statusFromError(w, err) {
		return
	}
	ok(w, "device blocked", nil)
}

func (a *App) handleTrustDevice(w http.ResponseWriter, r *http.Request, params Params) {
	uid := current(r).User.UID
	deviceID := params["device_id"]
	if err := a.store.UpdateDevice(uid, deviceID, func(d *store.Device) { d.Trusted = true; d.Blocked = false }); statusFromError(w, err) {
		return
	}
	ok(w, "device trusted", nil)
}

func (a *App) handleDeleteDevice(w http.ResponseWriter, r *http.Request, params Params) {
	deviceID := params["device_id"]
	if deviceID == "" {
		fail(w, http.StatusBadRequest, "设备 ID 不能为空")
		return
	}
	if err := a.store.DeleteDevice(current(r).User.UID, deviceID); statusFromError(w, err) {
		return
	}
	ok(w, "device removed", nil)
}

func (a *App) handleIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", a.store.ListIPBlacklist())
}

func (a *App) handleAddIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ip := stringValue(payload, "ip")
	if ip == "" {
		fail(w, http.StatusBadRequest, "IP 不能为空")
		return
	}
	hours := intValue(payload, "hours", -1)
	expireAt := int64(-1)
	if hours > 0 {
		expireAt = time.Now().Add(time.Duration(hours) * time.Hour).Unix()
	}
	if err := a.store.AddIPBlacklist(ip, stringValue(payload, "reason"), expireAt); statusFromError(w, err) {
		return
	}
	ok(w, "IP 已加入黑名单", nil)
}

func (a *App) handleDeleteIPBlacklist(w http.ResponseWriter, r *http.Request, _ Params) {
	ip := stringValue(decodeMap(r), "ip")
	if ip == "" {
		fail(w, http.StatusBadRequest, "IP 不能为空")
		return
	}
	if err := a.store.RemoveIPBlacklist(ip); statusFromError(w, err) {
		return
	}
	ok(w, "IP 已移出黑名单", nil)
}

func (a *App) handleSuspicious(w http.ResponseWriter, r *http.Request, _ Params) {
	hours := queryInt(r, "hours", 24)
	logs := a.store.LoginHistory(0, true, time.Now().Add(-time.Duration(hours)*time.Hour).Unix(), 100)
	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		items = append(items, map[string]any{"uid": log.UID, "ip": log.IP, "device": log.DeviceName, "time": log.Time, "reason": firstNonEmpty(log.Reason, "blocked")})
	}
	ok(w, "OK", items)
}

func (a *App) handleGetBackground(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	u, okUser := a.store.User(uid)
	if !okUser {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	ok(w, "OK", map[string]any{"background": u.Background})
}

func (a *App) handleUpdateBackground(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	payload := decodeMap(r)
	bg, err := sanitizedBackgroundConfig(payload)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.Background = bg; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "background updated", map[string]any{"background": u.Background})
}

func sanitizedBackgroundConfig(payload map[string]any) (string, error) {
	if len(payload) == 0 {
		return "", fmt.Errorf("背景配置不能为空")
	}
	if raw := firstNonEmpty(stringValue(payload, "background"), stringValue(payload, "url")); raw != "" {
		var nested map[string]any
		if err := json.Unmarshal([]byte(raw), &nested); err == nil && len(nested) > 0 {
			payload = nested
		} else {
			css, err := sanitizeBackgroundCSSValue(raw)
			if err != nil {
				return "", err
			}
			return mustJSON(map[string]any{"lightBg": css, "darkBg": css}), nil
		}
	}

	lightBg, err := sanitizeBackgroundCSSValue(stringValue(payload, "lightBg"))
	if err != nil {
		return "", err
	}
	darkBg, err := sanitizeBackgroundCSSValue(stringValue(payload, "darkBg"))
	if err != nil {
		return "", err
	}
	lightImage, err := sanitizeBackgroundImageValue(stringValue(payload, "lightBgImage"))
	if err != nil {
		return "", err
	}
	darkImage, err := sanitizeBackgroundImageValue(stringValue(payload, "darkBgImage"))
	if err != nil {
		return "", err
	}
	if lightBg == "" && darkBg == "" && lightImage == "" && darkImage == "" {
		return "", fmt.Errorf("背景配置不能为空")
	}

	cfg := map[string]any{
		"lightBg":      lightBg,
		"darkBg":       darkBg,
		"lightBgImage": lightImage,
		"darkBgImage":  darkImage,
		"lightFlow":    boolValue(payload, "lightFlow", false),
		"darkFlow":     boolValue(payload, "darkFlow", false),
		"lightBlur":    clamp(intValue(payload, "lightBlur", 0), 0, 30),
		"darkBlur":     clamp(intValue(payload, "darkBlur", 0), 0, 30),
		"lightOpacity": clamp(intValue(payload, "lightOpacity", 100), 10, 100),
		"darkOpacity":  clamp(intValue(payload, "darkOpacity", 100), 10, 100),
	}
	return mustJSON(cfg), nil
}

func sanitizeBackgroundCSSValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 2000 || strings.ContainsAny(value, "\x00\r\n<>;{}") || strings.Contains(strings.ToLower(value), "url(") || strings.Contains(value, "@") {
		return "", fmt.Errorf("背景 CSS 只允许安全渐变表达式")
	}
	if !backgroundGradientPattern.MatchString(value) {
		return "", fmt.Errorf("背景 CSS 只允许 linear/radial/conic gradient")
	}
	return value, nil
}

func sanitizeBackgroundImageValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return "", nil
	}
	if len(value) > 1000 || strings.ContainsAny(value, "\x00\r\n<>") {
		return "", fmt.Errorf("背景图片地址无效")
	}
	if strings.HasPrefix(strings.ToLower(value), "url(") && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[4 : len(value)-1])
		value = strings.Trim(value, `"'`)
	}
	const prefix = "/api/v1/users/assets/background/"
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("背景图片只允许使用本系统上传的背景资源")
	}
	filename := strings.TrimPrefix(value, prefix)
	if strings.ContainsAny(filename, `/\`) || !uploadFilenamePattern.MatchString(filename) {
		return "", fmt.Errorf("背景图片文件名无效")
	}
	return `url("` + value + `")`, nil
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func (a *App) handleDeleteBackground(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	_, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.Background = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "background deleted", nil)
}

func (a *App) handleGetAvatar(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	u, okUser := a.store.User(uid)
	if !okUser {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	ok(w, "OK", map[string]any{"avatar": u.Avatar, "uid": u.UID, "username": u.Username})
}

func (a *App) handleUploadBackground(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleUpload(w, r, "background")
}

func (a *App) handleUploadAvatar(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleUpload(w, r, "avatar")
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request, kind string) {
	if !a.limiter.Allow(r.Context(), rateKey("upload:", current(r).User.UID), 10, time.Minute) {
		fail(w, http.StatusTooManyRequests, "上传过于频繁")
		return
	}
	if err := r.ParseMultipartForm(a.cfg.MaxUploadSize); err != nil {
		fail(w, http.StatusBadRequest, "上传内容无效")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "缺少文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, a.cfg.MaxUploadSize+1))
	if err != nil || int64(len(data)) > a.cfg.MaxUploadSize {
		fail(w, http.StatusRequestEntityTooLarge, "文件过大")
		return
	}
	contentType := strings.ToLower(strings.Split(http.DetectContentType(data), ";")[0])
	ext, okImage := uploadImageExtension(contentType)
	if !okImage {
		fail(w, http.StatusBadRequest, "only image uploads are allowed")
		return
	}
	filename := randomCode(16) + ext
	dir := filepath.Join(a.cfg.UploadDir, kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(w, http.StatusInternalServerError, "创建上传目录失败")
		return
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o600); err != nil {
		fail(w, http.StatusInternalServerError, "保存文件失败")
		return
	}
	url := "/api/v1/users/assets/" + kind + "/" + filename
	p := current(r)
	_, _ = a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		if kind == "avatar" {
			u.Avatar = url
		} else {
			u.Background = url
		}
		return nil
	})
	if kind == "avatar" {
		ok(w, "上传成功", map[string]any{"avatar_url": url, "url": url, "filename": filename})
		return
	}
	ok(w, "上传成功", map[string]any{"url": url, "type": kind, "filename": filename})
}

func uploadImageExtension(contentType string) (string, bool) {
	switch contentType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	case "image/bmp":
		return ".bmp", true
	default:
		return "", false
	}
}

func (a *App) handleAsset(w http.ResponseWriter, r *http.Request, params Params) {
	kind := params["kind"]
	filename := params["filename"]
	if kind != "avatar" && kind != "background" {
		fail(w, http.StatusNotFound, "resource not found")
		return
	}
	if !uploadFilenamePattern.MatchString(filename) {
		fail(w, http.StatusNotFound, "resource not found")
		return
	}
	filePath, okPath := resolveUploadAssetPath(a.cfg.UploadDir, kind, filename)
	if !okPath {
		fail(w, http.StatusNotFound, "resource not found")
		return
	}
	http.ServeFile(w, r, filePath)
}

func resolveUploadAssetPath(uploadDir, kind, filename string) (string, bool) {
	root, err := filepath.Abs(firstNonEmpty(uploadDir, "uploads"))
	if err != nil {
		return "", false
	}
	target, err := filepath.Abs(filepath.Join(root, kind, filename))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}

func (a *App) handleDeleteAvatar(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	_, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.Avatar = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "avatar deleted", nil)
}

func (a *App) handleLegacyAPIKeyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	u := current(r).User
	ok(w, "OK", map[string]any{"enabled": u.LegacyAPIKeyStatus, "has_key": u.LegacyAPIKeyHash != ""})
}

func (a *App) handleLegacyAPIKeyGenerate(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	key := "key-" + randomCode(40)
	prefix, suffix, _ := maskAPIKey(key)
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error {
		u.LegacyAPIKeyHash = hashAPIKey(key)
		u.LegacyAPIKeyPrefix = prefix
		u.LegacyAPIKeySuffix = suffix
		u.LegacyAPIKeyStatus = true
		if len(u.LegacyPermissions) == 0 {
			u.LegacyPermissions = defaultPermissions()
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "API key generated", map[string]any{"apikey": key, "enabled": true, "user": publicUser(u)})
}

func (a *App) handleLegacyAPIKeyDelete(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	_, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyAPIKeyStatus = false; u.LegacyAPIKeyHash = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "API key deleted", nil)
}

func (a *App) handleLegacyAPIKeyEnable(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleLegacyAPIKeyGenerate(w, r, params)
}

func (a *App) handleLegacyAPIKeyPermissions(w http.ResponseWriter, r *http.Request, _ Params) {
	perms := current(r).User.LegacyPermissions
	if len(perms) == 0 {
		perms = defaultPermissions()
	}
	ok(w, "OK", map[string]any{"permissions": perms, "all_permissions": defaultPermissions()})
}

func (a *App) handleLegacyAPIKeyPermissionsUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	permissions := stringSlice(payload["permissions"])
	if len(permissions) == 0 {
		permissions = defaultPermissions()
	}
	p := current(r)
	_, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyPermissions = permissions; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "permissions updated", map[string]any{"permissions": permissions})
}

func (a *App) handleListAPIKeys(w http.ResponseWriter, r *http.Request, _ Params) {
	keys := a.store.ListAPIKeys(current(r).User.UID)
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, publicAPIKey(key, ""))
	}
	ok(w, "OK", map[string]any{"keys": items, "total": len(items)})
}

func (a *App) handleCreateAPIKey(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	key := "key-" + randomCode(40)
	prefix, suffix, _ := maskAPIKey(key)
	name := stringValue(payload, "name")
	if name == "" {
		name = "Default"
	}
	k, err := a.store.CreateAPIKey(store.APIKey{UID: current(r).User.UID, Name: name, Hash: hashAPIKey(key), Prefix: prefix, Suffix: suffix, AllowQuery: boolValue(payload, "allow_query", false), RateLimit: intValue(payload, "rate_limit", 100), Permissions: defaultPermissions()})
	if statusFromError(w, err) {
		return
	}
	ok(w, "API key created", publicAPIKey(k, key))
}

func (a *App) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "key_id")
	payload := decodeMap(r)
	k, err := a.store.UpdateAPIKey(current(r).User.UID, id, func(k *store.APIKey) error {
		if name := stringValue(payload, "name"); name != "" {
			k.Name = name
		}
		if _, ok := payload["enabled"]; ok {
			k.Enabled = boolValue(payload, "enabled", k.Enabled)
		}
		if _, ok := payload["allow_query"]; ok {
			k.AllowQuery = boolValue(payload, "allow_query", k.AllowQuery)
		}
		if _, ok := payload["rate_limit"]; ok {
			k.RateLimit = intValue(payload, "rate_limit", k.RateLimit)
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "API key updated", publicAPIKey(k, ""))
}

func (a *App) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "key_id")
	if statusFromError(w, a.store.DeleteAPIKey(current(r).User.UID, id)) {
		return
	}
	ok(w, "API key deleted", nil)
}

func (a *App) handleAPIKeyEnableAccount(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.Active = true; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "account enabled", publicUser(u))
}

func (a *App) handleAPIKeyDisableAccount(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	u, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.Active = false; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "account disabled", publicUser(u))
}

func (a *App) handleAPIKeyDisableKey(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.APIKey.ID > 0 {
		_, err := a.store.UpdateAPIKey(p.User.UID, p.APIKey.ID, func(k *store.APIKey) error { k.Enabled = false; return nil })
		if statusFromError(w, err) {
			return
		}
	} else {
		_, err := a.store.UpdateUser(p.User.UID, func(u *store.User) error { u.LegacyAPIKeyStatus = false; return nil })
		if statusFromError(w, err) {
			return
		}
	}
	ok(w, "API key disabled", nil)
}

func (a *App) handleAPIKeyEnableKey(w http.ResponseWriter, r *http.Request, _ Params) {
	p := current(r)
	if p.APIKey.ID > 0 {
		_, err := a.store.UpdateAPIKey(p.User.UID, p.APIKey.ID, func(k *store.APIKey) error { k.Enabled = true; return nil })
		if statusFromError(w, err) {
			return
		}
	}
	ok(w, "API key enabled", nil)
}

func (a *App) handleAPIKeyEmbyKick(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleKickUser(w, r, Params{"uid": strconv.FormatInt(current(r).User.UID, 10)})
}

func publicAPIKey(k store.APIKey, plain string) map[string]any {
	masked := k.Prefix + "..." + k.Suffix
	data := map[string]any{"id": k.ID, "name": k.Name, "key": masked, "key_prefix": k.Prefix, "key_suffix": k.Suffix, "enabled": k.Enabled, "allow_query": k.AllowQuery, "permissions": k.Permissions, "rate_limit": k.RateLimit, "request_count": k.RequestCount, "last_used": zeroNil(k.LastUsed), "created_at": k.CreatedAt, "expired_at": zeroNil(k.ExpiredAt)}
	if plain != "" {
		data["key"] = plain
	}
	return data
}

func defaultPermissions() []string {
	return []string{"account:read", "account:write", "emby:read", "emby:write"}
}

func (a *App) handleSystemInfo(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{
		"name":        a.cfg.AppName,
		"icon":        "/api/v1/system/server-icon",
		"version":     a.cfg.Version,
		"api_version": "v1",
		"features": map[string]any{
			"register":             true,
			"emby_direct_register": true,
			"telegram":             a.cfg.TelegramMode,
			"force_bind_telegram":  a.cfg.ForceBindTelegram,
			"bangumi_sync":         a.cfg.BangumiEnabled,
		},
		"limits":               map[string]any{"user_limit": a.cfg.UserLimit, "stream_limit": a.cfg.MaxStreams},
		"telegram_bot":         a.publicTelegramBotInfo(r.Context()),
		"telegram_links":       publicTelegramLinks(a.cfg.TelegramGroupIDs, a.cfg.TelegramChannelIDs),
		"telegram_mode":        a.cfg.TelegramMode,
		"bangumi_sync_enabled": a.cfg.BangumiEnabled,
	})
}

func (a *App) publicTelegramBotInfo(ctx context.Context) map[string]any {
	empty := map[string]any{"username": nil, "url": nil}
	if !a.telegramAvailable() {
		return empty
	}
	token := strings.TrimSpace(a.cfg.TelegramBotToken)
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
			bot = map[string]any{"username": username, "url": "https://t.me/" + username}
		}
	}

	a.telegramBotMu.Lock()
	a.telegramBotCacheToken = token
	a.telegramBotCacheUntil = now.Add(10 * time.Minute)
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
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(serverIconPNG)
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"status": "healthy", "time": time.Now().Unix(), "redis": a.redis != nil, "storage": "json"})
}

func (a *App) handleSystemStats(w http.ResponseWriter, r *http.Request, _ Params) {
	users := a.store.ListUsers()
	ok(w, "OK", map[string]any{"users": len(users), "total_users": len(users), "active_users": countActive(users), "redis_enabled": a.redis != nil, "routes": len(a.routes), "uptime": 0})
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
	for _, line := range a.cfg.EmbyURLList {
		lines = append(lines, map[string]string{"name": line.Name, "url": line.URL})
	}
	if a.cfg.EmbyPublicURL != "" {
		lines = append(lines, map[string]string{"name": "榛樿绾胯矾", "url": a.cfg.EmbyPublicURL})
	} else if len(lines) == 0 && a.cfg.EmbyURL != "" {
		lines = append(lines, map[string]string{"name": "榛樿绾胯矾", "url": a.cfg.EmbyURL})
	}
	whitelist := []map[string]string{}
	if u.Role == store.RoleAdmin || u.Role == store.RoleWhitelist {
		for _, line := range a.cfg.EmbyWhitelistURLList {
			whitelist = append(whitelist, map[string]string{"name": line.Name, "url": line.URL})
		}
		if a.cfg.EmbyWhitelistURL != "" {
			whitelist = append(whitelist, map[string]string{"name": "whitelist route", "url": a.cfg.EmbyWhitelistURL})
		}
	}
	ok(w, "OK", map[string]any{"lines": lines, "whitelist_lines": whitelist, "requires_emby_account": false, "requires_renewal": false, "emby_disabled_by_expiry": false})
}

func (a *App) handlePublicConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"upload_limit": a.cfg.MaxUploadSize, "bangumi_sync_enabled": a.cfg.BangumiEnabled, "telegram_mode": a.cfg.TelegramMode, "device_limit": map[string]any{"enabled": a.cfg.DeviceLimitEnabled, "max_devices": a.cfg.MaxDevices, "max_streams": a.cfg.MaxStreams}, "bangumi_sync": map[string]any{"enabled": a.cfg.BangumiEnabled}})
}

func (a *App) handleAdminConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"host": a.cfg.Host, "port": a.cfg.Port, "redis_enabled": a.redis != nil, "state_file": a.cfg.StateFile, "upload_dir": a.cfg.UploadDir})
}

func (a *App) handleConfigTOMLGet(w http.ResponseWriter, r *http.Request, _ Params) {
	path := a.configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		fail(w, http.StatusNotFound, "config file not found")
		return
	}
	ok(w, "OK", map[string]any{"content": string(data), "path": path})
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
	ok(w, "config check completed", map[string]any{"changed": false, "config_file": a.cfg.ConfigFile})
}

func (a *App) handleAPIRoutes(w http.ResponseWriter, r *http.Request, _ Params) {
	items := make([]map[string]string, 0, len(a.routes))
	for _, route := range a.routes {
		items = append(items, map[string]string{"method": route.Method, "path": strings.TrimPrefix(route.Pattern, "/api/v1"), "endpoint": route.Pattern, "full_path": route.Pattern})
	}
	ok(w, "OK", map[string]any{"apis": items, "total": len(items)})
}

func (a *App) handleBotTest(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.cfg.TelegramBotToken == "" {
		fail(w, http.StatusBadRequest, "未配置 Telegram Bot Token")
		return
	}
	apiURL := strings.TrimRight(firstNonEmpty(a.cfg.TelegramAPIURL, "https://api.telegram.org"), "/") + "/bot" + a.cfg.TelegramBotToken + "/getMe"
	var payload map[string]any
	if err := getJSON(r.Context(), apiURL, nil, &payload); err != nil {
		ok(w, "测试完成", map[string]any{"results": []map[string]any{{"target": "bot", "success": false, "error": err.Error()}}})
		return
	}
	result, _ := payload["result"].(map[string]any)
	ok(w, "测试完成", map[string]any{"results": []map[string]any{{"target": "bot", "success": payload["ok"] == true, "username": result["username"]}}})
}

func (a *App) handleEmbyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	info := map[string]any{}
	online := false
	if a.cfg.EmbyURL != "" {
		if err := a.embyGet(r.Context(), "/System/Info/Public", &info); err == nil {
			online = true
		} else if err := a.embyGet(r.Context(), "/System/Info", &info); err == nil {
			online = true
		}
	}
	sessions := []map[string]any{}
	if online {
		_ = a.embyGet(r.Context(), "/Sessions", &sessions)
	}
	ok(w, "OK", map[string]any{"online": online, "server": a.cfg.EmbyURL, "server_name": firstNonEmpty(asString(info["ServerName"]), asString(info["Name"])), "version": firstNonEmpty(asString(info["Version"]), "unknown"), "operating_system": asString(info["OperatingSystem"]), "active_sessions": len(sessions), "total_sessions": len(sessions), "is_synced": current(r).User.EmbyID != "", "is_active": current(r).User.Active, "message": "OK"})
}

func (a *App) handleDeprecatedEmbyURLs(w http.ResponseWriter, r *http.Request, _ Params) {
	fail(w, http.StatusGone, "该接口已废弃，请使用 /api/v1/system/emby-urls")
}

func (a *App) handleEmbyLatest(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.cfg.EmbyURL == "" {
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
		fail(w, http.StatusBadGateway, "failed to read latest Emby media")
		return
	}
	items, _ := payload["Items"].([]any)
	ok(w, "OK", map[string]any{"items": items, "total": int(numeric(payload["TotalRecordCount"]))})
}

func (a *App) handleSessionCount(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.cfg.EmbyURL == "" {
		ok(w, "OK", map[string]any{"active": 0, "total": 0})
		return
	}
	var sessions []map[string]any
	if err := a.embyGet(r.Context(), "/Sessions", &sessions); err != nil {
		fail(w, http.StatusBadGateway, "failed to read Emby sessions")
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
	users := a.store.ListUsers()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))
	roleFilter := r.URL.Query().Get("role")
	activeFilter := r.URL.Query().Get("active")
	embyFilter := r.URL.Query().Get("emby")
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

func (a *App) handleAdminUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	u, okUser := a.store.User(uid)
	if !okUser {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	ok(w, "OK", publicUser(u))
}

func (a *App) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
		if username := stringValue(payload, "username"); username != "" {
			u.Username = username
		}
		if email := stringValue(payload, "email"); email != "" {
			u.Email = email
		}
		if _, ok := payload["active"]; ok {
			u.Active = boolValue(payload, "active", u.Active)
		}
		if _, ok := payload["role"]; ok {
			u.Role = intValue(payload, "role", u.Role)
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "user updated", publicUser(u))
}

func (a *App) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	if uid == current(r).User.UID {
		fail(w, http.StatusForbidden, "cannot delete current admin")
		return
	}
	payload := decodeMap(r)
	mode := firstNonEmpty(stringValue(payload, "mode"), "local_only")
	depth := intValue(payload, "cascade_depth", queryInt(r, "cascade_depth", 1))
	deleted := []int64{}
	skipped := []map[string]any{}
	failed := []map[string]any{}
	for _, targetUID := range a.collectCascadeUIDs(uid, depth) {
		target, okUser := a.store.User(targetUID)
		if !okUser {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": "not_found"})
			continue
		}
		if target.Role == store.RoleAdmin {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": "admin"})
			continue
		}
		if mode == "emby_only" {
			if target.EmbyID != "" && a.cfg.EmbyURL != "" {
				if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil {
					failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: " + err.Error()})
					continue
				}
			}
			_, err := a.store.UpdateUser(targetUID, func(u *store.User) error { u.EmbyID = ""; u.EmbyUsername = ""; return nil })
			if err != nil {
				failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			} else {
				deleted = append(deleted, targetUID)
			}
			continue
		}
		if mode == "with_emby" && target.EmbyID != "" && a.cfg.EmbyURL != "" {
			if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(target.EmbyID)); err != nil {
				failed = append(failed, map[string]any{"uid": targetUID, "reason": "delete_emby: " + err.Error()})
				continue
			}
		}
		if err := a.store.DeleteUser(targetUID); err != nil {
			failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			continue
		}
		deleted = append(deleted, targetUID)
	}
	ok(w, "users deleted", map[string]any{"deleted": deleted, "skipped": skipped, "failed": failed, "mode": mode, "cascade_depth": depth})
}

func (a *App) handleAdminToggleUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	if target, okUser := a.store.User(uid); okUser && target.Role == store.RoleAdmin && target.UID != current(r).User.UID {
		fail(w, http.StatusForbidden, "cannot operate on another admin")
		return
	}
	enable := strings.HasSuffix(r.URL.Path, "/enable")
	payload := decodeMap(r)
	depth := intValue(payload, "cascade_depth", queryInt(r, "cascade_depth", 1))
	affected := []int64{}
	skipped := []map[string]any{}
	failed := []map[string]any{}
	for _, targetUID := range a.collectCascadeUIDs(uid, depth) {
		if target, okUser := a.store.User(targetUID); okUser && target.Role == store.RoleAdmin && target.UID != current(r).User.UID {
			skipped = append(skipped, map[string]any{"uid": targetUID, "reason": "admin"})
			continue
		}
		updated, err := a.store.UpdateUser(targetUID, func(u *store.User) error { u.Active = enable; return nil })
		if err != nil {
			failed = append(failed, map[string]any{"uid": targetUID, "reason": err.Error()})
			continue
		}
		if updated.EmbyID != "" && a.cfg.EmbyURL != "" {
			_ = a.embySetUserEnabled(r.Context(), updated.EmbyID, a.embyShouldEnableUser(updated))
		}
		affected = append(affected, targetUID)
	}
	u, _ := a.store.User(uid)
	ok(w, "鐢ㄦ埛鐘舵€佸凡鏇存柊", map[string]any{"user": publicUser(u), "active": enable, "affected": affected, "skipped": skipped, "failed": failed, "cascade_depth": depth, "enable": enable})
}

func (a *App) handleAdminUnbindEmby(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	u, err := a.store.UpdateUser(uid, func(u *store.User) error { u.EmbyID = ""; u.EmbyUsername = ""; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "Emby unbound", publicUser(u))
}

func (a *App) handleAdminForceUnbind(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	scope := stringValue(decodeMap(r), "scope")
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
		if scope == "telegram" || scope == "both" || scope == "" {
			u.TelegramID = 0
			u.TelegramUsername = ""
		}
		if scope == "emby" || scope == "both" || scope == "" {
			u.EmbyID = ""
			u.EmbyUsername = ""
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "unbound", map[string]any{"changed": true, "user": publicUser(u)})
}

func (a *App) handleRegistrationQueueClear(w http.ResponseWriter, r *http.Request, params Params) {
	if params["uid"] != "" {
		uid, _ := int64Param(params, "uid")
		u, err := a.store.UpdateUser(uid, func(u *store.User) error { u.PendingEmby = false; u.PendingEmbyDays = nil; return nil })
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
	for _, u := range a.store.ListUsers() {
		if u.PendingEmby {
			candidates = append(candidates, u.UID)
		}
	}
	if dryRun || confirm != "CLEAR_REGISTRATION_QUEUE" {
		ok(w, "dry-run", map[string]any{"dry_run": true, "candidates": candidates, "count": len(candidates), "confirm_required": "CLEAR_REGISTRATION_QUEUE"})
		return
	}
	for _, uid := range candidates {
		_, _ = a.store.UpdateUser(uid, func(u *store.User) error { u.PendingEmby = false; u.PendingEmbyDays = nil; return nil })
	}
	ok(w, "registration queue cleaned", map[string]any{"updated": len(candidates), "uids": candidates})
}

func (a *App) handleRegistrationEntitlement(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	days := intValue(decodeMap(r), "days", a.cfg.InviteDefaultDays)
	if days == 0 {
		days = -1
	}
	if days < -1 || days > 3650 {
		fail(w, http.StatusBadRequest, "days 瓒呭嚭鑼冨洿")
		return
	}
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
		if u.EmbyID != "" {
			return store.ErrConflict
		}
		u.PendingEmby = true
		u.PendingEmbyDays = &days
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
	days := intValue(payload, "days", a.cfg.InviteDefaultDays)
	candidates := []int64{}
	for _, u := range a.store.ListUsers() {
		if u.PendingEmby && u.EmbyID == "" && u.Active {
			candidates = append(candidates, u.UID)
		}
	}
	if dryRun || confirm != "GRANT_AND_CLEAR_REGISTRATION_QUEUE" {
		ok(w, "dry-run", map[string]any{"dry_run": true, "candidates": candidates, "count": len(candidates), "confirm_required": "GRANT_AND_CLEAR_REGISTRATION_QUEUE"})
		return
	}
	for _, uid := range candidates {
		_, _ = a.store.UpdateUser(uid, func(u *store.User) error {
			u.PendingEmby = true
			u.PendingEmbyDays = &days
			if u.Role == store.RoleUnrecognized {
				u.Role = store.RoleNormal
			}
			return nil
		})
	}
	ok(w, "batch access granted", map[string]any{"updated": len(candidates), "uids": candidates, "days": days})
}

func (a *App) handleSyncBindings(w http.ResponseWriter, r *http.Request, _ Params) {
	seenTG := map[int64]int64{}
	duplicates := []map[string]any{}
	missingEmby := []int64{}
	checked := 0
	for _, u := range a.store.ListUsers() {
		checked++
		if u.TelegramID != 0 {
			if previous, ok := seenTG[u.TelegramID]; ok {
				duplicates = append(duplicates, map[string]any{"telegram_id": u.TelegramID, "uids": []int64{previous, u.UID}})
			} else {
				seenTG[u.TelegramID] = u.UID
			}
		}
		if u.EmbyID != "" && a.cfg.EmbyURL != "" {
			var remote map[string]any
			if err := a.embyGet(r.Context(), "/Users/"+urlPathEscape(u.EmbyID), &remote); err != nil {
				missingEmby = append(missingEmby, u.UID)
			}
		}
	}
	ok(w, "binding sync checked", map[string]any{"matched": checked, "telegram_duplicates": duplicates, "emby_missing": missingEmby, "repaired": 0, "synced": checked - len(missingEmby), "failed": len(missingEmby)})
}

func (a *App) handleKickUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	u, okUser := a.store.User(uid)
	if !okUser {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	kicked := 0
	if a.cfg.EmbyURL != "" && u.EmbyID != "" {
		var sessions []map[string]any
		if err := a.embyGet(r.Context(), "/Sessions", &sessions); err == nil {
			for _, session := range sessions {
				if asString(session["UserId"]) == u.EmbyID {
					if sid := asString(session["Id"]); sid != "" {
						var ignored map[string]any
						if err := a.embyPost(r.Context(), "/Sessions/"+urlPathEscape(sid)+"/Logout", nil, &ignored); err == nil {
							kicked++
						}
					}
				}
			}
		}
	}
	ok(w, "会话踢出完成", map[string]any{"kicked_count": kicked})
}

func (a *App) handleBulkEnableLibrarySelfService(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != "ENABLE_LIBRARY_SELF_SERVICE" {
		fail(w, http.StatusBadRequest, "缺少确认标记")
		return
	}
	updated := 0
	for _, u := range a.store.ListUsers() {
		if u.Role == store.RoleUnrecognized {
			continue
		}
		if _, err := a.store.UpdateUser(u.UID, func(u *store.User) error { u.LibrarySelfService = true; return nil }); err == nil {
			updated++
		}
	}
	ok(w, "batch enabled", map[string]any{"updated": updated})
}

func (a *App) handleAdminRenewUser(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	days := int64(intValue(decodeMap(r), "days", 30))
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
		base := time.Now().Unix()
		if u.ExpiredAt > base {
			base = u.ExpiredAt
		}
		u.ExpiredAt = base + days*86400
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	if u.EmbyID != "" && a.cfg.EmbyURL != "" {
		_ = a.embySetUserEnabled(r.Context(), u.EmbyID, a.embyShouldEnableUser(u))
	}
	ok(w, "缁湡鎴愬姛", publicUser(u))
}

func (a *App) handleAdminResetPassword(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	if uid == 0 {
		uid = current(r).User.UID
	}
	body := decodeMap(r)
	scope := strings.ToLower(firstNonEmpty(stringValue(body, "scope"), "both"))
	if scope != "system" && scope != "emby" && scope != "both" {
		fail(w, http.StatusBadRequest, "invalid password reset scope")
		return
	}
	targetUser, okUser := a.store.User(uid)
	if !okUser {
		fail(w, http.StatusNotFound, "user not found")
		return
	}
	newPassword := stringValue(body, "password")
	autoGenerated := newPassword == ""
	if autoGenerated {
		newPassword = "Twilight-" + randomCode(12)
	} else if okPass, msg := validateStrongPassword(newPassword, "password"); !okPass {
		fail(w, http.StatusBadRequest, msg)
		return
	}
	if (scope == "emby" || scope == "both") && targetUser.EmbyID == "" {
		if scope == "emby" {
			fail(w, http.StatusBadRequest, "user has no linked Emby account")
			return
		}
		scope = "system"
	}
	if scope == "emby" || scope == "both" {
		if err := a.embySetPassword(r.Context(), targetUser.EmbyID, newPassword); err != nil {
			fail(w, http.StatusBadGateway, "failed to reset Emby password")
			return
		}
	}
	if scope == "system" || scope == "both" {
		hashValue, err := security.HashPassword(newPassword)
		if err != nil {
			fail(w, http.StatusInternalServerError, "password processing failed")
			return
		}
		if _, updateErr := a.store.UpdateUser(uid, func(u *store.User) error { u.PasswordHash = hashValue; return nil }); updateErr != nil {
			statusFromError(w, updateErr)
			return
		}
	}
	ok(w, "password reset", map[string]any{"scope": scope, "new_password": newPassword, "auto_generated": autoGenerated})
}

func (a *App) handleAdminLibrarySelfService(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	enabled := boolValue(decodeMap(r), "enabled", true)
	u, err := a.store.UpdateUser(uid, func(u *store.User) error { u.LibrarySelfService = enabled; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "updated", map[string]any{"uid": uid, "library_self_service": u.LibrarySelfService})
}

func (a *App) handleAdminSetRole(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	role := store.RoleNormal
	if boolValue(payload, "is_admin", boolValue(payload, "admin", false)) {
		role = store.RoleAdmin
	}
	// Prevent demoting the last admin - count remaining admins
	if role != store.RoleAdmin {
		target, okTarget := a.store.User(uid)
		if okTarget && target.Role == store.RoleAdmin {
			adminCount := 0
			for _, u := range a.store.ListUsers() {
				if u.Role == store.RoleAdmin && u.Active {
					adminCount++
				}
			}
			if adminCount <= 1 {
				fail(w, http.StatusConflict, "无法移除最后一个管理员的权限，系统至少需要一个管理员")
				return
			}
		}
	}
	u, err := a.store.UpdateUser(uid, func(u *store.User) error { u.Role = role; return nil })
	if statusFromError(w, err) {
		return
	}
	ok(w, "role updated", publicUser(u))
}

func (a *App) handleAdminUnbindTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	old := int64(0)
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
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
	ok(w, "Telegram unbound", map[string]any{"uid": u.UID, "username": u.Username, "old_telegram_id": old})
}

func (a *App) handleAdminBindTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	uid, _ := int64Param(params, "uid")
	payload := decodeMap(r)
	tgid := int64(intValue(payload, "telegram_id", 0))
	if tgid <= 0 {
		fail(w, http.StatusBadRequest, "telegram_id 鏃犳晥")
		return
	}
	if existing, okUser := a.store.FindUserByTelegramID(tgid); okUser && existing.UID != uid {
		fail(w, http.StatusConflict, "Telegram ID is already bound to another user")
		return
	}
	old := int64(0)
	u, err := a.store.UpdateUser(uid, func(u *store.User) error {
		if u.Role == store.RoleAdmin && u.UID != current(r).User.UID {
			return store.ErrConflict
		}
		old = u.TelegramID
		u.TelegramID = tgid
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "Telegram bound", map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": u.TelegramID, "old_telegram_id": zeroNil(old)})
}

func (a *App) handleUserByTelegram(w http.ResponseWriter, r *http.Request, params Params) {
	tgid, _ := int64Param(params, "telegram_id")
	for _, u := range a.store.ListUsers() {
		if u.TelegramID == tgid {
			ok(w, "OK", publicUser(u))
			return
		}
	}
	fail(w, http.StatusNotFound, "user not found")
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
		fail(w, http.StatusBadGateway, "failed to read Emby user")
		return
	}
	if !found {
		fail(w, http.StatusNotFound, "Emby user not found")
		return
	}
	embyID := asString(remoteUser["Id"])
	embyName := firstNonEmpty(asString(remoteUser["Name"]), embyNameInput, embyID)
	if existing, okExisting := a.store.FindUserByEmbyID(embyID); okExisting && existing.UID != targetUID {
		if !force {
			ok(w, "Emby account already linked", map[string]any{"conflict": true, "conflict_uid": existing.UID, "conflict_username": existing.Username, "emby_id": embyID, "emby_username": embyName})
			return
		}
		_, _ = a.store.UpdateUser(existing.UID, func(u *store.User) error {
			u.EmbyID = ""
			u.EmbyUsername = ""
			u.PendingEmby = true
			return nil
		})
	}
	updatedUser, updateErr := a.store.UpdateUser(targetUID, func(u *store.User) error {
		u.EmbyID = embyID
		u.EmbyUsername = embyName
		u.PendingEmby = false
		u.PendingEmbyDays = nil
		return nil
	})
	if statusFromError(w, updateErr) {
		return
	}
	_ = a.embySetUserEnabled(r.Context(), updatedUser.EmbyID, a.embyShouldEnableUser(updatedUser))
	ok(w, "Emby account linked", map[string]any{"uid": updatedUser.UID, "emby_id": updatedUser.EmbyID, "emby_username": updatedUser.EmbyUsername, "force_taken": force, "previous_uid": nil, "user": publicUser(updatedUser)})
}

func (a *App) handleTelegramRosterStats(w http.ResponseWriter, r *http.Request, _ Params) {
	chats := telegramChatIDs(a.cfg.TelegramGroupIDs)
	chatID := ""
	if len(chats) > 0 {
		chatID = chats[0]
	}
	stats := a.store.TelegramRosterStats(chatID)
	entries := a.store.TelegramRoster(chatID, true)
	bound := 0
	unbound := 0
	if len(entries) > 0 {
		for _, entry := range entries {
			if entry.IsBot {
				continue
			}
			if _, okUser := a.store.FindUserByTelegramID(entry.TelegramID); okUser {
				bound++
			} else {
				unbound++
			}
		}
	} else {
		for _, u := range a.store.ListUsers() {
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

func (a *App) handleAdminAnnouncements(w http.ResponseWriter, r *http.Request, _ Params) {
	anns := a.store.ListAnnouncements(true)
	ok(w, "OK", map[string]any{"announcements": anns, "total": len(anns)})
}
func (a *App) handleAnnouncements(w http.ResponseWriter, r *http.Request, _ Params) {
	anns := a.store.ListAnnouncements(false)
	ok(w, "OK", map[string]any{"announcements": anns, "total": len(anns)})
}

func (a *App) handleCreateAnnouncement(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	ann, err := a.store.UpsertAnnouncement(store.Announcement{Title: firstNonEmpty(stringValue(payload, "title"), "鍏憡"), Content: stringValue(payload, "content"), Visible: boolValue(payload, "visible", true), Level: firstNonEmpty(stringValue(payload, "level"), "info")})
	if statusFromError(w, err) {
		return
	}
	created(w, "announcement created", ann)
}

func (a *App) handleUpdateAnnouncement(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "announcement_id")
	payload := decodeMap(r)
	ann, err := a.store.UpsertAnnouncement(store.Announcement{ID: id, Title: firstNonEmpty(stringValue(payload, "title"), "鍏憡"), Content: stringValue(payload, "content"), Visible: boolValue(payload, "visible", true), Level: firstNonEmpty(stringValue(payload, "level"), "info")})
	if statusFromError(w, err) {
		return
	}
	ok(w, "announcement updated", ann)
}

func (a *App) handleDeleteAnnouncement(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "announcement_id")
	if statusFromError(w, a.store.DeleteAnnouncement(id)) {
		return
	}
	ok(w, "announcement deleted", nil)
}

func (a *App) handleAPIKeyInfo(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"user": publicUser(current(r).User), "permissions": current(r).APIKey.Permissions})
}
func (a *App) handleAPIKeyStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"active": current(r).User.Active, "expired_at": current(r).User.ExpiredAt})
}
func (a *App) handleAPIKeyRenew(w http.ResponseWriter, r *http.Request, params Params) {
	a.handleRenew(w, r, params)
}
func (a *App) handleAPIKeyPermissions(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"permissions": current(r).APIKey.Permissions, "all_permissions": defaultPermissions()})
}
func (a *App) handleForbiddenSelfPermission(w http.ResponseWriter, r *http.Request, _ Params) {
	fail(w, http.StatusForbidden, "不允许通过当前 API Key 修改自身权限")
}

func (a *App) handleExportUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "email", "role", "active"})
	for _, u := range a.store.ListUsers() {
		_ = cw.Write([]string{strconv.FormatInt(u.UID, 10), u.Username, u.Email, strconv.Itoa(u.Role), strconv.FormatBool(u.Active)})
	}
	cw.Flush()
}

func (a *App) handleExportPlayback(w http.ResponseWriter, r *http.Request, _ Params) {
	days := queryInt(r, "days", 30)
	since := int64(0)
	if days > 0 {
		since = time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	}
	records := a.store.PlaybackRecords(0, since, 10000)
	usernameByUID := map[int64]string{}
	for _, u := range a.store.ListUsers() {
		usernameByUID[u.UID] = u.Username
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=playback_stats.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "username", "item_id", "title", "media_type", "duration_seconds", "played_at"})
	for _, record := range records {
		_ = cw.Write([]string{
			strconv.FormatInt(record.UID, 10),
			usernameByUID[record.UID],
			record.ItemID,
			record.Title,
			record.MediaType,
			strconv.FormatInt(record.Duration, 10),
			strconv.FormatInt(record.PlayedAt, 10),
		})
	}
	cw.Flush()
	return
}

func (a *App) handleWatchStats(w http.ResponseWriter, r *http.Request, _ Params) {
	uid := current(r).User.UID
	parts := splitPath(r.URL.Path)
	global := false
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last == "global" {
			global = true
			uid = 0
		} else if parsed, err := strconv.ParseInt(last, 10, 64); err == nil && strings.Contains(r.URL.Path, "/watch-stats/") {
			uid = parsed
		}
	}
	if strings.Contains(r.URL.Path, "/stats/user/") && current(r).User.Role != store.RoleAdmin && uid != current(r).User.UID {
		fail(w, http.StatusForbidden, "cannot view another user's watch stats")
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
	records := a.store.PlaybackRecords(uid, since, 1000)
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

func (a *App) handleBatchDisableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleBatchToggleUsers(w, r, false)
}

func (a *App) handleBatchEnableUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	a.handleBatchToggleUsers(w, r, true)
}

func (a *App) handleBatchToggleUsers(w http.ResponseWriter, r *http.Request, enable bool) {
	payload := decodeMap(r)
	uids := int64Slice(payload["uids"])
	result := batchResult(len(uids))
	for _, uid := range uids {
		updated, err := a.store.UpdateUser(uid, func(u *store.User) error { u.Active = enable; return nil })
		if err == nil && updated.EmbyID != "" && a.cfg.EmbyURL != "" {
			if syncErr := a.embySetUserEnabled(r.Context(), updated.EmbyID, a.embyShouldEnableUser(updated)); syncErr != nil {
				err = syncErr
			}
		}
		addBatchOutcome(result, uid, err)
	}
	ok(w, "批量操作完成", result)
}

func (a *App) handleBatchRenewUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	uids := int64Slice(payload["uids"])
	days := intValue(payload, "days", 30)
	if days <= 0 {
		fail(w, http.StatusBadRequest, "days 必须大于 0")
		return
	}
	result := batchResult(len(uids))
	for _, uid := range uids {
		_, err := a.store.UpdateUser(uid, func(u *store.User) error {
			base := time.Now().Unix()
			if u.ExpiredAt > base {
				base = u.ExpiredAt
			}
			u.ExpiredAt = base + int64(days)*86400
			return nil
		})
		addBatchOutcome(result, uid, err)
	}
	result["days"] = days
	ok(w, "批量续期完成", result)
}

func (a *App) handleBatchDeleteUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	uids := int64Slice(payload["uids"])
	deleteEmby := boolValue(payload, "delete_emby", r.URL.Query().Get("delete_emby") != "false")
	result := batchResult(len(uids))
	for _, uid := range uids {
		if uid == current(r).User.UID {
			addBatchOutcome(result, uid, fmt.Errorf("cannot delete current admin"))
			continue
		}
		if deleteEmby && a.cfg.EmbyURL != "" {
			if u, okUser := a.store.User(uid); okUser && u.EmbyID != "" {
				if err := a.embyDelete(r.Context(), "/Users/"+urlPathEscape(u.EmbyID)); err != nil {
					addBatchOutcome(result, uid, err)
					continue
				}
			}
		}
		addBatchOutcome(result, uid, a.store.DeleteUser(uid))
	}
	ok(w, "批量删除完成", result)
}

func (a *App) handleExpiringUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	days := queryInt(r, "days", 3)
	deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()
	now := time.Now().Unix()
	items := []map[string]any{}
	for _, u := range a.store.ListUsers() {
		if u.ExpiredAt > now && u.ExpiredAt <= deadline {
			remaining := u.ExpiredAt - now
			items = append(items, map[string]any{"uid": u.UID, "username": u.Username, "telegram_id": nullableInt(u.TelegramID), "expired_at": u.ExpiredAt, "remaining_seconds": remaining, "remaining_str": formatSeconds(remaining)})
		}
	}
	ok(w, "OK", map[string]any{"days": days, "count": len(items), "users": items})
}

func (a *App) handleSigninConfig(w http.ResponseWriter, r *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"enabled": true, "currency_name": "积分", "daily_points": 1})
}
func (a *App) handleSigninMe(w http.ResponseWriter, r *http.Request, _ Params) {
	si := a.store.Signin(current(r).User.UID)
	ok(w, "OK", map[string]any{"points": si.Points, "streak": si.Streak, "last_signin": si.LastSignin, "currency_name": "绉垎"})
}
func (a *App) handleSignin(w http.ResponseWriter, r *http.Request, _ Params) {
	si, createdToday, err := a.store.AddSignin(current(r).User.UID, 1)
	if statusFromError(w, err) {
		return
	}
	ok(w, "OK", map[string]any{"created": createdToday, "points": si.Points, "streak": si.Streak, "award": 1, "currency_name": "绉垎"})
}
func (a *App) handleSigninHistory(w http.ResponseWriter, r *http.Request, _ Params) {
	si := a.store.Signin(current(r).User.UID)
	records := append([]store.SigninRecord(nil), si.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt > records[j].CreatedAt })
	ok(w, "OK", map[string]any{"records": records, "currency_name": "绉垎"})
}

func (a *App) handleDemoBootstrap(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	role := strings.ToLower(firstNonEmpty(r.URL.Query().Get("role"), "user"))
	if role != "admin" {
		role = "user"
	}
	ok(w, "OK", map[string]any{
		"readonly": true,
		"notice":   "TestWeb 演示接口只返回固定模拟数据，不读取登录态，不执行真实写入。",
		"user":     map[string]any{"uid": 1, "username": "demo_" + role, "role": map[string]int{"admin": 0, "user": 1}[role], "role_name": role, "active": true},
		"metrics": map[string]any{
			"admin": []map[string]string{{"label": "总用户", "value": "186", "description": "+12 本月"}, {"label": "Emby 绑定", "value": "143", "description": "77%"}, {"label": "待处理求片", "value": "8", "description": "3 个下载中"}, {"label": "定时任务", "value": "11", "description": "9 个启用"}},
			"user":  []map[string]string{{"label": "账号状态", "value": "正常", "description": "Emby 已绑定"}, {"label": "剩余天数", "value": "42", "description": "到期提醒开启"}, {"label": "积分", "value": "1,280", "description": "今日已签到"}, {"label": "求片", "value": "3", "description": "1 个已完成"}},
		},
		"stats": map[string]any{"users": 186, "requests": 8, "readonly": true},
	})
}
func (a *App) handleDemoMe(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"uid": 1, "username": "demo", "role": 0, "role_name": "Admin", "active": true})
}
func (a *App) handleDemoUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"users": []map[string]any{{"uid": 1, "username": "demo", "role": 0, "active": true}}, "total": 1})
}
func (a *App) handleDemoRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	ok(w, "OK", map[string]any{"regcodes": []any{}, "total": 0})
}
func (a *App) handleDemoMediaSearch(w http.ResponseWriter, r *http.Request, _ Params) {
	setDemoHeaders(w)
	query := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"), r.URL.Query().Get("keyword"))))
	items := []map[string]any{
		{"title": "The Bear", "type": "剧集", "year": "2022", "status": "可求片", "rating": "8.6", "source": "demo"},
		{"title": "Dune: Part Two", "type": "电影", "year": "2024", "status": "已入库", "rating": "8.4", "source": "demo"},
		{"title": "Frieren", "type": "动画", "year": "2023", "status": "处理中", "rating": "9.1", "source": "demo"},
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if query == "" || strings.Contains(strings.ToLower(asString(item["title"])), query) {
			results = append(results, item)
		}
	}
	ok(w, "OK", map[string]any{"results": results, "total": len(results), "readonly": true})
}
func (a *App) handleDemoAction(w http.ResponseWriter, r *http.Request, params Params) {
	setDemoHeaders(w)
	if !a.limiter.Allow(r.Context(), rateKey("demo-action:", a.clientIP(r)), 60, time.Minute) {
		fail(w, http.StatusTooManyRequests, "演示操作过于频繁")
		return
	}
	action := strings.TrimSpace(params["action_name"])
	if action == "" {
		action = "noop"
	}
	if !demoActionPattern.MatchString(action) || strings.ContainsAny(action, "/\\\x00\r\n\t") {
		fail(w, http.StatusBadRequest, "演示操作名称无效")
		return
	}
	ok(w, "OK", map[string]any{"demo": true, "action": action, "mutated": false, "readonly": true, "simulated": true})
}

func setDemoHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Twilight-Demo", "true")
}

func randomCode(length int) string {
	buf := make([]byte, (length+1)/2)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
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
