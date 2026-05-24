package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/redis"
	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

type sessionStore struct {
	mu     sync.RWMutex
	items  map[string]sessionRecord
	ttl    time.Duration
	redis  *redis.Client
	st     *store.Store
	prefix string
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
func (s *sessionStore) restoreFromPostgres(db *sql.DB) {
	now := time.Now().Unix()
	// Purge expired sessions
	_, _ = db.ExecContext(context.Background(), `DELETE FROM twilight_sessions WHERE expires_at <= $1`, now)

	rows, err := db.QueryContext(context.Background(), `SELECT token, uid, expires_at FROM twilight_sessions WHERE expires_at > $1`, now)
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
				if err := s.redis.SetEX(context.Background(), s.prefix+token, int(remainTTL), string(payload)); err == nil {
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
	token, err := security.RandomHex(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(s.ttl)
	record := sessionRecord{UID: uid, ExpiresAt: expires.Unix()}

	redisOK := false
	if s.redis != nil {
		payload, _ := json.Marshal(record)
		if err := s.redis.SetEX(ctx, s.prefix+token, int(s.ttl/time.Second), string(payload)); err == nil {
			redisOK = true
		} else {
			zap.L().Warn("redis session create failed; using memory+pg fallback", zap.Error(err))
		}
	}

	if !redisOK {
		// Memory fallback when Redis is unavailable
		s.mu.Lock()
		s.items[token] = record
		s.mu.Unlock()
	}

	// Always persist to PostgreSQL for durability (survives both Redis and process restart)
	s.persistToPostgres(token, record)
	return token, expires, nil
}

func (s *sessionStore) persistToPostgres(token string, record sessionRecord) {
	db := s.pgDB()
	if db == nil {
		return
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO twilight_sessions (token, uid, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (token) DO UPDATE SET uid = EXCLUDED.uid, expires_at = EXCLUDED.expires_at`,
		token, record.UID, record.ExpiresAt)
	if err != nil {
		zap.L().Warn("failed to persist session to PostgreSQL", zap.Error(err))
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
		_, _ = db.ExecContext(context.Background(), `DELETE FROM twilight_sessions WHERE token = $1`, token)
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

	// Also remove from PostgreSQL and collect tokens for Redis cleanup
	if db := s.pgDB(); db != nil {
		if s.redis != nil {
			// Fetch tokens before deleting so we can remove from Redis
			rows, err := db.QueryContext(context.Background(),
				`SELECT token FROM twilight_sessions WHERE uid = $1`, uid)
			if err == nil {
				for rows.Next() {
					var token string
					if rows.Scan(&token) == nil {
						_ = s.redis.Del(ctx, s.prefix+token)
					}
				}
				rows.Close()
			}
		}
		_, _ = db.ExecContext(context.Background(), `DELETE FROM twilight_sessions WHERE uid = $1`, uid)
	}
}

// CleanupExpired removes expired sessions from all layers.
func (s *sessionStore) CleanupExpired() int {
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
		result, _ := db.ExecContext(context.Background(), `DELETE FROM twilight_sessions WHERE expires_at <= $1`, now)
		if result != nil {
			if n, err := result.RowsAffected(); err == nil && n > 0 {
				removed = int(n) // PG count is more accurate
			}
		}
	}
	return removed
}

// ActiveCount returns the number of active sessions across all layers.
func (s *sessionStore) ActiveCount() int {
	if db := s.pgDB(); db != nil {
		var count int
		if err := db.QueryRowContext(context.Background(),
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
