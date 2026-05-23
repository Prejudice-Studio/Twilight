package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/redis"
	"github.com/prejudice-studio/twilight/internal/store"
)

type AuthLevel int

const (
	AuthPublic AuthLevel = iota
	AuthUser
	AuthAdmin
	AuthAPIKey
)

type Params map[string]string
type HandlerFunc func(http.ResponseWriter, *http.Request, Params)

type Route struct {
	Method  string
	Pattern string
	Auth    AuthLevel
	Handler HandlerFunc
}

type App struct {
	cfg                   config.Config
	store                 *store.Store
	sessions              *sessionStore
	limiter               *rateLimiter
	redis                 *redis.Client
	routes                []Route
	runtimeMu             sync.Mutex
	telegramBotMu         sync.Mutex
	telegramBotCacheToken string
	telegramBotCacheUntil time.Time
	telegramBotCache      map[string]any
}

type principal struct {
	User       store.User
	APIKey     store.APIKey
	Token      string
	FromCookie bool
}

type contextKey string

const principalKey contextKey = "principal"

func New(cfg config.Config, st *store.Store) (*App, error) {
	redisClient, err := newRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	app := &App{
		cfg:      cfg,
		store:    st,
		sessions: newSessionStore(cfg.SessionTTL, redisClient),
		limiter:  newRateLimiter(redisClient),
		redis:    redisClient,
	}
	app.registerRoutes()
	return app, nil
}

func newRedisClient(cfg config.Config) (*redis.Client, error) {
	redisClient, err := redis.New(cfg.RedisURL, 32)
	if err != nil {
		return nil, err
	}
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := redisClient.Ping(ctx); err != nil {
			cancel()
			slog.Warn("redis configured but unavailable; using memory fallback", "error", err)
			redisClient = nil
		} else {
			cancel()
			slog.Info("redis enabled for sessions and rate limits")
		}
	}
	return redisClient, nil
}

func (a *App) Routes() []Route {
	out := make([]Route, len(a.routes))
	copy(out, a.routes)
	return out
}

func (a *App) reloadConfig() (map[string]any, error) {
	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()

	previous := a.cfg
	next, err := config.Load(previous.ConfigFile)
	if err != nil {
		return nil, err
	}

	reinitialized := []string{}
	if previous.RedisURL != next.RedisURL || previous.SessionTTL != next.SessionTTL {
		redisClient, err := newRedisClient(next)
		if err != nil {
			return nil, err
		}
		oldRedis := a.redis
		a.sessions = newSessionStore(next.SessionTTL, redisClient)
		a.limiter = newRateLimiter(redisClient)
		a.redis = redisClient
		if oldRedis != nil && oldRedis != redisClient {
			_ = oldRedis.Close()
		}
		reinitialized = append(reinitialized, "sessions", "rate_limiter")
	}

	a.cfg = next
	restartRequired := []string{}
	if previous.DatabaseDriver != next.DatabaseDriver || previous.StateFile != next.StateFile || previous.PostgresDSN() != next.PostgresDSN() {
		restartRequired = append(restartRequired, "database")
	}
	if previous.Host != next.Host || previous.Port != next.Port {
		restartRequired = append(restartRequired, "listen_addr")
	}

	return map[string]any{
		"reloaded":            true,
		"config_file":         next.ConfigFile,
		"reinitialized":       reinitialized,
		"restart_required":    restartRequired,
		"active_database":     a.store.Backend(),
		"configured_database": next.DatabaseDriver,
	}, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("panic in api handler", "panic", recovered)
			fail(w, http.StatusInternalServerError, "服务器内部错误")
		}
	}()
	a.applySecurityHeaders(w)
	if a.applyCORS(w, r) && r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if a.cfg.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadSize)
	}
	if a.store.IsIPBlacklisted(a.clientIP(r)) {
		fail(w, http.StatusForbidden, "IP 已被封禁")
		return
	}

	if !a.limiter.Allow(r.Context(), rateKey("global:", a.clientIP(r)), 600, time.Minute) {
		fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	route, params, methodAllowed := a.match(r.Method, r.URL.Path)
	if route == nil {
		if methodAllowed {
			fail(w, http.StatusMethodNotAllowed, "请求方法不允许")
		} else {
			fail(w, http.StatusNotFound, "接口不存在")
		}
		return
	}

	principal, ok := a.authenticate(w, r, route.Auth)
	if !ok {
		return
	}
	if principal != nil && principal.FromCookie && isMutating(r.Method) && r.Header.Get("X-Twilight-Client") != "webui" {
		fail(w, http.StatusForbidden, "缺少客户端校验头")
		return
	}
	if principal != nil {
		r = r.WithContext(context.WithValue(r.Context(), principalKey, *principal))
	}
	route.Handler(w, r, params)
}

func (a *App) add(method, pattern string, auth AuthLevel, handler HandlerFunc) {
	a.routes = append(a.routes, Route{Method: method, Pattern: pattern, Auth: auth, Handler: handler})
}

func (a *App) match(method, requestPath string) (*Route, Params, bool) {
	methodAllowed := false
	for i := range a.routes {
		route := &a.routes[i]
		params, ok := matchPattern(route.Pattern, requestPath)
		if !ok {
			continue
		}
		if route.Method != method {
			methodAllowed = true
			continue
		}
		return route, params, true
	}
	return nil, nil, methodAllowed
}

func matchPattern(pattern, requestPath string) (Params, bool) {
	pp := splitPath(pattern)
	rp := splitPath(requestPath)
	if len(pp) != len(rp) {
		return nil, false
	}
	params := Params{}
	for i := range pp {
		if strings.HasPrefix(pp[i], ":") {
			params[strings.TrimPrefix(pp[i], ":")] = rp[i]
			continue
		}
		if pp[i] != rp[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(p string) []string {
	p = path.Clean("/" + p)
	if p == "/" {
		return nil
	}
	return strings.Split(strings.Trim(p, "/"), "/")
}

func (a *App) authenticate(w http.ResponseWriter, r *http.Request, auth AuthLevel) (*principal, bool) {
	if auth == AuthPublic {
		return nil, true
	}
	if auth == AuthAPIKey {
		p, ok := a.authenticateAPIKey(r)
		if !ok {
			fail(w, http.StatusUnauthorized, "API Key 无效")
			return nil, false
		}
		return p, true
	}
	p, ok := a.authenticateUser(r)
	if !ok {
		fail(w, http.StatusUnauthorized, "登录状态已失效，请重新登录")
		return nil, false
	}
	if !p.User.Active {
		fail(w, http.StatusForbidden, "账号已被禁用")
		return nil, false
	}
	if auth == AuthAdmin && p.User.Role != store.RoleAdmin {
		fail(w, http.StatusForbidden, "需要管理员权限")
		return nil, false
	}
	return p, true
}

func (a *App) authenticateUser(r *http.Request) (*principal, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	fromCookie := false
	if token == "" {
		if cookie, err := r.Cookie(a.cfg.SessionCookie); err == nil {
			token = cookie.Value
			fromCookie = true
		}
	}
	uid, ok := a.sessions.Get(r.Context(), token)
	if !ok {
		return nil, false
	}
	u, ok := a.store.User(uid)
	if !ok {
		return nil, false
	}
	return &principal{User: u, Token: token, FromCookie: fromCookie}, true
}

func (a *App) authenticateAPIKey(r *http.Request) (*principal, bool) {
	key := strings.TrimSpace(r.Header.Get("X-API-Key"))
	fromQuery := false
	if key == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		lower := strings.ToLower(auth)
		if strings.HasPrefix(lower, "bearer ") {
			key = strings.TrimSpace(auth[7:])
		} else if strings.HasPrefix(lower, "apikey ") {
			key = strings.TrimSpace(auth[7:])
		}
	}
	if key == "" && r.URL.Query().Get("apikey") != "" {
		key = strings.TrimSpace(r.URL.Query().Get("apikey"))
		fromQuery = true
	}
	if key == "" {
		return nil, false
	}
	hash := hashAPIKey(key)
	ak, u, ok := a.store.FindAPIKeyByHash(hash)
	if !ok || !u.Active {
		return nil, false
	}
	if fromQuery && (ak.ID == 0 || !ak.AllowQuery) {
		return nil, false
	}
	limit := ak.RateLimit
	if limit <= 0 {
		limit = 100
	}
	if !a.limiter.Allow(r.Context(), rateKey("apikey:", hash), limit, time.Minute) {
		return nil, false
	}
	if ak.ID > 0 {
		_ = a.store.RecordAPIKeyUse(ak.ID)
	}
	return &principal{User: u, APIKey: ak}, true
}

func current(r *http.Request) principal {
	p, _ := r.Context().Value(principalKey).(principal)
	return p
}

func bearerToken(header string) string {
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return ""
}

func (a *App) applySecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
}

func (a *App) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := normalizeCORSOrigin(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	allowed := false
	for _, candidate := range a.cfg.CORSOrigins {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			continue
		}
		if strings.EqualFold(normalizeCORSOrigin(candidate), origin) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Twilight-Client")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	w.Header().Set("Access-Control-Max-Age", "600")
	return true
}

func normalizeCORSOrigin(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" || raw == "*" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	parsed.Scheme = scheme
	parsed.Path = ""
	return parsed.String()
}

func (a *App) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	cookie := &http.Cookie{
		Name:     a.cfg.SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: sameSite(a.cfg.CookieSameSite),
	}
	http.SetCookie(w, cookie)
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: a.cfg.SessionCookie, Path: "/", MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: sameSite(a.cfg.CookieSameSite)})
}

func sameSite(value string) http.SameSite {
	switch strings.ToLower(value) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (a *App) clientIP(r *http.Request) string {
	if a.cfg.TrustProxyHeaders {
		for _, header := range []string{"CF-Connecting-IP", "X-Real-IP"} {
			if value := parseClientIPHeader(r.Header.Get(header)); value != "" {
				return value
			}
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := parseClientIPHeader(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func parseClientIPHeader(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func urlPathEscape(value string) string {
	return url.PathEscape(value)
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

func decodeMap(r *http.Request) map[string]any {
	var payload map[string]any
	if r.Body == nil {
		return map[string]any{}
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func stringValue(payload map[string]any, key string) string {
	if v, ok := payload[key]; ok {
		switch typed := v.(type) {
		case string:
			return strings.TrimSpace(typed)
		case float64:
			return strconv.FormatInt(int64(typed), 10)
		case bool:
			return strconv.FormatBool(typed)
		}
	}
	return ""
}

func intValue(payload map[string]any, key string, fallback int) int {
	if v, ok := payload[key]; ok {
		switch typed := v.(type) {
		case float64:
			return int(typed)
		case string:
			if i, err := strconv.Atoi(typed); err == nil {
				return i
			}
		}
	}
	return fallback
}

func boolValue(payload map[string]any, key string, fallback bool) bool {
	if v, ok := payload[key]; ok {
		switch typed := v.(type) {
		case bool:
			return typed
		case string:
			return typed == "1" || strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes")
		}
	}
	return fallback
}

func int64Param(params Params, key string) (int64, error) {
	return strconv.ParseInt(params[key], 10, 64)
}

func publicUser(u store.User) map[string]any {
	return map[string]any{
		"uid":                     u.UID,
		"username":                u.Username,
		"email":                   u.Email,
		"telegram_id":             nullableInt(u.TelegramID),
		"telegram_username":       u.TelegramUsername,
		"role":                    u.Role,
		"role_name":               roleName(u.Role),
		"active":                  u.Active,
		"expire_status":           expireStatus(u.ExpiredAt),
		"expired_at":              u.ExpiredAt,
		"emby_id":                 u.EmbyID,
		"emby_username":           u.EmbyUsername,
		"emby_bound":              u.EmbyID != "",
		"avatar":                  u.Avatar,
		"background":              u.Background,
		"bgm_mode":                u.BGMMode,
		"bgm_token_set":           u.BGMToken != "",
		"bgm_sync_ready":          u.BGMMode && u.BGMToken != "",
		"created_at":              u.CreatedAt,
		"register_time":           u.RegisterTime,
		"is_pending":              u.Role == store.RoleUnrecognized,
		"pending_emby":            u.PendingEmby,
		"pending_emby_days":       u.PendingEmbyDays,
		"emby_disabled_by_expiry": false,
		"library_self_service":    u.LibrarySelfService,
	}
}

func nullableInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func roleName(role int) string {
	switch role {
	case store.RoleAdmin:
		return "管理员"
	case store.RoleWhitelist:
		return "白名单"
	case store.RoleUnrecognized:
		return "未注册"
	default:
		return "普通用户"
	}
}

func expireStatus(expiredAt int64) string {
	if expiredAt < 0 {
		return "永不过期"
	}
	if expiredAt == 0 {
		return "未设置"
	}
	now := time.Now().Unix()
	if expiredAt < now {
		return "已过期"
	}
	days := int((expiredAt - now) / 86400)
	return fmt.Sprintf("剩余 %d天", days)
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func maskAPIKey(key string) (prefix, suffix, masked string) {
	if len(key) <= 16 {
		return key, "", key
	}
	prefix = key[:12]
	suffix = key[len(key)-6:]
	return prefix, suffix, prefix + "..." + suffix
}

func statusFromError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "资源不存在")
		return true
	}
	if errors.Is(err, store.ErrConflict) {
		fail(w, http.StatusConflict, "资源已存在")
		return true
	}
	if errors.Is(err, store.ErrExpired) {
		fail(w, http.StatusBadRequest, "资源已过期")
		return true
	}
	fail(w, http.StatusInternalServerError, "操作失败")
	return true
}
