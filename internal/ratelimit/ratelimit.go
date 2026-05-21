package ratelimit

import (
	"math/rand"
	"sync"
	"time"

	"web_fetcher/internal/utils"
)

// domainDelay 域名级延迟配置 (min, max) 秒
type domainDelay struct {
	min, max float64
}

var domainDelays = map[string]domainDelay{
	"mp.weixin.qq.com":   {2.0, 3.5},
	"zhuanlan.zhihu.com": {1.5, 2.5},
	"juejin.cn":          {1.0, 1.8},
	"csdn.net":           {0.5, 1.0},
	"sspai.com":          {0.3, 0.8},
}

const defaultMinDelay = 0.5
const defaultMaxDelay = 1.2

// RateLimiter 智能速率限制
type RateLimiter struct {
	mu            sync.Mutex
	requestTimes  []time.Time
	domainLastReq map[string]time.Time
	maxRPS        float64
}

func NewRateLimiter(maxRPS float64) *RateLimiter {
	return &RateLimiter{
		requestTimes:  make([]time.Time, 0, 100),
		domainLastReq: make(map[string]time.Time),
		maxRPS:        maxRPS,
	}
}

func (rl *RateLimiter) Wait(targetURL string) {
	domain := utils.DomainFromURL(targetURL)
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// 全局速率限制：滑动窗口
	if len(rl.requestTimes) >= int(rl.maxRPS) {
		oldest := rl.requestTimes[0]
		elapsed := now.Sub(oldest)
		if elapsed < time.Second {
			wait := time.Second - elapsed + 10*time.Millisecond
			rl.mu.Unlock()
			time.Sleep(wait)
			rl.mu.Lock()
			now = time.Now()
		}
	}

	// 域名级延迟
	delay := domainDelays[domain]
	if delay.min == 0 && delay.max == 0 {
		delay = domainDelay{defaultMinDelay, defaultMaxDelay}
	}

	lastReq := rl.domainLastReq[domain]
	sinceLast := now.Sub(lastReq)
	waitDuration := delay.min + rand.Float64()*(delay.max-delay.min)

	if sinceLast < time.Duration(waitDuration*float64(time.Second)) {
		actualWait := time.Duration(waitDuration*float64(time.Second)) - sinceLast
		rl.mu.Unlock()
		time.Sleep(actualWait)
		now = time.Now()
		rl.mu.Lock()
	}

	// 记录请求时间
	if len(rl.requestTimes) >= 100 {
		rl.requestTimes = rl.requestTimes[1:]
	}
	rl.requestTimes = append(rl.requestTimes, now)
	rl.domainLastReq[domain] = now
}
