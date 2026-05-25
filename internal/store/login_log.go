package store

import "time"

const maxStoredLoginLogs = 1000

func (s *Store) AddLoginLog(log LoginLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	if log.ID == 0 {
		log.ID = s.state.NextLoginLogID
		s.state.NextLoginLogID++
	}
	if log.Time == 0 {
		log.Time = time.Now().Unix()
	}
	s.state.LoginLogs = append([]LoginLog{log}, s.state.LoginLogs...)
	if len(s.state.LoginLogs) > maxStoredLoginLogs {
		s.state.LoginLogs = s.state.LoginLogs[:maxStoredLoginLogs]
	}
	return s.saveLocked()
}

func (s *Store) LoginHistory(uid int64, blockedOnly bool, since int64, limit int) []LoginLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	out := make([]LoginLog, 0, limit)
	for _, log := range s.state.LoginLogs {
		if uid != 0 && log.UID != uid {
			continue
		}
		if blockedOnly && !log.Blocked {
			continue
		}
		if since > 0 && log.Time < since {
			continue
		}
		out = append(out, log)
		if len(out) >= limit {
			break
		}
	}
	return out
}
