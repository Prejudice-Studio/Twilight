package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// telegramCommands 是 Telegram bot 命令注册表，把过去 handleTelegramUpdate
// switch 里"判定 private → 判定 admin → 调用 handler → 重复"的样板抽到这里。
// 设计原则：
//  1. **gating 集中** —— private / admin 由 dispatcher 统一执行，handler 只
//     看到已通过校验的 ctx。新增管理员命令时不会忘记加 admin 检查。
//  2. **handler 签名收敛** —— 全部接收 telegramCommandCtx，参数从 ctx 取，
//     避免每个 case 自己 strings.Join(fields[1:], " ")。
//  3. **特殊命令保留 switch** —— /start /help /twihelp /twguser 这几个有
//     「群组也可用 / 群组转私聊提示 / 群组匿名管理员鉴权」等非典型逻辑，
//     强行塞进 spec 字段会让结构体更乱，所以仍走 switch 分支。
type telegramCommandSpec struct {
	name        string
	label       string
	description string
	usage       string
	category    string
	order       int
	// private 为 true 时，dispatcher 在非私聊场景调用 telegramRequirePrivate
	// 提示并直接返回，handler 不会被执行。
	private bool
	// admin 为 true 时，dispatcher 检查 telegramAdminID(fromID)，
	// 失败发送统一文案"没有管理员权限。"。
	admin bool
	// handler 接收已通过 gating 的 ctx，可直接处理业务。
	handler func(*App, context.Context, telegramCommandCtx)
}

type telegramCommandCatalogItem struct {
	Command     string `json:"command"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
	Category    string `json:"category"`
	Private     bool   `json:"private"`
	Admin       bool   `json:"admin"`
	Disableable bool   `json:"disableable"`
	Disabled    bool   `json:"disabled"`
}

// telegramCommandCtx 把命令分发上下文打包成单一参数，方便 handler 签名收敛、
// 后续扩展（如加 traceID / requestID）也不用回头改所有 handler 签名。
type telegramCommandCtx struct {
	ChatID   int64
	FromID   int64
	Username string
	Command  string
	// Args 是 fields[1:]，handler 自行选择 strings.Join 还是按位置取。
	Args []string
}

var telegramAdminHelpCommandHandler = func(a *App, ctx context.Context, c telegramCommandCtx) {
	_ = a.telegramSendMessage(ctx, c.ChatID, "管理员帮助暂不可用。")
}

func init() {
	telegramAdminHelpCommandHandler = func(a *App, ctx context.Context, c telegramCommandCtx) {
		_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramAdminHelpText())
	}
}

// argString 把 Args 拼成单个查询字符串（多关键词以空格分隔），
// 等价于过去 strings.Join(fields[1:], " ") 的写法。
func (c telegramCommandCtx) argString() string {
	return strings.Join(c.Args, " ")
}

// telegramCommandRegistry 定义所有"私聊 + 普通 gating"的命令。
// 列表顺序无意义；多次注册相同命令会以最后一次为准（go map 行为）。
var telegramCommandRegistry = map[string]telegramCommandSpec{
	"/bind": {
		name:        "bind",
		label:       "/bind",
		description: "绑定 Telegram 账号到 Web 账户",
		usage:       "/bind <绑定码>",
		category:    "user",
		order:       10,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			if len(c.Args) < 1 {
				_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramBindPrompt())
				return
			}
			code := c.Args[0]
			if !telegramBindCodePattern.MatchString(code) {
				_ = a.telegramSendMessage(ctx, c.ChatID, "绑定码格式无效，请在网页重新获取后发送。\n\n示例：/bind ABC123")
				return
			}
			a.telegramConfirmBindCode(ctx, c.ChatID, c.FromID, c.Username, code)
		},
	},
	"/about": {
		name:        "about",
		label:       "/about",
		description: "查看服务说明",
		usage:       "/about",
		category:    "user",
		order:       20,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramAboutText())
		},
	},
	"/cancel": {
		name:        "cancel",
		label:       "/cancel",
		description: "取消当前 Bot 操作",
		usage:       "/cancel",
		category:    "user",
		order:       30,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.clearDelAccountPending(c.ChatID, c.FromID)
			_ = a.telegramSendMessage(ctx, c.ChatID, "已取消当前 Bot 操作。")
		},
	},
	"/me": {
		name:        "me",
		label:       "/me",
		description: "查看当前绑定信息",
		usage:       "/me",
		category:    "user",
		order:       40,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleMe(ctx, c.ChatID, c.FromID)
		},
	},
	"/emby": {
		name:        "emby",
		label:       "/emby",
		description: "查看 Emby 状态",
		usage:       "/emby",
		category:    "user",
		order:       50,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleEmby(ctx, c.ChatID, c.FromID)
		},
	},
	"/resetpwd": {
		name:        "resetpwd",
		label:       "/resetpwd",
		description: "查看密码修改说明",
		usage:       "/resetpwd",
		category:    "user",
		order:       60,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleResetPassword(ctx, c.ChatID, c.FromID)
		},
	},
	"/stats": {
		name:        "stats",
		label:       "/stats",
		description: "查看服务统计",
		usage:       "/stats",
		category:    "admin",
		order:       210,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleStats(ctx, c.ChatID, c.FromID)
		},
	},
	"/admin": {
		name:        "admin",
		label:       "/admin",
		description: "打开管理员查询入口",
		usage:       "/admin",
		category:    "admin",
		order:       220,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleAdmin(ctx, c.ChatID, c.FromID)
		},
	},
	"/userinfo": {
		name:        "userinfo",
		label:       "/userinfo",
		description: "查看指定用户详情",
		usage:       "/userinfo <用户名/UID/关键词>",
		category:    "admin",
		order:       230,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleUserInfo(ctx, c.ChatID, c.FromID, c.argString())
		},
	},
	"/twfind": {
		name:        "twfind",
		label:       "/twfind",
		description: "搜索用户",
		usage:       "/twfind <用户名/UID/关键词>",
		category:    "admin",
		order:       240,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleFind(ctx, c.ChatID, c.FromID, c.argString())
		},
	},
	"/twishelp": {
		name:        "twishelp",
		label:       "/twishelp",
		description: "查看管理员帮助",
		usage:       "/twishelp",
		category:    "admin",
		order:       250,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			telegramAdminHelpCommandHandler(a, ctx, c)
		},
	},
	"/banweb": {
		name:        "banweb",
		label:       "/banweb",
		description: "禁用 Web 账号",
		usage:       "/banweb <用户> [理由]",
		category:    "admin",
		order:       260,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleBanWeb(ctx, c.ChatID, c.FromID, c.Args)
		},
	},
	"/banemby": {
		name:        "banemby",
		label:       "/banemby",
		description: "禁用 Emby 账号",
		usage:       "/banemby <用户> [理由]",
		category:    "admin",
		order:       270,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleBanEmby(ctx, c.ChatID, c.FromID, c.Args)
		},
	},
	"/delaccount": {
		name:        "delaccount",
		label:       "/delaccount",
		description: "自助销号",
		usage:       "/delaccount",
		category:    "user",
		order:       70,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleDelAccount(ctx, c.ChatID, c.FromID, c.Args)
		},
	},
	"/version": {
		name:        "version",
		label:       "/version",
		description: "显示版本号",
		usage:       "/version",
		category:    "system",
		order:       310,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			ver := a.cfg().Version
			name := a.cfg().AppName
			_ = a.telegramSendMessage(ctx, c.ChatID, name+" v"+ver)
		},
	},
	"/ping": {
		name:        "ping",
		label:       "/ping",
		description: "连通性测试",
		usage:       "/ping",
		category:    "system",
		order:       320,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			_ = a.telegramSendMessage(ctx, c.ChatID, "pong")
		},
	},
	"/notice": {
		name:        "notice",
		label:       "/notice",
		description: "查看最新公告",
		usage:       "/notice",
		category:    "user",
		order:       80,
		private:     true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleNotice(ctx, c.ChatID)
		},
	},
	"/broadcast": {
		name:        "broadcast",
		label:       "/broadcast",
		description: "向启用 Telegram 通知的用户广播消息",
		usage:       "/broadcast <消息内容>",
		category:    "admin",
		order:       280,
		private:     true,
		admin:       true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleBroadcast(ctx, c.ChatID, c.FromID, c.argString())
		},
	},
}

var telegramSpecialCommandCatalog = []telegramCommandCatalogItem{
	{Command: "/start", Name: "start", Label: "/start", Description: "打开 Bot 入口", Usage: "/start", Category: "system", Private: false, Admin: false, Disableable: false},
	{Command: "/help", Name: "help", Label: "/help", Description: "查看完整帮助", Usage: "/help", Category: "system", Private: false, Admin: false, Disableable: false},
	{Command: "/twihelp", Name: "twihelp", Label: "/twihelp", Description: "查看群组使用提示", Usage: "/twihelp", Category: "group", Private: false, Admin: false, Disableable: false},
	{Command: "/twguser", Name: "twguser", Label: "/twguser", Description: "群组用户管理面板", Usage: "/twguser <用户名/UID/关键词>", Category: "group", Private: false, Admin: true, Disableable: false},
}

// telegramDispatchRegistry 在 dispatcher 中统一执行注册表里命令的 gating，
// 命中并调用成功返回 true；未命中（特殊命令或自定义命令）返回 false 让上层继续 switch。
// gating 顺序：private → admin。任何一关失败都直接发统一文案并返回 true（命令"已处理"），
// 不要让上层再当作 unknown command 继续追加错误提示。
// telegramHandleNotice 返回最新一条公告。
func (a *App) telegramHandleNotice(ctx context.Context, chatID int64) {
	announcements := a.store().ListAnnouncements(false)
	if len(announcements) == 0 {
		_ = a.telegramSendMessage(ctx, chatID, "暂无公告。")
		return
	}
	latest := announcements[len(announcements)-1]
	title := latest.Title
	content := latest.Content
	if len(content) > 500 {
		content = content[:500] + "…"
	}
	if title != "" {
		_ = a.telegramSendMessage(ctx, chatID, "📢 "+title+"\n\n"+content)
	} else {
		_ = a.telegramSendMessage(ctx, chatID, content)
	}
}

// telegramHandleBroadcast 向所有启用了 Telegram 通知的用户广播消息。
func (a *App) telegramHandleBroadcast(ctx context.Context, chatID, fromID int64, message string) {
	if strings.TrimSpace(message) == "" {
		_ = a.telegramSendMessage(ctx, chatID, "用法：/broadcast <消息内容>")
		return
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	sent := 0
	failed := 0
	for _, u := range a.store().ListUsers() {
		if u.TelegramID == 0 || !u.NotifyOnLoginTelegram {
			continue
		}
		if err := a.telegramSendMessage(ctx, u.TelegramID, "📢 系统通知\n\n"+message); err != nil {
			failed++
		} else {
			sent++
		}
	}
	_ = a.telegramSendMessage(ctx, chatID, "广播完成。已发送 "+fmt.Sprintf("%d", sent)+" 人，失败 "+fmt.Sprintf("%d", failed)+" 人。")
}

func normalizeTelegramDisabledCommand(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "/")
	return raw
}

func (a *App) telegramDisabledCommandSet() map[string]bool {
	out := make(map[string]bool)
	for _, disabled := range a.cfg().TelegramDisabledCommands {
		if name := normalizeTelegramDisabledCommand(disabled); name != "" {
			out[name] = true
		}
	}
	return out
}

func (a *App) telegramCommandDisabled(command string) bool {
	name := normalizeTelegramDisabledCommand(telegramCommand(command))
	if name == "" {
		return false
	}
	return a.telegramDisabledCommandSet()[name]
}

func (a *App) telegramCommandCatalog() []telegramCommandCatalogItem {
	disabled := a.telegramDisabledCommandSet()
	items := make([]telegramCommandCatalogItem, 0, len(telegramCommandRegistry)+len(telegramSpecialCommandCatalog))
	for command, spec := range telegramCommandRegistry {
		name := spec.name
		if name == "" {
			name = strings.TrimPrefix(command, "/")
		}
		label := spec.label
		if label == "" {
			label = command
		}
		items = append(items, telegramCommandCatalogItem{
			Command:     command,
			Name:        name,
			Label:       label,
			Description: spec.description,
			Usage:       spec.usage,
			Category:    firstNonEmpty(spec.category, "user"),
			Private:     spec.private,
			Admin:       spec.admin,
			Disableable: true,
			Disabled:    disabled[name],
		})
	}
	items = append(items, telegramSpecialCommandCatalog...)
	sort.SliceStable(items, func(i, j int) bool {
		left, right := telegramCatalogOrder(items[i]), telegramCatalogOrder(items[j])
		if left != right {
			return left < right
		}
		return items[i].Command < items[j].Command
	})
	return items
}

func telegramCatalogOrder(item telegramCommandCatalogItem) int {
	if spec, ok := telegramCommandRegistry[item.Command]; ok && spec.order > 0 {
		return spec.order
	}
	switch item.Command {
	case "/start":
		return 1
	case "/help":
		return 2
	case "/twihelp":
		return 401
	case "/twguser":
		return 402
	default:
		return 900
	}
}

func (a *App) handleTelegramCommandCatalog(w http.ResponseWriter, r *http.Request, _ Params) {
	disabled := make([]string, 0, len(a.telegramDisabledCommandSet()))
	for name := range a.telegramDisabledCommandSet() {
		disabled = append(disabled, name)
	}
	sort.Strings(disabled)
	ok(w, "ok", map[string]any{
		"commands":          a.telegramCommandCatalog(),
		"disabled_commands": disabled,
	})
}

func (a *App) telegramDispatchRegistry(ctx context.Context, command string, c telegramCommandCtx, privateChat bool) (handled bool) {
	spec, ok := telegramCommandRegistry[command]
	if !ok {
		return false
	}
	// 检查内置指令是否被管理员禁用
	if a.telegramCommandDisabled(command) {
		_ = a.telegramSendMessage(ctx, c.ChatID, "该内置指令已被管理员停用。")
		return true
	}
	if spec.private && !a.telegramRequirePrivate(ctx, c.ChatID, privateChat) {
		return true
	}
	if spec.admin && !a.telegramAdminID(c.FromID) {
		_ = a.telegramSendMessage(ctx, c.ChatID, "没有管理员权限。")
		return true
	}
	spec.handler(a, ctx, c)
	return true
}
