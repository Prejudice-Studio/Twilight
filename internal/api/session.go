package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/redis"
	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

const maxSessionTokenCreateAttempts = 8

type sessionStore struct {
	mu     sync.RWMutex
	items  map[string]sessionRecord
	ttl    time.Duration
	redis  *redis.Client
	st     *store.Store
	prefix string

	// redis 写 / 读失败导致路径退到 memory + postgres 时累加，便于运维通过
	// /system/stats 观察是否进入降级。值持续增长意味着 redis 不可用，登录会
	// 话存活仍由 postgres 兜底，但 ttl 失效不再实时同步多副本。
	fallbackCount atomic.Int64
}

type sessionRecord struct {
	UID       int64 `json:"uid"`
	ExpiresAt int64 `json:"expires_at"`
}

func newSessionStore(ttl time.Duration, redisClient *redis.Client) *sessionStore {
	return &sessionStore{items: map[string]sessionRecord{}, ttl: ttl, redis: redisClient, prefix: "twilight:session:"}
}

func newSessionStoreWithDB(ttl time.Duration, redisClient *redis.Client, st *store.Store) *sessionStore {
	ss := &sessionStore{items: map[string]sessionRecord{}, ttl: ttl, redis: redisClient, st: st, prefix: "twilight:session:"}
	if db := st.DB(); db != nil {
		ss.restoreFromPostgres(db)
	}
	return ss
}

// restoreFromPostgres loads valid sessions from PostgreSQL.
// When Redis is available, it re-populates Redis with any sessions that survived
// a Redis restart. When Redis is unavailable, sessions are loaded into memory.
//
// 启动期没有 caller ctx，这里用显式 30s WithTimeout 兜底，避免 PG 慢响应让
// newSessionStoreWithDB 阻塞整个启动流程；之前裸 context.Background() 一旦
// PG 卡住会让 App.New 卡死。
func (s *sessionStore) restoreFromPostgres(db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Now().Unix()
	// Purge expired sessions
	_, _ = db.ExecContext(ctx, `DELETE FROM twilight_sessions WHERE expires_at <= $1`, now)

	rows, err := db.QueryContext(ctx, `SELECT token, uid, expires_at FROM twilight_sessions WHERE expires_at > $1`, now)
	if err != nil {
		zap.L().Warn("failed to load sessions from PostgreSQL", zap.Error(err))
		return
	}
	defer rows.Close()

	restored := 0
	restoredToRedis := 0
	for rows.Next() {
		var token string
		var record sessionRecord
		if err := rows.Scan(&token, &record.UID, &record.ExpiresAt); err != nil {
			continue
		}
		restored++

		// If Redis is available, push sessions back into Redis (handles Redis restart)
		if s.redis != nil {
			remainTTL := record.ExpiresAt - now
			if remainTTL > 0 {
				payload, _ := json.Marshal(record)
				if err := s.redis.SetEX(ctx, s.prefix+token, int(remainTTL), string(payload)); err == nil {
					restoredToRedis++
					continue
				}
			}
		}
		// Fallback: load into memory
		s.mu.Lock()
		s.items[token] = record
		s.mu.Unlock()
	}
	if restored > 0 {
		zap.L().Info("restored sessions from PostgreSQL",
			zap.Int("total", restored),
			zap.Int("to_redis", restoredToRedis),
			zap.Int("to_memory", restored-restoredToRedis))
	}
}

func (s *sessionStore) pgDB() *sql.DB {
	if s.st == nil {
		return nil
	}
	return s.st.DB()
}

func (s *sessionStore) Create(ctx context.Context, uid int64) (string, time.Time, error) {
	for attempt := 0; attempt < maxSessionTokenCreateAttempts; attempt++ {
		token, err := security.RandomHex(32)
		if err != nil {
			return "", time.Time{}, err
		}
		expires := time.Now().Add(s.ttl)
		record := sessionRecord{UID: uid, ExpiresAt: expires.Unix()}

		if !s.persistToPostgres(ctx, token, record) {
			zap.L().Warn("session token collision in PostgreSQL; retrying")
			continue
		}

		redisOK := false
		if s.redis != nil {
			payload, _ := json.Marshal(record)
			ok, err := s.redis.SetEXNX(ctx, s.prefix+token, int(s.ttl/time.Second), string(payload))
			if err == nil && ok {
				redisOK = true
			} else if err == nil {
				s.deletePostgresToken(ctx, token)
				zap.L().Warn("session token collision in Redis; retrying")
				continue
			} else {
				s.fallbackCount.Add(1)
				zap.L().Warn("redis session create failed; using memory+pg fallback", zap.Error(err))
			}
		}

		if !redisOK {
			// Memory fallback when Redis is unavailable. Check again under the
			// write lock so concurrent fallback writers cannot reuse a token.
			s.mu.Lock()
			if _, exists := s.items[token]; exists {
				s.mu.Unlock()
				s.deletePostgresToken(ctx, token)
				zap.L().Warn("session token collision in memory fallback; retrying")
				continue
			}
			s.items[token] = record
			s.mu.Unlock()
		}

		return token, expires, nil
	}
	return "", time.Time{}, errors.New("failed to create unique session token")
}

func (s *sessionStore) persistToPostgres(ctx context.Context, token string, record sessionRecord) bool {
	db := s.pgDB()
	if db == nil {
		return true
	}
	result, err := db.ExecContext(ctx,
		`INSERT INTO twilight_sessions (token, uid, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO NOTHING`,
		token, record.UID, record.ExpiresAt)
	if err != nil {
		zap.L().Warn("failed to persist session to PostgreSQL", zap.Error(err))
		return true
	}
	if rows, err := result.RowsAffected(); err == nil && rows == 0 {
		return false
	}
	return true
}

func (s *sessionStore) deletePostgresToken(ctx context.Context, token string) {
	if db := s.pgDB(); db != nil {
		_, _ = db.ExecContext(ctx, `DELETE FROM twilight_sessions WHERE token = $1`, token)
	}
}

func (s *sessionStore) Get(ctx context.Context, token string) (int64, bool) {
	if token == "" {
		return 0, false
	}

	// 1. Try Redis (fastest path)
	if s.redis != nil {
		payload, ok, err := s.redis.Get(ctx, s.prefix+token)
		if err == nil && ok {
			var record sessionRecord
			if json.Unmarshal([]byte(payload), &record) == nil && record.ExpiresAt >= time.Now().Unix() {
				return record.UID, true
			}
			// Expired in Redis - clean up
			return 0, false
		}
		if err != nil {
			s.fallbackCount.Add(1)
			zap.L().Warn("redis session read failed; checking fallbacks", zap.Error(err))
		}
		// Redis miss (not error) - check PostgreSQL for sessions that survived Redis restart
	}

	// 2. Try in-memory cache
	s.mu.RLock()
	record, ok := s.items[token]
	s.mu.RUnlock()
	if ok {
		if record.ExpiresAt >= time.Now().Unix() {
			return record.UID, true
		}
		// Expired in memory - clean up lazily
		s.mu.Lock()
		delete(s.items, token)
		s.mu.Unlock()
		return 0, false
	}

	// 3. Try PostgreSQL (handles Redis restart scenario)
	if db := s.pgDB(); db != nil {
		var rec sessionRecord
		err := db.QueryRowContext(ctx,
			`SELECT uid, expires_at FROM twilight_sessions WHERE token = $1 AND expires_at > $2`,
			token, time.Now().Unix()).Scan(&rec.UID, &rec.ExpiresAt)
		if err == nil {
			// Found in PG - re-populate Redis for future fast lookups
			if s.redis != nil {
				remainTTL := rec.ExpiresAt - time.Now().Unix()
				if remainTTL > 0 {
					payload, _ := json.Marshal(rec)
					_ = s.redis.SetEX(ctx, s.prefix+token, int(remainTTL), string(payload))
				}
			}
			return rec.UID, true
		}
	}

	return 0, false
}

func (s *sessionStore) Delete(ctx context.Context, token string) {
	if token == "" {
		return
	}
	// Remove from all layers
	if s.redis != nil {
		_ = s.redis.Del(ctx, s.prefix+token)
	}
	s.mu.Lock()
	delete(s.items, token)
	s.mu.Unlock()
	if db := s.pgDB(); db != nil {
		_, _ = db.ExecContext(ctx, `DELETE FROM twilight_sessions WHERE token = $1`, token)
	}
}

func (s *sessionStore) DeleteUser(ctx context.Context, uid int64) {
	// Collect tokens from memory
	s.mu.Lock()
	for token, record := range s.items {
		if record.UID == uid {
			delete(s.items, token)
			if s.redis != nil {
				_ = s.redis.Del(ctx, s.prefix+token)
			}
		}
	}
	s.mu.Unlock()

	// Also remove from PostgreSQL. Use DELETE ... RETURNING token in a single
	// statement so we collect tokens for Redis cleanup atomically — the old
	// SELECT-then-DELETE pair left a window where another connection could
	// INSERT a fresh session for this uid between the two statements: the new
	// token would either be missed by Redis cleanup (left dangling) or wiped
	// by the broad DELETE (kicking a user who just logged in).
	if db := s.pgDB(); db != nil {
		rows, err := db.QueryContext(ctx,
			`DELETE FROM twilight_sessions WHERE uid = $1 RETURNING token`, uid)
		if err == nil {
			for rows.Next() {
				var token string
				if rows.Scan(&token) == nil && s.redis != nil {
					_ = s.redis.Del(ctx, s.prefix+token)
				}
			}
			rows.Close()
		}
	}
}

// CleanupExpired removes expired sessions from all layers.
//
// 接受 caller ctx：scheduler 已提供 runCtx，被 admin 终止时能立刻取消。
// 不接 ctx 的旧版本若 PG DELETE 卡住，scheduler 永远等不到 finish()。
func (s *sessionStore) CleanupExpired(ctx context.Context) int {
	now := time.Now().Unix()
	removed := 0

	// Clean memory
	s.mu.Lock()
	for token, record := range s.items {
		if record.ExpiresAt <= now {
			delete(s.items, token)
			removed++
		}
	}
	s.mu.Unlock()

	// Clean PostgreSQL (Redis handles TTL expiry automatically)
	if db := s.pgDB(); db != nil {
		result, _ := db.ExecContext(ctx, `DELETE FROM twilight_sessions WHERE expires_at <= $1`, now)
		if result != nil {
			if n, err := result.RowsAffected(); err == nil && n > 0 {
				removed = int(n) // PG count is more accurate
			}
		}
	}
	return removed
}

// ActiveCount returns the number of active sessions across all layers.
//
// 接受 caller ctx，避免在 system_stats 等监控请求里被卡死的 PG count(*) 拖
// 累整个 handler 超时。调用方可用 r.Context() 或 WithTimeout 兜底。
func (s *sessionStore) ActiveCount(ctx context.Context) int {
	if db := s.pgDB(); db != nil {
		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT count(*) FROM twilight_sessions WHERE expires_at > $1`,
			time.Now().Unix()).Scan(&count); err == nil {
			return count
		}
	}
	// Fallback to memory count
	now := time.Now().Unix()
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, record := range s.items {
		if record.ExpiresAt > now {
			count++
		}
	}
	return count
}

// FallbackCount 报告自启动以来 redis session 失败回退到内存 / postgres 的累计
// 次数。值持续增长意味着 redis 不可用：sessions 仍能通过 postgres 维持登录，
// 但跨副本 ttl 同步不再走 redis；运维应优先恢复 redis。
func (s *sessionStore) FallbackCount() int64 {
	if s == nil {
		return 0
	}
	return s.fallbackCount.Load()
}
