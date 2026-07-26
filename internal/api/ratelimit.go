package api

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/prejudice-studio/twilight/internal/redis"
)

type rateLimiter struct {
	mu          sync.Mutex
	items       map[string]rateBucket
	redis       *redis.Client
	prefix      string
	lastCleanup time.Time

	// redis 失败回退到内存桶时累加，运维通过 /system/stats 观察是否进入降级。
	// 任何非 nil err（连接断开、超时、命令拒绝）都计入；命中后 fallbackCount
	// 持续递增意味着 redis 已不可用，登录限流退化为单进程内存桶。
	fallbackCount atomic.Int64

	// 上次输出降级 Warn 的 unix 纳秒。redis 失联时每个请求都会走回退分支，
	// 若逐条 Warn，高 QPS 端点（global / apikey）会把 twilight_runtime_logs
	// 写爆——日志本身反噬成二次 DoS。这里节流：Warn 最多每 fallbackWarnInterval
	// 出一条，而 fallbackCount 仍逐次自增，保证 /system/stats 观测不失真。
	lastFallbackWarnNanos atomic.Int64
}

// fallbackWarnInterval 降级告警节流窗口：redis 持续不可用时，最多每隔这么久
// 才落一条 Warn，避免高频端点把运行日志表打爆。
const fallbackWarnInterval = 30 * time.Second

type rateBucket struct {
	Count   int
	ResetAt time.Time
}

func newRateLimiter(redisClient *redis.Client) *rateLimiter {
	return &rateLimiter{
		items:       map[string]rateBucket{},
		redis:       redisClient,
		prefix:      "twilight:rate:",
		lastCleanup: time.Now(),
	}
}

func (r *rateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	if r.redis != nil {
		// 向下取整到秒；亚秒窗口会被截成 0，而 EXPIRE key 0 在 redis 中等价于
		// 立即删除 key，桶永远攒不满 → 限流被静默关闭。兜底给 1s 下限。
		expireSeconds := int(window / time.Second)
		if expireSeconds < 1 {
			expireSeconds = 1
		}
		count, err := r.redis.IncrExpire(ctx, r.prefix+key, expireSeconds)
		if err == nil {
			return count <= int64(limit)
		}
		// 计数逐次自增（观测不失真），但 Warn 按 fallbackWarnInterval 节流，
		// 避免 redis 长时间失联时把 twilight_runtime_logs 写爆成二次 DoS。
		r.fallbackCount.Add(1)
		if r.shouldWarnFallback() {
			zap.L().Warn("redis rate limit failed; falling back to memory",
				zap.Error(err), zap.Int64("fallback_total", r.fallbackCount.Load()))
		}
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Periodically purge expired buckets to prevent memory leak
	if now.Sub(r.lastCleanup) > 5*time.Minute {
		for k, b := range r.items {
			if now.After(b.ResetAt) {
				delete(r.items, k)
			}
		}
		r.lastCleanup = now
	}

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

// shouldWarnFallback 判断当前这次降级是否应落一条 Warn。用 CAS 抢占时间戳：
// 并发请求里只有推进时间戳成功的那一路返回 true，其余静默，保证 redis 长时间
// 失联时告警频率被钉在 ~1 条 / fallbackWarnInterval，而非每请求一条。
func (r *rateLimiter) shouldWarnFallback() bool {
	now := time.Now().UnixNano()
	last := r.lastFallbackWarnNanos.Load()
	if now-last < int64(fallbackWarnInterval) {
		return false
	}
	// 只有 CAS 成功的调用方获得本轮告警资格；失败说明已有并发调用抢先，跳过。
	return r.lastFallbackWarnNanos.CompareAndSwap(last, now)
}

// FallbackCount 报告自启动以来 redis 限流失败回退到内存桶的累计次数。
// 仅观察用：值持续增长说明 redis 实例失联或被熔断，多副本部署会出现"每副本
// 各自一份内存桶"的降级，限流上限实际被放大 N 倍。
func (r *rateLimiter) FallbackCount() int64 {
	if r == nil {
		return 0
	}
	return r.fallbackCount.Load()
}
