package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
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
	// Telegram 在 429 / 部分 4xx 上返回 parameters.retry_after（秒）告诉调用方
	// 何时可以重试。旧实现整个 parameters 都被丢弃，限速重试只能 sleep 死板
	// 2 秒，对真实 30s+ 限速完全无效——批处理（kick / 提醒）变成"重试 → 再次
	// 429 → 全部 failed"。这里抽出来后由 telegramRateLimitPause 复用。
	Parameters telegramResponseParameters `json:"parameters"`
}

type telegramResponseParameters struct {
	RetryAfter      int   `json:"retry_after"`
	MigrateToChatID int64 `json:"migrate_to_chat_id"`
}

// telegramRetryAfterSentinel 让 telegramRateLimitPause 能从 Telegram 协议层
// 的 error message 中把 retry_after 反解出来。把秒数编进 error 字符串可让
// 现有批处理和调度调用方保持统一的轻量重试路径。
//
// 取一个不可能出现在 telegram 真实文案里的前缀，避免和 description 混淆。
const telegramRetryAfterSentinel = "tw_retry_after_seconds="

func (a *App) telegramAvailable() bool {
	cfg := a.cfg()
	return cfg.TelegramMode && strings.TrimSpace(cfg.TelegramBotToken) != ""
}

type telegramEndpointCacheEntry struct {
	base   string
	token  string
	prefix string
}

func (a *App) telegramEndpoint(method string) (string, error) {
	cfg := a.cfg()
	rawBase := strings.TrimRight(firstNonEmpty(cfg.TelegramAPIURL, "https://api.telegram.org"), "/")
	token := strings.TrimSpace(cfg.TelegramBotToken)
	if cached := a.telegramEndpointCache.Load(); cached != nil && cached.base == rawBase && cached.token == token {
		return cached.prefix + "/" + method, nil
	}
	// telegramEndpoint 现在返回 error：与 Emby / Bangumi / TMDB 对齐，配置面
	// 被入侵或 admin 误填后，能在拼出含 bot token 的 URL **之前** 否决。
	// 之前是 "TrimRight + 拼字符串"，scheme=javascript: 或 host=元数据 IP 都
	// 会原样喂给 net/http，bot token 直接泄漏到攻击者控制的目标。
	//
	// 在 base 已经带 "/bot<TOKEN>" 路径时，要校验的仍然只是 host+scheme，
	// 所以校验前剥掉 path（与 bangumiEndpoint 同样套路）。
	probeBase := rawBase
	if pb, err := url.Parse(rawBase); err == nil {
		clean := *pb
		clean.Path = ""
		clean.RawPath = ""
		probeBase = clean.String()
	}
	if _, err := validateOutboundBaseURL(probeBase, "Telegram"); err != nil {
		return "", err
	}
	prefix := rawBase + "/bot" + token
	if strings.HasSuffix(rawBase, "/bot"+token) {
		prefix = rawBase
	} else if strings.HasSuffix(rawBase, "/bot") {
		prefix = rawBase + token
	}
	a.telegramEndpointCache.Store(&telegramEndpointCacheEntry{base: rawBase, token: token, prefix: prefix})
	return prefix + "/" + method, nil
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

func (a *App) telegramSanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	token := strings.TrimSpace(a.cfg().TelegramBotToken)
	if token == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, "/bot"+token, "/bot<redacted>")
	msg = strings.ReplaceAll(msg, token, "<redacted>")
	return msg
}

func (a *App) telegramGetMe(ctx context.Context) (map[string]any, error) {
	return telegramPostResult[map[string]any](a, ctx, "getMe", struct{}{}, 20*time.Second)
}

func (a *App) telegramGetChat(ctx context.Context, chatID any) (map[string]any, error) {
	return telegramPostResult[map[string]any](a, ctx, "getChat", struct {
		ChatID any `json:"chat_id"`
	}{ChatID: chatID}, 20*time.Second)
}

func (a *App) telegramSendMessage(ctx context.Context, chatID any, text string) error {
	_, err := a.telegramSendMessageWithMarkup(ctx, chatID, text, nil)
	return err
}

func (a *App) telegramSendPlainMessage(ctx context.Context, chatID any, text string) error {
	text = truncateString(strings.TrimSpace(text), 3900)
	if text == "" {
		return fmt.Errorf("message text is empty")
	}
	body := telegramSendMessageRequest{ChatID: chatID, Text: text, DisableWebPagePreview: true}
	return a.telegramPostNoResult(ctx, "sendMessage", body)
}

func (a *App) telegramSendMessageWithMarkup(ctx context.Context, chatID any, text string, replyMarkup any) (int64, error) {
	text = truncateString(strings.TrimSpace(text), 3900)
	if text == "" {
		return 0, fmt.Errorf("message text is empty")
	}
	body := telegramSendMessageRequest{
		ChatID: chatID, Text: text, ParseMode: a.telegramParseMode(),
		DisableWebPagePreview: true, ReplyMarkup: replyMarkup,
	}
	result, err := telegramPostResult[telegramMessageResult](a, ctx, "sendMessage", body, 20*time.Second)
	if err != nil {
		return 0, err
	}
	return result.MessageID, nil
}

func (a *App) telegramSendPhoto(ctx context.Context, chatID any, filename, contentType string, data []byte, caption, parseMode string) error {
	if len(data) == 0 {
		return fmt.Errorf("photo data is empty")
	}
	endpoint, endpointErr := a.telegramEndpoint("sendPhoto")
	if endpointErr != nil {
		return fmt.Errorf("%s", a.telegramSanitizeError(endpointErr))
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("chat_id", fmt.Sprint(chatID)); err != nil {
		return err
	}
	if text := truncateString(strings.TrimSpace(caption), 900); text != "" {
		if err := writer.WriteField("caption", text); err != nil {
			return err
		}
		if parseMode != "" {
			if err := writer.WriteField("parse_mode", parseMode); err != nil {
				return err
			}
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="photo"; filename="%s"`, strings.ReplaceAll(filename, `"`, "")))
	if contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if _, ok := req.Context().Deadline(); !ok {
		reqCtx, cancel := context.WithTimeout(req.Context(), 20*time.Second)
		defer cancel()
		req = req.WithContext(reqCtx)
	}
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s", a.telegramSanitizeError(err))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var payload telegramResponse
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
	}
	if resp.StatusCode >= 400 || !payload.OK {
		msg := strings.TrimSpace(payload.Description)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if msg == "" {
			msg = "Telegram API request failed"
		}
		return fmt.Errorf("%s", a.telegramSanitizeError(fmt.Errorf("telegram sendPhoto failed: %s", truncateString(msg, 300))))
	}
	return nil
}

func (a *App) telegramEditMessageText(ctx context.Context, chatID, messageID int64, text string, replyMarkup any) error {
	text = truncateString(strings.TrimSpace(text), 3900)
	if text == "" {
		return fmt.Errorf("message text is empty")
	}
	body := telegramEditMessageRequest{
		ChatID: chatID, MessageID: messageID, Text: text, ParseMode: a.telegramParseMode(),
		DisableWebPagePreview: true, ReplyMarkup: replyMarkup,
	}
	return a.telegramPostNoResult(ctx, "editMessageText", body)
}

// telegramParseMode 返回当前配置的消息解析模式。
func (a *App) telegramParseMode() string {
	pm := a.cfg().TelegramParseMode
	switch pm {
	case "Markdown", "MarkdownV2", "HTML":
		return pm
	}
	return ""
}

// telegramEscapeHTML 转义 Telegram HTML parse mode 必须转义的三个字符。
// 只处理 & < >（Telegram HTML 子集不要求转义引号），避免 html.EscapeString
// 把引号变成 &#34; / &#39; 这类冗长实体污染文案。用于把用户可控内容（标题、
// 用户名、回复正文等）安全地嵌入我们自己拼的 HTML 结构里。
func telegramEscapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// stripTelegramHTML 把我们发出的有限 HTML（<b>/<code>/<blockquote> 等）还原成
// 纯文本：去标签 + 反转义三个实体。仅用于 HTML 发送失败后的纯文本降级兜底。
func stripTelegramHTML(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&amp;", "&")
	return out
}

// telegramIsParseError 判断错误是否为 Telegram HTML 实体解析失败（400）。
// 自定义模板可能混入裸 < / & 导致解析失败，此时应降级为纯文本重发而非放弃。
func telegramIsParseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't parse entities") || strings.Contains(msg, "can't parse") || strings.Contains(msg, "unsupported start tag") || strings.Contains(msg, "unmatched end tag")
}

// telegramSendRichMessage 用 HTML parse mode 发送富文本通知（工单等系统消息专用），
// 不受运营配置的全局 parse_mode 影响，保证排版稳定统一。若 Telegram 因 HTML 解析
// 失败（例如自定义模板混入裸 < / &）返回 400，自动用 plainFallback 纯文本重发，
// plainFallback 为空时回退到去标签后的 HTML，确保消息始终可送达。
func (a *App) telegramSendRichMessage(ctx context.Context, chatID any, htmlText, plainFallback string) error {
	htmlText = truncateString(strings.TrimSpace(htmlText), 3900)
	if htmlText == "" {
		return fmt.Errorf("message text is empty")
	}
	err := a.telegramPostNoResult(ctx, "sendMessage", telegramSendMessageRequest{
		ChatID: chatID, Text: htmlText, ParseMode: "HTML", DisableWebPagePreview: true,
	})
	if err == nil || !telegramIsParseError(err) {
		return err
	}
	plain := truncateString(strings.TrimSpace(plainFallback), 3900)
	if plain == "" {
		plain = truncateString(stripTelegramHTML(htmlText), 3900)
	}
	if plain == "" {
		return err
	}
	return a.telegramPostNoResult(ctx, "sendMessage", telegramSendMessageRequest{
		ChatID: chatID, Text: plain, DisableWebPagePreview: true,
	})
}

func (a *App) telegramDeleteMessage(ctx context.Context, chatID, messageID int64) error {
	if chatID == 0 || messageID == 0 {
		return nil
	}
	return a.telegramPostNoResult(ctx, "deleteMessage", telegramDeleteMessageRequest{ChatID: chatID, MessageID: messageID})
}

func (a *App) telegramAnswerCallbackQuery(ctx context.Context, callbackID, text string, showAlert bool) error {
	if strings.TrimSpace(callbackID) == "" {
		return nil
	}
	return a.telegramPostNoResult(ctx, "answerCallbackQuery", telegramAnswerCallbackRequest{
		CallbackQueryID: callbackID, Text: truncateString(text, 190), ShowAlert: showAlert,
	})
}

func (a *App) telegramGetChatMember(ctx context.Context, chatID string, userID int64) (map[string]any, error) {
	return a.telegramGetChatMemberWithTimeout(ctx, chatID, userID, 20*time.Second)
}

func (a *App) telegramGetChatMemberWithTimeout(ctx context.Context, chatID string, userID int64, timeout time.Duration) (map[string]any, error) {
	return telegramPostResult[map[string]any](a, ctx, "getChatMember", telegramChatMemberRequest{ChatID: chatID, UserID: userID}, timeout)
}

func (a *App) telegramGetChatAdministrators(ctx context.Context, chatID string) ([]map[string]any, error) {
	return telegramPostResult[[]map[string]any](a, ctx, "getChatAdministrators", struct {
		ChatID string `json:"chat_id"`
	}{ChatID: chatID}, 20*time.Second)
}

func (a *App) telegramKickChatMember(ctx context.Context, chatID string, userID int64) error {
	if err := a.telegramPostNoResult(ctx, "banChatMember", telegramBanMemberRequest{ChatID: chatID, UserID: userID, RevokeMessages: false}); err != nil {
		return err
	}
	return a.telegramPostNoResult(ctx, "unbanChatMember", telegramUnbanMemberRequest{ChatID: chatID, UserID: userID, OnlyIfBanned: true})
}

func (a *App) telegramBanChatMember(ctx context.Context, chatID string, userID int64) error {
	return a.telegramPostNoResult(ctx, "banChatMember", telegramBanMemberRequest{ChatID: chatID, UserID: userID, RevokeMessages: false})
}

func (a *App) telegramMembershipMissing(ctx context.Context, telegramID int64, strict bool) ([]string, error) {
	chats := telegramChatIDs(a.cfg().TelegramGroupIDs)
	return a.telegramMembershipMissingForChats(ctx, telegramID, chats, strict)
}

func (a *App) telegramBindRequirementMissing(ctx context.Context, telegramID int64) ([]string, error) {
	chats := []string{}
	if a.cfg().TelegramForceBindGroup {
		chats = append(chats, telegramChatIDs(a.cfg().TelegramGroupIDs)...)
	}
	if a.cfg().TelegramForceBindChannel {
		chats = append(chats, telegramChatIDs(a.cfg().TelegramChannelIDs)...)
	}
	return a.telegramMembershipMissingForChats(ctx, telegramID, chats, true)
}

func (a *App) telegramMembershipMissingForChats(ctx context.Context, telegramID int64, chats []string, strict bool) ([]string, error) {
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				return missing, ctxErr
			}
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "not found") || strings.Contains(msg, "participant") || strings.Contains(msg, "user not found") {
				missing = append(missing, chatID)
				_ = a.store().MarkTelegramRosterLeft(chatID, telegramID, "left")
				continue
			}
			if strict {
				return missing, err
			}
			if !telegramRateLimitPauseContext(ctx, err) {
				return missing, ctx.Err()
			}
			continue
		}
		status := strings.ToLower(asString(member["status"]))
		if status == "left" || status == "kicked" {
			missing = append(missing, chatID)
			_ = a.store().MarkTelegramRosterLeft(chatID, telegramID, status)
			continue
		}
		user, _ := member["user"].(map[string]any)
		_ = a.store().UpsertTelegramRoster(chatID, telegramID, firstNonEmpty(status, "member"), boolish(user["is_bot"]))
	}
	return missing, nil
}

func (a *App) telegramAdminSet(ctx context.Context, chatID string) map[int64]bool {
	out := map[int64]bool{}
	for _, id := range a.cfg().TelegramAdminIDs {
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
	for _, u := range a.store().ListUsers() {
		if u.TelegramID == 0 {
			skipped["no_telegram"]++
			continue
		}
		if reason := a.protectedUserReason(u); reason != "" {
			if reason == "whitelist" {
				skipped["whitelist"]++
				preservedBound++
			} else {
				skipped["admin"]++
			}
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

func (a *App) telegramKickPlan(chatID string) (telegramKickPlan, error) {
	entries, err := a.store().TelegramRoster(chatID, true)
	if err != nil {
		return telegramKickPlan{}, err
	}
	if len(entries) == 0 {
		targets, skipped, preserved := a.telegramKickTargets()
		return telegramKickPlan{Targets: targets, Skipped: skipped, PreservedBound: preserved, RosterSize: len(targets) + preserved + skipped["admin"] + skipped["whitelist"], KnownOnly: true}, nil
	}
	skipped := map[string]int{"admin": 0, "whitelist": 0, "bound": 0, "no_telegram": 0, "bot": 0}
	adminIDs := map[int64]bool{}
	for _, id := range a.cfg().TelegramAdminIDs {
		adminIDs[id] = true
	}
	usersByTG := map[int64]store.User{}
	for _, u := range a.store().ListUsers() {
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
		if reason := a.protectedUserReason(u); reason != "" {
			if reason == "whitelist" {
				skipped["whitelist"]++
				preserved++
			} else {
				skipped["admin"]++
			}
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
	return telegramKickPlan{Targets: targets, Skipped: skipped, PreservedBound: preserved, RosterSize: len(entries), Bots: skipped["bot"], KnownOnly: false}, nil
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

// telegramRateLimitPause 在批处理（kick / 提醒）路径上充当一行式背压：
// 看到 telegram 限速错误就 sleep。
//
// 两条来源：
//  1. Telegram 协议层在 OK=false + parameters.retry_after>0 时把秒数编进错误
//     字符串（telegramRetryAfterSentinel）；这里反解出来，sleep 真实秒数；
//  2. fallback 到旧行为——没有 sentinel 但 description 里出现 "too many
//     requests" 时 sleep 2s（调用方与 admin 路径上偶尔有自己拼装的 429 错
//     误，不走统一协议层）。
//
// 上限 60s 是工程妥协：scheduler 任务总 ctx 限制在 30 分钟级别，单次 sleep
// 太长会让 admin 取消任务时反应迟钝；retry_after > 60s 的情况一般是 chat
// 已经被 telegram 临时禁言，重试再多也是失败，让本轮 batch 提前 fail 反而
// 让上层的 failedCount 阈值更早触发。
func telegramRateLimitPause(err error) {
	_ = telegramRateLimitPauseContext(context.Background(), err)
}

func telegramRateLimitPauseContext(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	d := time.Duration(0)
	if d, ok := telegramRetryAfterFromError(err); ok {
		if d > 60*time.Second {
			d = 60 * time.Second
		}
	} else if strings.Contains(strings.ToLower(err.Error()), "too many requests") {
		d = 2 * time.Second
	}
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// telegramRetryAfterFromError 从 telegram 错误字符串里反解出 retry_after 秒数。
// 找不到时第二个返回值 false，调用方走原有 fallback 行为。
func telegramRetryAfterFromError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	msg := err.Error()
	idx := strings.Index(msg, telegramRetryAfterSentinel)
	if idx < 0 {
		return 0, false
	}
	tail := msg[idx+len(telegramRetryAfterSentinel):]
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	secs, err := strconv.Atoi(tail[:end])
	if err != nil || secs <= 0 {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}
