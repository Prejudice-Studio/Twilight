package store

import (
	"sort"
	"time"
)

func (s *Store) AddIPBlacklist(ip, reason string, expireAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	s.state.IPBlacklist[ip] = IPBlacklistEntry{IP: ip, Reason: reason, CreatedAt: time.Now().Unix(), ExpireAt: expireAt}
	return s.saveLocked()
}

func (s *Store) RemoveIPBlacklist(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	delete(s.state.IPBlacklist, ip)
	return s.saveLocked()
}

func (s *Store) ListIPBlacklist() []IPBlacklistEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]IPBlacklistEntry, 0, len(s.state.IPBlacklist))
	for _, entry := range s.state.IPBlacklist {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

func (s *Store) IsIPBlacklisted(ip string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.IPBlacklist[ip]
	if !ok {
		return false
	}
	return entry.ExpireAt == -1 || entry.ExpireAt > time.Now().Unix()
}
