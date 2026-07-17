package store

import (
	"container/heap"
	"sort"
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
	Requests []MediaRequest
	Total    int
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
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage
	window := offset + perPage
	if window < perPage {
		return MediaRequestPage{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	var top mediaRequestIDMinHeap
	total := 0
	heapReady := false
	for _, r := range s.state.MediaRequests {
		if !all && r.UID != uid {
			continue
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
	if total == 0 || offset >= len(top) {
		return MediaRequestPage{Total: total}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].ID > top[j].ID })
	end := offset + perPage
	if end > len(top) {
		end = len(top)
	}
	requests := make([]MediaRequest, end-offset)
	copy(requests, top[offset:end])
	return MediaRequestPage{Requests: requests, Total: total}
}

func (s *Store) UpdateMediaRequestStatus(id int64, rawStatus string, adminNote string, replaceNote bool) (MediaRequest, error) {
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
		r.Status = status
		if replaceNote || strings.TrimSpace(adminNote) != "" {
			r.AdminNote = adminNote
		}
		r.UpdatedAt = time.Now().Unix()
		s.state.MediaRequests[id] = r
		updated = r
		return nil
	})
	if err != nil {
		return MediaRequest{}, err
	}
	return updated, nil
}
