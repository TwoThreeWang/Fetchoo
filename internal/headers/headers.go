package headers

import (
	"math/rand"
	"strings"
	"sync"

	"web_fetcher/internal/fingerprint"
)

var refererPool = []string{
	"https://www.google.com/",
	"https://www.bing.com/",
	"https://duckduckgo.com/",
}

// HeadersManager 请求头管理
type HeadersManager struct {
	mu         sync.RWMutex
	profileMgr *fingerprint.ProfileManager
}

func NewHeadersManager() *HeadersManager {
	return &HeadersManager{
		profileMgr: fingerprint.NewProfileManager(false),
	}
}

// GetProfile 返回当前会话使用的浏览器指纹档案
func (hm *HeadersManager) GetProfile() fingerprint.BrowserProfile {
	return hm.profileMgr.GetProfile()
}

func (hm *HeadersManager) randomReferer() string {
	return refererPool[rand.Intn(len(refererPool))]
}

// GetHeaders 生成伪装请求头
func (hm *HeadersManager) GetHeaders(targetURL string, refererOverride ...string) map[string]string {
	profile := hm.GetProfile()
	headers := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Encoding":           "gzip, deflate, br",
		"Accept-Language":           "zh-CN,zh;q=0.9,en;q=0.8",
		"Cache-Control":             "max-age=0",
		"DNT":                       "1",
		"User-Agent":                profile.UserAgent,
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
