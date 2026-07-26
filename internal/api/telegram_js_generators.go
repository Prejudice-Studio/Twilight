package api

import (
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/prejudice-studio/twilight/internal/store"
)

// developerJSRegcodesAPI exposes admin-only registration / renewal code generation
// helpers to the Telegram custom-command JS sandbox. Read helpers reuse the same
// masked snapshots as the db.* namespace; the generate helper mirrors the
// validation, collision detection and audit semantics of handleCreateRegcodes.
func (a *App) developerJSRegcodesAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"list": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListRegcodes(user, call.Argument(0).Export(), logs))
		},
		"get": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGetRegcode(vm, user, call.Argument(0).String(), logs)
		},
		"generate": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGenerateRegcode(vm, user, opts, logs, call.Argument(0).Export())
		},
		// quick(days?, count?, type?) 是 generate 的位置参数简化版：少写 options 对象，
		// 直接给天数 / 数量 / 类型即可。未传的参数沿用 generate 的默认值（30 天 / 1 个 / 注册码）。
		"quick": func(call goja.FunctionCall) goja.Value {
			return a.developerJSQuickRegcode(vm, user, opts, logs, call)
		},
		// grantSelf(options?) 是面向「已绑定普通用户」的自助发码通道：区别于 admin-only
		// 的 generate，它把注册码强制锁定给触发者本人，并以其 Bangumi 账号数值 id 为
		// 全局去重键——一个 Bangumi 账号永远只能领一张（可重复调用但幂等复用旧码）。
		// 强约束：仅私聊、必须已绑定 BGM Token、已有 Emby 账号则拒发。
		"grantSelf": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGrantSelfRegcode(vm, user, opts, logs, call.Argument(0).Export())
		},
	}
}

// developerJSInvitesAPI exposes admin-only invite code generation helpers.
func (a *App) developerJSInvitesAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"list": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListInviteCodes(user, call.Argument(0).Export(), logs))
		},
		"generate": func(call goja.FunctionCall) goja.Value {
			return a.developerJSGenerateInviteCode(vm, user, opts, logs, call.Argument(0).Export())
		},
		// quick(days?) 是 generate 的位置参数简化版：只给天数即可生成单个邀请码，
		// 不传则使用配置默认天数。其余校验（功能开关 / 天数上限 / 去重 / 审计）完全复用 generate。
		"quick": func(call goja.FunctionCall) goja.Value {
			return a.developerJSQuickInvite(vm, user, opts, logs, call)
		},
	}
}

// developerJSAnnouncementsAPI exposes announcement read/create helpers.
func (a *App) developerJSAnnouncementsAPI(vm *goja.Runtime, user *store.User, opts developerJSRunOptions, logs *[]string) map[string]any {
	return map[string]any{
		"list": func(call goja.FunctionCall) goja.Value {
			return vm.ToValue(a.developerJSListAnnouncements(call.Argument(0).Export()))
		},
		"create": func(call goja.FunctionCall) goja.Value {
			return a.developerJSCreateAnnouncement(vm, user, opts, logs, call.Argument(0).Export())
		},
		// post(title, content, level?) 是 create 的位置参数简化版：直接给标题 / 正文 / 级别，
		// 渲染模式默认安全 plain、默认可见，复用 create 的 admin 校验、预览 dry-run 与审计。
		"post": func(call goja.FunctionCall) goja.Value {
			return a.developerJSQuickAnnouncement(vm, user, opts, logs, call)
		},
	}
}

// developerJSGetRegcode returns a single masked registration code snapshot. Admin-only.
func (a *App) developerJSGetRegcode(vm *goja.Runtime, user *store.User, code string, logs *[]string) goja.Value {
	if !developerJSAdminUser(user, logs, "regcodes.get") {
		return goja.Null()
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return goja.Null()
	}
	reg, ok := a.store().RegCode(code)
	if !ok {
		return goja.Null()
	}
	return vm.ToValue(developerJSRegcodeSnapshot(reg))
}

// developerJSGenerateRegcode mirrors handleCreateRegcodes: admin-gated, validated,
// collision-checked, preview-aware (dry-run) and audited. Returns a result map.
func (a *App) developerJSGenerateRegcode(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, options any) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "regcodes.generate") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	// 卡码写入依赖 store 与配置一致的单一状态文档；运行态与配置不一致时拒绝写入，
	// 与 HTTP 侧 rejectRegcodeWriteIfStorageMismatch 的保护口径保持一致。
	if a.runtimeDatabaseMismatch() {
		result["error"] = "storage_mismatch"
		return vm.ToValue(result)
	}
	values, _ := options.(map[string]any)
	if values == nil {
		values = map[string]any{}
	}

	count := developerJSOptionInt(values, 1, "count")
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	days := normalizeRegCodeDays(developerJSOptionInt(values, 30, "days"))
	if days > 36500 {
		result["error"] = "days_out_of_range"
		return vm.ToValue(result)
	}

	codeType := developerJSOptionInt(values, 1, "type")
	if codeType < 1 || codeType > 3 {
		result["error"] = "invalid_type"
		return vm.ToValue(result)
	}

	validity := int64(developerJSOptionInt(values, -1, "validity_time", "validity"))
	if validity == 0 {
		validity = -1
	}
	if validity < -1 {
		result["error"] = "invalid_validity_time"
		return vm.ToValue(result)
	}
	if validity > maxRegCodeValidityHours {
		result["error"] = "invalid_validity_time"
		return vm.ToValue(result)
	}

	useLimit := developerJSOptionInt(values, 1, "use_count_limit", "use_limit")
	if useLimit == 0 {
		useLimit = 1
	}
	if useLimit < -1 {
		result["error"] = "invalid_use_count_limit"
		return vm.ToValue(result)
	}

	note := truncateString(developerJSOptionString(values, "note"), 120)
	decoy, _ := developerJSBoolOption(values, "decoy")
	format := a.regCodeFormatForType(codeType, developerJSOptionString(values, "format"))
	algorithm := firstNonEmpty(developerJSOptionString(values, "random_algorithm", "algorithm"), a.cfg().RegCodeRandomAlgorithm, "base32-20")

	targetUsername := strings.TrimSpace(developerJSOptionString(values, "target_username"))
	if targetUsername != "" && !validRegcodeTargetUsername(targetUsername) {
		result["error"] = "invalid_target_username"
		return vm.ToValue(result)
	}

	result["count"] = count
	result["type"] = codeType
	result["type_name"] = regcodeTypeName(codeType)
	result["days"] = days
	result["validity_time"] = validity
	result["use_count_limit"] = useLimit
	result["decoy"] = decoy
	if targetUsername != "" {
		result["target_username"] = targetUsername
	}

	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}

	seen := map[string]bool{}
	existingCodes := map[string]bool{}
	for _, c := range a.store().ListRegCodes() {
		existingCodes[c.Code] = true
	}
	for _, c := range a.store().ListAllInviteCodes() {
		existingCodes[c.Code] = true
	}
	maxAttempts := 10
	if strings.Contains(algorithm, "16") {
		maxAttempts = 20
	}
	if algorithm == "digits-12" || algorithm == "hex10" {
		maxAttempts = 30
	}

	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := ""
		for attempt := 0; attempt < maxAttempts; attempt++ {
			candidate := generateRegCode(format, codeType, algorithm, days, i+1, validity, useLimit)
			if seen[candidate] || existingCodes[candidate] {
				continue
			}
			code = candidate
			break
		}
		if code == "" {
			result["error"] = "generation_conflict"
			result["codes"] = codes
			return vm.ToValue(result)
		}
		seen[code] = true
		if err := a.store().UpsertRegCode(store.RegCode{
			Code:           code,
			Type:           codeType,
			ValidityTime:   validity,
			UseCountLimit:  useLimit,
			Days:           days,
			Note:           note,
			IsDecoy:        decoy,
			TargetUsername: targetUsername,
			Active:         true,
			Source:         "telegram_js",
			CreatorUID:     actor.UID,
		}); err != nil {
			result["error"] = err.Error()
			result["codes"] = codes
			return vm.ToValue(result)
		}
		codes = append(codes, code)
	}

	result["ok"] = true
	result["codes"] = codes
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "regcodes.generate created codes")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_regcode_generate", "admin", 0, map[string]any{
		"count":        len(codes),
		"type":         codeType,
		"days":         days,
		"codes":        codes,
		"decoy":        decoy,
		"script_api":   "regcodes.generate",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

// developerJSGrantSelfRegcode 是面向「已绑定普通用户」的自助发码通道。它不是 admin
// 专属 generate 的放权版本，而是一条语义收窄、强约束的独立通道：
//   - 仅私聊可用（opts.PrivateChat==false 直接拒绝）；
//   - 触发者必须已绑定本地用户且已设置 BGM Token；
//   - 触发者已有 Emby 账号则拒发（这是服务端硬不变量，脚本层再友好提示）；
//   - 注册码强制锁定给触发者本人（TargetUsername/UID/TelegramID 全部落本人）；
//   - 以「服务端持 Token 重新调 /me 拿到的 Bangumi 数值 id」为全局唯一去重键，
//     一个 Bangumi 账号永远只能领一张：命中既有领取记录即幂等复用旧码，绝不再生成。
//
// 是否满足「注册时长 / 看过+在看数量 / 最早收藏时长」等业务门槛由脚本用 bangumi.summary()
// 自行判定；本函数只负责发码通道自身的身份、去重与账号不变量，并始终审计。
func (a *App) developerJSGrantSelfRegcode(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, options any) goja.Value {
	result := map[string]any{"ok": false}
	if actor == nil || actor.UID == 0 {
		result["error"] = "not_bound"
		return vm.ToValue(result)
	}
	if !opts.PrivateChat {
		result["error"] = "group_chat_forbidden"
		return vm.ToValue(result)
	}
	if strings.TrimSpace(actor.BGMToken) == "" {
		result["error"] = "no_token"
		return vm.ToValue(result)
	}
	// 已有 Emby 账号则不给：EmbyID 非空即视为「已落地账号」。这是服务端硬闸，
	// 即便脚本漏判也拦得住。
	if strings.TrimSpace(actor.EmbyID) != "" {
		result["error"] = "already_has_account"
		return vm.ToValue(result)
	}
	if a.runtimeDatabaseMismatch() {
		result["error"] = "storage_mismatch"
		return vm.ToValue(result)
	}

	values, _ := options.(map[string]any)
	if values == nil {
		values = map[string]any{}
	}
	days := normalizeRegCodeDays(developerJSOptionInt(values, 30, "days"))
	if days > 36500 {
		result["error"] = "days_out_of_range"
		return vm.ToValue(result)
	}
	note := truncateString(developerJSOptionString(values, "note"), 120)
	// 锁定给本人：注册码指名到触发者当前用户名（沿用 admin「指名码」既有兑换语义），
	// 并冗余记录 UID / TelegramID 作为纵深校验元数据。
	targetUsername := strings.TrimSpace(actor.Username)
	codeType := 1
	format := a.regCodeFormatForType(codeType, developerJSOptionString(values, "format"))
	algorithm := firstNonEmpty(developerJSOptionString(values, "random_algorithm", "algorithm"), a.cfg().RegCodeRandomAlgorithm, "base32-20")

	result["days"] = days
	result["type"] = codeType
	result["type_name"] = regcodeTypeName(codeType)
	result["target_username"] = targetUsername

	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}

	// 服务端持 Token 重新核验身份，拿到权威 Bangumi 数值 id 作为去重键（不信任 JS 传值）。
	bangumiID, bangumiUsername, errCode := a.developerJSBangumiClaimSelf(actor, opts, logs)
	if errCode != "" {
		result["error"] = errCode
		return vm.ToValue(result)
	}

	// 预检：该 Bangumi 账号是否已领过。命中即幂等复用旧码，连码都不再生成。
	if existing, ok := a.store().BangumiRegcodeClaimByID(bangumiID); ok {
		result["ok"] = true
		result["already_claimed"] = true
		result["code"] = existing.Code
		result["days"] = existing.Days
		return vm.ToValue(result)
	}

	// 生成候选码 + 全量碰撞检测（复用 generate 的去重口径）。
	existingCodes := map[string]bool{}
	for _, c := range a.store().ListRegCodes() {
		existingCodes[c.Code] = true
	}
	for _, c := range a.store().ListAllInviteCodes() {
		existingCodes[c.Code] = true
	}
	maxAttempts := 10
	if strings.Contains(algorithm, "16") {
		maxAttempts = 20
	}
	if algorithm == "digits-12" || algorithm == "hex10" {
		maxAttempts = 30
	}
	code := ""
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate := generateRegCode(format, codeType, algorithm, days, 1, -1, 1)
		if existingCodes[candidate] {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		result["error"] = "generation_conflict"
		return vm.ToValue(result)
	}

	if err := a.store().UpsertRegCode(store.RegCode{
		Code:             code,
		Type:             codeType,
		ValidityTime:     -1,
		UseCountLimit:    1,
		Days:             days,
		Note:             note,
		TargetUsername:   targetUsername,
		TargetUID:        actor.UID,
		TargetTelegramID: actor.TelegramID,
		Active:           true,
		Source:           "telegram_js_self",
		CreatorUID:       actor.UID,
	}); err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}

	// 原子登记去重记录。claimed==false 说明并发下他人（其实只可能是同一 Bangumi 账号的
	// 并发调用）已抢先登记：回收本次刚生成的码，幂等返回既有码，绝不下发第二张。
	stored, claimed, err := a.store().ClaimBangumiRegcode(store.BangumiRegcodeClaim{
		BangumiID:       bangumiID,
		BangumiUsername: bangumiUsername,
		Code:            code,
		UID:             actor.UID,
		Username:        actor.Username,
		TelegramID:      actor.TelegramID,
		Days:            days,
		Source:          "telegram_js_self",
	})
	if err != nil {
		// 登记失败：回收刚生成的码，避免留下无主码。
		_ = a.store().DeleteRegCode(code)
		result["error"] = "claim_failed"
		return vm.ToValue(result)
	}
	if !claimed {
		_ = a.store().DeleteRegCode(code)
		result["ok"] = true
		result["already_claimed"] = true
		result["code"] = stored.Code
		result["days"] = stored.Days
		return vm.ToValue(result)
	}

	result["ok"] = true
	result["code"] = code
	result["bangumi_claimed"] = true
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "regcodes.grantSelf issued code")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_regcode_grant_self", "system", actor.UID, map[string]any{
		"code":             code,
		"days":             days,
		"bangumi_id":       bangumiID,
		"bangumi_username": bangumiUsername,
		"target_username":  targetUsername,
		"script_api":       "regcodes.grantSelf",
		"private_chat":     opts.PrivateChat,
	})
	return vm.ToValue(result)
}

// developerJSGenerateInviteCode mirrors handleCreateInviteCode for admins, honoring
// the InviteEnabled feature gate, day caps, collision detection, preview dry-run and audit.
func (a *App) developerJSGenerateInviteCode(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, options any) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "invites.generate") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	if !a.cfg().InviteEnabled {
		result["error"] = "invite_disabled"
		return vm.ToValue(result)
	}
	if a.runtimeDatabaseMismatch() {
		result["error"] = "storage_mismatch"
		return vm.ToValue(result)
	}
	values, _ := options.(map[string]any)
	if values == nil {
		values = map[string]any{}
	}

	days := developerJSOptionInt(values, a.cfg().InviteDefaultDays, "days")
	maxDays, _ := a.maxCodeDays(*actor)
	if days <= 0 || days > maxDays {
		result["error"] = "days_out_of_range"
		result["max_days"] = maxDays
		return vm.ToValue(result)
	}

	expiresAt := int64(developerJSOptionInt(values, -1, "expires_at", "expired_at"))
	if expiresAt > 0 && expiresAt <= time.Now().Unix() {
		result["error"] = "expires_before_now"
		return vm.ToValue(result)
	}

	targetUsername := strings.TrimSpace(developerJSOptionString(values, "target_username"))
	if targetUsername != "" && !validRegcodeTargetUsername(targetUsername) {
		result["error"] = "invalid_target_username"
		return vm.ToValue(result)
	}

	note := truncateString(developerJSOptionString(values, "note"), 255)
	format := a.inviteCodeFormat(developerJSOptionString(values, "format"))
	algorithm := firstNonEmpty(developerJSOptionString(values, "random_algorithm", "algorithm"), a.cfg().InviteCodeRandomAlgorithm, "hex10")

	result["days"] = days
	if targetUsername != "" {
		result["target_username"] = targetUsername
	}

	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}

	code := ""
	for attempt := 0; attempt < 20; attempt++ {
		candidate := generateInviteCode(format, algorithm, days, 1)
		if _, exists := a.store().InviteCode(candidate); exists {
			continue
		}
		if _, exists := a.store().RegCode(candidate); exists {
			continue
		}
		code = candidate
		break
	}
	if code == "" {
		result["error"] = "generation_conflict"
		return vm.ToValue(result)
	}

	invite := store.InviteCode{
		Code:           code,
		UID:            actor.UID,
		InviterUID:     actor.UID,
		Days:           days,
		UseCountLimit:  1,
		Active:         true,
		Note:           note,
		TargetUsername: targetUsername,
		CreatedAt:      time.Now().Unix(),
		ExpiredAt:      expiresAt,
	}
	if err := a.store().UpsertInviteCode(invite); err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}

	result["ok"] = true
	result["code"] = code
	result["invite"] = developerJSInviteSnapshot(invite)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "invites.generate created invite code")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_invite_generate", "admin", 0, map[string]any{
		"code":         code,
		"days":         days,
		"script_api":   "invites.generate",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

// developerJSCreateAnnouncement mirrors handleCreateAnnouncement: admin-gated,
// preview-aware and audited. Render mode is normalized to the safe subset.
func (a *App) developerJSCreateAnnouncement(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, options any) goja.Value {
	result := map[string]any{"ok": false}
	if !developerJSAdminUser(actor, logs, "announcements.create") {
		result["error"] = "admin_required"
		return vm.ToValue(result)
	}
	values, _ := options.(map[string]any)
	if values == nil {
		values = map[string]any{}
	}

	title := firstNonEmpty(strings.TrimSpace(developerJSOptionString(values, "title")), "公告")
	content := developerJSOptionString(values, "content")
	level := firstNonEmpty(strings.TrimSpace(developerJSOptionString(values, "level")), "info")
	renderMode := safeAnnouncementRenderMode(developerJSOptionString(values, "render_mode"))
	visible, hasVisible := developerJSBoolOption(values, "visible")
	if !hasVisible {
		visible = true
	}
	pinned, _ := developerJSBoolOption(values, "pinned")
	expiresAt := int64(developerJSOptionInt(values, 0, "expires_at", "expired_at"))

	result["title"] = title
	result["level"] = level
	result["render_mode"] = renderMode
	result["visible"] = visible
	result["pinned"] = pinned

	if opts.Preview {
		result["dry_run"] = true
		result["ok"] = true
		return vm.ToValue(result)
	}

	ann, err := a.store().UpsertAnnouncement(store.Announcement{
		Title:        title,
		Content:      content,
		Visible:      visible,
		Level:        level,
		RenderMode:   renderMode,
		Pinned:       pinned,
		CreatedByUID: actor.UID,
		ExpiredAt:    expiresAt,
	})
	if err != nil {
		result["error"] = err.Error()
		return vm.ToValue(result)
	}

	result["ok"] = true
	result["announcement"] = developerJSAnnouncementSnapshot(ann)
	if logs != nil && len(*logs) < 8 {
		*logs = append(*logs, "announcements.create created announcement")
	}
	a.auditEntryIP("telegram", actor.UID, actor.Username, "telegram_js_announcement_create", "admin", 0, map[string]any{
		"id":           ann.ID,
		"title":        title,
		"script_api":   "announcements.create",
		"private_chat": opts.PrivateChat,
	})
	return vm.ToValue(result)
}

// developerJSQuickRegcode adapts the positional quick(days?, count?, type?) form
// to the options-object developerJSGenerateRegcode, so scripts can omit the options
// object for the common case. All validation, collision and audit semantics are reused.
func (a *App) developerJSQuickRegcode(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, call goja.FunctionCall) goja.Value {
	values := map[string]any{}
	if v := call.Argument(0); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["days"] = v.ToInteger()
	}
	if v := call.Argument(1); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["count"] = v.ToInteger()
	}
	if v := call.Argument(2); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["type"] = v.ToInteger()
	}
	return a.developerJSGenerateRegcode(vm, actor, opts, logs, values)
}

// developerJSQuickInvite adapts the positional quick(days?) form to
// developerJSGenerateInviteCode.
func (a *App) developerJSQuickInvite(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, call goja.FunctionCall) goja.Value {
	values := map[string]any{}
	if v := call.Argument(0); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["days"] = v.ToInteger()
	}
	return a.developerJSGenerateInviteCode(vm, actor, opts, logs, values)
}

// developerJSQuickAnnouncement adapts the positional post(title, content, level?)
// form to developerJSCreateAnnouncement.
func (a *App) developerJSQuickAnnouncement(vm *goja.Runtime, actor *store.User, opts developerJSRunOptions, logs *[]string, call goja.FunctionCall) goja.Value {
	values := map[string]any{}
	if v := call.Argument(0); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["title"] = v.String()
	}
	if v := call.Argument(1); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["content"] = v.String()
	}
	if v := call.Argument(2); !goja.IsUndefined(v) && !goja.IsNull(v) {
		values["level"] = v.String()
	}
	return a.developerJSCreateAnnouncement(vm, actor, opts, logs, values)
}

// developerJSOptionInt reads the first matching key from an exported JS options
// object as an int, falling back to the supplied default when absent.
func developerJSOptionInt(values map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			return int(numeric(raw))
		}
	}
	return fallback
}

// developerJSOptionString reads the first matching key as a trimmed-free string
// (no trim, callers trim where needed), returning "" when none are present.
func developerJSOptionString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if s, isStr := raw.(string); isStr {
				return s
			}
		}
	}
	return ""
}
