package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleListRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store().ListRegCodes()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	statusFilter := strings.ToLower(r.URL.Query().Get("status"))
	typeFilter := r.URL.Query().Get("type")
	search := strings.ToLower(r.URL.Query().Get("search"))
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		dto := a.regcodeDTO(code)
		if typeFilter != "" && strconv.Itoa(code.Type) != typeFilter {
			continue
		}
		if statusFilter != "" && statusFilter != "all" && dto["status"] != statusFilter {
			if !(statusFilter == "decoy" && code.IsDecoy) && !(statusFilter == "active" && code.Active) {
				continue
			}
		}
		if search != "" && !strings.Contains(strings.ToLower(code.Code+" "+code.Note+" "+code.TargetUsername+" "+code.TargetTelegramUsername+" "+strconv.FormatInt(code.TargetTelegramID, 10)+" "+joinInt64(regcodeUsedByUIDs(code))+" "+joinInt64(code.UsedByTelegramIDs)), search) {
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
	targetCount := 0
	for _, hasTarget := range []bool{targetUsername != "", targetTelegramUsername != "", targetTelegramID > 0} {
		if hasTarget {
			targetCount++
		}
	}
	if targetCount > 1 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeTargetBad, "指名卡码只能指定一个目标：用户名、Telegram 用户名或 Telegram ID")
		return
	}
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		code := ""
		for attempt := 0; attempt < 20; attempt++ {
			candidate := generateRegCode(format, codeType, algorithm, days, i+1, validity, useLimit)
			if seen[candidate] {
				continue
			}
			if _, exists := a.store().RegCode(candidate); exists {
				continue
			}
			if _, exists := a.store().InviteCode(candidate); exists {
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
		if err := a.store().UpsertRegCode(store.RegCode{Code: code, Type: codeType, ValidityTime: validity, UseCountLimit: useLimit, Days: days, Note: truncateString(stringValue(payload, "note"), 120), IsDecoy: boolValue(payload, "decoy", false), TargetUsername: targetUsername, TargetTelegramUsername: targetTelegramUsername, TargetTelegramID: targetTelegramID, Active: true}); statusFromError(w, err) {
			return
		}
		codes = append(codes, code)
	}
	ok(w, "注册码已创建", map[string]any{"codes": codes, "count": len(codes), "decoy": boolValue(payload, "decoy", false), "target_username": targetUsername, "target_telegram_username": targetTelegramUsername, "target_telegram_id": zeroNil(targetTelegramID)})
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
	reg.Note = stringValue(decodeMap(r), "note")
	if err := a.store().UpsertRegCode(reg); statusFromError(w, err) {
		return
	}
	ok(w, "注册码已更新", a.regcodeDTO(reg))
}

func (a *App) handleDeleteRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	if a.rejectRegcodeWriteIfStorageMismatch(w) {
		return
	}
	if statusFromError(w, a.store().DeleteRegCode(params["code"])) {
		return
	}
	ok(w, "注册码已删除", nil)
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
	codes := regcodePayloadCodes(payload["codes"])
	if len(codes) == 0 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchEmpty, "请选择要删除的注册码")
		return
	}
	if len(codes) > 200 {
		failWithCode(w, http.StatusBadRequest, ErrRegcodeBatchTooLarge, "单次最多删除 200 个注册码")
		return
	}
	deleted, missing, err := a.store().DeleteRegCodes(codes)
	if err != nil {
		failWithCode(w, http.StatusInternalServerError, ErrRegcodeBatchFailed, "批量删除注册码失败")
		return
	}
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
	if err := a.store().UpsertRegCode(reg); statusFromError(w, err) {
		return
	}
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
