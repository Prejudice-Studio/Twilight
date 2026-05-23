package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

func (a *App) embyIsAdmin(ctx context.Context, embyID string) bool {
	if embyID == "" || a.cfg.EmbyURL == "" {
		return false
	}
	now := time.Now()
	a.embyAdminMu.Lock()
	if cached, ok := a.embyAdminCache[embyID]; ok && now.Sub(cached.checked) < 5*time.Minute {
		a.embyAdminMu.Unlock()
		return cached.admin
	}
	a.embyAdminMu.Unlock()

	user, found, err := a.embyUserByID(ctx, embyID)
	if err != nil || !found {
		return false
	}
	policy := embyPolicy(user)
	isAdmin := boolish(policy["IsAdministrator"])
	a.embyAdminMu.Lock()
	a.embyAdminCache[embyID] = embyAdminCacheEntry{admin: isAdmin, checked: now}
	a.embyAdminMu.Unlock()
	return isAdmin
}

func (a *App) requireNonEmbyAdmin(w http.ResponseWriter, r *http.Request, user store.User) bool {
	if user.Role == store.RoleAdmin {
		return false
	}
	if user.EmbyID == "" {
		return false
	}
	if a.embyIsAdmin(r.Context(), user.EmbyID) {
		zap.L().Warn("blocked sensitive operation for non-admin user with Emby admin account",
			zap.Int64("uid", user.UID), zap.String("username", user.Username), zap.String("emby_id", user.EmbyID))
		fail(w, http.StatusForbidden, "安全限制：您绑定的 Emby 账号具有管理员权限，但您不是系统管理员。为防止越权操作，已禁止此请求。请联系系统管理员。")
		return true
	}
	return false
}

func (a *App) blockRestrictedEmbyAdmin(w http.ResponseWriter, r *http.Request, route *Route, user store.User) bool {
	if route == nil || route.Auth == AuthAdmin || user.Role == store.RoleAdmin || user.EmbyID == "" {
		return false
	}
	if !a.embyIsAdmin(r.Context(), user.EmbyID) {
		return false
	}
	if embyAdminRestrictionAllowed(r.Method, r.URL.Path) {
		return false
	}
	zap.L().Warn("blocked request for non-admin user bound to Emby administrator",
		zap.Int64("uid", user.UID), zap.String("username", user.Username), zap.String("method", r.Method), zap.String("path", r.URL.Path))
	fail(w, http.StatusForbidden, "安全限制：当前系统账号不是管理员，但绑定的 Emby 账号具有管理员权限。除查看账号状态和退出登录外，所有操作已被禁止，请联系系统管理员处理。")
	return true
}

func embyAdminRestrictionAllowed(method, requestPath string) bool {
	if method == http.MethodPost && (requestPath == "/api/v1/auth/logout" || requestPath == "/api/v1/auth/logout/all") {
		return true
	}
	if method == http.MethodGet && (requestPath == "/api/v1/auth/me" || requestPath == "/api/v1/users/me") {
		return true
	}
	return false
}

func (a *App) embyUserByName(ctx context.Context, username string) (map[string]any, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, false, nil
	}
	var users []map[string]any
	if err := a.embyGet(ctx, "/Users", &users); err != nil {
		return nil, false, err
	}
	for _, user := range users {
		if strings.EqualFold(asString(user["Name"]), username) {
			return user, true, nil
		}
	}
	return nil, false, nil
}

func (a *App) embyUserByID(ctx context.Context, id string) (map[string]any, bool, error) {
	if strings.TrimSpace(id) == "" {
		return nil, false, nil
	}
	var user map[string]any
	if err := a.embyGet(ctx, "/Users/"+urlPathEscape(id), &user); err != nil {
		if strings.Contains(err.Error(), "remote status 404") {
			return nil, false, nil
		}
		return nil, false, err
	}
	return user, true, nil
}

func (a *App) embyCreateUser(ctx context.Context, username, password string) (map[string]any, error) {
	var created map[string]any
	if err := a.embyPost(ctx, "/Users/New", map[string]any{"Name": username}, &created); err != nil {
		return nil, err
	}
	id := asString(created["Id"])
	if id == "" {
		return nil, fmt.Errorf("Emby did not return a user id")
	}
	_ = a.embyUpdatePolicy(ctx, id, func(policy map[string]any) {
		policy["EnableContentDownloading"] = false
	})
	if password != "" {
		if err := a.embySetPassword(ctx, id, password); err != nil {
			_ = a.embyDelete(ctx, "/Users/"+urlPathEscape(id))
			return nil, err
		}
	}
	return created, nil
}

func (a *App) embySetPassword(ctx context.Context, userID, password string) error {
	var ignored map[string]any
	if err := a.embyPost(ctx, "/Users/"+urlPathEscape(userID)+"/Password", map[string]any{"ResetPassword": true}, &ignored); err != nil {
		return err
	}
	if password == "" {
		return nil
	}
	return a.embyPost(ctx, "/Users/"+urlPathEscape(userID)+"/Password", map[string]any{"CurrentPw": "", "NewPw": password}, &ignored)
}

func (a *App) embyUpdatePolicy(ctx context.Context, userID string, update func(map[string]any)) error {
	user, found, err := a.embyUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("Emby user not found")
	}
	policy := map[string]any{}
	if existing, ok := user["Policy"].(map[string]any); ok {
		for key, value := range existing {
			policy[key] = value
		}
	}
	update(policy)
	var ignored map[string]any
	return a.embyPost(ctx, "/Users/"+urlPathEscape(userID)+"/Policy", policy, &ignored)
}

func (a *App) embySetUserEnabled(ctx context.Context, userID string, enabled bool) error {
	return a.embyUpdatePolicy(ctx, userID, func(policy map[string]any) {
		policy["IsDisabled"] = !enabled
	})
}

func (a *App) embyShouldEnableUser(u store.User) bool {
	return u.Active && !embyAccessExpired(u)
}

func embyAccessExpired(u store.User) bool {
	return u.EmbyID != "" && u.ExpiredAt > 0 && u.ExpiredAt < time.Now().Unix()
}

func validateStrongPassword(password, label string) (bool, string) {
	if password == "" {
		return false, "missing " + label
	}
	if len(password) < 8 {
		return false, label + " must be at least 8 characters"
	}
	if len(password) > 128 {
		return false, label + " is too long"
	}
	hasLower, hasUpper, hasDigit := false, false, false
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLower || !hasUpper || !hasDigit {
		return false, label + " must include lowercase, uppercase and digits"
	}
	return true, ""
}
