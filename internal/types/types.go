package types

// WebMetadata 网页元信息
type WebMetadata struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Author      string `json:"author"`
	PublishDate string `json:"publish_date"`
	SiteName    string `json:"site_name"`
}

// FetchResult 抓取结果
type FetchResult struct {
	URL             string      `json:"url"`
	Title           string      `json:"title"`
	Content         string      `json:"content"`
	Metadata        WebMetadata `json:"metadata"`
	Mode            string      `json:"mode"`
	FetchTime       float64     `json:"fetch_time"`
	ContentLength   int         `json:"content_length"`
	SelectorMatched string      `json:"selector_matched,omitempty"`
	Success         bool        `json:"success"`
	Error           string      `json:"error,omitempty"`
}

// FetchResultWithMeta 用于 batch 接口的失败结果
type FetchResultWithMeta struct {
	URL     string `json:"url"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// APIResponse 统一 API 响应格式
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// BatchFetchRequest 批量请求体
type BatchFetchRequest struct {
	URLs          []string `json:"urls" binding:"required"`
	MaxConcurrent int      `json:"max_concurrent"`
}
