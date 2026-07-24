package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/config"
	"github.com/prejudice-studio/twilight/internal/store"
	"go.uber.org/zap"
)

// handleMyTickets 用户查看自己提交的工单。
func (a *App) handleMyTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	if a.refreshStoreForRequest(w) {
		return
	}
	p := current(r)
	tickets := a.store().ListTickets(store.TicketFilter{UID: p.User.UID})
	ok(w, "OK", map[string]any{"tickets": ticketDTOs(tickets), "total": len(tickets), "ticket_types": a.store().TicketTypes()})
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
	ticketType := store.NormalizeTicketType(a.store().TicketTypes(), firstNonEmpty(stringValue(payload, "type"), store.TicketTypeDefault))
	priority := store.NormalizeTicketPriority(firstNonEmpty(stringValue(payload, "priority"), store.TicketPriorityMedium))

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
	var notifyTG *bool
	if _, ok := payload["notify_telegram"]; ok {
		b := boolValue(payload, "notify_telegram", true)
		notifyTG = &b
	}

	ticket, err := a.store().CreateTicket(store.Ticket{
		UID:            p.User.UID,
		Username:       p.User.Username,
		Title:          title,
		Content:        content,
		Type:           ticketType,
		Priority:       priority,
		Status:         store.TicketStatusOpen,
		NotifyTelegram: notifyTG,
	}, cfg.TicketUserOpenLimit, cfg.TicketGlobalOpenLimit)
	if errors.Is(err, store.ErrTicketUserOpenLimit) {
		failWithCode(w, http.StatusConflict, ErrTicketUserLimit, "您当前待处理 / 处理中的工单已达上限，请先关闭部分工单后再提交")
		return
	}
	if errors.Is(err, store.ErrTicketGlobalOpenLimit) {
		failWithCode(w, http.StatusConflict, ErrTicketGlobalLimit, "系统当前待处理 / 处理中的工单已达上限，请稍后再提交")
		return
	}
	if statusFromError(w, err) {
		return
	}
	zap.L().Info("工单已创建",
		zap.Int64("ticket_id", ticket.ID),
		zap.Int64("uid", ticket.UID),
		zap.String("username", ticket.Username),
		zap.String("type", ticket.Type),
		zap.String("priority", ticket.Priority),
	)
	a.audit(r, "create_ticket", "user", p.User.UID, map[string]any{"ticket_id": ticket.ID, "type": ticketType, "priority": priority})
	a.notifyTicketAdmins(r.Context(), "created", ticket, p.User)
	created(w, "工单已提交", ticketDTO(ticket))
}

// handleCloseOwnTicket 用户关闭自己的工单。
func (a *App) handleCloseOwnTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	p := current(r)
	if a.refreshStoreForRequest(w) {
		return
	}
	existing, found := a.store().Ticket(id)
	if !found || existing.UID != p.User.UID {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	if !store.TicketStatusAllowsConversation(existing.Status) {
		failWithCode(w, http.StatusBadRequest, ErrTicketAlreadyClosed, "工单已关闭")
		return
	}
	status := store.TicketStatusClosed
	ticket, err := a.store().UpdateTicket(id, store.TicketUpdate{Status: &status})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "close_ticket", "user", 0, map[string]any{"ticket_id": id})
	a.notifyTicketAdmins(r.Context(), "closed", ticket, p.User)
	ok(w, "工单已关闭", ticketDTO(ticket))
}

// handleReopenOwnTicket 用户重开自己的已关闭工单。
func (a *App) handleReopenOwnTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	p := current(r)
	if a.refreshStoreForRequest(w) {
		return
	}
	existing, found := a.store().Ticket(id)
	if !found || existing.UID != p.User.UID {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	if store.NormalizeTicketStatus(existing.Status) != store.TicketStatusClosed {
		failWithCode(w, http.StatusBadRequest, ErrTicketNotClosed, "只有已关闭的工单可以重开")
		return
	}
	status := store.TicketStatusOpen
	ticket, err := a.store().UpdateTicket(id, store.TicketUpdate{Status: &status})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "reopen_ticket", "user", 0, map[string]any{"ticket_id": id})
	a.notifyTicketAdmins(r.Context(), "reopened", ticket, p.User)
	ok(w, "工单已重开", ticketDTO(ticket))
}

// handleToggleTicketNotify 切换单个工单的 Telegram 通知开关。
func (a *App) handleToggleTicketNotify(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	p := current(r)
	if a.refreshStoreForRequest(w) {
		return
	}
	existing, found := a.store().Ticket(id)
	if !found || existing.UID != p.User.UID {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	payload := decodeMap(r)
	if _, ok := payload["enabled"]; !ok {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "缺少 enabled 字段")
		return
	}
	enabled := boolValue(payload, "enabled", true)
	ticket, err := a.store().SetTicketNotify(id, enabled)
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "toggle_ticket_notify", "user", 0, map[string]any{"ticket_id": id, "enabled": enabled})
	ok(w, "通知设置已更新", ticketDTO(ticket))
}

// ---- 管理员工单接口 ----

// handleAdminTickets 管理员查看所有工单（支持筛选）。管理端接口不受 TicketSystemEnabled 开关限制。
// 默认仅显示未解决的工单（open / in_progress），传 ?all=1 可查看全部包括已解决/已关闭。
func (a *App) handleAdminTickets(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w) {
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	showAll := r.URL.Query().Get("all") == "1"
	page := clamp(queryInt(r, "page", 1), 1, 1000000)
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	if status == "all" {
		showAll = true
		status = ""
	}
	if status != "" && !store.ValidTicketStatus(status) {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单状态")
		return
	}
	ticketType := strings.TrimSpace(r.URL.Query().Get("type"))
	if ticketType != "" {
		ticketType = store.NormalizeTicketType(a.store().TicketTypes(), ticketType)
	}
	priority := strings.TrimSpace(r.URL.Query().Get("priority"))
	if priority != "" {
		if !store.ValidTicketPriority(priority) {
			failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的优先级")
			return
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
	result := a.store().ListTicketsPage(filter, page, perPage)
	ok(w, "OK", map[string]any{
		"tickets":      ticketDTOs(result.Tickets),
		"total":        result.Total,
		"page":         page,
		"per_page":     perPage,
		"ticket_types": a.store().TicketTypes(),
	})
}

// handleAdminTicket 返回单个工单及完整对话。管理端接口不受 TicketSystemEnabled 开关限制。
func (a *App) handleAdminTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w) {
		return
	}
	id, _ := int64Param(params, "ticket_id")
	ticket, found := a.store().Ticket(id)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	ok(w, "OK", map[string]any{"ticket": ticketDTO(ticket), "ticket_types": a.store().TicketTypes()})
}

// handleAdminUpdateTicket 管理员更新工单状态、优先级、类型和处理摘要。
// 聊天回复必须走 POST /admin/tickets/:ticket_id/reply，避免元数据表单把双方对话覆盖或伪造成回复。
func (a *App) handleAdminUpdateTicket(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "ticket_id")
	payload := decodeMap(r)

	if a.refreshStoreForRequest(w) {
		return
	}
	existing, foundTicket := a.store().Ticket(id)
	if !foundTicket {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}

	status := strings.TrimSpace(firstNonEmpty(stringValue(payload, "status"), existing.Status))
	if !store.ValidTicketStatus(status) {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单状态")
		return
	}
	status = store.NormalizeTicketStatus(status)

	priority := strings.TrimSpace(firstNonEmpty(stringValue(payload, "priority"), existing.Priority))
	if !store.ValidTicketPriority(priority) {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的优先级")
		return
	}
	priority = store.NormalizeTicketPriority(priority)

	ticketType := strings.TrimSpace(firstNonEmpty(stringValue(payload, "type"), existing.Type))
	if !validTicketType(a.store().TicketTypes(), ticketType) {
		failWithCode(w, http.StatusBadRequest, ErrInvalidPayload, "无效的工单类型")
		return
	}
	ticketType = store.NormalizeTicketType(a.store().TicketTypes(), ticketType)
	var adminNote *string
	if _, ok := payload["admin_note"]; ok {
		value := strings.TrimSpace(stringValue(payload, "admin_note"))
		adminNote = &value
	}

	ticket, err := a.store().UpdateTicket(id, store.TicketUpdate{
		Status:    &status,
		Priority:  &priority,
		Type:      &ticketType,
		AdminNote: adminNote,
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "update_ticket", "admin", ticket.UID, map[string]any{
		"ticket_id":    ticket.ID,
		"old_status":   existing.Status,
		"new_status":   ticket.Status,
		"old_priority": existing.Priority,
		"new_priority": ticket.Priority,
		"old_type":     existing.Type,
		"new_type":     ticket.Type,
		"note_changed": adminNote != nil && strings.TrimSpace(existing.AdminNote) != ticket.AdminNote,
	})

	// 工单变动后通知工单所属用户（如果用户开启了 TG 通知）
	a.notifyTicketOwner(r.Context(), ticket, existing)
	a.notifyTicketAdmins(r.Context(), "updated", ticket, current(r).User)

	ok(w, "工单已更新", ticketDTO(ticket))
}

// handleAdminReplyTicket 追加管理员文字回复，不要求提交状态 / 类型 / 优先级表单。
func (a *App) handleAdminReplyTicket(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "ticket_id")
	if a.refreshStoreForRequest(w) {
		return
	}
	existing, foundTicket := a.store().Ticket(id)
	if !foundTicket {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	payload := decodeMap(r)
	content := strings.TrimSpace(stringValue(payload, "content"))
	if content == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "回复内容不能为空")
		return
	}
	if len(content) > 5000 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "回复内容过长（上限 5000 字符）")
		return
	}
	p := current(r)
	reply := store.TicketReply{
		UID:      p.User.UID,
		Username: p.User.Username,
		Role:     p.User.Role,
		Content:  content,
	}
	ticket, err := a.store().AddTicketReply(id, reply)
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "reply_ticket", "admin", ticket.UID, map[string]any{"ticket_id": id, "reply_len": len(content)})
	a.notifyTicketOwner(r.Context(), ticket, existing)
	a.notifyTicketAdmins(r.Context(), "admin_replied", ticket, p.User)
	ok(w, "回复成功", map[string]any{
		"ticket_id": id,
		"ticket":    ticketDTO(ticket),
		"replies":   ticketReplyDTOs(ticket.Replies),
	})
}

// handleAdminDeleteTicket 管理员删除工单。管理端接口不受 TicketSystemEnabled 开关限制。
func (a *App) handleAdminDeleteTicket(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "ticket_id")
	if a.refreshStoreForRequest(w) {
		return
	}
	existing, found := a.store().Ticket(id)
	if statusFromError(w, a.store().DeleteTicket(id)) {
		return
	}
	a.audit(r, "delete_ticket", "admin", 0, map[string]any{"ticket_id": id})
	if found {
		a.notifyTicketAdmins(r.Context(), "deleted", existing, current(r).User)
	}
	// 工单删除后立即清掉其图片目录，避免遗留孤儿附件。删除失败仅记日志，
	// 不影响工单删除结果（store 层已删除记录，文件可由清理任务兜底）。
	a.removeTicketAttachmentDir(id)
	ok(w, "工单已删除", nil)
}

// ---- 工单图片附件 ----

// ticketImageFilenamePattern 工单图片文件名白名单：随机 16 hex + 已知图片扩展名。
var ticketImageFilenamePattern = regexp.MustCompile(`^[a-f0-9]{16}\.(jpg|png|gif|webp|bmp)$`)

// ticketAttachmentDir 返回某工单的图片目录绝对路径（约束在 uploads/tickets/<id> 内）。
func (a *App) ticketAttachmentDir(ticketID int64) (string, error) {
	uploadRoot := firstNonEmpty(a.cfg().UploadDir, "uploads")
	return ResolveWithinRoot(uploadRoot, filepath.Join("tickets", strconv.FormatInt(ticketID, 10)))
}

// removeTicketAttachmentDir 删除某工单的整个图片目录。删除失败仅记日志。
func (a *App) removeTicketAttachmentDir(ticketID int64) {
	dir, err := a.ticketAttachmentDir(ticketID)
	if err != nil {
		zap.L().Warn("解析工单图片目录失败", zap.Int64("ticket_id", ticketID), zap.Error(err))
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		zap.L().Warn("删除工单图片目录失败", zap.Int64("ticket_id", ticketID), zap.Error(err))
	}
}

// ticketAccessible 校验当前用户能否访问该工单（本人或管理员）。
func (a *App) ticketAccessible(p principal, ticketID int64) (store.Ticket, bool) {
	ticket, found := a.store().Ticket(ticketID)
	if !found {
		return store.Ticket{}, false
	}
	if ticket.UID != p.User.UID && p.User.Role != store.RoleAdmin {
		return store.Ticket{}, false
	}
	return ticket, true
}

// handleUploadTicketImage 为工单上传交流图片。本人或管理员可上传。
func (a *App) handleUploadTicketImage(w http.ResponseWriter, r *http.Request, params Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	p := current(r)
	if !a.allowRate(r.Context(), rateKey("ticket-img:", p.User.UID), 30, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrTicketRateLimited, "上传过于频繁，请稍后再试")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	if a.refreshStoreForRequest(w) {
		return
	}
	ticket, allowed := a.ticketAccessible(p, id)
	if !allowed {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	// 已关闭工单冻结普通用户侧变更；管理员仍可补充排查图片。
	if !store.TicketStatusAllowsConversation(ticket.Status) && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusBadRequest, ErrTicketAlreadyClosed, "工单已关闭，无法上传图片")
		return
	}

	maxSize := cfg.TicketImageMaxSize
	if maxSize <= 0 {
		maxSize = 5 * 1024 * 1024
	}
	maxCount := cfg.TicketImageMaxCount
	if maxCount <= 0 {
		maxCount = 5
	}
	if len(ticket.Attachments) >= maxCount {
		failWithCode(w, http.StatusConflict, ErrTicketImageTooMany, fmt.Sprintf("每个工单最多上传 %d 张图片", maxCount))
		return
	}

	if err := r.ParseMultipartForm(maxSize + 1024); err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUploadInvalidPayload, "上传内容无效")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUploadFileMissing, "缺少文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrUploadInvalidPayload, "读取文件失败")
		return
	}
	if int64(len(data)) > maxSize {
		failWithCode(w, http.StatusRequestEntityTooLarge, ErrTicketImageTooLarge, fmt.Sprintf("单张图片不能超过 %d MB", maxSize/(1024*1024)))
		return
	}
	if len(data) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrTicketImageInvalid, "图片内容为空")
		return
	}
	// 通过真实内容嗅探图片类型，而不是信任扩展名 / Content-Type 头。
	contentType := strings.ToLower(strings.Split(http.DetectContentType(data), ";")[0])
	ext, okImage := uploadImageExtension(contentType)
	if !okImage {
		failWithCode(w, http.StatusBadRequest, ErrTicketImageInvalid, "只允许上传 jpg / png / gif / webp / bmp 图片")
		return
	}

	filename := randomCode(16) + ext
	if !ticketImageFilenamePattern.MatchString(filename) {
		failWithCode(w, http.StatusInternalServerError, ErrUploadSaveFailed, "保存图片失败")
		return
	}
	dir, err := a.ticketAttachmentDir(id)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrUploadDirInvalid, "图片目录无效")
		return
	}
	target, err := ResolveWithinRoot(dir, filename)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrUploadDirInvalid, "图片目录无效")
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrUploadDirCreateFailed, "创建图片目录失败")
		return
	}
	// MkdirAll 后再 lstat，挡住把目录替换成 symlink 的 TOCTOU。
	if info, lerr := os.Lstat(dir); lerr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		failWithCode(w, http.StatusInternalServerError, ErrUploadDirInvalid, "图片目录无效")
		return
	}
	if err := store.WriteFileAtomicSync(target, data, 0o600); err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrUploadSaveFailed, "保存图片失败")
		return
	}

	att := store.TicketAttachment{
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(data)),
		UploadedUID: p.User.UID,
	}
	updated, err := a.store().AddTicketAttachment(id, att, p.User.Role)
	if err != nil {
		// 落库失败则回滚已写入的文件，避免产生孤儿文件。
		_ = os.Remove(target)
		if errors.Is(err, store.ErrTicketClosed) {
			failWithCode(w, http.StatusBadRequest, ErrTicketAlreadyClosed, "工单已关闭，无法上传图片")
			return
		}
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "upload_ticket_image", auditCategoryForRole(p.User.Role), ticket.UID, map[string]any{"ticket_id": id, "filename": filename})
	if p.User.Role == store.RoleAdmin {
		a.notifyTicketOwner(r.Context(), updated, ticket)
	} else {
		a.notifyTicketAdmins(r.Context(), "image_uploaded", updated, p.User)
	}
	created(w, "图片已上传", map[string]any{
		"ticket_id":   id,
		"attachment":  ticketAttachmentDTO(id, att),
		"attachments": ticketAttachmentDTOs(id, updated.Attachments),
	})
}

// handleGetTicketImage 提供工单图片访问。本人或管理员可访问。
func (a *App) handleGetTicketImage(w http.ResponseWriter, r *http.Request, params Params) {
	p := current(r)
	id, _ := int64Param(params, "ticket_id")
	filename := params["filename"]
	if !ticketImageFilenamePattern.MatchString(filename) {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	if a.refreshStoreForRequest(w) {
		return
	}
	ticket, allowed := a.ticketAccessible(p, id)
	if !allowed {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	if !ticketHasAttachment(ticket, filename) {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	dir, err := a.ticketAttachmentDir(id)
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	target, err := ResolveWithinRoot(dir, filename)
	if err != nil {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	info, lerr := os.Lstat(target)
	if lerr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		failWithCode(w, http.StatusNotFound, ErrAssetNotFound, "resource not found")
		return
	}
	setImmutableCacheHeader(w)
	http.ServeFile(w, r, target)
}

// handleDeleteTicketImage 删除工单图片。工单关闭前本人或管理员可删除；
// 关闭后仅管理员可删除（冻结用户侧的历史图片）。
func (a *App) handleDeleteTicketImage(w http.ResponseWriter, r *http.Request, params Params) {
	cfg := a.cfg()
	if !cfg.TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	p := current(r)
	id, _ := int64Param(params, "ticket_id")
	filename := params["filename"]
	if !ticketImageFilenamePattern.MatchString(filename) {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "图片不存在")
		return
	}
	if a.refreshStoreForRequest(w) {
		return
	}
	ticket, allowed := a.ticketAccessible(p, id)
	if !allowed {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	// 工单关闭后冻结历史图片：普通用户不能再删除自己的图片，仅管理员可清理。
	if !store.TicketStatusAllowsConversation(ticket.Status) && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrTicketAlreadyClosed, "工单已关闭，无法删除图片")
		return
	}
	if !ticketHasAttachment(ticket, filename) {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "图片不存在")
		return
	}
	updated, err := a.store().RemoveTicketAttachment(id, filename, p.User.Role)
	if errors.Is(err, store.ErrTicketClosed) {
		failWithCode(w, http.StatusForbidden, ErrTicketAlreadyClosed, "工单已关闭，无法删除图片")
		return
	}
	if statusFromError(w, err) {
		return
	}
	// 落库成功后再删文件；删文件失败仅记日志，清理任务可兜底。
	if dir, derr := a.ticketAttachmentDir(id); derr == nil {
		if target, terr := ResolveWithinRoot(dir, filename); terr == nil {
			if rerr := os.Remove(target); rerr != nil && !os.IsNotExist(rerr) {
				zap.L().Warn("删除工单图片文件失败", zap.Int64("ticket_id", id), zap.String("filename", filename), zap.Error(rerr))
			}
		}
	}
	a.audit(r, "delete_ticket_image", auditCategoryForRole(p.User.Role), ticket.UID, map[string]any{"ticket_id": id, "filename": filename})
	if p.User.Role != store.RoleAdmin {
		a.notifyTicketAdmins(r.Context(), "image_deleted", updated, p.User)
	} else {
		a.notifyTicketOwner(r.Context(), updated, ticket)
	}
	ok(w, "图片已删除", map[string]any{
		"ticket_id":   id,
		"attachments": ticketAttachmentDTOs(id, updated.Attachments),
	})
}

// handleReplyToTicket 用户或管理员向工单追加文字回复。每次回复独立追加到 Replies 数组中。
func (a *App) handleReplyToTicket(w http.ResponseWriter, r *http.Request, params Params) {
	if !a.cfg().TicketSystemEnabled {
		failWithCode(w, http.StatusServiceUnavailable, ErrTicketDisabled, "工单系统未启用")
		return
	}
	p := current(r)
	if !a.allowRate(r.Context(), rateKey("ticket-reply:uid:", p.User.UID), 20, 10*time.Minute) {
		failWithCode(w, http.StatusTooManyRequests, ErrTicketRateLimited, "回复过于频繁，请稍后再试")
		return
	}
	id, _ := int64Param(params, "ticket_id")
	if a.refreshStoreForRequest(w) {
		return
	}
	ticket, okTicket := a.store().Ticket(id)
	if !okTicket {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	if ticket.UID != p.User.UID && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "无权回复此工单")
		return
	}
	if !store.TicketStatusAllowsConversation(ticket.Status) && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusBadRequest, ErrTicketAlreadyClosed, "工单已关闭，无法回复")
		return
	}
	payload := decodeMap(r)
	content := strings.TrimSpace(stringValue(payload, "content"))
	if content == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "回复内容不能为空")
		return
	}
	if len(content) > 5000 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "回复内容过长（上限 5000 字符）")
		return
	}
	reply := store.TicketReply{
		UID:      p.User.UID,
		Username: p.User.Username,
		Role:     p.User.Role,
		Content:  content,
	}
	updated, err := a.store().AddTicketReply(id, reply)
	if errors.Is(err, store.ErrTicketClosed) {
		failWithCode(w, http.StatusBadRequest, ErrTicketAlreadyClosed, "工单已关闭，无法回复")
		return
	}
	if statusFromError(w, err) {
		return
	}
	category := auditCategoryForRole(p.User.Role)
	a.audit(r, "reply_ticket", category, ticket.UID, map[string]any{"ticket_id": id, "reply_len": len(content)})
	if p.User.Role == store.RoleAdmin {
		a.notifyTicketOwner(r.Context(), updated, ticket)
	} else {
		a.notifyTicketAdmins(r.Context(), "replied", updated, p.User)
	}
	ok(w, "回复成功", map[string]any{
		"ticket_id": id,
		"ticket":    ticketDTO(updated),
		"replies":   ticketReplyDTOs(updated.Replies),
	})
}

func ticketHasAttachment(ticket store.Ticket, filename string) bool {
	for _, att := range ticket.Attachments {
		if att.Filename == filename {
			return true
		}
	}
	return false
}

func ticketImageURL(ticketID int64, filename string) string {
	return "/api/v1/tickets/" + strconv.FormatInt(ticketID, 10) + "/images/" + filename
}

func ticketAttachmentDTO(ticketID int64, att store.TicketAttachment) map[string]any {
	return map[string]any{
		"filename":     att.Filename,
		"url":          ticketImageURL(ticketID, att.Filename),
		"content_type": att.ContentType,
		"size":         att.Size,
		"uploaded_uid": att.UploadedUID,
		"created_at":   att.CreatedAt,
	}
}

func ticketAttachmentDTOs(ticketID int64, atts []store.TicketAttachment) []map[string]any {
	out := make([]map[string]any, 0, len(atts))
	for _, att := range atts {
		out = append(out, ticketAttachmentDTO(ticketID, att))
	}
	return out
}

func ticketReplyDTO(reply store.TicketReply) map[string]any {
	return map[string]any{
		"uid":        reply.UID,
		"username":   reply.Username,
		"role":       reply.Role,
		"author":     ticketReplyAuthor(reply.Role),
		"content":    reply.Content,
		"created_at": reply.CreatedAt,
	}
}

func ticketReplyDTOs(replies []store.TicketReply) []map[string]any {
	out := make([]map[string]any, 0, len(replies))
	for _, reply := range replies {
		out = append(out, ticketReplyDTO(reply))
	}
	return out
}

func ticketReplyAuthor(role int) string {
	if role == store.RoleAdmin {
		return "admin"
	}
	return "user"
}

// ticketDTO 把单个工单序列化为响应 map，并为每张附件补上可访问的 url 字段。
func ticketDTO(t store.Ticket) map[string]any {
	notifyTelegram := true
	if t.NotifyTelegram != nil {
		notifyTelegram = *t.NotifyTelegram
	}
	dto := map[string]any{
		"id":              t.ID,
		"uid":             t.UID,
		"username":        t.Username,
		"title":           t.Title,
		"content":         t.Content,
		"type":            t.Type,
		"status":          t.Status,
		"priority":        t.Priority,
		"admin_note":      t.AdminNote,
		"replies":         ticketReplyDTOs(t.Replies),
		"attachments":     ticketAttachmentDTOs(t.ID, t.Attachments),
		"notify_telegram": notifyTelegram,
		"created_at":      t.CreatedAt,
		"updated_at":      t.UpdatedAt,
		"resolved_at":     t.ResolvedAt,
		"closed_at":       t.ClosedAt,
	}
	return dto
}

// ticketDTOs 批量序列化工单列表。
func ticketDTOs(tickets []store.Ticket) []map[string]any {
	out := make([]map[string]any, 0, len(tickets))
	for _, t := range tickets {
		out = append(out, ticketDTO(t))
	}
	return out
}

func auditCategoryForRole(role int) string {
	if role == store.RoleAdmin {
		return "admin"
	}
	return "user"
}

// ---- 校验工具 ----

func validTicketType(types []string, input string) bool {
	for _, t := range types {
		if strings.EqualFold(t, input) {
			return true
		}
	}
	return false
}

// ---- 工单类型管理 ----

func (a *App) handleAdminTicketTypes(w http.ResponseWriter, r *http.Request, _ Params) {
	types := a.store().TicketTypes()
	ok(w, "OK", map[string]any{"types": types})
}

func (a *App) handleAdminAddTicketType(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	name := strings.TrimSpace(stringValue(payload, "name"))
	if name == "" || len(name) > 50 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "类型名称需为 1-50 个字符")
		return
	}
	if err := a.store().AddTicketType(name); statusFromError(w, err) {
		return
	}
	a.persistTicketTypesFromStore()
	a.audit(r, "add_ticket_type", "admin", 0, map[string]any{"name": name})
	ok(w, "类型已添加", map[string]any{"name": name, "types": a.store().TicketTypes()})
}

func (a *App) handleAdminDeleteTicketType(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	name := strings.TrimSpace(stringValue(payload, "name"))
	if name == "" {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "类型名称不能为空")
		return
	}
	if err := a.store().DeleteTicketType(name); statusFromError(w, err) {
		return
	}
	a.persistTicketTypesFromStore()
	a.audit(r, "delete_ticket_type", "admin", 0, map[string]any{"name": name})
	ok(w, "类型已删除", map[string]any{"name": name, "types": a.store().TicketTypes()})
}

func (a *App) handleAdminRenameTicketType(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	oldName := strings.TrimSpace(stringValue(payload, "old_name"))
	newName := strings.TrimSpace(stringValue(payload, "new_name"))
	if oldName == "" || newName == "" || len(newName) > 50 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "类型名称需为 1-50 个字符")
		return
	}
	if strings.EqualFold(oldName, newName) {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "新旧名称相同")
		return
	}
	count, err := a.store().RenameTicketType(oldName, newName)
	if statusFromError(w, err) {
		return
	}
	a.persistTicketTypesFromStore()
	a.audit(r, "rename_ticket_type", "admin", 0, map[string]any{"old": oldName, "new": newName, "tickets_renamed": count})
	ok(w, "类型已重命名", map[string]any{"old_name": oldName, "new_name": newName, "types": a.store().TicketTypes()})
}

func (a *App) persistTicketTypesFromStore() {
	values := configValues(*a.cfg())
	if values["Ticket"] == nil {
		values["Ticket"] = map[string]any{}
	}
	values["Ticket"]["types"] = a.store().TicketTypes()
	if _, status, message := a.saveConfigContent(renderConfigTOML(values)); status != http.StatusOK {
		zap.L().Warn("failed to persist ticket types to config.toml", zap.Int("status", status), zap.String("message", message))
	}
}

const ticketNotificationTimeout = 12 * time.Second

type ticketNotificationResult struct {
	Targets  int
	Failures int
	Skipped  string
}

func (a *App) enqueueTicketNotification(scope, event string, ticketID, targetUID int64, send func(context.Context) ticketNotificationResult) {
	if send == nil {
		return
	}
	// Telegram 未配置时不存在任何可送达对象：直接不启协程，避免每次工单动作都
	// 空跑一个「已跳过」通知。JSON 文件模式下 zap→runtime log sink 对每条日志都做
	// 整份 state 落盘，这类非事件日志会平白触发一次全量状态重写（PG 模式为廉价
	// INSERT）；后台协程对 state 的异步写在测试里还会与 t.TempDir() 清理竞争。
	if !a.telegramAvailable() {
		return
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				zap.L().Warn("工单 Telegram 通知任务异常", zap.Int64("ticket_id", ticketID), zap.String("scope", scope), zap.String("event", event), zap.String("panic", redactSensitiveText(fmt.Sprint(recovered))))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), ticketNotificationTimeout)
		defer cancel()
		result := send(ctx)
		fields := []zap.Field{
			zap.Int64("ticket_id", ticketID),
			zap.Int64("target_uid", targetUID),
			zap.String("scope", scope),
			zap.String("event", event),
			zap.Int("targets", result.Targets),
			zap.Int("failures", result.Failures),
		}
		// 通知发送是对外副作用而非状态变更：结果只进 zap 运行日志，不写审计库。
		// 否则未配置 Telegram 的实例每次工单动作都会因 skip 触发一次全量 state
		// 落盘，并把有界审计日志刷满"通知已跳过"噪声、挤掉真正的变更记录。
		switch {
		case result.Skipped != "":
			zap.L().Info("工单 Telegram 通知已跳过", append(fields, zap.String("skipped", result.Skipped))...)
		case result.Failures > 0:
			zap.L().Warn("工单 Telegram 通知部分失败", fields...)
		default:
			zap.L().Info("工单 Telegram 通知已发送", fields...)
		}
	}()
}

// notifyTicketAdmins 在新工单 / 工单变动后向已在个人设置中开启工单 TG 通知的管理员推送。
func (a *App) notifyTicketAdmins(ctx context.Context, event string, ticket store.Ticket, actor store.User) {
	a.enqueueTicketNotification("admin", event, ticket.ID, ticket.UID, func(sendCtx context.Context) ticketNotificationResult {
		if !a.telegramAvailable() {
			return ticketNotificationResult{Skipped: "telegram_unavailable"}
		}
		admins := a.ticketAdminNotificationTargets(actor)
		if len(admins) == 0 {
			return ticketNotificationResult{Skipped: "no_admin_targets"}
		}
		text := a.ticketAdminNotificationText(event, ticket, actor)
		var photoName, photoType string
		var photoData []byte
		if event == "image_uploaded" {
			photoName, photoType, photoData = a.ticketNotificationPhoto(ticket)
		}
		result := ticketNotificationResult{Targets: len(admins)}
		for _, admin := range admins {
			if len(photoData) > 0 {
				if err := a.telegramSendPhoto(sendCtx, admin.TelegramID, photoName, photoType, photoData, text); err == nil {
					continue
				} else {
					result.Failures++
					zap.L().Warn("发送管理员工单图片通知失败，降级为纯文本", zap.Int64("ticket_id", ticket.ID), zap.Int64("admin_uid", admin.UID), zap.Error(err))
				}
			}
			if err := a.telegramSendPlainMessage(sendCtx, admin.TelegramID, text); err != nil {
				result.Failures++
				zap.L().Warn("发送管理员工单通知失败", zap.Int64("ticket_id", ticket.ID), zap.Int64("admin_uid", admin.UID), zap.Error(err))
			}
		}
		return result
	})
}

func (a *App) ticketAdminNotificationTargets(actor store.User) []store.User {
	admins := make([]store.User, 0)
	for _, user := range a.store().ListUsers() {
		if user.Role != store.RoleAdmin || !user.Active || !user.NotifyOnTicketTelegram || user.TelegramID == 0 {
			continue
		}
		if actor.Role == store.RoleAdmin && actor.UID != 0 && user.UID == actor.UID {
			continue
		}
		admins = append(admins, user)
	}
	return admins
}

func (a *App) ticketAdminNotificationText(event string, ticket store.Ticket, actor store.User) string {
	lines := []string{
		"工单通知 - " + ticketEventLabel(event),
		"",
		"站点：" + a.cfg().AppName,
		"工单：#" + strconv.FormatInt(ticket.ID, 10) + " " + strings.TrimSpace(ticket.Title),
		"状态：" + statusLabel(ticket.Status),
		"优先级：" + ticket.Priority,
		"类型：" + ticket.Type,
		"提交人：" + ticket.Username + " (UID " + strconv.FormatInt(ticket.UID, 10) + ")",
	}
	if actor.UID != 0 {
		lines = append(lines, "操作者："+actor.Username+" (UID "+strconv.FormatInt(actor.UID, 10)+")")
	}
	if label, summary := ticketAdminNotificationSummary(event, ticket); summary != "" {
		lines = append(lines, "", label+"："+truncateString(summary, 220))
	}
	if len(ticket.Attachments) > 0 {
		lines = append(lines, "", "附件图片："+strconv.Itoa(len(ticket.Attachments))+" 张")
	}
	lines = append(lines, "时间："+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
}

func ticketAdminNotificationSummary(event string, ticket store.Ticket) (string, string) {
	switch event {
	case "replied":
		if reply, ok := latestTicketReplyByRole(ticket, store.RoleNormal); ok {
			return "用户回复摘要", reply.Content
		}
	case "admin_replied":
		if reply, ok := latestTicketReplyByRole(ticket, store.RoleAdmin); ok {
			return "管理员回复摘要", reply.Content
		}
	case "updated":
		if note := strings.TrimSpace(ticket.AdminNote); note != "" {
			return "处理摘要", note
		}
	}
	if content := strings.TrimSpace(ticket.Content); content != "" {
		return "内容摘要", content
	}
	return "", ""
}

func latestTicketReplyByRole(ticket store.Ticket, role int) (store.TicketReply, bool) {
	for i := len(ticket.Replies) - 1; i >= 0; i-- {
		reply := ticket.Replies[i]
		if reply.Role == role && strings.TrimSpace(reply.Content) != "" {
			return reply, true
		}
	}
	return store.TicketReply{}, false
}

func ticketEventLabel(event string) string {
	switch event {
	case "created":
		return "新工单"
	case "updated":
		return "工单更新"
	case "closed":
		return "用户关闭"
	case "reopened":
		return "用户重开"
	case "image_uploaded":
		return "新增图片"
	case "image_deleted":
		return "删除图片"
	case "replied":
		return "用户回复"
	case "admin_replied":
		return "管理员回复"
	case "deleted":
		return "工单删除"
	default:
		return "工单变动"
	}
}

func (a *App) ticketNotificationPhoto(ticket store.Ticket) (string, string, []byte) {
	if len(ticket.Attachments) == 0 {
		return "", "", nil
	}
	att := ticket.Attachments[len(ticket.Attachments)-1]
	if !ticketImageFilenamePattern.MatchString(att.Filename) {
		return "", "", nil
	}
	dir, err := a.ticketAttachmentDir(ticket.ID)
	if err != nil {
		return "", "", nil
	}
	target, err := ResolveWithinRoot(dir, att.Filename)
	if err != nil {
		return "", "", nil
	}
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 10*1024*1024 {
		return "", "", nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", "", nil
	}
	return att.Filename, att.ContentType, data
}

// notifyTicketOwner 工单变动后向工单所属用户发送 Telegram 通知。
func (a *App) notifyTicketOwner(ctx context.Context, updated, existing store.Ticket) {
	a.enqueueTicketNotification("owner", "updated", updated.ID, updated.UID, func(sendCtx context.Context) ticketNotificationResult {
		owner, found := a.store().User(updated.UID)
		if !found {
			return ticketNotificationResult{Skipped: "owner_missing"}
		}
		if !a.telegramAvailable() || owner.TelegramID == 0 {
			return ticketNotificationResult{Skipped: "telegram_unavailable"}
		}
		// 优先使用工单级别的通知设置，未设置时回退到用户全局设置
		notify := owner.NotifyOnTicketTelegram
		if updated.NotifyTelegram != nil {
			notify = *updated.NotifyTelegram
		}
		if !notify {
			return ticketNotificationResult{Skipped: "owner_disabled"}
		}
		now := time.Now().Format("2006-01-02 15:04:05")
		adminNote := ticketOwnerNotificationNote(updated, existing)
		adminNoteContent := ""
		if adminNote != "" {
			adminNoteContent = "回复内容：\n" + adminNote
		}
		notifValues := map[string]string{
			"{ticket_id}":          strconv.FormatInt(updated.ID, 10),
			"{title}":              updated.Title,
			"{status}":             statusLabel(updated.Status),
			"{priority}":           updated.Priority,
			"{type}":               updated.Type,
			"{admin_note}":         adminNote,
			"{admin_note_content}": adminNoteContent,
			"{time}":               now,
			"{server_name}":        a.cfg().AppName,
		}
		tmpl := a.cfg().TicketNotifyTelegramTemplate
		if tmpl == "" {
			tmpl = config.DefaultTicketNotifyTelegramTemplate
		}
		text := replaceNotifPlaceholders(tmpl, notifValues)
		result := ticketNotificationResult{Targets: 1}
		if err := a.telegramSendPlainMessage(sendCtx, owner.TelegramID, text); err != nil {
			result.Failures = 1
			zap.L().Warn("发送工单变动通知失败", zap.Int64("ticket_id", updated.ID), zap.Int64("uid", owner.UID), zap.Error(err))
		}
		return result
	})
}

func ticketOwnerNotificationNote(updated, existing store.Ticket) string {
	if len(updated.Replies) > len(existing.Replies) {
		for i := len(updated.Replies) - 1; i >= len(existing.Replies); i-- {
			reply := updated.Replies[i]
			if reply.Role == store.RoleAdmin && strings.TrimSpace(reply.Content) != "" {
				return strings.TrimSpace(reply.Content)
			}
		}
	}
	if strings.TrimSpace(updated.AdminNote) != strings.TrimSpace(existing.AdminNote) {
		return strings.TrimSpace(updated.AdminNote)
	}
	return ""
}

// statusLabel 返回工单状态的中文描述。
func statusLabel(status string) string {
	switch status {
	case store.TicketStatusOpen:
		return "待处理"
	case store.TicketStatusInProgress:
		return "处理中"
	case store.TicketStatusResolved:
		return "已解决"
	case store.TicketStatusClosed:
		return "已关闭"
	}
	return status
}
