package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"web_fetcher/internal/fetcher"
	"web_fetcher/internal/types"
)

// SetupRouter 配置路由
func SetupRouter(f *fetcher.WebContentFetcher) *gin.Engine {
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

	// 全局异常处理
	r.Use(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				c.JSON(http.StatusOK, types.APIResponse{
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
	r.GET("/", handleLandingPage())

	return r
}

// handleLandingPage 返回首页落地页 HTML
func handleLandingPage() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, landingPageHTML)
	}
}

const landingPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Fetchoo - Fetch the web, cleanly.</title>
	<meta name="description" content="Fetchoo 将任意网页转换为结构化 JSON，为 AI 应用提供干净、可用的内容。支持串行降级抓取、多源聚合和智能缓存。">
	
	<!-- Open Graph -->
	<meta property="og:title" content="Fetchoo - Fetch the web, cleanly.">
	<meta property="og:description" content="Web content, ready for AI. 多源降级抓取，JSON 结构化输出，智能缓存。">
	<meta property="og:type" content="website">
	<meta property="og:url" content="https://fetchoo.dev">
	<meta property="og:image" content="https://fetchoo.dev/og-image.png">
	
	<!-- Twitter Card -->
	<meta name="twitter:card" content="summary_large_image">
	<meta name="twitter:title" content="Fetchoo - Fetch the web, cleanly.">
	<meta name="twitter:description" content="Web content, ready for AI. 多源降级抓取，JSON 结构化输出，智能缓存。">
	<meta name="twitter:image" content="https://fetchoo.dev/og-image.png">
	
	<!-- Favicon & Theme -->
	<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🦝</text></svg>">
	<meta name="theme-color" content="#0B0F19">
	
	<!-- Fonts -->
	<link rel="preconnect" href="https://fonts.googleapis.com">
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
	<link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
	
	<!-- Tailwind CDN -->
	<script src="https://cdn.tailwindcss.com"></script>
	<script>
		tailwind.config = {
			theme: {
				extend: {
					colors: {
						bg: '#0B0F19',
						surface: '#111827',
						accent: '#D97706',
						'text-primary': '#F3F4F6',
						'text-secondary': '#9CA3AF',
						border: '#1F2937',
					},
					fontFamily: {
						'display': ['Space Grotesk', 'sans-serif'],
						'body': ['Inter', 'sans-serif'],
						'mono': ['JetBrains Mono', 'monospace'],
					}
				}
			}
		}
	</script>
	
	<!-- JSON-LD Structured Data -->
	<script type="application/ld+json">
	{
		"@context": "https://schema.org",
		"@type": "SoftwareApplication",
		"name": "Fetchoo",
		"description": "Web content fetching service for AI applications",
		"applicationCategory": "DeveloperApplication",
		"offers": {
			"@type": "Offer",
			"price": "0",
			"priceCurrency": "USD"
		}
	}
	</script>
	
	<style>
		body {
			background: linear-gradient(180deg, #0B0F19 0%, #0F1623 100%);
			min-height: 100vh;
		}
		.code-window {
			background: #0D1117;
			border: 1px solid #30363D;
			border-radius: 12px;
			overflow: hidden;
		}
		.code-header {
			background: #161B22;
			border-bottom: 1px solid #30363D;
			padding: 12px 16px;
			display: flex;
			align-items: center;
			gap: 8px;
		}
		.dot { width: 12px; height: 12px; border-radius: 50%; }
		.dot-red { background: #FF5F56; }
		.dot-yellow { background: #FFBD2E; }
		.dot-green { background: #27CA40; }
		.glow {
			position: absolute;
			width: 600px;
			height: 600px;
			background: radial-gradient(circle, rgba(217, 119, 6, 0.08) 0%, transparent 70%);
			pointer-events: none;
		}
	</style>
</head>
<body class="text-text-primary font-body antialiased">
	<div class="glow top-0 left-1/2 -translate-x-1/2 -translate-y-1/2"></div>
	
	<!-- Hero Section -->
	<section class="relative px-6 py-20 lg:py-32">
		<div class="max-w-5xl mx-auto text-center">
			<div class="mb-6">
				<span class="text-6xl lg:text-7xl">🦝</span>
			</div>
			<h1 class="font-display text-5xl lg:text-7xl font-bold tracking-tight mb-6">
				Fetchoo
			</h1>
			<p class="text-text-secondary text-xl lg:text-2xl mb-4 font-display">
				Fetch the web, cleanly.
			</p>
			<p class="text-text-secondary text-lg mb-12 max-w-2xl mx-auto">
				Web content, ready for AI. 多源降级抓取，JSON 结构化输出，智能缓存。
			</p>
			<div class="flex flex-wrap justify-center gap-4">
				<a href="#api" class="bg-accent hover:bg-amber-600 text-white font-medium px-8 py-3 rounded-lg transition-colors inline-flex items-center gap-2">
					<span>Try API</span>
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path></svg>
				</a>
				<a href="https://github.com" class="bg-surface hover:bg-gray-800 text-text-primary font-medium px-8 py-3 rounded-lg border border-border transition-colors inline-flex items-center gap-2">
					<svg class="w-5 h-5" fill="currentColor" viewBox="0 0 24 24"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>
					<span>GitHub</span>
				</a>
			</div>
		</div>
	</section>
	
	<!-- Features Section -->
	<section class="px-6 py-16">
		<div class="max-w-5xl mx-auto">
			<div class="grid md:grid-cols-3 gap-6">
				<div class="bg-surface border border-border rounded-xl p-6 hover:border-accent/50 transition-colors">
					<div class="w-10 h-10 bg-accent/10 rounded-lg flex items-center justify-center mb-4">
						<svg class="w-5 h-5 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
					</div>
					<h3 class="font-display font-semibold text-lg mb-2">串行降级抓取</h3>
					<p class="text-text-secondary text-sm">HTTP → Browser → WebPageSnap → markdown.new → Defuddle → Jina，智能选择最快路径，失败自动降级。</p>
				</div>
				<div class="bg-surface border border-border rounded-xl p-6 hover:border-accent/50 transition-colors">
					<div class="w-10 h-10 bg-accent/10 rounded-lg flex items-center justify-center mb-4">
						<svg class="w-5 h-5 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
					</div>
					<h3 class="font-display font-semibold text-lg mb-2">多源聚合</h3>
					<p class="text-text-secondary text-sm">7 种抓取策略：HTTP、Browser、WebPageSnap、markdown.new、Defuddle、Jina、FxTwitter（Twitter 专用）。</p>
				</div>
				<div class="bg-surface border border-border rounded-xl p-6 hover:border-accent/50 transition-colors">
					<div class="w-10 h-10 bg-accent/10 rounded-lg flex items-center justify-center mb-4">
						<svg class="w-5 h-5 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
					</div>
					<h3 class="font-display font-semibold text-lg mb-2">智能缓存</h3>
					<p class="text-text-secondary text-sm">内存 LRU + SQLite 双级缓存，TTL 过期自动清理，重复请求毫秒级响应。</p>
				</div>
			</div>
		</div>
	</section>
	
	<!-- API Demo Section -->
	<section id="api" class="px-6 py-16">
		<div class="max-w-3xl mx-auto">
			<h2 class="font-display text-2xl font-semibold mb-6 text-center">API 演示</h2>
			<div class="code-window">
				<div class="code-header">
					<div class="dot dot-red"></div>
					<div class="dot dot-yellow"></div>
					<div class="dot dot-green"></div>
					<span class="text-text-secondary text-sm ml-2 font-mono">GET /fetch?url=...</span>
				</div>
				<div class="p-4 font-mono text-sm overflow-x-auto">
					<div class="text-text-secondary mb-2">// Request</div>
					<div class="text-green-400">curl</div>
					<div class="text-text-primary ml-4">"http://localhost:8000/fetch?url=https://example.com"</div>
					
					<div class="text-text-secondary mt-4 mb-2">// Response</div>
					<pre class="text-text-primary">{
  "code": 200,
  "message": "success",
  "data": {
    "url": "https://example.com",
    "title": "Example Domain",
    "content": "This domain is for use in...",
    "mode": "http",
    "fetch_time": 0.23,
    "content_length": 1256,
    "metadata": {
      "title": "Example Domain",
      "description": "...",
      "image": "..."
    }
  }
}</pre>
				</div>
			</div>
			
			<div class="mt-6 grid md:grid-cols-2 gap-4 text-sm">
				<div class="bg-surface border border-border rounded-lg p-4">
					<div class="text-accent font-mono mb-1">GET /fetch</div>
					<div class="text-text-secondary">单页抓取</div>
					<div class="text-text-secondary mt-2">?url= 必需 | ?stealth= | ?no_cache=</div>
				</div>
				<div class="bg-surface border border-border rounded-lg p-4">
					<div class="text-accent font-mono mb-1">POST /batch-fetch</div>
					<div class="text-text-secondary">批量抓取（最多 10 并发）</div>
					<div class="text-text-secondary mt-2">Body: {urls: [], max_concurrent: 3}</div>
				</div>
			</div>
		</div>
	</section>
	
	<!-- Data Sources Section -->
	<section class="px-6 py-16">
		<div class="max-w-5xl mx-auto">
			<h2 class="font-display text-2xl font-semibold mb-8 text-center">支持的抓取源</h2>
			<div class="flex flex-wrap justify-center gap-3">
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-green-500 rounded-full"></span>
					<span class="text-sm font-medium">HTTP</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-blue-500 rounded-full"></span>
					<span class="text-sm font-medium">Browser</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-purple-500 rounded-full"></span>
					<span class="text-sm font-medium">WebPageSnap</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-pink-500 rounded-full"></span>
					<span class="text-sm font-medium">markdown.new</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-indigo-500 rounded-full"></span>
					<span class="text-sm font-medium">Defuddle</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-teal-500 rounded-full"></span>
					<span class="text-sm font-medium">Jina</span>
				</div>
				<div class="flex items-center gap-2 bg-surface border border-border rounded-full px-4 py-2">
					<span class="w-2 h-2 bg-sky-500 rounded-full"></span>
					<span class="text-sm font-medium">FxTwitter</span>
				</div>
			</div>
			<p class="text-text-secondary text-center text-sm mt-6">Twitter/X 链接自动使用 FxTwitter 优先抓取</p>
		</div>
	</section>
	
	<!-- Footer -->
	<footer class="px-6 py-12 border-t border-border">
		<div class="max-w-5xl mx-auto flex flex-col md:flex-row justify-between items-center gap-4">
			<div class="flex items-center gap-2">
				<span class="text-2xl">🦝</span>
				<span class="font-display font-semibold">Fetchoo</span>
			</div>
			<div class="text-text-secondary text-sm">
				© 2026 Fetchoo. MIT License.
			</div>
			<a href="https://github.com" class="text-text-secondary hover:text-text-primary text-sm transition-colors">
				GitHub →
			</a>
		</div>
	</footer>
</body>
</html>`

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
