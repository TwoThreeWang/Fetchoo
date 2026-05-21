package fetcher

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/types"
	"web_fetcher/internal/utils"
)

const markdownNewAPIURL = "https://markdown.new"

// MarkdownNewFetcher markdown.new 抓取器
type MarkdownNewFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	client         *http.Client
}

// NewMarkdownNewFetcher 创建 MarkdownNewFetcher 实例
func NewMarkdownNewFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *MarkdownNewFetcher {
	return &MarkdownNewFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Fetch 使用 markdown.new 抓取，返回 (content, selector, metadata)
func (mn *MarkdownNewFetcher) Fetch(targetURL string, maxChars int) (string, string, types.WebMetadata, error) {
	mn.rateLimiter.Wait(targetURL)
	apiURL := fmt.Sprintf("%s/%s", markdownNewAPIURL, targetURL)
	h := mn.headersManager.GetHeaders(targetURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}
	// markdown.new 推荐加 Accept: text/markdown
	req.Header.Set("Accept", "text/markdown")

	resp, err := mn.client.Do(req)
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("markdown.new 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", types.WebMetadata{}, fmt.Errorf("markdown.new HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("markdown.new 读取响应失败: %w", err)
	}

	content := strings.TrimSpace(string(bodyBytes))
	if len(content) > maxChars && maxChars > 0 {
		content = content[:maxChars]
	}

	// markdown.new 返回的 markdown 通常带有 frontmatter（--- title: ... ---）
	body, fm := ParseFrontmatter(content)

	meta := types.WebMetadata{
		URL:         targetURL,
		Title:       fm["title"],
		Description: fm["description"],
		SiteName:    utils.DomainFromURL(targetURL),
	}

	log.Printf("[markdown.new] ✓ %s | %d chars", utils.Truncate(targetURL, 50), len(body))

	return body, "markdown-new", meta, nil
}
