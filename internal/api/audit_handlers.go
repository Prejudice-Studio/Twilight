package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/prejudice-studio/twilight/internal/store"
)

const (
	auditDetailMaxBytes       = 8 * 1024
	auditDetailMaxNodes       = 512
	auditDetailMaxDepth       = 6
	auditDetailMaxItems       = 64
	auditDetailMaxStringRunes = 512
	auditRedactedValue        = "[REDACTED]"
	auditTruncatedValue       = "[TRUNCATED]"
)

var (
	destructiveAuditKeywords = []string{
		"delete", "disable", "clear", "prune", "revoke", "ban", "kick",
		"terminate", "reset_password", "force_unbind", "unbind", "detach",
	}
	securityAuditKeywords = []string{
		"login", "logout", "password", "role", "telegram", "developer",
		"security", "audit", "violation", "ip", "device", "apikey",
	}
)

// audit 是写入操作审计日志的便捷方法。category 为 "admin" / "user" / "system"。
// AuditLog.enabled=false 时静默跳过记录。从 current(r) 提取操作者身份。
func (a *App) audit(r *http.Request, action, category string, targetUID int64, detail map[string]any) {
	p := current(r)
	a.auditEntry(r, p.User.UID, p.User.Username, action, category, targetUID, detail)
}

// auditWithUser 用于登录等尚无会话上下文但已知用户身份的路径。
// 避免因为 AuthPublic 接口中 current(r) 返回零值导致审计日志 uid=0 / username=""。
func (a *App) auditWithUser(r *http.Request, uid int64, username, action, category string, targetUID int64, detail map[string]any) {
	a.auditEntry(r, uid, username, action, category, targetUID, detail)
}

func (a *App) auditEntry(r *http.Request, uid int64, username, action, category string, targetUID int64, detail map[string]any) {
	entry := store.AuditLog{
		UID:       uid,
		Username:  username,
		Action:    action,
		Category:  category,
		Source:    "http",
		Method:    r.Method,
		TargetUID: targetUID,
		Detail:    detail,
		IP:        a.clientIP(r),
	}
	a.writeAuditEntry(entry)
}

// auditEntryIP 是不依赖 *http.Request 的审计写入入口，供没有 HTTP 上下文的路径
// （如 Telegram Bot 命令）使用，IP 由调用方显式传入（如 "telegram"）。
func (a *App) auditEntryIP(ip string, uid int64, username, action, category string, targetUID int64, detail map[string]any) {
	source := "system"
	if strings.EqualFold(strings.TrimSpace(ip), "telegram") {
		source = "telegram"
	}
	a.writeAuditEntry(store.AuditLog{
		UID:       uid,
		Username:  username,
		Action:    action,
		Category:  category,
		Source:    source,
		TargetUID: targetUID,
		Detail:    detail,
		IP:        ip,
	})
}

// auditSystem records non-HTTP work such as scheduler and background-service
// mutations through the same normalization, redaction, and retention path.
func (a *App) auditSystem(source, action string, targetUID int64, detail map[string]any) {
	source = normalizeAuditSource(source)
	a.writeAuditEntry(store.AuditLog{
		Username:  source,
		Action:    action,
		Category:  "system",
		Source:    source,
		TargetUID: targetUID,
		Detail:    detail,
	})
}

func (a *App) auditTelegramAction(telegramID int64, action, category string, targetUID int64, detail map[string]any) {
	uid, username := a.telegramAdminIdentity(telegramID)
	if username == "" && telegramID != 0 {
		username = fmt.Sprintf("telegram:%d", telegramID)
	}
	if detail == nil {
		detail = map[string]any{}
	}
	if telegramID != 0 {
		detail["telegram_id"] = telegramID
	}
	a.auditEntryIP("telegram", uid, username, action, category, targetUID, detail)
}

func (a *App) writeAuditEntry(entry store.AuditLog) {
	cfg := a.cfg()
	if !cfg.AuditLogEnabled {
		return
	}
	if entry.UID < 0 {
		entry.UID = 0
	}
	if entry.TargetUID < 0 {
		entry.TargetUID = 0
	}
	entry.Username = truncateString(redactSensitiveText(strings.TrimSpace(entry.Username)), 128)
	entry.Action = normalizeAuditAction(entry.Action)
	entry.Category = normalizeAuditCategory(entry.Category)
	entry.Source = normalizeAuditSource(entry.Source)
	entry.Method = truncateString(strings.ToUpper(strings.TrimSpace(entry.Method)), 16)
	entry.IP = truncateString(strings.TrimSpace(entry.IP), 128)
	entry.Detail = sanitizeAuditDetail(entry.Detail)
	limit := cfg.AuditLogMaxEntries
	if limit <= 0 {
		limit = 10000
	}
	if err := a.store().AddAuditLog(entry, limit); err != nil {
		// Do not recurse into runtime logging with the full detail payload. The
		// persistence error is already sanitized by the shared text redactor.
		fmt.Printf("audit log persistence failed: %s\n", redactSensitiveText(err.Error()))
	}
}

func normalizeAuditAction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastSeparator := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastSeparator = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' || r == '/':
			if out.Len() > 0 && !lastSeparator {
				out.WriteByte('_')
				lastSeparator = true
			}
		}
		if out.Len() >= 80 {
			break
		}
	}
	action := strings.Trim(out.String(), "_")
	if action == "" {
		return "unknown_action"
	}
	return action
}

func normalizeAuditCategory(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "admin":
		return "admin"
	case "user":
		return "user"
	default:
		return "system"
	}
}

func normalizeAuditSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http", "api":
		return "http"
	case "telegram", "bot":
		return "telegram"
	case "scheduler", "schedule", "job":
		return "scheduler"
	default:
		return "system"
	}
}

type auditDetailSanitizer struct {
	nodesLeft int
	truncated bool
}

func sanitizeAuditDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	sanitizer := &auditDetailSanitizer{nodesLeft: auditDetailMaxNodes}
	out := sanitizer.sanitizeMap(detail, 0)
	if sanitizer.truncated {
		out["_truncated"] = true
	}
	return fitAuditDetail(out)
}

func (s *auditDetailSanitizer) sanitizeMap(value map[string]any, depth int) map[string]any {
	out := make(map[string]any, min(len(value), auditDetailMaxItems)+1)
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index >= auditDetailMaxItems || s.nodesLeft <= 0 {
			s.truncated = true
			break
		}
		cleanKey := truncateString(strings.ToValidUTF8(strings.TrimSpace(key), ""), 64)
		if cleanKey == "" {
			cleanKey = "field"
		}
		out[cleanKey] = s.sanitizeValue(cleanKey, value[key], depth+1)
	}
	return out
}

func (s *auditDetailSanitizer) sanitizeValue(key string, value any, depth int) any {
	if auditSensitiveDetailKey(key) {
		return auditRedactedValue
	}
	if s.nodesLeft <= 0 || depth > auditDetailMaxDepth {
		s.truncated = true
		return auditTruncatedValue
	}
	s.nodesLeft--
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return typed
	case string:
		return sanitizeAuditString(typed, s)
	case error:
		return sanitizeAuditString(typed.Error(), s)
	case map[string]any:
		return s.sanitizeMap(typed, depth)
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			converted[childKey] = childValue
		}
		return s.sanitizeMap(converted, depth)
	case []any:
		return s.sanitizeSlice(typed, depth)
	case []string:
		converted := make([]any, len(typed))
		for i, item := range typed {
			converted[i] = item
		}
		return s.sanitizeSlice(converted, depth)
	case []int64:
		converted := make([]any, len(typed))
		for i, item := range typed {
			converted[i] = item
		}
		return s.sanitizeSlice(converted, depth)
	case []int:
		converted := make([]any, len(typed))
		for i, item := range typed {
			converted[i] = item
		}
		return s.sanitizeSlice(converted, depth)
	default:
		encoded, err := json.Marshal(value)
		if err == nil {
			var generic any
			if json.Unmarshal(encoded, &generic) == nil {
				return s.sanitizeValue(key, generic, depth)
			}
		}
		return sanitizeAuditString(fmt.Sprint(value), s)
	}
}

func (s *auditDetailSanitizer) sanitizeSlice(value []any, depth int) []any {
	limit := min(len(value), auditDetailMaxItems)
	out := make([]any, 0, limit+1)
	for index := 0; index < limit; index++ {
		if s.nodesLeft <= 0 {
			s.truncated = true
			break
		}
		out = append(out, s.sanitizeValue("", value[index], depth+1))
	}
	if len(value) > limit {
		s.truncated = true
	}
	return out
}

func sanitizeAuditString(value string, sanitizer *auditDetailSanitizer) string {
	value = strings.ToValidUTF8(redactSensitiveText(value), "")
	if len([]rune(value)) > auditDetailMaxStringRunes {
		sanitizer.truncated = true
		value = truncateString(value, auditDetailMaxStringRunes) + "..."
	}
	return value
}

func auditSensitiveDetailKey(key string) bool {
	if sensitiveLogKey(key) {
		return true
	}
	normalized := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(key))
	if normalized == "code" || normalized == "codes" || normalized == "credential" || normalized == "credentials" {
		return true
	}
	if strings.Contains(normalized, "regcode") || strings.Contains(normalized, "invitecode") || strings.Contains(normalized, "bindcode") || strings.Contains(normalized, "verificationcode") {
		return true
	}
	if strings.HasSuffix(normalized, "code") {
		switch normalized {
		case "errorcode", "statuscode", "codetype", "sourcecode":
			return false
		default:
			return true
		}
	}
	return false
}

func fitAuditDetail(detail map[string]any) map[string]any {
	encoded, err := json.Marshal(detail)
	if err == nil && len(encoded) <= auditDetailMaxBytes {
		return detail
	}
	keys := make([]string, 0, len(detail))
	for key := range detail {
		if key != "_truncated" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := map[string]any{"_truncated": true}
	for _, key := range keys {
		out[key] = detail[key]
		candidate, marshalErr := json.Marshal(out)
		if marshalErr == nil && len(candidate) <= auditDetailMaxBytes {
			continue
		}
		out[key] = auditTruncatedValue
		candidate, marshalErr = json.Marshal(out)
		if marshalErr != nil || len(candidate) > auditDetailMaxBytes {
			delete(out, key)
		}
	}
	return out
}

func (a *App) handleListAuditLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	page := clamp(queryInt(r, "page", 1), 1, 1000000)
	perPage := clamp(queryInt(r, "per_page", 50), 1, 200)
	presetFilter := strings.ToLower(r.URL.Query().Get("preset"))
	categoryFilter := strings.ToLower(r.URL.Query().Get("category"))
	actionFilter := strings.ToLower(r.URL.Query().Get("action"))
	uidFilter := r.URL.Query().Get("uid")
	targetUIDFilter := r.URL.Query().Get("target_uid")
	search := strings.ToLower(r.URL.Query().Get("search"))
	from := auditLogUnixQuery(r, "from", "start")
	to := auditLogUnixQuery(r, "to", "end")
	sortBy := normalizeAuditLogSort(r.URL.Query().Get("sort"))
	order := normalizeSortOrder(r.URL.Query().Get("order"))

	uid, _ := strconv.ParseInt(uidFilter, 10, 64)
	targetUID, _ := strconv.ParseInt(targetUIDFilter, 10, 64)
	if categoryFilter == "all" {
		categoryFilter = ""
	}
	if actionFilter == "all" {
		actionFilter = ""
	}
	actionKeywords := []string(nil)
	switch presetFilter {
	case "admin", "user", "system":
		if categoryFilter == "" {
			categoryFilter = presetFilter
		} else if categoryFilter != presetFilter {
			categoryFilter = "__no_matching_category__"
		}
	case "destructive":
		actionKeywords = destructiveAuditKeywords
	case "security":
		actionKeywords = securityAuditKeywords
	case "today":
		now := time.Now()
		presetFrom := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
		if presetFrom > from {
			from = presetFrom
		}
	case "week":
		presetFrom := time.Now().Add(-7 * 24 * time.Hour).Unix()
		if presetFrom > from {
			from = presetFrom
		}
	}

	result := a.store().QueryAuditLogs(store.AuditLogQuery{
		Category:       categoryFilter,
		Action:         actionFilter,
		UID:            uid,
		TargetUID:      targetUID,
		From:           from,
		To:             to,
		Search:         truncateString(search, 200),
		ActionKeywords: actionKeywords,
		SortBy:         sortBy,
		Order:          order,
		Offset:         (page - 1) * perPage,
		Limit:          perPage,
	})
	dto := make([]map[string]any, 0, len(result.Logs))
	for _, log := range result.Logs {
		dto = append(dto, auditLogDTO(log))
	}
	ok(w, "OK", map[string]any{
		"logs":     dto,
		"total":    result.Total,
		"page":     page,
		"per_page": perPage,
		"sort":     sortBy,
		"order":    order,
	})
}

func auditLogUnixQuery(r *http.Request, names ...string) int64 {
	for _, name := range names {
		raw := strings.TrimSpace(r.URL.Query().Get(name))
		if raw == "" {
			continue
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func isDestructiveAuditAction(action string) bool {
	action = strings.ToLower(action)
	for _, keyword := range destructiveAuditKeywords {
		if strings.Contains(action, keyword) {
			return true
		}
	}
	return false
}

func isSecurityAuditAction(action string) bool {
	action = strings.ToLower(action)
	for _, keyword := range securityAuditKeywords {
		if strings.Contains(action, keyword) {
			return true
		}
	}
	return false
}

func normalizeAuditLogSort(value string) string {
	switch strings.ToLower(value) {
	case "id", "action", "category", "source", "method", "username", "uid", "target_uid", "ip":
		return strings.ToLower(value)
	default:
		return "created_at"
	}
}

func normalizeSortOrder(value string) string {
	if strings.EqualFold(value, "asc") {
		return "asc"
	}
	return "desc"
}

func (a *App) handleDeleteAuditLog(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := strconv.ParseInt(params["id"], 10, 64)
	if id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "无效的日志 ID")
		return
	}
	if err := a.store().DeleteAuditLog(id); err != nil {
		failWithCode(w, http.StatusNotFound, ErrNotFound, "日志不存在")
		return
	}
	ok(w, "已删除", nil)
}

func (a *App) handleClearAuditLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmClearAuditLogs {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "需要确认短语 confirm="+confirmClearAuditLogs)
		return
	}
	removed := a.store().AuditLogCount()
	if err := a.store().ClearAuditLogs(); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "清空失败")
		return
	}
	ok(w, "审计日志已清空", map[string]any{"removed": removed})
}

// handlePruneAuditLogs 条件清理审计日志：支持按条数裁剪（max_entries）和按天数裁剪（retention_days），
// 两者可同时指定。需要确认短语。preserve_admin 控制是否保留管理员操作日志（仅对天数裁剪有效）。
func (a *App) handlePruneAuditLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmPruneAuditLogs {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "需要确认短语 confirm="+confirmPruneAuditLogs)
		return
	}

	maxEntries := clamp(intValue(payload, "max_entries", 0), 0, 100000)
	retentionDays := clamp(intValue(payload, "retention_days", 0), 0, 3650)
	if maxEntries == 0 && retentionDays == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "请指定 max_entries 或 retention_days")
		return
	}
	preserveAdmin := boolValue(payload, "preserve_admin", true)
	cutoff := int64(0)
	if retentionDays > 0 {
		cutoff = time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	}
	result, err := a.store().PruneAuditLogsWithPolicy(store.AuditLogPruneOptions{
		MaxEntries:    maxEntries,
		CutoffUnix:    cutoff,
		PreserveAdmin: preserveAdmin,
	})
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "裁剪失败")
		return
	}
	logs := []string{}
	if maxEntries > 0 {
		logs = append(logs, fmt.Sprintf("保留最近 %d 条，删除 %d 条", maxEntries, result.RemovedByLimit))
	}
	if retentionDays > 0 {
		logs = append(logs, fmt.Sprintf("删除 %d 天前 %d 条（保留管理员=%v）", retentionDays, result.RemovedByAge, preserveAdmin))
	}
	ok(w, "审计日志已清理", map[string]any{
		"current": a.store().AuditLogCount(),
		"logs":    logs,
	})
}

func auditLogDTO(log store.AuditLog) map[string]any {
	source := log.Source
	if source == "" {
		switch {
		case strings.EqualFold(log.IP, "telegram"):
			source = "telegram"
		case strings.EqualFold(log.Category, "system") && log.UID == 0:
			source = "system"
		default:
			source = "http"
		}
	}
	return map[string]any{
		"id":         log.ID,
		"uid":        log.UID,
		"username":   log.Username,
		"action":     log.Action,
		"category":   log.Category,
		"source":     source,
		"method":     log.Method,
		"target_uid": zeroNil(log.TargetUID),
		"detail":     log.Detail,
		"ip":         log.IP,
		"created_at": log.CreatedAt,
	}
}
