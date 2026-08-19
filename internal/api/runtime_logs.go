package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type RuntimeLogEntry = store.RuntimeLogEntry

type runtimeLogBuffer struct {
	mu      sync.Mutex
	cond    *sync.Cond
	nextID  int64
	entries []RuntimeLogEntry
	limit   int
}

var (
	runtimeStartedAt = time.Now()
	runtimeLogs      = newRuntimeLogSink(1000)
	// runtimeConsoleLevel 控制控制台（stdout）输出级别，跟随配置 log_level（默认 info），
	// 避免默认配置下 Debug 噪声刷屏。
	runtimeConsoleLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	// runtimeSinkLevel 控制持久化 sink（运行日志面板 + store）的捕获级别，恒定为 Debug：
	// 全局所有级别日志都要落进运行日志面板供事后排查，与 console 输出级别解耦。
	// zap core 的 LevelEnabler 用它（最宽松），保证 Debug 条目能通过 Check 进入 Write；
	// console 写出在 Write 内用 runtimeConsoleLevel 二次门控。
	runtimeSinkLevel = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	// 在 token / authorization 通用关键字之外，
	// 显式列出 Emby / MediaBrowser / Session 等高频敏感字段变体。
	// 子串匹配本来已能兜住 emby_token 这类（包含 "token"），但显式枚举：
	//   1) 让审计 / grep 一眼可见保护范围；
	//   2) 杜绝未来如果通用关键字被收窄时悄悄漏过；
	//   3) 与 sensitiveLogKey 的 normalized 子串列表保持一字段一映射。
	sensitivePattern = regexp.MustCompile(`(?i)(authorization|cookie|session[_-]?(?:id|token)|x[_-]?emby[_-]?(?:token|authorization)|emby[_-]?(?:token|authorization)|media[_-]?browser[_-]?token|access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|private[_-]?key|connection[_-]?string|database[_-]?url|token|secret|password|passwd|api[_-]?key|x[_-]?api[_-]?key|bot[_-]?token|dsn)\s*[:=]\s*[^ \t\r\n,;]+`)
	// 引号包围的 key="value" 变体（Emby Authorization 头的标准格式：
	//   MediaBrowser Client="Twilight", Token="xxx", DeviceId="..."
	// 普通 sensitivePattern 的值末尾用 [^ \t\r\n,;]+ 截断，但 quoted value
	// 内若含逗号 / 空格反而会被截掉一部分；另起一支 quoted regex 兜底。
	// 必须与 sensitivePattern 列表保持镜像，新增条目两边都要补。
	sensitiveQuotedPattern = regexp.MustCompile(`(?i)(authorization|cookie|session[_-]?(?:id|token)|x[_-]?emby[_-]?(?:token|authorization)|emby[_-]?(?:token|authorization)|media[_-]?browser[_-]?token|access[_-]?token|refresh[_-]?token|id[_-]?token|client[_-]?secret|private[_-]?key|connection[_-]?string|database[_-]?url|token|secret|password|passwd|api[_-]?key|x[_-]?api[_-]?key|bot[_-]?token|dsn)\s*=\s*"[^"]*"`)
	bearerPattern          = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`)
	keyPattern             = regexp.MustCompile(`key-[A-Za-z0-9._~+/=-]{12,}`)
)

// sensitiveLogKeyReplacer 把敏感字段名的 `_` / `-` / `.` 剥掉后做小写匹配，
// 供 sensitiveLogKey 使用。原实现在每次调用时新建 Replacer（字符串切片装箱），
// 现提升为包级变量。注意：sensitiveLogKey 历史上不剥空格，与
// auditSensitiveDetailKey（剥空格）口径不同，故两处各自保留独立 Replacer。
var sensitiveLogKeyReplacer = strings.NewReplacer("_", "", "-", "", ".", "")

type runtimeLogSink struct {
	mu       sync.RWMutex
	cond     *sync.Cond
	st       *store.Store
	fallback *runtimeLogBuffer
	limit    int
}

func newRuntimeLogSink(limit int) *runtimeLogSink {
	s := &runtimeLogSink{fallback: newRuntimeLogBuffer(limit), limit: clamp(limit, 100, 50000)}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *runtimeLogSink) configure(st *store.Store, limit int) {
	if limit <= 0 {
		limit = s.currentLimit()
	}
	limit = clamp(limit, 100, 50000)
	s.mu.Lock()
	if st != nil {
		s.st = st
	}
	s.limit = limit
	s.fallback.setLimit(limit)
	s.cond.Broadcast()
	s.mu.Unlock()
	if st != nil {
		for _, entry := range s.fallback.drain() {
			entry.ID = 0
			_, _ = st.AddRuntimeLog(entry, limit)
		}
		_ = st.PruneRuntimeLogs(limit)
	}
}

func (s *runtimeLogSink) currentLimit() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.limit <= 0 {
		return 1000
	}
	return s.limit
}

func (s *runtimeLogSink) append(entry RuntimeLogEntry) {
	s.mu.RLock()
	st := s.st
	limit := s.limit
	s.mu.RUnlock()
	if st != nil {
		if _, err := st.AddRuntimeLog(entry, limit); err == nil {
			s.mu.Lock()
			s.cond.Broadcast()
			s.mu.Unlock()
			return
		}
	}
	s.fallback.append(entry)
	s.mu.Lock()
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *runtimeLogSink) setLimit(limit int) {
	s.configure(nil, limit)
}

func (s *runtimeLogSink) stats() (int, int) {
	s.mu.RLock()
	st := s.st
	limit := s.limit
	s.mu.RUnlock()
	if st != nil {
		_, entries := st.RuntimeLogStats()
		return limit, entries
	}
	fallbackLimit, entries := s.fallback.stats()
	return fallbackLimit, entries
}

func (s *runtimeLogSink) snapshot(limit int, after int64) ([]RuntimeLogEntry, int64) {
	s.mu.RLock()
	st := s.st
	maxLimit := s.limit
	s.mu.RUnlock()
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	if st != nil {
		return st.RuntimeLogs(limit, after)
	}
	return s.fallback.snapshot(limit, after)
}

func (s *runtimeLogSink) waitAfter(ctx context.Context, after int64, limit int) ([]RuntimeLogEntry, int64, bool) {
	deadline := time.NewTimer(25 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		entries, next := s.snapshot(limit, after)
		if len(entries) > 0 {
			return entries, next, true
		}
		select {
		case <-ctx.Done():
			return nil, after, false
		case <-deadline.C:
			return nil, after, true
		case <-ticker.C:
		}
	}
}

func newRuntimeLogBuffer(limit int) *runtimeLogBuffer {
	b := &runtimeLogBuffer{limit: limit}
	b.cond = sync.NewCond(&b.mu)
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (b *runtimeLogBuffer) append(entry RuntimeLogEntry) {
	b.mu.Lock()
	b.nextID++
	entry.ID = b.nextID
	if entry.Time == 0 {
		entry.Time = time.Now().Unix()
	}
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.limit {
		b.entries = trimRuntimeLogBufferEntries(b.entries, b.limit)
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *runtimeLogBuffer) setLimit(limit int) {
	limit = clamp(limit, 100, 50000)
	b.mu.Lock()
	b.limit = limit
	if len(b.entries) > b.limit {
		b.entries = trimRuntimeLogBufferEntries(b.entries, b.limit)
	}
	b.mu.Unlock()
}

func trimRuntimeLogBufferEntries(entries []RuntimeLogEntry, limit int) []RuntimeLogEntry {
	if limit <= 0 {
		clear(entries)
		return nil
	}
	if len(entries) <= limit {
		return entries
	}
	start := len(entries) - limit
	if cap(entries) > limit*2 {
		out := make([]RuntimeLogEntry, limit)
		copy(out, entries[start:])
		clear(entries)
		return out
	}
	copy(entries, entries[start:])
	clear(entries[limit:])
	return entries[:limit]
}

func (b *runtimeLogBuffer) stats() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit, len(b.entries)
}

func (b *runtimeLogBuffer) snapshot(limit int, after int64) ([]RuntimeLogEntry, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 || limit > b.limit {
		limit = b.limit
	}
	next := b.nextID
	start, end := runtimeLogBufferWindow(b.entries, limit, after)
	if start == end {
		return nil, next
	}
	filtered := b.entries[start:end]
	next = filtered[len(filtered)-1].ID
	out := make([]RuntimeLogEntry, len(filtered))
	copy(out, filtered)
	return out, next
}

func runtimeLogBufferWindow(entries []RuntimeLogEntry, limit int, after int64) (int, int) {
	if len(entries) == 0 || limit <= 0 {
		return 0, 0
	}
	if limit > len(entries) {
		limit = len(entries)
	}
	if after <= 0 {
		start := len(entries) - limit
		if start < 0 {
			start = 0
		}
		return start, len(entries)
	}
	start := sort.Search(len(entries), func(i int) bool {
		return entries[i].ID > after
	})
	if start >= len(entries) {
		return len(entries), len(entries)
	}
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	return start, end
}

func (b *runtimeLogBuffer) drain() []RuntimeLogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]RuntimeLogEntry, len(b.entries))
	copy(out, b.entries)
	b.entries = nil
	return out
}

func (b *runtimeLogBuffer) waitAfter(ctx context.Context, after int64, limit int) ([]RuntimeLogEntry, int64, bool) {
	deadline := time.NewTimer(25 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer deadline.Stop()
	defer ticker.Stop()
	for {
		entries, next := b.snapshot(limit, after)
		if len(entries) > 0 {
			return entries, next, true
		}
		select {
		case <-ctx.Done():
			return nil, after, false
		case <-deadline.C:
			return nil, after, true
		case <-ticker.C:
		}
	}
}

type runtimeLogCore struct {
	zapcore.Core
	fields []zapcore.Field
}

func InstallRuntimeLogger(w io.Writer, level zapcore.Level) {
	if w == nil {
		w = io.Discard
	}
	runtimeConsoleLevel.SetLevel(level)
	// std log 桥接级别取 Info 与 console 级别的较大值：std log 本身没有级别语义，
	// 用 console 级别决定它以何种 zap 级别注入，仍会被 sink 全量捕获。
	stdLogLevel := zapcore.InfoLevel
	if level > stdLogLevel {
		stdLogLevel = level
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	// 底层 console core 的 LevelEnabler 用 runtimeSinkLevel（Debug，最宽松），
	// 保证所有级别条目都能通过 runtimeLogCore.Check 进入 Write；
	// 真正的 console 输出裁剪在 Write 内按 runtimeConsoleLevel 二次门控。
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), zapcore.AddSync(w), runtimeSinkLevel)
	logger := zap.New(&runtimeLogCore{Core: core}, zap.AddCaller(), zap.ErrorOutput(zapcore.AddSync(w)))
	zap.ReplaceGlobals(logger)
	zap.RedirectStdLogAt(logger, stdLogLevel)
}

func ConfigureRuntimeLogging(level zapcore.Level, limit int) {
	// 只调 console 输出级别；sink 恒定 Debug 全量捕获，不随 log_level 变化。
	runtimeConsoleLevel.SetLevel(level)
	if limit > 0 {
		runtimeLogs.configure(nil, limit)
	}
}

func ConfigureRuntimeLoggingStore(st *store.Store, level zapcore.Level, limit int) {
	// 只调 console 输出级别；sink 恒定 Debug 全量捕获，不随 log_level 变化。
	runtimeConsoleLevel.SetLevel(level)
	runtimeLogs.configure(st, limit)
}

func (c *runtimeLogCore) With(fields []zapcore.Field) zapcore.Core {
	nextFields := append([]zapcore.Field{}, c.fields...)
	nextFields = append(nextFields, cloneZapFields(fields)...)
	return &runtimeLogCore{Core: c.Core.With(sanitizeZapFields(fields)), fields: nextFields}
}

func (c *runtimeLogCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *runtimeLogCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	allFields := append(cloneZapFields(c.fields), cloneZapFields(fields)...)
	attrs := zapFieldsToAttrs(allFields)
	runtimeLogs.append(RuntimeLogEntry{
		Time:    entry.Time.Unix(),
		Level:   strings.ToUpper(entry.Level.String()),
		Message: redactSensitiveText(entry.Message),
		Attrs:   attrs,
	})
	// console 输出按 runtimeConsoleLevel 二次门控：sink 已全量捕获上面的 append，
	// 但低于 console 级别的条目不再写 stdout，保持默认配置下终端不被 Debug 刷屏。
	if !runtimeConsoleLevel.Enabled(entry.Level) {
		return nil
	}
	sanitized := zapcore.Entry{
		Level:      entry.Level,
		Time:       entry.Time,
		LoggerName: entry.LoggerName,
		Message:    redactSensitiveText(entry.Message),
		Caller:     entry.Caller,
		Stack:      redactSensitiveText(entry.Stack),
	}
	return c.Core.Write(sanitized, sanitizeZapFields(fields))
}

func cloneZapFields(fields []zapcore.Field) []zapcore.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zapcore.Field, len(fields))
	copy(out, fields)
	return out
}

func sanitizeZapFields(fields []zapcore.Field) []zapcore.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		out = append(out, sanitizeZapField(field))
	}
	return out
}

func sanitizeZapField(field zapcore.Field) zapcore.Field {
	if sensitiveLogKey(field.Key) {
		return zap.String(field.Key, "[REDACTED]")
	}
	if field.Type == zapcore.StringType {
		field.String = redactSensitiveText(field.String)
		return field
	}
	if field.Type == zapcore.ErrorType {
		if err, ok := field.Interface.(error); ok && err != nil {
			return zap.String(field.Key, redactSensitiveText(err.Error()))
		}
	}
	// ReflectType / 任意未识别的复合类型：强制走 fmt.Sprint -> redact -> zap.String，
	// 否则 zap encoder 会在底层用反射输出原 interface（panic value、map、struct 等
	// 会原样展开），绕过敏感字段脱敏。
	if field.Type == zapcore.ReflectType {
		return zap.String(field.Key, redactSensitiveText(zapFieldValueString(field)))
	}
	text := zapFieldValueString(field)
	if redacted := redactSensitiveText(text); redacted != text {
		return zap.String(field.Key, redacted)
	}
	return field
}

func zapFieldsToAttrs(fields []zapcore.Field) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	attrs := map[string]string{}
	for _, field := range fields {
		if sensitiveLogKey(field.Key) {
			attrs[field.Key] = "[REDACTED]"
			continue
		}
		attrs[field.Key] = redactSensitiveText(zapFieldValueString(field))
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func zapFieldValueString(field zapcore.Field) string {
	switch field.Type {
	case zapcore.StringType:
		return field.String
	case zapcore.ErrorType:
		if err, ok := field.Interface.(error); ok && err != nil {
			return err.Error()
		}
	case zapcore.BoolType:
		return strconv.FormatBool(field.Integer == 1)
	case zapcore.Int8Type, zapcore.Int16Type, zapcore.Int32Type, zapcore.Int64Type:
		return strconv.FormatInt(field.Integer, 10)
	case zapcore.Uint8Type, zapcore.Uint16Type, zapcore.Uint32Type, zapcore.Uint64Type, zapcore.UintptrType:
		return strconv.FormatUint(uint64(field.Integer), 10)
	case zapcore.Float32Type:
		return strconv.FormatFloat(float64(math.Float32frombits(uint32(field.Integer))), 'f', -1, 32)
	case zapcore.Float64Type:
		return strconv.FormatFloat(math.Float64frombits(uint64(field.Integer)), 'f', -1, 64)
	case zapcore.DurationType:
		return time.Duration(field.Integer).String()
	case zapcore.TimeType:
		return time.Unix(0, field.Integer).UTC().Format(time.RFC3339Nano)
	}
	if field.Interface != nil {
		return fmt.Sprint(field.Interface)
	}
	if field.String != "" {
		return field.String
	}
	return strconv.FormatInt(field.Integer, 10)
}

func sensitiveLogKey(key string) bool {
	normalized := sensitiveLogKeyReplacer.Replace(strings.ToLower(key))
	return normalized == "key" ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "bottoken") ||
		strings.Contains(normalized, "embytoken") || // 显式 Emby 变体
		strings.Contains(normalized, "embyauthorization") || // 显式 Emby 变体
		strings.Contains(normalized, "mediabrowsertoken") || // MediaBrowser
		strings.Contains(normalized, "sessionid") || // 会话标识
		strings.Contains(normalized, "privatekey") || // PEM / SSH 私钥
		strings.Contains(normalized, "connectionstring") || // 连接串
		strings.Contains(normalized, "databaseurl") || // PG / MySQL URL
		strings.Contains(normalized, "dsn")
}

// redactSensitiveTriggers 是四条脱敏正则各自匹配所必需的字面量子串（小写）。
// 任一正则要命中，其命中片段里必然包含本列表中的某个词——因此若文本（小写化后）
// 一个都不含，四次 ReplaceAllString 必然原样返回，可整段跳过。
// 该列表只做「更宽」的近似：多列几个只是让快路径少走，绝不会漏（漏才会泄密）。
var redactSensitiveTriggers = []string{
	"bearer", "key", "authorization", "cookie", "session",
	"token", "secret", "password", "passwd",
	"connection", "database", "dsn", "emby", "mediabrowser",
}

// mightContainSecret 是脱敏正则的廉价前置过滤：绝大多数日志字段（uid / path /
// duration 等）不含任何敏感触发词，直接短路，避免每条日志每个字符串字段都跑四遍
// NFA 正则 + 全量分配。返回 true 只表示「可能命中」，最终仍由正则判定。
func mightContainSecret(value string) bool {
	lower := strings.ToLower(value)
	for _, trigger := range redactSensitiveTriggers {
		if strings.Contains(lower, trigger) {
			return true
		}
	}
	return false
}

func redactSensitiveText(value string) string {
	if value == "" {
		return value
	}
	// 快路径：不含任何触发词 ⇒ 四条正则都不可能命中，原样返回、零分配。
	if !mightContainSecret(value) {
		return value
	}
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = keyPattern.ReplaceAllString(value, "key-[REDACTED]")
	// 先匹配 quoted variant（保留 key 前缀 + 用 "[REDACTED]" 覆盖整段值）
	value = sensitiveQuotedPattern.ReplaceAllString(value, `$1="[REDACTED]"`)
	value = sensitivePattern.ReplaceAllString(value, "$1=[REDACTED]")
	return value
}

func (a *App) handleRuntimeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	logLimit, logEntries := runtimeLogs.stats()
	status := map[string]any{
		"started_at":              runtimeStartedAt.Unix(),
		"uptime_seconds":          int64(time.Since(runtimeStartedAt).Seconds()),
		"go_version":              runtime.Version(),
		"goos":                    runtime.GOOS,
		"goarch":                  runtime.GOARCH,
		"goroutines":              runtime.NumGoroutine(),
		"cpu_count":               runtime.NumCPU(),
		"redis_enabled":           a.redis() != nil,
		"routes":                  len(a.routes),
		"active_database":         a.store().Backend(),
		"config_database":         strings.ToLower(a.cfg().DatabaseDriver),
		"storage_mismatch":        a.runtimeDatabaseMismatch(),
		"storage_warning":         a.databaseMismatchWarning(),
		"users":                   a.store().UserCount(),
		"log_level":               a.cfg().LogLevel,
		"runtime_log_limit":       logLimit,
		"runtime_log_entries":     logEntries,
		"runtime_log_backend":     a.store().Backend(),
		"runtime_memory_limit_mb": a.cfg().RuntimeMemoryLimitMB,
		"memory": map[string]any{
			"alloc":           mem.Alloc,
			"sys":             mem.Sys,
			"heap_alloc":      mem.HeapAlloc,
			"heap_sys":        mem.HeapSys,
			"heap_inuse":      mem.HeapInuse,
			"stack_inuse":     mem.StackInuse,
			"next_gc":         mem.NextGC,
			"num_gc":          mem.NumGC,
			"go_memory_limit": runtimeMemoryLimitBytes(),
		},
	}
	// sink 恒定 Debug 全量捕获，与 console log_level 解耦；明示给运维状态面板。
	status["runtime_log_capture"] = strings.ToLower(runtimeSinkLevel.Level().String())
	if host := safeHostname(); host != "" {
		status["hostname"] = host
	}
	if load := readLinuxLoadAverage(); len(load) > 0 {
		status["load_average"] = load
	}
	if memInfo := readLinuxMemInfo(); len(memInfo) > 0 {
		status["host_memory"] = memInfo
	}
	if uptime := readLinuxUptime(); uptime > 0 {
		status["host_uptime_seconds"] = uptime
	}
	ok(w, "OK", status)
}

func (a *App) handleRuntimeLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	maxLimit, _ := runtimeLogs.stats()
	limit := clamp(queryInt(r, "limit", 200), 1, maxLimit)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	entries, next := runtimeLogs.snapshot(limit, after)
	ok(w, "OK", runtimeLogPayload(entries, next, limit))
}

func runtimeLogPayload(entries []RuntimeLogEntry, next int64, limit int) map[string]any {
	return map[string]any{"entries": entries, "next_cursor": next, "limit": limit}
}

func runtimeLogEventPayload(entries []RuntimeLogEntry, next int64) map[string]any {
	return map[string]any{"entries": entries, "next_cursor": next}
}

func (a *App) handleRuntimeLogStream(w http.ResponseWriter, r *http.Request, _ Params) {
	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		failWithCode(w, http.StatusInternalServerError, ErrRuntimeLogStreamUnsupported, "当前响应不支持实时日志")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	maxLimit, _ := runtimeLogs.stats()
	limit := clamp(queryInt(r, "limit", 100), 1, minInt(maxLimit, 1000))
	cursor, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	send := func(event string, data any) bool {
		payload, err := json.Marshal(data)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	entries, next := runtimeLogs.snapshot(limit, cursor)
	if !send("snapshot", runtimeLogEventPayload(entries, next)) {
		return
	}
	cursor = next
	for {
		entries, next, okWait := runtimeLogs.waitAfter(r.Context(), cursor, limit)
		if !okWait {
			return
		}
		if len(entries) == 0 {
			if !send("ping", map[string]any{"time": time.Now().Unix(), "next_cursor": cursor}) {
				return
			}
			continue
		}
		cursor = next
		if !send("logs", runtimeLogEventPayload(entries, next)) {
			return
		}
	}
}

func safeHostname() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return redactSensitiveText(host)
}

func readLinuxLoadAverage() []float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	parts := strings.Fields(string(data))
	out := make([]float64, 0, 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		value, err := strconv.ParseFloat(parts[i], 64)
		if err == nil {
			out = append(out, value)
		}
	}
	return out
}

func readLinuxUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	return int64(value)
}

func readLinuxMemInfo() map[string]uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}
	keys := map[string]string{
		"MemTotal:":     "total_kb",
		"MemAvailable:": "available_kb",
		"MemFree:":      "free_kb",
		"Buffers:":      "buffers_kb",
		"Cached:":       "cached_kb",
	}
	out := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key, ok := keys[fields[0]]
		if !ok {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
