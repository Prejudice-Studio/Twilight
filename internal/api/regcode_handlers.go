package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleListRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	codes := a.store.ListRegCodes()
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	statusFilter := strings.ToLower(r.URL.Query().Get("status"))
	typeFilter := r.URL.Query().Get("type")
	search := strings.ToLower(r.URL.Query().Get("search"))
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		dto := regcodeDTO(code)
		if typeFilter != "" && strconv.Itoa(code.Type) != typeFilter {
			continue
		}
		if statusFilter != "" && statusFilter != "all" && dto["status"] != statusFilter {
			if !(statusFilter == "decoy" && code.IsDecoy) {
				continue
			}
		}
		if search != "" && !strings.Contains(strings.ToLower(code.Code+" "+code.Note), search) {
			continue
		}
		items = append(items, dto)
	}
	total := len(items)
	items = paginate(items, page, perPage)
	ok(w, "OK", map[string]any{"regcodes": items, "total": total, "page": page, "per_page": perPage})
}

func (a *App) handleCreateRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	count := intValue(payload, "count", 1)
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	days := intValue(payload, "days", 30)
	codeType := intValue(payload, "type", 1)
	if codeType < 1 || codeType > 3 {
		fail(w, http.StatusBadRequest, "invalid regcode type")
		return
	}
	validity := int64(intValue(payload, "validity_time", -1))
	useLimit := intValue(payload, "use_count_limit", 1)
	format := firstNonEmpty(stringValue(payload, "format"), "TW-{type}-{random}")
	algorithm := firstNonEmpty(stringValue(payload, "random_algorithm"), "base32-20")
	codes := make([]string, 0, count)
	targetUsername := strings.TrimSpace(stringValue(payload, "target_username"))
	for i := 0; i < count; i++ {
		code := generateRegCode(format, codeType, algorithm)
		_ = a.store.UpsertRegCode(store.RegCode{Code: code, Type: codeType, ValidityTime: validity, UseCountLimit: useLimit, Days: days, Note: truncateString(stringValue(payload, "note"), 120), IsDecoy: boolValue(payload, "decoy", false), TargetUsername: targetUsername, Active: true})
		codes = append(codes, code)
	}
	ok(w, "注册码已创建", map[string]any{"codes": codes, "count": len(codes), "decoy": boolValue(payload, "decoy", false), "target_username": targetUsername})
}

func (a *App) handleUpdateRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	reg, okReg := a.store.RegCode(params["code"])
	if !okReg {
		fail(w, http.StatusNotFound, "娉ㄥ唽鐮佷笉瀛樺湪")
		return
	}
	reg.Note = stringValue(decodeMap(r), "note")
	_ = a.store.UpsertRegCode(reg)
	ok(w, "娉ㄥ唽鐮佸凡鏇存柊", regcodeDTO(reg))
}

func (a *App) handleDeleteRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	if statusFromError(w, a.store.DeleteRegCode(params["code"])) {
		return
	}
	ok(w, "娉ㄥ唽鐮佸凡鍒犻櫎", nil)
}

func (a *App) handleBatchDeleteRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	codes := regcodePayloadCodes(payload["codes"])
	if len(codes) == 0 {
		fail(w, http.StatusBadRequest, "请选择要删除的注册码")
		return
	}
	if len(codes) > 200 {
		fail(w, http.StatusBadRequest, "单次最多删除 200 个注册码")
		return
	}
	deleted, missing, err := a.store.DeleteRegCodes(codes)
	if err != nil {
		fail(w, http.StatusInternalServerError, "批量删除注册码失败")
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
	reg, okReg := a.store.RegCode(params["code"])
	if !okReg {
		fail(w, http.StatusNotFound, "娉ㄥ唽鐮佷笉瀛樺湪")
		return
	}
	users := []map[string]any{}
	for _, uid := range reg.UsedByUIDs {
		if u, okUser := a.store.User(uid); okUser {
			users = append(users, publicUser(u))
		}
	}
	ok(w, "OK", map[string]any{"users": users, "unresolved_telegram_ids": reg.UsedByTelegramIDs, "total": len(users)})
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
