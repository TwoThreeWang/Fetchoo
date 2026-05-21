package headers

import (
	"math/rand"
	"strings"
	"sync"
)

// 常用 User-Agent 池
var userAgentPool = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
}

var refererPool = []string{
	"https://www.google.com/",
	"https://www.bing.com/",
	"https://duckduckgo.com/",
}

// HeadersManager 请求头管理
type HeadersManager struct {
	mu       sync.RWMutex
	lastUAIx int
}

func NewHeadersManager() *HeadersManager {
	return &HeadersManager{}
}

func (hm *HeadersManager) randomUA() string {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.lastUAIx = rand.Intn(len(userAgentPool))
	return userAgentPool[hm.lastUAIx]
}

func (hm *HeadersManager) randomReferer() string {
	return refererPool[rand.Intn(len(refererPool))]
}

// GetHeaders 生成伪装请求头
func (hm *HeadersManager) GetHeaders(targetURL string, refererOverride ...string) map[string]string {
	headers := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Encoding":           "gzip, deflate, br",
		"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8",
		"Cache-Control":             "max-age=0",
		"DNT":                       "1",
		"User-Agent":                hm.randomUA(),
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
	}

	referer := ""
	if len(refererOverride) > 0 && refererOverride[0] != "" {
		referer = refererOverride[0]
	}
	if referer == "" {
		referer = hm.randomReferer()
	}
	if !strings.Contains(targetURL, referer) {
		headers["Referer"] = referer
	}

	return headers
}
