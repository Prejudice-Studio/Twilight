package api

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

var telegramBindCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{6,16}$`)

func (a *App) RunTelegramBot(ctx context.Context) error {
	offset := int64(0)
	activeConfig := ""
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		a.reloadConfigIfChanged()
		if !a.telegramAvailable() {
			slog.Info("Telegram bot configuration disabled; waiting before next config check")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		currentConfig := strings.TrimSpace(a.cfg.TelegramAPIURL) + "|" + strings.TrimSpace(a.cfg.TelegramBotToken)
		if currentConfig != activeConfig {
			me, err := a.telegramGetMe(ctx)
			if err != nil {
				slog.Warn("Telegram bot initialization failed", "error", err)
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(3 * time.Second):
					continue
				}
			}
			activeConfig = currentConfig
			offset = 0
			slog.Info("Telegram bot polling started", "username", me["username"])
		}
		updates, err := a.telegramGetUpdates(ctx, offset)
		if err != nil {
			slog.Warn("Telegram getUpdates failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		for _, update := range updates {
			if id := numeric(update["update_id"]); id >= offset {
				offset = id + 1
			}
			a.handleTelegramUpdate(ctx, update)
		}
	}
}

func (a *App) telegramGetUpdates(ctx context.Context, offset int64) ([]map[string]any, error) {
	var result []map[string]any
	body := map[string]any{"timeout": 30, "allowed_updates": []string{"message", "chat_member", "my_chat_member"}}
	if offset > 0 {
		body["offset"] = offset
	}
	err := a.telegramPost(ctx, "getUpdates", body, &result)
	return result, err
}

func (a *App) handleTelegramUpdate(ctx context.Context, update map[string]any) {
	a.observeTelegramRoster(update)
	message, _ := update["message"].(map[string]any)
	if message == nil {
		return
	}
	text := strings.TrimSpace(asString(message["text"]))
	if text == "" {
		return
	}
	chat, _ := message["chat"].(map[string]any)
	from, _ := message["from"].(map[string]any)
	chatID := numeric(chat["id"])
	fromID := numeric(from["id"])
	if chatID == 0 {
		chatID = fromID
	}
	username := strings.TrimPrefix(asString(from["username"]), "@")
	privateChat := chatID == fromID || strings.EqualFold(asString(chat["type"]), "private")
	fields := strings.Fields(text)
	command := telegramCommand(fields[0])
	switch command {
	case "/start", "/help", "/twihelp":
		if !privateChat {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramGroupPrompt())
			return
		}
		if command == "/start" {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramStartText())
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, a.telegramHelpText(a.telegramAdminID(fromID)))
	case "/twishelp":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		if !a.telegramAdminID(fromID) {
			_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, a.telegramAdminHelpText())
	case "/about":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, a.telegramAboutText())
	case "/cancel":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, "已取消当前 Bot 操作。")
	case "/me":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleMe(ctx, chatID, fromID)
	case "/emby":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleEmby(ctx, chatID, fromID)
	case "/playinfo":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandlePlayInfo(ctx, chatID, fromID)
	case "/resetpwd":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleResetPassword(ctx, chatID, fromID)
	case "/stats":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleStats(ctx, chatID, fromID)
	case "/admin":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleAdmin(ctx, chatID, fromID)
	case "/userinfo":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleUserInfo(ctx, chatID, fromID, strings.Join(fields[1:], " "))
	case "/twfind":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		a.telegramHandleFind(ctx, chatID, fromID, strings.Join(fields[1:], " "))
	case "/twguser":
		a.telegramHandleGroupUser(ctx, chatID, fromID, fields, message)
	case "/bind":
		if !a.telegramRequirePrivate(ctx, chatID, privateChat) {
			return
		}
		if len(fields) < 2 {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramBindPrompt())
			return
		}
		a.telegramConfirmBindCode(ctx, chatID, fromID, username, fields[1])
	default:
		if reply, ok := a.telegramCustomCommandReply(command); ok {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramRenderText(reply))
			return
		}
		if privateChat && telegramBindCodePattern.MatchString(command) && !strings.HasPrefix(command, "/") {
			a.telegramConfirmBindCode(ctx, chatID, fromID, username, command)
			return
		}
		if privateChat && strings.HasPrefix(command, "/") {
			_ = a.telegramSendMessage(ctx, chatID, "未知命令。发送 /help 查看可用命令。")
		}
	}
}

func (a *App) observeTelegramRoster(update map[string]any) {
	if message, _ := update["message"].(map[string]any); message != nil {
		chat, _ := message["chat"].(map[string]any)
		from, _ := message["from"].(map[string]any)
		chatID := numeric(chat["id"])
		fromID := numeric(from["id"])
		if chatID != 0 && fromID > 0 && chatID != fromID {
			_ = a.store.UpsertTelegramRoster(fmt.Sprint(chatID), fromID, "member", boolish(from["is_bot"]))
		}
		return
	}
	for _, key := range []string{"chat_member", "my_chat_member"} {
		event, _ := update[key].(map[string]any)
		if event == nil {
			continue
		}
		chat, _ := event["chat"].(map[string]any)
		newMember, _ := event["new_chat_member"].(map[string]any)
		user, _ := newMember["user"].(map[string]any)
		chatID := numeric(chat["id"])
		userID := numeric(user["id"])
		status := strings.ToLower(asString(newMember["status"]))
		if chatID == 0 || userID <= 0 {
			return
		}
		if status == "left" || status == "kicked" {
			_ = a.store.MarkTelegramRosterLeft(fmt.Sprint(chatID), userID, status)
			return
		}
		_ = a.store.UpsertTelegramRoster(fmt.Sprint(chatID), userID, firstNonEmpty(status, "member"), boolish(user["is_bot"]))
		return
	}
}

func (a *App) telegramConfirmBindCode(ctx context.Context, chatID, telegramID int64, username, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !telegramBindCodePattern.MatchString(code) {
		_ = a.telegramSendMessage(ctx, chatID, "绑定码格式无效，请在 Web 端重新生成后再发送。")
		return
	}
	bind, okBind := a.store.BindCode(code)
	if !okBind || bind.ExpiresAt < time.Now().Unix() {
		_ = a.telegramSendMessage(ctx, chatID, "绑定码无效或已过期，请在网页重新获取。")
		return
	}
	if existing, okUser := a.store.FindUserByTelegramID(telegramID); okUser && (bind.UID == 0 || existing.UID != bind.UID) {
		_ = a.telegramSendMessage(ctx, chatID, fmt.Sprintf("该 Telegram 已绑定到账号 %s。", existing.Username))
		return
	}
	bind.Confirmed = true
	bind.TelegramID = telegramID
	_ = a.store.UpsertBindCode(bind)
	if bind.UID != 0 {
		_, err := a.store.UpdateUser(bind.UID, func(u *store.User) error {
			u.TelegramID = telegramID
			u.TelegramUsername = username
			return nil
		})
		if err != nil {
			_ = a.telegramSendMessage(ctx, chatID, "绑定失败：系统用户不存在。")
			return
		}
	}
	_ = a.telegramSendMessage(ctx, chatID, "Telegram 绑定已确认，可以回到网页继续。")
}

func (a *App) telegramHandleMe(ctx context.Context, chatID, telegramID int64) {
	if u, okUser := a.store.FindUserByTelegramID(telegramID); okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前绑定信息\n\n"+telegramUserSummary(u))
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
}

func (a *App) telegramHandleEmby(ctx context.Context, chatID, telegramID int64) {
	u, okUser := a.store.FindUserByTelegramID(telegramID)
	if !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	online := false
	checked := false
	if strings.TrimSpace(a.cfg.EmbyURL) != "" {
		checked = true
		info := map[string]any{}
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := a.embyGet(checkCtx, "/System/Info/Public", &info); err == nil {
			online = true
		} else if err := a.embyGet(checkCtx, "/System/Info", &info); err == nil {
			online = true
		}
	}
	status := "未检测"
	if checked && online {
		status = "正常"
	} else if checked {
		status = "不可用"
	}
	lines := []string{
		"Emby 状态",
		"",
		"账号: " + u.Username,
		"本地状态: " + telegramActiveLabel(u.Active),
		"到期: " + expireStatus(u.ExpiredAt),
		"Emby 绑定: " + telegramEmbyLabel(u),
		"服务器配置: " + telegramConfiguredLabel(a.cfg.EmbyURL != ""),
		"连通性: " + status,
	}
	_ = a.telegramSendMessage(ctx, chatID, strings.Join(lines, "\n"))
}

func (a *App) telegramHandlePlayInfo(ctx context.Context, chatID, telegramID int64) {
	u, okUser := a.store.FindUserByTelegramID(telegramID)
	if !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	since := time.Now().AddDate(0, 0, -30).Unix()
	records := a.store.PlaybackRecords(u.UID, since, 1000)
	totalDuration := int64(0)
	for _, record := range records {
		totalDuration += record.Duration
	}
	lines := []string{
		"近 30 天播放统计",
		"",
		"账号: " + u.Username,
		"播放次数: " + strconv.Itoa(len(records)),
		"播放时长: " + formatSeconds(totalDuration),
	}
	if len(records) > 0 {
		lines = append(lines, "", "最近播放:")
		for i, record := range records {
			if i >= 5 {
				break
			}
			title := firstNonEmpty(record.Title, record.ItemID, "未知条目")
			lines = append(lines, fmt.Sprintf("- %s / %s / %s", truncateString(title, 48), firstNonEmpty(record.MediaType, "unknown"), formatSeconds(record.Duration)))
		}
	}
	_ = a.telegramSendMessage(ctx, chatID, strings.Join(lines, "\n"))
}

func (a *App) telegramHandleResetPassword(ctx context.Context, chatID, telegramID int64) {
	if _, okUser := a.store.FindUserByTelegramID(telegramID); !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "请在 Web 端的账号安全页面修改密码。Bot 不接收、不生成也不发送密码。")
}

func (a *App) telegramHandleStats(ctx context.Context, chatID, telegramID int64) {
	if !a.telegramAdminID(telegramID) {
		_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
		return
	}
	users := a.store.ListUsers()
	active := 0
	embyBound := 0
	telegramBound := 0
	pendingEmby := 0
	for _, u := range users {
		if u.Active {
			active++
		}
		if u.EmbyID != "" {
			embyBound++
		}
		if u.TelegramID != 0 {
			telegramBound++
		}
		if u.PendingEmby {
			pendingEmby++
		}
	}
	regcodes := a.store.ListRegCodes()
	inviteCodes := a.store.ListAllInviteCodes()
	text := fmt.Sprintf("服务统计\n\n用户总数: %d\n活跃用户: %d\nTelegram 已绑定: %d\nEmby 已绑定: %d\n待开通 Emby: %d\n注册码: %d\n邀请码: %d", len(users), active, telegramBound, embyBound, pendingEmby, len(regcodes), len(inviteCodes))
	_ = a.telegramSendMessage(ctx, chatID, text)
}

func (a *App) telegramHandleAdmin(ctx context.Context, chatID, telegramID int64) {
	if !a.telegramAdminID(telegramID) {
		_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "管理员查询入口\n\n/stats 服务统计\n/userinfo <关键词> 查看单个用户\n/twfind <关键词> 搜索用户\n/twishelp 查看管理员帮助\n\n涉及写入、删除、密码和服务重启的操作请在 Web 后台完成。")
}

func (a *App) telegramHandleUserInfo(ctx context.Context, chatID, telegramID int64, query string) {
	if !a.telegramAdminID(telegramID) {
		_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
		return
	}
	if strings.TrimSpace(query) == "" {
		_ = a.telegramSendMessage(ctx, chatID, "请发送 /userinfo <用户名/UID/关键词>")
		return
	}
	users := a.telegramFindUsers(query, 6)
	if len(users) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "未找到匹配用户。")
		return
	}
	if len(users) > 1 {
		_ = a.telegramSendMessage(ctx, chatID, "找到多个匹配项，请缩小关键词。\n\n"+telegramUserList(users))
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "用户详情\n\n"+telegramUserSummary(users[0]))
}

func (a *App) telegramHandleFind(ctx context.Context, chatID, telegramID int64, query string) {
	if !a.telegramAdminID(telegramID) {
		_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
		return
	}
	if strings.TrimSpace(query) == "" {
		_ = a.telegramSendMessage(ctx, chatID, "请发送 /twfind <用户名/UID/关键词>")
		return
	}
	users := a.telegramFindUsers(query, 10)
	if len(users) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "未找到匹配用户。")
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "用户搜索结果\n\n"+telegramUserList(users))
}

func (a *App) telegramHandleGroupUser(ctx context.Context, chatID, telegramID int64, fields []string, message map[string]any) {
	if !a.telegramAdminID(telegramID) {
		_ = a.telegramSendMessage(ctx, chatID, "没有管理员权限。")
		return
	}
	query := strings.Join(fields[1:], " ")
	if strings.TrimSpace(query) == "" {
		if reply, _ := message["reply_to_message"].(map[string]any); reply != nil {
			if from, _ := reply["from"].(map[string]any); from != nil {
				if id := numeric(from["id"]); id != 0 {
					if u, okUser := a.store.FindUserByTelegramID(id); okUser {
						_ = a.telegramSendMessage(ctx, chatID, "群组用户查询\n\n"+telegramUserSummary(u))
						return
					}
				}
			}
		}
		_ = a.telegramSendMessage(ctx, chatID, "请回复目标用户消息后发送 /twguser，或发送 /twguser <用户名/UID/关键词>。")
		return
	}
	users := a.telegramFindUsers(query, 6)
	if len(users) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "未找到匹配用户。")
		return
	}
	if len(users) > 1 {
		_ = a.telegramSendMessage(ctx, chatID, "找到多个匹配项，请缩小关键词。\n\n"+telegramUserList(users))
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "群组用户查询\n\n"+telegramUserSummary(users[0]))
}

func (a *App) telegramAdminID(telegramID int64) bool {
	for _, id := range a.cfg.TelegramAdminIDs {
		if id == telegramID {
			return true
		}
	}
	if u, okUser := a.store.FindUserByTelegramID(telegramID); okUser && u.Role == store.RoleAdmin {
		return true
	}
	return false
}

func (a *App) telegramRequirePrivate(ctx context.Context, chatID int64, private bool) bool {
	if private {
		return true
	}
	_ = a.telegramSendMessage(ctx, chatID, "请私聊 Bot 使用该命令，避免泄露账号信息。")
	return false
}

func telegramCommand(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.IndexByte(raw, '@'); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.ToLower(raw)
}

func (a *App) telegramCustomCommandReply(command string) (string, bool) {
	command = telegramCommand(command)
	if command == "" || !strings.HasPrefix(command, "/") {
		return "", false
	}
	for _, item := range a.cfg.TelegramCustomCommands {
		if telegramCommand(item.Command) == command && strings.TrimSpace(item.Reply) != "" {
			return item.Reply, true
		}
	}
	return "", false
}

func (a *App) telegramStartText() string {
	if text := strings.TrimSpace(a.cfg.TelegramBotStartText); text != "" {
		return a.telegramRenderText(text)
	}
	title := firstNonEmpty(strings.TrimSpace(a.cfg.TelegramBotStartTitle), "Twilight Bot")
	intro := firstNonEmpty(strings.TrimSpace(a.cfg.TelegramBotStartIntro), "用于绑定 Telegram、查看账号状态和接收服务通知。")
	return strings.Join([]string{
		title,
		"",
		intro,
		"",
		"常用命令:",
		"/bind <绑定码> 绑定 Telegram",
		"/me 查看当前绑定",
		"/emby 查看 Emby 状态",
		"/playinfo 查看播放统计",
		"/help 查看完整帮助",
	}, "\n")
}

func (a *App) telegramGroupPrompt() string {
	if text := strings.TrimSpace(a.cfg.TelegramBotGroupStartText); text != "" {
		return a.telegramRenderText(text)
	}
	return "为避免泄露账号信息，请私聊 Bot 使用 /bind、/me、/emby 等账号命令。管理员可在群内使用 /twguser 进行只读查询。"
}

func (a *App) telegramBindPrompt() string {
	if text := strings.TrimSpace(a.cfg.TelegramBotBindPromptText); text != "" {
		return a.telegramRenderText(text)
	}
	return "请发送 /bind <绑定码>。绑定码需要先在 Web 端生成，有效期较短。"
}

func (a *App) telegramHelpText(admin bool) string {
	if text := strings.TrimSpace(a.cfg.TelegramBotHelpText); text != "" {
		return a.telegramRenderText(text)
	}
	lines := []string{}
	if header := strings.TrimSpace(a.cfg.TelegramBotHelpHeader); header != "" {
		lines = append(lines, a.telegramRenderText(header), "")
	}
	lines = append(lines,
		"Twilight Bot 帮助",
		"",
		"用户命令:",
		"/start 打开帮助入口",
		"/bind <绑定码> 绑定 Telegram",
		"/me 查看当前绑定",
		"/emby 查看 Emby 状态",
		"/playinfo 查看近 30 天播放统计",
		"/resetpwd 查看密码修改说明",
		"/cancel 取消当前 Bot 操作",
		"/about 查看服务说明",
	)
	if admin {
		lines = append(lines, "", "管理员命令:", "/stats 服务统计", "/userinfo <关键词> 查看单个用户", "/twfind <关键词> 搜索用户", "/twishelp 管理员帮助")
	}
	if footer := strings.TrimSpace(a.cfg.TelegramBotHelpFooter); footer != "" {
		lines = append(lines, "", a.telegramRenderText(footer))
	}
	return strings.Join(lines, "\n")
}

func (a *App) telegramAdminHelpText() string {
	if text := strings.TrimSpace(a.cfg.TelegramBotAdminHelpText); text != "" {
		return a.telegramRenderText(text)
	}
	return "管理员帮助\n\n/stats 服务统计\n/userinfo <用户名/UID/关键词> 查看用户详情\n/twfind <用户名/UID/关键词> 搜索用户\n/twguser <关键词> 群组只读查询\n/twguser 回复群成员消息时按 Telegram 绑定关系查询\n\nBot 管理命令只展示非敏感摘要；用户写入、删除、封禁、密码和系统更新请在 Web 后台完成。"
}

func (a *App) telegramAboutText() string {
	if text := strings.TrimSpace(a.cfg.TelegramBotAbout); text != "" {
		return a.telegramRenderText(text)
	}
	return a.cfg.AppName + "\n\nTelegram Bot 仅用于绑定、查询、统计和通知。不会通过 Telegram 展示密码、Token、Emby ID 或服务器线路。"
}

func (a *App) telegramRenderText(text string) string {
	text = strings.ReplaceAll(text, "{server_name}", a.cfg.AppName)
	text = strings.ReplaceAll(text, "{bot_username}", "")
	text = strings.ReplaceAll(text, "{user_name}", "")
	return text
}

func (a *App) telegramFindUsers(query string, limit int) []store.User {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	lower := strings.ToLower(query)
	out := []store.User{}
	for _, u := range a.store.ListUsers() {
		if telegramUserMatches(u, lower) {
			out = append(out, u)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func telegramUserMatches(u store.User, query string) bool {
	if strconv.FormatInt(u.UID, 10) == query {
		return true
	}
	if u.TelegramID != 0 && strconv.FormatInt(u.TelegramID, 10) == query {
		return true
	}
	fields := []string{u.Username, u.Email, u.TelegramUsername, u.EmbyUsername, u.EmbyID}
	for _, field := range fields {
		if field != "" && strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func telegramUserSummary(u store.User) string {
	return strings.Join([]string{
		"账号: " + u.Username,
		"UID: " + strconv.FormatInt(u.UID, 10),
		"角色: " + roleName(u.Role),
		"状态: " + telegramActiveLabel(u.Active),
		"到期: " + expireStatus(u.ExpiredAt),
		"Telegram: " + telegramBoundLabel(u.TelegramID != 0),
		"Emby: " + telegramEmbyLabel(u),
		"待开通 Emby: " + telegramYesNoLabel(u.PendingEmby),
	}, "\n")
}

func telegramUserList(users []store.User) string {
	lines := make([]string, 0, len(users))
	for _, u := range users {
		lines = append(lines, fmt.Sprintf("- UID %d / %s / %s / TG %s / Emby %s", u.UID, u.Username, roleName(u.Role), telegramBoundLabel(u.TelegramID != 0), telegramEmbyLabel(u)))
	}
	return strings.Join(lines, "\n")
}

func telegramActiveLabel(active bool) string {
	if active {
		return "启用"
	}
	return "禁用"
}

func telegramConfiguredLabel(ok bool) string {
	if ok {
		return "已配置"
	}
	return "未配置"
}

func telegramBoundLabel(ok bool) string {
	if ok {
		return "已绑定"
	}
	return "未绑定"
}

func telegramYesNoLabel(ok bool) string {
	if ok {
		return "是"
	}
	return "否"
}

func telegramEmbyLabel(u store.User) string {
	if u.EmbyID != "" {
		return "已绑定"
	}
	if u.PendingEmby {
		return "待开通"
	}
	return "未绑定"
}
