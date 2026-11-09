package main

// ⚠️ 参考实现：建议你先自己写一遍，卡住再对照本文件。

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"

	"gochat/internal/config"
	"gochat/internal/httpapi"
)

func main() {
	// -config 指定配置文件路径，默认读当前目录的 config.yaml。
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 1) 加载配置：失败直接退出，不要带着空配置继续跑。
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 2) 设置 gin 运行模式（必须在创建引擎/注册路由之前）。
	gin.SetMode(cfg.Server.Mode)

	// 3) 构建路由并启动 HTTP 服务。
	r := httpapi.NewRouter()

	log.Printf("gochat 启动，监听 %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
