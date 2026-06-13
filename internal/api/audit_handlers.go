package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

// audit 是写入操作审计日志的便捷方法。category 为 "admin" / "user" / "system"。
// AuditLog.enabled=false 时静默跳过记录。
func (a *App) audit(r *http.Request, action, category string, targetUID int64, detail map[string]any) {
	cfg := a.cfg()
	if !cfg.AuditLogEnabled {
		return
	}
	p := current(r)
	entry := store.AuditLog{
		UID:       p.User.UID,
		Username:  p.User.Username,
		Action:    action,
		Category:  category,
		TargetUID: targetUID,
		Detail:    detail,
		IP:        a.clientIP(r),
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
	categoryFilter := strings.ToLower(r.URL.Query().Get("category"))
	actionFilter := strings.ToLower(r.URL.Query().Get("action"))
	uidFilter := r.URL.Query().Get("uid")
	search := strings.ToLower(r.URL.Query().Get("search"))

	filtered := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		if categoryFilter != "" && categoryFilter != "all" && log.Category != categoryFilter {
			continue
		}
		if actionFilter != "" && actionFilter != "all" && log.Action != actionFilter {
			continue
		}
		if uidFilter != "" {
			uid, _ := strconv.ParseInt(uidFilter, 10, 64)
			if uid > 0 && log.UID != uid {
				continue
			}
		}
		if search != "" {
			haystack := strings.ToLower(log.Username + " " + log.Action + " " + log.Category + " " + log.IP)
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		filtered = append(filtered, auditLogDTO(log))
	}

	total := len(filtered)
	filtered = paginate(filtered, page, perPage)
	ok(w, "OK", map[string]any{"logs": filtered, "total": total, "page": page, "per_page": perPage})
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
