package api

import (
	"net/http"
	"strconv"
	"strings"
)

// handleListViolations returns all violation audit logs for admin review.
func (a *App) handleListViolations(w http.ResponseWriter, r *http.Request, _ Params) {
	logs := a.store.ListViolationLogs()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	typeFilter := strings.ToLower(r.URL.Query().Get("type"))
	search := strings.ToLower(r.URL.Query().Get("search"))

	items := make([]map[string]any, 0, len(logs))
	for _, log := range logs {
		if typeFilter != "" && typeFilter != "all" && log.CodeType != typeFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(log.Username+" "+log.Code+" "+log.Reason), search) {
			continue
		}
		items = append(items, map[string]any{
			"id":          log.ID,
			"uid":         log.UID,
			"username":    log.Username,
			"code":        log.Code,
			"code_type":   log.CodeType,
			"reason":      log.Reason,
			"action":      log.Action,
			"ip":          log.IP,
			"telegram_id": nullableInt(log.TelegramID),
			"created_at":  log.CreatedAt,
		})
	}
	total := len(items)
	items = paginate(items, page, perPage)
	ok(w, "OK", map[string]any{"violations": items, "total": total, "page": page, "per_page": perPage})
}

// handleDeleteViolation removes a single violation log entry.
func (a *App) handleDeleteViolation(w http.ResponseWriter, r *http.Request, params Params) {
	id, err := strconv.ParseInt(params["violation_id"], 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "invalid violation ID")
		return
	}
	if statusFromError(w, a.store.DeleteViolationLog(id)) {
		return
	}
	ok(w, "violation log deleted", nil)
}

// handleClearViolations removes all violation logs.
func (a *App) handleClearViolations(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != "CLEAR_VIOLATIONS" {
		fail(w, http.StatusBadRequest, "需要确认短语 confirm=CLEAR_VIOLATIONS")
		return
	}
	if err := a.store.ClearViolationLogs(); err != nil {
		fail(w, http.StatusInternalServerError, "清除失败")
		return
	}
	ok(w, "all violation logs cleared", nil)
}
