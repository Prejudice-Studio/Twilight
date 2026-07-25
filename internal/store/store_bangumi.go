package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxStoredBangumiSyncLogs = 1000

func (s *Store) AddBangumiSyncLog(entry BangumiSyncLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		entry.ID = s.state.NextBangumiSyncLogID
		s.state.NextBangumiSyncLogID++
		if entry.CreatedAt == 0 {
			entry.CreatedAt = time.Now().Unix()
		}
		s.state.BangumiSyncLogs = append(s.state.BangumiSyncLogs, entry)
		if len(s.state.BangumiSyncLogs) > maxStoredBangumiSyncLogs {
			s.state.BangumiSyncLogs = compactTail(s.state.BangumiSyncLogs, maxStoredBangumiSyncLogs)
		}
		return nil
	})
}

func (s *Store) ListBangumiSyncLogs(uid int64, limit int) []BangumiSyncLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > maxStoredBangumiSyncLogs {
		limit = maxStoredBangumiSyncLogs
	}
	out := make([]BangumiSyncLog, 0, limit)
	for i := len(s.state.BangumiSyncLogs) - 1; i >= 0; i-- {
		entry := s.state.BangumiSyncLogs[i]
		if uid != 0 && entry.UID != uid {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Store) DeleteBangumiSyncLog(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		for i, log := range s.state.BangumiSyncLogs {
			if log.ID == id {
				s.state.BangumiSyncLogs = append(s.state.BangumiSyncLogs[:i], s.state.BangumiSyncLogs[i+1:]...)
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) ClearBangumiSyncLogs(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if uid == 0 {
			s.state.BangumiSyncLogs = nil
			return nil
		}
		filtered := make([]BangumiSyncLog, 0, len(s.state.BangumiSyncLogs))
		for _, log := range s.state.BangumiSyncLogs {
			if log.UID != uid {
				filtered = append(filtered, log)
			}
		}
		s.state.BangumiSyncLogs = filtered
		return nil
	})
}

func bangumiCollectionCacheKey(uid int64, collectType int) string {
	return strconv.FormatInt(uid, 10) + ":" + strconv.Itoa(collectType)
}

const maxBangumiSubjectCacheEntries = 5000

func bangumiSubjectCacheKey(subjectID int64) string {
	return strconv.FormatInt(subjectID, 10)
}

func bangumiSubjectIDFromValue(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func bangumiCollectionSubjectID(item map[string]any) int64 {
	if item == nil {
		return 0
	}
	if id := bangumiSubjectIDFromValue(item["subject_id"]); id > 0 {
		return id
	}
	if subject, ok := item["subject"].(map[string]any); ok {
		return bangumiSubjectIDFromValue(subject["id"])
	}
	return 0
}

func cloneBangumiMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	copied := make(map[string]any, len(value))
	for key, item := range value {
		copied[key] = cloneBangumiValue(item)
	}
	return copied
}

func cloneBangumiValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneBangumiMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneBangumiValue(item)
		}
		return out
	case []map[string]any:
		return cloneBangumiMapSlice(typed)
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []int64:
		return append([]int64(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	default:
		return value
	}
}

func cloneBangumiMapSlice(entries []map[string]any) []map[string]any {
	if entries == nil {
		return nil
	}
	copied := make([]map[string]any, len(entries))
	for i, item := range entries {
		copied[i] = cloneBangumiMap(item)
	}
	return copied
}

func cloneBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry) BangumiCollectionCacheEntry {
	entry.Entries = cloneBangumiMapSlice(entry.Entries)
	return entry
}

func hydrateBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry, subjects map[string]BangumiSubjectCacheEntry) BangumiCollectionCacheEntry {
	entry = cloneBangumiCollectionCacheEntry(entry)
	for i, item := range entry.Entries {
		if item == nil {
			continue
		}
		if _, ok := item["subject"]; ok {
			continue
		}
		subjectID := bangumiCollectionSubjectID(item)
		if subjectID <= 0 {
			continue
		}
		if cached, ok := subjects[bangumiSubjectCacheKey(subjectID)]; ok && cached.Subject != nil {
			item["subject"] = cloneBangumiMap(cached.Subject)
			entry.Entries[i] = item
		}
	}
	return entry
}

func normalizeBangumiCollectionCacheEntry(entry BangumiCollectionCacheEntry, now int64, subjects map[string]BangumiSubjectCacheEntry) BangumiCollectionCacheEntry {
	entry = cloneBangumiCollectionCacheEntry(entry)
	for i, item := range entry.Entries {
		if item == nil {
			continue
		}
		subjectID := bangumiCollectionSubjectID(item)
		if subjectID <= 0 {
			continue
		}
		if _, ok := item["subject_id"]; !ok {
			item["subject_id"] = subjectID
		}
		if subject, ok := item["subject"].(map[string]any); ok && subject != nil {
			subjectCopy := cloneBangumiMap(subject)
			if _, ok := subjectCopy["id"]; !ok {
				subjectCopy["id"] = subjectID
			}
			subjects[bangumiSubjectCacheKey(subjectID)] = BangumiSubjectCacheEntry{
				SubjectID: subjectID,
				Subject:   subjectCopy,
				UpdatedAt: now,
				ExpiresAt: entry.ExpiresAt,
			}
			delete(item, "subject")
		}
		entry.Entries[i] = item
	}
	return entry
}

func (s *State) normalizeBangumiCollectionSubjectCache() {
	if s.BangumiCollectionCache == nil {
		return
	}
	if s.BangumiSubjectCache == nil {
		s.BangumiSubjectCache = map[string]BangumiSubjectCacheEntry{}
	}
	now := time.Now().Unix()
	for key, entry := range s.BangumiCollectionCache {
		updatedAt := entry.UpdatedAt
		if updatedAt == 0 {
			updatedAt = now
		}
		s.BangumiCollectionCache[key] = normalizeBangumiCollectionCacheEntry(entry, updatedAt, s.BangumiSubjectCache)
	}
	pruneBangumiSubjectCacheLocked(s.BangumiSubjectCache)
}

func pruneBangumiSubjectCacheLocked(subjects map[string]BangumiSubjectCacheEntry) {
	if len(subjects) <= maxBangumiSubjectCacheEntries {
		return
	}
	type cacheItem struct {
		key       string
		updatedAt int64
	}
	items := make([]cacheItem, 0, len(subjects))
	for key, entry := range subjects {
		items = append(items, cacheItem{key: key, updatedAt: entry.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].updatedAt == items[j].updatedAt {
			return items[i].key < items[j].key
		}
		return items[i].updatedAt < items[j].updatedAt
	})
	for len(subjects) > maxBangumiSubjectCacheEntries && len(items) > 0 {
		delete(subjects, items[0].key)
		items = items[1:]
	}
}

func (s *Store) BangumiCollectionCache(uid int64, collectType int) (BangumiCollectionCacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.BangumiCollectionCache[bangumiCollectionCacheKey(uid, collectType)]
	if !ok {
		return BangumiCollectionCacheEntry{}, false
	}
	return hydrateBangumiCollectionCacheEntry(entry, s.state.BangumiSubjectCache), true
}

func (s *Store) RawBangumiSubjectCache(subjectID int64) (BangumiSubjectCacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.state.BangumiSubjectCache[bangumiSubjectCacheKey(subjectID)]
	return entry, ok
}

func (s *Store) UpsertBangumiCollectionCache(entry BangumiCollectionCacheEntry) error {
	if entry.UID <= 0 || entry.Type <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			s.state.BangumiCollectionCache = map[string]BangumiCollectionCacheEntry{}
		}
		if s.state.BangumiSubjectCache == nil {
			s.state.BangumiSubjectCache = map[string]BangumiSubjectCacheEntry{}
		}
		if entry.UpdatedAt == 0 {
			entry.UpdatedAt = time.Now().Unix()
		}
		entry = normalizeBangumiCollectionCacheEntry(entry, entry.UpdatedAt, s.state.BangumiSubjectCache)
		s.state.BangumiCollectionCache[bangumiCollectionCacheKey(entry.UID, entry.Type)] = entry
		pruneBangumiSubjectCacheLocked(s.state.BangumiSubjectCache)
		return nil
	})
}

func (s *Store) MarkBangumiCollectionCacheError(uid int64, collectType int, message string) error {
	if uid <= 0 || collectType <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			s.state.BangumiCollectionCache = map[string]BangumiCollectionCacheEntry{}
		}
		key := bangumiCollectionCacheKey(uid, collectType)
		entry := s.state.BangumiCollectionCache[key]
		entry.UID = uid
		entry.Type = collectType
		entry.LastError = message
		entry.LastErrorAt = time.Now().Unix()
		s.state.BangumiCollectionCache[key] = entry
		return nil
	})
}

func (s *Store) DeleteBangumiCollectionCache(uid int64, collectType int) error {
	if uid <= 0 {
		return fmt.Errorf("invalid bangumi collection cache key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		if s.state.BangumiCollectionCache == nil {
			return nil
		}
		if collectType > 0 {
			delete(s.state.BangumiCollectionCache, bangumiCollectionCacheKey(uid, collectType))
			return nil
		}
		for key, entry := range s.state.BangumiCollectionCache {
			if entry.UID == uid {
				delete(s.state.BangumiCollectionCache, key)
			}
		}
		return nil
	})
}
