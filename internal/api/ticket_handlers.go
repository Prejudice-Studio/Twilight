package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// handleMyTickets 用户查看自己提交的工单。
func (a *App) handleMyTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	p := current(r)
	tickets := a.store().ListTickets(store.TicketFilter{UID: p.User.UID})
	ok(w, "OK", map[string]any{"tickets": tickets, "total": len(tickets), "ticket_types": cfg.TicketTypes})
}

// handleCreateTicket 用户提交工单。
func (a *App) handleCreateTicket(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	p := current(r)
	if !a.allowRate(r.Context(), rateKey("ticket:uid:", p.User.UID), 10, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrTicketRateLimited, "提交工单过于频繁，请稍后再试")
		return
	}
	payload := decodeMap(r)
	title := strings.TrimSpace(stringValue(payload, "title"))
	content := strings.TrimSpace(stringValue(payload, "content"))
	ticketType := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringValue(payload, "type"), "general")))
	priority := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringValue(payload, "priority"), "medium")))

	if title == "" {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "请填写工单标题")
		return
	}
	if len(title) > 200 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "工单标题过长")
		return
	}
	if content == "" {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "请填写工单内容")
		return
	}
	if len(content) > 10000 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "工单内容过长")
		return
	}
	if !validTicketType(cfg.TicketTypes, ticketType) {
		ticketType = "other"
	}
	if !validTicketPriority(priority) {
		priority = "medium"
	}

	ticket, err := a.store().UpsertTicket(store.Ticket{
		UID:      p.User.UID,
		Username: p.User.Username,
		Title:    title,
		Content:  content,
		Type:     ticketType,
		Priority: priority,
		Status:   "open",
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "create_ticket", "user", 0, map[string]any{"ticket_id": ticket.ID, "type": ticketType, "priority": priority})
	created(w, "工单已提交", ticket)
}

// ---- 管理员工单接口 ----

// handleAdminTickets 管理员查看所有工单（支持筛选）。
func (a *App) handleAdminTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	filter := store.TicketFilter{
		UID:      int64(queryInt(r, "uid", 0)),
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Type:     strings.TrimSpace(r.URL.Query().Get("type")),
		Priority: strings.TrimSpace(r.URL.Query().Get("priority")),
	}
	tickets := a.store().ListTickets(filter)
	ok(w, "OK", map[string]any{"tickets": tickets, "total": len(tickets), "ticket_types": cfg.TicketTypes})
}

// handleAdminUpdateTicket 管理员更新工单状态 / 回复。
func (a *App) handleAdminUpdateTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	payload := decodeMap(r)

	existing, foundTicket := a.store().Ticket(id)
	if !foundTicket {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}

	status := strings.TrimSpace(firstNonEmpty(stringValue(payload, "status"), existing.Status))
	if !validTicketStatus(status) {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单状态")
		return
	}

	priority := strings.TrimSpace(firstNonEmpty(stringValue(payload, "priority"), existing.Priority))
	if !validTicketPriority(priority) {
		priority = existing.Priority
	}

	ticketType := strings.TrimSpace(firstNonEmpty(stringValue(payload, "type"), existing.Type))
	adminNote := strings.TrimSpace(stringValue(payload, "admin_note"))

	// 管理员更新时保护已有字段
	ticket, err := a.store().UpsertTicket(store.Ticket{
		ID:        id,
		UID:       existing.UID,
		Username:  existing.Username,
		Title:     existing.Title,
		Content:   existing.Content,
		Type:      ticketType,
		Status:    status,
		Priority:  priority,
		AdminNote: adminNote,
		CreatedAt: existing.CreatedAt,
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "update_ticket", "admin", ticket.UID, map[string]any{"ticket_id": ticket.ID, "new_status": status})
	ok(w, "工单已更新", ticket)
}

// handleAdminDeleteTicket 管理员删除工单。
func (a *App) handleAdminDeleteTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	if statusFromError(w, a.store().DeleteTicket(id)) {
		return
	}
	a.audit(r, "delete_ticket", "admin", 0, map[string]any{"ticket_id": id})
	ok(w, "工单已删除", nil)
}

// ---- 校验工具 ----

func validTicketStatus(status string) bool {
	switch status {
	case "open", "in_progress", "resolved", "closed":
		return true
	}
	return false
}

func validTicketPriority(priority string) bool {
	switch priority {
	case "low", "medium", "high", "urgent":
		return true
	}
	return false
}

func validTicketType(types []string, input string) bool {
	for _, t := range types {
		if strings.EqualFold(t, input) {
			return true
		}
	}
	return false
}
