package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	visitors map[string]*Visitor
	mu       sync.RWMutex
	rate     int           // 每分钟允许的请求数
	burst    int           // 突发请求数
	cleanup  time.Duration // 清理间隔
}

// Visitor 访问者信息
type Visitor struct {
	tokens     int       // 剩余令牌数
	lastUpdate time.Time // 上次更新时间
}

// NewRateLimiter 创建速率限制器
// rate: 每分钟允许的请求数
// burst: 突发请求数（令牌桶容量）
func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*Visitor),
		rate:     rate,
		burst:    burst,
		cleanup:  5 * time.Minute,
	}

	// 启动清理协程
	go rl.cleanupVisitors()

	return rl
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// 获取或创建访问者
	visitor, exists := rl.visitors[ip]
	if !exists {
		visitor = &Visitor{
			tokens:     rl.burst,
			lastUpdate: now,
		}
		rl.visitors[ip] = visitor
	}

	// 计算应该补充的令牌数
	elapsed := now.Sub(visitor.lastUpdate)
	tokensToAdd := int(elapsed.Minutes() * float64(rl.rate))

	// 补充令牌（不超过burst）
	visitor.tokens += tokensToAdd
	if visitor.tokens > rl.burst {
		visitor.tokens = rl.burst
	}
	visitor.lastUpdate = now

	// 检查是否有可用令牌
	if visitor.tokens > 0 {
		visitor.tokens--
		return true
	}

	return false
}

// GetRemaining 获取剩余令牌数
func (rl *RateLimiter) GetRemaining(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if visitor, exists := rl.visitors[ip]; exists {
		return visitor.tokens
	}
	return rl.burst
}

// cleanupVisitors 清理过期的访问者记录
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, visitor := range rl.visitors {
			// 如果超过清理间隔未访问，删除记录
			if now.Sub(visitor.lastUpdate) > rl.cleanup {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware 速率限制中间件
// rate: 每分钟允许的请求数（默认60）
// burst: 突发请求数（默认100）
func RateLimitMiddleware(rate, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.Allow(ip) {
			remaining := limiter.GetRemaining(ip)
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rate))
			c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "请求过于频繁，请稍后再试",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		// 设置速率限制响应头
		remaining := limiter.GetRemaining(ip)
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rate))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		c.Next()
	}
}

// DefaultRateLimitMiddleware 默认速率限制中间件（60请求/分钟，突发100）
func DefaultRateLimitMiddleware() gin.HandlerFunc {
	return RateLimitMiddleware(60, 100)
}
