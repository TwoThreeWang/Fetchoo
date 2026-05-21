package utils

import (
	"net/url"
	"regexp"
	"strings"
)

// ValidateURL 预校验 URL 合法性
func ValidateURL(rawURL string) (bool, string) {
	if rawURL == "" {
		return false, "URL 不能为空"
	}
	rawURL = strings.TrimSpace(rawURL)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, "URL 格式错误: " + err.Error()
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false, "不支持协议: " + parsed.Scheme + "（仅支持 http/https）"
	}
	if parsed.Host == "" {
		return false, "域名不能为空"
	}

	return true, ""
}

// whitespaceRe 用于 SanitizeText
var whitespaceRe = regexp.MustCompile(`\s+`)

// SanitizeText 清理文本中的多余空白
func SanitizeText(text string) string {
	if text == "" {
		return ""
	}
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(text, " "))
}

// DomainFromURL 从 URL 提取 domain
func DomainFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Truncate 截断字符串用于日志显示
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
