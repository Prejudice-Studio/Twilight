package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

const telegramPanelTTL = time.Minute

type telegramPanelContext struct {
	Token            string
	ChatID           int64
	MessageID        int64
	CommandMessageID int64
	TargetUID        int64
	Query            string
	ReplyTelegramID  int64
	ExpiresAt        int64
	ConfirmAction    string
}

func telegramIsAnonymousGroupMessage(message map[string]any) bool {
	if message == nil {
		return false
	}
	if senderChat, _ := message["sender_chat"].(map[string]any); senderChat != nil {
		return true
	}
	chat, _ := message["chat"].(map[string]any)
	from, _ := message["from"].(map[string]any)
	return numeric(chat["id"]) != 0 && !strings.EqualFold(asString(chat["type"]), "private") && numeric(from["id"]) == 0
}

func (a *App) telegramResolveGroupUserTarget(fields []string, message map[string]any) (store.User, string) {
	return a.telegramResolveGroupUserTargetValues(telegramCommandQuery(fields), telegramReplyTelegramID(message))
}

func (a *App) telegramResolveGroupUserTargetValues(query string, replyTelegramID int64) (store.User, string) {
	if strings.TrimSpace(query) == "" {
		if replyTelegramID != 0 {
			if u, okUser := a.store.FindUserByTelegramID(replyTelegramID); okUser {
				return u, ""
			}
			return store.User{}, "目标 Telegram 尚未绑定 Twilight 账号。"
		}
		return store.User{}, "请回复目标用户消息后发送 /twguser，或发送 /twguser <用户名/UID/关键词>。"
	}
	users := a.telegramFindUsers(query, 6)
	if len(users) == 0 {
		return store.User{}, "未找到匹配用户。"
	}
	if len(users) > 1 {
		return store.User{}, "找到多个匹配项，请缩小关键词。\n\n" + telegramUserList(users)
	}
	return users[0], ""
}

func (a *App) telegramSendGroupAdminAuth(ctx context.Context, chatID, commandMessageID int64, fields []string, message map[string]any) {
	panel := a.telegramCreateAuthPanel(chatID, commandMessageID, telegramCommandQuery(fields), telegramReplyTelegramID(message))
	markup := telegramInlineKeyboard([][]telegramInlineButton{{
		{Text: "验证管理员身份", Data: "gadm:auth:" + panel.Token},
	}})
	messageID, err := a.telegramSendMessageWithMarkup(ctx, chatID, "匿名管理员指令需要先验证真实 Telegram 身份。", markup)
	if err != nil {
		return
	}
	panel.MessageID = messageID
	a.telegramSavePanel(panel)
}

func (a *App) telegramSendGroupUserPanel(ctx context.Context, chatID, commandMessageID int64, target store.User, requireAuth bool) {
	panel := a.telegramCreatePanel(chatID, commandMessageID, target)
	text := a.telegramGroupUserPanelText(target)
	markup := a.telegramGroupUserPanelMarkup(panel.Token, target, false)
	messageID, err := a.telegramSendMessageWithMarkup(ctx, chatID, text, markup)
	if err != nil {
		return
	}
	panel.MessageID = messageID
	if requireAuth {
		panel.ConfirmAction = "auth"
	}
	a.telegramSavePanel(panel)
}

func (a *App) telegramCreatePanel(chatID, commandMessageID int64, target store.User) telegramPanelContext {
	token := telegramRandomToken()
	return telegramPanelContext{
		Token:            token,
		ChatID:           chatID,
		CommandMessageID: commandMessageID,
		TargetUID:        target.UID,
		ExpiresAt:        time.Now().Add(telegramPanelTTL).Unix(),
	}
}

func (a *App) telegramCreateAuthPanel(chatID, commandMessageID int64, query string, replyTelegramID int64) telegramPanelContext {
	token := telegramRandomToken()
	return telegramPanelContext{
		Token:            token,
		ChatID:           chatID,
		CommandMessageID: commandMessageID,
		Query:            strings.TrimSpace(query),
		ReplyTelegramID:  replyTelegramID,
		ExpiresAt:        time.Now().Add(telegramPanelTTL).Unix(),
	}
}

func (a *App) telegramSavePanel(panel telegramPanelContext) {
	a.telegramPanelMu.Lock()
	if a.telegramPanels == nil {
		a.telegramPanels = map[string]telegramPanelContext{}
	}
	a.telegramPanels[panel.Token] = panel
	a.telegramPanelMu.Unlock()
	a.telegramSchedulePanelExpiry(panel.Token)
}

func (a *App) telegramSchedulePanelExpiry(token string) {
	time.AfterFunc(telegramPanelTTL+time.Second, func() {
		a.telegramPanelMu.Lock()
		panel, ok := a.telegramPanels[token]
		if !ok {
			a.telegramPanelMu.Unlock()
			return
		}
		delay := time.Until(time.Unix(panel.ExpiresAt, 0))
		if delay > 0 {
			a.telegramPanelMu.Unlock()
			time.AfterFunc(delay+time.Second, func() { a.telegramExpirePanel(token) })
			return
		}
		delete(a.telegramPanels, token)
		a.telegramPanelMu.Unlock()
		_ = a.telegramDeleteMessage(context.Background(), panel.ChatID, panel.MessageID)
	})
}

func (a *App) telegramExpirePanel(token string) {
	a.telegramPanelMu.Lock()
	panel, ok := a.telegramPanels[token]
	if !ok || panel.ExpiresAt > time.Now().Unix() {
		a.telegramPanelMu.Unlock()
		if ok {
			a.telegramSchedulePanelExpiry(token)
		}
		return
	}
	delete(a.telegramPanels, token)
	a.telegramPanelMu.Unlock()
	_ = a.telegramDeleteMessage(context.Background(), panel.ChatID, panel.MessageID)
}

func (a *App) telegramPanel(token string) (telegramPanelContext, bool) {
	a.telegramPanelMu.Lock()
	defer a.telegramPanelMu.Unlock()
	panel, ok := a.telegramPanels[token]
	if !ok || panel.ExpiresAt < time.Now().Unix() {
		if ok {
			delete(a.telegramPanels, token)
		}
		return telegramPanelContext{}, false
	}
	return panel, true
}

func (a *App) telegramTouchPanel(panel telegramPanelContext) telegramPanelContext {
	panel.ExpiresAt = time.Now().Add(telegramPanelTTL).Unix()
	a.telegramPanelMu.Lock()
	a.telegramPanels[panel.Token] = panel
	a.telegramPanelMu.Unlock()
	a.telegramSchedulePanelExpiry(panel.Token)
	return panel
}

func (a *App) telegramDeletePanel(token string) {
	a.telegramPanelMu.Lock()
	delete(a.telegramPanels, token)
	a.telegramPanelMu.Unlock()
}

func (a *App) telegramHandleCallback(ctx context.Context, callback map[string]any) {
	data := asString(callback["data"])
	parts := strings.Split(data, ":")
	if len(parts) < 3 || parts[0] != "gadm" {
		return
	}
	callbackID := asString(callback["id"])
	from, _ := callback["from"].(map[string]any)
	actorID := numeric(from["id"])
	message, _ := callback["message"].(map[string]any)
	chat, _ := message["chat"].(map[string]any)
	chatID := numeric(chat["id"])
	messageID := numeric(message["message_id"])
	token := parts[len(parts)-1]
	panel, ok := a.telegramPanel(token)
	if !ok {
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "面板已过期，请重新发送 /twguser。", true)
		_ = a.telegramDeleteMessage(ctx, chatID, messageID)
		return
	}
	if panel.MessageID == 0 && messageID != 0 {
		panel.MessageID = messageID
	}
	if panel.ChatID == 0 && chatID != 0 {
		panel.ChatID = chatID
	}
	if !a.telegramAdminID(actorID) {
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "没有管理员权限。", true)
		a.telegramSendUnauthorizedAndCleanup(ctx, panel.ChatID, panel.CommandMessageID)
		return
	}
	if parts[1] == "auth" {
		panel.ConfirmAction = ""
		panel = a.telegramTouchPanel(panel)
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "身份验证通过。", false)
		if panel.TargetUID == 0 {
			target, reason := a.telegramResolveGroupUserTargetValues(panel.Query, panel.ReplyTelegramID)
			if reason != "" {
				_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, reason, nil)
				a.telegramDeletePanel(panel.Token)
				return
			}
			panel.TargetUID = target.UID
			panel = a.telegramTouchPanel(panel)
		}
		a.telegramEditPanel(ctx, panel, false)
		return
	}
	if len(parts) < 4 || parts[1] != "act" {
		return
	}
	action := parts[2]
	panel = a.telegramTouchPanel(panel)
	_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "操作处理中。", false)
	a.telegramApplyPanelAction(ctx, panel, action)
}

func (a *App) telegramApplyPanelAction(ctx context.Context, panel telegramPanelContext, action string) {
	target, ok := a.store.User(panel.TargetUID)
	if !ok {
		a.telegramDeletePanel(panel.Token)
		_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, "目标用户不存在或已被删除。", nil)
		return
	}
	switch action {
	case "refresh":
		panel.ConfirmAction = ""
		a.telegramTouchPanel(panel)
		a.telegramEditPanel(ctx, panel, false)
	case "enable", "disable":
		enabled := action == "enable"
		if !enabled && a.telegramProtectedTarget(target) {
			a.telegramEditPanelWithNotice(ctx, panel, target, "管理员账号禁止通过 Telegram 面板禁用。")
			return
		}
		updated, err := a.store.UpdateUser(target.UID, func(u *store.User) error { u.Active = enabled; return nil })
		if err != nil {
			a.telegramEditPanelWithNotice(ctx, panel, target, "更新用户状态失败: "+err.Error())
			return
		}
		if updated.EmbyID != "" && a.cfg.EmbyURL != "" {
			_ = a.embySetUserEnabled(ctx, updated.EmbyID, a.embyShouldEnableUser(updated))
		}
		a.telegramEditPanelWithNotice(ctx, panel, updated, "用户状态已更新。")
	case "delete":
		if a.telegramProtectedTarget(target) {
			a.telegramEditPanelWithNotice(ctx, panel, target, "管理员账号禁止通过 Telegram 面板删除。")
			return
		}
		panel.ConfirmAction = "delete"
		panel = a.telegramTouchPanel(panel)
		a.telegramEditPanel(ctx, panel, true)
	case "delete_confirm":
		if panel.ConfirmAction != "delete" {
			a.telegramEditPanelWithNotice(ctx, panel, target, "请先点击删除按钮确认风险。")
			return
		}
		if a.telegramProtectedTarget(target) {
			a.telegramEditPanelWithNotice(ctx, panel, target, "管理员账号禁止通过 Telegram 面板删除。")
			return
		}
		if err := a.store.DeleteUser(target.UID); err != nil {
			a.telegramEditPanelWithNotice(ctx, panel, target, "删除用户失败: "+err.Error())
			return
		}
		a.sessions.DeleteUser(ctx, target.UID)
		a.telegramDeletePanel(panel.Token)
		_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, fmt.Sprintf("已删除用户 %s。", target.Username), nil)
	case "kick", "ban":
		if target.TelegramID == 0 {
			a.telegramEditPanelWithNotice(ctx, panel, target, "目标用户未绑定 Telegram，无法执行群组操作。")
			return
		}
		if a.telegramProtectedTarget(target) {
			a.telegramEditPanelWithNotice(ctx, panel, target, "管理员账号禁止通过 Telegram 面板移出或封禁。")
			return
		}
		var err error
		if action == "kick" {
			err = a.telegramKickChatMember(ctx, fmt.Sprint(panel.ChatID), target.TelegramID)
		} else {
			err = a.telegramBanChatMember(ctx, fmt.Sprint(panel.ChatID), target.TelegramID)
		}
		if err != nil {
			a.telegramEditPanelWithNotice(ctx, panel, target, "Telegram 群组操作失败: "+a.telegramSanitizeError(err))
			return
		}
		a.telegramEditPanelWithNotice(ctx, panel, target, "Telegram 群组操作已完成。")
	default:
		a.telegramEditPanelWithNotice(ctx, panel, target, "未知操作。")
	}
}

func (a *App) telegramEditPanel(ctx context.Context, panel telegramPanelContext, confirmDelete bool) {
	target, ok := a.store.User(panel.TargetUID)
	if !ok {
		_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, "目标用户不存在或已被删除。", nil)
		return
	}
	_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, a.telegramGroupUserPanelText(target), a.telegramGroupUserPanelMarkup(panel.Token, target, confirmDelete))
}

func (a *App) telegramEditPanelWithNotice(ctx context.Context, panel telegramPanelContext, target store.User, notice string) {
	panel.ConfirmAction = ""
	panel = a.telegramTouchPanel(panel)
	text := a.telegramGroupUserPanelText(target)
	if strings.TrimSpace(notice) != "" {
		text += "\n\n" + notice
	}
	_ = a.telegramEditMessageText(ctx, panel.ChatID, panel.MessageID, text, a.telegramGroupUserPanelMarkup(panel.Token, target, false))
}

func (a *App) telegramGroupUserPanelText(u store.User) string {
	return "群组用户面板\n\n" + telegramUserSummary(u) + "\n\n面板 1 分钟无操作会自动删除。"
}

func (a *App) telegramGroupUserPanelMarkup(token string, u store.User, confirmDelete bool) any {
	rows := [][]telegramInlineButton{{
		{Text: "刷新", Data: "gadm:act:refresh:" + token},
	}}
	if u.Active {
		rows = append(rows, []telegramInlineButton{{Text: "禁用账号", Data: "gadm:act:disable:" + token}})
	} else {
		rows = append(rows, []telegramInlineButton{{Text: "启用账号", Data: "gadm:act:enable:" + token}})
	}
	if confirmDelete {
		rows = append(rows, []telegramInlineButton{{Text: "确认删除用户", Data: "gadm:act:delete_confirm:" + token}})
	} else {
		rows = append(rows, []telegramInlineButton{{Text: "删除用户", Data: "gadm:act:delete:" + token}})
	}
	if u.TelegramID != 0 {
		rows = append(rows, []telegramInlineButton{
			{Text: "移出群组", Data: "gadm:act:kick:" + token},
			{Text: "封禁群组", Data: "gadm:act:ban:" + token},
		})
	}
	return telegramInlineKeyboard(rows)
}

func (a *App) telegramProtectedTarget(u store.User) bool {
	return u.Role == store.RoleAdmin || (u.TelegramID != 0 && a.telegramAdminID(u.TelegramID))
}

func (a *App) telegramSendUnauthorizedAndCleanup(ctx context.Context, chatID, sourceMessageID int64) {
	warnID, _ := a.telegramSendMessageWithMarkup(ctx, chatID, "没有管理员权限。此提示和越权指令将在 30 秒后自动删除。", nil)
	time.AfterFunc(30*time.Second, func() {
		_ = a.telegramDeleteMessage(context.Background(), chatID, warnID)
		_ = a.telegramDeleteMessage(context.Background(), chatID, sourceMessageID)
	})
}

type telegramInlineButton struct {
	Text string
	Data string
}

func telegramInlineKeyboard(rows [][]telegramInlineButton) any {
	keyboard := make([][]map[string]string, 0, len(rows))
	for _, row := range rows {
		items := make([]map[string]string, 0, len(row))
		for _, button := range row {
			items = append(items, map[string]string{"text": button.Text, "callback_data": button.Data})
		}
		keyboard = append(keyboard, items)
	}
	return map[string]any{"inline_keyboard": keyboard}
}

func telegramCommandQuery(fields []string) string {
	if len(fields) <= 1 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

func telegramReplyTelegramID(message map[string]any) int64 {
	if reply, _ := message["reply_to_message"].(map[string]any); reply != nil {
		if from, _ := reply["from"].(map[string]any); from != nil {
			return numeric(from["id"])
		}
	}
	return 0
}

func telegramRandomToken() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
