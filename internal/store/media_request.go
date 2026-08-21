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
