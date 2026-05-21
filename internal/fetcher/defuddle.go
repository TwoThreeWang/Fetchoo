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

const defuddleAPIURL = "https://defuddle.md"

// DefuddleReader Defuddle — https://defuddle.md/{url}
type DefuddleReader struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	client         *http.Client
}

func NewDefuddleReader(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *DefuddleReader {
	return &DefuddleReader{
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

// Fetch 使用 Defuddle 抓取，返回 (content, selector, frontmatterMeta)
func (dr *DefuddleReader) Fetch(targetURL string) (string, string, map[string]string, error) {
	dr.rateLimiter.Wait(targetURL)
	defuddleURL := fmt.Sprintf("%s/%s", defuddleAPIURL, targetURL)
	h := dr.headersManager.GetHeaders(targetURL)

	req, err := http.NewRequest("GET", defuddleURL, nil)
	if err != nil {
		return "", "", nil, err
	}
	for k, v := range h {
		req.Header.Set(k, v)
	}

	resp, err := dr.client.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", nil, fmt.Errorf("Defuddle HTTP %d", resp.StatusCode)
	}

	rawText, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 最多 10MB
	if err != nil {
		return "", "", nil, err
	}

	body, frontmatter := ParseFrontmatter(string(rawText))

	re := regexp.MustCompile(`\n{3,}`)
	body = re.ReplaceAllString(body, "\n\n")
	body = strings.TrimSpace(body)

	log.Printf("[defuddle] ✓ %s | %d chars | meta=%v", utils.Truncate(targetURL, 50), len(body), len(frontmatter) > 0)

	return body, "defuddle-reader", frontmatter, nil
}

// ParseFrontmatter 分离 YAML front-matter 和正文内容
func ParseFrontmatter(text string) (string, map[string]string) {
	if !strings.HasPrefix(text, "---\n") {
		return text, nil
	}

	endIdx := strings.Index(text[4:], "\n---")
	if endIdx == -1 {
		return text, nil
	}
	yamlPart := text[4 : 4+endIdx]
	body := text[4+endIdx+4:]
	body = strings.TrimSpace(body)

	meta := make(map[string]string)
	for _, line := range strings.Split(yamlPart, "\n") {
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, "\"'")
		meta[key] = val
	}

	return body, meta
}
