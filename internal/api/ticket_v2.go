package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

// adminTicketPageQuery is shared by the V1 compatibility queue and the V2
// resource collection. Keeping the filter parser in one place prevents the
// two endpoints from silently returning different work queues.
func (a *App) adminTicketPageQuery(r *http.Request) (store.TicketFilter, int, int, string) {
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	showAll := r.URL.Query().Get("all") == "1"
	page := clamp(queryInt(r, "page", 1), 1, 1_000_000)
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	if status == "all" {
		showAll = true
		status = ""
	}
	if status != "" && !store.ValidTicketStatus(status) {
		return store.TicketFilter{}, 0, 0, "无效的工单状态"
	}
	ticketType := strings.TrimSpace(r.URL.Query().Get("type"))
	if ticketType != "" {
		ticketType = store.NormalizeTicketType(a.store().TicketTypes(), ticketType)
	}
	priority := strings.TrimSpace(r.URL.Query().Get("priority"))
	if priority != "" {
		if !store.ValidTicketPriority(priority) {
			return store.TicketFilter{}, 0, 0, "无效的优先级"
		}
		priority = store.NormalizeTicketPriority(priority)
	}
	filter := store.TicketFilter{
		UID:        int64(queryInt(r, "uid", 0)),
		Status:     store.NormalizeTicketStatus(status),
		Type:       ticketType,
		Priority:   priority,
		ActiveOnly: status == "" && !showAll,
	}
	if status == "" {
		filter.Status = ""
	}
	return filter, page, perPage, ""
}

type v2TicketPagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type v2AdminTicketListResponse struct {
	Items       []adminTicketListDTO `json:"items"`
	Pagination  v2TicketPagination   `json:"pagination"`
	TicketTypes []string             `json:"ticket_types"`
}

type v2AdminTicketDetailResponse struct {
	Item        map[string]any `json:"item"`
	TicketTypes []string       `json:"ticket_types"`
}

type v2UserTicketListResponse struct {
	Items       []userTicketListDTO `json:"items"`
	Pagination  v2TicketPagination  `json:"pagination"`
	TicketTypes []string            `json:"ticket_types"`
}

type v2UserTicketDetailResponse struct {
	Item        map[string]any `json:"item"`
	TicketTypes []string       `json:"ticket_types"`
}

func v2UserTicketAttachmentURL(ticketID int64, filename string) string {
	return "/api/v2/tickets/" + strconv.FormatInt(ticketID, 10) + "/attachments/" + url.PathEscape(filename)
}

func v2UserTicketDTO(ticket store.Ticket) map[string]any {
	dto := ticketDTO(ticket, false)
	attachments := make([]map[string]any, 0, len(ticket.Attachments))
	for _, attachment := range ticket.Attachments {
		attachments = append(attachments, map[string]any{
			"filename":     attachment.Filename,
			"url":          v2UserTicketAttachmentURL(ticket.ID, attachment.Filename),
			"content_type": attachment.ContentType,
			"size":         attachment.Size,
			"uploaded_uid": attachment.UploadedUID,
			"created_at":   attachment.CreatedAt,
		})
	}
	dto["attachments"] = attachments
	return dto
}

// User ticket resources keep the browser-facing contract independent from the
// rollback API. Reply writes use the shared application operation so ownership,
// closed-state checks and atomic persistence cannot drift between versions.
func (a *App) handleV2UserTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	if a.refreshStoreForRequest(w, r) {
		return
	}
	p := current(r)
	page := clamp(queryInt(r, "page", 1), 1, 1_000_000)
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	result := a.store().ListTicketsPage(store.TicketFilter{UID: p.User.UID}, page, perPage)
	totalPages := (result.Total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	ok(w, "OK", v2UserTicketListResponse{
		Items: userTicketListDTOs(result.Tickets),
		Pagination: v2TicketPagination{
			Page:       page,
			PerPage:    perPage,
			Total:      result.Total,
			TotalPages: totalPages,
		},
		TicketTypes: a.store().TicketTypes(),
	})
}

func (a *App) handleV2UserTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, err := int64Param(params, "ticket_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单编号")
		return
	}
	if a.refreshStoreForRequest(w, r) {
		return
	}
	p := current(r)
	ticket, found := a.store().Ticket(id)
	if !found || ticket.UID != p.User.UID {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	ok(w, "OK", v2UserTicketDetailResponse{
		Item:        v2UserTicketDTO(ticket),
		TicketTypes: a.store().TicketTypes(),
	})
}

func (a *App) handleV2CreateTicket(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleCreateTicket(w, r, p)
}

func (a *App) handleV2UserTicketReply(w http.ResponseWriter, r *http.Request, p Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	actor := current(r).User
	if !a.allowRate(r.Context(), rateKey("ticket-reply:uid:", actor.UID), 20, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrTicketRateLimited, "回复过于频繁，请稍后再试")
		return
	}
	id, err := int64Param(p, "ticket_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单编号")
		return
	}
	if a.refreshStoreForRequest(w, r) {
		return
	}
	payload := decodeMap(r)
	updated, existing, err := a.appendTicketReply(id, actor, stringValue(payload, "content"))
	if writeTicketReplyFailure(w, err) {
		return
	}
	a.audit(r, "reply_ticket", auditCategoryForRole(actor.Role), existing.UID, map[string]any{"ticket_id": id, "reply_len": len(strings.TrimSpace(stringValue(payload, "content")))})
	if actor.Role == store.RoleAdmin {
		a.notifyTicketOwner(r.Context(), updated, existing)
	} else {
		a.notifyTicketAdmins(r.Context(), "replied", updated, actor)
	}
	ok(w, "回复成功", map[string]any{
		"ticket_id": id,
		"ticket":    v2UserTicketDTO(updated),
		"replies":   ticketReplyDTOs(updated.Replies),
	})
}

func (a *App) handleV2CloseUserTicket(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleCloseOwnTicket(w, r, p)
}

func (a *App) handleV2ReopenUserTicket(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleReopenOwnTicket(w, r, p)
}

func (a *App) handleV2ToggleUserTicketNotify(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleToggleTicketNotify(w, r, p)
}

func (a *App) handleV2UserTicketAttachmentUpload(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleUploadTicketImage(w, r, p)
}

func (a *App) handleV2UserTicketAttachment(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleGetTicketImage(w, r, p)
}

func (a *App) handleV2UserTicketAttachmentDelete(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleDeleteTicketImage(w, r, p)
}

func (a *App) handleV2AdminTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	filter, page, perPage, invalid := a.adminTicketPageQuery(r)
	if invalid != "" {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, invalid)
		return
	}
	result := a.store().ListTicketsPage(filter, page, perPage)
	totalPages := (result.Total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	ok(w, "OK", v2AdminTicketListResponse{
		Items: ticketListDTOs(result.Tickets),
		Pagination: v2TicketPagination{
			Page:       page,
			PerPage:    perPage,
			Total:      result.Total,
			TotalPages: totalPages,
		},
		TicketTypes: a.store().TicketTypes(),
	})
}

func v2AdminTicketAttachmentURL(ticketID int64, filename string) string {
	return "/api/v2/admin/tickets/" + strconv.FormatInt(ticketID, 10) + "/attachments/" + url.PathEscape(filename)
}

func v2AdminTicketDTO(ticket store.Ticket) map[string]any {
	dto := ticketDTO(ticket, true)
	attachments := make([]map[string]any, 0, len(ticket.Attachments))
	for _, attachment := range ticket.Attachments {
		attachments = append(attachments, map[string]any{
			"filename":     attachment.Filename,
			"url":          v2AdminTicketAttachmentURL(ticket.ID, attachment.Filename),
			"content_type": attachment.ContentType,
			"size":         attachment.Size,
			"uploaded_uid": attachment.UploadedUID,
			"created_at":   attachment.CreatedAt,
		})
	}
	dto["attachments"] = attachments
	return dto
}

func (a *App) handleV2AdminTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	id, err := int64Param(params, "ticket_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单编号")
		return
	}
	ticket, found := a.store().Ticket(id)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	ok(w, "OK", v2AdminTicketDetailResponse{
		Item:        v2AdminTicketDTO(ticket),
		TicketTypes: a.store().TicketTypes(),
	})
}

func (a *App) handleV2AdminReplyTicket(w http.ResponseWriter, r *http.Request, params Params) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := int64Param(params, "ticket_id")
	if err != nil || id <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单编号")
		return
	}
	if a.refreshStoreForRequest(w, r) {
		return
	}
	payload := decodeMap(r)
	actor := current(r).User
	ticket, existing, err := a.appendTicketReply(id, actor, stringValue(payload, "content"))
	if writeTicketReplyFailure(w, err) {
		return
	}
	content := strings.TrimSpace(stringValue(payload, "content"))
	a.audit(r, "reply_ticket", "admin", existing.UID, map[string]any{"ticket_id": id, "reply_len": len(content)})
	a.notifyTicketOwner(r.Context(), ticket, existing)
	a.notifyTicketAdmins(r.Context(), "admin_replied", ticket, actor)
	ok(w, "回复成功", map[string]any{
		"ticket_id": id,
		"item":      v2AdminTicketDTO(ticket),
		"replies":   ticketReplyDTOs(ticket.Replies),
	})
}

func (a *App) handleV2AdminTicketTypes(w http.ResponseWriter, _ *http.Request, _ Params) {
	ok(w, "OK", map[string]any{"items": a.store().TicketTypes()})
}

func (a *App) handleV2AdminRenameTicketType(w http.ResponseWriter, r *http.Request, params Params) {
	oldName := strings.TrimSpace(params["ticket_type"])
	payload := decodeMap(r)
	newName := strings.TrimSpace(stringValue(payload, "name"))
	if oldName == "" || newName == "" || len(newName) > 50 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "类型名称需为 1-50 个字符")
		return
	}
	count, err := a.store().RenameTicketType(oldName, newName)
	if statusFromError(w, err) {
		return
	}
	a.persistTicketTypesFromStore()
	a.audit(r, "rename_ticket_type", "admin", 0, map[string]any{"old": oldName, "new": newName, "tickets_renamed": count})
	ok(w, "类型已重命名", map[string]any{"item": newName, "items": a.store().TicketTypes(), "tickets_renamed": count})
}

func (a *App) handleV2AdminDeleteTicketType(w http.ResponseWriter, r *http.Request, params Params) {
	name := strings.TrimSpace(params["ticket_type"])
	if name == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "类型名称不能为空")
		return
	}
	if err := a.store().DeleteTicketType(name); statusFromError(w, err) {
		return
	}
	a.persistTicketTypesFromStore()
	a.audit(r, "delete_ticket_type", "admin", 0, map[string]any{"name": name})
	ok(w, "类型已删除", map[string]any{"items": a.store().TicketTypes()})
}
