package api

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

var telegramBindCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{6,16}$`)

func (a *App) RunTelegramBot(ctx context.Context) error {
	// 协程入口加 panic recover：单条 update 处理崩溃不应让整个 bot 退出。
	defer func() {
		if r := recover(); r != nil {
			// 改用 zap.String：panic value 反射 dump 可能含 token / chat 私密内容；
			// 走脱敏分支保证不绕过 sensitiveLogKey / redactSensitiveText 链路。
			zap.L().Error("telegram bot panic", zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
		}
	}()
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
			a.setTelegramRuntimeStatus(false, fmt.Errorf("Telegram bot is disabled or token is not configured"))
			zap.L().Info("Telegram bot configuration disabled; waiting before next config check")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		currentConfig := strings.TrimSpace(a.cfg().TelegramAPIURL) + "|" + strings.TrimSpace(a.cfg().TelegramBotToken)
		if currentConfig != activeConfig {
			me, err := a.telegramGetMe(ctx)
			if err != nil {
				a.setTelegramRuntimeStatus(false, err)
				zap.L().Warn("Telegram bot initialization failed", zap.Error(err))
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(3 * time.Second):
					continue
				}
			}
			activeConfig = currentConfig
			offset = 0
			a.setTelegramRuntimeStatus(true, nil)
			zap.L().Info("Telegram bot polling started", zap.String("username", asString(me["username"])))
		}
		updates, err := a.telegramGetUpdates(ctx, offset)
		if err != nil {
			a.setTelegramRuntimeStatus(true, err)
			zap.L().Warn("Telegram getUpdates failed", zap.Error(err))
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
				continue
			}
		}
		for _, update := range updates {
			a.setTelegramRuntimeStatus(true, nil)
			if id := numeric(update["update_id"]); id >= offset {
				offset = id + 1
			}
			func(u map[string]any) {
				// 单条 update 处理 panic 隔离：一条坏消息不能让整个 bot 退出。
				defer func() {
					if r := recover(); r != nil {
						// update_id 走 numeric 提取避免 zap.Any 反射；panic value 同理强制
						// 经 redact 转字符串，避免 reflect 路径输出原始 chat / user 数据。
						zap.L().Error("telegram update panic", zap.Int64("update_id", int64(numeric(u["update_id"]))), zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
					}
				}()
				a.handleTelegramUpdate(ctx, u)
			}(update)
		}
	}
}

func (a *App) telegramGetUpdates(ctx context.Context, offset int64) ([]map[string]any, error) {
	var result []map[string]any
	body := map[string]any{"timeout": 30, "allowed_updates": []string{"message", "callback_query", "chat_member", "my_chat_member"}}
	if offset > 0 {
		body["offset"] = offset
	}
	err := a.telegramPostWithTimeout(ctx, "getUpdates", body, &result, 45*time.Second)
	return result, err
}

func (a *App) handleTelegramUpdate(ctx context.Context, update map[string]any) {
	a.observeTelegramRoster(update)
	if callback, _ := update["callback_query"].(map[string]any); callback != nil {
		a.telegramHandleCallback(ctx, callback)
		return
	}
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

	// 先把"私聊 + 普通 gating"的标准命令交给注册表统一分发，dispatcher 内部
	// 集中处理 private/admin 校验，避免每个 case 重复 telegramRequirePrivate +
	// telegramAdminID 模板。
	cmdCtx := telegramCommandCtx{ChatID: chatID, FromID: fromID, Username: username, Args: fields[1:]}
	if a.telegramDispatchRegistry(ctx, command, cmdCtx, privateChat) {
		return
	}

	switch command {
	case "/start", "/help", "/twihelp":
		// 群聊里这三个命令转发"请私聊"提示而不是 gating 失败，
		// /help 还要根据是否管理员渲染不同文本，所以保留 switch 单独处理。
		if !privateChat {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramGroupPrompt())
			return
		}
		if command == "/start" {
			_ = a.telegramSendMessage(ctx, chatID, a.telegramStartText())
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, a.telegramHelpText(a.telegramAdminID(fromID)))
	case "/twguser":
		// 群组管理命令：群内匿名管理员要走 inline 按钮二次鉴权，
		// 私聊也允许，gating 逻辑和注册表的"private + admin"模式不一样。
		a.telegramHandleGroupUser(ctx, chatID, fromID, fields, message)
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
			_ = a.store().UpsertTelegramRoster(fmt.Sprint(chatID), fromID, "member", boolish(from["is_bot"]))
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
			_ = a.store().MarkTelegramRosterLeft(fmt.Sprint(chatID), userID, status)
			return
		}
		_ = a.store().UpsertTelegramRoster(fmt.Sprint(chatID), userID, firstNonEmpty(status, "member"), boolish(user["is_bot"]))
		return
	}
}

func (a *App) telegramConfirmBindCode(ctx context.Context, chatID, telegramID int64, username, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !telegramBindCodePattern.MatchString(code) {
		_ = a.telegramSendMessage(ctx, chatID, "绑定码格式无效，请在 Web 端重新生成后再发送。")
		return
	}
	bind, okBind := a.store().BindCode(code)
	if !okBind || bind.ExpiresAt <= time.Now().Unix() {
		if okBind {
			_ = a.store().DeleteBindCode(code)
		}
		_ = a.telegramSendMessage(ctx, chatID, "绑定码无效或已过期，请在网页重新获取。")
		return
	}
	// 幂等：注册流（bind.UID == 0）下 Confirm 写完不会立刻删 code，
	// bot 命令重发可绕过 group check 反复打 Telegram getChatMember。
	if bind.Confirmed && bind.TelegramID != 0 {
		if bind.TelegramID != telegramID {
			_ = a.telegramSendMessage(ctx, chatID, "该绑定码已被其它 Telegram 账号确认，无法重复使用。")
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, "Telegram 绑定已确认，可以回到网页继续。")
		return
	}
	if existing, okUser := a.store().FindUserByTelegramID(telegramID); okUser && (bind.UID == 0 || existing.UID != bind.UID) {
		_ = a.telegramSendMessage(ctx, chatID, fmt.Sprintf("该 Telegram 已绑定到账号 %s。", existing.Username))
		return
	}
	// per-tg-id 速率限制：阻止用同一个 tg 账号反复对 bot 发未确认的合法 code，
	// 触发对 Telegram API 的 getChatMember 流量放大。
	if !a.allowRate(ctx, rateKey("tg-bind-confirm:", telegramID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		_ = a.telegramSendMessage(ctx, chatID, "操作过于频繁，请稍后再试。")
		return
	}
	if missing, err := a.telegramBindRequirementMissing(ctx, telegramID); err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "Telegram 加群/频道校验失败，请稍后重试或联系管理员："+a.telegramSanitizeError(err))
		return
	} else if len(missing) > 0 {
		_ = a.telegramSendMessage(ctx, chatID, "绑定前需要先加入指定 Telegram 群组/频道："+strings.Join(missing, ", "))
		return
	}
	bind.Confirmed = true
	bind.TelegramID = telegramID
	bind.TelegramUsername = username
	_ = a.store().UpsertBindCode(bind)
	if bind.UID != 0 {
		defer func() { _ = a.store().DeleteBindCode(code) }()
		_, err := a.store().UpdateUser(bind.UID, func(u *store.User) error {
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
	if u, okUser := a.store().FindUserByTelegramID(telegramID); okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前绑定信息\n\n"+telegramUserSummary(u))
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
}

func (a *App) telegramHandleEmby(ctx context.Context, chatID, telegramID int64) {
	u, okUser := a.store().FindUserByTelegramID(telegramID)
	if !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	online := false
	checked := false
	if strings.TrimSpace(a.cfg().EmbyURL) != "" {
		checked = true
		// 5s ctx 走 embyHealth：双段 fallback 由 helper 集中处理。
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, online = a.embyHealth(checkCtx)
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
		"服务器配置: " + telegramConfiguredLabel(a.cfg().EmbyURL != ""),
		"连通性: " + status,
	}
	_ = a.telegramSendMessage(ctx, chatID, strings.Join(lines, "\n"))
}

func (a *App) telegramHandlePlayInfo(ctx context.Context, chatID, telegramID int64) {
	u, okUser := a.store().FindUserByTelegramID(telegramID)
	if !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	since := time.Now().AddDate(0, 0, -30).Unix()
	records := a.store().PlaybackRecords(u.UID, since, 1000)
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
	if _, okUser := a.store().FindUserByTelegramID(telegramID); !okUser {
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
	users := a.store().ListUsers()
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
	regcodes := a.store().ListRegCodes()
	inviteCodes := a.store().ListAllInviteCodes()
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
	messageID := numeric(message["message_id"])
	anonymousCommand := telegramIsAnonymousGroupMessage(message)
	if anonymousCommand {
		a.telegramSendGroupAdminAuth(ctx, chatID, messageID, fields, message)
		return
	}
	if !a.telegramAdminID(telegramID) {
		a.telegramSendUnauthorizedAndCleanup(ctx, chatID, messageID)
		return
	}
	user, reason := a.telegramResolveGroupUserTarget(fields, message)
	if reason != "" {
		_ = a.telegramSendMessage(ctx, chatID, reason)
		return
	}
	a.telegramSendGroupUserPanel(ctx, chatID, messageID, user, false)
}

func (a *App) telegramAdminID(telegramID int64) bool {
	for _, id := range a.cfg().TelegramAdminIDs {
		if id == telegramID {
			return true
		}
	}
	if u, okUser := a.store().FindUserByTelegramID(telegramID); okUser && u.Role == store.RoleAdmin {
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
	for _, item := range a.cfg().TelegramCustomCommands {
		if telegramCommand(item.Command) == command && strings.TrimSpace(item.Reply) != "" {
			return item.Reply, true
		}
	}
	return "", false
}

func (a *App) telegramStartText() string {
	if text := strings.TrimSpace(a.cfg().TelegramBotStartText); text != "" {
		return a.telegramRenderText(text)
	}
	title := firstNonEmpty(strings.TrimSpace(a.cfg().TelegramBotStartTitle), "Twilight Bot")
	intro := firstNonEmpty(strings.TrimSpace(a.cfg().TelegramBotStartIntro), "用于绑定 Telegram、查看账号状态和接收服务通知。")
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
	if text := strings.TrimSpace(a.cfg().TelegramBotGroupStartText); text != "" {
		return a.telegramRenderText(text)
	}
	return "为避免泄露账号信息，请私聊 Bot 使用 /bind、/me、/emby 等账号命令。管理员可在群内使用 /twguser 打开用户管理面板。"
}

func (a *App) telegramBindPrompt() string {
	if text := strings.TrimSpace(a.cfg().TelegramBotBindPromptText); text != "" {
		return a.telegramRenderText(text)
	}
	return "请发送 /bind <绑定码>。绑定码需要先在 Web 端生成，有效期较短。"
}

func (a *App) telegramHelpText(admin bool) string {
	if text := strings.TrimSpace(a.cfg().TelegramBotHelpText); text != "" {
		return a.telegramRenderText(text)
	}
	lines := []string{}
	if header := strings.TrimSpace(a.cfg().TelegramBotHelpHeader); header != "" {
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
	if footer := strings.TrimSpace(a.cfg().TelegramBotHelpFooter); footer != "" {
		lines = append(lines, "", a.telegramRenderText(footer))
	}
	return strings.Join(lines, "\n")
}

func (a *App) telegramAdminHelpText() string {
	if text := strings.TrimSpace(a.cfg().TelegramBotAdminHelpText); text != "" {
		return a.telegramRenderText(text)
	}
	return "管理员帮助\n\n/stats 服务统计\n/userinfo <用户名/UID/关键词> 查看用户详情\n/twfind <用户名/UID/关键词> 搜索用户\n/twguser <关键词> 群组用户管理面板\n/twguser 回复群成员消息时按 Telegram 绑定关系查询\n\n群组匿名管理员使用 /twguser 时需要通过 inline 按钮验证身份；每次按钮操作都会重新鉴权。"
}

func (a *App) telegramAboutText() string {
	if text := strings.TrimSpace(a.cfg().TelegramBotAbout); text != "" {
		return a.telegramRenderText(text)
	}
	return a.cfg().AppName + "\n\nTelegram Bot 仅用于绑定、查询、统计和通知。不会通过 Telegram 展示密码、Token、Emby ID 或服务器线路。"
}

func (a *App) telegramRenderText(text string) string {
	text = strings.ReplaceAll(text, "{server_name}", a.cfg().AppName)
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
	for _, u := range a.store().ListUsers() {
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
