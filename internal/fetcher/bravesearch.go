package fetcher

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/types"
)

const braveSearchAPI = "https://api.search.brave.com/res/v1/web/search"

// BraveSearchFetcher Brave Search API 抓取器
type BraveSearchFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	apiKey         string
	count          int
	client         *http.Client
}

// NewBraveSearchFetcher 创建 Brave Search 抓取器
func NewBraveSearchFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter, apiKey string) *BraveSearchFetcher {
	if apiKey == "" {
		return nil
	}
	return &BraveSearchFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		apiKey:         apiKey,
		count:          5,
		client:         &http.Client{Timeout: 15 * time.Second},
	}
}

// Fetch 用目标 URL 作为查询词搜索 Brave Search，返回第一条结果的标题+摘要
func (bs *BraveSearchFetcher) Fetch(targetURL string, maxChars int) (string, string, types.WebMetadata, error) {
	if bs == nil || bs.apiKey == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("Brave Search API Key 未配置")
	}

	bs.rateLimiter.Wait("brave-search")

	query := url.QueryEscape(targetURL)
	reqURL := fmt.Sprintf("%s?q=%s&count=%d", braveSearchAPI, query, bs.count)

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", bs.apiKey)

	resp, err := bs.client.Do(req)
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", "", types.WebMetadata{}, fmt.Errorf("Brave Search HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result braveSearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&result); err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("解析 Brave Search 响应失败: %w", err)
	}

	if len(result.Web.Results) == 0 {
		return "", "", types.WebMetadata{}, fmt.Errorf("Brave Search 无结果")
	}

	first := result.Web.Results[0]
	content := first.Title + "\n\n" + first.Description
	if len(content) > maxChars && maxChars > 0 {
		content = content[:maxChars]
	}

	meta := types.WebMetadata{
		URL:         first.URL,
		Title:       first.Title,
		Description: first.Description,
	}
	if first.MetaURL != nil {
		meta.SiteName = first.MetaURL.Hostname
	}

	return content, "", meta, nil
}

// braveSearchResponse Brave Search API 响应结构
type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			MetaURL     *struct {
				Hostname string `json:"hostname"`
			} `json:"meta_url,omitempty"`
		} `json:"results"`
	} `json:"web"`
}
