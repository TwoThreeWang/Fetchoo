package fetcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"web_fetcher/internal/extractor"
	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/utils"
)

// StealthBrowser 浏览器爬虫（Chromium 可选）
type StealthBrowser struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	extractor      *extractor.ContentExtractor
	ctx            context.Context
	cancel         context.CancelFunc
	allocCancel    context.CancelFunc
	ready          bool
}

// CheckChromiumAvailable 检测 Chromium 是否可用
func CheckChromiumAvailable() bool {
	chromiumPaths := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/snap/bin/chromium",
	}

	for _, p := range chromiumPaths {
		if _, err := os.Stat(p); err == nil {
			log.Printf("[browser] ✓ 找到 Chromium: %s", p)
			return true
		}
	}

	pwHome := filepath.Join(os.Getenv("HOME"), ".cache", "ms-playwright")
	if entries, err := os.ReadDir(pwHome); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "chromium-") {
				log.Printf("[browser] ✓ 找到 Playwright Chromium: %s", entry.Name())
				return true
			}
		}
	}

	log.Println("[browser] ⚠ 未检测到已安装的 Chromium，chromedp 将尝试自动下载")
	return true
}

func NewStealthBrowser(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *StealthBrowser {
	return &StealthBrowser{
		headersManager: hm,
		rateLimiter:    rl,
		extractor:      extractor.NewContentExtractor(),
	}
}

func (sb *StealthBrowser) ensureBrowser() error {
	if sb.ready {
		return nil
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("headless", "new"),
		chromedp.UserAgent(sb.headersManager.GetHeaders("")["User-Agent"]),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	sb.ctx, sb.cancel = context.WithCancel(allocCtx)
	// 记住 allocCancel，Close 时先 cancel 子 ctx 再 cancel allocator
	sb.allocCancel = allocCancel

	testCtx, testCancel := chromedp.NewContext(sb.ctx, chromedp.WithLogf(func(string, ...interface{}) {}))
	defer testCancel()

	if err := chromedp.Run(testCtx, chromedp.Navigate("about:blank")); err != nil {
		return fmt.Errorf("浏览器启动失败: %s\n请确保容器内已安装 Chromium 或允许网络下载", err)
	}

	sb.ready = true
	log.Println("[browser] ✓ Chromium 启动成功")
	return nil
}

// Fetch 浏览器抓取，返回 (markdown, selector, rawHTML)
func (sb *StealthBrowser) Fetch(targetURL string, maxChars int) (string, string, string, error) {
	sb.rateLimiter.Wait(targetURL)

	if err := sb.ensureBrowser(); err != nil {
		return "", "", "", err
	}

	ctx, cancel := chromedp.NewContext(sb.ctx, chromedp.WithLogf(func(string, ...interface{}) {}))
	defer cancel()

	ctx, timeoutCancel := context.WithTimeout(ctx, 30*time.Second)
	defer timeoutCancel()

	var htmlStr string

	err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &htmlStr),
	)

	if err != nil {
		return "", "", "", fmt.Errorf("浏览器抓取失败: %w", err)
	}

	md, selector := sb.extractor.Extract(htmlStr, targetURL)
	log.Printf("[stealth] ✓ %s | %s | %d chars", utils.Truncate(targetURL, 50), selector, len(md))

	return md, selector, htmlStr, nil
}

// Close 关闭浏览器
func (sb *StealthBrowser) Close() {
	if sb.cancel != nil {
		sb.cancel()
	}
	if sb.allocCancel != nil {
		sb.allocCancel()
	}
	sb.ready = false
}
