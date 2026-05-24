package fetcher

import (
	"bytes"
	"encoding/json"
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

const firecrawlAPIURL = "https://api.firecrawl.dev/v1/scrape"

// FirecrawlFetcher Firecrawl 抓取器（自带 stealth 代理，绕 Cloudflare）
type FirecrawlFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	apiKey         string
	client         *http.Client
}

// NewFirecrawlFetcher 创建 Firecrawl 实例
func NewFirecrawlFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter, apiKey string) *FirecrawlFetcher {
	if apiKey == "" {
		return nil
	}
	return &FirecrawlFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		apiKey:         apiKey,
		client:         &http.Client{Timeout: 25 * time.Second},
	}
}

// Fetch 使用 Firecrawl 抓取，返回 (markdown, selector, metadata, error)
func (fc *FirecrawlFetcher) Fetch(targetURL string, maxChars int) (string, string, types.WebMetadata, error) {
	if fc == nil || fc.apiKey == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("Firecrawl API Key 未配置")
	}

	fc.rateLimiter.Wait("firecrawl")

	reqBody, _ := json.Marshal(map[string]interface{}{
		"url": targetURL,
		"formats": []string{"markdown"},
	})

	req, err := http.NewRequest("POST", firecrawlAPIURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+fc.apiKey)

	resp, err := fc.client.Do(req)
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("firecrawl 请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("firecrawl 读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", types.WebMetadata{}, fmt.Errorf("firecrawl HTTP %d: %s", resp.StatusCode, utils.Truncate(string(bodyBytes), 200))
	}

	var result firecrawlResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("firecrawl 解析 JSON 失败: %w", err)
	}

	if !result.Success || result.Data.Markdown == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("firecrawl 返回失败: success=%v", result.Success)
	}

	content := strings.TrimSpace(result.Data.Markdown)
	if len(content) > maxChars && maxChars > 0 {
		content = content[:maxChars]
	}

	// 构建 metadata
	meta := types.WebMetadata{
		URL:         targetURL,
		Title:       result.Data.Metadata.Title,
		Description: result.Data.Metadata.Description,
		Image:       result.Data.Metadata.OGImage,
		Author:      result.Data.Metadata.Author,
		SiteName:    utils.DomainFromURL(targetURL),
	}
	if result.Data.Metadata.OGTitle != "" && meta.Title == "" {
		meta.Title = result.Data.Metadata.OGTitle
	}
	if result.Data.Metadata.OGDescription != "" && meta.Description == "" {
		meta.Description = result.Data.Metadata.OGDescription
	}
	if result.Data.Metadata.SourceURL != "" {
		meta.URL = result.Data.Metadata.SourceURL
	}

	log.Printf("[firecrawl] ✓ %s | %d chars", utils.Truncate(targetURL, 50), len(content))

	return content, "firecrawl", meta, nil
}

// firecrawlResponse Firecrawl API 响应结构
type firecrawlResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Markdown string `json:"markdown"`
		Metadata struct {
			Title        string `json:"title"`
			Description  string `json:"description"`
			OGTitle      string `json:"ogTitle"`
			OGDescription string `json:"ogDescription"`
			OGImage      string `json:"ogImage"`
			Author       string `json:"author"`
			SourceURL    string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
}
