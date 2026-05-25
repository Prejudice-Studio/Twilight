package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (s *Store) AddRuntimeLog(entry RuntimeLogEntry, limit int) (RuntimeLogEntry, error) {
	if s == nil {
		return entry, ErrNotFound
	}
	limit = clampRuntimeLogLimit(limit)
	if entry.Time == 0 {
		entry.Time = time.Now().Unix()
	}
	if s.db != nil {
		attrs, err := json.Marshal(entry.Attrs)
		if err != nil {
			return entry, err
		}
		var id int64
		err = s.db.QueryRowContext(
			context.Background(),
			`INSERT INTO twilight_runtime_logs (time, level, message, attrs) VALUES ($1, $2, $3, $4::jsonb) RETURNING id`,
			entry.Time,
			entry.Level,
			entry.Message,
			string(attrs),
		).Scan(&id)
		if err != nil {
			return entry, err
		}
		entry.ID = id
		_ = s.PruneRuntimeLogs(limit)
		return entry, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return entry, err
	}
	if entry.ID == 0 {
		entry.ID = s.state.NextRuntimeLogID
		s.state.NextRuntimeLogID++
	}
	if entry.Time == 0 {
		entry.Time = time.Now().Unix()
	}
	s.state.RuntimeLogs = append(s.state.RuntimeLogs, entry)
	if len(s.state.RuntimeLogs) > limit {
		copy(s.state.RuntimeLogs, s.state.RuntimeLogs[len(s.state.RuntimeLogs)-limit:])
		s.state.RuntimeLogs = s.state.RuntimeLogs[:limit]
	}
	return entry, s.saveLocked()
}

func (s *Store) RuntimeLogs(limit int, after int64) ([]RuntimeLogEntry, int64) {
	if s == nil {
		return nil, after
	}
	if s.db != nil {
		return s.postgresRuntimeLogs(limit, after)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	maxLimit := len(s.state.RuntimeLogs)
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	filtered := make([]RuntimeLogEntry, 0, maxLimit)
	for _, entry := range s.state.RuntimeLogs {
		if after <= 0 || entry.ID > after {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	next := after
	if s.state.NextRuntimeLogID > 1 {
		next = s.state.NextRuntimeLogID - 1
	}
	if len(filtered) > 0 {
		next = filtered[len(filtered)-1].ID
	}
	out := make([]RuntimeLogEntry, len(filtered))
	copy(out, filtered)
	return out, next
}

func (s *Store) RuntimeLogStats() (int64, int) {
	if s == nil {
		return 0, 0
	}
	if s.db != nil {
		var next sql.NullInt64
		var count int
		if err := s.db.QueryRowContext(context.Background(), `SELECT max(id), count(*) FROM twilight_runtime_logs`).Scan(&next, &count); err != nil {
			return 0, 0
		}
		return next.Int64, count
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	next := int64(0)
	if s.state.NextRuntimeLogID > 1 {
		next = s.state.NextRuntimeLogID - 1
	}
	return next, len(s.state.RuntimeLogs)
}

func (s *Store) PruneRuntimeLogs(limit int) error {
	if s == nil {
		return nil
	}
	limit = clampRuntimeLogLimit(limit)
	if s.db != nil {
		_, err := s.db.ExecContext(context.Background(), `
DELETE FROM twilight_runtime_logs
WHERE id NOT IN (
	SELECT id FROM twilight_runtime_logs ORDER BY id DESC LIMIT $1
)`, limit)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return err
	}
	if len(s.state.RuntimeLogs) <= limit {
		return nil
	}
	copy(s.state.RuntimeLogs, s.state.RuntimeLogs[len(s.state.RuntimeLogs)-limit:])
	s.state.RuntimeLogs = s.state.RuntimeLogs[:limit]
	return s.saveLocked()
}

func (s *Store) postgresRuntimeLogs(limit int, after int64) ([]RuntimeLogEntry, int64) {
	limit = clampRuntimeLogReadLimit(limit)
	var (
		rows *sql.Rows
		err  error
	)
	if after > 0 {
		rows, err = s.db.QueryContext(context.Background(), `
SELECT id, time, level, message, COALESCE(attrs, '{}'::jsonb)::text
FROM twilight_runtime_logs
WHERE id > $1
ORDER BY id ASC
LIMIT $2`, after, limit)
	} else {
		rows, err = s.db.QueryContext(context.Background(), `
SELECT id, time, level, message, COALESCE(attrs, '{}'::jsonb)::text
FROM twilight_runtime_logs
ORDER BY id DESC
LIMIT $1`, limit)
	}
	if err != nil {
		return nil, after
	}
	defer rows.Close()
	out := []RuntimeLogEntry{}
	for rows.Next() {
		var entry RuntimeLogEntry
		var attrsText string
		if err := rows.Scan(&entry.ID, &entry.Time, &entry.Level, &entry.Message, &attrsText); err != nil {
			continue
		}
		if attrsText != "" {
			_ = json.Unmarshal([]byte(attrsText), &entry.Attrs)
		}
		out = append(out, entry)
	}
	if after <= 0 {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	next := after
	if len(out) > 0 {
		next = out[len(out)-1].ID
	} else if maxID, _ := s.RuntimeLogStats(); maxID > next {
		next = maxID
	}
	return out, next
}

func clampRuntimeLogReadLimit(limit int) int {
	if limit <= 0 {
		return 200
	}
	if limit > 50000 {
		return 50000
	}
	return limit
}

func clampRuntimeLogLimit(limit int) int {
	if limit < 100 {
		return 100
	}
	if limit > 50000 {
		return 50000
	}
	return limit
}
