package fetcher

import (
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

const fxTwitterAPIURL = "https://api.fxtwitter.com"

// FxTwitterAPIResponse FxTwitter API 返回结构
type FxTwitterAPIResponse struct {
	Code int       `json:"code"`
	Tweet FxTweet  `json:"tweet"`
}

type FxTweet struct {
	URL         string        `json:"url"`
	ID          string        `json:"id"`
	Text        string        `json:"text"`
	Author      FxAuthor      `json:"author"`
	Media       FxMedia       `json:"media"`
	CreatedAt   string        `json:"created_at"`
	Likes       int           `json:"likes"`
	Retweets    int           `json:"retweets"`
	Replies     int           `json:"replies"`
}

type FxAuthor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ScreenName  string `json:"screen_name"`
	AvatarURL   string `json:"avatar_url"`
}

type FxMedia struct {
	Photos []FxPhoto `json:"photos"`
	Videos []FxVideo `json:"videos"`
}

type FxPhoto struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type FxVideo struct {
	URL        string `json:"url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Thumbnail  string `json:"thumbnail_url"`
}

// FxTwitterFetcher FxTwitter API 抓取器
type FxTwitterFetcher struct {
	headersManager *headers.HeadersManager
	rateLimiter    *ratelimit.RateLimiter
	client         *http.Client
}

// NewFxTwitterFetcher 创建 FxTwitterFetcher 实例
func NewFxTwitterFetcher(hm *headers.HeadersManager, rl *ratelimit.RateLimiter) *FxTwitterFetcher {
	return &FxTwitterFetcher{
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

// Fetch 使用 FxTwitter API 抓取推文，返回 (content, selector, metadata)
func (fx *FxTwitterFetcher) Fetch(targetURL string, maxChars int) (string, string, types.WebMetadata, error) {
	fx.rateLimiter.Wait(targetURL)

	// 将 twitter.com/x.com 的 URL 转换为 FxTwitter API URL
	apiURL := toFxTwitterAPIURL(targetURL)
	if apiURL == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("无法识别 Twitter/X URL 格式: %s", targetURL)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", types.WebMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := fx.client.Do(req)
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("fxtwitter 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", types.WebMetadata{}, fmt.Errorf("fxtwitter HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("fxtwitter 读取响应失败: %w", err)
	}

	var apiResp FxTwitterAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return "", "", types.WebMetadata{}, fmt.Errorf("fxtwitter 解析 JSON 失败: %w", err)
	}

	if apiResp.Code != 200 || apiResp.Tweet.Text == "" {
		return "", "", types.WebMetadata{}, fmt.Errorf("fxtwitter 返回失败: code=%d", apiResp.Code)
	}

	tweet := apiResp.Tweet
	content := formatTweetToMarkdown(tweet, maxChars)

	meta := types.WebMetadata{
		URL:         tweet.URL,
		Title:       tweet.Author.Name + " (@" + tweet.Author.ScreenName + ")",
		Description: tweet.Text,
		Image:       tweet.Author.AvatarURL,
		Author:      tweet.Author.Name,
		SiteName:    "X (Twitter)",
	}
	if len(tweet.Media.Photos) > 0 {
		meta.Image = tweet.Media.Photos[0].URL
	}

	log.Printf("[fxtwitter] ✓ %s | %d chars", utils.Truncate(targetURL, 50), len(content))

	return content, "fxtwitter", meta, nil
}

// toFxTwitterAPIURL 将 twitter.com/x.com 链接转为 api.fxtwitter.com 格式
func toFxTwitterAPIURL(rawURL string) string {
	// 支持格式:
	// https://twitter.com/username/status/123456
	// https://x.com/username/status/123456
	// https://fxtwitter.com/username/status/123456
	// https://fixupx.com/username/status/123456
	rawURL = strings.TrimSpace(rawURL)
	for _, prefix := range []string{
		"https://twitter.com/",
		"https://x.com/",
		"https://fxtwitter.com/",
		"https://fixupx.com/",
	} {
		if strings.HasPrefix(rawURL, prefix) {
			path := strings.TrimPrefix(rawURL, prefix)
			// 移除可能的查询参数
			if idx := strings.Index(path, "?"); idx != -1 {
				path = path[:idx]
			}
			return "https://api.fxtwitter.com/" + path
		}
	}
	return ""
}

// formatTweetToMarkdown 将推文格式化为 Markdown
func formatTweetToMarkdown(tweet FxTweet, maxChars int) string {
	var sb strings.Builder
	
	sb.WriteString("## ")
	sb.WriteString(tweet.Author.Name)
	sb.WriteString(" (@")
	sb.WriteString(tweet.Author.ScreenName)
	sb.WriteString(")\n\n")
	sb.WriteString(tweet.Text)
	sb.WriteString("\n\n")

	if len(tweet.Media.Photos) > 0 {
		for _, p := range tweet.Media.Photos {
			sb.WriteString("![]")
			sb.WriteString("(")
			sb.WriteString(p.URL)
			sb.WriteString(")\n")
		}
	}

	if len(tweet.Media.Videos) > 0 {
		for _, v := range tweet.Media.Videos {
			sb.WriteString("[视频](")
			sb.WriteString(v.URL)
			sb.WriteString(")\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\n👍 %d  🔄 %d  💬 %d\n", tweet.Likes, tweet.Retweets, tweet.Replies))
	sb.WriteString(fmt.Sprintf("\n[查看原推文](%s)\n", tweet.URL))

	result := sb.String()
	if maxChars > 0 && len(result) > maxChars {
		result = result[:maxChars]
	}
	return result
}
