package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
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
	a.auditEntryIP(a.clientIP(r), uid, username, action, category, targetUID, detail)
}

// auditEntryIP 是不依赖 *http.Request 的审计写入入口，供没有 HTTP 上下文的路径
// （如 Telegram Bot 命令）使用，IP 由调用方显式传入（如 "telegram"）。
func (a *App) auditEntryIP(ip string, uid int64, username, action, category string, targetUID int64, detail map[string]any) {
	cfg := a.cfg()
	if !cfg.AuditLogEnabled {
		return
	}
	entry := store.AuditLog{
		UID:       uid,
		Username:  username,
		Action:    action,
		Category:  category,
		TargetUID: targetUID,
		Detail:    detail,
		IP:        ip,
	}
	if cfg.AuditLogMaxEntries > 0 {
		_ = a.store().AddAuditLog(entry, cfg.AuditLogMaxEntries)
	} else {
		_ = a.store().AddAuditLog(entry, 10000)
	}
}

func (a *App) handleListAuditLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	logs := a.store().ListAuditLogs()
	page := max(1, queryInt(r, "page", 1))
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

	filtered := make([]store.AuditLog, 0, len(logs))
	for _, log := range logs {
		if !auditLogMatchesPreset(log, presetFilter) {
			continue
		}
		if categoryFilter != "" && categoryFilter != "all" && strings.ToLower(log.Category) != categoryFilter {
			continue
		}
		if actionFilter != "" && actionFilter != "all" && strings.ToLower(log.Action) != actionFilter {
			continue
		}
		if uid > 0 && log.UID != uid {
			continue
		}
		if targetUID > 0 && log.TargetUID != targetUID {
			continue
		}
		if from > 0 && log.CreatedAt < from {
			continue
		}
		if to > 0 && log.CreatedAt > to {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				log.Username,
				log.Action,
				log.Category,
				log.IP,
				strconv.FormatInt(log.UID, 10),
				strconv.FormatInt(log.TargetUID, 10),
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, log)
	}

	sortAuditLogs(filtered, sortBy, order)
	total := len(filtered)
	paged := paginate(filtered, page, perPage)
	dto := make([]map[string]any, 0, len(paged))
	for _, log := range paged {
		dto = append(dto, auditLogDTO(log))
	}
	ok(w, "OK", map[string]any{
		"logs":     dto,
		"total":    total,
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

func auditLogMatchesPreset(log store.AuditLog, preset string) bool {
	switch preset {
	case "", "all":
		return true
	case "admin", "user", "system":
		return strings.EqualFold(log.Category, preset)
	case "destructive":
		return isDestructiveAuditAction(log.Action)
	case "security":
		return isSecurityAuditAction(log.Action)
	case "today":
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
		return log.CreatedAt >= start
	case "week":
		return log.CreatedAt >= time.Now().Add(-7*24*time.Hour).Unix()
	default:
		return true
	}
}

func isDestructiveAuditAction(action string) bool {
	action = strings.ToLower(action)
	keywords := []string{
		"delete", "disable", "clear", "prune", "revoke", "ban", "kick",
		"terminate", "reset_password", "force_unbind", "unbind", "detach",
	}
	for _, keyword := range keywords {
		if strings.Contains(action, keyword) {
			return true
		}
	}
	return false
}

func isSecurityAuditAction(action string) bool {
	action = strings.ToLower(action)
	keywords := []string{
		"login", "logout", "password", "role", "telegram", "developer",
		"security", "audit", "violation", "ip", "device", "apikey",
	}
	for _, keyword := range keywords {
		if strings.Contains(action, keyword) {
			return true
		}
	}
	return false
}

func normalizeAuditLogSort(value string) string {
	switch strings.ToLower(value) {
	case "id", "action", "category", "username", "uid", "target_uid", "ip":
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

func sortAuditLogs(logs []store.AuditLog, sortBy, order string) {
	desc := order != "asc"
	sort.SliceStable(logs, func(i, j int) bool {
		left, right := logs[i], logs[j]
		cmp := int64(0)
		switch sortBy {
		case "id":
			cmp = left.ID - right.ID
		case "action":
			cmp = int64(strings.Compare(strings.ToLower(left.Action), strings.ToLower(right.Action)))
		case "category":
			cmp = int64(strings.Compare(strings.ToLower(left.Category), strings.ToLower(right.Category)))
		case "username":
			cmp = int64(strings.Compare(strings.ToLower(left.Username), strings.ToLower(right.Username)))
		case "uid":
			cmp = left.UID - right.UID
		case "target_uid":
			cmp = left.TargetUID - right.TargetUID
		case "ip":
			cmp = int64(strings.Compare(strings.ToLower(left.IP), strings.ToLower(right.IP)))
		default:
			cmp = left.CreatedAt - right.CreatedAt
		}
		if cmp == 0 {
			cmp = left.ID - right.ID
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
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
	if err := a.store().ClearAuditLogs(); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrInternal, "清空失败")
		return
	}
	ok(w, "审计日志已清空", nil)
}

// handlePruneAuditLogs 条件清理审计日志：支持按条数裁剪（max_entries）和按天数裁剪（retention_days），
// 两者可同时指定。需要确认短语。preserve_admin 控制是否保留管理员操作日志（仅对天数裁剪有效）。
func (a *App) handlePruneAuditLogs(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmPruneAuditLogs {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "需要确认短语 confirm="+confirmPruneAuditLogs)
		return
	}

	logs := []string{}

	// 按条数裁剪：保留最新 N 条
	if maxEntries := intValue(payload, "max_entries", 0); maxEntries > 0 {
		if err := a.store().PruneAuditLogs(maxEntries); err != nil {
			failWithCode(w, http.StatusInternalServerError, ErrInternal, "裁剪失败")
			return
		}
		logs = append(logs, fmt.Sprintf("保留最近 %d 条", maxEntries))
	}

	// 按天数裁剪：删除早于 retention_days 的记录
	if retentionDays := intValue(payload, "retention_days", 0); retentionDays > 0 {
		preserveAdmin := boolValue(payload, "preserve_admin", true)
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
		removed := a.store().PruneAuditLogsByAge(cutoff, preserveAdmin)
		logs = append(logs, fmt.Sprintf("删除 %d 天前 %d 条（保留管理员=%v）", retentionDays, removed, preserveAdmin))
	}

	if len(logs) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "请指定 max_entries 或 retention_days")
		return
	}

	ok(w, "审计日志已清理", map[string]any{
		"current": a.store().AuditLogCount(),
		"logs":    logs,
	})
}

func auditLogDTO(log store.AuditLog) map[string]any {
	return map[string]any{
		"id":         log.ID,
		"uid":        log.UID,
		"username":   log.Username,
		"action":     log.Action,
		"category":   log.Category,
		"target_uid": zeroNil(log.TargetUID),
		"detail":     log.Detail,
		"ip":         log.IP,
		"created_at": log.CreatedAt,
	}
}
