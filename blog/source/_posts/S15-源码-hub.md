---
title: "源码走读 S15：hub.go"
date: 2026-11-12 17:46:00
categories:
  - 源码走读
tags:
  - 源码
---
> **文件路径**：`internal/ws/hub.go`　**行数**：115 行
> **职责**：WebSocket 连接管理：userID → 多连接、RWMutex、优雅关闭

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
// Package ws WebSocket 长连接：连接管理、消息收发、在线状态。
//
// 【学习重点】这是 M2 最核心的并发代码：
//   - 一个 Hub 管理所有连接，被多个 goroutine 并发访问 → 必须加锁
//   - 每个连接有独立的「读 goroutine」和「写 goroutine」
//   - 写操作只能在写 goroutine 里做（gorilla/websocket 不允许并发写）
package ws

import "sync"

// Hub 管理所有在线连接。
//
// 【为什么是 map[uint64]map[*Client]struct{}】
// 一个用户可能有多个连接（手机 + 电脑 + 网页），所以要 userID → 连接集合。
// 用 map 而不是 slice：注册/注销是 O(1)。
//
// 【并发安全】
// Hub 会被「连接建立/断开」和「消息推送」多个 goroutine 同时访问，
// 必须用 RWMutex：读多写少，读写锁比普通 Mutex 性能好。
type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]map[*Client]struct{}
}

// NewHub 创建连接管理器。
func NewHub() *Hub {
	return &Hub{clients: make(map[uint64]map[*Client]struct{})}
}

// Register 注册一个连接。
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.UserID] == nil {
		h.clients[c.UserID] = make(map[*Client]struct{})
	}
	h.clients[c.UserID][c] = struct{}{}
}

// Unregister 注销一个连接。
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[c.UserID]; ok {
		delete(conns, c)
		if len(conns) == 0 {
			delete(h.clients, c.UserID)
		}
	}
}

// SendToUser 给某个用户的所有连接推送消息。
// 返回 true 表示至少有一个在线连接（即"在线"）。
//
// 【注意：这里只把数据放进每个连接的 send channel，不直接写 socket】
// 写 socket 只能由各自的写 goroutine 负责（gorilla/websocket 不允许并发写）。
func (h *Hub) SendToUser(userID uint64, payload []byte) bool {
	h.mu.RLock()
	conns := h.clients[userID]
	// 复制一份连接列表，避免持锁期间做 channel 操作
	list := make([]*Client, 0, len(conns))
	for c := range conns {
		list = append(list, c)
	}
	h.mu.RUnlock()

	for _, c := range list {
		c.Enqueue(payload)
	}
	return len(list) > 0
}

// IsOnline 判断用户是否在线。
func (h *Hub) IsOnline(userID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// OnlineCount 当前在线连接数（调试/监控用）。
func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, conns := range h.clients {
		n += len(conns)
	}
	return n
}

// DefaultHub 全局连接管理器。
// 学习项目用一个全局实例最简单；生产环境一般做成依赖注入，避免隐式全局状态。
var DefaultHub = NewHub()

// CloseAll 关闭所有连接（进程优雅退出时调用）。
//
// 【为什么需要】
// 进程退出时如果不断开 WebSocket，客户端会「假在线」一段时间才超时。
// 主动发 Close 帧能让客户端立即感知并走重连逻辑。
func (h *Hub) CloseAll() {
	h.mu.Lock()
	all := make([]*Client, 0, len(h.clients))
	for _, conns := range h.clients {
		for c := range conns {
			all = append(all, c)
		}
	}
	h.clients = make(map[uint64]map[*Client]struct{})
	h.mu.Unlock()

	for _, c := range all {
		c.Close()
	}
}

```
