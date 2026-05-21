package fetcher

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"web_fetcher/internal/extractor"
	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/utils"
)

// HTTPFetcher HTTP 爬虫 — 返回 (markdown, selector, rawHTML)
type HTTPFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	extractor      *extractor.ContentExtractor
	client         *http.Client
}

func NewHTTPFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *HTTPFetcher {
	return &HTTPFetcher{
		headersManager: hm,
		rateLimiter:    rl,
		extractor:      extractor.NewContentExtractor(),
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Fetch 快速 HTTP 爬取，返回 (markdown, selector, rawHTML, error)
func (hf *HTTPFetcher) Fetch(targetURL string, maxChars int) (string, string, string, error) {
	hf.rateLimiter.Wait(targetURL)
	h := hf.headersManager.GetHeaders(targetURL)

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", "", "", err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}

	resp, err := hf.client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		_, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 最多读 1MB 后丢弃
		return "", "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body

	// 处理 gzip 压缩
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gr, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			reader = resp.Body
		} else {
			defer gr.Close()
			reader = gr
		}
	case "br", "deflate":
		reader = resp.Body
	}

	bodyBytes, readErr := io.ReadAll(io.LimitReader(reader, 10<<20)) // 最多 10MB
	if readErr != nil {
		return "", "", "", fmt.Errorf("读取响应失败: %w", readErr)
	}

	htmlStr := string(bodyBytes)

	if !isValidUTF8(htmlStr) {
		htmlStr = tryDecodeGBK(bodyBytes)
	}

	md, selector := hf.extractor.Extract(htmlStr, targetURL)
	log.Printf("[http] ✓ %s | %s | %d chars", utils.Truncate(targetURL, 50), selector, len(md))

	return md, selector, htmlStr, nil
}

// isValidUTF8 检测字符串是否为有效 UTF-8
func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

// tryDecodeGBK 尝试 GBK 解码（简单实现）
func tryDecodeGBK(b []byte) string {
	runes := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		if b[i] < 0x80 {
			runes = append(runes, rune(b[i]))
			i++
		} else if i+1 < len(b) {
			codePoint := uint16(b[i])<<8 | uint16(b[i+1])
			runes = append(runes, rune(codePoint))
			i += 2
		} else {
			runes = append(runes, '\uFFFD')
			i++
		}
	}
	return string(runes)
}
