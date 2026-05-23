package api

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/redis"
	"github.com/prejudice-studio/twilight/internal/security"
)

type sessionStore struct {
	mu     sync.RWMutex
	items  map[string]sessionRecord
	ttl    time.Duration
	redis  *redis.Client
	prefix string
}

type sessionRecord struct {
	UID       int64 `json:"uid"`
	ExpiresAt int64 `json:"expires_at"`
}

func newSessionStore(ttl time.Duration, redisClient *redis.Client) *sessionStore {
	return &sessionStore{items: map[string]sessionRecord{}, ttl: ttl, redis: redisClient, prefix: "twilight:session:"}
}

func (s *sessionStore) Create(ctx context.Context, uid int64) (string, time.Time, error) {
	token, err := security.RandomHex(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(s.ttl)
	record := sessionRecord{UID: uid, ExpiresAt: expires.Unix()}
	if s.redis != nil {
		payload, _ := json.Marshal(record)
		if err := s.redis.SetEX(ctx, s.prefix+token, int(s.ttl/time.Second), string(payload)); err == nil {
			return token, expires, nil
		} else {
			zap.L().Warn("redis session create failed; falling back to memory", zap.Error(err))
		}
	}
	s.mu.Lock()
	s.items[token] = record
	s.mu.Unlock()
	return token, expires, nil
}

func (s *sessionStore) Get(ctx context.Context, token string) (int64, bool) {
	if token == "" {
		return 0, false
	}
	if s.redis != nil {
		payload, ok, err := s.redis.Get(ctx, s.prefix+token)
		if err == nil && ok {
			var record sessionRecord
			if json.Unmarshal([]byte(payload), &record) == nil && record.ExpiresAt >= time.Now().Unix() {
				return record.UID, true
			}
		} else if err != nil {
			zap.L().Warn("redis session read failed; falling back to memory", zap.Error(err))
		}
	}
	s.mu.RLock()
	record, ok := s.items[token]
	s.mu.RUnlock()
	if !ok || record.ExpiresAt < time.Now().Unix() {
		return 0, false
	}
	return record.UID, true
}

func (s *sessionStore) Delete(ctx context.Context, token string) {
	if token == "" {
		return
	}
	if s.redis != nil {
		_ = s.redis.Del(ctx, s.prefix+token)
	}
	s.mu.Lock()
	delete(s.items, token)
	s.mu.Unlock()
}

func (s *sessionStore) DeleteUser(ctx context.Context, uid int64) {
	// Redis scans are intentionally avoided in the hot path. API refresh/logout rotates single tokens;
	// full user logout clears in-memory sessions and lets Redis sessions expire by TTL.
	s.mu.Lock()
	for token, record := range s.items {
		if record.UID == uid {
			delete(s.items, token)
		}
	}
	s.mu.Unlock()
}
