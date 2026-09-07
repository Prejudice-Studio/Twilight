package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/prejudice-studio/twilight/internal/store"
)

type v2AdminUserPagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type v2AdminUserListResponse struct {
	Items      []map[string]any      `json:"items"`
	Pagination v2AdminUserPagination `json:"pagination"`
}

type adminUserListQuery struct {
	page    int
	perPage int
	sortKey string
	filter  adminUserListFilter
}

// parseAdminUserListQuery is the single query boundary for the V1 compatibility
// list and the V2 resource. The two frontends must not silently develop
// different filter or pagination semantics.
func parseAdminUserListQuery(r *http.Request) adminUserListQuery {
	query := r.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	if len(search) > 100 {
		search = truncateString(search, 100)
	}
	return adminUserListQuery{
		page:    max(1, queryInt(r, "page", 1)),
		perPage: clamp(queryInt(r, "per_page", 20), 1, 100),
		sortKey: strings.TrimSpace(query.Get("sort")),
		filter: adminUserListFilter{
			roleFilter:        query.Get("role"),
			hasRole:           query.Get("role") != "",
			activeFilter:      query.Get("active"),
			hasActive:         query.Get("active") != "",
			strictQueryActive: true,
			embyFilter:        strings.ToLower(strings.TrimSpace(query.Get("emby"))),
			embyStatusFilter:  strings.ToLower(strings.TrimSpace(query.Get("emby_status"))),
			emailFilter:       strings.ToLower(strings.TrimSpace(query.Get("email_status"))),
			search:            strings.ToLower(search),
			now:               time.Now().Unix(),
		},
	}
}

func (a *App) adminUserListResource(r *http.Request) v2AdminUserListResponse {
	query := parseAdminUserListQuery(r)
	matched := a.store().UsersMatching(0, func(u store.User) bool {
		return adminUserMatchesListFilters(u, query.filter)
	})
	sortUsers(matched, query.sortKey)
	total := len(matched)
	pageUsers := paginate(matched, query.page, query.perPage)
	items := make([]map[string]any, 0, len(pageUsers))
	for _, user := range pageUsers {
		items = append(items, publicUserAt(user, query.filter.now))
	}
	return v2AdminUserListResponse{
		Items: items,
		Pagination: v2AdminUserPagination{
			Page:       query.page,
			PerPage:    query.perPage,
			Total:      total,
			TotalPages: pages(total, query.perPage),
		},
	}
}

func (a *App) handleV2AdminUsers(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", a.adminUserListResource(r))
}

func (a *App) handleV2AdminUser(w http.ResponseWriter, r *http.Request, params Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	user, found := a.userFromPath(w, params, "uid")
	if !found {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", map[string]any{"item": publicUser(user)})
}

// V2 mutations deliberately reuse the existing handlers. This keeps the
// Store atomicity, permission checks, audit records and external side effects
// single-sourced while the SSR frontend moves to the resource namespace.
func (a *App) handleV2AdminCreateUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminCreateUser(w, r, p)
}

func (a *App) handleV2AdminUpdateUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminUpdateUser(w, r, p)
}

func (a *App) handleV2AdminDeleteUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminDeleteUser(w, r, p)
}

func (a *App) handleV2AdminToggleUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminToggleUser(w, r, p)
}

func (a *App) handleV2AdminToggleEmby(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminToggleEmby(w, r, p)
}

func (a *App) handleV2AdminUnbindEmby(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminUnbindEmby(w, r, p)
}

func (a *App) handleV2AdminForceUnbind(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminForceUnbind(w, r, p)
}

func (a *App) handleV2AdminRefreshUserStatus(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminRefreshUserStatus(w, r, p)
}

func (a *App) handleV2AdminRenewUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminRenewUser(w, r, p)
}

func (a *App) handleV2AdminSetUserExpiry(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminSetUserExpiry(w, r, p)
}

func (a *App) handleV2AdminResetPassword(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminResetPassword(w, r, p)
}

func (a *App) handleV2AdminKickUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleKickUser(w, r, p)
}

func (a *App) handleV2AdminSetRole(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminSetRole(w, r, p)
}

func (a *App) handleV2AdminUnbindTelegram(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminUnbindTelegram(w, r, p)
}

func (a *App) handleV2AdminBindTelegram(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminBindTelegram(w, r, p)
}

func (a *App) handleV2AdminBindEmby(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminBindEmby(w, r, p)
}

func (a *App) handleV2AdminRegistrationQueueClear(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRegistrationQueueClear(w, r, p)
}

func (a *App) handleV2AdminRegistrationEntitlement(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleRegistrationEntitlement(w, r, p)
}
