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

// embyAdminCacheMaxEntries 限制 embyIsAdmin 的内存缓存大小，
// 防止任意 emby_id 输入下 map 无限增长。
// 命中下用 LRU 风格的"超上限就丢最老"策略：写入时若超过上限，
// 先扫一遍把过期 entry（>5min）清掉；如仍超过上限，再丢一个最早的。
// 因为 5 分钟 TTL 已限制单条目寿命，这里用最简单的扫描淘汰即可（O(N) 但
// 仅在写入越界时触发，且 N 受 max 控制）。
const embyAdminCacheMaxEntries = 10000

func (a *App) embyIsAdmin(ctx context.Context, embyID string) bool {
	if embyID == "" || a.cfg().EmbyURL == "" {
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
	a.evictEmbyAdminCacheLocked(now)
	a.embyAdminCache[embyID] = embyAdminCacheEntry{admin: isAdmin, checked: now}
	a.embyAdminMu.Unlock()
	return isAdmin
}

// evictEmbyAdminCacheLocked 必须持有 embyAdminMu 调用：
//  1. 先扫一遍淘汰过期项（>5min）
//  2. 若仍 ≥ max，丢掉 checked 最早的一项（防 OOM 兜底）
func (a *App) evictEmbyAdminCacheLocked(now time.Time) {
	if len(a.embyAdminCache) < embyAdminCacheMaxEntries {
		return
	}
	for k, v := range a.embyAdminCache {
		if now.Sub(v.checked) >= 5*time.Minute {
			delete(a.embyAdminCache, k)
		}
	}
	if len(a.embyAdminCache) < embyAdminCacheMaxEntries {
		return
	}
	var oldestKey string
	var oldestAt time.Time
	for k, v := range a.embyAdminCache {
		if oldestKey == "" || v.checked.Before(oldestAt) {
			oldestKey, oldestAt = k, v.checked
		}
	}
	if oldestKey != "" {
		delete(a.embyAdminCache, oldestKey)
	}
}

// embyHealth 统一封装 emby 健康探活：先 /System/Info/Public（无需鉴权），
// 失败再 /System/Info（带 token 鉴权），把"双段 fallback"集中一处。
// 之前同样的 if/else 分散在 admin_extra.go / handlers.go(×2) / telegram_bot.go，
// 而且每处超时各异（1.5s / 10s / 无超时）。统一后调用方只需选超时档位
// （embyHealthFast 1.5s / 默认 5s / 自定义 ctx），不再自己写 fallback。
func (a *App) embyHealth(ctx context.Context) (info map[string]any, ok bool) {
	if a.cfg().EmbyURL == "" {
		return nil, false
	}
	if err := a.embyGet(ctx, "/System/Info/Public", &info); err == nil && info != nil {
		return info, true
	}
	info = nil
	if err := a.embyGet(ctx, "/System/Info", &info); err == nil && info != nil {
		return info, true
	}
	return nil, false
}

// embyHealthFast 是 embyHealth 的 1.5 秒超时版本，专用于"系统首页摘要"
// 这类不能阻塞用户响应的场景。
func (a *App) embyHealthFast(parent context.Context) (map[string]any, bool) {
	ctx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
	defer cancel()
	return a.embyHealth(ctx)
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
		failWithCode(w, http.StatusForbidden, ErrEmbyAdminBlocked, "安全限制：您绑定的 Emby 账号具有管理员权限，但您不是系统管理员。为防止越权操作，已禁止此请求。请联系系统管理员。")
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
	failWithCode(w, http.StatusForbidden, ErrEmbyAdminRestricted, "安全限制：当前系统账号不是管理员，但绑定的 Emby 账号具有管理员权限。除查看账号状态和退出登录外，所有操作已被禁止，请联系系统管理员处理。")
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
	// embySetPassword 是 Emby 标准两步：① ResetPassword=true 把账号清空成"无密码"
	// ② NewPw 写新密码。② 一旦失败（context deadline、Emby 5xx、网络抖动），账号
	// 会停留在"任何人 LoginByName 都能进"的危险状态。
	//
	// 这里加最小可用的回滚/重试：
	//   - password=="" 走 ResetPassword 单步即可，无需第二步。
	//   - password != ""：① 成功后 ② 用独立 ctx + 重试（最多 3 次，指数退避）
	//     执行；如全部失败，再 fallback 走"用一个不可登陆的强随机密码挡门"
	//     避免账号停在无密码状态，最后把原始错误返回给调用方。
	if err := a.embyPost(ctx, "/Users/"+urlPathEscape(userID)+"/Password", map[string]any{"ResetPassword": true}, &ignored); err != nil {
		return err
	}
	if password == "" {
		return nil
	}
	setPw := func(opCtx context.Context, pw string) error {
		return a.embyPost(opCtx, "/Users/"+urlPathEscape(userID)+"/Password", map[string]any{"CurrentPw": "", "NewPw": pw}, &ignored)
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				lastErr = ctx.Err()
			case <-time.After(time.Duration(attempt*attempt) * 200 * time.Millisecond):
			}
		}
		if err := setPw(ctx, password); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	// 兜底：尝试用一个强随机密码堵住"无密码窗口"。这一步独立于调用方 ctx：
	// 即便外层 ctx 已经 cancel，我们也尽力关门，再把原 lastErr 返回给调用方。
	guardCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	guardPw := randomCode(32)
	if err := setPw(guardCtx, guardPw); err != nil {
		zap.L().Error("emby password rollback failed; account may be left without a password",
			zap.String("emby_user_id", userID),
			zap.String("guard_error", redactSensitiveText(err.Error())),
			zap.String("origin_error", redactSensitiveText(lastErr.Error())),
		)
	} else {
		zap.L().Warn("emby password write failed; account locked with random guard password",
			zap.String("emby_user_id", userID),
			zap.String("origin_error", redactSensitiveText(lastErr.Error())),
		)
	}
	return lastErr
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
