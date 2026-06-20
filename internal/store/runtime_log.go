package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"sync/atomic"
	"time"
)

// pgRuntimeLogPruneEvery 控制 PG 后端 runtime log 的 prune 节奏：
// 旧实现是每条 INSERT 后立即跑一次 `DELETE … WHERE id NOT IN (SELECT … LIMIT N)`
// 全表反向扫描，busy 期间会反向卡住所有 zap 调用方。改成每 N 条触发一次
// 后台 prune（异步、带自身 ctx），写入路径只做 INSERT。
const pgRuntimeLogPruneEvery = 256

const (
	pgRuntimeLogWriteTimeout = 5 * time.Second
	pgRuntimeLogReadTimeout  = 5 * time.Second
	pgRuntimeLogPruneTimeout = 10 * time.Second
)

const pgRuntimeLogPruneSQL = `
WITH keep AS (
	SELECT MIN(id) AS min_id
	FROM (
		SELECT id FROM twilight_runtime_logs ORDER BY id DESC LIMIT $1
	) latest
)
DELETE FROM twilight_runtime_logs
WHERE id < COALESCE((SELECT min_id FROM keep), 0)`

// pgRuntimeLogPruneCounter 用 atomic 共享自增计数；触发阈值时跳一次
// goroutine 异步 prune。pruneInFlight 互斥锁防止多 goroutine 同时跑同一个
// DELETE，避免在 burst 时叠加成 N 个并发全表扫描。
var (
	pgRuntimeLogPruneCounter atomic.Uint64
	pgRuntimeLogPruneGate    atomic.Bool
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
		ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogWriteTimeout)
		defer cancel()
		var id int64
		err = s.db.QueryRowContext(
			ctx,
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
		s.maybeAsyncPrunePGRuntimeLogs(limit)
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

// maybeAsyncPrunePGRuntimeLogs 每 pgRuntimeLogPruneEvery 条 INSERT 触发一次
// 后台 prune；写入路径不再阻塞在 DELETE 上。pgRuntimeLogPruneGate 保证同一
// 时刻只有一个 prune goroutine 在跑，避免 burst 时叠加并发全表 DELETE。
func (s *Store) maybeAsyncPrunePGRuntimeLogs(limit int) {
	if s == nil || s.db == nil {
		return
	}
	if pgRuntimeLogPruneCounter.Add(1)%pgRuntimeLogPruneEvery != 0 {
		return
	}
	if !pgRuntimeLogPruneGate.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer pgRuntimeLogPruneGate.Store(false)
		defer func() {
			// 异步 goroutine 入口加 recover：prune SQL 异常不能反向拖垮调用 zap.Info 的协程。
			_ = recover()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogPruneTimeout)
		defer cancel()
		_, _ = s.db.ExecContext(ctx, pgRuntimeLogPruneSQL, clampRuntimeLogLimit(limit))
	}()
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
	next := after
	if s.state.NextRuntimeLogID > 1 {
		next = s.state.NextRuntimeLogID - 1
	}
	start, end := runtimeLogWindow(s.state.RuntimeLogs, limit, after)
	if start == end {
		return nil, next
	}
	filtered := s.state.RuntimeLogs[start:end]
	next = filtered[len(filtered)-1].ID
	out := make([]RuntimeLogEntry, len(filtered))
	copy(out, filtered)
	return out, next
}

func runtimeLogWindow(entries []RuntimeLogEntry, limit int, after int64) (int, int) {
	if len(entries) == 0 || limit <= 0 {
		return 0, 0
	}
	if limit > len(entries) {
		limit = len(entries)
	}
	if after <= 0 {
		start := len(entries) - limit
		if start < 0 {
			start = 0
		}
		return start, len(entries)
	}
	start := sort.Search(len(entries), func(i int) bool {
		return entries[i].ID > after
	})
	if start >= len(entries) {
		return len(entries), len(entries)
	}
	end := start + limit
	if end > len(entries) {
		end = len(entries)
	}
	return start, end
}

func (s *Store) RuntimeLogStats() (int64, int) {
	if s == nil {
		return 0, 0
	}
	if s.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogReadTimeout)
		defer cancel()
		var next sql.NullInt64
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT max(id), count(*) FROM twilight_runtime_logs`).Scan(&next, &count); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogPruneTimeout)
		defer cancel()
		_, err := s.db.ExecContext(ctx, pgRuntimeLogPruneSQL, limit)
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
	ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogReadTimeout)
	defer cancel()
	if after > 0 {
		rows, err = s.db.QueryContext(ctx, `
SELECT id, time, level, message, COALESCE(attrs, '{}'::jsonb)::text
FROM twilight_runtime_logs
WHERE id > $1
ORDER BY id ASC
LIMIT $2`, after, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
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
	} else if maxID := s.postgresRuntimeLogMaxID(); maxID > next {
		next = maxID
	}
	return out, next
}

func (s *Store) postgresRuntimeLogMaxID() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), pgRuntimeLogReadTimeout)
	defer cancel()
	var next sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT max(id) FROM twilight_runtime_logs`).Scan(&next); err != nil {
		return 0
	}
	return next.Int64
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
