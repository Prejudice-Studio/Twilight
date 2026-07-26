package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/prejudice-studio/twilight/internal/store"
)

// developerJSBangumi* 为 Telegram 自定义指令沙箱提供「以当前绑定用户身份访问
// Bangumi」的受控能力。核心安全约束：用户的 BGM Token 只存在于服务端闭包里，
// 全程不进入 JS——JS 只能拿到脱敏后的统计/资料，拿不到 Token 本身。所有调用复用
// 已带 SSRF 否决的 getBangumiMe / getBangumiUserCollections，并统一加超时上限，
// 避免慢响应把整个指令拖过 VM 8s 中断阈值。
const (
	// developerJSBangumiSummaryBudget 限定 summary() 内多次串行 Bangumi 请求的总预算，
	// 明确低于 developerJSExecutionTimeout（8s），确保即便远端慢也能在 VM 中断前返回。
	developerJSBangumiSummaryBudget = 6 * time.Second
	// developerJSBangumiCallBudget 限定单次 me()/collections() 的耗时上限。
	developerJSBangumiCallBudget = 4 * time.Second
)

// developerJSBangumiAPI 构造 bangumi.* 绑定。token 通过闭包捕获 user 指针在调用时
// 现取，绝不写入返回给 JS 的任何字段。
func (a *App) developerJSBangumiAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		// enabled() 报告当前是否具备「以本人身份调用 Bangumi」的前提：已绑定本地用户
		// 且该用户已设置 BGM Token。不发起任何网络请求。
		"enabled": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(user != nil && user.UID != 0 && strings.TrimSpace(user.BGMToken) != "")
		},
		// me() 以本人 Token 调 /me，返回脱敏资料（id/username/nickname/reg_time 等），
		// 失败或未绑定返回 null。
		"me": func(goja.FunctionCall) goja.Value {
			return a.developerJSBangumiMe(vm, user, opts, logs)
		},
		// collections(type, limit?, offset?) 分页读取本人某类动画收藏，返回
		// {ok,total,type,limit,offset,data:[{updated_at,subject_id,name,type,rate}]}。
		"collections": func(call goja.FunctionCall) goja.Value {
			return a.developerJSBangumiCollections(vm, user, opts, logs, call)
		},
		// summary() 聚合自助发码判定所需的全部指标（注册时长 / 看过+在看合计 /
		// 最早收藏时间），一次调用内串行取数并受总预算约束。
		"summary": func(goja.FunctionCall) goja.Value {
			return a.developerJSBangumiSummary(vm, user, opts, logs)
		},
	}
}

// developerJSBangumiCtx 从运行上下文派生一个带上限的子 context，保证 Bangumi 请求
// 不会突破 VM 中断阈值。budget<=0 时退化为直接用父 context（由 VM 计时器兜底）。
func developerJSBangumiCtx(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if budget <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, budget)
}

// developerJSBangumiLog 追加一条固定文案日志（有上限），绝不把 Token / 原始错误
// 写进去，避免任何形式的敏感信息外泄。
func developerJSBangumiLog(logs *[]string, message string) {
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, message)
	}
}

// developerJSParseBangumiTime 把 Bangumi 的 RFC3339（带时区）时间串解析为 Unix 秒；
// 解析失败返回 0。
func developerJSParseBangumiTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Unix()
	}
	return 0
}

// developerJSMonthsSince 返回自 unixSeconds 起至今经过的「月数」（按 30 天/月近似）。
// unixSeconds<=0 或在未来时返回 0。
func developerJSMonthsSince(unixSeconds int64) int {
	if unixSeconds <= 0 {
		return 0
	}
	delta := time.Now().Unix() - unixSeconds
	if delta <= 0 {
		return 0
	}
	return int(delta / (30 * 24 * 3600))
}

// developerJSBangumiProfile 把 /me 原始 payload 收敛成脱敏资料对象；同时以字符串形式
// 回填数值 id（供去重键使用）与解析出的注册时间 Unix 秒 + 注册月数。
func developerJSBangumiProfile(me map[string]any) map[string]any {
	idNum := numeric(me["id"])
	id := ""
	if idNum > 0 {
		id = strconv.FormatInt(idNum, 10)
	} else {
		id = strings.TrimSpace(asString(me["id"]))
	}
	regTimeRaw := asString(me["reg_time"])
	regUnix := developerJSParseBangumiTime(regTimeRaw)
	return map[string]any{
		"id":              id,
		"username":        asString(me["username"]),
		"nickname":        asString(me["nickname"]),
		"user_group":      numeric(me["user_group"]),
		"reg_time":        regTimeRaw,
		"reg_time_unix":   zeroNil(regUnix),
		"register_months": developerJSMonthsSince(regUnix),
		"has_id":          id != "",
	}
}

// developerJSBangumiFetchMe 以本人 Token 拉取 /me，返回（脱敏资料, 原始 id 字符串, 错误码）。
// 错误码为空表示成功；否则为 "not_bound" / "no_token" / "unauthorized" / "request_failed"。
// 原始错误只落固定文案日志、绝不外抛，避免 Token / URL 外泄。
func (a *App) developerJSBangumiFetchMe(user *store.User, opts developerJSRunOptions, logs *[]string) (map[string]any, string, string) {
	if user == nil || user.UID == 0 {
		return nil, "", "not_bound"
	}
	token := strings.TrimSpace(user.BGMToken)
	if token == "" {
		return nil, "", "no_token"
	}
	ctx, cancel := developerJSBangumiCtx(opts.Context, developerJSBangumiCallBudget)
	defer cancel()
	me, unauthorized, err := a.getBangumiMe(ctx, token)
	if err != nil {
		developerJSBangumiLog(logs, "bangumi.me request failed")
		return nil, "", "request_failed"
	}
	if unauthorized {
		developerJSBangumiLog(logs, "bangumi.me unauthorized (token rejected)")
		return nil, "", "unauthorized"
	}
	profile := developerJSBangumiProfile(me)
	id, _ := profile["id"].(string)
	return profile, id, ""
}

// developerJSBangumiMe 是 bangumi.me() 的实现：成功返回脱敏资料对象，任何失败返回 null。
func (a *App) developerJSBangumiMe(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) goja.Value {
	profile, _, errCode := a.developerJSBangumiFetchMe(user, opts, logs)
	if errCode != "" {
		return goja.Null()
	}
	return vm.ToValue(profile)
}

// developerJSBangumiRow 把一条收藏原始记录收敛成轻量对象，剥掉体积巨大的 subject
// 嵌套，仅保留脚本判定/展示会用到的字段。
func developerJSBangumiRow(row map[string]any) map[string]any {
	subject, _ := row["subject"].(map[string]any)
	name := ""
	if subject != nil {
		name = firstNonEmpty(asString(subject["name_cn"]), asString(subject["name"]))
	}
	updatedAt := asString(row["updated_at"])
	return map[string]any{
		"updated_at":      updatedAt,
		"updated_at_unix": zeroNil(developerJSParseBangumiTime(updatedAt)),
		"subject_id":      numeric(row["subject_id"]),
		"name":            name,
		"type":            numeric(row["type"]),
		"rate":            numeric(row["rate"]),
	}
}

// developerJSBangumiTypeStats 取某类收藏的总量与「最早一条」的 updated_at（Unix 秒）。
// Bangumi 列表按 updated_at 降序返回，故 offset=total-1 即最早一条。total==0 时
// earliestUnix 返回 0。错误只落固定日志、以 (0,0,false) 表达失败。
func (a *App) developerJSBangumiTypeStats(username, token string, collectType int, opts developerJSRunOptions, logs *[]string) (total int, earliestUnix int64, ok bool) {
	ctx, cancel := developerJSBangumiCtx(opts.Context, developerJSBangumiSummaryBudget)
	defer cancel()
	_, total, err := a.getBangumiUserCollections(ctx, username, token, collectType, 1, 0)
	if err != nil {
		developerJSBangumiLog(logs, "bangumi collections head request failed")
		return 0, 0, false
	}
	if total <= 0 {
		return 0, 0, true
	}
	rows, _, err := a.getBangumiUserCollections(ctx, username, token, collectType, 1, total-1)
	if err != nil {
		developerJSBangumiLog(logs, "bangumi collections tail request failed")
		// 总量已知、只是取不到最早一条：仍返回 total，earliest 记 0（由脚本按需处理）。
		return total, 0, true
	}
	if len(rows) > 0 {
		earliestUnix = developerJSParseBangumiTime(asString(rows[0]["updated_at"]))
	}
	return total, earliestUnix, true
}

// developerJSBangumiCollections 是 bangumi.collections(type, limit?, offset?) 的实现。
// subject_type 固定为动画（复用 getBangumiUserCollections 语义）；username 内部经
// me() 解析（不信任 JS 传值），因此每次调用至少一发 /me + 一发 collections。
func (a *App) developerJSBangumiCollections(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string, call goja.FunctionCall) goja.Value {
	result := map[string]any{"ok": false}
	profile, _, errCode := a.developerJSBangumiFetchMe(user, opts, logs)
	if errCode != "" {
		result["error"] = errCode
		return vm.ToValue(result)
	}
	username := asString(profile["username"])
	if username == "" {
		result["error"] = "no_username"
		return vm.ToValue(result)
	}
	collectType := int(call.Argument(0).ToInteger())
	if collectType < 1 || collectType > 3 {
		result["error"] = "invalid_type"
		return vm.ToValue(result)
	}
	limit := 8
	if v := call.Argument(1); !goja.IsUndefined(v) && !goja.IsNull(v) {
		limit = int(v.ToInteger())
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}
	offset := 0
	if v := call.Argument(2); !goja.IsUndefined(v) && !goja.IsNull(v) {
		offset = int(v.ToInteger())
	}
	if offset < 0 {
		offset = 0
	}
	token := strings.TrimSpace(user.BGMToken)
	ctx, cancel := developerJSBangumiCtx(opts.Context, developerJSBangumiCallBudget)
	defer cancel()
	rows, total, err := a.getBangumiUserCollections(ctx, username, token, collectType, limit, offset)
	if err != nil {
		developerJSBangumiLog(logs, "bangumi.collections request failed")
		result["error"] = "request_failed"
		return vm.ToValue(result)
	}
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, developerJSBangumiRow(row))
	}
	result["ok"] = true
	result["total"] = total
	result["type"] = collectType
	result["limit"] = limit
	result["offset"] = offset
	result["data"] = data
	return vm.ToValue(result)
}

// developerJSBangumiSummary 是 bangumi.summary() 的实现：一次调用聚合自助发码判定
// 所需全部指标。串行取数（me + 看过 head/tail + 在看 head/tail，至多 5 发），共享
// developerJSBangumiSummaryBudget 总预算。返回：
//
//	{ ok, id, username, nickname, register_months, reg_time_unix,
//	  watched, watching, total, earliest_unix, earliest_months }
//
// 其中 total = 看过 + 在看；earliest_* 取两类最早收藏里更早的那条。任何一步失败返回
// {ok:false,error:<code>}。id/username 均来自服务端 /me（不信任 JS）。
func (a *App) developerJSBangumiSummary(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) goja.Value {
	result := map[string]any{"ok": false}
	profile, id, errCode := a.developerJSBangumiFetchMe(user, opts, logs)
	if errCode != "" {
		result["error"] = errCode
		return vm.ToValue(result)
	}
	username := asString(profile["username"])
	if username == "" || id == "" {
		result["error"] = "no_username"
		return vm.ToValue(result)
	}
	token := strings.TrimSpace(user.BGMToken)

	watched, watchedEarliest, ok := a.developerJSBangumiTypeStats(username, token, 2, opts, logs)
	if !ok {
		result["error"] = "request_failed"
		return vm.ToValue(result)
	}
	watching, watchingEarliest, ok := a.developerJSBangumiTypeStats(username, token, 3, opts, logs)
	if !ok {
		result["error"] = "request_failed"
		return vm.ToValue(result)
	}

	earliest := watchedEarliest
	if watchingEarliest > 0 && (earliest == 0 || watchingEarliest < earliest) {
		earliest = watchingEarliest
	}

	result["ok"] = true
	result["id"] = id
	result["username"] = username
	result["nickname"] = asString(profile["nickname"])
	result["register_months"] = profile["register_months"]
	result["reg_time_unix"] = profile["reg_time_unix"]
	result["watched"] = watched
	result["watching"] = watching
	result["total"] = watched + watching
	result["earliest_unix"] = zeroNil(earliest)
	result["earliest_months"] = developerJSMonthsSince(earliest)
	return vm.ToValue(result)
}

// developerJSBangumiClaimSelf 是 regcodes.grantSelf() 依赖的「服务端权威取 Bangumi id」
// 入口，供发码前重新核验身份并作为全局去重键。返回（id, username, 错误码）。
func (a *App) developerJSBangumiClaimSelf(user *store.User, opts developerJSRunOptions, logs *[]string) (string, string, string) {
	profile, id, errCode := a.developerJSBangumiFetchMe(user, opts, logs)
	if errCode != "" {
		return "", "", errCode
	}
	username := asString(profile["username"])
	if id == "" {
		return "", "", "no_bangumi_id"
	}
	return id, username, ""
}
