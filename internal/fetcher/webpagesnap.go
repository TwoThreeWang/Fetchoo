package fetcher

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"web_fetcher/internal/extractor"
	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/types"
	"web_fetcher/internal/utils"
)

const webpagesnapAPIURL = "https://webpagesnap.com/api/scrape"

// WebPageSnapResponse webpagesnap API 返回结构
type WebPageSnapResponse struct {
	Success  bool              `json:"success"`
	URL      string            `json:"url"`
	FinalURL string            `json:"finalUrl"`
	Header   WebPageSnapHeader `json:"header"`
	Body     string            `json:"body"`
}

// WebPageSnapHeader webpagesnap 返回的页面头部信息
type WebPageSnapHeader struct {
	Title            string `json:"title"`
	Description      string `json:"description"`
	Keywords         string `json:"keywords"`
	Author           string `json:"author"`
	Charset          string `json:"charset"`
	OGTitle          string `json:"ogTitle"`
	OGDescription    string `json:"ogDescription"`
	OGImage          string `json:"ogImage"`
	OGURL            string `json:"ogUrl"`
	TwitterCard      string `json:"twitterCard"`
	TwitterTitle     string `json:"twitterTitle"`
	TwitterDescription string `json:"twitterDescription"`
	TwitterImage     string `json:"twitterImage"`
}

// WebPageSnapFetcher webpagesnap 抓取器
type WebPageSnapFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	extractor      *extractor.ContentExtractor
	client         *http.Client
}

// NewWebPageSnapFetcher 创建 WebPageSnap 实例
func NewWebPageSnapFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *WebPageSnapFetcher {
	return &WebPageSnapFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		extractor:      extractor.NewContentExtractor(),
		client: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Fetch 使用 webpagesnap 抓取，返回 (content, selector, metadata)
func (ws *WebPageSnapFetcher) Fetch(targetURL string, maxChars int) (string, string, types.WebMetadata, error) {
	ws.rateLimiter.Wait(targetURL)
	apiURL := fmt.Sprintf("%s?url=%s", webpagesnapAPIURL, targetURL)
	h := ws.headersManager.GetHeaders(targetURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}

	resp, err := ws.client.Do(req)
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap 读取响应失败: %w", err)
	}

	// 处理 gzip 压缩响应（Accept-Encoding 显式设置后 Go 不会自动解压）
	if len(bodyBytes) >= 2 && bodyBytes[0] == 0x1f && bodyBytes[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(bodyBytes))
		if err != nil {
			return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap gzip 解压失败: %w", err)
		}
		defer gr.Close()
		bodyBytes, err = io.ReadAll(gr)
		if err != nil {
			return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap gzip 读取失败: %w", err)
		}
	}

	var snapResp WebPageSnapResponse
	if err := json.Unmarshal(bodyBytes, &snapResp); err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap 解析 JSON 失败: %w", err)
	}

	if !snapResp.Success || snapResp.Body == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("webpagesnap 返回失败: success=%v, body 为空", snapResp.Success)
	}

	content := snapResp.Body
	content = strings.TrimSpace(content)
	// 压缩连续换行
	if len(content) > maxChars && maxChars > 0 {
		content = content[:maxChars]
	}

	// 构建 metadata
	hdr := snapResp.Header
	title := hdr.OGTitle
	if title == "" {
		title = hdr.Title
	}
	desc := hdr.OGDescription
	if desc == "" {
		desc = hdr.Description
	}
	image := hdr.OGImage
	if image == "" {
		image = hdr.TwitterImage
	}

	finalURL := snapResp.FinalURL
	if finalURL == "" {
		finalURL = targetURL
	}

	meta := types.WebMetadata{
		URL:         finalURL,
		Title:       utils.SanitizeText(title),
		Description: utils.SanitizeText(desc),
		Image:       image,
		Author:      hdr.Author,
		SiteName:    utils.DomainFromURL(targetURL),
	}

	log.Printf("[webpagesnap] ✓ %s | %d chars", utils.Truncate(targetURL, 50), len(content))

	return content, "webpagesnap", meta, nil
}
