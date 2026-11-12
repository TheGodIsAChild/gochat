package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gochat/core"
	"gochat/global"
	"gochat/internal/router"
	"gochat/internal/ws"
)

// main 程序入口。
//
// 【M4 改进：优雅退出】
// 以前直接 r.Run()，进程被 kill 时：
//   - 处理中的请求被硬切断
//   - WebSocket 连接没有 Close 帧，客户端要等超时才感知
//   - DB/Redis 连接没释放
//
// 现在：
//  1. 用 http.Server 启动
//  2. 监听 SIGINT/SIGTERM
//  3. 收到信号后：停止接收新请求 → 等待处理中的请求（最多 10s）→ 断开 WS → 释放 DB/Redis
func main() {
	core.LoadConfigFile()

	if _, err := core.LoadConfig(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := core.InitBaseServices(); err != nil {
		log.Fatalf("初始化基础服务失败: %v", err)
	}

	srv := &http.Server{
		Addr:    global.Config.Server.Addr,
		Handler: router.NewRouter(),
	}

	// HTTP 服务放到 goroutine 里跑，主 goroutine 去等退出信号
	go func() {
		log.Printf("HTTP 服务启动: %s", global.Config.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常: %v", err)
		}
	}()

	// 等待退出信号（Ctrl+C / kill）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，开始优雅关闭...")

	// 1. 停止接收新请求，等待处理中的请求完成（最多 10 秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP 优雅关闭失败: %v", err)
	}

	// 2. 主动断开所有 WebSocket 连接（让客户端立即感知，触发重连）
	ws.DefaultHub.CloseAll()

	// 3. 释放 DB / Redis
	core.CloseBaseServices()

	log.Println("已退出")
}
