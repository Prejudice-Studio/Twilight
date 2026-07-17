package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

const maxStoredPlaybackRecords = 5000

const maxPlaybackSessions = 2000

func (s *Store) AddPlaybackSession(session PlaybackSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateAndSaveLocked(func() error {
		s.state.PlaybackSessions = append(s.state.PlaybackSessions, session)
		if len(s.state.PlaybackSessions) > maxPlaybackSessions {
			s.state.PlaybackSessions = compactTail(s.state.PlaybackSessions, maxPlaybackSessions)
		}
		return nil
	})
}

func (s *Store) UserPlaybackSessions(uid int64, limit int) []PlaybackSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.state.PlaybackSessions) {
		limit = len(s.state.PlaybackSessions)
	}
	out := make([]PlaybackSession, 0, limit)
	for i := len(s.state.PlaybackSessions) - 1; i >= 0 && len(out) < limit; i-- {
		sess := s.state.PlaybackSessions[i]
		if uid > 0 && sess.UID != uid {
			continue
		}
		out = append(out, sess)
	}
	return out
}

const maxEmbyActivityLogs = 10000

func (s *Store) SyncEmbyActivityLogs(entries []EmbyActivityLog) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	added := 0
	err := s.mutateAndSaveLocked(func() error {
		if s.state.NextEmbyActivityLogID <= 0 {
			maxID := int64(0)
			for _, entry := range s.state.EmbyActivityLogs {
				if entry.ID > maxID {
					maxID = entry.ID
				}
			}
			s.state.NextEmbyActivityLogID = maxID + 1
		}
		existing := map[int64]int{}
		for i, entry := range s.state.EmbyActivityLogs {
			existing[entry.EmbyLogID] = i
		}
		for _, entry := range entries {
			if index, ok := existing[entry.EmbyLogID]; ok {
				current := s.state.EmbyActivityLogs[index]
				entry.ID = current.ID
				entry.CreatedAt = current.CreatedAt
				s.state.EmbyActivityLogs[index] = entry
				continue
			}
			if entry.ID == 0 {
				entry.ID = s.state.NextEmbyActivityLogID
				s.state.NextEmbyActivityLogID++
			}
			if entry.CreatedAt == 0 {
				entry.CreatedAt = time.Now().Unix()
			}
			s.state.EmbyActivityLogs = append(s.state.EmbyActivityLogs, entry)
			existing[entry.EmbyLogID] = len(s.state.EmbyActivityLogs) - 1
			added++
		}
		sort.Slice(s.state.EmbyActivityLogs, func(i, j int) bool {
			if s.state.EmbyActivityLogs[i].Date != s.state.EmbyActivityLogs[j].Date {
				return s.state.EmbyActivityLogs[i].Date < s.state.EmbyActivityLogs[j].Date
			}
			return s.state.EmbyActivityLogs[i].EmbyLogID < s.state.EmbyActivityLogs[j].EmbyLogID
		})
		if len(s.state.EmbyActivityLogs) > maxEmbyActivityLogs {
			s.state.EmbyActivityLogs = compactTail(s.state.EmbyActivityLogs, maxEmbyActivityLogs)
		}
		return nil
	})
	return added, err
}

func (s *Store) ListEmbyActivityLogs(uid int64, limit int) []EmbyActivityLog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.state.EmbyActivityLogs) {
		limit = len(s.state.EmbyActivityLogs)
	}
	targetEmbyID := ""
	if uid > 0 {
		user, ok := s.state.Users[uid]
		if !ok || user.EmbyID == "" {
			return nil
		}
		targetEmbyID = user.EmbyID
	}
	out := make([]EmbyActivityLog, 0, limit)
	for i := len(s.state.EmbyActivityLogs) - 1; i >= 0 && len(out) < limit; i-- {
		entry := s.state.EmbyActivityLogs[i]
		if uid > 0 {
			if entry.UserID != targetEmbyID {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

func (s *Store) AddPlaybackRecord(record PlaybackRecord) error {
	_, err := s.AddPlaybackRecordIdempotent(record)
	return err
}

// AddPlaybackRecordIdempotent 在 (UID, ItemID, PlayedAt) 已经存在时跳过写入，
// 返回 inserted=false。bangumi webhook 没有 timestamp + nonce 强签名，
// 攻击者重放同一份合法请求体本来会让 PlaybackRecords 不停堆积；这里以
// (uid + 媒体条目 + 播放秒) 作为天然幂等键阻断 replay 放大写入。
//
// 真实业务里同一用户在同一秒 stop 同一条目的概率为 0：webhook 由 Emby
// "PlaybackStopped" 事件触发，事件之间至少相隔几秒。即使因网络重试导致同
// 一事件重发，时间戳也会被 server 端的 time.Now() 在重试间隔内推进——所
// 以受害的并不是合法用户，幂等去重不会丢失任何真实播放。
//
// 注意：本方法仍然假定调用方已经做过身份校验。它只阻止"已校验请求"被
// 多次重放——签名伪造 / token 泄露这类外部信任问题不在这里处理。
func (s *Store) AddPlaybackRecordIdempotent(record PlaybackRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return false, err
	}
	if record.PlayedAt == 0 {
		record.PlayedAt = time.Now().Unix()
	}
	if record.UID != 0 && record.ItemID != "" {
		for _, existing := range s.state.PlaybackRecords {
			if existing.UID == record.UID && existing.ItemID == record.ItemID && existing.PlayedAt == record.PlayedAt {
				return false, nil
			}
		}
	}
	s.prependPlaybackRecordLocked(record)
	if err := s.saveLocked(); err != nil {
		return false, err
	}
	if s.db != nil && record.UID != 0 && record.ItemID != "" {
		inserted, dbErr := insertPlaybackRecordDB(s.db, record)
		if dbErr != nil {
			return inserted, nil
		}
	}
	return true, nil
}

func (s *Store) prependPlaybackRecordLocked(record PlaybackRecord) {
	records := s.state.PlaybackRecords
	if len(records) >= maxStoredPlaybackRecords {
		if maxStoredPlaybackRecords <= 0 {
			s.state.PlaybackRecords = nil
			return
		}
		if cap(records) > maxStoredPlaybackRecords {
			compacted := make([]PlaybackRecord, maxStoredPlaybackRecords)
			copy(compacted, records[:maxStoredPlaybackRecords])
			records = compacted
		} else {
			records = records[:maxStoredPlaybackRecords]
		}
		copy(records[1:], records[:maxStoredPlaybackRecords-1])
		records[0] = record
		s.state.PlaybackRecords = records
		return
	}
	records = append(records, PlaybackRecord{})
	copy(records[1:], records[:len(records)-1])
	records[0] = record
	s.state.PlaybackRecords = records
}

func (s *Store) PlaybackRecords(uid int64, since int64, limit int) []PlaybackRecord {
	s.mu.RLock()
	db := s.db
	s.mu.RUnlock()
	if db != nil {
		records, err := queryPlaybackRecordsDB(db, uid, since, limit)
		if err == nil && len(records) > 0 {
			return records
		}
	}
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

func (s *Store) PlaybackRecordSummary(since int64) (totalPlays int, totalDuration int64, uniqueUsers int, err error) {
	if s.db != nil {
		err = queryPlaybackSummaryDB(s.db, since, &totalPlays, &totalDuration, &uniqueUsers)
		if err == nil {
			return
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := map[int64]bool{}
	for _, record := range s.state.PlaybackRecords {
		if since > 0 && record.PlayedAt < since {
			continue
		}
		totalPlays++
		totalDuration += record.Duration
		users[record.UID] = true
	}
	uniqueUsers = len(users)
	return
}

func queryPlaybackRecordsDB(db *sql.DB, uid int64, since int64, limit int) ([]PlaybackRecord, error) {
	var args []any
	var clauses []string
	if uid > 0 {
		clauses = append(clauses, fmt.Sprintf("uid = $%d", len(args)+1))
		args = append(args, uid)
	}
	if since > 0 {
		clauses = append(clauses, fmt.Sprintf("played_at >= $%d", len(args)+1))
		args = append(args, since)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	if limit <= 0 {
		limit = 10000
	}
	query := fmt.Sprintf(`SELECT uid, item_id, title, series_name, media_type, index_number, duration, played_at
FROM twilight_playback_records %s ORDER BY played_at DESC LIMIT $%d`, where, len(args)+1)
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []PlaybackRecord
	for rows.Next() {
		var r PlaybackRecord
		if err := rows.Scan(&r.UID, &r.ItemID, &r.Title, &r.SeriesName, &r.MediaType, &r.IndexNumber, &r.Duration, &r.PlayedAt); err != nil {
			return records, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func queryPlaybackSummaryDB(db *sql.DB, since int64, totalPlays *int, totalDuration *int64, uniqueUsers *int) error {
	query := `SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(duration), 0), COALESCE(COUNT(DISTINCT uid), 0)
FROM twilight_playback_records WHERE played_at >= $1`
	return db.QueryRow(query, since).Scan(totalPlays, totalDuration, uniqueUsers)
}

func insertPlaybackRecordDB(db *sql.DB, record PlaybackRecord) (bool, error) {
	result, err := db.Exec(`INSERT INTO twilight_playback_records (uid, item_id, title, series_name, media_type, index_number, duration, played_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (uid, item_id, played_at) DO NOTHING`,
		record.UID, record.ItemID, record.Title, record.SeriesName, record.MediaType, record.IndexNumber, record.Duration, record.PlayedAt)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func (s *Store) DeletePlaybackRecordsBefore(ctx context.Context, cutoff int64) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM twilight_playback_records WHERE played_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
