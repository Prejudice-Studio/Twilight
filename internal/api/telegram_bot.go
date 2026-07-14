package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

var telegramBindCodePattern = regexp.MustCompile(`^[A-Za-z0-9]{6,16}$`)

// delAccountPendingState 记录 /delAccount emby 两步验证的中间状态。
type delAccountPendingState struct {
	TelegramID  int64
	ChatID      int64
	UserUID     int64
	WebVerified bool
	SkipEmby    bool
	ExpiresAt   int64
	Reason      string // 用户可选的删除原因
}

func delAccountPendingKey(chatID, fromID int64) string {
	return fmt.Sprintf("%d:%d", chatID, fromID)
}

func (a *App) saveDelAccountPending(state *delAccountPendingState) {
	a.delAccountPendingMu.Lock()
	defer a.delAccountPendingMu.Unlock()
	if a.delAccountPending == nil {
		a.delAccountPending = map[string]*delAccountPendingState{}
	}
	a.delAccountPending[delAccountPendingKey(state.ChatID, state.TelegramID)] = state
}

func (a *App) takeDelAccountPending(chatID, telegramID int64) *delAccountPendingState {
	key := delAccountPendingKey(chatID, telegramID)
	a.delAccountPendingMu.Lock()
	defer a.delAccountPendingMu.Unlock()
	state, ok := a.delAccountPending[key]
	if !ok || state.ExpiresAt < time.Now().Unix() {
		delete(a.delAccountPending, key)
		return nil
	}
	delete(a.delAccountPending, key)
	return state
}

func (a *App) peekDelAccountPending(chatID, telegramID int64) *delAccountPendingState {
	key := delAccountPendingKey(chatID, telegramID)
	a.delAccountPendingMu.Lock()
	defer a.delAccountPendingMu.Unlock()
	state, ok := a.delAccountPending[key]
	if !ok || state.ExpiresAt < time.Now().Unix() {
		delete(a.delAccountPending, key)
		return nil
	}
	return state
}

func (a *App) clearDelAccountPending(chatID, telegramID int64) {
	key := delAccountPendingKey(chatID, telegramID)
	a.delAccountPendingMu.Lock()
	defer a.delAccountPendingMu.Unlock()
	delete(a.delAccountPending, key)
}

func (a *App) RunTelegramBot(ctx context.Context) error {
	// 协程入口加 panic recover：单条 update 处理崩溃不应让整个 bot 退出。
	defer func() {
		if r := recover(); r != nil {
			// 改用 zap.String：panic value 反射 dump 可能含 token / chat 私密内容；
			// 走脱敏分支保证不绕过 sensitiveLogKey / redactSensitiveText 链路。
			zap.L().Error("telegram bot panic", zap.String("panic", redactSensitiveText(fmt.Sprintf("%v", r))))
		}
	}()
	// 启动时从 store 恢复上一次成功 ack 的 offset，避免进程重启后把 24h 内
	// 已经处理过的 update 重新分发一遍（telegramConfirmBindCode 这类有副作用
	// 的命令会被重放）。0 = 历史 state / 新部署，按 getUpdates 默认行为走。
	offset := a.store().TelegramBotOffset()
	activeBot := ""
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
		// botIdentity = 当前 bot 的 username。token 改了但 bot 实体（同一
		// @bot_name）不变时复用旧 offset 是正确的；只有 bot 实体真的换了才
		// 必须 reset，否则会把新 bot 的真实 update 当成"已处理"丢掉。
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
		botIdentity := strings.TrimSpace(asString(me["username"]))
		if botIdentity != "" && activeBot != "" && botIdentity != activeBot {
			// bot 实体切换：旧 offset 和新 bot 的 update 序列没有任何关系，
			// 必须 reset 否则会跳过新 bot 的真实初始 update。
			if err := a.store().ResetTelegramBotOffset(); err != nil {
				zap.L().Warn("reset telegram offset failed", zap.Error(err))
			}
			offset = 0
		}
		if botIdentity != "" && botIdentity != activeBot {
			activeBot = botIdentity
			a.setTelegramRuntimeStatus(true, nil)
			zap.L().Info("Telegram bot polling started", zap.String("username", botIdentity), zap.Int64("resume_offset", offset))
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
		// 整个 batch 处理完成才推进持久化 offset：一旦写入成功，下一次重启
		// 不会再分发到已 ack 的 update。SetTelegramBotOffset 内部做单调防退化，
		// 偶尔的 store 写失败只是不更新 disk，下一轮 batch 会再尝试。
		if len(updates) > 0 {
			if err := a.store().SetTelegramBotOffset(offset); err != nil {
				zap.L().Warn("persist telegram offset failed", zap.Error(err), zap.Int64("offset", offset))
			}
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
		if a.telegramHandleDeveloperJSCallback(ctx, callback) {
			return
		}
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
	if a.telegramConsumeDeveloperJSWaiter(ctx, chatID, fromID, text) {
		return
	}
	// 拦截非命令文本：delAccount 两步验证（Web密码→Emby密码）
	if !strings.HasPrefix(text, "/") && a.telegramConsumeDelAccountPending(ctx, chatID, fromID, text) {
		return
	}
	fields := strings.Fields(text)
	command := telegramCommand(fields[0])

	// 先把"私聊 + 普通 gating"的标准命令交给注册表统一分发，dispatcher 内部
	// 集中处理 private/admin 校验，避免每个 case 重复 telegramRequirePrivate +
	// telegramAdminID 模板。
	cmdCtx := telegramCommandCtx{ChatID: chatID, FromID: fromID, Username: username, Command: command, Args: fields[1:]}
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
		if a.telegramHandleCustomCommand(ctx, command, cmdCtx, privateChat) {
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
		// 私聊 + 群聊都顺手刷新已绑定用户的 Telegram 用户名（无额外 API 调用）。
		a.refreshTelegramUsername(fromID, asString(from["username"]))
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
		a.refreshTelegramUsername(userID, asString(user["username"]))
		if status == "left" || status == "kicked" {
			_ = a.store().MarkTelegramRosterLeft(fmt.Sprint(chatID), userID, status)
			return
		}
		_ = a.store().UpsertTelegramRoster(fmt.Sprint(chatID), userID, firstNonEmpty(status, "member"), boolish(user["is_bot"]))
		return
	}
}

// refreshTelegramUsername 在收到任意来自已绑定用户的更新时被动刷新其存储的
// Telegram 用户名。只有解析到绑定账号、且新用户名非空并与现存不同才写库，避免
// 无谓写盘；用户名为空（对方删了 @username）时保留旧值不清空，以免破坏指名码匹配。
func (a *App) refreshTelegramUsername(telegramID int64, rawUsername string) {
	if telegramID <= 0 {
		return
	}
	username := strings.TrimPrefix(strings.TrimSpace(rawUsername), "@")
	if username == "" {
		return
	}
	u, ok := a.store().FindUserByTelegramID(telegramID)
	if !ok || u.TelegramUsername == username {
		return
	}
	_, _ = a.store().UpdateUser(u.UID, func(cur *store.User) error {
		cur.TelegramUsername = username
		return nil
	})
}

func (a *App) telegramConfirmBindCode(ctx context.Context, chatID, telegramID int64, username, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !telegramBindCodePattern.MatchString(code) {
		_ = a.telegramSendMessage(ctx, chatID, "绑定码格式无效，请在 Web 端重新生成后再发送。")
		return
	}
	// 绑定码只活在 API 进程的 in-memory hub 里。本进程能查到该码（all 模式：
	// Bot 与 API 同进程共享 hub）才在本地完成 gating + confirm；查不到说明是
	// 独立 Bot 进程，统一委托 API 进程的 HTTP 端点处理。
	//
	// 关键点：gating（限速 / 加群校验）只做一次。独立 Bot 进程不再本地预检——
	// handleBindConfirmSecure 已经做同样的限速 + 加群校验，并且会把失败写回
	// API 进程的 hub（网页端状态轮询读的就是这个 hub）。两进程各做一遍会重复
	// getChatMember、并且 Bot 侧预检失败时根本不调 API，网页端就只能一直 pending。
	bind, okBind := a.bindCode(code)
	if !okBind || bind.ExpiresAt <= time.Now().Unix() {
		a.confirmBindCodeViaHTTP(ctx, chatID, code, telegramID, username)
		return
	}
	// 本进程持有该绑定码（all 模式）：就地 gating，并把 gating 失败写回 hub，
	// 让网页端轮询看到终态失败而不是一直 pending——与 HTTP 委托路径表现一致。
	if !a.allowRate(ctx, rateKey("tg-bind-confirm:", telegramID), a.cfg().RateLimitLoginPerMinute, time.Minute) {
		a.recordRegisterBindFailure(bind, code, "rate_limited", ErrUploadRateLimited, http.StatusTooManyRequests, "操作过于频繁，请稍后再试")
		_ = a.telegramSendMessage(ctx, chatID, "操作过于频繁，请稍后再试。")
		return
	}
	if missing, err := a.telegramBindRequirementMissing(ctx, telegramID); err != nil {
		a.recordRegisterBindFailure(bind, code, "group_check_failed", ErrTGBindGroupCheckFailed, http.StatusBadGateway, "Telegram 加群/频道校验失败，请稍后重试")
		_ = a.telegramSendMessage(ctx, chatID, "Telegram 加群/频道校验失败，请稍后重试或联系管理员。")
		return
	} else if len(missing) > 0 {
		a.recordRegisterBindFailure(bind, code, "group_membership_required", ErrTGBindGroupMembershipRequired, http.StatusForbidden, "绑定前需要先加入指定 Telegram 群组/频道: "+strings.Join(missing, ", "))
		_ = a.telegramSendMessage(ctx, chatID, "绑定前需要先加入指定 Telegram 群组/频道："+strings.Join(missing, ", "))
		return
	}
	a.confirmBindCodeInline(ctx, chatID, bind, code, telegramID, username)
}

func (a *App) confirmBindCodeInline(ctx context.Context, chatID int64, bind store.BindCode, code string, telegramID int64, username string) {
	if bind.Confirmed && bind.TelegramID != 0 {
		if bind.TelegramID != telegramID {
			// 与 handleBindConfirmSecure 的重放保护一致：把"已绑其他 TG"写回 hub，
			// 让网页端状态轮询能看到终态失败而不是一直 pending。
			a.recordRegisterBindFailure(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "绑定码已绑定其他 Telegram，无法重放")
			_ = a.telegramSendMessage(ctx, chatID, "该绑定码已被其它 Telegram 账号确认，无法重复使用。")
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, "Telegram 绑定已确认，可以回到网页继续。")
		return
	}
	_, _, _, err := a.confirmBindCodeAtomic(code, telegramID, username, time.Now().Unix())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrExpired) {
			_ = a.telegramSendMessage(ctx, chatID, "绑定码无效或已过期，请在网页重新获取。")
			return
		}
		if errors.Is(err, store.ErrConflict) {
			a.recordRegisterBindFailure(bind, code, "telegram_taken", ErrTGBindTargetTaken, http.StatusConflict, "该 Telegram 已绑定到其他账号或绑定码状态已变化")
			_ = a.telegramSendMessage(ctx, chatID, "该 Telegram 已被其他账号绑定。")
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, "绑定失败，请稍后重试。")
		return
	}
	a.clearRegisterBindFailure(code)
	_ = a.telegramSendMessage(ctx, chatID, "Telegram 绑定已确认，可以回到网页继续。")
}

func (a *App) confirmBindCodeViaHTTP(ctx context.Context, chatID int64, code string, telegramID int64, username string) {
	port := a.cfg().Port
	if port <= 0 {
		port = 5000
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/v1/users/me/telegram/bind-confirm", port)
	reqBody := map[string]any{"code": code, "telegram_id": telegramID, "telegram_username": username}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "绑定请求异常，请稍后重试。")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// 不再附带共享密钥：服务端按“回环直连 + 无反代头”判定为同机内部调用。
	// Bot 直连 127.0.0.1，Go http 客户端默认不会加任何 X-Forwarded-* 头，天然满足。

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	if err := doJSONRequestWithTimeout(req, &resp, 5*time.Second); err != nil {
		zap.L().Warn("telegram bind code HTTP fallback failed", zap.String("code", code), zap.Int64("tgid", telegramID), zap.Error(err))
		_ = a.telegramSendMessage(ctx, chatID, "绑定请求异常，请稍后重试。若问题持续，请联系管理员。")
		return
	}
	if resp.Success {
		_ = a.telegramSendMessage(ctx, chatID, "Telegram 绑定已确认，可以回到网页继续。")
		return
	}
	switch resp.Code {
	case http.StatusNotFound:
		_ = a.telegramSendMessage(ctx, chatID, "绑定码无效或已过期，请在网页重新获取。")
	case http.StatusConflict:
		_ = a.telegramSendMessage(ctx, chatID, "该 Telegram 已被其他账号绑定。")
	case http.StatusTooManyRequests:
		_ = a.telegramSendMessage(ctx, chatID, "操作过于频繁，请稍后再试。")
	case http.StatusForbidden:
		// 加群/频道校验未通过：透传 API 的群组列表文案（已含具体群名）。
		_ = a.telegramSendMessage(ctx, chatID, firstNonEmpty(resp.Message, "绑定前需要先加入指定 Telegram 群组/频道。"))
	case http.StatusBadGateway:
		_ = a.telegramSendMessage(ctx, chatID, firstNonEmpty(resp.Message, "Telegram 加群/频道校验失败，请稍后重试。"))
	default:
		if resp.Message != "" {
			_ = a.telegramSendMessage(ctx, chatID, "绑定失败："+resp.Message)
		} else {
			_ = a.telegramSendMessage(ctx, chatID, "绑定失败，请稍后重试。")
		}
	}
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

func (a *App) telegramHandleResetPassword(ctx context.Context, chatID, telegramID int64) {
	if _, okUser := a.store().FindUserByTelegramID(telegramID); !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "请在 Web 端的账号安全页面修改密码。Bot 不接收、不生成也不发送密码。")
}

// telegramConsumeDelAccountPending 消费 delAccount 两步验证中的非命令文本。
// 用户发送 Web 密码后，Bot 在此验证并进入 Emby 密码等待状态。
func (a *App) telegramConsumeDelAccountPending(ctx context.Context, chatID, fromID int64, text string) bool {
	state := a.peekDelAccountPending(chatID, fromID)
	if state == nil {
		return false
	}
	if !state.WebVerified {
		u, ok := a.store().User(state.UserUID)
		if !ok {
			a.clearDelAccountPending(chatID, fromID)
			_ = a.telegramSendMessage(ctx, chatID, "用户状态异常，请重新发送 /delAccount 开始。")
			return true
		}
		if !verifyPasswordThrottled(text, u.PasswordHash) {
			_ = a.telegramSendMessage(ctx, chatID, "Web 密码错误，请重试。发送 /cancel 取消操作。")
			return true
		}
		if state.SkipEmby {
			a.clearDelAccountPending(chatID, fromID)
			a.telegramExecuteDelAccount(ctx, chatID, u, state.Reason)
			return true
		}
		state.WebVerified = true
		state.ExpiresAt = time.Now().Add(60 * time.Second).Unix()
		a.saveDelAccountPending(state)
		_ = a.telegramSendMessage(ctx, chatID, "Web 密码验证通过。请在 60 秒内发送你的 Emby 密码继续。\n\n发送 /cancel 取消操作。")
		return true
	}
	a.clearDelAccountPending(chatID, fromID)
	u, ok := a.store().User(state.UserUID)
	if !ok {
		_ = a.telegramSendMessage(ctx, chatID, "用户状态异常，请重新发送 /delAccount 开始。")
		return true
	}
	if _, authOK, err := a.embyAuthenticateByName(ctx, u.EmbyUsername, text); err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "Emby 验证服务异常，请稍后重试。")
	} else if !authOK {
		_ = a.telegramSendMessage(ctx, chatID, "Emby 密码验证失败。请重新发送 /delAccount emby 开始。")
	} else {
		a.telegramExecuteDelAccount(ctx, chatID, u, state.Reason)
	}
	return true
}

// telegramHandleDelAccount 用户通过 Telegram 删除自己的账号，支持多因素验证。
// 禁止群聊使用（已在 dispatch 层通过 private=true 强制私聊）。
// 验证优先级：
//  1. 已绑定已验证邮箱 → 必须使用邮箱验证码
//  2. 已绑定 Emby → 需要 Web 密码 + Emby 密码两步验证
//  3. 无绑定 → 直接确认删除
//
// 安全约束：
//   - 管理员/白名单账号拒绝删除
//   - Web 被禁用时拒绝删除，防止通过删号绕过封禁
//   - Emby 被禁用时默认拒绝；邀请关系中的已到期账号可用 Web 密码完成退出清理
//
// 用法：
//
//	/delAccount                     查看可用的验证方式
//	/delAccount <reason>            附带删除原因开始流程
//	/delAccount email               向绑定邮箱发送验证码
//	/delAccount email <code>        验证邮箱验证码
//	/delAccount emby                开始 Web + Emby 密码两步验证
//	/delAccount cancel              取消操作
//	/delAccount confirm             直接确认删除（无邮箱/Emby 时）
func (a *App) telegramHandleDelAccount(ctx context.Context, chatID, telegramID int64, args []string) {
	u, okUser := a.store().FindUserByTelegramID(telegramID)
	if !okUser {
		_ = a.telegramSendMessage(ctx, chatID, "当前 Telegram 尚未绑定 Twilight 账号。")
		return
	}
	if a.userIsProtected(u) {
		_ = a.telegramSendMessage(ctx, chatID, "管理员/白名单账号不支持通过 Telegram 删除，请联系其他管理员。")
		return
	}
	if !u.Active {
		_ = a.telegramSendMessage(ctx, chatID, "你的 Web 账号已被禁用，不允许通过 Telegram 删除。请联系管理员。")
		return
	}
	expiredInviteCleanup := a.inviteExpiredSelfCleanupAllowed(u, time.Now().Unix())
	if u.EmbyDisabled && !expiredInviteCleanup {
		_ = a.telegramSendMessage(ctx, chatID, "你的 Emby 账号已被禁用，不允许通过 Telegram 删除。请联系管理员。")
		return
	}

	hasEmail := u.Email != "" && u.EmailVerified
	hasEmby := u.EmbyID != ""
	emailConfigured := emailConfigured(a.cfg())

	if len(args) == 0 {
		msg := "⚠️ 账号删除确认\n\n此操作将永久删除你的账号，不可恢复！\n你的所有数据（播放记录、设备、邀请关系等）将被删除，注册码使用记录将被匿名化。\n\n"
		if hasEmail && emailConfigured {
			msg += "📧 你已绑定已验证邮箱，必须使用邮箱验证码删除。\n发送 /delAccount email 将验证码发送到你的绑定邮箱。\n\n可选：附带删除原因 /delAccount email 原因说明"
		} else if expiredInviteCleanup {
			msg += "🎬 你的邀请关系 Emby 权益已到期，可通过 Web 密码验证后删除本地账号并完全删除 Emby 账号。\n发送 /delAccount emby 开始验证。\n\n可选：附带删除原因 /delAccount emby 原因说明"
		} else if hasEmby {
			msg += "🎬 你已绑定 Emby 账号，必须通过 Web 密码 + Emby 密码两步验证删除。\n发送 /delAccount emby 开始验证。\n\n可选：附带删除原因 /delAccount emby 原因说明"
		} else {
			msg += "\n由于未绑定邮箱或 Emby，发送 /delAccount confirm 即可直接删除。\n\n可选：附带删除原因 /delAccount confirm 原因说明"
		}
		_ = a.telegramSendMessage(ctx, chatID, msg)
		return
	}

	// 提取删除原因：第一个词如果不是关键字且能以中文/英文开头，当作原因处理
	reason := ""
	actionArgs := args
	if !isKnownDelAccountVerb(args[0]) {
		if len(args) >= 1 {
			reason = strings.Join(args, " ")
		}
		actionArgs = []string{}
		if hasEmail && emailConfigured {
			actionArgs = []string{"email"}
		} else if expiredInviteCleanup {
			actionArgs = []string{"emby"}
		} else if hasEmby {
			actionArgs = []string{"emby"}
		} else {
			actionArgs = []string{"confirm"}
		}
	}

	switch actionArgs[0] {
	case "email":
		if !hasEmail || !emailConfigured {
			_ = a.telegramSendMessage(ctx, chatID, "你未绑定已验证的邮箱，或邮件服务未配置。")
			return
		}
		a.clearDelAccountPending(chatID, telegramID)
		if len(actionArgs) == 1 && reason == "" {
			// 检查是否后面跟着原因
			if len(args) > 1 {
				reason = strings.Join(args[1:], " ")
			}
		}
		if len(actionArgs) == 1 {
			_, _, errCode, errMsg := a.issueEmailCode(ctx, "telegram", emailPurposeDelAccount, u.Email, u.UID)
			if errCode != "" {
				_ = a.telegramSendMessage(ctx, chatID, "发送验证码失败："+errMsg)
				return
			}
			msg := "验证码已发送到你的绑定邮箱，请查收后发送 /delAccount email <验证码>"
			if reason != "" {
				msg += "\n\n删除原因将在验证后提交：" + truncateString(reason, 200)
			}
			_ = a.telegramSendMessage(ctx, chatID, msg)
			return
		}
		if len(actionArgs) == 2 {
			code := actionArgs[1]
			rec, found := a.store().FindActiveEmailVerification(emailPurposeDelAccount, u.Email, time.Now().Unix())
			if !found {
				_ = a.telegramSendMessage(ctx, chatID, "未找到有效的验证码，请重新发送 /delAccount email 获取新验证码。")
				return
			}
			candidateHash := a.hashEmailCode(rec.ID, code)
			_, result, err := a.store().ConsumeEmailVerificationAtomic(rec.ID, candidateHash, time.Now().Unix())
			if err != nil {
				_ = a.telegramSendMessage(ctx, chatID, "验证失败："+err.Error())
				return
			}
			if result == store.EmailVerificationOK {
				a.telegramExecuteDelAccount(ctx, chatID, u, reason)
				return
			}
			_ = a.telegramSendMessage(ctx, chatID, "验证码无效或已过期，请重新发送 /delAccount email 获取新验证码。")
			return
		}
		_ = a.telegramSendMessage(ctx, chatID, "用法：/delAccount email 或 /delAccount email <验证码>")

	case "emby":
		if !hasEmby {
			_ = a.telegramSendMessage(ctx, chatID, "你未绑定 Emby 账号。")
			return
		}
		if hasEmail && emailConfigured {
			_ = a.telegramSendMessage(ctx, chatID, "你已绑定已验证邮箱，必须使用邮箱验证码删除。发送 /delAccount email 开始。")
			return
		}
		if len(actionArgs) > 1 && reason == "" {
			reason = strings.Join(actionArgs[1:], " ")
		}
		a.clearDelAccountPending(chatID, telegramID)
		state := &delAccountPendingState{
			TelegramID:  telegramID,
			ChatID:      chatID,
			UserUID:     u.UID,
			WebVerified: false,
			SkipEmby:    expiredInviteCleanup,
			ExpiresAt:   time.Now().Add(180 * time.Second).Unix(),
			Reason:      reason,
		}
		a.saveDelAccountPending(state)
		msg := "两步验证已启动。请在 3 分钟内发送你的 Web 登录密码以进行第一步验证。\n\n密码不会记录或分享，仅用于验证身份。\n发送 /cancel 取消操作。"
		if expiredInviteCleanup {
			msg = "邀请到期清理已启动。请在 3 分钟内发送你的 Web 登录密码；验证通过后将删除本地账号并完全删除对应 Emby 账号。\n\n密码不会记录或分享，仅用于验证身份。\n发送 /cancel 取消操作。"
		}
		if reason != "" {
			msg += "\n\n删除原因：" + truncateString(reason, 200)
		}
		_ = a.telegramSendMessage(ctx, chatID, msg)

	case "confirm", "force":
		if hasEmail && emailConfigured {
			_ = a.telegramSendMessage(ctx, chatID, "你绑定了已验证邮箱，必须使用邮箱验证码删除。发送 /delAccount email 开始。")
			return
		}
		if expiredInviteCleanup {
			_ = a.telegramSendMessage(ctx, chatID, "你的邀请关系 Emby 权益已到期，需要先验证 Web 密码。发送 /delAccount emby 开始。")
			return
		}
		if hasEmby {
			_ = a.telegramSendMessage(ctx, chatID, "你绑定了 Emby 账号，必须通过 Web 密码 + Emby 密码两步验证删除。发送 /delAccount emby 开始。")
			return
		}
		if len(args) > 1 && reason == "" {
			reason = strings.Join(args[1:], " ")
		}
		a.clearDelAccountPending(chatID, telegramID)
		a.telegramExecuteDelAccount(ctx, chatID, u, reason)

	case "cancel":
		a.clearDelAccountPending(chatID, telegramID)
		_ = a.telegramSendMessage(ctx, chatID, "已取消删除操作。")

	default:
		_ = a.telegramSendMessage(ctx, chatID, "未知参数。请发送 /delAccount 查看可用的验证方式。")
	}
}

// isKnownDelAccountVerb 检查是否是 delAccount 的已知子命令。
func isKnownDelAccountVerb(s string) bool {
	switch s {
	case "email", "emby", "confirm", "force", "cancel":
		return true
	}
	return false
}

// telegramExecuteDelAccount 执行账号删除操作（从本地删除 + 远端 Emby 解绑 + 清除会话）。
// reason 为可选的删除原因，会记入审计日志。
func (a *App) telegramExecuteDelAccount(ctx context.Context, chatID int64, u store.User, reason string) {
	if a.userIsProtected(u) {
		_ = a.telegramSendMessage(ctx, chatID, "受保护账号不允许通过 Telegram 删除。")
		return
	}
	if u.EmbyID != "" && a.embyConfigured() {
		if err := a.embyDelete(ctx, "/Users/"+urlPathEscape(u.EmbyID)); err != nil && !strings.Contains(err.Error(), "remote status 404") {
			zap.L().Warn("telegram delAccount: emby delete failed", zap.Int64("uid", u.UID), zap.Error(err))
			_ = a.telegramSendMessage(ctx, chatID, "删除 Emby 账号失败，请稍后重试或联系管理员。你的本地账号尚未删除。")
			return
		}
	}
	if err := a.deleteLocalUser(ctx, u); err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "删除账号失败："+err.Error())
		return
	}
	detail := map[string]any{"source": "telegram"}
	if reason != "" {
		detail["reason"] = truncateString(reason, 200)
	}
	a.auditEntryIP("telegram", u.UID, u.Username, "self_delete_via_telegram", "user", u.UID, detail)
	msg := "你的账号已永久删除。感谢你的使用，再见。"
	if reason != "" {
		msg += "\n\n已记录的删除原因：" + truncateString(reason, 200)
	}
	_ = a.telegramSendMessage(ctx, chatID, msg)
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

// telegramHandleBanWeb 禁用指定用户的 Web 账号（可选理由）。
// 用法：/banweb <用户名/UID/关键词> [理由]
func (a *App) telegramHandleBanWeb(ctx context.Context, chatID, telegramID int64, args []string) {
	if len(args) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "请发送 /banweb <用户名/UID/关键词> [理由]")
		return
	}
	query := args[0]
	reason := ""
	if len(args) > 1 {
		reason = strings.TrimSpace(strings.Join(args[1:], " "))
		if len(reason) > 200 {
			reason = reason[:200]
		}
	}
	users := a.telegramFindUsers(query, 6)
	if len(users) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "未找到匹配用户。")
		return
	}
	if len(users) > 1 {
		_ = a.telegramSendMessage(ctx, chatID, "找到多个匹配项，请使用 UID 精确指定。\n\n"+telegramUserList(users))
		return
	}
	target := users[0]
	if a.telegramProtectedTarget(target) {
		_ = a.telegramSendMessage(ctx, chatID, "受保护账号禁止通过 Telegram 禁用。")
		return
	}
	updated, err := a.store().SetUserActiveAtomic(target.UID, false)
	if err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "禁用 Web 账号失败: "+err.Error())
		return
	}
	a.sessions().DeleteUser(ctx, updated.UID)
	if _, syncErr := a.disableRemoteEmbyForWebState(ctx, updated); syncErr != nil {
		_ = a.telegramSendMessage(ctx, chatID, "Web 账号已禁用，但 Emby 远端关停失败: "+syncErr.Error())
	} else {
		_ = a.telegramSendMessage(ctx, chatID, "已禁用 Web 账号（Emby 已同步关停）。")
	}
	// 审计日志
	opUID, opName := a.telegramAdminIdentity(telegramID)
	detail := map[string]any{"source": "telegram"}
	if reason != "" {
		detail["reason"] = reason
	}
	a.auditEntryIP("telegram", opUID, opName, "banweb_via_telegram", "admin", target.UID, detail)
}

// telegramHandleBanEmby 单独禁用指定用户的 Emby 账号，不影响 Web 账号（可选理由）。
// 用法：/banemby <用户名/UID/关键词> [理由]
func (a *App) telegramHandleBanEmby(ctx context.Context, chatID, telegramID int64, args []string) {
	if len(args) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "请发送 /banemby <用户名/UID/关键词> [理由]")
		return
	}
	query := args[0]
	reason := ""
	if len(args) > 1 {
		reason = strings.TrimSpace(strings.Join(args[1:], " "))
		if len(reason) > 200 {
			reason = reason[:200]
		}
	}
	users := a.telegramFindUsers(query, 6)
	if len(users) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "未找到匹配用户。")
		return
	}
	if len(users) > 1 {
		_ = a.telegramSendMessage(ctx, chatID, "找到多个匹配项，请使用 UID 精确指定。\n\n"+telegramUserList(users))
		return
	}
	target := users[0]
	if a.telegramProtectedTarget(target) {
		_ = a.telegramSendMessage(ctx, chatID, "受保护账号禁止通过 Telegram 禁用 Emby。")
		return
	}
	if target.EmbyID == "" {
		_ = a.telegramSendMessage(ctx, chatID, "目标用户未绑定 Emby 账号。")
		return
	}
	if !a.embyConfigured() {
		_ = a.telegramSendMessage(ctx, chatID, "Emby URL 或 API Token 未配置，无法操作远端账号。")
		return
	}
	if err := a.embyApplyEnabledState(ctx, target.UID, target.EmbyID, false); err != nil {
		_ = a.telegramSendMessage(ctx, chatID, "禁用 Emby 账号失败: "+telegramPanelSafeError(err))
		return
	}
	_ = a.telegramSendMessage(ctx, chatID, "已禁用 Emby 账号（Web 账号保持不变）。")
	// 审计日志
	opUID, opName := a.telegramAdminIdentity(telegramID)
	detail := map[string]any{"source": "telegram"}
	if reason != "" {
		detail["reason"] = reason
	}
	a.auditEntryIP("telegram", opUID, opName, "banemby_via_telegram", "admin", target.UID, detail)
}

// telegramAdminIdentity 返回 Telegram 管理员在系统中的 UID 和 Username
// （UID=0/Username="" 表示配置管理员未绑定面板账号）。
func (a *App) telegramAdminIdentity(telegramID int64) (int64, string) {
	if u, ok := a.store().FindUserByTelegramID(telegramID); ok {
		return u.UID, u.Username
	}
	return 0, ""
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
	if a.telegramSendGroupUserPanel(ctx, chatID, messageID, user, false) {
		a.telegramDeleteGroupUserCommandMessage(ctx, chatID, messageID)
	}
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
	// 检查是否与内置指令冲突，冲突时拒绝自定义覆盖
	if _, isBuiltIn := telegramCommandRegistry[command]; isBuiltIn {
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
	)
	for _, item := range a.telegramCommandCatalog() {
		if item.Admin || item.Category != "user" || item.Disabled {
			continue
		}
		lines = append(lines, telegramCommandHelpLine(item))
	}
	if admin {
		lines = append(lines, "", "管理员命令:")
		for _, item := range a.telegramCommandCatalog() {
			if !item.Admin || item.Category != "admin" || item.Disabled {
				continue
			}
			lines = append(lines, telegramCommandHelpLine(item))
		}
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
	lines := []string{"管理员帮助", ""}
	for _, item := range a.telegramCommandCatalog() {
		if (!item.Admin && item.Category != "group") || item.Disabled {
			continue
		}
		lines = append(lines, telegramCommandHelpLine(item))
	}
	lines = append(lines,
		"",
		"/twguser 回复群成员消息时按 Telegram 绑定关系查询",
		"",
		"/twguser 面板支持启用/禁用 Web 账号、启用/禁用 Emby、删除 Emby、本地删除用户、移出/封禁群组和授予 7/30/365 天或永久 Emby 注册资格。删除类操作需要二次确认；每次按钮操作都会重新鉴权。群组匿名管理员使用 /twguser 时需要先通过 inline 按钮验证身份。",
	)
	return strings.Join(lines, "\n")
}

func telegramCommandHelpLine(item telegramCommandCatalogItem) string {
	usage := strings.TrimSpace(item.Usage)
	if usage == "" {
		usage = item.Label
	}
	desc := strings.TrimSpace(item.Description)
	if desc == "" {
		return usage
	}
	return usage + " " + desc
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
