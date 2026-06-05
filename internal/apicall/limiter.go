package apicall

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"web_fetcher/internal/types"
)

// RateLimiter 基于滑动窗口的全局请求限速器
type RateLimiter struct {
	mu           sync.Mutex
	requestTimes []time.Time
	maxRPS       float64
}

// NewRateLimiter 初始化限速器
func NewRateLimiter(maxRPS float64) *RateLimiter {
	return &RateLimiter{
		requestTimes: make([]time.Time, 0, int(maxRPS+1)*2),
		maxRPS:       maxRPS,
	}
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow() bool {
	if rl.maxRPS <= 0 {
		return true // <= 0 禁用限速
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	// 清理 1 秒之前的请求记录
	cutoff := now.Add(-time.Second)

	// 找到第一个在 1 秒以内的请求
	keepIdx := 0
	for i, t := range rl.requestTimes {
		if t.After(cutoff) {
			keepIdx = i
			break
		}
		if i == len(rl.requestTimes)-1 {
			keepIdx = len(rl.requestTimes)
		}
	}

	// 截断过期记录
	if keepIdx > 0 {
		rl.requestTimes = rl.requestTimes[keepIdx:]
	}

	// 检查速率
	if len(rl.requestTimes) >= int(rl.maxRPS) {
		return false
	}

	rl.requestTimes = append(rl.requestTimes, now)
	return true
}

// Middleware 返回 Gin 的限速和计数中间件
func Middleware(limiter *RateLimiter, store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		isAPI := path == "/fetch" || path == "/batch-fetch"
		if !isAPI {
			c.Next()
			return
		}

		if limiter != nil && !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, types.APIResponse{
				Code:    429,
				Message: "请求过于频繁，请稍后再试",
			})
			return
		}

		c.Next()

		if store != nil && !c.IsAborted() {
			store.Incr()
		}
	}
}
