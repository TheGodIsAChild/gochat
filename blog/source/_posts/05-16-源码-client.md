---
title: "源码走读 S16：client.go"
date: 2026-11-14 23:09:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/ws/client.go`　**行数**：339 行
> **职责**：单个连接：读泵/写泵/心跳/慢消费者隔离/通话信令处理

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"gochat/global"
	callservice "gochat/internal/service/call_service"
	chatservice "gochat/internal/service/chat_service"
)

const (
	// writeWait 写一条消息的超时
	writeWait = 10 * time.Second
	// pongWait 服务端等待客户端 pong 的最长时间；超时说明连接已死
	pongWait = 60 * time.Second
	// pingPeriod 服务端主动 ping 的间隔，必须小于 pongWait
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize 单条消息最大字节数，防止恶意超大消息打爆内存
	maxMessageSize = 4096
)

// Client 一个 WebSocket 连接。
//
// 【读写分离】
//   - ReadPump：只负责从 socket 读（一个 goroutine）
//   - WritePump：只负责往 socket 写（一个 goroutine）
//
// 分离的原因：gorilla/websocket 的写操作不是并发安全的，
// 如果多个 goroutine 同时写同一个连接会 panic。所以所有写都交给 WritePump，
// 其他 goroutine 只往 Send channel 里塞数据。
type Client struct {
	UserID uint64
	conn   *websocket.Conn
	hub    *Hub
	send   chan []byte
	once   sync.Once
}

// NewClient 创建连接。
func NewClient(hub *Hub, userID uint64, conn *websocket.Conn) *Client {
	return &Client{
		UserID: userID,
		conn:   conn,
		hub:    hub,
		send:   make(chan []byte, 64), // 带缓冲：写慢时不至于立刻阻塞推送方
	}
}

// Enqueue 把消息放进发送队列。
//
// 【为什么要 select + default】
// 如果客户端很慢，send channel 满了，直接 `c.send <- payload` 会阻塞调用方
// （可能是推送消息的其他用户 goroutine）。这里用 non-blocking 写：
// 队列满了就丢弃（或者可以关掉连接），保证「慢消费者」不影响别人。
// 这就是消息系统里的「慢消费者隔离」思路。
func (c *Client) Enqueue(payload []byte) {
	defer func() { _ = recover() }() // 连接关闭后 channel 可能已关闭
	select {
	case c.send <- payload:
	default:
		// 发送队列满 → 直接断开这个慢连接，避免拖垮服务
		c.Close()
	}
}

// SendJSON 发送 JSON 数据。
func (c *Client) SendJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Enqueue(b)
}

// ReadPump 读循环：处理客户端发来的消息。
//
// 【心跳机制】
//   - 设置 readDeadline，并注册 PongHandler 在收到 pong 时续期
//   - 客户端长时间不发 pong → readDeadline 触发 → 读失败 → 断开连接
//   这样能及时清理「网线拔了但 TCP 没断」的僵尸连接。
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return // 连接断开 / 超时
		}
		c.handleMessage(data)
	}
}

// WritePump 写循环：把发送队列里的数据写到 socket，并定期发 ping。
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Close 关闭连接（幂等）。
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.send)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

// ============================ 消息处理 ============================

// inMessage 客户端发来的消息格式。
type inMessage struct {
	Type           string `json:"type"` // text / read / ping / call_* / webrtc_*
	ConversationID uint64 `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Content        string `json:"content"`
	MessageID      uint64 `json:"message_id"`

	// ---- 通话信令字段 ----
	ToUserID uint64 `json:"to_user_id"` // 信令接收方
	CallID   uint64 `json:"call_id"`    // 通话 id
	CallType int8   `json:"call_type"`  // 1音频 2视频
}

func (c *Client) handleMessage(data []byte) {
	var in inMessage
	if err := json.Unmarshal(data, &in); err != nil {
		c.SendJSON(map[string]any{"type": "error", "message": "消息格式错误"})
		return
	}

	switch in.Type {
	case "ping":
		c.SendJSON(map[string]any{"type": "pong"})

	case "text":
		c.handleText(in)

	case "read":
		if err := chatservice.MarkRead(context.Background(), c.UserID, in.ConversationID, in.MessageID); err != nil {
			c.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		}

	// ---- 通话信令（M5）----
	case "call_invite":
		c.handleCallInvite(in)
	case "call_accept":
		c.handleCallAccept(in)
	case "call_reject":
		c.handleCallReject(in)
	case "call_cancel":
		c.handleCallCancel(in)
	case "call_end":
		c.handleCallEnd(in)
	case "webrtc_offer", "webrtc_answer", "webrtc_ice":
		c.handleRelay(data, in)

	default:
		c.SendJSON(map[string]any{"type": "error", "message": "未知消息类型: " + in.Type})
	}
}

// handleText 处理文本消息：落库 → ack → 推送给会话所有成员。
//
// 【注意：这里用 context.Background()】
// WebSocket 连接的生命周期远长于单个 HTTP 请求，不能用某个请求的 ctx
// （请求结束 ctx 就取消了）。所以用 Background，必要时自己加超时。
func (c *Client) handleText(in inMessage) {
	msg, err := chatservice.SendMessage(context.Background(), c.UserID, in.ConversationID, in.ClientMsgID, 1, in.Content)
	if err != nil {
		c.SendJSON(map[string]any{"type": "error", "client_msg_id": in.ClientMsgID, "message": err.Error()})
		return
	}

	// 1. 给发送者回 ack（客户端据此把「发送中」改成「已发送」）
	c.SendJSON(map[string]any{
		"type":            "ack",
		"client_msg_id":   msg.ClientMsgID,
		"message_id":      msg.ID,
		"conversation_id": msg.ConversationID,
	})

	// 2. 推送给会话所有成员（包括自己，方便多端同步；客户端用 client_msg_id 去重）
	// M7：启用 MQ 时由异步消费者推送；未启用则同步推送
	if global.MQ == nil || !global.MQ.Enabled {
		PushMessage(uint64(msg.ConversationID), chatservice.ToItem(msg))
	}
}

// ============================ 通话信令（M5）============================

// handleCallInvite 主叫发起通话：创建通话记录，推送给被叫。
func (c *Client) handleCallInvite(in inMessage) {
	call, err := callservice.Invite(context.Background(), c.UserID, in.ToUserID, in.CallType)
	if err != nil {
		c.SendJSON(map[string]any{"type": "error", "message": err.Error()})
		return
	}

	// 给主叫 ack
	c.SendJSON(map[string]any{"type": "call_ack", "call_id": call.ID})

	// 推给被叫：有来电
	c.hub.SendToUser(in.ToUserID, mustJSON(map[string]any{
		"type":         "call_incoming",
		"call_id":      call.ID,
		"from_user_id": c.UserID,
		"call_type":    call.Type,
	}))
}

// handleCallAccept 被叫接听：通知主叫可以开始交换 SDP 了。
func (c *Client) handleCallAccept(in inMessage) {
	call, err := callservice.Accept(context.Background(), in.CallID, c.UserID)
	if err != nil {
		c.SendJSON(map[string]any{"type": "error", "call_id": in.CallID, "message": err.Error()})
		return
	}
	c.SendJSON(map[string]any{"type": "call_accepted", "call_id": call.ID})
	c.hub.SendToUser(uint64(call.CallerID), mustJSON(map[string]any{
		"type": "call_accepted", "call_id": call.ID,
	}))
}

// handleCallReject 被叫拒绝。
func (c *Client) handleCallReject(in inMessage) {
	if err := callservice.Reject(context.Background(), in.CallID, c.UserID); err != nil {
		c.SendJSON(map[string]any{"type": "error", "call_id": in.CallID, "message": err.Error()})
		return
	}
	call, _ := callservice.Get(context.Background(), in.CallID, c.UserID)
	if call != nil {
		c.hub.SendToUser(uint64(call.CallerID), mustJSON(map[string]any{
			"type": "call_rejected", "call_id": call.ID,
		}))
	}
	c.SendJSON(map[string]any{"type": "call_rejected", "call_id": in.CallID})
}

// handleCallCancel 主叫取消。
func (c *Client) handleCallCancel(in inMessage) {
	if err := callservice.Cancel(context.Background(), in.CallID, c.UserID); err != nil {
		c.SendJSON(map[string]any{"type": "error", "call_id": in.CallID, "message": err.Error()})
		return
	}
	call, _ := callservice.Get(context.Background(), in.CallID, c.UserID)
	if call != nil {
		c.hub.SendToUser(uint64(call.CalleeID), mustJSON(map[string]any{
			"type": "call_canceled", "call_id": call.ID,
		}))
	}
	c.SendJSON(map[string]any{"type": "call_canceled", "call_id": in.CallID})
}

// handleCallEnd 挂断：通知对方，并返回通话时长。
func (c *Client) handleCallEnd(in inMessage) {
	call, err := callservice.End(context.Background(), in.CallID, c.UserID)
	if err != nil {
		c.SendJSON(map[string]any{"type": "error", "call_id": in.CallID, "message": err.Error()})
		return
	}

	// 找出对方
	peer := uint64(call.CallerID)
	if uint64(call.CallerID) == c.UserID {
		peer = uint64(call.CalleeID)
	}
	frame := map[string]any{
		"type":     "call_ended",
		"call_id":  call.ID,
		"duration": call.Duration,
	}
	c.hub.SendToUser(peer, mustJSON(frame))
	c.SendJSON(frame)
}

// handleRelay 转发 WebRTC 信令（offer/answer/ice）给对方。
//
// 服务器不解析 SDP 内容，只加一个 from_user_id 后原样转发。
// 这就是信令服务器最核心的职责：**帮两端交换连接信息**。
func (c *Client) handleRelay(raw []byte, in inMessage) {
	if in.ToUserID == 0 {
		c.SendJSON(map[string]any{"type": "error", "message": "缺少 to_user_id"})
		return
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		c.SendJSON(map[string]any{"type": "error", "message": "信令格式错误"})
		return
	}
	m["from_user_id"] = c.UserID
	c.hub.SendToUser(in.ToUserID, mustJSON(m))
}

// mustJSON 把对象序列化成 JSON（出错返回 nil，调用方 SendToUser 会忽略 nil）。
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

```
