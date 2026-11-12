package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimiter 内存滑动窗口限流器。
//
// 【为什么用「滑动窗口」而不是固定窗口】
// 固定窗口（比如「每分钟最多 10 次」）在窗口边界会有 2 倍突发：
// 59 秒发 10 次 + 60 秒发 10 次 = 1 秒内 20 次。
// 滑动窗口只统计「最近 N 秒」内的请求，边界更平滑。
//
// 【为什么用懒清理而不是后台 goroutine】
// 每次请求时顺手清理这个 key 的过期记录，不需要额外 goroutine 定时扫描，
// 避免「每个限流器一个 goroutine」的资源浪费（也是避免 goroutine 泄漏的好习惯）。
//
// 【局限：单机内存】
// 多实例部署时每个实例独立计数，总量会翻倍。生产环境一般用 Redis 实现
// （INCR + EXPIRE，或用 Lua 脚本做滑动窗口），M6 拆微服务时再升级。
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

// RateLimit 返回一个限流中间件：同一 IP 在 window 时间内最多 limit 次请求。
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	rl := &rateLimiter{
		hits:   make(map[string][]time.Time),
		limit:  limit,
		window: window,
	}
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !rl.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    http.StatusTooManyRequests,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

// allow 判断该 key 是否还有配额。
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 1. 清理过期记录（懒清理）
	old := rl.hits[key]
	valid := old[:0]
	for _, t := range old {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// 2. 判断是否超限
	if len(valid) >= rl.limit {
		rl.hits[key] = valid
		return false
	}

	// 3. 记录本次请求
	rl.hits[key] = append(valid, now)
	return true
}
