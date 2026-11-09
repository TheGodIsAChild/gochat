package main

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"

	"gochat/internal/config"
	"gochat/internal/database"
	"gochat/internal/httpapi"
	"gochat/internal/model"
)

func main() {
	// -config 指定配置文件路径，默认读当前目录的 config.yaml。
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 1) 加载配置：失败直接退出。
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 2) 初始化数据库连接。
	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	// 3) 自动建表 / 迁移（开发期使用；生产环境应改用 migration 工具）。
	if err := db.AutoMigrate(&model.User{}, &model.FriendRequest{}, &model.Friendship{}); err != nil {
		log.Fatalf("自动迁移失败: %v", err)
	}
	log.Printf("数据库自动迁移完成")

	// 4) 设置 gin 运行模式（必须在创建引擎之前）。
	gin.SetMode(cfg.Server.Mode)

	// 5) 构建路由并启动 HTTP 服务。
	r := httpapi.NewRouter()

	log.Printf("gochat 启动，监听 %s", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
