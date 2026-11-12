---
title: "源码走读 S21：outbox.go"
date: 2026-11-14 23:03:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/outbox/outbox.go`　**行数**：47 行
> **职责**：本地消息表写入：在业务事务里写事件记录

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
// Package outbox 本地消息表的写入。
//
// 只做一件事：在业务事务里写一条 outbox 记录。
// 真正的「发 MQ」由 internal/mq 的后台 worker 做。
//
// 独立成包是为了打破循环依赖：
//
//	chatservice → outbox → mqmodel
//	           ↘ mqmodel
//
// 如果 chatservice 直接 import internal/mq，而 mq 又要 import ws/chatservice 推送，
// 就会循环依赖。
package outbox

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"gochat/global"
	"gochat/internal/model/mq_model"
)

// Enabled 是否启用了 MQ（未启用时业务走同步推送）。
func Enabled() bool {
	return global.MQ != nil && global.MQ.Enabled
}

// Enqueue 在给定事务里写一条 outbox 记录。
//
// 【必须在业务事务内调用】
// 这样「业务数据 + outbox 记录」要么都成功、要么都回滚。
func Enqueue(tx *gorm.DB, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rec := &mqmodel.Outbox{
		EventType: eventType,
		Payload:   string(b),
		Status:    0,
		NextRetry: time.Now(),
	}
	return tx.Create(rec).Error
}

```
