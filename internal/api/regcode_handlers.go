package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleListRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store().ListRegCodes()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	statusFilter := strings.ToLower(r.URL.Query().Get("status"))
	typeFilter := r.URL.Query().Get("type")
	sourceFilter := strings.ToLower(r.URL.Query().Get("source"))
	search := strings.ToLower(r.URL.Query().Get("search"))
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		dto := a.regcodeDTO(code)
		if typeFilter != "" && strconv.Itoa(code.Type) != typeFilter {
			continue
		}
		// source 筛选：admin 包含历史空值和显式 "admin"，invite 只匹配 "invite"
		if sourceFilter != "" && sourceFilter != "all" {
			codeSource := code.Source
			if codeSource == "" {
				codeSource = "admin"
			}
			if codeSource != sourceFilter {
				continue
			}
		}
		if statusFilter != "" && statusFilter != "all" && dto["status"] != statusFilter {
			if !(statusFilter == "decoy" && code.IsDecoy) && !(statusFilter == "active" && code.Active) {
				continue
			}
		}
		if !regcodeMatchesSearch(code, search) {
			continue
		}
		items = append(items, dto)
	}
	sortRegcodeDTOs(items, r.URL.Query().Get("sort"), r.URL.Query().Get("order"))
	total := len(items)
	items = paginate(items, page, perPage)
	ok(w, "OK", map[string]any{"regcodes": items, "total": total, "page": page, "per_page": perPage})
}

func (a *App) handleCreateRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	payload := decodeMap(r)
	count := intValue(payload, "count", 1)
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	days := normalizeRegCodeDays(intValue(payload, "days", 30))
	// 正天数封顶 36500（约百年），与 renew / set-expiry / bulk-expire 的口径一致：
	// 否则巨大的 Days 会在用户兑换时经 addDaysToExpiry 落进永久区间，绕过显式
	// 「-1 永久」路径静默发放永久权益。-1（永久）由 normalize 保留，放行。
	if days > 36500 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "days 不能超过 36500")
		return
	}
	codeType := intValue(payload, "type", 1)
	if codeType < 1 || codeType > 3 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTypeInvalid, "注册码类型无效")
		return
	}
	validity := int64(intValue(payload, "validity_time", -1))
	if validity == 0 {
		validity = -1
	}
	if validity < -1 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "卡码有效期只能为 -1 或正整数小时")
		return
	}
	useLimit := intValue(payload, "use_count_limit", 1)
	if useLimit == 0 {
		useLimit = 1
	}
	if useLimit < -1 {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "使用次数上限只能为 -1 或正整数")
		return
	}
	format := a.regCodeFormatForType(codeType, stringValue(payload, "format"))
	algorithm := firstNonEmpty(stringValue(payload, "random_algorithm"), a.cfg().RegCodeRandomAlgorithm, "base32-20")
	codes := make([]string, 0, count)
	targetUsername := strings.TrimSpace(stringValue(payload, "target_username"))
	targetTelegramUsername := normalizeRegcodeTargetTelegramUsername(firstNonEmpty(stringValue(payload, "target_telegram_username"), stringValue(payload, "target_tg_username"), stringValue(payload, "target_tgusername"), stringValue(payload, "tgusername")))
	rawTargetTelegramID := firstNonEmpty(stringValue(payload, "target_telegram_id"), stringValue(payload, "target_tg_id"), stringValue(payload, "target_tg_userid"), stringValue(payload, "target_tguserid"), stringValue(payload, "tguserid"))
	targetTelegramID := numeric(rawTargetTelegramID)
	targetUID := int64(numeric(payload["target_uid"]))
	if targetUsername != "" && !validRegcodeTargetUsername(targetUsername) {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "目标用户名长度需为 3-32 个字符，且不能包含特殊路径或注入字符")
		return
	}
	if targetTelegramUsername != "" && !validRegcodeTargetTelegramUsername(targetTelegramUsername) {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "目标 Telegram 用户名需为 5-32 个字符，只能包含字母、数字和下划线")
		return
	}
	if rawTargetTelegramID != "" && targetTelegramID <= 0 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "目标 Telegram ID 必须为正整数")
		return
	}
	if targetUID > 0 {
		if _, ok := a.store().User(targetUID); !ok {
			failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "目标 UID 对应的用户不存在")
			return
		}
	}
	targetCount := 0
	for _, hasTarget := range []bool{targetUsername != "", targetTelegramUsername != "", targetTelegramID > 0, targetUID > 0} {
		if hasTarget {
			targetCount++
		}
	}
	if targetCount > 1 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "指名卡码只能指定一个目标：用户名、Telegram 用户名、Telegram ID 或 Web 账号 UID")
		return
	}
	seen := map[string]bool{}
	// 预加载已存在的卡码到集合，避免逐次 RegCode()/InviteCode() 查 store。
	existingCodes := map[string]bool{}
	for _, c := range a.store().ListRegCodes() {
		existingCodes[c.Code] = true
	}
	for _, c := range a.store().ListAllInviteCodes() {
		existingCodes[c.Code] = true
	}
	// 短随机码(<=16 位)碰撞概率稍高，允许更多尝试；长码已足够唯一。
	maxAttempts := 10
	if len(algorithm) > 0 && strings.Contains(algorithm, "16") {
		maxAttempts = 20
	}
	if algorithm == "digits-12" || algorithm == "hex10" {
		maxAttempts = 30
	}
	admin := current(r).User
	note := truncateString(stringValue(payload, "note"), 120)
	isDecoy := boolValue(payload, "decoy", false)
	pending := make([]store.RegCode, 0, count)
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
			failWithCode(w, http.StatusConflict, ErrRegcodeGenerateConflict, "注册码生成冲突，请调整格式或随机算法后重试")
			return
		}
		seen[code] = true
		pending = append(pending, store.RegCode{Code: code, Type: codeType, ValidityTime: validity, UseCountLimit: useLimit, Days: days, Note: note, IsDecoy: isDecoy, TargetUsername: targetUsername, TargetTelegramUsername: targetTelegramUsername, TargetTelegramID: targetTelegramID, TargetUID: targetUID, Active: true, Source: "admin", CreatorUID: admin.UID})
		codes = append(codes, code)
	}
	// 一次性落盘：避免逐条 UpsertRegCode 触发 count 次全量状态序列化写入。
	if err := a.store().UpsertRegCodes(pending); statusFromError(w, err) {
		return
	}
	a.audit(r, "create_regcode", "admin", 0, map[string]any{
		"count": len(codes), "type": codeType, "days": days, "codes": codes,
	})
	ok(w, "注册码已创建", map[string]any{"codes": codes, "count": len(codes), "decoy": boolValue(payload, "decoy", false), "target_username": targetUsername, "target_telegram_username": targetTelegramUsername, "target_telegram_id": zeroNil(targetTelegramID), "target_uid": zeroNil(targetUID)})
}

func (a *App) handleUpdateRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	reg, okReg := a.store().RegCode(params["code"])
	if !okReg {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	payload := decodeMap(r)
	// 部分更新：只改动 payload 中显式出现的字段，缺省字段保持原值。
	// 支持备注、停用/启用、有效期（小时）、授予天数、使用次数上限。
	if _, has := payload["note"]; has {
		reg.Note = truncateString(stringValue(payload, "note"), 120)
	}
	if _, has := payload["active"]; has {
		was := reg.Active
		reg.Active = boolValue(payload, "active", reg.Active)
		now := time.Now().Unix()
		if was && !reg.Active {
			reg.PauseStart = now
		} else if !was && reg.Active {
			if reg.PauseStart > 0 {
				reg.PausedSeconds += now - reg.PauseStart
			}
			reg.PauseStart = 0
		}
	}
	if _, has := payload["validity_time"]; has {
		validity := int64(intValue(payload, "validity_time", int(reg.ValidityTime)))
		if validity == 0 {
			validity = -1
		}
		if validity < -1 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "卡码有效期只能为 -1 或正整数小时")
			return
		}
		reg.ValidityTime = validity
	}
	if _, has := payload["days"]; has {
		days := normalizeRegCodeDays(intValue(payload, "days", reg.Days))
		// 与创建口径一致：正天数封顶 36500，避免静默发放永久权益。
		if days > 36500 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "days 不能超过 36500")
			return
		}
		reg.Days = days
	}
	if _, has := payload["use_count_limit"]; has {
		useLimit := intValue(payload, "use_count_limit", reg.UseCountLimit)
		if useLimit == 0 {
			useLimit = 1
		}
		if useLimit < -1 {
			failWithCode(w, http.StatusBadRequest, ErrBadRequest, "使用次数上限只能为 -1 或正整数")
			return
		}
		reg.UseCountLimit = useLimit
	}
	if err := a.store().UpsertRegCode(reg); statusFromError(w, err) {
		return
	}
	a.audit(r, "update_regcode", "admin", 0, map[string]any{
		"code": reg.Code,
	})
	ok(w, "注册码已更新", a.regcodeDTO(reg))
}

func (a *App) handleDeleteRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	code := strings.TrimSpace(params["code"])
	if code == "" {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeNotFound, "注册码不能为空")
		return
	}
	reg, okReg := a.store().RegCode(code)
	if !okReg {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	if statusFromError(w, a.store().DeleteRegCode(code)) {
		return
	}
	a.audit(r, "delete_regcode", "admin", 0, map[string]any{
		"code": code, "type": reg.Type, "days": reg.Days,
	})
	ok(w, "注册码已删除", map[string]any{
		"deleted":       1,
		"deleted_codes": []string{code},
		"missing":       0,
		"missing_codes": []string{},
	})
}

func validRegcodeTargetUsername(username string) bool {
	return len(username) >= 3 && len(username) <= 32 && !strings.ContainsAny(username, "/\\@:\x00<>\"'&")
}

func normalizeRegcodeTargetTelegramUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func validRegcodeTargetTelegramUsername(username string) bool {
	if len(username) < 5 || len(username) > 32 {
		return false
	}
	for _, ch := range username {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *App) handleBatchDeleteRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmBatchDeleteRegcodes {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchConfirm, "missing confirm "+confirmBatchDeleteRegcodes)
		return
	}
	var codes []string
	selectAll := boolValue(payload, "select_all", false)
	if selectAll {
		// 跨页“选中全部匹配”：按与列表完全一致的筛选口径解析目标码，
		// 再扣除前端明确排除的 exclude_codes，避免把筛选外的码卷入删除。
		codes = a.filteredBatchRegcodeCodes(payload, 0)
	} else {
		codes = regcodePayloadCodes(payload["codes"])
	}
	if len(codes) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchEmpty, "请选择要删除的注册码")
		return
	}
	if !selectAll && len(codes) > 200 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchTooLarge, "单次最多删除 200 个注册码")
		return
	}
	if selectAll && len(codes) > 5000 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchTooLarge, "选中全部匹配时最多删除 5000 个注册码，请缩小筛选范围")
		return
	}
	deleted, missing, err := a.store().DeleteRegCodes(codes)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrRegcodeBatchFailed, "批量删除注册码失败")
		return
	}
	a.audit(r, "batch_delete_regcode", "admin", 0, map[string]any{
		"count": len(deleted), "deleted": deleted, "missing": len(missing), "select_all": selectAll,
	})
	ok(w, "注册码已批量删除", map[string]any{
		"deleted":       len(deleted),
		"deleted_codes": deleted,
		"missing":       len(missing),
		"missing_codes": missing,
	})
}

func (a *App) handleRegcodeUsers(w http.ResponseWriter, r *http.Request, params Params) {
	reg, okReg := a.store().RegCode(params["code"])
	if !okReg {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	users := []map[string]any{}
	seenUID := map[int64]bool{}
	for _, uid := range regcodeUsedByUIDs(reg) {
		if u, okUser := a.store().User(uid); okUser {
			item := publicUser(u)
			item["found"] = true
			item["source"] = "uid"
			users = append(users, item)
			seenUID[u.UID] = true
		} else {
			users = append(users, map[string]any{"uid": uid, "found": false, "source": "uid"})
		}
	}
	telegramOnly := []map[string]any{}
	for _, telegramID := range reg.UsedByTelegramIDs {
		if telegramID == 0 {
			continue
		}
		if u, okUser := a.store().FindUserByTelegramID(telegramID); okUser {
			if seenUID[u.UID] {
				continue
			}
			item := publicUser(u)
			item["found"] = true
			item["source"] = "telegram"
			users = append(users, item)
			seenUID[u.UID] = true
			continue
		}
		telegramOnly = append(telegramOnly, map[string]any{"telegram_id": telegramID, "found": false, "source": "telegram"})
	}
	ok(w, "OK", map[string]any{"code": reg.Code, "use_count": reg.UseCount, "users": users, "telegram_only": telegramOnly, "unresolved_telegram_ids": reg.UsedByTelegramIDs, "total": len(users)})
}

func sortRegcodeDTOs(items []map[string]any, sortKey, order string) {
	sortKey = strings.ToLower(strings.TrimSpace(sortKey))
	if sortKey == "" {
		sortKey = "created_time"
	}
	desc := !strings.EqualFold(order, "asc")
	less := func(i, j int) bool {
		a, b := items[i], items[j]
		switch sortKey {
		case "code", "note", "status", "type_name", "target_username":
			return strings.ToLower(asString(a[sortKey])) < strings.ToLower(asString(b[sortKey]))
		case "type", "days", "use_count", "use_count_limit", "validity_time", "created_time":
			return numeric(a[sortKey]) < numeric(b[sortKey])
		default:
			return numeric(a["created_time"]) < numeric(b["created_time"])
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return less(j, i)
		}
		return less(i, j)
	})
}

// handleClearRegcodeUsage 一键清理注册码的使用记录（UsedByUIDs、UsedByTelegramIDs、UseCount）。
// 适用于因历史不兼容导致的脏数据清理场景。清理后注册码恢复为"未使用"状态，
// 但不会影响已注册用户的账号。
func (a *App) handleClearRegcodeUsage(w http.ResponseWriter, r *http.Request, params Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	payload := decodeMap(r)
	if stringValue(payload, "confirm") != confirmClearRegcodeUsage {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchConfirm, "需要确认短语 confirm="+confirmClearRegcodeUsage)
		return
	}
	reg, okReg := a.store().RegCode(params["code"])
	if !okReg {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	oldUseCount := reg.UseCount
	oldUsedByUIDs := regcodeUsedByUIDs(reg)
	oldUsedByTelegramIDs := reg.UsedByTelegramIDs
	reg.UseCount = 0
	reg.UsedBy = 0
	reg.UsedByUIDs = nil
	reg.UsedByTelegramIDs = nil
	reg.Active = true
	now := time.Now().Unix()
	if reg.PauseStart > 0 {
		reg.PausedSeconds += now - reg.PauseStart
	}
	reg.PauseStart = 0
	if err := a.store().UpsertRegCode(reg); statusFromError(w, err) {
		return
	}
	a.audit(r, "clear_regcode_usage", "admin", 0, map[string]any{
		"code": reg.Code, "cleared_use_count": oldUseCount,
	})
	ok(w, "使用记录已清理", map[string]any{
		"code":                     reg.Code,
		"cleared_use_count":        oldUseCount,
		"cleared_used_by_uids":     oldUsedByUIDs,
		"cleared_used_by_telegram": oldUsedByTelegramIDs,
	})
}

func regcodePayloadCodes(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		code := strings.TrimSpace(asString(item))
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}

// filteredBatchRegcodeCodes 按与 handleListRegcodes 完全一致的筛选口径
// （type / status / source / search）解析“选中全部匹配”要删除的注册码集合，
// 并扣除前端 exclude_codes 中明确排除的码。filter 口径必须与列表 handler
// 保持一致，否则“按筛选全选”会把筛选外的码卷入批量删除（AGENTS.md filter 口径一致约定）。
// limit<=0 表示不限制返回数量。
func (a *App) filteredBatchRegcodeCodes(payload map[string]any, limit int) []string {
	filter, _ := payload["filter"].(map[string]any)
	typeFilter := strings.TrimSpace(asString(filter["type"]))
	statusFilter := strings.ToLower(strings.TrimSpace(asString(filter["status"])))
	sourceFilter := strings.ToLower(strings.TrimSpace(asString(filter["source"])))
	search := strings.ToLower(strings.TrimSpace(asString(filter["search"])))

	excluded := map[string]bool{}
	for _, code := range regcodePayloadCodes(payload["exclude_codes"]) {
		excluded[code] = true
	}

	codes := a.store().ListRegCodes()
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		dto := a.regcodeDTO(code)
		if typeFilter != "" && strconv.Itoa(code.Type) != typeFilter {
			continue
		}
		if sourceFilter != "" && sourceFilter != "all" {
			codeSource := code.Source
			if codeSource == "" {
				codeSource = "admin"
			}
			if codeSource != sourceFilter {
				continue
			}
		}
		if statusFilter != "" && statusFilter != "all" && dto["status"] != statusFilter {
			if !(statusFilter == "decoy" && code.IsDecoy) && !(statusFilter == "active" && code.Active) {
				continue
			}
		}
		if !regcodeMatchesSearch(code, search) {
			continue
		}
		if excluded[code.Code] {
			continue
		}
		out = append(out, code.Code)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
