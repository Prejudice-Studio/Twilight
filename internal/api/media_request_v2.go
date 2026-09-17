package api

import (
	"net/http"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

type v2MediaRequestPagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type v2AdminMediaRequestListResponse struct {
	Items        []map[string]any         `json:"items"`
	Pagination   v2MediaRequestPagination `json:"pagination"`
	RequestTotal int                      `json:"request_total"`
	HasNext      bool                     `json:"has_next"`
	StatusCounts map[string]int           `json:"status_counts"`
}

type adminMediaRequestQuery struct {
	statusFilter string
	sourceFilter string
	query        string
	page         int
	perPage      int
}

// parseAdminMediaRequestQuery is shared by V1 compatibility and V2 resources.
// Keeping validation here makes status/source semantics identical for both
// frontends and prevents a malformed URL from reaching Store filtering.
func parseAdminMediaRequestQuery(r *http.Request) (adminMediaRequestQuery, ErrCode, string) {
	statusFilter := strings.ToLower(firstNonEmpty(r.URL.Query().Get("status"), "active"))
	if !validMediaRequestAdminFilter(statusFilter) {
		return adminMediaRequestQuery{}, ErrMediaRequestStatusInvalid, "invalid status filter"
	}
	sourceFilter := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("source"), "all")))
	if sourceFilter == "bgm" {
		sourceFilter = "bangumi"
	}
	if sourceFilter != "all" && sourceFilter != "tmdb" && sourceFilter != "bangumi" {
		return adminMediaRequestQuery{}, ErrMediaRequestSourceInvalid, "invalid source filter"
	}
	return adminMediaRequestQuery{
		statusFilter: statusFilter,
		sourceFilter: sourceFilter,
		query:        truncateString(strings.TrimSpace(firstNonEmpty(r.URL.Query().Get("q"), r.URL.Query().Get("query"))), 120),
		page:         clamp(queryInt(r, "page", 1), 1, 1_000_000),
		perPage:      clamp(queryInt(r, "per_page", 20), 1, 100),
	}, "", ""
}

func (a *App) adminMediaRequestListResource(r *http.Request) (v2AdminMediaRequestListResponse, ErrCode, string) {
	query, code, message := parseAdminMediaRequestQuery(r)
	if code != "" {
		return v2AdminMediaRequestListResponse{}, code, message
	}
	result := a.store().ListMediaRequestGroupsPageWithOptions(store.MediaRequestListOptions{
		All: true, StatusFilter: query.statusFilter, Source: query.sourceFilter,
		Query: query.query, Page: query.page, PerPage: query.perPage,
	})
	items := make([]map[string]any, 0, len(result.Groups))
	for _, group := range result.Groups {
		if item := mediaRequestAdminGroupDTO(group, result.Users); item != nil {
			items = append(items, item)
		}
	}
	return v2AdminMediaRequestListResponse{
		Items:        items,
		Pagination:   v2MediaRequestPagination{Page: result.Page, PerPage: result.PerPage, Total: result.Total, TotalPages: result.TotalPages},
		RequestTotal: result.RequestTotal,
		HasNext:      result.HasNext,
		StatusCounts: result.StatusCounts,
	}, "", ""
}

func (a *App) handleV2AdminMediaRequests(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	resource, code, message := a.adminMediaRequestListResource(r)
	if code != "" {
		failWithCode(w, http.StatusBadRequest, code, message)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", resource)
}

// Mutations reuse the audited V1 handlers so Store revision checks, audit
// entries, and error mapping remain single-sourced during the migration.
func (a *App) handleV2UpdateMediaRequestStatus(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleUpdateMediaRequestStatus(w, r, p)
}

func (a *App) handleV2UpdateMediaRequestByKey(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleUpdateMediaRequestByKey(w, r, p)
}

func (a *App) handleV2UpdateMediaRequestsByKey(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleUpdateMediaRequestsByKey(w, r, p)
}

func (a *App) handleV2DeleteMediaRequest(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleDeleteMediaRequest(w, r, p)
}

func (a *App) handleV2DeleteMediaRequestByKey(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleDeleteMediaRequestByKey(w, r, p)
}
