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
	runtimeLogs      = newRuntimeLogSink(5000)
	runtimeLogLevel  = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	sensitivePattern = regexp.MustCompile(`(?i)(authorization|cookie|token|secret|password|passwd|api[_-]?key|bot[_-]?token|dsn)\s*[:=]\s*[^ \t\r\n,;]+`)
	bearerPattern    = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`)
	keyPattern       = regexp.MustCompile(`key-[A-Za-z0-9._~+/=-]{12,}`)
)

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
		return 5000
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
		copy(b.entries, b.entries[len(b.entries)-b.limit:])
		b.entries = b.entries[:b.limit]
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *runtimeLogBuffer) setLimit(limit int) {
	limit = clamp(limit, 100, 50000)
	b.mu.Lock()
	b.limit = limit
	if len(b.entries) > b.limit {
		copy(b.entries, b.entries[len(b.entries)-b.limit:])
		b.entries = b.entries[:b.limit]
	}
	b.mu.Unlock()
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
	filtered := make([]RuntimeLogEntry, 0, len(b.entries))
	for _, entry := range b.entries {
		if after <= 0 || entry.ID > after {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	next := b.nextID
	if len(filtered) > 0 {
		next = filtered[len(filtered)-1].ID
	}
	out := make([]RuntimeLogEntry, len(filtered))
	copy(out, filtered)
	return out, next
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
	runtimeLogLevel.SetLevel(level)
	stdLogLevel := zapcore.InfoLevel
	if level > stdLogLevel {
		stdLogLevel = level
	}
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	core := zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), zapcore.AddSync(w), runtimeLogLevel)
	logger := zap.New(&runtimeLogCore{Core: core}, zap.AddCaller(), zap.ErrorOutput(zapcore.AddSync(w)))
	zap.ReplaceGlobals(logger)
	zap.RedirectStdLogAt(logger, stdLogLevel)
}

func ConfigureRuntimeLogging(level zapcore.Level, limit int) {
	runtimeLogLevel.SetLevel(level)
	if limit > 0 {
		runtimeLogs.configure(nil, limit)
	}
}

func ConfigureRuntimeLoggingStore(st *store.Store, level zapcore.Level, limit int) {
	runtimeLogLevel.SetLevel(level)
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
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	return normalized == "key" ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "bottoken") ||
		strings.Contains(normalized, "dsn")
}

func redactSensitiveText(value string) string {
	if value == "" {
		return value
	}
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = keyPattern.ReplaceAllString(value, "key-[REDACTED]")
	value = sensitivePattern.ReplaceAllString(value, "$1=[REDACTED]")
	return value
}

func (a *App) handleRuntimeStatus(w http.ResponseWriter, r *http.Request, _ Params) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	logLimit, logEntries := runtimeLogs.stats()
	status := map[string]any{
		"started_at":          runtimeStartedAt.Unix(),
		"uptime_seconds":      int64(time.Since(runtimeStartedAt).Seconds()),
		"go_version":          runtime.Version(),
		"goos":                runtime.GOOS,
		"goarch":              runtime.GOARCH,
		"goroutines":          runtime.NumGoroutine(),
		"cpu_count":           runtime.NumCPU(),
		"redis_enabled":       a.redis != nil,
		"routes":              len(a.routes),
		"active_database":     a.store.Backend(),
		"config_database":     strings.ToLower(a.cfg.DatabaseDriver),
		"users":               a.store.UserCount(),
		"log_level":           a.cfg.LogLevel,
		"runtime_log_limit":   logLimit,
		"runtime_log_entries": logEntries,
		"runtime_log_backend": a.store.Backend(),
		"memory": map[string]any{
			"alloc":       mem.Alloc,
			"sys":         mem.Sys,
			"heap_alloc":  mem.HeapAlloc,
			"heap_sys":    mem.HeapSys,
			"heap_inuse":  mem.HeapInuse,
			"stack_inuse": mem.StackInuse,
			"next_gc":     mem.NextGC,
			"num_gc":      mem.NumGC,
		},
	}
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
	ok(w, "OK", map[string]any{"entries": entries, "next_cursor": next, "limit": limit})
}

func (a *App) handleRuntimeLogStream(w http.ResponseWriter, r *http.Request, _ Params) {
	flusher, okFlush := w.(http.Flusher)
	if !okFlush {
		fail(w, http.StatusInternalServerError, "当前响应不支持实时日志")
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
	if !send("snapshot", map[string]any{"entries": entries, "next_cursor": next}) {
		return
	}
	cursor = next
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send("ping", map[string]any{"time": time.Now().Unix(), "next_cursor": cursor}) {
				return
			}
		default:
			entries, next, okWait := runtimeLogs.waitAfter(r.Context(), cursor, limit)
			if !okWait {
				return
			}
			if len(entries) == 0 {
				continue
			}
			cursor = next
			if !send("logs", map[string]any{"entries": entries, "next_cursor": next}) {
				return
			}
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
