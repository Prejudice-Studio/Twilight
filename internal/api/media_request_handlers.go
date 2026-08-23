package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

func (a *App) handleMediaSearch(w http.ResponseWriter, r *http.Request, params Params) {
	query := firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"), r.URL.Query().Get("keyword"))
	limit := clamp(queryInt(r, "limit", queryInt(r, "per_page", 20)), 1, 50)
	// The source-specific aliases are part of the public API and take precedence
	// over a query parameter, so the URL path and returned source cannot diverge.
	routeSource := params["source"]
	if routeSource == "" {
		switch r.URL.Path {
		case "/api/v1/media/search/tmdb":
			routeSource = "tmdb"
		case "/api/v1/media/search/bangumi":
			routeSource = "bangumi"
		}
	}
	source := normalizeSource(firstNonEmpty(routeSource, r.URL.Query().Get("source"), "all"))
	mediaType := firstNonEmpty(r.URL.Query().Get("type"), r.URL.Query().Get("media_type"))
	results, message, sourceErrors := a.searchMedia(r.Context(), query, source, mediaType, limit, false)
	if source != "all" {
		if detail := sourceErrors[source]; detail != "" {
			failWithCode(w, http.StatusBadGateway, ErrMediaSearchSourceFailed, detail)
			return
		}
	}
	data := map[string]any{"results": results, "total": len(results)}
	if len(sourceErrors) > 0 {
		data["warnings"] = sourceErrors
	}
	ok(w, message, data)
}

func (a *App) handleMediaDetail(w http.ResponseWriter, r *http.Request, params Params) {
	id := firstNonEmpty(params["media_id"], params["tmdb_id"], params["bgm_id"], r.URL.Query().Get("media_id"))
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	source := normalizeSource(firstNonEmpty(params["source_type"], r.URL.Query().Get("source"), "tmdb"))
	if !isPositiveNumericID(id) {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "media_id invalid")
		return
	}
	mediaType := firstNonEmpty(r.URL.Query().Get("media_type"), r.URL.Query().Get("type"), "movie")
	if source == "tmdb" {
		mediaType = normalizeTMDBMediaType(mediaType)
	}
	result, found := a.mediaDetail(r.Context(), source, id, mediaType)
	if !found {
		result = mediaResultFromFields(source, id, "", mediaType, "")
	}
	ok(w, "OK", result)
}

func (a *App) handleInventoryCheck(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	if firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "media_id"), stringValue(payload, "id"), stringValue(payload, "tmdb_id")) == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "缺少必要参数")
		return
	}
	result := a.embyCheckInventory(r.Context(), payload)
	ok(w, asString(result["message"]), result)
}

func (a *App) handleInventorySearch(w http.ResponseWriter, r *http.Request, _ Params) {
	query := strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query")))
	if query == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestQueryRequired, "missing search query")
		return
	}
	if a.requireEmbyConfigured(w) {
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
		failWithCode(w, http.StatusBadGateway, ErrMediaInventorySearchFailed, "搜索库存失败")
		return
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		results = append(results, embyItemDTO(item))
	}
	ok(w, fmt.Sprintf("found %d results", len(results)), map[string]any{"query": query, "count": len(results), "results": results, "total": len(results)})
}

func (a *App) handleCreateMediaRequest(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireMediaRequestEnabled(w) {
		return
	}
	p := current(r)
	if a.requireEmailVerified(w, p.User) {
		return
	}
	if p.User.TelegramID == 0 {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestTGRequired, "请先在个人设置中绑定 Telegram 账号后再进行求片")
		return
	}

	payload := decodeMap(r)
	title := firstNonEmpty(stringValue(payload, "title"), stringValue(payload, "name"), "Unknown")
	source := normalizeSource(firstNonEmpty(stringValue(payload, "source"), "tmdb"))
	mediaID, _ := strconv.ParseInt(firstNonEmpty(stringValue(payload, "media_id"), stringValue(payload, "tmdb_id"), stringValue(payload, "bgm_id"), "0"), 10, 64)
	mediaType := firstNonEmpty(stringValue(payload, "media_type"), stringValue(payload, "type"), "movie")
	season := intValue(payload, "season", 0)
	mediaInfo := map[string]any{"title": title, "source": source}
	for key, value := range payload {
		mediaInfo[key] = value
	}
	note := truncateString(stringValue(payload, "note"), 500)
	if !(p.User.Role == store.RoleAdmin && boolValue(payload, "skip_inventory_check", false)) {
		inventoryPayload := cloneMap(mediaInfo)
		inventoryPayload["source"] = source
		inventoryPayload["media_id"] = mediaID
		inventoryPayload["media_type"] = mediaType
		inventoryPayload["season"] = season
		inventory := a.embyCheckInventory(r.Context(), inventoryPayload)
		if boolish(inventory["exists"]) {
			if strings.TrimSpace(note) == "" {
				failWithCode(w, http.StatusBadRequest, ErrMediaRequestExists, "media already exists: "+asString(inventory["message"]))
				return
			}
			mediaInfo["inventory_issue"] = true
			mediaInfo["inventory_exists"] = true
			mediaInfo["inventory_message"] = inventory["message"]
		} else {
			mediaInfo["inventory_message"] = inventory["message"]
		}
		mediaInfo["inventory_checked"] = true
	}
	if mediaID == 0 {
		mediaID = int64(time.Now().UnixNano())
	}

	createOpts := store.MediaRequestCreateOptions{UserActiveLimit: a.cfg().MaxConcurrentRequestsPerUser}
	// Global queue limit does not apply to admins so they can still handle urgent requests.
	if globalLimit := a.cfg().MaxConcurrentRequestsGlobal; globalLimit > 0 && p.User.Role != store.RoleAdmin {
		createOpts.GlobalActiveLimit = globalLimit
	}
	req, err := a.store().CreateMediaRequestWithOptions(store.MediaRequest{
		UID:           p.User.UID,
		TelegramID:    p.User.TelegramID,
		Username:      p.User.Username,
		Title:         title,
		OriginalTitle: stringValue(payload, "original_title"),
		Source:        source,
		MediaID:       mediaID,
		MediaType:     mediaType,
		Season:        season,
		Year:          stringValue(payload, "year"),
		Note:          note,
		MediaInfo:     mediaInfo,
	}, createOpts)
	if errors.Is(err, store.ErrMediaRequestUserActiveLimit) {
		failWithCode(w, http.StatusTooManyRequests, ErrMediaRequestPendingLimit, "pending media request limit reached")
		return
	}
	if errors.Is(err, store.ErrMediaRequestGlobalActiveLimit) {
		failWithCode(w, http.StatusTooManyRequests, ErrMediaRequestGlobalLimit, fmt.Sprintf("全站求片队列已达上限 %d，请稍后再试", createOpts.GlobalActiveLimit))
		return
	}
	if errors.Is(err, store.ErrConflict) && req.ID != 0 {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestExists, "已有同源同季的活跃求片请求")
		return
	}
	if statusFromError(w, err) {
		return
	}
	a.audit(r, "create_media_request", "user", p.User.UID, map[string]any{
		"request_id": req.ID, "require_key": req.RequireKey, "source": req.Source, "media_id": req.MediaID,
	})
	created(w, "media request submitted", mediaRequestUserDTO(req))
}

func (a *App) handleMyMediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.requireMediaRequestEnabled(w) {
		return
	}
	requests := a.store().ListMediaRequests(current(r).User.UID, false)
	items := make([]map[string]any, 0, len(requests))
	for _, req := range requests {
		items = append(items, mediaRequestUserDTO(req))
	}
	ok(w, "OK", items)
}

func (a *App) handleAdminMediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	statusFilter := strings.ToLower(firstNonEmpty(r.URL.Query().Get("status"), "active"))
	if !validMediaRequestAdminFilter(statusFilter) {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestStatusInvalid, "invalid status filter")
		return
	}
	sourceFilter := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("source"), "all")))
	if sourceFilter == "bgm" {
		sourceFilter = "bangumi"
	}
	if sourceFilter != "all" && sourceFilter != "tmdb" && sourceFilter != "bangumi" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestSourceInvalid, "invalid source filter")
		return
	}
	query := truncateString(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"))), 120)
	page := clamp(queryInt(r, "page", 1), 1, 1000000)
	perPage := clamp(queryInt(r, "per_page", 20), 1, 100)
	result := a.store().ListMediaRequestGroupsPageWithOptions(store.MediaRequestListOptions{
		All: true, StatusFilter: statusFilter, Source: sourceFilter, Query: query,
		Page: page, PerPage: perPage,
	})
	items := make([]map[string]any, 0, len(result.Groups))
	for _, group := range result.Groups {
		item := mediaRequestAdminGroupDTO(group, result.Users)
		if item == nil {
			continue
		}
		items = append(items, item)
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", map[string]any{
		"requests":      items,
		"total":         result.Total,
		"request_total": result.RequestTotal,
		"page":          result.Page,
		"per_page":      result.PerPage,
		"total_pages":   result.TotalPages,
		"has_next":      result.HasNext,
		"status_counts": result.StatusCounts,
	})
}

func mediaRequestAdminGroupDTO(group store.MediaRequestGroup, users map[int64]store.User) map[string]any {
	if len(group.Requests) == 0 {
		return nil
	}
	members := make([]map[string]any, 0, len(group.Requests))
	for _, req := range group.Requests {
		var user *store.User
		if value, exists := users[req.UID]; exists {
			copy := value
			user = &copy
		}
		members = append(members, mediaRequestAdminDTO(req, user))
	}
	// Keep the representative separate from members[0]. Reusing that map and
	// attaching members to it creates a circular JSON value.
	item := cloneMap(members[0])
	item["group_key"] = group.Key
	item["group_count"] = len(members)
	item["grouped_requests"] = members
	return item
}

func validMediaRequestAdminFilter(filter string) bool {
	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "all", "active", "pending", "unhandled", "accepted", "downloading", "rejected", "completed":
		return true
	default:
		return false
	}
}

func mediaRequestExpectedRevision(r *http.Request) (*int64, error) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" {
		return nil, nil
	}
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "W/"))
	raw = strings.Trim(raw, `"`)
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 0 {
		return nil, store.ErrInvalid
	}
	return &revision, nil
}

func writeMediaRequestETag(w http.ResponseWriter, revision int64) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, revision))
}

func mediaRequestUserForDTO(st *store.Store, req store.MediaRequest) *store.User {
	if user, exists := st.User(req.UID); exists {
		return &user
	}
	return nil
}

func failMediaRequestMutation(w http.ResponseWriter, err error) bool {
	if errors.Is(err, store.ErrConflict) {
		failWithCode(w, http.StatusConflict, ErrMediaRequestConflict, "求片已被其他操作更新，请刷新后重试")
		return true
	}
	return statusFromError(w, err)
}

func decodeMediaRequestStatusUpdate(w http.ResponseWriter, r *http.Request) (string, string, *int64, bool) {
	payload := decodeMap(r)
	return decodeMediaRequestStatusUpdatePayload(w, r, payload)
}

func decodeMediaRequestStatusUpdatePayload(w http.ResponseWriter, r *http.Request, payload map[string]any) (string, string, *int64, bool) {
	rawStatus := stringValue(payload, "status")
	if rawStatus == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestStatusInvalid, "status required")
		return "", "", nil, false
	}
	status := store.NormalizeMediaRequestStatus(rawStatus)
	if status == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestStatusInvalid, "invalid status")
		return "", "", nil, false
	}
	expectedRevision, err := mediaRequestExpectedRevision(r)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestRevisionInvalid, "invalid If-Match revision")
		return "", "", nil, false
	}
	note := truncateString(firstNonEmpty(stringValue(payload, "note"), stringValue(payload, "admin_note")), 1000)
	return status, note, expectedRevision, true
}

func (a *App) respondMediaRequestUpdated(w http.ResponseWriter, r *http.Request, req store.MediaRequest) {
	writeMediaRequestETag(w, req.Revision)
	a.audit(r, "update_media_request", "admin", req.UID, map[string]any{
		"request_id": req.ID, "require_key": req.RequireKey, "status": store.MediaRequestAdminStatus(req.Status), "revision": req.Revision,
	})
	ok(w, "状态已更新", mediaRequestAdminDTO(req, mediaRequestUserForDTO(a.store(), req)))
}

func (a *App) handleUpdateMediaRequestStatus(w http.ResponseWriter, r *http.Request, params Params) {
	if current(r).User.Role != store.RoleAdmin {
		failWithCode(w, http.StatusForbidden, ErrMediaAdminRoleRequired, "需要管理员权限")
		return
	}
	id, _ := int64Param(params, "request_id")
	status, note, expectedRevision, valid := decodeMediaRequestStatusUpdate(w, r)
	if !valid {
		return
	}
	req, err := a.store().UpdateMediaRequestStatusIfRevision(id, status, note, false, expectedRevision)
	if failMediaRequestMutation(w, err) {
		return
	}
	a.respondMediaRequestUpdated(w, r, req)
}

func (a *App) handleUpdateMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	status, note, expectedRevision, valid := decodeMediaRequestStatusUpdate(w, r)
	if !valid {
		return
	}
	req, err := a.store().UpdateMediaRequestStatusByKey(params["require_key"], status, note, false, expectedRevision)
	if failMediaRequestMutation(w, err) {
		return
	}
	a.respondMediaRequestUpdated(w, r, req)
}

func (a *App) handleUpdateMediaRequestsByKey(w http.ResponseWriter, r *http.Request, _ Params) {
	payload := decodeMap(r)
	status, note, _, valid := decodeMediaRequestStatusUpdatePayload(w, r, payload)
	if !valid {
		return
	}
	rawItems, itemsValid := payload["items"].([]any)
	if !itemsValid || len(rawItems) == 0 || len(rawItems) > 100 {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "items must contain 1 to 100 requests")
		return
	}
	items := make([]store.MediaRequestBatchItem, 0, len(rawItems))
	for _, raw := range rawItems {
		item, itemValid := raw.(map[string]any)
		if !itemValid {
			failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "invalid batch item")
			return
		}
		key := strings.TrimSpace(stringValue(item, "require_key"))
		if key == "" {
			failWithCode(w, http.StatusBadRequest, ErrMediaRequestPayloadEmpty, "require_key required")
			return
		}
		var revision *int64
		if _, exists := item["revision"]; exists {
			rawRevision := stringValue(item, "revision")
			parsed, err := strconv.ParseInt(rawRevision, 10, 64)
			if err != nil || parsed < 0 {
				failWithCode(w, http.StatusBadRequest, ErrMediaRequestRevisionInvalid, "invalid batch revision")
				return
			}
			revision = &parsed
		}
		items = append(items, store.MediaRequestBatchItem{RequireKey: key, Revision: revision})
	}
	updated, err := a.store().UpdateMediaRequestsStatusByKey(items, status, note, false)
	if failMediaRequestMutation(w, err) {
		return
	}
	result := make([]map[string]any, 0, len(updated))
	keys := make([]string, 0, len(updated))
	for _, req := range updated {
		result = append(result, mediaRequestAdminDTO(req, mediaRequestUserForDTO(a.store(), req)))
		keys = append(keys, req.RequireKey)
	}
	a.audit(r, "batch_update_media_requests", "admin", 0, map[string]any{
		"count": len(updated), "require_keys": keys, "status": store.MediaRequestAdminStatus(status),
	})
	ok(w, "状态已批量更新", map[string]any{"requests": result})
}

func (a *App) handleExternalMediaUpdate(w http.ResponseWriter, r *http.Request, _ Params) {
	secret := firstNonEmpty(r.Header.Get("X-Internal-Secret"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if a.cfg().BotInternalSecret == "" || !constantTimeStringEqual(secret, a.cfg().BotInternalSecret) {
		failWithCode(w, http.StatusForbidden, ErrInternalSecretInvalid, "内部密钥无效")
		return
	}
	payload := decodeMap(r)
	key := firstNonEmpty(stringValue(payload, "key"), stringValue(payload, "require_key"))
	req, okReq := a.store().FindMediaRequestByKey(key)
	if !okReq {
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
		return
	}
	rawStatus := stringValue(payload, "status")
	if rawStatus == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestStatusInvalid, "status required")
		return
	}
	status := store.NormalizeMediaRequestStatus(rawStatus)
	if status == "" {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestStatusInvalid, "invalid status")
		return
	}
	req, err := a.store().UpdateMediaRequestStatus(req.ID, status, truncateString(stringValue(payload, "note"), 1000), true)
	if statusFromError(w, err) {
		return
	}
	a.auditEntryIP(a.clientIP(r), 0, "external-media", "external_update_media_request", "system", req.UID, map[string]any{
		"request_id": req.ID, "require_key": req.RequireKey, "status": store.MediaRequestAdminStatus(req.Status), "revision": req.Revision,
	})
	writeMediaRequestETag(w, req.Revision)
	ok(w, "状态已更新", mediaRequestAdminDTO(req, mediaRequestUserForDTO(a.store(), req)))
}

func (a *App) handleMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	req, okReq := a.store().FindMediaRequestByKey(params["require_key"])
	if !okReq {
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
		return
	}
	if !canAccessMediaRequest(current(r).User, req) {
		failWithCode(w, http.StatusForbidden, ErrMediaRequestAccessDenied, "cannot access this request")
		return
	}
	writeMediaRequestETag(w, req.Revision)
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", mediaRequestUserDTO(req))
}

func (a *App) handleDeleteMediaRequestByKey(w http.ResponseWriter, r *http.Request, params Params) {
	req, okReq := a.store().FindMediaRequestByKey(params["require_key"])
	if !okReq {
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
		return
	}
	if !canAccessMediaRequest(current(r).User, req) {
		failWithCode(w, http.StatusForbidden, ErrMediaRequestDeleteDenied, "cannot delete this request")
		return
	}
	expectedRevision, err := mediaRequestExpectedRevision(r)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestRevisionInvalid, "invalid If-Match revision")
		return
	}
	deleted, err := a.store().DeleteMediaRequestByKey(params["require_key"], expectedRevision)
	if failMediaRequestMutation(w, err) {
		return
	}
	a.audit(r, "delete_media_request", auditCategoryForRole(current(r).User.Role), deleted.UID, map[string]any{
		"request_id": deleted.ID, "require_key": deleted.RequireKey, "source": deleted.Source, "media_id": deleted.MediaID,
	})
	ok(w, "request deleted", nil)
}

func (a *App) handleMediaRequestByID(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "request_id")
	req, okReq := a.store().MediaRequest(id)
	if okReq {
		if !canAccessMediaRequest(current(r).User, req) {
			// Return the same 404 as a missing row to avoid request-id enumeration.
			failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
			return
		}
		writeMediaRequestETag(w, req.Revision)
		w.Header().Set("Cache-Control", "private, no-store")
		ok(w, "OK", mediaRequestUserDTO(req))
		return
	}
	failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
}

func (a *App) handleDeleteMediaRequest(w http.ResponseWriter, r *http.Request, params Params) {
	id, _ := int64Param(params, "request_id")
	request, okReq := a.store().MediaRequest(id)
	if !okReq {
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
		return
	} else if !canAccessMediaRequest(current(r).User, request) {
		// Match GET by id: existing-but-forbidden rows are hidden as 404.
		failWithCode(w, http.StatusNotFound, ErrMediaRequestNotFound, "request not found")
		return
	}
	expectedRevision, err := mediaRequestExpectedRevision(r)
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrMediaRequestRevisionInvalid, "invalid If-Match revision")
		return
	}
	deleted, err := a.store().DeleteMediaRequestIfRevision(id, expectedRevision)
	if failMediaRequestMutation(w, err) {
		return
	}
	a.audit(r, "delete_media_request", auditCategoryForRole(current(r).User.Role), deleted.UID, map[string]any{
		"request_id": deleted.ID, "require_key": deleted.RequireKey, "source": deleted.Source, "media_id": deleted.MediaID,
	})
	ok(w, "request deleted", nil)
}
