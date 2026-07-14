package store

import (
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
	if filter == "pending" {
		switch NormalizeMediaRequestStatus(status) {
		case MediaRequestStatusUnhandled, MediaRequestStatusAccepted, MediaRequestStatusDownloading:
			return true
		default:
			return false
		}
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
