package store

import (
	"sort"
	"strings"
	"time"
)

// ---- Tickets ----

func NormalizeTicketStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case TicketStatusInProgress:
		return TicketStatusInProgress
	case TicketStatusResolved:
		return TicketStatusResolved
	case TicketStatusClosed:
		return TicketStatusClosed
	default:
		return TicketStatusOpen
	}
}

func ValidTicketStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case TicketStatusOpen, TicketStatusInProgress, TicketStatusResolved, TicketStatusClosed:
		return true
	}
	return false
}

func NormalizeTicketPriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case TicketPriorityLow:
		return TicketPriorityLow
	case TicketPriorityHigh:
		return TicketPriorityHigh
	case TicketPriorityUrgent:
		return TicketPriorityUrgent
	default:
		return TicketPriorityMedium
	}
}

func ValidTicketPriority(priority string) bool {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh, TicketPriorityUrgent:
		return true
	}
	return false
}

func NormalizeTicketType(types []string, input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		trimmed = TicketTypeDefault
	}
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), trimmed) {
			return strings.TrimSpace(t)
		}
	}
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), TicketTypeDefault) {
			return strings.TrimSpace(t)
		}
	}
	if len(types) > 0 {
		if first := strings.TrimSpace(types[0]); first != "" {
			return first
		}
	}
	return TicketTypeDefault
}

func TicketStatusOpenForQuota(status string) bool {
	switch NormalizeTicketStatus(status) {
	case TicketStatusOpen, TicketStatusInProgress:
		return true
	}
	return false
}

func TicketStatusAllowsConversation(status string) bool {
	return NormalizeTicketStatus(status) != TicketStatusClosed
}

func applyTicketStatusTimestamps(t *Ticket, existing Ticket, now int64) {
	previousStatus := NormalizeTicketStatus(existing.Status)
	nextStatus := NormalizeTicketStatus(t.Status)
	t.Status = nextStatus
	if t.ResolvedAt == 0 {
		t.ResolvedAt = existing.ResolvedAt
	}
	if t.ClosedAt == 0 {
		t.ClosedAt = existing.ClosedAt
	}
	if nextStatus == TicketStatusResolved {
		if previousStatus != TicketStatusResolved || t.ResolvedAt == 0 {
			t.ResolvedAt = now
		}
	} else {
		t.ResolvedAt = 0
	}
	if nextStatus == TicketStatusClosed {
		if previousStatus != TicketStatusClosed || t.ClosedAt == 0 {
			t.ClosedAt = now
		}
	} else {
		t.ClosedAt = 0
	}
}

func (s *Store) CreateTicket(t Ticket, userOpenLimit, globalOpenLimit int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.mutateAndSaveLocked(func() error {
		if userOpenLimit > 0 && s.countOpenTicketsLocked(t.UID) >= userOpenLimit {
			return ErrTicketUserOpenLimit
		}
		if globalOpenLimit > 0 && s.countOpenTicketsLocked(0) >= globalOpenLimit {
			return ErrTicketGlobalOpenLimit
		}
		t.ID = 0
		if strings.TrimSpace(t.Status) == "" {
			t.Status = TicketStatusOpen
		}
		t = s.upsertTicketLocked(t, time.Now().Unix())
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return t, nil
}

func (s *Store) upsertTicketLocked(t Ticket, now int64) Ticket {
	t.Type = NormalizeTicketType(s.state.TicketTypes, t.Type)
	t.Status = NormalizeTicketStatus(t.Status)
	t.Priority = NormalizeTicketPriority(t.Priority)
	if t.ID == 0 {
		t.ID = s.state.NextTicketID
		s.state.NextTicketID++
		t.CreatedAt = now
	} else if existing, ok := s.state.Tickets[t.ID]; ok {
		if t.CreatedAt == 0 {
			t.CreatedAt = existing.CreatedAt
		}
		if t.UID == 0 {
			t.UID = existing.UID
		}
		if t.Username == "" {
			t.Username = existing.Username
		}
		// 附件由专用方法维护，普通 upsert 不携带附件时保留原有列表，
		// 避免管理员更新状态 / 用户改内容时把交流图片清空。
		if t.Attachments == nil {
			t.Attachments = existing.Attachments
		}
		if t.Replies == nil {
			t.Replies = existing.Replies
		}
		// NotifyTelegram nil 表示沿用已有值，避免更新其他字段时意外重置。
		if t.NotifyTelegram == nil {
			t.NotifyTelegram = existing.NotifyTelegram
		}
		applyTicketStatusTimestamps(&t, existing, now)
	} else {
		empty := Ticket{Status: TicketStatusOpen}
		applyTicketStatusTimestamps(&t, empty, now)
	}
	t.UpdatedAt = now
	s.state.Tickets[t.ID] = t
	return t
}

func (s *Store) ListTickets(filter TicketFilter) []Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Ticket, 0)
	for _, t := range s.state.Tickets {
		if ticketMatchesFilter(t, filter) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) CountTickets(filter TicketFilter) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, t := range s.state.Tickets {
		if ticketMatchesFilter(t, filter) {
			count++
		}
	}
	return count
}

func ticketMatchesFilter(t Ticket, filter TicketFilter) bool {
	if filter.UID > 0 && t.UID != filter.UID {
		return false
	}
	if filter.Status != "" && t.Status != filter.Status {
		return false
	}
	if filter.Type != "" && t.Type != filter.Type {
		return false
	}
	if filter.Priority != "" && t.Priority != filter.Priority {
		return false
	}
	if filter.ActiveOnly && !TicketStatusOpenForQuota(t.Status) {
		return false
	}
	return true
}

type TicketFilter struct {
	UID        int64
	Status     string
	Type       string
	Priority   string
	ActiveOnly bool
}

func (s *Store) Ticket(id int64) (Ticket, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.state.Tickets[id]
	return t, ok
}

func (s *Store) DeleteTicket(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if _, ok := s.state.Tickets[id]; !ok {
			return ErrNotFound
		}
		delete(s.state.Tickets, id)
		return nil
	})
}

// CountUserOpenTickets 统计某用户当前处于待处理 / 处理中的工单数量。
func (s *Store) CountUserOpenTickets(uid int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countOpenTicketsLocked(uid)
}

// CountOpenTickets 统计全局处于待处理 / 处理中的工单数量。
func (s *Store) CountOpenTickets() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countOpenTicketsLocked(0)
}

func (s *Store) countOpenTicketsLocked(uid int64) int {
	count := 0
	for _, t := range s.state.Tickets {
		if (uid == 0 || t.UID == uid) && TicketStatusOpenForQuota(t.Status) {
			count++
		}
	}
	return count
}

func (s *Store) UpdateTicket(ticketID int64, patch TicketUpdate) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		now := time.Now().Unix()
		existing := t
		if patch.Status != nil {
			t.Status = NormalizeTicketStatus(*patch.Status)
		}
		if patch.Priority != nil {
			t.Priority = NormalizeTicketPriority(*patch.Priority)
		}
		if patch.Type != nil {
			t.Type = NormalizeTicketType(s.state.TicketTypes, *patch.Type)
		}
		if patch.AdminNote != nil {
			t.AdminNote = strings.TrimSpace(*patch.AdminNote)
		}
		applyTicketStatusTimestamps(&t, existing, now)
		if patch.Reply != nil {
			applyTicketReplyLocked(&t, *patch.Reply, now)
		}
		t.UpdatedAt = now
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// ReopenTicket 用户重开自己已关闭的工单：把「已关闭 → 待处理」的状态翻转与开单配额复核
// 放在同一把锁内原子完成，堵住「关掉再重开」绕过 user/global open limit 的漏洞。
// 单纯走 UpdateTicket 改状态不复核配额，用户可先关满上限外的旧单、逐个重开越过上限。
// 传入 userOpenLimit/globalOpenLimit 语义与 CreateTicket 一致（<=0 表示不限）。
// 复核时目标工单仍为 Closed 态，不计入 countOpenTicketsLocked，故与建单同为「新增一个开态」口径。
func (s *Store) ReopenTicket(ticketID, uid int64, userOpenLimit, globalOpenLimit int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if uid != 0 && t.UID != uid {
			return ErrNotFound
		}
		if NormalizeTicketStatus(t.Status) != TicketStatusClosed {
			return ErrTicketNotClosed
		}
		if userOpenLimit > 0 && s.countOpenTicketsLocked(t.UID) >= userOpenLimit {
			return ErrTicketUserOpenLimit
		}
		if globalOpenLimit > 0 && s.countOpenTicketsLocked(0) >= globalOpenLimit {
			return ErrTicketGlobalOpenLimit
		}
		now := time.Now().Unix()
		existing := t
		t.Status = TicketStatusOpen
		applyTicketStatusTimestamps(&t, existing, now)
		t.UpdatedAt = now
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

func (s *Store) SetTicketNotify(ticketID int64, enabled bool) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		t.NotifyTelegram = &enabled
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// AddTicketAttachment 给工单追加一张图片元数据。返回更新后的工单。
func (s *Store) AddTicketAttachment(ticketID int64, att TicketAttachment, actorRole int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && actorRole != RoleAdmin {
			return ErrTicketClosed
		}
		if att.CreatedAt == 0 {
			att.CreatedAt = time.Now().Unix()
		}
		t.Attachments = append(t.Attachments, att)
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// AddTicketReply 向工单追加一条回复。
func (s *Store) AddTicketReply(ticketID int64, reply TicketReply) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && reply.Role != RoleAdmin {
			return ErrTicketClosed
		}
		now := time.Now().Unix()
		applyTicketReplyLocked(&t, reply, now)
		t.UpdatedAt = now
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

func applyTicketReplyLocked(t *Ticket, reply TicketReply, now int64) {
	if reply.CreatedAt == 0 {
		reply.CreatedAt = now
	}
	reply.Content = strings.TrimSpace(reply.Content)
	t.Replies = append(t.Replies, reply)
	if reply.Role == RoleAdmin && NormalizeTicketStatus(t.Status) == TicketStatusOpen {
		t.Status = TicketStatusInProgress
	}
	// AdminNote 是「处理备注」这一独立元数据字段（仅经 UpdateTicket 显式写入），
	// 不再被管理员聊天回复顺手覆盖：此前每条 admin 回复都把 AdminNote 改成回复正文，
	// 与前端草稿保存冲突——管理员回一句话就把处理备注冲掉，且发送回复触发的 setTicket
	// 会把未保存的备注草稿一并重置，造成「回复与聊天信息互相覆盖 / 部分消失」。
	// 聊天正文已完整存在于 t.Replies，AdminNote 与它彻底解耦后互不干扰。
	if reply.Role != RoleAdmin && NormalizeTicketStatus(t.Status) == TicketStatusResolved {
		t.Status = TicketStatusOpen
		t.ResolvedAt = 0
	}
}

// RemoveTicketAttachment 从工单移除指定文件名的图片元数据。返回更新后的工单。
func (s *Store) RemoveTicketAttachment(ticketID int64, filename string, actorRole int) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out Ticket
	err := s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if !TicketStatusAllowsConversation(t.Status) && actorRole != RoleAdmin {
			return ErrTicketClosed
		}
		idx := -1
		for i, att := range t.Attachments {
			if att.Filename == filename {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		t.Attachments = append(t.Attachments[:idx], t.Attachments[idx+1:]...)
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		out = t
		return nil
	})
	if err != nil {
		return Ticket{}, err
	}
	return out, nil
}

// ClosedTicketsWithAttachmentsBefore 返回所有已关闭且 ClosedAt 早于 cutoff 的工单，
// 用于定时清理过期的工单交流图片。
func (s *Store) ClosedTicketsWithAttachmentsBefore(cutoff int64) []Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Ticket, 0)
	for _, t := range s.state.Tickets {
		if NormalizeTicketStatus(t.Status) == TicketStatusClosed && t.ClosedAt > 0 && t.ClosedAt < cutoff && len(t.Attachments) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// ClearTicketAttachments 清空某工单的图片元数据（用于保留期清理后同步状态）。
func (s *Store) ClearTicketAttachments(ticketID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		t, ok := s.state.Tickets[ticketID]
		if !ok {
			return ErrNotFound
		}
		if len(t.Attachments) == 0 {
			return nil
		}
		t.Attachments = nil
		t.UpdatedAt = time.Now().Unix()
		s.state.Tickets[ticketID] = t
		return nil
	})
}

// TicketTypes 返回当前工单类型列表。
func (s *Store) TicketTypes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.state.TicketTypes))
	copy(out, s.state.TicketTypes)
	return out
}

// SetTicketTypes 原子替换工单类型列表。
func (s *Store) SetTicketTypes(types []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalized := normalizeTicketTypeList(types)
	// 确保至少一个类型
	if len(normalized) == 0 {
		normalized = []string{TicketTypeDefault}
	}
	return s.mutateAndSaveLocked(func() error {
		s.state.TicketTypes = normalized
		return nil
	})
}

// AddTicketType 添加工单类型，已存在则返回 ErrConflict。
func (s *Store) AddTicketType(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if !validTicketTypeName(name) {
		return ErrInvalid
	}
	return s.mutateAndSaveLocked(func() error {
		for _, t := range s.state.TicketTypes {
			if strings.EqualFold(t, name) {
				return ErrConflict
			}
		}
		s.state.TicketTypes = append(s.state.TicketTypes, name)
		return nil
	})
}

// DeleteTicketType 删除工单类型，不允许删除最后一个。
func (s *Store) DeleteTicketType(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSpace(name)
	if !validTicketTypeName(name) {
		return ErrInvalid
	}
	return s.mutateAndSaveLocked(func() error {
		if len(s.state.TicketTypes) <= 1 {
			return ErrConflict
		}
		idx := -1
		for i, t := range s.state.TicketTypes {
			if strings.EqualFold(t, name) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		s.state.TicketTypes = append(s.state.TicketTypes[:idx], s.state.TicketTypes[idx+1:]...)
		return nil
	})
}

// RenameTicketType 重命名工单类型。
func (s *Store) RenameTicketType(oldName, newName string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if !validTicketTypeName(oldName) || !validTicketTypeName(newName) {
		return 0, ErrInvalid
	}
	err := s.mutateAndSaveLocked(func() error {
		for _, t := range s.state.TicketTypes {
			if strings.EqualFold(t, newName) && !strings.EqualFold(t, oldName) {
				return ErrConflict
			}
		}
		found := false
		for i, t := range s.state.TicketTypes {
			if strings.EqualFold(t, oldName) {
				s.state.TicketTypes[i] = newName
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
		now := time.Now().Unix()
		for id, ticket := range s.state.Tickets {
			if strings.EqualFold(ticket.Type, oldName) {
				ticket.Type = newName
				ticket.UpdatedAt = now
				s.state.Tickets[id] = ticket
				count++
			}
		}
		return nil
	})
	return count, err
}

// SyncTicketTypesFromConfig 从配置同步类型（首次启动或配置变更时调用），
// 配置中的类型会覆盖 store 中的类型。
func (s *Store) SyncTicketTypesFromConfig(cfgTypes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	types := normalizeTicketTypeList(cfgTypes)
	if len(types) == 0 {
		return
	}
	_ = s.mutateAndSaveLocked(func() error {
		s.state.TicketTypes = types
		return nil
	})
}

func normalizeTicketTypeList(types []string) []string {
	out := make([]string, 0, len(types))
	seen := map[string]bool{}
	for _, raw := range types {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func validTicketTypeName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && len(name) <= 50
}
