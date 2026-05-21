package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"web_fetcher/internal/fetcher"
	"web_fetcher/internal/server"
)

func main() {
	var (
		port           string
		cacheDB        string
		useBrowser     bool
		disableBrowser bool
		proxy          string
	)

	flag.StringVar(&port, "port", "8000", "服务端口")
	flag.StringVar(&cacheDB, "cache-db", "/app/data/fetch_cache.db", "缓存数据库路径")
	flag.BoolVar(&useBrowser, "browser", false, "强制启用浏览器模式")
	flag.BoolVar(&disableBrowser, "no-browser", false, "禁用浏览器模式")
	flag.StringVar(&proxy, "proxy", "", "代理地址 (如 http://127.0.0.1:7890)")
	flag.Parse()

	gin.SetMode(gin.ReleaseMode)
	logDir := "/app/logs"
	os.MkdirAll(logDir, 0755)

	// 初始化日志（同时写文件和控制台）
	logFile, err := os.OpenFile(logDir+"/web_fetcher.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("无法打开日志文件: %v", err)
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var browserFlag *bool
	if disableBrowser {
		b := false
		browserFlag = &b
	} else if useBrowser {
		b := true
		browserFlag = &b
	}

	f, err := fetcher.NewWebContentFetcher(cacheDB, browserFlag, proxy)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	defer f.Close()

	router := server.SetupRouter(f)

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
