---
title: 实时消息与 WebSocket
date: 2026-11-14 23:35:00
categories:
  - 04-里程碑
tags:
  - WebSocket
  - 幂等
  - 分页
  - 并发
---

## 一、会话 + 消息两层抽象

单聊和群聊共用一套表：

```
conversations         会话（type: 1单聊 / 2群聊）
conversation_members  成员（谁在会话、读到哪）
messages              消息（conversation_id + sender_id + content）
```

好处：消息表不用区分单聊/群聊，统一按 `conversation_id` 查。
M3 做群聊时只需要新增 `groups` 表，消息逻辑完全复用。

## 二、消息幂等（重点）

网络重试、用户连点、WS 重发都会导致同一条消息发多次。

**方案**：客户端生成 `client_msg_id`，数据库 `UNIQUE(sender_id, client_msg_id)`。

服务端：

1. 先查是否已存在 → 存在直接返回（幂等）
2. 不存在 → 插入
3. 并发撞唯一索引 → 捕获 `gorm.ErrDuplicatedKey` → 再查返回

配套：GORM 开启 `TranslateError: true`，把 MySQL 的 Duplicate 错误翻译成哨兵错误。

## 三、分页：游标 > offset

```sql
-- ❌ offset：越翻越慢，新消息插入还会错位
SELECT * FROM messages WHERE conversation_id=? ORDER BY id DESC LIMIT 20 OFFSET 1000;

-- ✅ 游标：用上一页最后一条 id 作锚点
SELECT * FROM messages WHERE conversation_id=? AND id < ? ORDER BY id DESC LIMIT 20;
```

## 四、WebSocket 架构

```
Hub      管理所有连接（userID -> 多个连接），RWMutex 保护
Client   一个连接，读 goroutine + 写 goroutine
ServeWS  HTTP 握手升级 + token 校验
```

### 为什么读写分离

gorilla/websocket **不允许并发写**。所以：

- `ReadPump`：唯一从 socket 读
- `WritePump`：唯一往 socket 写
- 其他 goroutine 发消息 = 往 `send` channel 塞数据

### 心跳

```go
conn.SetReadDeadline(now + 60s)
conn.SetPongHandler(func(string) error {
    return conn.SetReadDeadline(now + 60s)  // 收到 pong 续期
})
// WritePump 每 54s 发一次 ping
```

作用：清理「网线拔了但 TCP 没断」的僵尸连接。

### 慢消费者隔离

```go
select {
case c.send <- payload:
default:
    c.Close()   // 队列满 → 断开慢连接
}
```

一个慢客户端不能拖垮给其他人推送的 goroutine。

## 五、离线消息

**不需要单独离线表**：

- 消息落库到 `messages`
- 用户上线后拉历史 `GET /conversations/:id/messages`
- `conversation_members.last_read_message_id` 算未读数

这就是「持久化 + 上线拉取 + 未读数」的标准做法。

## 六、WS 鉴权

浏览器 WebSocket API 不能自定义 header，所以 token 走 query：

```
ws://host/ws?token=<access>
```

服务端在**升级连接之前**校验，非法直接 401。

## 七、踩过的坑

1. `CheckOrigin` 生产环境必须校验（CSWSH 跨站 WebSocket 劫持）
2. WS 连接用 `context.Background()`，不能用 HTTP 请求的 ctx
3. `Enqueue` 要做 non-blocking + recover（连接关闭后 channel 会关闭）
4. Hub 持锁时不要做 IO，先复制连接列表再释放锁

## 八、面试点

1. WebSocket 和长轮询/SSE 的区别？
2. 为什么读写要分离？
3. 心跳解决什么问题？
4. 大量连接怎么管理？多实例怎么办？
5. 消息怎么保证不重复？
6. 离线消息怎么实现？
7. 慢消费者怎么处理？
8. 游标分页 vs offset 分页？
9. 未读数怎么高效计算？
