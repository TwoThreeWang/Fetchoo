package fetcher

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"

	"web_fetcher/internal/extractor"
	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/utils"
)

// StealthBrowser 浏览器爬虫（rod + stealth + Headless Chromium）
type StealthBrowser struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	extractor      *extractor.ContentExtractor

	launcher *launcher.Launcher
	browser  *rod.Browser

	initOnce sync.Once
	initErr  error
	ready    bool
	mu       sync.Mutex
}

// chromiumCandidatePaths 常见的 Chromium 可执行文件路径
var chromiumCandidatePaths = []string{
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/google-chrome",
	"/usr/bin/google-chrome-stable",
	"/snap/bin/chromium",
}

// findChromiumBin 返回首个存在的 Chromium 可执行文件路径
func findChromiumBin() string {
	// 优先环境变量
	for _, env := range []string{"CHROMIUM_PATH", "CHROME_PATH", "ROD_BROWSER_BIN"} {
		if p := os.Getenv(env); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, p := range chromiumCandidatePaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// CheckChromiumAvailable 检测 Chromium 是否可用
func CheckChromiumAvailable() bool {
	if bin := findChromiumBin(); bin != "" {
		log.Printf("[browser] ✓ 找到 Chromium: %s", bin)
		return true
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

	log.Println("[browser] ⚠ 未检测到已安装的 Chromium，rod 将尝试自动下载")
	return true
}

// NewStealthBrowser 创建浏览器抓取器（懒加载，首次 Fetch 时才启动 Chromium）
func NewStealthBrowser(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *StealthBrowser {
	return &StealthBrowser{
		headersManager: hm,
		rateLimiter:    rl,
		extractor:      extractor.NewContentExtractor(),
	}
}

// ensureBrowser 懒加载启动浏览器
func (sb *StealthBrowser) ensureBrowser() error {
	sb.initOnce.Do(func() {
		sb.initErr = sb.startBrowser()
	})
	if sb.initErr != nil {
		return sb.initErr
	}
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if !sb.ready {
		return fmt.Errorf("浏览器未就绪")
	}
	return nil
}

// startBrowser 实际启动 Chromium
func (sb *StealthBrowser) startBrowser() error {
	l := launcher.New().
		Headless(true).
		NoSandbox(true).
		Set("disable-setuid-sandbox").
		Set("disable-dev-shm-usage").
		Set("disable-gpu").
		Set("disable-blink-features", "AutomationControlled").
		Set("disable-features", "IsolateOrigins,site-per-process").
		Set("disable-infobars").
		Set("disable-extensions").
		Set("disable-background-networking").
		Set("disable-background-timer-throttling").
		Set("disable-renderer-backgrounding").
		Set("disable-backgrounding-occluded-windows").
		Set("disable-client-side-phishing-detection").
		Set("disable-default-apps").
		Set("disable-hang-monitor").
		Set("disable-popup-blocking").
		Set("disable-prompt-on-repost").
		Set("disable-sync").
		Set("disable-translate").
		Set("metrics-recording-only").
		Set("no-first-run").
		Set("safebrowsing-disable-auto-update").
		Set("password-store", "basic").
		Set("use-mock-keychain").
		Set("lang", "zh-CN,zh,en").
		UserDataDir(filepath.Join(os.TempDir(), "go_web_fetcher_rod"))

	if bin := findChromiumBin(); bin != "" {
		l = l.Bin(bin)
	}

	url, err := l.Launch()
	if err != nil {
		return fmt.Errorf("启动 Chromium 失败: %w（请确保已安装 Chromium 或允许 rod 下载）", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return fmt.Errorf("连接 Chromium 失败: %w", err)
	}

	sb.mu.Lock()
	sb.launcher = l
	sb.browser = browser
	sb.ready = true
	sb.mu.Unlock()

	log.Println("[browser] ✓ rod + stealth + Headless Chromium 启动成功")
	return nil
}

// Fetch 浏览器抓取，返回 (markdown, selector, rawHTML, error)
func (sb *StealthBrowser) Fetch(targetURL string, maxChars int) (string, string, string, error) {
	sb.rateLimiter.Wait(targetURL)

	if err := sb.ensureBrowser(); err != nil {
		return "", "", "", err
	}

	// 通过 stealth.Page 创建反检测页面
	page, err := stealth.Page(sb.browser)
	if err != nil {
		return "", "", "", fmt.Errorf("创建 stealth 页面失败: %w", err)
	}
	defer func() {
		_ = page.Close()
	}()

	// 30 秒超时
	page = page.Timeout(30 * time.Second)

	// 设置 UA 及额外请求头
	hdrs := sb.headersManager.GetHeaders(targetURL)
	ua := hdrs["User-Agent"]
	if ua != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent:      ua,
			AcceptLanguage: hdrs["Accept-Language"],
		}); err != nil {
			log.Printf("[browser] 设置 UA 失败: %v", err)
		}
	}
	if len(hdrs) > 0 {
		var pairs []string
		for k, v := range hdrs {
			if k == "User-Agent" {
				continue
			}
			pairs = append(pairs, k, v)
		}
		if len(pairs) > 0 {
			if _, err := page.SetExtraHeaders(pairs); err != nil {
				log.Printf("[browser] 设置请求头失败: %v", err)
			}
		}
	}

	// 导航
	if err := page.Navigate(targetURL); err != nil {
		return "", "", "", fmt.Errorf("浏览器导航失败: %w", err)
	}

	// 等待页面加载完成
	if err := page.WaitLoad(); err != nil {
		return "", "", "", fmt.Errorf("浏览器等待加载失败: %w", err)
	}

	// 等待网络空闲一段时间，让 JS 渲染完成
	_ = page.WaitDOMStable(500*time.Millisecond, 0.05)
	page.MustWaitIdle()

	// 额外固定等待，确保动态内容渲染
	time.Sleep(1 * time.Second)

	htmlStr, err := page.HTML()
	if err != nil {
		return "", "", "", fmt.Errorf("获取页面 HTML 失败: %w", err)
	}

	md, selector := sb.extractor.Extract(htmlStr, targetURL)
	log.Printf("[stealth] ✓ %s | %s | %d chars", utils.Truncate(targetURL, 50), selector, len(md))

	return md, selector, htmlStr, nil
}

// Close 关闭浏览器
func (sb *StealthBrowser) Close() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.browser != nil {
		_ = sb.browser.Close()
		sb.browser = nil
	}
	if sb.launcher != nil {
		sb.launcher.Cleanup()
		sb.launcher = nil
	}
	sb.ready = false
}
