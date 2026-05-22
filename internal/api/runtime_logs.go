package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RuntimeLogEntry struct {
	ID      int64             `json:"id"`
	Time    int64             `json:"time"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}

type runtimeLogBuffer struct {
	mu      sync.Mutex
	cond    *sync.Cond
	nextID  int64
	entries []RuntimeLogEntry
	limit   int
}

var (
	runtimeStartedAt = time.Now()
	runtimeLogs      = newRuntimeLogBuffer(1200)
	sensitivePattern = regexp.MustCompile(`(?i)(authorization|cookie|token|secret|password|passwd|api[_-]?key|bot[_-]?token|dsn)\s*[:=]\s*[^ \t\r\n,;]+`)
	bearerPattern    = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{12,}`)
	keyPattern       = regexp.MustCompile(`key-[A-Za-z0-9._~+/=-]{12,}`)
)

func newRuntimeLogBuffer(limit int) *runtimeLogBuffer {
	b := &runtimeLogBuffer{limit: limit}
	b.cond = sync.NewCond(&b.mu)
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

func (b *runtimeLogBuffer) snapshot(limit int, after int64) ([]RuntimeLogEntry, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 || limit > b.limit {
		limit = 200
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

type runtimeLogHandler struct {
	next  slog.Handler
	attrs []slog.Attr
}

func InstallRuntimeLogger(w io.Writer, level slog.Leveler) {
	if w == nil {
		w = io.Discard
	}
	if level == nil {
		level = slog.LevelInfo
	}
	next := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(&runtimeLogHandler{next: next}))
}

func (h *runtimeLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *runtimeLogHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := map[string]string{}
	for _, attr := range h.attrs {
		addLogAttr(attrs, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		addLogAttr(attrs, attr)
		return true
	})
	runtimeLogs.append(RuntimeLogEntry{
		Time:    record.Time.Unix(),
		Level:   record.Level.String(),
		Message: redactSensitiveText(record.Message),
		Attrs:   attrs,
	})
	return h.next.Handle(ctx, record)
}

func (h *runtimeLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nextAttrs := append([]slog.Attr{}, h.attrs...)
	nextAttrs = append(nextAttrs, attrs...)
	return &runtimeLogHandler{next: h.next.WithAttrs(attrs), attrs: nextAttrs}
}

func (h *runtimeLogHandler) WithGroup(name string) slog.Handler {
	return &runtimeLogHandler{next: h.next.WithGroup(name), attrs: append([]slog.Attr{}, h.attrs...)}
}

func addLogAttr(attrs map[string]string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	key := strings.ToLower(attr.Key)
	value := fmt.Sprint(attr.Value.Any())
	if sensitiveLogKey(key) {
		attrs[attr.Key] = "[REDACTED]"
		return
	}
	attrs[attr.Key] = redactSensitiveText(value)
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
	status := map[string]any{
		"started_at":      runtimeStartedAt.Unix(),
		"uptime_seconds":  int64(time.Since(runtimeStartedAt).Seconds()),
		"go_version":      runtime.Version(),
		"goos":            runtime.GOOS,
		"goarch":          runtime.GOARCH,
		"goroutines":      runtime.NumGoroutine(),
		"cpu_count":       runtime.NumCPU(),
		"redis_enabled":   a.redis != nil,
		"routes":          len(a.routes),
		"active_database": a.store.Backend(),
		"config_database": strings.ToLower(a.cfg.DatabaseDriver),
		"users":           a.store.UserCount(),
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
	limit := clamp(queryInt(r, "limit", 200), 1, 1000)
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

	limit := clamp(queryInt(r, "limit", 100), 1, 300)
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
