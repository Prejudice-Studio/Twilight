package api

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"sync"
	"time"

	"github.com/prejudice-studio/twilight/internal/redis"
)

type rateLimiter struct {
	mu     sync.Mutex
	items  map[string]rateBucket
	redis  *redis.Client
	prefix string
}

type rateBucket struct {
	Count   int
	ResetAt time.Time
}

func newRateLimiter(redisClient *redis.Client) *rateLimiter {
	return &rateLimiter{items: map[string]rateBucket{}, redis: redisClient, prefix: "twilight:rate:"}
}

func (r *rateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	if r.redis != nil {
		count, err := r.redis.IncrExpire(ctx, r.prefix+key, int(window/time.Second))
		if err == nil {
			return count <= int64(limit)
		}
		zap.L().Warn("redis rate limit failed; falling back to memory", zap.Error(err))
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket := r.items[key]
	if now.After(bucket.ResetAt) {
		bucket = rateBucket{ResetAt: now.Add(window)}
	}
	bucket.Count++
	r.items[key] = bucket
	return bucket.Count <= limit
}

func rateKey(parts ...any) string {
	return fmt.Sprint(parts...)
}
