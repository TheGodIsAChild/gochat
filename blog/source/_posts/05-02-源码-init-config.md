---
title: "源码走读 S02：init_config.go"
date: 2026-11-14 23:20:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`core/init_config.go`　**行数**：84 行
> **职责**：配置加载：命令行参数、读 YAML、初始化/关闭所有基础服务

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package core

import (
	"flag"
	"fmt"
	"log"
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
	// RabbitMQ 是可选依赖：连不上时降级为同步推送，不影响应用启动
	if err := mq.LoadMq(); err != nil {
		log.Printf("RabbitMQ 连接失败，消息将使用同步推送: %v", err)
	}
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
	// 关闭 MQ 连接
	mq.CloseMq()
}

```
