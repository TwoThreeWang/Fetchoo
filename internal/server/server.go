package server

import (
	_ "embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"web_fetcher/internal/apicall"
	"web_fetcher/internal/fetcher"
	"web_fetcher/internal/types"
)

//go:embed landing.html
var landingPageHTML string

// SetupRouter 配置路由
func SetupRouter(f *fetcher.WebContentFetcher, store *apicall.Store, limiter *apicall.RateLimiter) *gin.Engine {
	r := gin.Default()

	// CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 限速和计数中间件
	r.Use(apicall.Middleware(limiter, store))

	// 全局异常处理
	r.Use(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, types.APIResponse{
					Code:    500,
					Message: recoverToString(err),
				})
			}
		}()
		c.Next()
	})

	api := r.Group("")
	{
		api.GET("/fetch", handleFetch(f))
		api.POST("/batch-fetch", handleBatchFetch(f))
	}

	// 首页落地页
	r.GET("/", handleLandingPage(store))

	return r
}

// handleLandingPage 返回首页落地页 HTML
func handleLandingPage(store *apicall.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		today := "0"
		total := "0"
		if store != nil {
			today = fmt.Sprintf("%d", store.GetTodayCount())
			total = fmt.Sprintf("%d", store.GetTotalCount())
		}
		html := landingPageHTML
		html = strings.Replace(html, "{{TODAY_CALLS}}", today, 1)
		html = strings.Replace(html, "{{TOTAL_CALLS}}", total, 1)
		c.String(http.StatusOK, html)
	}
}

// GET /fetch — 获取网页内容
func handleFetch(f *fetcher.WebContentFetcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		targetURL := c.Query("url")
		if targetURL == "" {
			c.JSON(http.StatusOK, types.APIResponse{Code: 400, Message: "缺少 url 参数"})
			return
		}

		stealth := c.DefaultQuery("stealth", "false") == "true"
		noCache := c.DefaultQuery("no_cache", "false") == "true"

		result := f.Fetch(targetURL, 30000, stealth, noCache)

		if !result.Success {
			c.JSON(http.StatusOK, types.APIResponse{
				Code:    500,
				Message: result.Error,
			})
			return
		}

		data := map[string]interface{}{
			"url":            result.URL,
			"title":          result.Title,
			"content":        result.Content,
			"mode":           result.Mode,
			"fetch_time":     round2(result.FetchTime),
			"content_length": result.ContentLength,
			"metadata":       result.Metadata,
		}
		c.JSON(http.StatusOK, types.APIResponse{Code: 200, Message: "success", Data: data})
	}
}

// POST /batch-fetch — 批量获取
func handleBatchFetch(f *fetcher.WebContentFetcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.BatchFetchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, types.APIResponse{Code: 400, Message: "请求参数错误: " + err.Error()})
			return
		}

		if len(req.URLs) > 20 {
			c.JSON(http.StatusOK, types.APIResponse{Code: 400, Message: "最多支持 20 个 URL"})
			return
		}

		maxConcurrent := req.MaxConcurrent
		if maxConcurrent <= 0 || maxConcurrent > 10 {
			maxConcurrent = 3
		}

		results := f.BatchFetch(req.URLs, maxConcurrent)

		data := make([]interface{}, len(results))
		for i, r := range results {
			if r.Success {
				data[i] = r
			} else {
				data[i] = types.FetchResultWithMeta{
					URL:     r.URL,
					Success: false,
					Error:   r.Error,
				}
			}
		}
		c.JSON(http.StatusOK, types.APIResponse{Code: 200, Message: "success", Data: data})
	}
}

func round2(f float64) float64 { return float64(int(f*100)) / 100 }

func recoverToString(err interface{}) string {
	if s, ok := err.(string); ok {
		return s
	}
	if e, ok := err.(error); ok {
		return e.Error()
	}
	return "未知错误"
}
