package fetcher

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"web_fetcher/internal/cache"
	"web_fetcher/internal/extractor"
	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/types"
	"web_fetcher/internal/utils"
)

const (
	maxMemCacheEntries    = 500             // 内存缓存最大条目数
	memCacheCleanInterval = 5 * time.Minute // 内存缓存清理间隔
	maxRetries            = 1
)

var reMultiNewline = regexp.MustCompile(`\n{3,}`)

// stealthDomains 需要强制使用浏览器抓取的域名
var stealthDomains = map[string]bool{
	"mp.weixin.qq.com":   true,
	"zhuanlan.zhihu.com": true,
	"juejin.cn":          true,
	"mp.toutiao.com":     true,
}

// WebContentFetcher 网页内容获取服务 — 串行降级 + 缓存
type WebContentFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	cache          *cache.SQLiteCache
	memCache       *sync.Map
	memCacheCount  int32         // 内存缓存条目计数（原子操作）
	memCacheKeys   []string      // 用于 LRU 淘汰的 key 顺序
	memCacheMu     sync.Mutex    // 保护 memCacheCount 和 memCacheKeys
	stopClean      chan struct{} // 停止清理 goroutine

	httpFetcher *HTTPFetcher
	defuddle    *DefuddleReader
	jina        *JinaReader
	webpagesnap *WebPageSnapFetcher
	markdownnew *MarkdownNewFetcher
	firecrawl   *FirecrawlFetcher
	fxtwitter   *FxTwitterFetcher
	bravesearch *BraveSearchFetcher
	browser     *StealthBrowser
	useBrowser  bool
}

// NewWebContentFetcher 创建 Fetcher 实例
func NewWebContentFetcher(db *sql.DB, useBrowser *bool, proxy string, braveAPIKey string, firecrawlAPIKey string) (*WebContentFetcher, error) {
	hm := headers.NewHeadersManager()
	rl := ratelimit.NewRateLimiter(5.0)
	c, err := cache.NewSQLiteCache(db, 7)
	if err != nil {
		return nil, err
	}

	wf := &WebContentFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		cache:          c,
		memCache:       &sync.Map{},
		stopClean:      make(chan struct{}),
		httpFetcher:    NewHTTPFetcher(hm, rl),
		defuddle:       NewDefuddleReader(hm, rl),
		jina:           NewJinaReader(hm, rl),
		webpagesnap:    NewWebPageSnapFetcher(hm, rl),
		markdownnew:    NewMarkdownNewFetcher(hm, rl),
		fxtwitter:      NewFxTwitterFetcher(hm, rl),
		firecrawl:      NewFirecrawlFetcher(hm, rl, firecrawlAPIKey),
		bravesearch:    NewBraveSearchFetcher(hm, rl, braveAPIKey),
	}

	// 决定是否启用浏览器
	if useBrowser != nil && !*useBrowser {
		log.Println("[fetcher] 浏览器已禁用")
	} else if useBrowser != nil && *useBrowser {
		log.Println("[fetcher] 浏览器已强制启用")
		wf.browser = NewStealthBrowser(hm, rl)
		wf.useBrowser = true
	} else if CheckChromiumAvailable() {
		wf.browser = NewStealthBrowser(hm, rl)
		wf.useBrowser = true
		log.Println("[fetcher] ✓ Chromium 可用，已启用浏览器模式")
	} else {
		log.Println("[fetcher] ⚠ Chromium 不可用，使用 HTTP/Defuddle/Jina 模式")
	}

	// 启动内存缓存定期清理
	go wf.memCacheCleaner()

	return wf, nil
}

// shouldUseStealth 判断是否需要 Stealth 模式
func (wf *WebContentFetcher) shouldUseStealth(targetURL string) bool {
	if !wf.useBrowser || wf.browser == nil {
		return false
	}
	domain := utils.DomainFromURL(targetURL)
	for d := range stealthDomains {
		if domain == d || strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
}

// isTwitterDomain 判断是否为 Twitter/X 域名
func isTwitterDomain(targetURL string) bool {
	domain := utils.DomainFromURL(targetURL)
	return domain == "twitter.com" || domain == "x.com" ||
		strings.HasSuffix(domain, ".twitter.com") || strings.HasSuffix(domain, ".x.com")
}

// Fetch 获取网页内容（入口方法）
func (wf *WebContentFetcher) Fetch(targetURL string, maxChars int, forceStealth, skipCache bool) types.FetchResult {
	startTime := time.Now()

	// 0. URL 校验
	valid, errMsg := utils.ValidateURL(targetURL)
	if !valid {
		return types.FetchResult{
			URL: targetURL, Mode: "invalid", Success: false,
			Error: "URL 校验失败: " + errMsg, FetchTime: time.Since(startTime).Seconds(),
		}
	}

	// 1. 检查内存缓存
	if !skipCache {
		if cached, ok := wf.memCache.Load(targetURL); ok {
			cr, ok := cached.(*memCacheEntry)
			if !ok {
				wf.memCache.Delete(targetURL)
			} else if time.Since(cr.createdAt) < time.Duration(wf.cache.TTLDays)*24*time.Hour {
				return types.FetchResult{
					URL: targetURL, Title: cr.meta.Title, Content: cr.content,
					Metadata: cr.meta, Mode: "cached",
					FetchTime:     time.Since(startTime).Seconds(),
					ContentLength: len(cr.content), Success: true,
				}
			}
			wf.memCache.Delete(targetURL)
			wf.memCacheMu.Lock()
			wf.memCacheCount--
			wf.memCacheMu.Unlock()
		}

		// 2. 检查 SQLite 缓存
		content, meta, found := wf.cache.Get(targetURL)
		if found {
			wf.memCacheMu.Lock()
			if int(wf.memCacheCount) >= maxMemCacheEntries && len(wf.memCacheKeys) > 0 {
				evictKey := wf.memCacheKeys[0]
				wf.memCacheKeys = wf.memCacheKeys[1:]
				wf.memCache.Delete(evictKey)
				wf.memCacheCount--
			}
			wf.memCacheKeys = append(wf.memCacheKeys, targetURL)
			wf.memCacheCount++
			wf.memCacheMu.Unlock()
			wf.memCache.Store(targetURL, &memCacheEntry{content: content, meta: meta, createdAt: time.Now()})
			return types.FetchResult{
				URL: targetURL, Title: meta.Title, Content: content,
				Metadata: meta, Mode: "cached",
				FetchTime:     time.Since(startTime).Seconds(),
				ContentLength: len(content), Success: true,
			}
		}
	}

	// 3. 执行串行降级抓取
	content, mode, selector, metadata, err := wf.doFetch(targetURL, maxChars, forceStealth)

	fetchDuration := time.Since(startTime).Seconds()
	if err != nil {
		log.Printf("[fetcher] ✗ 所有方式都失败 %s: %v", utils.Truncate(targetURL, 50), err)
		return types.FetchResult{
			URL: targetURL, Metadata: types.WebMetadata{URL: targetURL},
			Mode: "failed", Success: false, Error: err.Error(),
			FetchTime: fetchDuration,
		}
	}

	// 4. 写入缓存（内存 + SQLite）
	if content != "" {
		wf.memCacheMu.Lock()
		// LRU 淘汰：超过上限时删除最早的条目
		for int(wf.memCacheCount) >= maxMemCacheEntries && len(wf.memCacheKeys) > 0 {
			evictKey := wf.memCacheKeys[0]
			wf.memCacheKeys = wf.memCacheKeys[1:]
			wf.memCache.Delete(evictKey)
			wf.memCacheCount--
		}
		wf.memCacheKeys = append(wf.memCacheKeys, targetURL)
		wf.memCacheCount++
		wf.memCacheMu.Unlock()
		wf.memCache.Store(targetURL, &memCacheEntry{content: content, meta: metadata, createdAt: time.Now()})
		wf.cache.Set(targetURL, content, metadata)
	}

	// 5. 返回结果
	return types.FetchResult{
		URL: metadata.URL, Title: metadata.Title, Content: content,
		Metadata: metadata, Mode: mode,
		FetchTime:       fetchDuration,
		ContentLength:   len(content),
		SelectorMatched: selector,
		Success:         true,
	}
}

type memCacheEntry struct {
	content   string
	meta      types.WebMetadata
	createdAt time.Time
}

// doFetch 核心抓取逻辑 — 串行降级
func (wf *WebContentFetcher) doFetch(targetURL string, maxChars int, forceStealth bool) (string, string, string, types.WebMetadata, error) {
	if isTwitterDomain(targetURL) {
		// Twitter/X 域名：FxTwitter → Browser → WebPageSnap → markdown.new → Defuddle → Jina
		return wf.fetchTwitterMode(targetURL, maxChars)
	}

	shouldStealth := forceStealth || wf.shouldUseStealth(targetURL)
	if shouldStealth && wf.browser != nil {
		// Stealth 域名：Browser → WebPageSnap → markdown.new → Defuddle → Jina（已知 HTTP 拿不到，跳过）
		return wf.fetchStealthMode(targetURL, maxChars)
	}

	// 普通域名：HTTP → Browser → WebPageSnap → markdown.new → Defuddle → Jina
	return wf.fetchNormalMode(targetURL, maxChars)
}

// fetchNormalMode HTTP → Browser → WebPageSnap → markdown.new → Defuddle → Jina
func (wf *WebContentFetcher) fetchNormalMode(targetURL string, maxChars int) (string, string, string, types.WebMetadata, error) {
	// 1. HTTP（最快、不费配额），失败直接降级不重试
	md, sel, htmlStr, contentType, httpErr := wf.httpFetcher.Fetch(targetURL, maxChars)
	if httpErr == nil && extractor.IsValidContent(md, contentType) {
		metadata := extractor.ExtractMetadata(htmlStr, targetURL)
		return md, "http", sel, metadata, nil
	}
	if httpErr != nil {
		log.Printf("[fetcher] HTTP 失败，降级到 Browser: %v", httpErr)
	} else {
		log.Printf("[fetcher] HTTP 内容过少或无效(%d字符, type=%s)，降级到 Browser", len(md), contentType)
	}

	// 2. Browser（JS 渲染）
	if wf.browser != nil {
		brMd, brSel, brHTML, brErr := wf.withRetryBrowser(targetURL, maxChars)
		if brErr == nil && extractor.IsValidContent(brMd, "text/html") {
			metadata := extractor.ExtractMetadata(brHTML, targetURL)
			return brMd, "stealth", brSel, metadata, nil
		}
		if brErr != nil {
			log.Printf("[fetcher] Browser 失败，降级到 Firecrawl: %v", brErr)
		} else {
			log.Printf("[fetcher] Browser 内容过少(%d字符)，降级到 Firecrawl", len(brMd))
		}
	}

	// 3. Firecrawl（stealth 代理，绕 Cloudflare）
	if wf.firecrawl != nil {
		fcContent, fcSel, fcMeta, fcErr := wf.firecrawl.Fetch(targetURL, maxChars)
		if fcErr == nil && extractor.IsValidContent(fcContent, "text/html") {
			return fcContent, "firecrawl", fcSel, fcMeta, nil
		}
		if fcErr != nil {
			log.Printf("[fetcher] Firecrawl 失败，降级到 WebPageSnap: %v", fcErr)
		} else {
			log.Printf("[fetcher] Firecrawl 内容过少(%d字符)，降级到 WebPageSnap", len(fcContent))
		}
	}

	// 4. WebPageSnap
	snapContent, snapSel, snapMeta, snapErr := wf.webpagesnap.Fetch(targetURL, maxChars)
	if snapErr == nil && extractor.IsValidContent(snapContent, "text/html") {
		return snapContent, "webpagesnap", snapSel, snapMeta, nil
	}
	if snapErr != nil {
		log.Printf("[fetcher] WebPageSnap 失败，降级到 markdown.new: %v", snapErr)
	} else {
		log.Printf("[fetcher] WebPageSnap 内容过少(%d字符)，降级到 markdown.new", len(snapContent))
	}

	// 5. markdown.new
	mnContent, mnSel, mnMeta, mnErr := wf.markdownnew.Fetch(targetURL, maxChars)
	if mnErr == nil && extractor.IsValidContent(mnContent, "text/html") {
		return mnContent, "markdown-new", mnSel, mnMeta, nil
	}
	if mnErr != nil {
		log.Printf("[fetcher] markdown.new 失败，降级到 Defuddle: %v", mnErr)
	} else {
		log.Printf("[fetcher] markdown.new 内容过少(%d字符)，降级到 Defuddle", len(mnContent))
	}

	// 6. Defuddle
	defContent, defSel, defFM, defErr := wf.defuddle.Fetch(targetURL)
	if defErr == nil && extractor.IsValidContent(defContent, "text/html") {
		metadata := wf.buildMetadataFromFrontmatter(defFM, targetURL)
		return defContent, "defuddle", defSel, metadata, nil
	}
	if defErr != nil {
		log.Printf("[fetcher] Defuddle 失败，降级到 Jina: %v", defErr)
	}

	// 7. Jina
	jinaContent, jinaSel, jinaErr := wf.jina.Fetch(targetURL)
	if jinaErr == nil && extractor.IsValidContent(jinaContent, "text/html") {
		return jinaContent, "jina", jinaSel, types.WebMetadata{URL: targetURL}, nil
	}
	if jinaErr != nil {
		log.Printf("[fetcher] Jina 失败，降级到 Brave Search: %v", jinaErr)
	} else {
		log.Printf("[fetcher] Jina 内容过少(%d字符)，降级到 Brave Search", len(jinaContent))
	}

	// 8. Brave Search 兜底（用目标 URL 作为搜索词）
	if wf.bravesearch != nil {
		brContent, brSel, brMeta, brErr := wf.bravesearch.Fetch(targetURL, maxChars)
		if brErr == nil {
			return brContent, "brave-search", brSel, brMeta, nil
		}
		log.Printf("[fetcher] Brave Search 失败: %v", brErr)
	}

	return "", "", "", types.WebMetadata{}, fmt.Errorf("all methods failed: http=%v, webpagesnap=%v, markdownnew=%v, defuddle=%v, jina=%v", httpErr, snapErr, mnErr, defErr, jinaErr)
}

// fetchStealthMode 浏览器优先，失败再降级
func (wf *WebContentFetcher) fetchStealthMode(targetURL string, maxChars int) (string, string, string, types.WebMetadata, error) {
	// 1. 浏览器抓取
	md, sel, htmlStr, browserErr := wf.withRetryBrowser(targetURL, maxChars)
	if browserErr == nil && extractor.IsValidContent(md, "text/html") {
		metadata := extractor.ExtractMetadata(htmlStr, targetURL)
		return md, "stealth", sel, metadata, nil
	}

	log.Printf("[fetcher] Stealth 失败，降级到 Firecrawl: %v", browserErr)

	// 2. Firecrawl
	if wf.firecrawl != nil {
		fcContent, fcSel, fcMeta, fcErr := wf.firecrawl.Fetch(targetURL, maxChars)
		if fcErr == nil && extractor.IsValidContent(fcContent, "text/html") {
			return fcContent, "firecrawl", fcSel, fcMeta, nil
		}
		if fcErr != nil {
			log.Printf("[fetcher] Firecrawl 失败，降级到 WebPageSnap: %v", fcErr)
		}
	}

	// 3. WebPageSnap
	snapContent, snapSel, snapMeta, snapErr := wf.webpagesnap.Fetch(targetURL, maxChars)
	if snapErr == nil && extractor.IsValidContent(snapContent, "text/html") {
		return snapContent, "webpagesnap", snapSel, snapMeta, nil
	}
	if snapErr != nil {
		log.Printf("[fetcher] WebPageSnap 失败，降级到 markdown.new: %v", snapErr)
	}

	// 4. markdown.new
	mnContent, mnSel, mnMeta, mnErr := wf.markdownnew.Fetch(targetURL, maxChars)
	if mnErr == nil && extractor.IsValidContent(mnContent, "text/html") {
		return mnContent, "markdown-new", mnSel, mnMeta, nil
	}
	if mnErr != nil {
		log.Printf("[fetcher] markdown.new 失败，降级到 Defuddle: %v", mnErr)
	}

	// 5. Defuddle
	defContent, defSel, defFM, defErr := wf.defuddle.Fetch(targetURL)
	if defErr == nil && extractor.IsValidContent(defContent, "text/html") {
		metadata := wf.buildMetadataFromFrontmatter(defFM, targetURL)
		return defContent, "defuddle", defSel, metadata, nil
	}

	// 6. Jina
	jinaContent, jinaSel, jinaErr := wf.jina.Fetch(targetURL)
	if jinaErr == nil && extractor.IsValidContent(jinaContent, "text/html") {
		return jinaContent, "jina", jinaSel, types.WebMetadata{URL: targetURL}, nil
	}
	if jinaErr != nil {
		log.Printf("[fetcher] Jina 失败，降级到 Brave Search: %v", jinaErr)
	}

	// 7. Brave Search 兜底
	if wf.bravesearch != nil {
		brContent, brSel, brMeta, brErr := wf.bravesearch.Fetch(targetURL, maxChars)
		if brErr == nil {
			return brContent, "brave-search", brSel, brMeta, nil
		}
		log.Printf("[fetcher] Brave Search 失败: %v", brErr)
	}

	return "", "", "", types.WebMetadata{}, fmt.Errorf("all methods failed: browser=%v, firecrawl=%v, webpagesnap=%v, markdownnew=%v, defuddle=%v, jina=%v", browserErr, "skipped", snapErr, mnErr, defErr, jinaErr)
}

// fetchTwitterMode FxTwitter → Browser → WebPageSnap → markdown.new → Defuddle → Jina → Brave Search
func (wf *WebContentFetcher) fetchTwitterMode(targetURL string, maxChars int) (string, string, string, types.WebMetadata, error) {
	// 1. FxTwitter（Twitter 专用 API，最快最准）
	fxContent, fxSel, fxMeta, fxErr := wf.fxtwitter.Fetch(targetURL, maxChars)
	if fxErr == nil && extractor.IsValidContent(fxContent, "application/json") {
		return fxContent, "fxtwitter", fxSel, fxMeta, nil
	}
	if fxErr != nil {
		log.Printf("[fetcher] FxTwitter 失败，降级到 Browser: %v", fxErr)
	} else {
		log.Printf("[fetcher] FxTwitter 内容过少(%d字符)，降级到 Browser", len(fxContent))
	}

	// 2. Browser（JS 渲染）
	if wf.browser != nil {
		brMd, brSel, brHTML, brErr := wf.withRetryBrowser(targetURL, maxChars)
		if brErr == nil && extractor.IsValidContent(brMd, "text/html") {
			metadata := extractor.ExtractMetadata(brHTML, targetURL)
			return brMd, "stealth", brSel, metadata, nil
		}
		if brErr != nil {
			log.Printf("[fetcher] Browser 失败，降级到 Firecrawl: %v", brErr)
		} else {
			log.Printf("[fetcher] Browser 内容过少(%d字符)，降级到 Firecrawl", len(brMd))
		}
	}

	// 3. Firecrawl（stealth 代理，绕 Cloudflare）
	if wf.firecrawl != nil {
		fcContent, fcSel, fcMeta, fcErr := wf.firecrawl.Fetch(targetURL, maxChars)
		if fcErr == nil && extractor.IsValidContent(fcContent, "text/html") {
			return fcContent, "firecrawl", fcSel, fcMeta, nil
		}
		if fcErr != nil {
			log.Printf("[fetcher] Firecrawl 失败，降级到 WebPageSnap: %v", fcErr)
		} else {
			log.Printf("[fetcher] Firecrawl 内容过少(%d字符)，降级到 WebPageSnap", len(fcContent))
		}
	}

	// 4. WebPageSnap
	snapContent, snapSel, snapMeta, snapErr := wf.webpagesnap.Fetch(targetURL, maxChars)
	if snapErr == nil && extractor.IsValidContent(snapContent, "text/html") {
		return snapContent, "webpagesnap", snapSel, snapMeta, nil
	}
	if snapErr != nil {
		log.Printf("[fetcher] WebPageSnap 失败，降级到 markdown.new: %v", snapErr)
	}

	// 5. markdown.new
	mnContent, mnSel, mnMeta, mnErr := wf.markdownnew.Fetch(targetURL, maxChars)
	if mnErr == nil && extractor.IsValidContent(mnContent, "text/html") {
		return mnContent, "markdown-new", mnSel, mnMeta, nil
	}
	if mnErr != nil {
		log.Printf("[fetcher] markdown.new 失败，降级到 Defuddle: %v", mnErr)
	}

	// 6. Defuddle
	defContent, defSel, defFM, defErr := wf.defuddle.Fetch(targetURL)
	if defErr == nil && extractor.IsValidContent(defContent, "text/html") {
		metadata := wf.buildMetadataFromFrontmatter(defFM, targetURL)
		return defContent, "defuddle", defSel, metadata, nil
	}
	if defErr != nil {
		log.Printf("[fetcher] Defuddle 失败，降级到 Jina: %v", defErr)
	}

	// 7. Jina
	jinaContent, jinaSel, jinaErr := wf.jina.Fetch(targetURL)
	if jinaErr == nil && extractor.IsValidContent(jinaContent, "text/html") {
		return jinaContent, "jina", jinaSel, types.WebMetadata{URL: targetURL}, nil
	}
	if jinaErr != nil {
		log.Printf("[fetcher] Jina 失败，降级到 Brave Search: %v", jinaErr)
	}

	// 8. Brave Search 兜底
	if wf.bravesearch != nil {
		brContent, brSel, brMeta, brErr := wf.bravesearch.Fetch(targetURL, maxChars)
		if brErr == nil {
			return brContent, "brave-search", brSel, brMeta, nil
		}
		log.Printf("[fetcher] Brave Search 失败: %v", brErr)
	}

	return "", "", "", types.WebMetadata{}, fmt.Errorf("all methods failed: fxtwitter=%v, browser=%v, webpagesnap=%v, markdownnew=%v, defuddle=%v, jina=%v", fxErr, "", snapErr, mnErr, defErr, jinaErr)
}

// buildMetadataFromFrontmatter 从 Defuddle/Jina 返回的 frontmatter map 构建 WebMetadata
func (wf *WebContentFetcher) buildMetadataFromFrontmatter(fm map[string]string, targetURL string) types.WebMetadata {
	meta := types.WebMetadata{URL: targetURL}
	if fm != nil {
		meta.Title = fm["title"]
		meta.Description = fm["description"]
		meta.Image = fm["image"]
		meta.Author = fm["author"]
		meta.PublishDate = fm["date"]
		meta.SiteName = fm["site_name"]
	}
	return meta
}

// withRetryBrowser 带重试的浏览器抓取
func (wf *WebContentFetcher) withRetryBrowser(targetURL string, maxChars int) (string, string, string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[fetcher] 第 %d 次 Browser 重试...", attempt)
			time.Sleep(300 * time.Millisecond)
		}
		md, sel, htmlStr, err := wf.browser.Fetch(targetURL, maxChars)
		if err == nil {
			return md, sel, htmlStr, nil
		}
		lastErr = err
	}
	return "", "", "", lastErr
}

// BatchFetch 批量获取（并发控制）
func (wf *WebContentFetcher) BatchFetch(urls []string, maxConcurrent int) []types.FetchResult {
	results := make([]types.FetchResult, len(urls))
	sem := make(chan struct{}, maxConcurrent)
	wg := sync.WaitGroup{}

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, urlStr string) {
			defer wg.Done()
			sem <- struct{}{}
			result := wf.Fetch(urlStr, 30000, false, false)
			results[idx] = result
			<-sem
		}(i, u)
	}

	wg.Wait()
	return results
}

// Close 清理资源
func (wf *WebContentFetcher) Close() {
	close(wf.stopClean)
	if wf.browser != nil {
		wf.browser.Close()
	}
	wf.cache.CleanupExpired()
}

// memCacheCleaner 定期清理过期的内存缓存条目
func (wf *WebContentFetcher) memCacheCleaner() {
	ticker := time.NewTicker(memCacheCleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			wf.cleanExpiredMemCache()
		case <-wf.stopClean:
			return
		}
	}
}

// cleanExpiredMemCache 清除过期的内存缓存
func (wf *WebContentFetcher) cleanExpiredMemCache() {
	ttl := time.Duration(wf.cache.TTLDays) * 24 * time.Hour
	var expiredKeys []string
	wf.memCache.Range(func(key, value interface{}) bool {
		entry, ok := value.(*memCacheEntry)
		if !ok || time.Since(entry.createdAt) > ttl {
			expiredKeys = append(expiredKeys, key.(string))
		}
		return true
	})
	wf.memCacheMu.Lock()
	for _, k := range expiredKeys {
		if _, loaded := wf.memCache.LoadAndDelete(k); loaded {
			wf.memCacheCount--
		}
	}
	// 重建 key 列表（去掉已删除的 key）
	newKeys := make([]string, 0, int(wf.memCacheCount))
	for _, k := range wf.memCacheKeys {
		if _, ok := wf.memCache.Load(k); ok {
			newKeys = append(newKeys, k)
		}
	}
	wf.memCacheKeys = newKeys
	wf.memCacheMu.Unlock()
	if len(expiredKeys) > 0 {
		log.Printf("[fetcher] 内存缓存清理: 删除 %d 条过期条目", len(expiredKeys))
	}
}
