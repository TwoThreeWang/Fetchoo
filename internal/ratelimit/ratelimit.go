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
const maxDomainEntries = 10000

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

	rl.mu.Lock()

	// 计算需要等待的时间
	now := time.Now()
	var totalWait time.Duration

	// 全局速率限制：滑动窗口
	if len(rl.requestTimes) >= int(rl.maxRPS) {
		oldest := rl.requestTimes[0]
		elapsed := now.Sub(oldest)
		if elapsed < time.Second {
			totalWait = time.Second - elapsed + 10*time.Millisecond
		}
	}

	// 域名级延迟
	delay := domainDelays[domain]
	if delay.min == 0 && delay.max == 0 {
		delay = domainDelay{defaultMinDelay, defaultMaxDelay}
	}

	lastReq := rl.domainLastReq[domain]
	sinceLast := now.Sub(lastReq) + totalWait
	waitDuration := time.Duration((delay.min + rand.Float64()*(delay.max-delay.min)) * float64(time.Second))

	if sinceLast < waitDuration {
		domainWait := waitDuration - sinceLast
		if domainWait > totalWait {
			totalWait = domainWait
		}
	}

	rl.mu.Unlock()

	// 在锁外等待，不阻塞其他 goroutine
	if totalWait > 0 {
		time.Sleep(totalWait)
	}

	// 重新获取锁，记录请求时间
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now = time.Now()

	if len(rl.requestTimes) >= 100 {
		rl.requestTimes = rl.requestTimes[1:]
	}
	rl.requestTimes = append(rl.requestTimes, now)
	rl.domainLastReq[domain] = now

	// 防止 domainLastReq 无限增长
	if len(rl.domainLastReq) > maxDomainEntries {
		rl.domainLastReq = make(map[string]time.Time)
	}
}
