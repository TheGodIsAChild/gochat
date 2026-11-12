---
title: "源码走读 S17：handler.go"
date: 2026-11-12 17:44:00
categories:
  - 源码走读
tags:
  - 源码
---
> **文件路径**：`internal/ws/handler.go`　**行数**：67 行
> **职责**：WebSocket 握手：token 鉴权 → 升级连接 → 启动读写 goroutine

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package ws

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"gochat/global"
	"gochat/internal/auth"
)

// upgrader 把 HTTP 连接升级成 WebSocket。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 【注意点】生产环境必须校验 Origin，否则任意网站都能连你的 WebSocket
	// （CSWSH 跨站 WebSocket 劫持）。这里学习项目放开，方便本地调试。
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeWS 处理 WebSocket 握手。
//
// 【鉴权方式】
// 浏览器 WebSocket API 不能自定义请求头（不能带 Authorization），
// 所以 token 通过 query 参数传：ws://host/ws?token=xxx
// 服务端在「升级连接之前」先校验 token，非法直接返回 401，不浪费连接资源。
func ServeWS(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		// 兼容非浏览器客户端：也支持 Authorization: Bearer xxx
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			tokenStr = parts[1]
		}
	}
	if tokenStr == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "缺少 token"})
		return
	}

	claims, err := auth.Parse(global.Config.JWT.Secret, tokenStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token 无效"})
		return
	}
	if claims.Type != auth.TypeAccess {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token 类型错误"})
		return
	}

	// 升级为 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := NewClient(DefaultHub, claims.UserID, conn)
	DefaultHub.Register(client)

	// 两个 goroutine 分别负责读和写（ReadPump 会阻塞，所以必须放 goroutine）
	go client.WritePump()
	go client.ReadPump()
}

```
