package core

import (
	"flag"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"

	"gochat/core/db"
	"gochat/core/jwt"
	"gochat/core/mq"
	"gochat/core/server"
	"gochat/global"
)

type FlagConfig struct {
	ConfigPath string
	InitDB     bool
}

var flagConfig = new(FlagConfig)

// LoadConfigFile 解析命令行参数。
func LoadConfigFile() {
	flag.StringVar(&flagConfig.ConfigPath, "config", "./config.yaml", "配置文件路径（默认 ./config.yaml）")
	flag.BoolVar(&flagConfig.InitDB, "db", false, "是否初始化数据库表（AutoMigrate）")
	flag.Parse()
	global.ConfigPath = &flagConfig.ConfigPath
}

// LoadConfig 读取并解析 YAML 配置。
func LoadConfig() (bool, error) {
	bytes, err := os.ReadFile(*global.ConfigPath)
	if err != nil {
		return false, fmt.Errorf("config 加载失败: %w", err)
	}
	if err := yaml.Unmarshal(bytes, &global.Config); err != nil {
		return false, fmt.Errorf("yaml 配置解析错误: %w", err)
	}
	return true, nil
}

// InitBaseServices 初始化所有基础服务。
//
// 【M4 改进：返回 error 而不是 os.Exit】
// 以前用 os.Exit(0) 会：① 退出码是 0（表示成功）掩盖故障；② 跳过 defer。
// 现在把错误往上抛，由 main 决定怎么处理（log.Fatal）。
func InitBaseServices() error {
	if _, err := db.LoadMysql(); err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	if flagConfig.InitDB {
		if _, err := db.InitMysqlTables(); err != nil {
			return fmt.Errorf("数据库建表失败: %w", err)
		}
	}
	if _, err := db.LoadRedis(); err != nil {
		return fmt.Errorf("初始化 Redis 失败: %w", err)
	}
	mq.LoadMq()
	jwt.LoadJWT()
	server.LoadServer()
	return nil
}

// CloseBaseServices 释放基础服务资源（优雅退出时调用）。
func CloseBaseServices() {
	if global.DB != nil {
		if sqlDB, err := global.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	if global.Redis != nil {
		_ = global.Redis.Close()
	}
}
