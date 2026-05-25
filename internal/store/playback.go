package store

import "time"

const maxStoredPlaybackRecords = 10000

func (s *Store) AddPlaybackRecord(record PlaybackRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	if record.PlayedAt == 0 {
		record.PlayedAt = time.Now().Unix()
	}
	s.state.PlaybackRecords = append([]PlaybackRecord{record}, s.state.PlaybackRecords...)
	if len(s.state.PlaybackRecords) > maxStoredPlaybackRecords {
		s.state.PlaybackRecords = s.state.PlaybackRecords[:maxStoredPlaybackRecords]
	}
	return s.saveLocked()
}

func (s *Store) PlaybackRecords(uid int64, since int64, limit int) []PlaybackRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > maxStoredPlaybackRecords {
		limit = maxStoredPlaybackRecords
	}
	out := make([]PlaybackRecord, 0, minInt(limit, len(s.state.PlaybackRecords)))
	for _, record := range s.state.PlaybackRecords {
		if uid != 0 && record.UID != uid {
			continue
		}
		if since > 0 && record.PlayedAt < since {
			continue
		}
		out = append(out, record)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
