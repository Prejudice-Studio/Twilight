package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleMediaSearch(w http.ResponseWriter, r *http.Request, _ Params) {
	query := firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"), r.URL.Query().Get("keyword"))
	limit := clamp(queryInt(r, "limit", queryInt(r, "per_page", 20)), 1, 50)
	source := normalizeSource(firstNonEmpty(r.URL.Query().Get("source"), "all"))
	mediaType := firstNonEmpty(r.URL.Query().Get("type"), r.URL.Query().Get("media_type"))
	results, message := a.searchMedia(r.Context(), query, source, mediaType, limit, false)
	ok(w, message, map[string]any{"results": results, "total": len(results)})
}

func (a *App) handleMediaDetail(w http.ResponseWriter, r *http.Request, params Params) {
	id := firstNonEmpty(params["media_id"], params["tmdb_id"], params["bgm_id"], r.URL.Query().Get("media_id"))
	if id == "" {
		id = firstNonEmpty(r.URL.Query().Get("id"), "0")
	}
	source := normalizeSource(firstNonEmpty(params["source_type"], r.URL.Query().Get("source"), "tmdb"))
	mediaType := firstNonEmpty(r.URL.Query().Get("media_type"), r.URL.Query().Get("type"), "movie")
	result, found := a.mediaDetail(r.Context(), source, id, mediaType)
	if !found {
		result = mediaResultFromFields(source, id, "", mediaType, "")
	}
	ok(w, "OK", result)
}

func (a *App) handleInventoryCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "media_id"), stringValue(payload, "id"), stringValue(payload, "tmdb_id")) == "" {
		fail(w, http.StatusBadRequest, "缂哄皯蹇呰鍙傛暟")
		return
	}
	result := a.embyCheckInventory(r.Context(), payload)
	ok(w, asString(result["message"]), result)
}

func (a *App) handleInventorySearch(w http.ResponseWriter, r *http.Request, _ Params) {
	query := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query")))
	if query == "" {
		fail(w, http.StatusBadRequest, "missing search query")
		return
	}
	if a.cfg.EmbyURL == "" {
		fail(w, http.StatusBadRequest, "Emby not configured")
		return
	}
	limit := clamp(queryInt(r, "limit", 20), 1, 50)
	itemType := strings.TrimSpace(r.URL.Query().Get("type"))
	includeTypes := []string{"Movie", "Series"}
	if itemType != "" {
		includeTypes = []string{itemType}
	}
	items, err := a.embySearchItems(r.Context(), query, includeTypes, queryInt(r, "year", 0), limit)
	if err != nil {
		fail(w, http.StatusBadGateway, "搜索库存失败")
		return
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, embyItemDTO(item))
	}
	ok(w, fmt.Sprintf("found %d results", len(results)), map[string]any{"query": query, "count": len(results), "results": results, "total": len(results)})
}

func (a *App) handleCreateMediaRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	if !a.cfg.MediaRequestEnabled {
		fail(w, http.StatusForbidden, "media requests are disabled")
		return
	}
	p := current(r)
	if p.User.TelegramID == 0 {
		fail(w, http.StatusBadRequest, "请先在个人设置中绑定 Telegram 账号后再进行求片")
		return
	}
	if a.cfg.MaxConcurrentRequestsPerUser > 0 && a.store.ActiveMediaRequestCount(p.User.UID) >= a.cfg.MaxConcurrentRequestsPerUser {
		fail(w, http.StatusTooManyRequests, "pending media request limit reached")
		return
	}
	payload := decodeMap(r)
	title := firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "name"), "Unknown")
	source := normalizeSource(firstNonEmpty(stringValue(payload, "source"), "tmdb"))
	mediaID, _ := strconv.ParseInt(firstNonEmpty(stringValue(payload, "media_id"), stringValue(payload, "tmdb_id"), stringValue(payload, "bgm_id"), "0"), 10, 64)
	mediaInfo := map[string]any{"title": title, "source": source}
	for key, value := range payload {
		mediaInfo[key] = value
	}
	if !boolValue(payload, "skip_inventory_check", false) {
		inventoryPayload := cloneMap(mediaInfo)
		inventoryPayload["source"] = source
		inventoryPayload["media_id"] = mediaID
		inventoryPayload["media_type"] = firstNonEmpty(stringValue(payload, "media_type"), stringValue(payload, "type"), "movie")
		inventoryPayload["season"] = intValue(payload, "season", 0)
		inventory := a.embyCheckInventory(r.Context(), inventoryPayload)
		if boolish(inventory["exists"]) {
			fail(w, http.StatusBadRequest, "media already exists: "+asString(inventory["message"]))
			return
		}
		mediaInfo["inventory_checked"] = true
		mediaInfo["inventory_message"] = inventory["message"]
	}
	if mediaID == 0 {
		mediaID = int64(time.Now().UnixNano())
	}
	req, err := a.store.CreateMediaRequest(store.MediaRequest{UID: p.User.UID, TelegramID: p.User.TelegramID, Username: p.User.Username, Title: title, OriginalTitle: stringValue(payload, "original_title"), Source: source, MediaID: mediaID, MediaType: firstNonEmpty(stringValue(payload, "media_type"), stringValue(payload, "type"), "movie"), Season: intValue(payload, "season", 0), Year: stringValue(payload, "year"), Note: truncateString(stringValue(payload, "note"), 500), MediaInfo: mediaInfo})
	if statusFromError(w, err) {
		return
	}
	created(w, "media request submitted", mediaRequestUserDTO(req))
}

func (a *App) handleMyMediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	requests := a.store.ListMediaRequests(current(r).User.UID, false)
	items := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		items = append(items, mediaRequestUserDTO(req))
	}
	ok(w, "OK", items)
}

func (a *App) handleAdminMediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	statusFilter := strings.ToLower(firstNonEmpty(r.URL.Query().Get("status"), "pending"))
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	requests := a.store.ListMediaRequests(0, true)
	items := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		if !mediaStatusMatches(req.Status, statusFilter) {
			continue
		}
		items = append(items, mediaRequestAdminDTO(req, a.store))
	}
	total := len(items)
	items = paginate(items, page, perPage)
	ok(w, "OK", map[string]any{"requests": items, "total": total, "page": page, "per_page": perPage})
}

func (a *App) handleUpdateMediaRequestStatus(w http.ResponseWriter, r *http.Request, params Params) {
	if current(r).User.Role != store.RoleAdmin {
		fail(w, http.StatusForbidden, "需要管理员权限")
		return
	}
	id, _ := int64Param(params, "request_id")
	payload := decodeMap(r)
	status := normalizeMediaStatus(firstNonEmpty(stringValue(payload, "status"), "ACCEPTED"))
	if status == "" {
		fail(w, http.StatusBadRequest, "invalid status")
		return
	}
	note := truncateString(firstNonEmpty(stringValue(payload, "note"), stringValue(payload, "admin_note")), 1000)
	req, err := a.store.UpdateMediaRequest(id, func(req *store.MediaRequest) error {
		req.Status = status
		if note != "" {
			req.AdminNote = note
		}
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "鐘舵€佸凡鏇存柊", mediaRequestAdminDTO(req, a.store))
}

func (a *App) handleUpdateMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	req, okReq := a.store.FindMediaRequestByKey(params["require_key"])
	if !okReq {
		fail(w, http.StatusNotFound, "request not found")
		return
	}
	params["request_id"] = strconv.FormatInt(req.ID, 10)
	a.handleUpdateMediaRequestStatus(w, r, params)
}

func (a *App) handleExternalMediaUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	secret := firstNonEmpty(r.Header.Get("X-Internal-Secret"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if a.cfg.BotInternalSecret == "" || secret != a.cfg.BotInternalSecret {
		fail(w, http.StatusForbidden, "鍐呴儴瀵嗛挜鏃犳晥")
		return
	}
	payload := decodeMap(r)
	key := firstNonEmpty(stringValue(payload, "key"), stringValue(payload, "require_key"))
	req, okReq := a.store.FindMediaRequestByKey(key)
	if !okReq {
		fail(w, http.StatusNotFound, "request not found")
		return
	}
	status := normalizeMediaStatus(firstNonEmpty(stringValue(payload, "status"), "ACCEPTED"))
	if status == "" {
		fail(w, http.StatusBadRequest, "invalid status")
		return
	}
	req, err := a.store.UpdateMediaRequest(req.ID, func(req *store.MediaRequest) error {
		req.Status = status
		req.AdminNote = truncateString(stringValue(payload, "note"), 1000)
		return nil
	})
	if statusFromError(w, err) {
		return
	}
	ok(w, "鐘舵€佸凡鏇存柊", mediaRequestAdminDTO(req, a.store))
}

func (a *App) handleMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	req, okReq := a.store.FindMediaRequestByKey(params["require_key"])
	if !okReq {
		fail(w, http.StatusNotFound, "request not found")
		return
	}
	if !canAccessMediaRequest(current(r).User, req) {
		fail(w, http.StatusForbidden, "cannot access this request")
		return
	}
	ok(w, "OK", mediaRequestUserDTO(req))
}

func (a *App) handleDeleteMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	req, okReq := a.store.FindMediaRequestByKey(params["require_key"])
	if !okReq {
		fail(w, http.StatusNotFound, "request not found")
		return
	}
	if !canAccessMediaRequest(current(r).User, req) {
		fail(w, http.StatusForbidden, "cannot delete this request")
		return
	}
	if statusFromError(w, a.store.DeleteMediaRequest(req.ID)) {
		return
	}
	ok(w, "request deleted", nil)
}

func (a *App) handleMediaRequestByID(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "request_id")
	req, okReq := a.store.MediaRequest(id)
	if okReq {
		if !canAccessMediaRequest(current(r).User, req) {
			fail(w, http.StatusForbidden, "cannot access this request")
			return
		}
		ok(w, "OK", mediaRequestUserDTO(req))
		return
	}
	fail(w, http.StatusNotFound, "request not found")
}

func (a *App) handleDeleteMediaRequest(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "request_id")
	if req, okReq := a.store.MediaRequest(id); okReq && !canAccessMediaRequest(current(r).User, req) {
		fail(w, http.StatusForbidden, "cannot delete this request")
		return
	}
	if statusFromError(w, a.store.DeleteMediaRequest(id)) {
		return
	}
	ok(w, "request deleted", nil)
}
