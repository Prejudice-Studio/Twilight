package api

import (
	"bufio"
	"context"
	"crypto/sha256"
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
	"sync/atomic"
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
	Parts   []string
	Auth    AuthLevel
	Handler HandlerFunc
}

type App struct {
	// runtime 持有 cfg / store / sessions / limiter / redis 这五个 hot reload
	// 期间会被整体替换的运行时句柄。读端必须经 a.cfg() / a.store() 等访问器，
	// 一次性 Load 出 *runtimeState 后再读字段；reload 端构造完整的 next 状态后
	// 一次 Store 完成原子切换，避免：
	//   - cfg（multi-word struct）字段半新半旧的撕裂读；
	//   - store / sessions / limiter / redis 接口/指针的 (type,data) 双 word
	//     非原子赋值导致 vtable 与 data 撕裂触发 segfault；
	//   - reload 中途读端拿到 cfg 是 next、store 仍是 prev 的混合视图。
	runtime               atomic.Pointer[runtimeState]
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
	bindStatus            *bindStatusHub
	// schedulerLocks: jobID -> *schedulerProcessRun。BATCH_07 之前在 package 级
	// 声明 (`var schedulerProcessLocks sync.Map`)，单进程 prod 不显问题，但
	// 测试 setup 反复 New() 出多个 App 时这张表共享 → 一个 case cancel 的 job
	// 让另一 case 的 LoadOrStore 误判为 already running，flake 出现。收回到
	// instance 字段后每个 App 自带独立锁表。零值即可使用。
	schedulerLocks sync.Map
}

// runtimeState 把 reload 期间会一并替换的运行时句柄打包成一个不可变快照，
// 走 atomic.Pointer 整体切换。任何一个字段都不能在 store 之后被修改——读端
// 拿到的快照必须自洽（cfg / store / sessions / limiter / redis 来自同一次
// reload）。
type runtimeState struct {
	cfg      config.Config
	store    *store.Store
	sessions *sessionStore
	limiter  *rateLimiter
	redis    *redis.Client
}

// cfg 返回当前生效的 config.Config 指针：调用方读字段后即可释放快照；
// 同一次 handler 内若需要前后一致，先 `cfg := a.cfg()` 一次再多次读字段。
// 返回指针而非值避免在 hot path 上每次拷贝 ~50 字段的 Config struct。
func (a *App) cfg() *config.Config {
	if rt := a.runtime.Load(); rt != nil {
		return &rt.cfg
	}
	// New() 之后 runtime 必然非 nil；这里仅作 nil 防御。
	var empty config.Config
	return &empty
}

func (a *App) store() *store.Store {
	if rt := a.runtime.Load(); rt != nil {
		return rt.store
	}
	return nil
}

func (a *App) sessions() *sessionStore {
	if rt := a.runtime.Load(); rt != nil {
		return rt.sessions
	}
	return nil
}

func (a *App) limiter() *rateLimiter {
	if rt := a.runtime.Load(); rt != nil {
		return rt.limiter
	}
	return nil
}

func (a *App) redis() *redis.Client {
	if rt := a.runtime.Load(); rt != nil {
		return rt.redis
	}
	return nil
}

// runtimeSnapshot 返回当前快照本身，便于持锁段或一次性需要多个字段的代码
// 一次 Load 后多次读取，避免重复走 atomic.Load。
func (a *App) runtimeSnapshot() *runtimeState {
	return a.runtime.Load()
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

func (w *statusResponseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

type contextKey string

const principalKey contextKey = "principal"

const (
	twilightClientHeader         = "X-Twilight-Client"
	twilightIntentHeader         = "X-Twilight-Intent"
	twilightIntentCreateBindCode = "create-bind-code"
)

func New(cfg config.Config, st *store.Store) (*App, error) {
	redisClient, err := newRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	app := &App{
		telegramPanels: map[string]telegramPanelContext{},
		embyAdminCache: map[string]embyAdminCacheEntry{},
		bindStatus:     newBindStatusHub(),
	}
	app.runtime.Store(&runtimeState{
		cfg:      cfg,
		store:    st,
		sessions: newSessionStoreWithDB(cfg.SessionTTL, redisClient, st),
		limiter:  newRateLimiter(redisClient),
		redis:    redisClient,
	})
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
	prevState := a.runtimeSnapshot()
	if prevState == nil {
		return nil, fmt.Errorf("runtime not initialized")
	}
	previous := prevState.cfg
	next, err := config.NewReader(previous.ConfigFile).Read()
	if err != nil {
		return nil, err
	}

	// 在快照副本上累积所有变更：开新 store / 新 redis / 新 sessions 等都先
	// 写 nextState，最后一次 atomic.Store 把整组 hot 字段一并切换。读端走
	// 访问器只能看到旧快照或新快照两种自洽视图，不会出现"cfg 已是 next、
	// store 还是 prev"的撕裂态。
	nextState := *prevState
	nextState.cfg = next

	reinitialized := []string{}
	closeOldStore := false
	if storeBackendChanged(previous, next) {
		// Backend / 路径 / DSN 真的变了：先开新 store，确认成功后再关旧 store。
		// 不能"先关后开"——那会留下 store==nil 窗口，并发 ServeHTTP 走到
		// store().IsIPBlacklisted / store().User 等点会直接 nil panic；
		// 失败时也无法干净回滚（旧 store 已 Close，再 Open 同一 JSON 路径
		// 会与还在挥发的 flock 竞争）。
		// storeBackendChanged 已经把"同 path JSON"过滤掉（normalizeStoreDriver
		// 把 "" / "json" / "file" 归一），所以这里不会两个 JSON store 抢同一个
		// 锁文件——对 PG → JSON、JSON 路径切换、PG DSN 切换都安全。
		nextStore, err := openConfiguredStore(context.Background(), next)
		if err != nil {
			return nil, err
		}
		nextState.store = nextStore
		closeOldStore = prevState.store != nil
		reinitialized = append(reinitialized, "database")
	}
	closeOldRedis := false
	if previous.RedisURL != next.RedisURL || previous.SessionTTL != next.SessionTTL {
		redisClient, err := newRedisClient(next)
		if err != nil {
			return nil, err
		}
		nextState.sessions = newSessionStoreWithDB(next.SessionTTL, redisClient, nextState.store)
		nextState.limiter = newRateLimiter(redisClient)
		nextState.redis = redisClient
		closeOldRedis = prevState.redis != nil && prevState.redis != redisClient
		reinitialized = append(reinitialized, "sessions", "rate_limiter")
	} else if nextState.store != prevState.store {
		// store 换了但 redis 没换：sessions 内部持有 store 引用，需要重新
		// 绑定到新 store，否则会话回写还是落到旧 backend 上。
		nextState.sessions = newSessionStoreWithDB(next.SessionTTL, nextState.redis, nextState.store)
	}

	if nextState.store.Backend() == store.BackendPostgres && (previous.PostgresMaxOpenConns != next.PostgresMaxOpenConns || previous.PostgresMaxIdleConns != next.PostgresMaxIdleConns) {
		nextState.store.ConfigurePostgres(next.PostgresMaxOpenConns, next.PostgresMaxIdleConns)
		reinitialized = append(reinitialized, "postgres_pool")
	}

	// 原子切换：在此之前任何 return 都不会污染当前运行时；之后读端就是新状态。
	a.runtime.Store(&nextState)

	// applyConfiguredAdmins 读 cfg / 写 store，必须在 atomic Store 之后调用，
	// 否则它内部的 a.cfg() / a.store() 还会看到旧快照。
	a.applyConfiguredAdmins()

	if closeOldStore {
		_ = prevState.store.Close()
	}
	if closeOldRedis {
		_ = prevState.redis.Close()
	}

	// reload 也走一遍 CORS 校验，捕获 hot reload 引入的错配置。
	if !sameStringSlice(previous.CORSOrigins, next.CORSOrigins) {
		validateCORSOriginsStartup(next.CORSOrigins, next.AllowCredential)
	}
	if previous.TrustProxyHeaders != next.TrustProxyHeaders ||
		!sameStringSlice(previous.TrustedProxyCIDRs, next.TrustedProxyCIDRs) {
		validateTrustedProxyStartup(next.TrustProxyHeaders, next.TrustedProxyCIDRs)
	}
	ConfigureRuntimeLoggingStore(nextState.store, next.ZapLevel(), next.RuntimeLogLimit)
	reinitialized = append(reinitialized, "runtime_logger")

	restartRequired := []string{}
	if previous.Host != next.Host || previous.Port != next.Port {
		restartRequired = append(restartRequired, "listen_addr")
	}
	// liveApplied 列出"本次 reload 后立即对运行中的 handler 生效"的字段。
	// 这些字段都不缓存到任何长寿命对象上：要么走 a.cfg() 在每次请求时重读
	// （SessionCookie / CookieSecure / CookieSameSite / RegisterEnabled /
	// 各类 RateLimit*），要么作为 reload 自身重建链路的输入（DatabaseDriver /
	// RedisURL / SessionTTL / Postgres pool / ZapLevel）。
	//
	// 这一段的目的不是改变行为——之前 cookie 三件套就是被实时读的——而是
	// 把"哪些字段实际生效 / 哪些不生效"做成可观察的响应字段，避免运营手改
	// 配置后只看到 200 而不知道是不是真的下发了。restartRequired 仍然只覆盖
	// listen_addr：那是唯一一个 reload 无法替换的整体监听器。
	liveApplied := liveAppliedConfigFields(previous, next)
	a.configSignature = configFileSignature(next.ConfigFile)

	info := map[string]any{
		"reloaded":            true,
		"config_file":         next.ConfigFile,
		"reinitialized":       reinitialized,
		"restart_required":    restartRequired,
		"live_applied":        liveApplied,
		"active_database":     nextState.store.Backend(),
		"configured_database": next.DatabaseDriver,
		"runtime_restarted":   len(reinitialized) > 0,
	}
	zap.L().Info("config hot reloaded", zap.String("config_file", next.ConfigFile), zap.String("reinitialized", strings.Join(reinitialized, ",")), zap.String("restart_required", strings.Join(restartRequired, ",")), zap.String("live_applied", strings.Join(liveApplied, ",")))
	return info, nil
}

// liveAppliedConfigFields 返回本次 reload 中"实际值发生变化且立即对运行
// 时生效"的字段名（首字母 lower / TOML key 风格）。给出这一列表是为了让
// /admin/config/reload 的响应能区分三种状态：
//   - reinitialized: 引发重建（store / sessions / limiter / postgres / logger）
//   - live_applied: 字段值变化但走 a.cfg() 实时重读，无需重建对象
//   - restart_required: 字段变化需要进程重启（目前只有 listen_addr）
//
// 字段没出现 = 字段没变。空列表 = reload 仅触发过 reinit，没有更细粒度的
// 即时变更。这一函数是纯读，不写任何运行时状态。
func liveAppliedConfigFields(previous, next config.Config) []string {
	out := []string{}
	add := func(name string) { out = append(out, name) }
	if previous.SessionCookie != next.SessionCookie {
		add("session_cookie_name")
	}
	if previous.CookieSecure != next.CookieSecure {
		add("session_cookie_secure")
	}
	if !strings.EqualFold(previous.CookieSameSite, next.CookieSameSite) {
		add("session_cookie_samesite")
	}
	if previous.CookieDomain != next.CookieDomain {
		add("session_cookie_domain")
	}
	if previous.RegisterEnabled != next.RegisterEnabled {
		add("register_enabled")
	}
	if previous.AllowPendingRegister != next.AllowPendingRegister {
		add("allow_pending_register")
	}
	if previous.MediaRequestEnabled != next.MediaRequestEnabled {
		add("media_request_enabled")
	}
	if previous.SigninEnabled != next.SigninEnabled {
		add("signin_enabled")
	}
	if previous.InviteEnabled != next.InviteEnabled {
		add("invite_enabled")
	}
	if previous.RateLimitEnabled != next.RateLimitEnabled {
		add("rate_limit_enabled")
	}
	if previous.RateLimitGlobalPerMinute != next.RateLimitGlobalPerMinute {
		add("rate_limit_global_per_minute")
	}
	if previous.RateLimitAPIKeyDefaultPerMinute != next.RateLimitAPIKeyDefaultPerMinute {
		add("rate_limit_apikey_default_per_minute")
	}
	if !sameStringSlice(previous.CORSOrigins, next.CORSOrigins) {
		add("cors_origins")
	}
	if previous.AllowCredential != next.AllowCredential {
		add("allow_credential")
	}
	if previous.TrustProxyHeaders != next.TrustProxyHeaders {
		add("trust_proxy_headers")
	}
	if !sameStringSlice(previous.TrustedProxyCIDRs, next.TrustedProxyCIDRs) {
		add("trusted_proxy_cidrs")
	}
	if previous.MaxUploadSize != next.MaxUploadSize {
		add("max_upload_size")
	}
	return out
}

func (a *App) reloadConfigIfChanged() {
	current := configFileSignature(a.cfg().ConfigFile)
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
	// 与 scheduler_daemon.go / telegram_bot.go 对齐：日志值统一走
	// redactSensitiveText 而非 zap.Any 反射 dump。当前 info 仅包含元数据
	// （reloaded / reinitialized 等），但任何未来加进来的字段（如 emby_url
	// / database_url 摘要）一旦上线就会沿用同一条日志路径，提前把敏感
	// 字段拦在 redact 这一步比逐 PR 审查每个新字段稳妥。
	zap.L().Info("config file change applied", zap.String("reload", redactSensitiveText(fmt.Sprintf("%+v", info))))
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
	if a.cfg().MaxUploadSize > 0 {
		r.Body = http.MaxBytesReader(lw, r.Body, a.cfg().MaxUploadSize)
	}
	if a.store().IsIPBlacklisted(a.clientIP(r)) {
		failWithCode(lw, http.StatusForbidden, ErrIPBlacklisted, "IP 已被封禁")
		return
	}

	if !a.allowRate(r.Context(), rateKey("global:", a.clientIP(r)), a.cfg().RateLimitGlobalPerMinute, time.Minute) {
		failWithCode(lw, http.StatusTooManyRequests, ErrGlobalRateLimited, "请求过于频繁，请稍后再试")
		return
	}

	route, params, methodAllowed := a.match(r.Method, r.URL.Path)
	if route == nil {
		if methodAllowed {
			failWithCode(lw, http.StatusMethodNotAllowed, ErrRouteMethodNotAllow, "请求方法不允许")
		} else {
			failWithCode(lw, http.StatusNotFound, ErrRouteNotFound, "接口不存在")
		}
		return
	}

	principal, ok := a.authenticate(lw, r, route.Auth)
	if !ok {
		return
	}
	principalLog = principal
	if principal != nil && a.blockRestrictedEmbyAdmin(lw, r, route, principal.User) {
		return
	}
	if principal != nil {
		r = r.WithContext(context.WithValue(r.Context(), principalKey, *principal))
	}
	route.Handler(lw, r, params)
}

func (a *App) allowRate(ctx context.Context, key string, limit int, window time.Duration) bool {
	if !a.cfg().RateLimitEnabled || limit <= 0 {
		return true
	}
	return a.limiter().Allow(ctx, key, limit, window)
}
func (a *App) add(method, pattern string, auth AuthLevel, handler HandlerFunc) {
	a.routes = append(a.routes, Route{Method: method, Pattern: pattern, Parts: splitPath(pattern), Auth: auth, Handler: handler})
}

func (a *App) match(method, requestPath string) (*Route, Params, bool) {
	methodAllowed := false
	requestParts := splitPath(requestPath)
	for i := range a.routes {
		route := &a.routes[i]
		params, ok := matchPattern(route.Parts, requestParts)
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

func matchPattern(patternParts, requestParts []string) (Params, bool) {
	if len(patternParts) != len(requestParts) {
		return nil, false
	}
	params := Params{}
	for i := range patternParts {
		if strings.HasPrefix(patternParts[i], ":") {
			params[strings.TrimPrefix(patternParts[i], ":")] = requestParts[i]
			continue
		}
		if patternParts[i] != requestParts[i] {
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
		// 与 handleLogin / handleLoginByAPIKey 同口径：session 仍然挂在 user 上，
		// 但 admin 把账号 Active=false（手动禁用 vs check_expired 触发的到期），
		// 路径上拿到不同 ErrCode 才能让 webui 把"续费"和"申诉"两条 CTA 分开。
		if userExpiredOnly(p.User) {
			failWithCode(w, http.StatusForbidden, ErrAccountExpired, "账号有效期已到期，请续费后再继续操作")
			return nil, false
		}
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
		if cookie, err := r.Cookie(a.cfg().SessionCookie); err == nil {
			token = cookie.Value
			fromCookie = true
		}
	}
	uid, ok := a.sessions().Get(r.Context(), token)
	if !ok {
		return nil, false
	}
	u, ok := a.store().User(uid)
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
	ak, u, ok := a.store().FindAPIKeyByHash(hash)
	if !ok || !u.Active {
		return nil, false
	}
	if fromQuery && (ak.ID == 0 || !ak.AllowQuery) {
		return nil, false
	}
	limit := ak.RateLimit
	if limit <= 0 {
		limit = a.cfg().RateLimitAPIKeyDefaultPerMinute
	}
	if !a.allowRate(r.Context(), rateKey("apikey:", hash), limit, time.Minute) {
		return nil, false
	}
	if ak.ID > 0 {
		_ = a.store().RecordAPIKeyUse(ak.ID)
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
	if !a.corsOriginAllowed(origin) {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	// 仅在 allow_credential 开启时下发凭据头；否则运维显式关闭凭据共享的意图
	// 必须被尊重（此前无条件下发，使该开关形同虚设，列入白名单的低信任 origin
	// 仍可携带 cookie 读取已登录用户的鉴权响应）。origin 反射始终受白名单约束。
	if a.cfg().AllowCredential {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Twilight-Client, X-Twilight-Intent")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
	w.Header().Set("Access-Control-Max-Age", "600")
	return true
}

func (a *App) corsOriginAllowed(origin string) bool {
	origin = normalizeCORSOrigin(origin)
	if origin == "" || origin == "*" {
		return false
	}
	for _, candidate := range a.cfg().CORSOrigins {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			// 启动期已 zap.Error 告警；运行期再次防御性跳过，
			// 即使管理员错配置成 `*` 也不会与 Allow-Credentials 组合。
			continue
		}
		if strings.EqualFold(normalizeCORSOrigin(candidate), origin) {
			return true
		}
	}
	return false
}

func requireWebUIIntent(w http.ResponseWriter, r *http.Request, intent string) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get(twilightClientHeader)), "webui") ||
		!strings.EqualFold(strings.TrimSpace(r.Header.Get(twilightIntentHeader)), intent) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "缺少显式前端操作意图")
		return false
	}
	if isPrefetchRequest(r) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "预取请求不能创建绑定码")
		return false
	}
	return true
}

func isPrefetchRequest(r *http.Request) bool {
	for _, name := range []string{"Purpose", "Sec-Purpose", "X-Moz"} {
		if strings.Contains(strings.ToLower(r.Header.Get(name)), "prefetch") {
			return true
		}
	}
	return false
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
		Name:     a.cfg().SessionCookie,
		Value:    token,
		Path:     "/",
		Domain:   a.cfg().CookieDomain,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   a.cfg().CookieSecure,
		SameSite: sameSite(a.cfg().CookieSameSite),
	}
	http.SetCookie(w, cookie)
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	// 清除时 Domain 必须与 setSessionCookie 完全一致，否则浏览器会按
	// "default-domain (= 设置时的请求 host)" 寻找另一份同名 cookie，登出
	// 留下幽灵 cookie 的概率极高——这正是双子域部署里常见的"登出后再访
	// 问还是登录态"现象。
	http.SetCookie(w, &http.Cookie{Name: a.cfg().SessionCookie, Path: "/", Domain: a.cfg().CookieDomain, MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: true, Secure: a.cfg().CookieSecure, SameSite: sameSite(a.cfg().CookieSameSite)})
}

func (a *App) issueSessionCookies(w http.ResponseWriter, sessionToken string, expires time.Time) {
	a.setSessionCookie(w, sessionToken, expires)
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
	if a.cfg().TrustProxyHeaders && upstreamIsTrustedProxy(r.RemoteAddr, a.cfg().TrustedProxyCIDRs) {
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
				if upstreamIsTrustedProxy(ipStr, a.cfg().TrustedProxyCIDRs) {
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

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	// 与 decodeMap 一致地施加 256KB 上限。MaxUploadSize（默认 5MB）面向上传场景，
	// JSON body 不应当吃到那个量级；任何接近 256KB 的请求基本是滥用 / 探测，
	// 提早 EOF 即可。
	body := http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

// maxJSONBodyBytes 是 decodeMap / decodeJSON 默认接受的 JSON 上限，避免攻击者
// 提交超大 payload 在 map[string]any 解码阶段吃光内存。256KB 远高于业务实际
// 用到的 payload（绑定 / 注册 / 列表查询都在数 KB 量级），但又足以挡住明显
// 异常的批量提交。需要更大的接口（如导入 / 上传）应单独走 MaxUploadSize 路径
// 与显式的 schema，不再过 decodeMap。
const maxJSONBodyBytes = 256 * 1024

// maxJSONNestingDepth 限制 decodeMap 接受的最大嵌套层级。Go 的 encoding/json
// 没有原生 depth guard，恶意构造的 [[[[…]]]] 在解码阶段并不会立刻报错，但会
// 让后续基于 reflect / type assertion 的遍历放大到栈爆炸的程度。32 层对真实
// 业务（最多嵌套对象/数组几层）远绰绰有余。
const maxJSONNestingDepth = 32

func decodeMap(r *http.Request) map[string]any {
	payload := map[string]any{}
	if r.Body == nil {
		return payload
	}
	body := http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return map[string]any{}
	}
	if jsonDepthExceeds(payload, maxJSONNestingDepth) {
		return map[string]any{}
	}
	return payload
}

// jsonDepthExceeds 在解码后做后置检查，发现深度超过 limit 立即返回 true。
// 按 map / slice 递归即可；其它叶子值（string / float64 / bool / nil）算 0 层。
func jsonDepthExceeds(value any, limit int) bool {
	return jsonDepth(value, 0, limit)
}

func jsonDepth(value any, current, limit int) bool {
	if current > limit {
		return true
	}
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			if jsonDepth(child, current+1, limit) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if jsonDepth(child, current+1, limit) {
				return true
			}
		}
	}
	return false
}

func stringValue(payload map[string]any, key string) string {
	if v, ok := payload[key]; ok {
		switch typed := v.(type) {
		case string:
			return strings.TrimSpace(typed)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
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

// userNotFoundMessage 是所有 ErrUserNotFound 响应的统一文案。
//
// 历史上 handlers.go / upload_handlers.go 用英文 "user not found"，
// admin_extra.go / library_handlers.go 用 "用户不存在"，invite_handlers.go
// 用 "目标用户不存在"——同一个语义在 9 个 call site 里漂移成 3 种 copy，
// 前端按 message 分支时永远落不了一致的 fallback 文案。统一成中文 "用户不存在"
// 以与系统其他错误响应一致；前端 validators.ts 已经映射 USER_NOT_FOUND → "用户不存在"，
// 这只是把后端 message 也对齐过来。
const userNotFoundMessage = "用户不存在"

// userFromPath 抽掉"按 :uid path-param 加载用户，不存在则 404"的
// 重复模板。原先 handlers.go / upload_handlers.go / admin_extra.go /
// library_handlers.go 各处共有 9 个完全一样的 4 行片段：
//
//	uid, _ := int64Param(params, "uid")
//	u, ok := a.store().User(uid)
//	if !ok { failWithCode(w, 404, ErrUserNotFound, "user not found"); return }
//
// 抽到这里之后调用方变成 `u, ok := a.userFromPath(w, params, "uid"); if !ok { return }`，
// 同时把所有响应统一到 ErrUserNotFound + userNotFoundMessage，消除 R64-7 提到
// 的"同一个 ErrUserNotFound 同义词散落在三种 copy"的问题。返回值用 ok 模式
// 而不是 error，是因为 handler 在 not-found 时已经写好响应直接 return，
// 调用方没有继续处理 error 的场景；强制返回 error 反倒诱使调用方再写一遍判空。
func (a *App) userFromPath(w http.ResponseWriter, params Params, key string) (store.User, bool) {
	uid, _ := int64Param(params, key)
	u, okUser := a.store().User(uid)
	if !okUser {
		failWithCode(w, http.StatusNotFound, ErrUserNotFound, userNotFoundMessage)
		return store.User{}, false
	}
	return u, true
}

func publicUser(u store.User) map[string]any {
	return map[string]any{
		"uid":                      u.UID,
		"username":                 u.Username,
		"email":                    u.Email,
		"telegram_id":              nullableInt(u.TelegramID),
		"telegram_username":        u.TelegramUsername,
		"role":                     u.Role,
		"role_name":                roleName(u.Role),
		"active":                   u.Active,
		"expire_status":            expireStatus(u.ExpiredAt),
		"expired_at":               publicExpiryUnix(u.ExpiredAt),
		"emby_id":                  u.EmbyID,
		"emby_username":            u.EmbyUsername,
		"emby_bound":               u.EmbyID != "",
		"avatar":                   u.Avatar,
		"background":               u.Background,
		"bgm_mode":                 u.BGMMode,
		"bgm_token_set":            u.BGMToken != "",
		"bgm_sync_ready":           u.BGMMode && u.BGMToken != "",
		"created_at":               u.CreatedAt,
		"register_time":            u.RegisterTime,
		"emby_grant_locked":        u.EmbyGrantLocked,
		"is_pending":               u.Role == store.RoleUnrecognized,
		"pending_emby":             u.PendingEmby,
		"pending_emby_days":        u.PendingEmbyDays,
		"registration_source":      u.RegistrationSource,
		"registration_source_name": registrationSourceLabel(u.RegistrationSource),
		"registration_code":        u.RegistrationCode,
		"emby_disabled_by_expiry":  false,
		"rebinding_in_progress":    u.RebindingInProgress,
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
	if expiryIsPermanent(expiredAt) {
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

// requireAdmin 是 admin-only handler 的入口断言。AuthAdmin 路由已在
// dispatcher（app.go:611）处把非 admin 阻挡在外，但 handler 自身再确认一次
// caller 角色，避免：
//
//  1. 路由表手抖把 admin handler 挂到 AuthUser；
//  2. 同一 handler 同时挂在 AuthUser / AuthAdmin 两条路由（典型如
//     handleLoginHistory / handleWatchStats / handleDevices），handler 内部
//     靠 path string 推断鉴权时漏判；
//  3. 未来引入 ApiKey / TG webhook 等新鉴权来源时绕过 AuthAdmin。
//
// 写为 true 时表示已写 403，调用方直接 return 即可。
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if current(r).User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "权限不足")
		return true
	}
	return false
}

// requireAdminForUIDParam 用于"AuthUser / AuthAdmin 共用 handler，路径上有
// :uid 时必须是 admin"的场景：在 :uid 存在但 caller 不是 admin 的情况下回
// 403。返回 (target uid, written 403)；调用方在 written 为 true 时直接
// return。target uid 解析失败则回退到 caller 自身（保留现状），但若 :uid 是
// 字符串且 parse 失败应改为 400（R44-7 单独跟进）。
func requireAdminForUIDParam(w http.ResponseWriter, r *http.Request, params Params) (int64, bool) {
	caller := current(r).User
	if params["uid"] == "" {
		return caller.UID, false
	}
	if caller.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrUserProtected, "权限不足")
		return 0, true
	}
	paramUID, err := int64Param(params, "uid")
	if err != nil || paramUID <= 0 {
		return caller.UID, false
	}
	return paramUID, false
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
	// ErrLastAdmin 之前由各 handler 单独 errors.Is 分支判，漏一处就直接降级
	// 到下面的 ErrInternal/500，前端拿到泛化错误码无法 routing 到"最后一个
	// 管理员"提示。集中映射后所有调用 statusFromError 的路径自动获得正确的
	// 409 + ErrAdminLastAdminProtected。
	if errors.Is(err, store.ErrLastAdmin) {
		failWithCode(w, http.StatusConflict, ErrAdminLastAdminProtected, "无法移除最后一个管理员的权限，系统至少需要一个管理员")
		return true
	}
	failWithCode(w, http.StatusInternalServerError, ErrInternal, "操作失败")
	return true
}
