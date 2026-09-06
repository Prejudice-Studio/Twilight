package api

import (
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

type v2RegcodePagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type v2RegcodeListResponse struct {
	Items      []map[string]any    `json:"items"`
	Pagination v2RegcodePagination `json:"pagination"`
}

type v2RegcodeItemResponse struct {
	Item map[string]any `json:"item"`
}

type v2RegcodeUsageResponse struct {
	Item map[string]any `json:"item"`
}

func (a *App) handleV2AdminRegcodes(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	codes := a.store().ListRegCodes()
	page := clamp(queryInt(r, "page", 1), 1, 1_000_000)
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	filter := regcodeListFilterFromQuery(r.URL.Query())

	items := make([]map[string]any, 0, len(codes))
	byCode := make(map[string]store.RegCode, len(codes))
	for _, code := range codes {
		if !filter.matches(code) {
			continue
		}
		items = append(items, regcodeDTO(code))
		byCode[code.Code] = code
	}
	sortRegcodeDTOs(items, r.URL.Query().Get("sort"), r.URL.Query().Get("order"))
	total := len(items)
	start := (page - 1) * perPage
	if start < 0 || start >= total {
		items = []map[string]any{}
	} else {
		end := start + perPage
		if end > total {
			end = total
		}
		items = items[start:end]
	}
	for i, item := range items {
		if code, ok := byCode[asString(item["code"])]; ok {
			items[i] = a.regcodeDTO(code)
		}
	}
	totalPages := (total + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}
	ok(w, "OK", v2RegcodeListResponse{
		Items: items,
		Pagination: v2RegcodePagination{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

func (a *App) handleV2AdminRegcode(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	code := strings.TrimSpace(params["code"])
	reg, found := a.store().RegCode(code)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	ok(w, "OK", v2RegcodeItemResponse{Item: a.regcodeDTO(reg)})
}

func (a *App) handleV2AdminRegcodeUsage(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	code := strings.TrimSpace(params["code"])
	reg, found := a.store().RegCode(code)
	if !found {
		failWithCode(w, http.StatusNotFound, ErrRegcodeNotFound, "注册码不存在")
		return
	}
	users := make([]map[string]any, 0)
	seenUID := map[int64]bool{}
	for _, uid := range regcodeUsedByUIDs(reg) {
		if user, ok := a.store().User(uid); ok {
			item := publicUser(user)
			item["found"] = true
			item["source"] = "uid"
			users = append(users, item)
			seenUID[user.UID] = true
		} else {
			users = append(users, map[string]any{"uid": uid, "found": false, "source": "uid"})
		}
	}
	telegramOnly := make([]map[string]any, 0)
	for _, telegramID := range reg.UsedByTelegramIDs {
		if telegramID == 0 {
			continue
		}
		if user, ok := a.store().FindUserByTelegramID(telegramID); ok {
			if seenUID[user.UID] {
				continue
			}
			item := publicUser(user)
			item["found"] = true
			item["source"] = "telegram"
			users = append(users, item)
			seenUID[user.UID] = true
			continue
		}
		telegramOnly = append(telegramOnly, map[string]any{"telegram_id": telegramID, "found": false, "source": "telegram"})
	}
	ok(w, "OK", v2RegcodeUsageResponse{Item: map[string]any{
		"code": reg.Code, "use_count": reg.UseCount, "users": users,
		"telegram_only": telegramOnly, "unresolved_telegram_ids": reg.UsedByTelegramIDs,
		"total": len(users),
	}})
}

// V2 mutation routes deliberately delegate to the audited V1 implementations.
// They share the Store lock, storage-mismatch guard, validation, and audit
// behavior while the page moves to resource-oriented URLs.
func (a *App) handleV2CreateRegcodes(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleCreateRegcodes(w, r, p)
}

func (a *App) handleV2UpdateRegcode(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleUpdateRegcode(w, r, p)
}

func (a *App) handleV2DeleteRegcode(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleDeleteRegcode(w, r, p)
}

func (a *App) handleV2BatchDeleteRegcodes(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleBatchDeleteRegcodes(w, r, p)
}

func (a *App) handleV2ClearRegcodeUsage(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleClearRegcodeUsage(w, r, p)
}
