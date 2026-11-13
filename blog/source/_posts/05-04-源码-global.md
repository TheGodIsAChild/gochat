---
title: "源码走读 S04：global.go"
date: 2026-11-14 23:18:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`global/global.go`　**行数**：29 行
> **职责**：全局状态：Config / DB / Redis / MQ 的位置

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package global

import (
	"gochat/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ConfigPath *string
var Config config.Config
var DB *gorm.DB // 全局 GORM 连接
var Redis *redis.Client

// MQClient RabbitMQ 客户端（M7）。
//
// 【为什么要有 Enabled 字段】
// RabbitMQ 是可选依赖：连不上时应用要能降级运行（消息改为同步推送），
// 而不是直接启动失败。所以用 Enabled 标记是否真正可用。
type MQClient struct {
	Conn      *amqp.Connection
	Channel   *amqp.Channel
	Enabled   bool
	QueueName string
}

var MQ *MQClient

```
