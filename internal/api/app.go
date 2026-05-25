package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	configSignature       string
	telegramBotMu         sync.Mutex
	telegramBotCacheToken string
	telegramBotCacheUntil time.Time
	telegramBotCache      map[string]any
	telegramStatusMu      sync.Mutex
	telegramLastOKAt      int64
	telegramLastErrorAt   int64
	telegramLastError     string
	telegramPolling       bool
	telegramPanelMu       sync.Mutex
	telegramPanels        map[string]telegramPanelContext
	embyAdminMu           sync.Mutex
	embyAdminCache        map[string]embyAdminCacheEntry
}

type embyAdminCacheEntry struct {
	admin   bool
	checked time.Time
}

type principal struct {
	User       store.User
	APIKey     store.APIKey
	Token      string
	FromCookie bool
}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

type contextKey string

const principalKey contextKey = "principal"

func New(cfg config.Config, st *store.Store) (*App, error) {
	redisClient, err := newRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	app := &App{
		cfg:            cfg,
		store:          st,
		sessions:       newSessionStoreWithDB(cfg.SessionTTL, redisClient, st),
		limiter:        newRateLimiter(redisClient),
		redis:          redisClient,
		telegramPanels: map[string]telegramPanelContext{},
		embyAdminCache: map[string]embyAdminCacheEntry{},
	}
	app.configSignature = configFileSignature(cfg.ConfigFile)
	app.registerRoutes()
	app.applyConfiguredAdmins()
	// 启动期校验 CORS 配置一致性。
	// applyCORS 默认就附带 Allow-Credentials: true，浏览器规范本就拒绝
	// `*` + credentials 组合；早期管理员若误填 `*` 会得到一个静默无效的配置。
	// 这里在启动 / reload 时强提示，配 .Error 让审计/告警系统能抓到。
	validateCORSOriginsStartup(cfg.CORSOrigins, cfg.AllowCredential)
	validateTrustedProxyStartup(cfg.TrustProxyHeaders, cfg.TrustedProxyCIDRs)
	ConfigureRuntimeLoggingStore(st, cfg.ZapLevel(), cfg.RuntimeLogLimit)
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
			zap.L().Warn("redis configured but unavailable; using memory fallback", zap.Error(err))
			redisClient = nil
		} else {
			cancel()
			zap.L().Info("redis enabled for sessions and rate limits")
		}
	}
	return redisClient, nil
}

func openConfiguredStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	switch cfg.DatabaseDriver {
	case "", store.BackendJSON, "file":
		return store.Open(cfg.StateFile)
	case store.BackendPostgres, "postgresql":
		dsn := cfg.PostgresDSN()
		if dsn == "" {
			return nil, fmt.Errorf("database driver is postgres but no PostgreSQL URL or host/user/database is configured")
		}
		openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		st, err := store.OpenPostgres(openCtx, dsn)
		if err != nil {
			return nil, err
		}
		st.ConfigurePostgres(cfg.PostgresMaxOpenConns, cfg.PostgresMaxIdleConns)
		return st, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
}

// storeBackendChanged 判定 reload 是否需要重开 store。Driver / state file
// 路径 / Postgres DSN 任一变化就必须重开；driver 含义以归一化为准（"" / "json"
// / "file" 视为同一 JSON 后端，避免 reload 时把同一路径的 JSON store 反复
// 关闭重开造成 flock 重入死锁）。
func storeBackendChanged(previous, next config.Config) bool {
	prevDriver := normalizeStoreDriver(previous.DatabaseDriver)
	nextDriver := normalizeStoreDriver(next.DatabaseDriver)
	if prevDriver != nextDriver {
		return true
	}
	switch nextDriver {
	case store.BackendJSON:
		return previous.StateFile != next.StateFile
	case store.BackendPostgres:
		return previous.PostgresDSN() != next.PostgresDSN()
	}
	return false
}

func normalizeStoreDriver(driver string) string {
	switch driver {
	case "", store.BackendJSON, "file":
		return store.BackendJSON
	case store.BackendPostgres, "postgresql":
		return store.BackendPostgres
	}
	return driver
}

func (a *App) Routes() []Route {
	out := make([]Route, len(a.routes))
	copy(out, a.routes)
	return out
}

func (a *App) reloadConfig() (map[string]any, error) {
	a.runtimeMu.Lock()
	defer a.runtimeMu.Unlock()
	return a.reloadConfigLocked()
}

func (a *App) reloadConfigLocked() (map[string]any, error) {
	previous := a.cfg
	next, err := config.NewReader(previous.ConfigFile).Read()
	if err != nil {
		return nil, err
	}

	reinitialized := []string{}
	if storeBackendChanged(previous, next) {
		// Backend / 路径 / DSN 真的变了：先开新 store，确认成功后再关旧 store。
		// 不能"先关后开"——那会留下 a.store==nil 窗口，并发 ServeHTTP 走到
		// `a.store.IsIPBlacklisted` / `a.store.User` 等点会直接 nil panic；
		// 失败时也无法干净回滚（旧 store 已 Close，再 Open 同一 JSON 路径
		// 会与还在挥发的 flock 竞争）。
		// storeBackendChanged 已经把"同 path JSON"过滤掉（normalizeStoreDriver
		// 把 "" / "json" / "file" 归一），所以这里不会两个 JSON store 抢同一个
		// 锁文件——对 PG → JSON、JSON 路径切换、PG DSN 切换都安全。
		nextStore, err := openConfiguredStore(context.Background(), next)
		if err != nil {
			return nil, err
		}
		oldStore := a.store
		a.store = nextStore
		if oldStore != nil {
			_ = oldStore.Close()
		}
		reinitialized = append(reinitialized, "database")
	}
	if previous.RedisURL != next.RedisURL || previous.SessionTTL != next.SessionTTL {
		redisClient, err := newRedisClient(next)
		if err != nil {
			return nil, err
		}
		oldRedis := a.redis
		a.sessions = newSessionStoreWithDB(next.SessionTTL, redisClient, a.store)
		a.limiter = newRateLimiter(redisClient)
		a.redis = redisClient
		if oldRedis != nil && oldRedis != redisClient {
			_ = oldRedis.Close()
		}
		reinitialized = append(reinitialized, "sessions", "rate_limiter")
	}

	a.cfg = next
	a.applyConfiguredAdmins()
	// reload 也走一遍 CORS 校验，捕获 hot reload 引入的错配置。
	if !sameStringSlice(previous.CORSOrigins, next.CORSOrigins) {
		validateCORSOriginsStartup(next.CORSOrigins, next.AllowCredential)
	}
	if previous.TrustProxyHeaders != next.TrustProxyHeaders ||
		!sameStringSlice(previous.TrustedProxyCIDRs, next.TrustedProxyCIDRs) {
		validateTrustedProxyStartup(next.TrustProxyHeaders, next.TrustedProxyCIDRs)
	}
	ConfigureRuntimeLoggingStore(a.store, next.ZapLevel(), next.RuntimeLogLimit)
	reinitialized = append(reinitialized, "runtime_logger")
	if a.store.Backend() == store.BackendPostgres && (previous.PostgresMaxOpenConns != next.PostgresMaxOpenConns || previous.PostgresMaxIdleConns != next.PostgresMaxIdleConns) {
		a.store.ConfigurePostgres(next.PostgresMaxOpenConns, next.PostgresMaxIdleConns)
		reinitialized = append(reinitialized, "postgres_pool")
	}
	restartRequired := []string{}
	if previous.Host != next.Host || previous.Port != next.Port {
		restartRequired = append(restartRequired, "listen_addr")
	}
	a.configSignature = configFileSignature(next.ConfigFile)

	info := map[string]any{
		"reloaded":            true,
		"config_file":         next.ConfigFile,
		"reinitialized":       reinitialized,
		"restart_required":    restartRequired,
		"active_database":     a.store.Backend(),
		"configured_database": next.DatabaseDriver,
		"runtime_restarted":   len(reinitialized) > 0,
	}
	zap.L().Info("config hot reloaded", zap.String("config_file", next.ConfigFile), zap.String("reinitialized", strings.Join(reinitialized, ",")), zap.String("restart_required", strings.Join(restartRequired, ",")))
	return info, nil
}

func (a *App) reloadConfigIfChanged() {
	current := configFileSignature(a.cfg.ConfigFile)
	a.runtimeMu.Lock()
	if current == "" || current == a.configSignature {
		a.runtimeMu.Unlock()
		return
	}
	info, err := a.reloadConfigLocked()
	if err != nil {
		// 失败也要把 configSignature 推到当前值。否则只要 admin 写错一次配置，
		// 每个 ServeHTTP 都会再走一次 reloadConfigLocked → config.Read →
		// 同样的解析错误，QPS 直接归零。等 admin 真的再改一次文件、
		// signature 再次变化，自然会触发下一轮 reload 重试。
		a.configSignature = current
		a.runtimeMu.Unlock()
		zap.L().Warn("config hot reload check failed", zap.Error(err))
		return
	}
	a.runtimeMu.Unlock()
	zap.L().Info("config file change applied", zap.Any("reload", info))
}

func configFileSignature(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "config.toml"
	}
	paths := []string{path}
	local := os.Getenv("TWILIGHT_CONFIG_LOCAL_FILE")
	if local == "" {
		local = strings.TrimSuffix(path, filepath.Ext(path)) + ".local" + filepath.Ext(path)
	}
	paths = append(paths, local)
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				parts = append(parts, p+":missing")
			} else {
				parts = append(parts, p+":error")
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", p, info.Size(), info.ModTime().UnixNano()))
	}
	return strings.Join(parts, "|")
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	lw := &statusResponseWriter{ResponseWriter: w}
	var principalLog *principal
	defer func() {
		if recovered := recover(); recovered != nil {
			// panic 值可能携带请求字段（密码 / token / Emby 凭据等），
			// zap.Any 会原样落盘绕过日志脱敏；先 fmt + redact 再写。
			panicMsg := redactSensitiveText(fmt.Sprintf("%v", recovered))
			zap.L().Error("panic in api handler",
				zap.String("panic", panicMsg),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
			)
			// envelope 必须带 error_code，前端才能用 errcode 而非文案匹配。
			// status==0 表示尚未写入响应头，避免 panic 后再写一次造成 superfluous WriteHeader。
			if lw.status == 0 {
				failWithCode(lw, http.StatusInternalServerError, ErrInternal, "服务器内部错误")
			}
		}
		status := lw.status
		if status == 0 {
			status = http.StatusOK
		}
		fields := []zap.Field{
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", status),
			zap.Int("bytes", lw.bytes),
			zap.Int64("duration_ms", time.Since(started).Milliseconds()),
			zap.String("ip", a.clientIP(r)),
		}
		if principalLog != nil {
			fields = append(fields, zap.Int64("uid", principalLog.User.UID), zap.String("username", principalLog.User.Username))
		}
		switch {
		case status >= 500:
			zap.L().Error("http request completed", fields...)
		case status >= 400:
			zap.L().Warn("http request completed", fields...)
		default:
			zap.L().Info("http request completed", fields...)
		}
	}()
	a.reloadConfigIfChanged()
	a.applySecurityHeaders(lw)
	if a.applyCORS(lw, r) && r.Method == http.MethodOptions {
		lw.WriteHeader(http.StatusNoContent)
		return
	}
	if a.cfg.MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(lw, r.Body, a.cfg.MaxUploadSize)
	}
	if a.store.IsIPBlacklisted(a.clientIP(r)) {
		fail(lw, http.StatusForbidden, "IP 已被封禁")
		return
	}

	if !a.allowRate(r.Context(), rateKey("global:", a.clientIP(r)), a.cfg.RateLimitGlobalPerMinute, time.Minute) {
		fail(lw, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}

	route, params, methodAllowed := a.match(r.Method, r.URL.Path)
	if route == nil {
		if methodAllowed {
			fail(lw, http.StatusMethodNotAllowed, "请求方法不允许")
		} else {
			fail(lw, http.StatusNotFound, "接口不存在")
		}
		return
	}

	principal, ok := a.authenticate(lw, r, route.Auth)
	if !ok {
		return
	}
	principalLog = principal
	if principal != nil && principal.FromCookie && isMutating(r.Method) {
		if !a.verifyCSRFToken(r) {
			failWithCode(lw, http.StatusForbidden, ErrCSRFMissing, "缺少或无效的 CSRF 令牌，请刷新页面后重试")
			return
		}
	}
	if principal != nil && a.blockRestrictedEmbyAdmin(lw, r, route, principal.User) {
		return
	}
	if principal != nil {
		r = r.WithContext(context.WithValue(r.Context(), principalKey, *principal))
	}
	route.Handler(lw, r, params)
}

func (a *App) allowRate(ctx context.Context, key string, limit int, window time.Duration) bool {
	if !a.cfg.RateLimitEnabled || limit <= 0 {
		return true
	}
	return a.limiter.Allow(ctx, key, limit, window)
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
			failWithCode(w, http.StatusUnauthorized, ErrAPIKeyInvalid, "API Key 无效")
			return nil, false
		}
		return p, true
	}
	p, ok := a.authenticateUser(r)
	if !ok {
		failWithCode(w, http.StatusUnauthorized, ErrUnauthorized, "登录状态已失效，请重新登录")
		return nil, false
	}
	if !p.User.Active {
		failWithCode(w, http.StatusForbidden, ErrAccountDisabled, "账号已被禁用")
		return nil, false
	}
	if auth == AuthAdmin && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "需要管理员权限")
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
	// Active 兜底：用户被 ban / scheduler 自动失活 / 用户主动注销后，stale token
	// 必须立即被拒。redis 缓存的 sessionRecord 不含 active 字段，所以不能依赖
	// 缓存层做这件事；store.User 才是 active 的唯一真相源。
	// 与 authenticateAPIKey:495 的 `!u.Active` 检查口径一致。
	if !u.Active {
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
		limit = a.cfg.RateLimitAPIKeyDefaultPerMinute
	}
	if !a.allowRate(r.Context(), rateKey("apikey:", hash), limit, time.Minute) {
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
	// 后端只输出 JSON / 静态上传资源，不渲染 HTML 应用本身：
	// 限制 default-src 'none'，避免 application/json 错误识别后被注入脚本。
	// 文档/图片仍可加载（img-src + style-src 'self'）。
	// 前端 Next.js 的 CSP 由 webui 自身在 next.config.ts / middleware 中负责。
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; "+
			"script-src 'self'; connect-src 'self'; font-src 'self' data:; "+
			"frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-site")
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
			// 启动期已 zap.Error 告警；运行期再次防御性跳过，
			// 即使管理员错配置成 `*` 也不会与 Allow-Credentials 组合。
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
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Twilight-Client, X-CSRF-Token")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	w.Header().Set("Access-Control-Max-Age", "600")
	return true
}

// validateCORSOriginsStartup 在启动 / reload 时对 CORS 配置做静态体检：
//   - 显式 `*` 与 Allow-Credentials: true 是浏览器规范禁止的组合，
//     applyCORS 已运行期跳过 `*`，但管理员看不到任何反馈，
//     因此这里强制 zap.Error 让监控/SRE 抓到错配置；
//   - 同时把无法 normalize 的 origin（拼写错误 / 含路径 / 含 query）
//     列出来，给运维一个可定位的失败信号。
//
// 函数本身不返回 error，仅打印 —— App 不会因为 CORS 错配置启动失败，
// 因为反代/容器重启场景下这往往是非致命的退化。
func validateCORSOriginsStartup(origins []string, allowCredential bool) {
	if len(origins) == 0 {
		return
	}
	var hasWildcard bool
	var hasLocalhost bool
	var invalid []string
	cleaned := make([]string, 0, len(origins))
	for _, raw := range origins {
		o := strings.TrimSpace(raw)
		if o == "" {
			continue
		}
		if o == "*" {
			hasWildcard = true
			continue
		}
		if normalizeCORSOrigin(o) == "" {
			invalid = append(invalid, o)
			continue
		}
		lower := strings.ToLower(o)
		if strings.Contains(lower, "://localhost") || strings.Contains(lower, "://127.0.0.1") || strings.Contains(lower, "://[::1]") {
			hasLocalhost = true
		}
		cleaned = append(cleaned, o)
	}
	if hasWildcard {
		zap.L().Error(
			"cors_origins 包含 `*` 通配符；运行期会被忽略并禁用 Allow-Credentials："+
				"浏览器规范禁止 `*` 与 credentials 同用，请改填具体 origin 列表",
			zap.Strings("configured_origins", origins),
			zap.Strings("effective_origins", cleaned),
		)
	}
	if len(invalid) > 0 {
		zap.L().Warn(
			"cors_origins 含无法解析的条目，已忽略；条目必须是 scheme://host[:port]，无 path/query/fragment",
			zap.Strings("invalid_entries", invalid),
		)
	}
	// AllowCredential=true 同时放行 localhost / 127.0.0.1 / [::1] 在生产环境是高危：
	// 任何用户在本机起 dev 前端就能携带 cookie 跨站访问。dev 场景应通过显式 dev profile
	// 而不是默认放行。
	if allowCredential && hasLocalhost {
		zap.L().Error(
			"cors_origins 中包含 localhost/127.0.0.1，且 allow_credential=true。"+
				"生产部署请移除本地回环 origin，或将 allow_credential 设为 false。",
			zap.Strings("configured_origins", origins),
		)
	}
}

// validateTrustedProxyStartup 在启动 / reload 时核验"信任代理头 + 上游 CIDR"
// 这对配置：
//   - TrustProxyHeaders=true 但 TrustedProxyCIDRs 为空：打 Error 提醒——
//     upstreamIsTrustedProxy 在 CIDRs 为空时直接返回 false（fail-closed），
//     意味着 trust_proxy_headers=true 此时实际无效，所有 IP 限流 / 黑名单
//     都基于 TCP 对端地址。运维必须补全 CIDR 才能生效。
//   - 单条 CIDR 解析失败：打 Warn 列出出错条目，调用方仍然按剩余可用条目工作；
//   - TrustProxyHeaders=false 且 CIDRs 非空：打 Info 提示 CIDRs 当前不会生效，
//     避免运维误以为已经启用。
func validateTrustedProxyStartup(trust bool, cidrs []string) {
	if !trust {
		if len(cidrs) > 0 {
			zap.L().Info(
				"trusted_proxy_cidrs 已配置但 trust_proxy_headers=false；当前不会消费任何代理头",
				zap.Strings("trusted_proxy_cidrs", cidrs),
			)
		}
		return
	}
	var invalid []string
	var valid int
	for _, raw := range cidrs {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if net.ParseIP(entry) == nil {
				invalid = append(invalid, entry)
				continue
			}
			valid++
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err != nil {
			invalid = append(invalid, entry)
			continue
		}
		valid++
	}
	if valid == 0 {
		zap.L().Error(
			"trust_proxy_headers=true 但 trusted_proxy_cidrs 未配置任何有效条目；"+
				"代理头将被忽略（fail-closed），所有 IP 维度策略基于 TCP 对端地址。"+
				"如需消费 X-Forwarded-For / X-Real-IP / CF-Connecting-IP，请补 "+
				"API.trusted_proxy_cidrs（CIDR 或单 IP，逗号分隔）",
			zap.Strings("configured_cidrs", cidrs),
		)
	}
	if len(invalid) > 0 {
		zap.L().Warn(
			"trusted_proxy_cidrs 含无法解析的条目，已忽略；条目必须是 IP 或 CIDR (a.b.c.d/N)",
			zap.Strings("invalid_entries", invalid),
		)
	}
}

// sameStringSlice 判断两个 string 切片在元素 + 顺序意义上完全相同。
// reload 时仅当 CORS 配置实际变化才重跑 validateCORSOriginsStartup，
// 避免每次 hot reload 都刷一条 Info 日志。
func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
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
	http.SetCookie(w, &http.Cookie{Name: a.csrfCookieName(), Path: "/", MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: false, Secure: a.cfg.CookieSecure, SameSite: sameSite(a.cfg.CookieSameSite)})
}

// csrfCookieName 返回 CSRF cookie 名，固定为 session cookie 名 + "_csrf" 后缀。
func (a *App) csrfCookieName() string {
	return a.cfg.SessionCookie + "_csrf"
}

// newCSRFToken 生成 32 字节随机 CSRF 令牌（hex 编码）。
// 在 crypto/rand 故障时返回 error；调用方需向上层失败响应而非伪造熵。
func newCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// setCSRFCookie 写一个非 HttpOnly 的 CSRF cookie，前端 JS 可读后塞进
// X-CSRF-Token 请求头。配合 verifyCSRFToken 形成"双提交 cookie"模式：
// 攻击者站点不能跨域读 cookie 也无法伪造同名 header，所以无法构造合法 mutating 请求。
func (a *App) setCSRFCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.csrfCookieName(),
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: false, // CRITICAL：必须可被前端 JS 读取
		Secure:   a.cfg.CookieSecure,
		SameSite: sameSite(a.cfg.CookieSameSite),
	})
}

// issueSessionCookies 一次性下发 session + csrf 两个 cookie，并返回
// CSRF 令牌字符串方便登录响应也回写到 body（前端可任选 cookie / body 来源）。
func (a *App) issueSessionCookies(w http.ResponseWriter, sessionToken string, expires time.Time) (string, error) {
	csrf, err := newCSRFToken()
	if err != nil {
		return "", err
	}
	a.setSessionCookie(w, sessionToken, expires)
	a.setCSRFCookie(w, csrf, expires)
	return csrf, nil
}

// verifyCSRFToken 校验 cookie-based mutating 请求携带合法 CSRF 令牌。
// 校验规则：
//  1. 必须有 csrf cookie
//  2. 必须有 X-CSRF-Token header
//  3. 两者使用 subtle.ConstantTimeCompare 比对
//
// 任一失败返回 false。调用点应回 403 + AUTH_CSRF_MISSING。
func (a *App) verifyCSRFToken(r *http.Request) bool {
	cookie, err := r.Cookie(a.csrfCookieName())
	if err != nil || cookie.Value == "" {
		return false
	}
	header := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if header == "" {
		return false
	}
	if len(header) != len(cookie.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) == 1
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
	if a.cfg.TrustProxyHeaders && upstreamIsTrustedProxy(r.RemoteAddr, a.cfg.TrustedProxyCIDRs) {
		for _, header := range []string{"CF-Connecting-IP", "X-Real-IP"} {
			if value := parseClientIPHeader(r.Header.Get(header)); value != "" {
				return value
			}
		}
		// XFF 右向左剥离：直接对端是受信代理，但代理本身可能再被另一层
		// 受信代理转发；攻击者控制的客户端在最左端塞任何字符串。我们从
		// 最右一跳开始，逐跳验证是否仍处于 TrustedProxyCIDRs 范围内，
		// 第一个不在范围内的 IP 才是真实客户端 IP。
		// 例：XFF = "spoofed, 1.1.1.1, 10.0.0.5"，RemoteAddr=10.0.0.1，
		//   trusted = [10.0.0.0/24]，则结果应为 1.1.1.1（spoofed 不可信）。
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ipStr := parseClientIPHeader(parts[i])
				if ipStr == "" {
					continue
				}
				// 最左侧（i==0）始终视为客户端原始 IP，直接返回。
				if i == 0 {
					return ipStr
				}
				if upstreamIsTrustedProxy(ipStr, a.cfg.TrustedProxyCIDRs) {
					continue
				}
				return ipStr
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// upstreamIsTrustedProxy 判断 r.RemoteAddr 的对端是否落在 cfg.TrustedProxyCIDRs
// 列表里。返回 true 时，clientIP 才会消费 X-Forwarded-For / X-Real-IP / CF-* 这
// 些可被任意调用方伪造的代理头；否则一律走 RemoteAddr。
//
// 安全策略：cidrs 为空 ⇒ 返回 false（fail-closed）。
// 配置不完整时直接走 RemoteAddr，由启动期日志引导运维补全 TrustedProxyCIDRs，
// 避免任何人通过伪造 X-Forwarded-For 绕过 IP 限速 / 黑名单。
func upstreamIsTrustedProxy(remoteAddr string, cidrs []string) bool {
	if len(cidrs) == 0 {
		return false
	}
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, raw := range cidrs {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		// 允许直接写单个 IP（不带 /N）：等价于 /32 或 /128。
		if !strings.Contains(entry, "/") {
			if peer := net.ParseIP(entry); peer != nil && peer.Equal(ip) {
				return true
			}
			continue
		}
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
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
	return fmt.Sprintf("剩余 %d 天", days)
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
		failWithCode(w, http.StatusNotFound, ErrNotFound, "资源不存在")
		return true
	}
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrConflict, "资源已存在")
		return true
	}
	if errors.Is(err, store.ErrExpired) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "资源已过期")
		return true
	}
	failWithCode(w, http.StatusInternalServerError, ErrInternal, "操作失败")
	return true
}
