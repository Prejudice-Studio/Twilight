package store

import (
	"container/heap"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MediaRequestStatusUnhandled   = "UNHANDLED"
	MediaRequestStatusAccepted    = "ACCEPTED"
	MediaRequestStatusRejected    = "REJECTED"
	MediaRequestStatusCompleted   = "COMPLETED"
	MediaRequestStatusDownloading = "DOWNLOADING"
)

type MediaRequestCreateOptions struct {
	UserActiveLimit   int
	GlobalActiveLimit int
}

type MediaRequestPage struct {
	Requests     []MediaRequest
	Users        map[int64]User
	StatusCounts map[string]int
	Total        int
	Page         int
	PerPage      int
	TotalPages   int
	HasNext      bool
}

// MediaRequestGroupPage is the administrator-facing view grouped by a
// normalized title. The underlying requests remain independent records so a
// group can be expanded and handled one member at a time.
type MediaRequestGroupPage struct {
	Groups       []MediaRequestGroup
	Users        map[int64]User
	StatusCounts map[string]int
	Total        int
	RequestTotal int
	Page         int
	PerPage      int
	TotalPages   int
	HasNext      bool
}

type MediaRequestGroup struct {
	Key      string
	Requests []MediaRequest
}

type MediaRequestBatchItem struct {
	RequireKey string
	Revision   *int64
}

type MediaRequestListOptions struct {
	UID          int64
	All          bool
	StatusFilter string
	Source       string
	Query        string
	Page         int
	PerPage      int
}

type mediaRequestIDMinHeap []MediaRequest

func (h mediaRequestIDMinHeap) Len() int           { return len(h) }
func (h mediaRequestIDMinHeap) Less(i, j int) bool { return h[i].ID < h[j].ID }
func (h mediaRequestIDMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *mediaRequestIDMinHeap) Push(x any) {
	*h = append(*h, x.(MediaRequest))
}

func (h *mediaRequestIDMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func NormalizeMediaRequestStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "unhandled", "pending_review":
		return MediaRequestStatusUnhandled
	case "accepted", "approved":
		return MediaRequestStatusAccepted
	case "rejected", "reject":
		return MediaRequestStatusRejected
	case "completed", "complete", "done":
		return MediaRequestStatusCompleted
	case "downloading", "download":
		return MediaRequestStatusDownloading
	default:
		return ""
	}
}

func MediaRequestAdminStatus(status string) string {
	switch NormalizeMediaRequestStatus(status) {
	case MediaRequestStatusUnhandled:
		return "pending"
	case MediaRequestStatusAccepted:
		return "accepted"
	case MediaRequestStatusRejected:
		return "rejected"
	case MediaRequestStatusCompleted:
		return "completed"
	case MediaRequestStatusDownloading:
		return "downloading"
	default:
		return "pending"
	}
}

func MediaRequestStatusMatches(status, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" || filter == "all" {
		return true
	}
	if filter == "active" {
		switch NormalizeMediaRequestStatus(status) {
		case MediaRequestStatusUnhandled, MediaRequestStatusAccepted, MediaRequestStatusDownloading:
			return true
		default:
			return false
		}
	}
	if filter == "pending" || filter == "unhandled" {
		return NormalizeMediaRequestStatus(status) == MediaRequestStatusUnhandled
	}
	return MediaRequestAdminStatus(status) == filter
}

func MediaRequestStatusText(status string) string {
	switch NormalizeMediaRequestStatus(status) {
	case MediaRequestStatusUnhandled:
		return "待处理"
	case MediaRequestStatusAccepted:
		return "已接受"
	case MediaRequestStatusRejected:
		return "已拒绝"
	case MediaRequestStatusCompleted:
		return "已完成"
	case MediaRequestStatusDownloading:
		return "正在下载"
	default:
		return "未知"
	}
}

func IsActiveMediaRequestStatus(status string) bool {
	switch NormalizeMediaRequestStatus(status) {
	case MediaRequestStatusUnhandled, MediaRequestStatusAccepted, MediaRequestStatusDownloading:
		return true
	default:
		return false
	}
}

func isActiveMediaStatus(status string) bool {
	return IsActiveMediaRequestStatus(status)
}

func (s *Store) ListMediaRequestsPage(uid int64, all bool, statusFilter string, page, perPage int) MediaRequestPage {
	return s.ListMediaRequestsPageWithOptions(MediaRequestListOptions{
		UID: uid, All: all, StatusFilter: statusFilter, Page: page, PerPage: perPage,
	})
}

func (s *Store) ListMediaRequestsPageWithOptions(opts MediaRequestListOptions) MediaRequestPage {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	window := offset + perPage
	if window < perPage {
		return MediaRequestPage{Page: page, PerPage: perPage}
	}
	statusFilter := strings.ToLower(strings.TrimSpace(opts.StatusFilter))
	sourceFilter := strings.ToLower(strings.TrimSpace(opts.Source))
	if sourceFilter == "bgm" {
		sourceFilter = "bangumi"
	}
	query := strings.ToLower(strings.TrimSpace(opts.Query))

	s.mu.RLock()
	defer s.mu.RUnlock()
	var top mediaRequestIDMinHeap
	counts := map[string]int{
		"all": 0, "active": 0, "pending": 0, "accepted": 0,
		"downloading": 0, "rejected": 0, "completed": 0,
	}
	total := 0
	heapReady := false
	for _, r := range s.state.MediaRequests {
		if !opts.All && r.UID != opts.UID {
			continue
		}
		if sourceFilter != "" && sourceFilter != "all" {
			requestSource := strings.ToLower(strings.TrimSpace(r.Source))
			if requestSource == "bgm" {
				requestSource = "bangumi"
			}
			if requestSource != sourceFilter {
				continue
			}
		}
		if query != "" && !mediaRequestMatchesQuery(r, query) {
			continue
		}
		adminStatus := MediaRequestAdminStatus(r.Status)
		counts["all"]++
		counts[adminStatus]++
		if IsActiveMediaRequestStatus(r.Status) {
			counts["active"]++
		}
		if !MediaRequestStatusMatches(r.Status, statusFilter) {
			continue
		}
		total++
		if len(top) < window {
			top = append(top, r)
			if len(top) == window {
				heap.Init(&top)
				heapReady = true
			}
			continue
		}
		if !heapReady {
			heap.Init(&top)
			heapReady = true
		}
		if len(top) > 0 && r.ID > top[0].ID {
			top[0] = r
			heap.Fix(&top, 0)
		}
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	if total == 0 || offset >= len(top) {
		return MediaRequestPage{
			StatusCounts: counts, Total: total, Page: page, PerPage: perPage,
			TotalPages: totalPages, HasNext: page < totalPages,
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].ID > top[j].ID })
	end := offset + perPage
	if end > len(top) {
		end = len(top)
	}
	requests := make([]MediaRequest, end-offset)
	copy(requests, top[offset:end])
	users := make(map[int64]User, len(requests))
	for _, request := range requests {
		if user, ok := s.state.Users[request.UID]; ok {
			users[request.UID] = user
		}
	}
	return MediaRequestPage{
		Requests: requests, Users: users, StatusCounts: counts, Total: total,
		Page: page, PerPage: perPage, TotalPages: totalPages, HasNext: page < totalPages,
	}
}

// ListMediaRequestGroupsPageWithOptions groups matching titles before
// pagination. This keeps a same-title group together even when the raw
// requests would otherwise straddle two pages.
func (s *Store) ListMediaRequestGroupsPageWithOptions(opts MediaRequestListOptions) MediaRequestGroupPage {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage < 1 {
		perPage = 20
	}
	statusFilter := strings.ToLower(strings.TrimSpace(opts.StatusFilter))
	sourceFilter := strings.ToLower(strings.TrimSpace(opts.Source))
	if sourceFilter == "bgm" {
		sourceFilter = "bangumi"
	}
	query := strings.ToLower(strings.TrimSpace(opts.Query))

	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{
		"all": 0, "active": 0, "pending": 0, "accepted": 0,
		"downloading": 0, "rejected": 0, "completed": 0,
	}
	groupMap := make(map[string][]MediaRequest)
	requestTotal := 0
	for _, r := range s.state.MediaRequests {
		if !opts.All && r.UID != opts.UID {
			continue
		}
		requestSource := strings.ToLower(strings.TrimSpace(r.Source))
		if requestSource == "bgm" {
			requestSource = "bangumi"
		}
		if sourceFilter != "" && sourceFilter != "all" && requestSource != sourceFilter {
			continue
		}
		if query != "" && !mediaRequestMatchesQuery(r, query) {
			continue
		}
		adminStatus := MediaRequestAdminStatus(r.Status)
		counts["all"]++
		counts[adminStatus]++
		if IsActiveMediaRequestStatus(r.Status) {
			counts["active"]++
		}
		if !MediaRequestStatusMatches(r.Status, statusFilter) {
			continue
		}
		requestTotal++
		key := MediaRequestGroupKey(r)
		groupMap[key] = append(groupMap[key], r)
	}

	groups := make([]MediaRequestGroup, 0, len(groupMap))
	for key, requests := range groupMap {
		sort.Slice(requests, func(i, j int) bool { return requests[i].ID > requests[j].ID })
		groups = append(groups, MediaRequestGroup{Key: key, Requests: requests})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Requests[0].ID > groups[j].Requests[0].ID
	})
	total := len(groups)
	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	offset := (page - 1) * perPage
	if offset >= total {
		return MediaRequestGroupPage{
			StatusCounts: counts, Total: total, RequestTotal: requestTotal, Page: page, PerPage: perPage,
			TotalPages: totalPages, HasNext: page < totalPages,
		}
	}
	end := offset + perPage
	if end > total {
		end = total
	}
	selected := groups[offset:end]
	users := make(map[int64]User)
	for _, group := range selected {
		for _, request := range group.Requests {
			if user, ok := s.state.Users[request.UID]; ok {
				users[request.UID] = user
			}
		}
	}
	return MediaRequestGroupPage{
		Groups: selected, Users: users, StatusCounts: counts, Total: total, RequestTotal: requestTotal,
		Page: page, PerPage: perPage, TotalPages: totalPages, HasNext: page < totalPages,
	}
}

// MediaRequestGroupKey normalizes only the display title. Source and media ID
// are intentionally excluded: a same-named TMDB and Bangumi request is one
// administrator work item, while both source records remain independently
// addressable by require_key.
func MediaRequestGroupKey(r MediaRequest) string {
	title := strings.TrimSpace(r.Title)
	if title == "" && r.MediaInfo != nil {
		if value, ok := r.MediaInfo["title"].(string); ok {
			title = strings.TrimSpace(value)
		}
	}
	if title == "" {
		return "request:" + r.RequireKey
	}
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

func mediaRequestMatchesQuery(r MediaRequest, query string) bool {
	fields := [...]string{
		r.Title,
		r.OriginalTitle,
		r.Username,
		r.RequireKey,
		r.Source,
		strconv.FormatInt(r.ID, 10),
		strconv.FormatInt(r.MediaID, 10),
		strconv.FormatInt(r.UID, 10),
		strconv.FormatInt(r.TelegramID, 10),
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func (s *Store) UpdateMediaRequestStatus(id int64, rawStatus string, adminNote string, replaceNote bool) (MediaRequest, error) {
	return s.UpdateMediaRequestStatusIfRevision(id, rawStatus, adminNote, replaceNote, nil)
}

func (s *Store) UpdateMediaRequestStatusIfRevision(id int64, rawStatus string, adminNote string, replaceNote bool, expectedRevision *int64) (MediaRequest, error) {
	status := NormalizeMediaRequestStatus(rawStatus)
	if status == "" {
		return MediaRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated MediaRequest
	err := s.mutateAndSaveLocked(func() error {
		r, ok := s.state.MediaRequests[id]
		if !ok {
			return ErrNotFound
		}
		if expectedRevision != nil && r.Revision != *expectedRevision {
			return ErrConflict
		}
		applyMediaRequestStatusUpdate(&r, status, adminNote, replaceNote)
		s.state.MediaRequests[id] = r
		updated = r
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return updated, nil
}

func (s *Store) UpdateMediaRequestStatusByKey(key, rawStatus, adminNote string, replaceNote bool, expectedRevision *int64) (MediaRequest, error) {
	status := NormalizeMediaRequestStatus(rawStatus)
	if status == "" || strings.TrimSpace(key) == "" {
		return MediaRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated MediaRequest
	err := s.mutateAndSaveLocked(func() error {
		id, current, ok := s.mediaRequestByKeyLocked(key)
		if !ok {
			return ErrNotFound
		}
		if expectedRevision != nil && current.Revision != *expectedRevision {
			return ErrConflict
		}
		applyMediaRequestStatusUpdate(&current, status, adminNote, replaceNote)
		s.state.MediaRequests[id] = current
		updated = current
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return updated, nil
}

// UpdateMediaRequestsStatusByKey applies one status transition to every
// member atomically. All keys and revisions are checked before any member is
// changed or the state document is persisted.
func (s *Store) UpdateMediaRequestsStatusByKey(items []MediaRequestBatchItem, rawStatus, adminNote string, replaceNote bool) ([]MediaRequest, error) {
	status := NormalizeMediaRequestStatus(rawStatus)
	if status == "" || len(items) == 0 {
		return nil, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := make([]MediaRequest, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	err := s.mutateAndSaveLocked(func() error {
		for _, item := range items {
			key := strings.TrimSpace(item.RequireKey)
			if key == "" {
				return ErrInvalid
			}
			if _, exists := seen[key]; exists {
				return ErrInvalid
			}
			seen[key] = struct{}{}
		}
		type batchTarget struct {
			id      int64
			request MediaRequest
		}
		targets := make(map[string]batchTarget, len(items))
		for id, current := range s.state.MediaRequests {
			if _, wanted := seen[current.RequireKey]; wanted {
				targets[current.RequireKey] = batchTarget{id: id, request: current}
			}
		}
		for _, item := range items {
			key := strings.TrimSpace(item.RequireKey)
			target, ok := targets[key]
			if !ok {
				return ErrNotFound
			}
			if item.Revision != nil && target.request.Revision != *item.Revision {
				return ErrConflict
			}
		}
		for _, item := range items {
			target := targets[strings.TrimSpace(item.RequireKey)]
			current := target.request
			applyMediaRequestStatusUpdate(&current, status, adminNote, replaceNote)
			s.state.MediaRequests[target.id] = current
			updated = append(updated, current)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func applyMediaRequestStatusUpdate(r *MediaRequest, status, adminNote string, replaceNote bool) {
	r.Status = status
	if replaceNote || strings.TrimSpace(adminNote) != "" {
		r.AdminNote = adminNote
	}
	r.Revision++
	r.UpdatedAt = time.Now().Unix()
}

func (s *Store) mediaRequestByKeyLocked(key string) (int64, MediaRequest, bool) {
	for id, request := range s.state.MediaRequests {
		if request.RequireKey == key {
			return id, request, true
		}
	}
	return 0, MediaRequest{}, false
}

func (s *Store) DeleteMediaRequestByKey(key string, expectedRevision *int64) (MediaRequest, error) {
	if strings.TrimSpace(key) == "" {
		return MediaRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var deleted MediaRequest
	err := s.mutateAndSaveLocked(func() error {
		id, current, ok := s.mediaRequestByKeyLocked(key)
		if !ok {
			return ErrNotFound
		}
		if expectedRevision != nil && current.Revision != *expectedRevision {
			return ErrConflict
		}
		deleted = current
		delete(s.state.MediaRequests, id)
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return deleted, nil
}
