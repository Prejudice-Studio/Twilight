package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type telegramResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (a *App) telegramAvailable() bool {
	return a.cfg.TelegramMode && strings.TrimSpace(a.cfg.TelegramBotToken) != ""
}

func (a *App) telegramEndpoint(method string) string {
	base := strings.TrimRight(firstNonEmpty(a.cfg.TelegramAPIURL, "https://api.telegram.org"), "/")
	token := strings.TrimSpace(a.cfg.TelegramBotToken)
	if strings.HasSuffix(base, "/bot"+token) {
		return base + "/" + method
	}
	if strings.HasSuffix(base, "/bot") {
		return base + token + "/" + method
	}
	return base + "/bot" + token + "/" + method
}

func (a *App) setTelegramRuntimeStatus(polling bool, err error) {
	a.telegramStatusMu.Lock()
	defer a.telegramStatusMu.Unlock()
	a.telegramPolling = polling
	if err != nil {
		a.telegramLastError = a.telegramSanitizeError(err)
		a.telegramLastErrorAt = time.Now().Unix()
		return
	}
	a.telegramLastError = ""
	a.telegramLastOKAt = time.Now().Unix()
}

func (a *App) telegramRuntimeStatus() map[string]any {
	a.telegramStatusMu.Lock()
	defer a.telegramStatusMu.Unlock()
	return map[string]any{
		"polling":       a.telegramPolling,
		"last_ok_at":    zeroNil(a.telegramLastOKAt),
		"last_error_at": zeroNil(a.telegramLastErrorAt),
		"last_error":    a.telegramLastError,
	}
}

func (a *App) telegramPost(ctx context.Context, method string, body map[string]any, dst any) error {
	return a.telegramPostWithTimeout(ctx, method, body, dst, 20*time.Second)
}

func (a *App) telegramPostWithTimeout(ctx context.Context, method string, body map[string]any, dst any, timeout time.Duration) error {
	if !a.telegramAvailable() {
		return fmt.Errorf("Telegram is not enabled or bot token is not configured")
	}
	var payload telegramResponse
	if err := postJSONWithTimeout(ctx, a.telegramEndpoint(method), map[string]string{"Accept": "application/json"}, body, &payload, timeout); err != nil {
		return fmt.Errorf("%s", a.telegramSanitizeError(err))
	}
	if !payload.OK {
		msg := strings.TrimSpace(payload.Description)
		if msg == "" {
			msg = "Telegram API request failed"
		}
		if payload.ErrorCode != 0 {
			return fmt.Errorf("telegram %s failed: %s (%d)", method, msg, payload.ErrorCode)
		}
		return fmt.Errorf("telegram %s failed: %s", method, msg)
	}
	if dst != nil && len(payload.Result) > 0 {
		if err := json.Unmarshal(payload.Result, dst); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) telegramSanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	token := strings.TrimSpace(a.cfg.TelegramBotToken)
	if token == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, "/bot"+token, "/bot<redacted>")
	msg = strings.ReplaceAll(msg, token, "<redacted>")
	return msg
}

func (a *App) telegramGetMe(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	err := a.telegramPost(ctx, "getMe", map[string]any{}, &result)
	return result, err
}

func (a *App) telegramSendMessage(ctx context.Context, chatID any, text string) error {
	_, err := a.telegramSendMessageWithMarkup(ctx, chatID, text, nil)
	return err
}

func (a *App) telegramSendMessageWithMarkup(ctx context.Context, chatID any, text string, replyMarkup any) (int64, error) {
	text = truncateString(strings.TrimSpace(text), 3900)
	if text == "" {
		return 0, fmt.Errorf("message text is empty")
	}
	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if replyMarkup != nil {
		body["reply_markup"] = replyMarkup
	}
	var result map[string]any
	if err := a.telegramPost(ctx, "sendMessage", body, &result); err != nil {
		return 0, err
	}
	return numeric(result["message_id"]), nil
}

func (a *App) telegramEditMessageText(ctx context.Context, chatID, messageID int64, text string, replyMarkup any) error {
	text = truncateString(strings.TrimSpace(text), 3900)
	if text == "" {
		return fmt.Errorf("message text is empty")
	}
	body := map[string]any{"chat_id": chatID, "message_id": messageID, "text": text, "disable_web_page_preview": true}
	if replyMarkup != nil {
		body["reply_markup"] = replyMarkup
	}
	return a.telegramPost(ctx, "editMessageText", body, nil)
}

func (a *App) telegramDeleteMessage(ctx context.Context, chatID, messageID int64) error {
	if chatID == 0 || messageID == 0 {
		return nil
	}
	return a.telegramPost(ctx, "deleteMessage", map[string]any{"chat_id": chatID, "message_id": messageID}, nil)
}

func (a *App) telegramAnswerCallbackQuery(ctx context.Context, callbackID, text string, showAlert bool) error {
	if strings.TrimSpace(callbackID) == "" {
		return nil
	}
	return a.telegramPost(ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              truncateString(text, 190),
		"show_alert":        showAlert,
	}, nil)
}

func (a *App) telegramGetChatMember(ctx context.Context, chatID string, userID int64) (map[string]any, error) {
	var result map[string]any
	err := a.telegramPost(ctx, "getChatMember", map[string]any{"chat_id": chatID, "user_id": userID}, &result)
	return result, err
}

func (a *App) telegramGetChatAdministrators(ctx context.Context, chatID string) ([]map[string]any, error) {
	var result []map[string]any
	err := a.telegramPost(ctx, "getChatAdministrators", map[string]any{"chat_id": chatID}, &result)
	return result, err
}

func (a *App) telegramKickChatMember(ctx context.Context, chatID string, userID int64) error {
	if err := a.telegramPost(ctx, "banChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "revoke_messages": false}, nil); err != nil {
		return err
	}
	return a.telegramPost(ctx, "unbanChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "only_if_banned": true}, nil)
}

func (a *App) telegramBanChatMember(ctx context.Context, chatID string, userID int64) error {
	return a.telegramPost(ctx, "banChatMember", map[string]any{"chat_id": chatID, "user_id": userID, "revoke_messages": false}, nil)
}

func (a *App) telegramMembershipMissing(ctx context.Context, telegramID int64, strict bool) ([]string, error) {
	chats := telegramChatIDs(a.cfg.TelegramGroupIDs)
	if len(chats) == 0 || telegramID == 0 {
		return nil, nil
	}
	if !a.telegramAvailable() {
		if strict {
			return chats, fmt.Errorf("Telegram not configured")
		}
		return nil, nil
	}
	missing := []string{}
	for _, chatID := range chats {
		member, err := a.telegramGetChatMember(ctx, chatID, telegramID)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "not found") || strings.Contains(msg, "participant") || strings.Contains(msg, "user not found") {
				missing = append(missing, chatID)
				_ = a.store.MarkTelegramRosterLeft(chatID, telegramID, "left")
				continue
			}
			if strict {
				return missing, err
			}
			telegramRateLimitPause(err)
			continue
		}
		status := strings.ToLower(asString(member["status"]))
		if status == "left" || status == "kicked" {
			missing = append(missing, chatID)
			_ = a.store.MarkTelegramRosterLeft(chatID, telegramID, status)
			continue
		}
		user, _ := member["user"].(map[string]any)
		_ = a.store.UpsertTelegramRoster(chatID, telegramID, firstNonEmpty(status, "member"), boolish(user["is_bot"]))
	}
	return missing, nil
}

func (a *App) telegramAdminSet(ctx context.Context, chatID string) map[int64]bool {
	out := map[int64]bool{}
	for _, id := range a.cfg.TelegramAdminIDs {
		out[id] = true
	}
	admins, err := a.telegramGetChatAdministrators(ctx, chatID)
	if err != nil {
		return out
	}
	for _, member := range admins {
		user, _ := member["user"].(map[string]any)
		if id := numeric(user["id"]); id != 0 {
			out[id] = true
		}
	}
	return out
}

func (a *App) telegramKickTargets() ([]telegramKickTarget, map[string]int, int) {
	skipped := map[string]int{"admin": 0, "whitelist": 0, "bound": 0, "no_telegram": 0}
	targets := []telegramKickTarget{}
	preservedBound := 0
	for _, u := range a.store.ListUsers() {
		if u.TelegramID == 0 {
			skipped["no_telegram"]++
			continue
		}
		if u.Role == store.RoleAdmin {
			skipped["admin"]++
			continue
		}
		if u.Role == store.RoleWhitelist {
			skipped["whitelist"]++
			preservedBound++
			continue
		}
		if u.Active && u.EmbyID != "" {
			skipped["bound"]++
			preservedBound++
			continue
		}
		reason := "no_emby"
		if !u.Active {
			reason = "disabled"
		}
		if u.Role == store.RoleUnrecognized {
			reason = "no_account"
		}
		targets = append(targets, telegramKickTarget{TelegramID: u.TelegramID, UID: u.UID, Username: u.Username, Reason: reason})
	}
	return targets, skipped, preservedBound
}

type telegramKickTarget struct {
	TelegramID int64  `json:"tg_id"`
	UID        int64  `json:"uid"`
	Username   string `json:"username"`
	Reason     string `json:"reason"`
}

type telegramKickPlan struct {
	Targets        []telegramKickTarget
	Skipped        map[string]int
	PreservedBound int
	RosterSize     int
	Bots           int
	KnownOnly      bool
}

func (t telegramKickTarget) dto() map[string]any {
	return map[string]any{"tg_id": t.TelegramID, "uid": t.UID, "username": t.Username, "reason": t.Reason}
}

func (a *App) telegramKickPlan(chatID string) telegramKickPlan {
	entries := a.store.TelegramRoster(chatID, true)
	if len(entries) == 0 {
		targets, skipped, preserved := a.telegramKickTargets()
		return telegramKickPlan{Targets: targets, Skipped: skipped, PreservedBound: preserved, RosterSize: len(targets) + preserved + skipped["admin"] + skipped["whitelist"], KnownOnly: true}
	}
	skipped := map[string]int{"admin": 0, "whitelist": 0, "bound": 0, "no_telegram": 0, "bot": 0}
	adminIDs := map[int64]bool{}
	for _, id := range a.cfg.TelegramAdminIDs {
		adminIDs[id] = true
	}
	usersByTG := map[int64]store.User{}
	for _, u := range a.store.ListUsers() {
		if u.TelegramID != 0 {
			usersByTG[u.TelegramID] = u
		} else {
			skipped["no_telegram"]++
		}
	}
	targets := []telegramKickTarget{}
	preserved := 0
	for _, entry := range entries {
		if entry.IsBot {
			skipped["bot"]++
			continue
		}
		if adminIDs[entry.TelegramID] {
			skipped["admin"]++
			continue
		}
		u, ok := usersByTG[entry.TelegramID]
		if !ok {
			targets = append(targets, telegramKickTarget{TelegramID: entry.TelegramID, Reason: "no_account"})
			continue
		}
		if u.Role == store.RoleAdmin {
			skipped["admin"]++
			preserved++
			continue
		}
		if u.Role == store.RoleWhitelist {
			skipped["whitelist"]++
			preserved++
			continue
		}
		if u.Active && u.EmbyID != "" {
			skipped["bound"]++
			preserved++
			continue
		}
		reason := "no_emby"
		if !u.Active {
			reason = "disabled"
		}
		if u.Role == store.RoleUnrecognized {
			reason = "no_account"
		}
		targets = append(targets, telegramKickTarget{TelegramID: entry.TelegramID, UID: u.UID, Username: u.Username, Reason: reason})
	}
	return telegramKickPlan{Targets: targets, Skipped: skipped, PreservedBound: preserved, RosterSize: len(entries), Bots: skipped["bot"], KnownOnly: false}
}

func telegramChatIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func telegramChatIDValue(value string) any {
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	return value
}

func telegramMemberIsGone(member map[string]any) bool {
	status := strings.ToLower(asString(member["status"]))
	return status == "left" || status == "kicked"
}

func telegramMemberIsAdminOrBot(member map[string]any) bool {
	status := strings.ToLower(asString(member["status"]))
	if status == "creator" || status == "administrator" {
		return true
	}
	user, _ := member["user"].(map[string]any)
	return boolish(user["is_bot"])
}

func telegramRateLimitPause(err error) {
	if err == nil {
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "too many requests") {
		time.Sleep(2 * time.Second)
	}
}
