package api

import (
	"context"
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
	// private 为 true 时，dispatcher 在非私聊场景调用 telegramRequirePrivate
	// 提示并直接返回，handler 不会被执行。
	private bool
	// admin 为 true 时，dispatcher 检查 telegramAdminID(fromID)，
	// 失败发送统一文案"没有管理员权限。"。
	admin bool
	// handler 接收已通过 gating 的 ctx，可直接处理业务。
	handler func(*App, context.Context, telegramCommandCtx)
}

// telegramCommandCtx 把命令分发上下文打包成单一参数，方便 handler 签名收敛、
// 后续扩展（如加 traceID / requestID）也不用回头改所有 handler 签名。
type telegramCommandCtx struct {
	ChatID   int64
	FromID   int64
	Username string
	// Args 是 fields[1:]，handler 自行选择 strings.Join 还是按位置取。
	Args []string
}

// argString 把 Args 拼成单个查询字符串（多关键词以空格分隔），
// 等价于过去 strings.Join(fields[1:], " ") 的写法。
func (c telegramCommandCtx) argString() string {
	return strings.Join(c.Args, " ")
}

// telegramCommandRegistry 定义所有"私聊 + 普通 gating"的命令。
// 列表顺序无意义；多次注册相同命令会以最后一次为准（go map 行为）。
var telegramCommandRegistry = map[string]telegramCommandSpec{
	"/about": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramAboutText())
		},
	},
	"/cancel": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			_ = a.telegramSendMessage(ctx, c.ChatID, "已取消当前 Bot 操作。")
		},
	},
	"/me": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleMe(ctx, c.ChatID, c.FromID)
		},
	},
	"/emby": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleEmby(ctx, c.ChatID, c.FromID)
		},
	},
	"/playinfo": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandlePlayInfo(ctx, c.ChatID, c.FromID)
		},
	},
	"/resetpwd": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleResetPassword(ctx, c.ChatID, c.FromID)
		},
	},
	"/stats": {
		private: true,
		admin:   true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleStats(ctx, c.ChatID, c.FromID)
		},
	},
	"/admin": {
		private: true,
		admin:   true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleAdmin(ctx, c.ChatID, c.FromID)
		},
	},
	"/userinfo": {
		private: true,
		admin:   true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleUserInfo(ctx, c.ChatID, c.FromID, c.argString())
		},
	},
	"/twfind": {
		private: true,
		admin:   true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			a.telegramHandleFind(ctx, c.ChatID, c.FromID, c.argString())
		},
	},
	"/twishelp": {
		private: true,
		admin:   true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramAdminHelpText())
		},
	},
	"/bind": {
		private: true,
		handler: func(a *App, ctx context.Context, c telegramCommandCtx) {
			if len(c.Args) < 1 {
				_ = a.telegramSendMessage(ctx, c.ChatID, a.telegramBindPrompt())
				return
			}
			a.telegramConfirmBindCode(ctx, c.ChatID, c.FromID, c.Username, c.Args[0])
		},
	},
}

// telegramDispatchRegistry 在 dispatcher 中统一执行注册表里命令的 gating，
// 命中并调用成功返回 true；未命中（特殊命令或自定义命令）返回 false 让上层继续 switch。
// gating 顺序：private → admin。任何一关失败都直接发统一文案并返回 true（命令"已处理"），
// 不要让上层再当作 unknown command 继续追加错误提示。
func (a *App) telegramDispatchRegistry(ctx context.Context, command string, c telegramCommandCtx, privateChat bool) (handled bool) {
	spec, ok := telegramCommandRegistry[command]
	if !ok {
		return false
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
