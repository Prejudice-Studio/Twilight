package api

import (
	"context"
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
	// 并发上限校验：服务端硬门，不仅靠前端隐藏入口。0 表示不限。
	if cfg.TicketUserOpenLimit > 0 && a.store().CountUserOpenTickets(p.User.UID) >= cfg.TicketUserOpenLimit {
		failWithCode(w, http.StatusConflict, ErrTicketUserLimit, "您当前待处理 / 处理中的工单已达上限，请先关闭部分工单后再提交")
		return
	}
	if cfg.TicketGlobalOpenLimit > 0 && a.store().CountOpenTickets() >= cfg.TicketGlobalOpenLimit {
		failWithCode(w, http.StatusConflict, ErrTicketGlobalLimit, "系统当前待处理 / 处理中的工单已达上限，请稍后再提交")
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

	ticket, err := a.store().UpsertTicket(store.Ticket{
		UID:            p.User.UID,
		Username:       p.User.Username,
		Title:          title,
		Content:        content,
		Type:           ticketType,
		Priority:       priority,
		Status:         store.TicketStatusOpen,
		NotifyTelegram: notifyTG,
	})
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "create_ticket", "user", 0, map[string]any{"ticket_id": ticket.ID, "type": ticketType, "priority": priority})
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
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	showAll := r.URL.Query().Get("all") == "1"
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
	tickets := a.store().ListTickets(filter)
	ok(w, "OK", map[string]any{"tickets": ticketDTOs(tickets), "total": len(tickets), "ticket_types": a.store().TicketTypes()})
}

// handleAdminUpdateTicket 管理员更新工单状态 / 回复。管理端接口不受 TicketSystemEnabled 开关限制。
func (a *App) handleAdminUpdateTicket(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "ticket_id")
	payload := decodeMap(r)

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
	if adminNote != nil && *adminNote != "" && *adminNote != strings.TrimSpace(existing.AdminNote) {
		reply := store.TicketReply{
			UID:      current(r).User.UID,
			Username: current(r).User.Username,
			Role:     current(r).User.Role,
			Content:  *adminNote,
		}
		ticket, err = a.store().AddTicketReply(id, reply)
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "update_ticket", "admin", ticket.UID, map[string]any{"ticket_id": ticket.ID, "new_status": status})

	// 工单变动后通知工单所属用户（如果用户开启了 TG 通知）
	a.notifyTicketOwner(r.Context(), ticket, existing)
	a.notifyTicketAdmins(r.Context(), "updated", ticket, current(r).User)

	ok(w, "工单已更新", ticketDTO(ticket))
}

// handleAdminDeleteTicket 管理员删除工单。管理端接口不受 TicketSystemEnabled 开关限制。
func (a *App) handleAdminDeleteTicket(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "ticket_id")
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
	ticket, allowed := a.ticketAccessible(p, id)
	if !allowed {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	// 已关闭工单不再允许追加图片，避免清理任务删除目录后又被写入。
	if !store.TicketStatusAllowsConversation(ticket.Status) {
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
	updated, err := a.store().AddTicketAttachment(id, att)
	if err != nil {
		// 落库失败则回滚已写入的文件，避免产生孤儿文件。
		_ = os.Remove(target)
		if statusFromError(w, err) {
			return
		}
	}
	a.audit(r, "upload_ticket_image", auditCategoryForRole(p.User.Role), ticket.UID, map[string]any{"ticket_id": id, "filename": filename})
	a.notifyTicketAdmins(r.Context(), "image_uploaded", updated, p.User)
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
	updated, err := a.store().RemoveTicketAttachment(id, filename)
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
	ticket, okTicket := a.store().Ticket(id)
	if !okTicket {
		failWithCode(w, http.StatusNotFound, ErrTicketNotFound, "工单不存在")
		return
	}
	if ticket.UID != p.User.UID && p.User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrForbidden, "无权回复此工单")
		return
	}
	if !store.TicketStatusAllowsConversation(ticket.Status) {
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

// notifyTicketAdmins 在新工单 / 工单变动后向已在个人设置中开启工单 TG 通知的管理员推送。
func (a *App) notifyTicketAdmins(ctx context.Context, event string, ticket store.Ticket, actor store.User) {
	if !a.telegramAvailable() {
		return
	}
	admins := make([]store.User, 0)
	for _, user := range a.store().ListUsers() {
		if user.Role != store.RoleAdmin || !user.NotifyOnTicketTelegram || user.TelegramID == 0 {
			continue
		}
		admins = append(admins, user)
	}
	if len(admins) == 0 {
		return
	}
	text := a.ticketAdminNotificationText(event, ticket, actor)
	photoName, photoType, photoData := a.ticketNotificationPhoto(ticket)
	for _, admin := range admins {
		if len(photoData) > 0 {
			if err := a.telegramSendPhoto(ctx, admin.TelegramID, photoName, photoType, photoData, text); err == nil {
				continue
			} else {
				zap.L().Warn("发送管理员工单图片通知失败，降级为纯文本", zap.Int64("ticket_id", ticket.ID), zap.Int64("admin_uid", admin.UID), zap.Error(err))
			}
		}
		if err := a.telegramSendPlainMessage(ctx, admin.TelegramID, text); err != nil {
			zap.L().Warn("发送管理员工单通知失败", zap.Int64("ticket_id", ticket.ID), zap.Int64("admin_uid", admin.UID), zap.Error(err))
		}
	}
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
	if note := strings.TrimSpace(ticket.AdminNote); note != "" {
		lines = append(lines, "", "回复摘要："+truncateString(note, 220))
	} else if content := strings.TrimSpace(ticket.Content); content != "" {
		lines = append(lines, "", "内容摘要："+truncateString(content, 220))
	}
	if len(ticket.Attachments) > 0 {
		lines = append(lines, "", "附件图片："+strconv.Itoa(len(ticket.Attachments))+" 张")
	}
	lines = append(lines, "时间："+time.Now().Format("2006-01-02 15:04:05"))
	return strings.Join(lines, "\n")
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
	case "replied":
		return "用户回复"
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
	owner, found := a.store().User(updated.UID)
	if !found {
		return
	}
	if !a.telegramAvailable() || owner.TelegramID == 0 {
		return
	}
	// 优先使用工单级别的通知设置，未设置时回退到用户全局设置
	notify := owner.NotifyOnTicketTelegram
	if updated.NotifyTelegram != nil {
		notify = *updated.NotifyTelegram
	}
	if !notify {
		return
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	adminNote := strings.TrimSpace(updated.AdminNote)
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
	if err := a.telegramSendPlainMessage(ctx, owner.TelegramID, text); err != nil {
		zap.L().Warn("发送工单变动通知失败", zap.Int64("ticket_id", updated.ID), zap.Int64("uid", owner.UID), zap.Error(err))
	}
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
