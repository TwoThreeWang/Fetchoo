package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"

	_ "modernc.org/sqlite"

	"web_fetcher/internal/apicall"
	"web_fetcher/internal/fetcher"
	"web_fetcher/internal/server"
)

func main() {
	var (
		port           string
		dbPath         string
		logDir         string
		rateLimit      float64
		useBrowser     bool
		disableBrowser bool
		proxy          string
	)

	flag.StringVar(&port, "port", "5000", "服务端口")
	flag.StringVar(&dbPath, "db", "./data/fetchoo.db", "数据库路径")
	flag.StringVar(&logDir, "log-dir", "./logs", "日志目录")
	rateLimitDefault := 5.0
	if v := os.Getenv("RATE_LIMIT"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			rateLimitDefault = n
		}
	}
	flag.Float64Var(&rateLimit, "rate-limit", rateLimitDefault, "入站 API 每秒最大请求数 (0 表示不限速)")
	flag.BoolVar(&useBrowser, "browser", false, "强制启用浏览器模式")
	flag.BoolVar(&disableBrowser, "no-browser", false, "禁用浏览器模式")
	flag.StringVar(&proxy, "proxy", "", "代理地址 (如 http://127.0.0.1:7890)")
	flag.Parse()

	gin.SetMode(gin.ReleaseMode)
	os.MkdirAll(logDir, 0755)

	// 初始化日志
	logFile, err := os.OpenFile(logDir+"/fetchoo.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("无法打开日志文件: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 初始化单实例数据库
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		os.MkdirAll(dir, 0755)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("无法连接数据库: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var browserFlag *bool
	if disableBrowser {
		b := false
		browserFlag = &b
	} else if useBrowser {
		b := true
		browserFlag = &b
	}

	braveAPIKey := os.Getenv("BRAVE_API_KEY")
	firecrawlAPIKey := os.Getenv("FIRECRAWL_API_KEY")

	f, err := fetcher.NewWebContentFetcher(db, browserFlag, proxy, braveAPIKey, firecrawlAPIKey)
	if err != nil {
		log.Fatalf("初始化 Fetcher 失败: %v", err)
	}

	// 初始化 API 调用次数存储
	callStore, err := apicall.NewStore(db)
	if err != nil {
		log.Fatalf("初始化调用统计数据库失败: %v", err)
	}
	defer callStore.Close()

	// 初始化入站限速器
	var inboundLimiter *apicall.RateLimiter
	if rateLimit > 0 {
		inboundLimiter = apicall.NewRateLimiter(rateLimit)
		log.Printf("[main] 启用了入站限速: %.2f QPS", rateLimit)
	}

	router := server.SetupRouter(f, callStore, inboundLimiter)

	go func() {
		addr := ":" + port
		log.Printf("✓ Web Content Fetcher 启动: http://0.0.0.0%s", addr)
		if runErr := router.Run(addr); runErr != nil {
			log.Fatalf("服务器启动失败: %v", runErr)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务...")
	f.Close()
	fmt.Println("服务已停止")
}
