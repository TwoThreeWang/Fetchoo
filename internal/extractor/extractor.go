package extractor

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"web_fetcher/internal/types"
	"web_fetcher/internal/utils"
)

const (
	MinContentLength = 100
	maxChars         = 30000
)

var (
	reLazyImages  = regexp.Compile(`<img([^>]*?)\sdata-src="([^"]+)"([^>]*?)>`)
	reMultiNewline = regexp.Compile(`\n{3,}`)
	reStripTags   = regexp.Compile(`<[^>]+>`)
)

var (
	wechatSelectors = []string{
		"div#js_content",
		"div.rich_media_content",
	}
	commonSelectors = []string{
		"article",
		"main",
		"[role='article']",
		"[itemprop='articleBody']",
		".post-content",
		".entry-content",
		".article-content",
		".article-body",
		".article-detail",
		".article-holder",
		".post_body",
		".markdown-body",
		".Post-RichText",
		"#article_content",
		".article-area",
		".ssa-article",
	}
)

// ContentExtractor 内容提取器
type ContentExtractor struct{}

func NewContentExtractor() *ContentExtractor {
	return &ContentExtractor{}
}

// IsValidContent 根据内容类型判断内容是否有效
// contentType: HTTP Content-Type 头值（可为空）
// content: 实际内容
func IsValidContent(content string, contentType string) bool {
	// 如果没有 Content-Type，按通用规则判断
	if contentType == "" {
		return len(strings.TrimSpace(content)) >= MinContentLength
	}

	contentType = strings.ToLower(contentType)

	// JSON / XML 类型：只要非空即可
	if strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "application/xml") ||
		strings.Contains(contentType, "text/xml") {
		return len(strings.TrimSpace(content)) > 0
	}

	// 纯文本类型：宽松一些，3 字符以上
	if strings.Contains(contentType, "text/plain") ||
		strings.Contains(contentType, "text/csv") {
		return len(strings.TrimSpace(content)) >= 3
	}

	// 二进制/媒体类型（图片、视频、PDF 等）：不需要字符检查
	if strings.Contains(contentType, "image/") ||
		strings.Contains(contentType, "video/") ||
		strings.Contains(contentType, "audio/") ||
		strings.Contains(contentType, "application/pdf") ||
		strings.Contains(contentType, "application/octet-stream") {
		return len(content) > 0
	}

	// 默认 HTML/其他：需要 100 字符
	return len(content) >= MinContentLength
}

// FixLazyImages 修复懒加载图片
func FixLazyImages(html string) string {
	return reLazyImages.ReplaceAllStringFunc(html, func(match string) string {
		sub := reLazyImages.FindStringSubmatch(match)
		if len(sub) >= 3 {
			return `<img` + sub[1] + ` src="` + sub[2] + `"` + sub[3] + `>`
		}
		return match
	})
}

// HTMLToMarkdown HTML 转 Markdown（轻量实现）
func (ce *ContentExtractor) HTMLToMarkdown(htmlStr string, max int) string {
	htmlStr = FixLazyImages(htmlStr)
	md := htmlToMD(htmlStr)
	// 压缩连续换行
	md = reMultiNewline.ReplaceAllString(md, "\n\n")
	md = strings.TrimSpace(md)
	if max > 0 && len(md) > max {
		return md[:max]
	}
	return md
}

func htmlToMD(s string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(s))
	if err != nil {
		return reStripTags.ReplaceAllString(s, "")
	}
	var buf bytes.Buffer
	walkNode(&buf, doc.Selection)
	return buf.String()
}

func walkNode(buf *bytes.Buffer, sel *goquery.Selection) {
	node := sel.Get(0)
	if node == nil {
		buf.WriteString(sel.Text())
		return
	}

	switch node.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(node.Data[1] - '0')
		buf.WriteString(strings.Repeat("#", level))
		buf.WriteString(" ")
		buf.WriteString(sel.Text())
		buf.WriteString("\n\n")
	case "p", "div":
		childrenText := ""
		sel.Contents().Each(func(i int, s *goquery.Selection) {
			if s.Is("br") || s.Is("hr") {
				childrenText += "\n"
			} else if s.Length() == 0 {
				childrenText += s.Text()
			} else {
				var childBuf bytes.Buffer
				walkNode(&childBuf, s)
				childrenText += childBuf.String()
			}
		})
		buf.WriteString(childrenText)
		if !isInlineElement(node.Data) {
			buf.WriteString("\n\n")
		}
	case "br":
		buf.WriteString("\n")
	case "hr":
		buf.WriteString("\n---\n")
	case "strong", "b":
		buf.WriteString("**")
		buf.WriteString(sel.Text())
		buf.WriteString("**")
	case "em", "i":
		buf.WriteString("*")
		buf.WriteString(sel.Text())
		buf.WriteString("*")
	case "a":
		href, _ := sel.Attr("href")
		text := sel.Text()
		if href != "" && text != "" {
			buf.WriteString("[")
			buf.WriteString(text)
			buf.WriteString("](")
			buf.WriteString(href)
			buf.WriteString(")")
		} else {
			buf.WriteString(text)
		}
	case "img":
		src, _ := sel.Attr("src")
		alt, _ := sel.Attr("alt")
		if src != "" {
			buf.WriteString("![")
			buf.WriteString(alt)
			buf.WriteString("](")
			buf.WriteString(src)
			buf.WriteString(")")
		}
	case "ul":
		sel.Children().Each(func(_ int, s *goquery.Selection) {
			if s.Is("li") {
				buf.WriteString("- ")
				walkNode(buf, s)
				buf.WriteString("\n")
			}
		})
		buf.WriteString("\n")
	case "ol":
		idx := 1
		sel.Children().Each(func(_ int, s *goquery.Selection) {
			if s.Is("li") {
				buf.WriteString(fmt.Sprintf("%d. ", idx))
				walkNode(buf, s)
				buf.WriteString("\n")
				idx++
			}
		})
		buf.WriteString("\n")
	case "li":
		sel.Contents().Each(func(_ int, s *goquery.Selection) {
			walkNode(buf, s)
		})
	case "code":
		buf.WriteString("`")
		buf.WriteString(sel.Text())
		buf.WriteString("`")
	case "pre":
		buf.WriteString("```\n")
		buf.WriteString(sel.Text())
		buf.WriteString("\n```\n\n")
	case "blockquote":
		lines := strings.Split(sel.Text(), "\n")
		for _, line := range lines {
			if line != "" {
				buf.WriteString("> ")
				buf.WriteString(line)
			}
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	default:
		sel.Contents().Each(func(_ int, s *goquery.Selection) {
			walkNode(buf, s)
		})
	}
}

func isInlineElement(tag string) bool {
	inlineTags := map[string]bool{
		"strong": true, "b": true, "em": true, "i": true,
		"a": true, "span": true, "code": true, "img": true,
		"br": true,
	}
	return inlineTags[tag]
}

// Extract 从 HTML 提取文章内容，返回 (markdown, selectorName)
func (ce *ContentExtractor) Extract(htmlStr, targetURL string) (string, string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return ce.HTMLToMarkdown(htmlStr, maxChars), "body(fallback)"
	}

	isWechat := strings.Contains(targetURL, "mp.weixin.qq.com")

	var selectors []string
	if isWechat {
		selectors = append(wechatSelectors, commonSelectors...)
	} else {
		selectors = commonSelectors
	}

	for _, sel := range selectors {
		node := doc.Find(sel).First()
		if node.Length() == 0 {
			continue
		}
		html, _ := node.Html()
		if html == "" {
			continue
		}
		md := ce.HTMLToMarkdown(html, maxChars)
		if len(md) >= MinContentLength {
			return md, sel
		}
	}

	// fallback: 全文转换
	return ce.HTMLToMarkdown(htmlStr, maxChars), "body(fallback)"
}

// ExtractMetadata 从 HTML 提取元信息（canonical URL 优先）
func ExtractMetadata(htmlStr, defaultURL string) types.WebMetadata {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(htmlStr)))
	if err != nil {
		return types.WebMetadata{URL: defaultURL}
	}

	// canonical URL
	canonicalURL := ""
	doc.Find("link[rel='canonical']").Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("href"); ok && canonicalURL == "" {
			canonicalURL = v
		}
	})
	finalURL := canonicalURL
	if finalURL == "" {
		finalURL = defaultURL
	}

	ogTitle := ogProp(doc, "og:title")
	title := ogTitle
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").Text())
	}

	meta := types.WebMetadata{
		URL:         finalURL,
		Title:       utils.SanitizeText(title),
		Description: utils.SanitizeText(ogOrName(doc, "og:description", "description")),
		Image:       ogProp(doc, "og:image"),
		Author:      ogProp(doc, "article:author"),
		PublishDate: ogProp(doc, "article:published_time"),
		SiteName:    utils.SanitizeText(ogOrName(doc, "og:site_name", "")),
	}
	if meta.SiteName == "" {
		meta.SiteName = utils.DomainFromURL(defaultURL)
	}

	return meta
}

func ogProp(doc *goquery.Document, prop string) string {
	var result string
	doc.Find(`meta[property="`+prop+`"]`).Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("content"); ok && result == "" {
			result = v
		}
	})
	return result
}

func ogOrName(doc *goquery.Document, ogKey, nameProp string) string {
	if v := ogProp(doc, ogKey); v != "" {
		return v
	}
	var result string
	doc.Find(`meta[name="`+nameProp+`"]`).Each(func(_ int, s *goquery.Selection) {
		if v, ok := s.Attr("content"); ok && result == "" {
			result = v
		}
	})
	return result
}
