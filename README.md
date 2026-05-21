# Fetchoo 🦝

**Fetch the web, cleanly.**

> Web content, ready for AI.

Go 编写的网页内容抓取服务，支持串行降级策略，自动选择最快抓取路径，为 AI 应用提供结构化 JSON 数据。

---

## 特性

- **串行降级抓取**: HTTP → Browser → WebPageSnap → markdown.new → Defuddle → Jina，智能选择最快路径，失败自动降级
- **7 种抓取策略**: HTTP、Browser（Chromium）、WebPageSnap、markdown.new、Defuddle、Jina、FxTwitter（Twitter 专用）
- **智能域名处理**: Twitter/X 链接自动走 FxTwitter，微信/知乎等强反爬站点自动启用 Browser
- **双级缓存**: 内存 LRU（500 条上限） + SQLite 持久化，TTL 过期自动清理
- **速率限制**: 全局 + 域名级双维度限速，内置微信/知乎/CSDN 等站点延迟配置
- **反爬措施**: 随机 UA 池、Referer 伪装、Sec-Fetch 头
- **安全限制**: HTTP 响应 10MB 上限，防止内存溢出
- **结构化输出**: JSON 格式，包含 title/content/metadata/mode/fetch_time 等字段

---

## 项目结构

```
go_web_fetcher/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口
├── internal/
│   ├── cache/
│   │   └── cache.go             # SQLite 持久化缓存
│   ├── extractor/
│   │   └── extractor.go         # HTML→Markdown 转换、元数据提取
│   ├── fetcher/
│   │   ├── fetcher.go           # 核心编排（串行降级策略）
│   │   ├── http_fetcher.go      # HTTP 直接抓取
│   │   ├── browser.go           # Chromium 浏览器抓取（chromedp）
│   │   ├── defuddle.go          # Defuddle.md 服务
│   │   ├── jina.go              # Jina Reader 服务
│   │   ├── webpagesnap.go       # WebPageSnap 服务
│   │   ├── markdownnew.go       # markdown.new 服务
│   │   └── fxtwitter.go         # FxTwitter API（Twitter 专用）
│   ├── headers/
│   │   └── headers.go           # 请求头管理（UA 池、反爬头）
│   ├── ratelimit/
│   │   └── ratelimit.go         # 智能速率限制器
│   ├── server/
│   │   └── server.go            # Gin HTTP Server + 落地页
│   ├── types/
│   │   └── types.go             # 公共类型定义
│   └── utils/
│       └── utils.go             # 工具函数（URL 校验、文本清理）
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

---

## 本地开发

```bash
cd go_web_fetcher

# 安装依赖
go mod tidy

# 运行（需要系统安装 Chromium 或允许 chromedp 自动下载）
go run ./cmd/server -port=8000

# 命令行参数
-port=8000                  服务端口（默认 8000）
-cache-db=/app/data/fetch_cache.db    缓存数据库路径
-browser                    强制启用浏览器模式
-no-browser                 禁用浏览器模式
-proxy=http://...           代理地址
```

---

## Docker 部署

```bash
cd go_web_fetcher

# 构建并启动
docker compose up -d --build

# 查看日志
docker compose logs -f

# 访问服务
curl "http://localhost:8000/fetch?url=https://example.com"
```

---

## API 接口

### 首页

访问 `http://localhost:8000/` 查看落地页。

### GET /fetch

获取网页内容，返回 JSON 结构化数据。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| url | string | 必填 | 目标网址 |
| stealth | bool | false | 强制使用浏览器模式 |
| no_cache | bool | false | 跳过缓存直接抓取 |

**响应格式:**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "url": "https://canonical-url.example.com/",
    "title": "页面标题",
    "content": "# Markdown 正文...",
    "mode": "http",
    "fetch_time": 1.23,
    "content_length": 12345,
    "selector_matched": "article",
    "metadata": {
      "url": "...",
      "title": "...",
      "description": "...",
      "image": "...",
      "author": "...",
      "publish_date": "...",
      "site_name": "..."
    }
  }
}
```

### POST /batch-fetch

批量获取网页内容（最多 10 并发）。

**请求体:**

```json
{
  "urls": ["https://a.com", "https://b.com", "https://c.com"],
  "max_concurrent": 3
}
```

**响应:** 与 `/fetch` 相同结构，返回数组。

---

## 抓取模式说明

| mode | 含义 |
|------|------|
| `cached` | 命中缓存（内存或 SQLite） |
| `http` | HTTP 直接抓取成功 |
| `stealth` | Chromium 浏览器抓取 |
| `webpagesnap` | WebPageSnap 服务抓取 |
| `markdown-new` | markdown.new 服务抓取 |
| `defuddle` | Defuddle.md 服务兜底 |
| `jina` | Jina Reader 最终兜底 |
| `fxtwitter` | FxTwitter API（Twitter/X 专用） |
| `invalid` | URL 校验失败 |
| `failed` | 所有方式均失败 |

---

## 降级策略详情

### 普通域名

```
HTTP → Browser → WebPageSnap → markdown.new → Defuddle → Jina
```

- HTTP 失败不重试，直接进入下一级
- 内容少于 200 字符视为失败，触发降级

### Stealth 域名（微信/知乎/掘金/头条）

```
Browser → WebPageSnap → markdown.new → Defuddle → Jina
```

- 跳过 HTTP，这些站点已知 HTTP 拿不到有效内容

### Twitter/X 域名

```
FxTwitter → Browser → WebPageSnap → markdown.new → Defuddle → Jina
```

- FxTwitter 第一级优先，专为 Twitter/X 优化
- 支持推文、线程、媒体、互动数提取

---

## 缓存机制

- **内存缓存**: LRU 淘汰，上限 500 条，每 5 分钟清理过期条目
- **SQLite 缓存**: 持久化存储，默认 TTL 7 天
- **缓存键**: URL SHA256 前 16 位

---

## 速率限制

- **全局**: 默认 5 RPS，滑动窗口控制
- **域名级**: 内置延迟配置
  - `mp.weixin.qq.com`: 2.0-3.5s
  - `zhuanlan.zhihu.com`: 1.5-2.5s
  - `juejin.cn`: 1.0-1.8s
  - `csdn.net`: 0.5-1.0s
  - 其他默认: 0.5-1.2s

---

## 依赖服务

| 服务 | 用途 | 免费额度 |
|------|------|----------|
| [WebPageSnap](https://webpagesnap.com) | 网页快照抓取 | - |
| [markdown.new](https://markdown.new) | URL→Markdown 转换 | 500 次/天/IP |
| [Defuddle](https://defuddle.md) | 内容清洗 | - |
| [Jina Reader](https://r.jina.ai) | 通用内容提取 | 免费 |
| [FxTwitter](https://api.fxtwitter.com) | Twitter/X 专用 | 免费 |

---

## License

MIT
