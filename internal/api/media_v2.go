package api

import (
	"net/http"
	"strings"
)

type v2MediaSearchResponse struct {
	Items    []map[string]any  `json:"items"`
	Total    int               `json:"total"`
	Warnings map[string]string `json:"warnings,omitempty"`
}

type v2MediaDetailResponse struct {
	Item map[string]any `json:"item"`
}

type v2MediaRequestListResponse struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

func (a *App) handleV2MediaSearch(w http.ResponseWriter, r *http.Request, params Params) {
	query := firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"), r.URL.Query().Get("keyword"))
	query = truncateString(strings.TrimSpace(query), 120)
	limit := clamp(queryInt(r, "limit", queryInt(r, "per_page", 20)), 1, 50)
	routeSource := params["source"]
	source := normalizeSource(firstNonEmpty(routeSource, r.URL.Query().Get("source"), "all"))
	mediaType := firstNonEmpty(r.URL.Query().Get("type"), r.URL.Query().Get("media_type"))
	results, _, sourceErrors := a.searchMedia(r.Context(), query, source, mediaType, limit, false)
	if source != "all" {
		if _, failed := sourceErrors[source]; failed {
			failWithCode(w, http.StatusBadGateway, ErrMediaSearchSourceFailed, "媒体搜索暂时不可用，请稍后重试")
			return
		}
	}
	// Upstream error details are intentionally reduced to source names. The
	// legacy endpoint exposed transport errors to clients, which can disclose
	// private upstream topology and makes UI error handling brittle.
	warnings := make(map[string]string, len(sourceErrors))
	for failedSource := range sourceErrors {
		warnings[failedSource] = "unavailable"
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", v2MediaSearchResponse{Items: results, Total: len(results), Warnings: warnings})
}

func (a *App) handleV2MediaDetail(w http.ResponseWriter, r *http.Request, params Params) {
	id := firstNonEmpty(params["media_id"], params["tmdb_id"], params["bgm_id"], r.URL.Query().Get("media_id"), r.URL.Query().Get("id"))
	if !isPositiveNumericID(id) {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "媒体编号无效")
		return
	}
	source := normalizeSource(firstNonEmpty(params["source_type"], r.URL.Query().Get("source"), "tmdb"))
	mediaType := firstNonEmpty(r.URL.Query().Get("media_type"), r.URL.Query().Get("type"), "movie")
	if source == "tmdb" {
		mediaType = normalizeTMDBMediaType(mediaType)
	}
	item, _ := a.mediaDetail(r.Context(), source, id, mediaType)
	if item == nil {
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "媒体条目不存在")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", v2MediaDetailResponse{Item: item})
}

func (a *App) handleV2MediaInventoryCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "media_id"), stringValue(payload, "id"), stringValue(payload, "tmdb_id")) == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "缺少必要参数")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", a.embyCheckInventory(r.Context(), payload))
}

func (a *App) handleV2MediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireMediaRequestEnabled(w) {
		return
	}
	requests := a.store().ListMediaRequests(current(r).User.UID, false)
	items := make([]map[string]any, 0, len(requests))
	for _, request := range requests {
		items = append(items, mediaRequestUserDTO(request))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", v2MediaRequestListResponse{Items: items, Total: len(items)})
}

// Mutations deliberately keep one implementation of entitlement checks,
// inventory checks, Store transactions, audit entries and revision conflicts.
// The V2 resource only changes the transport namespace used by the SSR app.
func (a *App) handleV2CreateMediaRequest(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleCreateMediaRequest(w, r, p)
}

func (a *App) handleV2MediaRequestByKey(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleMediaRequestByKey(w, r, p)
}
