package fetcher

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"web_fetcher/internal/headers"
	"web_fetcher/internal/ratelimit"
	"web_fetcher/internal/utils"
)

const jinaAPIURL = "https://r.jina.ai"

// JinaReader Jina Reader — https://r.jina.ai/{url}
type JinaReader struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	client         *http.Client
}

func NewJinaReader(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *JinaReader {
	return &JinaReader{
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

// Fetch 使用 Jina Reader 抓取，返回 (content, selector)
func (jr *JinaReader) Fetch(targetURL string) (string, string, error) {
	jr.rateLimiter.Wait(targetURL)
	jinaURL := fmt.Sprintf("%s/%s", jinaAPIURL, targetURL)
	h := jr.headersManager.GetHeaders(targetURL)

	req, err := http.NewRequest("GET", jinaURL, nil)
	if err != nil {
		return "", "", err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}

	resp, err := jr.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("Jina HTTP %d", resp.StatusCode)
	}

	text, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 最多 10MB
	if err != nil {
		return "", "", err
	}

	result := strings.TrimSpace(string(text))
	re := regexp.MustCompile(`\n{3,}`)
	result = re.ReplaceAllString(result, "\n\n")

	log.Printf("[jina] ✓ %s | %d chars", utils.Truncate(targetURL, 50), len(result))

	return result, "jina-reader", nil
}
