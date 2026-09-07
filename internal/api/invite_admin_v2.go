package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prejudice-studio/twilight/internal/store"
)

type v2AdminInviteTreeResponse struct {
	Item map[string]any `json:"item"`
}

type v2AdminInvitePagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type v2AdminInviteCodesResponse struct {
	Items      []map[string]any        `json:"items"`
	Pagination v2AdminInvitePagination `json:"pagination"`
}

var inviteAdminConfigKeys = map[string]bool{
	"invite_enabled":               true,
	"invite_limit":                 true,
	"invite_root_user_limit":       true,
	"invite_max_depth":             true,
	"invite_require_emby":          true,
	"invite_code_default_days":     true,
	"permanent_invite_max_days":    true,
	"invite_code_format":           true,
	"invite_code_random_algorithm": true,
}

// handleV2AdminInviteTree resolves search, collapse, root selection, and
// pagination at the API boundary. This keeps thousands of invite nodes out of
// the Svelte page payload while preserving the existing relationship rules.
func (a *App) handleV2AdminInviteTree(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ok(w, "OK", v2AdminInviteTreeResponse{Item: a.v2AdminInviteTree(r)})
}

func (a *App) v2AdminInviteTree(r *http.Request) map[string]any {
	relations, users := a.store().InviteForestSnapshot(a.cfg().InviteEnabled)
	nodes := make(map[int64]store.User, len(users))
	for uid, user := range users {
		nodes[uid] = user
	}
	children := make(map[int64][]int64, len(relations))
	parent := make(map[int64]int64, len(relations))
	for _, relation := range relations {
		if _, parentFound := nodes[relation.ParentUID]; !parentFound {
			continue
		}
		if _, childFound := nodes[relation.ChildUID]; !childFound {
			continue
		}
		children[relation.ParentUID] = append(children[relation.ParentUID], relation.ChildUID)
		if _, exists := parent[relation.ChildUID]; !exists {
			parent[relation.ChildUID] = relation.ParentUID
		}
	}
	for _, childUIDs := range children {
		sort.Slice(childUIDs, func(i, j int) bool { return childUIDs[i] < childUIDs[j] })
	}

	roots := make([]int64, 0, len(nodes))
	for uid := range nodes {
		if parent[uid] == 0 {
			roots = append(roots, uid)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })

	rootOf := make(map[int64]int64, len(nodes))
	depthOf := make(map[int64]int, len(nodes))
	descendants := make(map[int64]int, len(nodes))
	maxDepth := 0
	for _, root := range roots {
		if _, seen := rootOf[root]; seen {
			continue
		}
		order := make([]int64, 0)
		stack := []v2InviteTreeVisit{{uid: root, depth: 0}}
		rootOf[root] = root
		depthOf[root] = 0
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			order = append(order, current.uid)
			if current.depth > maxDepth {
				maxDepth = current.depth
			}
			childUIDs := children[current.uid]
			for index := len(childUIDs) - 1; index >= 0; index-- {
				childUID := childUIDs[index]
				if _, seen := rootOf[childUID]; seen {
					continue
				}
				rootOf[childUID] = root
				depthOf[childUID] = current.depth + 1
				stack = append(stack, v2InviteTreeVisit{uid: childUID, depth: current.depth + 1})
			}
		}
		for index := len(order) - 1; index >= 0; index-- {
			uid := order[index]
			count := 0
			for _, childUID := range children[uid] {
				count += 1 + descendants[childUID]
			}
			descendants[uid] = count
		}
	}

	search := strings.ToLower(strings.TrimSpace(truncateString(r.URL.Query().Get("search"), 120)))
	included := make(map[int64]bool, len(nodes))
	if search != "" {
		for uid, user := range nodes {
			telegramID := ""
			if user.TelegramID != 0 {
				telegramID = strconv.FormatInt(user.TelegramID, 10)
			}
			if !strings.Contains(strings.ToLower(user.Username+" "+strconv.FormatInt(uid, 10)+" "+telegramID), search) {
				continue
			}
			for current := uid; current != 0 && !included[current]; current = parent[current] {
				included[current] = true
			}
		}
	}

	selectedRoot := int64(0)
	if rawRoot := strings.TrimSpace(r.URL.Query().Get("root")); rawRoot != "" && rawRoot != "all" {
		if parsed, err := strconv.ParseInt(rawRoot, 10, 64); err == nil && parsed > 0 {
			for _, root := range roots {
				if root == parsed {
					selectedRoot = root
					break
				}
			}
		}
	}
	visibleRoots := roots
	if selectedRoot > 0 {
		visibleRoots = []int64{selectedRoot}
	}
	collapsed := v2InviteCollapsedUIDs(r.URL.Query().Get("collapsed"))
	rows := make([]map[string]any, 0, len(nodes))
	for _, root := range visibleRoots {
		stack := []v2InviteTreeVisit{{uid: root, depth: 0}}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			user, found := nodes[current.uid]
			if !found {
				continue
			}
			if search == "" || included[current.uid] {
				rows = append(rows, v2InviteTreeRow(user, current.uid, current.depth, rootOf[current.uid], len(children[current.uid]), descendants[current.uid], collapsed[current.uid], parent[current.uid] == 0))
			}
			if collapsed[current.uid] && search == "" {
				continue
			}
			childUIDs := children[current.uid]
			for index := len(childUIDs) - 1; index >= 0; index-- {
				stack = append(stack, v2InviteTreeVisit{uid: childUIDs[index], depth: current.depth + 1})
			}
		}
	}

	page := clamp(queryInt(r, "page", 1), 1, 1_000_000)
	perPage := queryInt(r, "per_page", 300)
	if perPage != 100 && perPage != 300 && perPage != 500 {
		perPage = 300
	}
	total := len(rows)
	totalPages := max(1, pages(total, perPage))
	page = min(page, totalPages)
	pageRows := paginate(rows, page, perPage)
	selectedUID := int64(queryInt(r, "selected", 0))
	selected := any(nil)
	if user, found := nodes[selectedUID]; found {
		selected = v2InviteTreeRow(user, selectedUID, depthOf[selectedUID], rootOf[selectedUID], len(children[selectedUID]), descendants[selectedUID], collapsed[selectedUID], parent[selectedUID] == 0)
	}
	rootItems := make([]map[string]any, 0, len(roots))
	for _, root := range roots {
		rootItems = append(rootItems, map[string]any{"uid": root, "username": nodes[root].Username})
	}
	return map[string]any{
		"rows": pageRows, "selected": selected, "roots": rootItems,
		"total_rows": total, "total_nodes": len(nodes), "total_relations": len(relations),
		"max_depth": maxDepth, "page": page, "per_page": perPage, "pages": totalPages,
		"config": a.inviteConfigPayload(),
	}
}

type v2InviteTreeVisit struct {
	uid   int64
	depth int
}

func v2InviteCollapsedUIDs(raw string) map[int64]bool {
	collapsed := make(map[int64]bool)
	for _, value := range strings.Split(truncateString(raw, 4_800), ",") {
		if len(collapsed) >= 200 {
			break
		}
		uid, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil && uid > 0 {
			collapsed[uid] = true
		}
	}
	return collapsed
}

func v2InviteTreeRow(user store.User, uid int64, depth int, rootUID int64, directChildren, descendantCount int, collapsed, isRoot bool) map[string]any {
	return map[string]any{
		"uid": uid, "username": user.Username, "role": user.Role,
		"emby_bound": user.EmbyID != "", "emby_disabled": user.EmbyDisabled,
		"active": user.Active, "telegram_id": nullableInt(user.TelegramID),
		"register_time": user.RegisterTime, "expired_at": user.ExpiredAt,
		"is_root": isRoot, "depth": depth, "root_uid": rootUID,
		"direct_children": directChildren, "descendants": descendantCount, "collapsed": collapsed,
	}
}

func (a *App) handleV2AdminInviteCodes(w http.ResponseWriter, r *http.Request, _ Params) {
	if a.refreshStoreForRequest(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	page := max(1, queryInt(r, "page", 1))
	perPage := clamp(queryInt(r, "per_page", 50), 1, 100)
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("search")))

	// Resolve inviter names once for the filtering pass. DTO enrichment is still
	// deferred until after pagination, so a large invite-code inventory does not
	// repeatedly scan users or retain a second browser-side full list.
	users := a.store().ListUsers()
	usernames := make(map[int64]string, len(users))
	for _, user := range users {
		usernames[user.UID] = user.Username
	}
	filtered := make([]store.InviteCode, 0)
	for _, code := range a.store().ListAllInviteCodes() {
		if search != "" {
			candidate := strings.ToLower(strings.Join([]string{
				code.Code,
				usernames[code.InviterUID],
				fmt.Sprintf("%d", code.InviterUID),
				code.TargetUsername,
				code.Note,
			}, " "))
			if !strings.Contains(candidate, search) {
				continue
			}
		}
		filtered = append(filtered, code)
	}
	total := len(filtered)
	totalPages := max(1, pages(total, perPage))
	page = min(page, totalPages)
	pageCodes := paginate(filtered, page, perPage)
	items := make([]map[string]any, 0, len(pageCodes))
	for _, code := range pageCodes {
		items = append(items, a.inviteCodeDTO(code))
	}
	ok(w, "OK", v2AdminInviteCodesResponse{
		Items: items,
		Pagination: v2AdminInvitePagination{
			Page: page, PerPage: perPage, Total: total, TotalPages: totalPages,
		},
	})
}

// The V2 mutations deliberately reuse the audited V1 application handlers.
// This keeps invitation relationship rollback, Emby cleanup, protected-admin
// checks, confirmation phrases, and scheduler-visible audit behavior single-sourced.
func (a *App) handleV2AdminInviteDetach(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleInviteDetach(w, r, p)
}

func (a *App) handleV2AdminInviteDetachDeleteEmby(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminInviteDetachDeleteEmby(w, r, p)
}

func (a *App) handleV2AdminInviteDetachBatch(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminInviteDetachBatch(w, r, p)
}

func (a *App) handleV2AdminInviteQuickMaintenance(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminInviteQuickMaintenance(w, r, p)
}

func (a *App) handleV2AdminInviteToggleUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminToggleUser(w, r, p)
}

func (a *App) handleV2AdminInviteDeleteUser(w http.ResponseWriter, r *http.Request, p Params) {
	a.handleAdminDeleteUser(w, r, p)
}

func (a *App) handleV2AdminInviteConfigSchema(w http.ResponseWriter, r *http.Request, p Params) {
	if r.Method == http.MethodGet {
		w.Header().Set("Cache-Control", "private, no-store")
		values := configValues(*a.cfg())
		for _, section := range configSectionDefs() {
			if section.Key != "SAR" {
				continue
			}
			fields := make([]map[string]any, 0, len(section.Fields))
			for _, field := range section.Fields {
				if !inviteAdminConfigKeys[field.Key] {
					continue
				}
				item := map[string]any{
					"key": field.Key, "label": field.Label, "type": field.Type,
					"description": field.Description, "value": values[section.Key][field.Key],
				}
				if len(field.Options) > 0 {
					item["options"] = field.Options
				}
				if len(field.PlaceholderHints) > 0 {
					item["placeholder_hints"] = field.PlaceholderHints
				}
				fields = append(fields, item)
			}
			ok(w, "OK", map[string]any{"categories": []map[string]string{{"key": "policy", "title": "策略"}}, "sections": []map[string]any{{
				"key": section.Key, "title": section.Title, "description": section.Description,
				"category": section.Category, "collapsed": section.Collapsed, "fields": fields,
			}}})
			return
		}
		failWithCode(w, http.StatusNotFound, ErrBadRequest, "邀请配置不可用")
		return
	}

	payload := decodeMap(r)
	rawSections, _ := payload["sections"].(map[string]any)
	filteredFields, _ := rawSections["SAR"].(map[string]any)
	allowedFields := make(map[string]any, len(filteredFields))
	for key, value := range filteredFields {
		if inviteAdminConfigKeys[key] {
			allowedFields[key] = value
		}
	}
	filteredPayload, err := json.Marshal(map[string]any{"sections": map[string]any{"SAR": allowedFields}})
	if err != nil {
		failWithCode(w, http.StatusBadRequest, ErrBadRequest, "邀请配置格式无效")
		return
	}
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(strings.NewReader(string(filteredPayload)))
	clone.ContentLength = int64(len(filteredPayload))
	a.handleConfigSchemaUpdateSafe(w, clone, p)
}
