package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	developerJSInteractionTTL      = 2 * time.Minute
	developerJSWaitMaxSeconds      = 60
	developerJSMaxInlineButtons    = 8
	developerJSMaxInteractionChars = 1200
)

type developerJSCallbackAction struct {
	Text   string
	Answer string
	Edit   string
	Reply  string
}

type developerJSCallbackContext struct {
	Token           string
	ChatID          int64
	MessageID       int64
	OwnerTelegramID int64
	ExpiresAt       int64
	Actions         []developerJSCallbackAction
	Timer           *time.Timer
}

type developerJSMessageWaiter struct {
	Key          string
	ChatID       int64
	FromID       int64
	ExpiresAt    int64
	ReplyPrefix  string
	TimeoutReply string
	MaxChars     int
	Numbered     bool
	Timer        *time.Timer
}

func developerJSWaiterKey(chatID, fromID int64) string {
	return fmt.Sprintf("%d:%d", chatID, fromID)
}

func (a *App) saveDeveloperJSCallback(item developerJSCallbackContext) {
	a.developerJSMu.Lock()
	if a.developerJSCallbacks == nil {
		a.developerJSCallbacks = map[string]developerJSCallbackContext{}
	}
	if existing, ok := a.developerJSCallbacks[item.Token]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}
	token := item.Token
	item.Timer = time.AfterFunc(time.Until(time.Unix(item.ExpiresAt, 0))+time.Second, func() {
		a.deleteDeveloperJSCallback(token)
	})
	a.developerJSCallbacks[item.Token] = item
	a.developerJSMu.Unlock()
}

func (a *App) deleteDeveloperJSCallback(token string) {
	a.developerJSMu.Lock()
	if existing, ok := a.developerJSCallbacks[token]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}
	delete(a.developerJSCallbacks, token)
	a.developerJSMu.Unlock()
}

func (a *App) developerJSCallback(token string) (developerJSCallbackContext, bool) {
	a.developerJSMu.Lock()
	defer a.developerJSMu.Unlock()
	item, ok := a.developerJSCallbacks[token]
	if !ok || item.ExpiresAt < time.Now().Unix() {
		if ok && item.Timer != nil {
			item.Timer.Stop()
		}
		delete(a.developerJSCallbacks, token)
		return developerJSCallbackContext{}, false
	}
	return item, true
}

func (a *App) telegramHandleDeveloperJSCallback(ctx context.Context, callback map[string]any) bool {
	data := asString(callback["data"])
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != "djs" {
		return false
	}
	if !a.store().DeveloperModeEnabled() {
		_ = a.telegramAnswerCallbackQuery(ctx, asString(callback["id"]), "Developer mode is disabled.", true)
		return true
	}
	token := parts[1]
	idx := int(numeric(parts[2]))
	callbackID := asString(callback["id"])
	from, _ := callback["from"].(map[string]any)
	actorID := numeric(from["id"])
	message, _ := callback["message"].(map[string]any)
	chat, _ := message["chat"].(map[string]any)
	chatID := numeric(chat["id"])
	messageID := numeric(message["message_id"])
	item, ok := a.developerJSCallback(token)
	if !ok {
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "This action has expired.", true)
		return true
	}
	if item.ChatID != chatID || item.MessageID != messageID || item.OwnerTelegramID != actorID {
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "This action is not available for your account.", true)
		a.auditDeveloperJSInteraction(actorID, "telegram_js_interaction_callback_denied", map[string]any{
			"reason":     "owner_mismatch",
			"chat_id":    chatID,
			"message_id": messageID,
		})
		return true
	}
	if idx < 0 || idx >= len(item.Actions) {
		_ = a.telegramAnswerCallbackQuery(ctx, callbackID, "Unknown action.", true)
		a.auditDeveloperJSInteraction(actorID, "telegram_js_interaction_callback_denied", map[string]any{
			"reason":     "unknown_action",
			"chat_id":    chatID,
			"message_id": messageID,
		})
		return true
	}
	action := item.Actions[idx]
	answer := firstNonEmpty(action.Answer, action.Text)
	_ = a.telegramAnswerCallbackQuery(ctx, callbackID, truncateString(answer, 190), false)
	if strings.TrimSpace(action.Edit) != "" {
		_ = a.telegramEditMessageText(ctx, chatID, messageID, action.Edit, nil)
		a.deleteDeveloperJSCallback(token)
	}
	if strings.TrimSpace(action.Reply) != "" {
		_ = a.telegramSendMessage(ctx, chatID, action.Reply)
	}
	a.auditDeveloperJSInteraction(actorID, "telegram_js_interaction_callback", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"action":     idx,
		"edited":     strings.TrimSpace(action.Edit) != "",
		"replied":    strings.TrimSpace(action.Reply) != "",
	})
	return true
}

func (a *App) saveDeveloperJSWaiter(item developerJSMessageWaiter) {
	a.developerJSMu.Lock()
	if a.developerJSWaiters == nil {
		a.developerJSWaiters = map[string]developerJSMessageWaiter{}
	}
	if existing, ok := a.developerJSWaiters[item.Key]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}
	key := item.Key
	timeoutReply := item.TimeoutReply
	chatID := item.ChatID
	item.Timer = time.AfterFunc(time.Until(time.Unix(item.ExpiresAt, 0))+time.Second, func() {
		a.deleteDeveloperJSWaiter(key)
		if strings.TrimSpace(timeoutReply) != "" {
			_ = a.telegramSendMessage(context.Background(), chatID, timeoutReply)
		}
	})
	a.developerJSWaiters[item.Key] = item
	a.developerJSMu.Unlock()
}

func (a *App) deleteDeveloperJSWaiter(key string) {
	a.developerJSMu.Lock()
	if existing, ok := a.developerJSWaiters[key]; ok && existing.Timer != nil {
		existing.Timer.Stop()
	}
	delete(a.developerJSWaiters, key)
	a.developerJSMu.Unlock()
}

func (a *App) takeDeveloperJSWaiter(chatID, fromID int64) (developerJSMessageWaiter, bool) {
	key := developerJSWaiterKey(chatID, fromID)
	a.developerJSMu.Lock()
	defer a.developerJSMu.Unlock()
	item, ok := a.developerJSWaiters[key]
	if !ok || item.ExpiresAt < time.Now().Unix() {
		if ok && item.Timer != nil {
			item.Timer.Stop()
		}
		delete(a.developerJSWaiters, key)
		return developerJSMessageWaiter{}, false
	}
	if item.Timer != nil {
		item.Timer.Stop()
	}
	delete(a.developerJSWaiters, key)
	return item, true
}

func (a *App) telegramConsumeDeveloperJSWaiter(ctx context.Context, chatID, fromID int64, text string) bool {
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		return false
	}
	if !a.store().DeveloperModeEnabled() {
		_, _ = a.takeDeveloperJSWaiter(chatID, fromID)
		return false
	}
	item, ok := a.takeDeveloperJSWaiter(chatID, fromID)
	if !ok {
		return false
	}
	maxChars := item.MaxChars
	if maxChars <= 0 || maxChars > developerJSMaxInteractionChars {
		maxChars = 240
	}
	value := developerJSLimitText(text, maxChars)
	if item.Numbered {
		fields := strings.Fields(value)
		lines := make([]string, 0, len(fields))
		for i, field := range fields {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, field))
		}
		value = strings.Join(lines, "\n")
	}
	prefix := strings.TrimSpace(item.ReplyPrefix)
	if prefix != "" {
		value = prefix + "\n" + value
	}
	_ = a.telegramSendMessage(ctx, chatID, value)
	a.auditDeveloperJSInteraction(fromID, "telegram_js_interaction_wait_text", map[string]any{
		"chat_id":   chatID,
		"max_chars": maxChars,
		"numbered":  item.Numbered,
	})
	return true
}

func (a *App) auditDeveloperJSInteraction(telegramID int64, action string, detail map[string]any) {
	user, ok := a.store().FindUserByTelegramID(telegramID)
	if !ok {
		a.auditEntryIP("telegram", 0, "", action, "user", 0, detail)
		return
	}
	a.auditEntryIP("telegram", user.UID, user.Username, action, "user", user.UID, detail)
}
