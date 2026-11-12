---
title: WebSocket 全解：握手、心跳、重连与 wss
date: 2026-11-14 23:50:00
categories:
  - 02-设计
tags:
  - WebSocket
  - wss
  - 心跳
  - 重连
---

> 从「为什么不用 HTTP 轮询」讲到「生产环境怎么用 wss + Nginx 反代」。
> 读完你应该能独立设计一个能扛住断线、跨实例的实时消息通道。

![WebSocket 生命周期](/blog/images/wss-lifecycle.svg)

## 一、为什么 IM 必须用 WebSocket

| 方案 | 实时性 | 开销 | 双向 | 适用 |
|---|---|---|---|---|
| 短轮询 | 差（间隔延迟） | 每次建连接 | 否 | 低频状态查询 |
| 长轮询 | 较好 | 每条消息重建连接 | 半双工 | 兼容性要求高 |
| SSE | 好 | 单连接 | **只下行** | 通知推送 |
| **WebSocket** | **好** | **单 TCP 连接** | **全双工** | **IM / 协同** |

IM 需要「服务端主动推 + 客户端主动发」，只有 WebSocket 满足。

## 二、握手：HTTP 是怎么「升级」成 WebSocket 的

```
客户端 → 服务端
GET /ws?token=xxx HTTP/1.1
Host: 127.0.0.1:8080
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13

服务端 → 客户端
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

**关键点**：

1. 握手本身是 **HTTP 请求**，所以可以带 Cookie/Header（浏览器 API 只能带 Cookie）
2. `101` 之后协议切换，同一条 TCP 连接上开始传 WebSocket 帧
3. `Sec-WebSocket-Accept` 是 `Sec-WebSocket-Key + 固定 GUID` 的 SHA1，防误连

## 三、鉴权：为什么 token 放 query

浏览器 `new WebSocket(url)` **不能自定义 header**，所以：

```
ws://127.0.0.1:8080/ws?token=<access_token>
```

服务端的顺序非常重要：

```go
// ✅ 先鉴权，再升级
if claims.Type != auth.TypeAccess { 401; return }
conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
```

**如果先升级再鉴权**：非法连接已经建立，还要额外断开，浪费资源且难处理。

> 生产环境可改为：短期一次性 ticket（避免 token 出现在 URL/日志里）。

## 四、帧结构（了解即可）

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
|N|V|V|V|       |S|             |                               |
+-+-+-+-+-------+-+-------------+-------------------------------+
```

- `opcode`：1 文本、2 二进制、8 Close、9 Ping、10 Pong
- `MASK`：客户端发往服务端必须掩码
- **Ping/Pong 是协议内置的**，我们的心跳就基于它

## 五、服务端架构：读写双泵

```go
type Client struct {
    UserID uint64
    conn   *websocket.Conn
    hub    *Hub
    send   chan []byte   // 发送队列
    once   sync.Once
}

go client.WritePump()  // 唯一写
go client.ReadPump()   // 唯一读
```

### 为什么必须读写分离

gorilla/websocket **明确不允许并发写**（会 panic：`concurrent write to websocket connection`）。
所以：

- 所有写操作 → 只由 WritePump 执行
- 其他 goroutine → 往 `send` channel 塞数据

### WritePump 的三个职责

1. 从 `send` 取数据 → `WriteMessage`
2. 定时发 `PingMessage`（心跳）
3. 写超时（`SetWriteDeadline`）

### ReadPump 的三个职责

1. 读消息 → 分发处理（text/read/ping/call_* / webrtc_*）
2. 设读超时 + PongHandler 续期
3. 断开时注销连接

## 六、心跳与超时

```go
const (
    pongWait   = 60 * time.Second          // 等 pong 的最长时间
    pingPeriod = (pongWait * 9) / 10       // 54s 发一次 ping
)

conn.SetReadDeadline(time.Now().Add(pongWait))
conn.SetPongHandler(func(string) error {
    return conn.SetReadDeadline(time.Now().Add(pongWait))   // 收到 pong 续期
})
```

**解决什么问题**：TCP 连接可能「假死」（网线拔了、手机切网），双方都不知道。
心跳能在 60s 内发现僵尸连接并释放资源。

**为什么 pingPeriod < pongWait**：要留出网络抖动余量，否则正常连接也会被误杀。

## 七、断线重连（客户端）

```js
sock.onclose = () => {
  if (!manualClose) scheduleReconnect();
};
function scheduleReconnect() {
  const delay = reconnectDelay;
  reconnectDelay = Math.min(reconnectDelay * 2, 15000);  // 指数退避，上限 15s
  setTimeout(() => WS.connect(token), delay);
}
```

**为什么指数退避**：服务端重启时，所有客户端同时重连 → 惊群。
退避能把重连压力摊平。

**重连后要补什么**：

1. 重新注册（服务端 Hub 里已经没有这个连接）
2. 拉取断线期间的消息（`GET /conversations/:id/messages?before_id=` 或按 last_message_id 增量拉）
3. 更新在线状态

## 八、wss：生产环境必须加密

| 环境 | 协议 | 说明 |
|---|---|---|
| 本地开发 | `ws://127.0.0.1:8080/ws` | localhost 属于安全上下文，可采集麦克风 |
| 生产 | **`wss://example.com/ws`** | TLS 加密，防窃听/篡改 |

### Nginx 反代配置（关键三行）

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;                          # 必须 1.1
    proxy_set_header Upgrade $http_upgrade;          # 必须
    proxy_set_header Connection "upgrade";           # 必须
    proxy_read_timeout 3600s;                        # 长连接别被断
}
```

### 常见 wss 问题

| 现象 | 原因 |
|---|---|
| 502 / 400 | 少了 `Upgrade`/`Connection` 头 |
| 连上 60s 就断 | Nginx `proxy_read_timeout` 太短 |
| 混合内容被浏览器拦 | HTTPS 页面里写了 `ws://`，必须用 `wss://` |
| 证书错误 | 用自签证书时浏览器不信任 |

## 九、多实例：内存 Hub 的天花板

```
用户 A → 实例 1 的 Hub
用户 B → 实例 2 的 Hub
A 发消息给 B → 实例 1 的 Hub 里找不到 B 的连接 → 推不到
```

**三种解法**：

| 方案 | 做法 | 本项目 |
|---|---|---|
| Redis Pub/Sub | 每个实例订阅频道，收到广播后推本机连接 | 未实现（可扩展） |
| **MQ 扇出** | 消息发到 RabbitMQ，所有实例消费后各推本机在线用户 | ✅ M7 已用 |
| 一致性哈希 | 按 userID 路由到固定实例 | 未实现 |

同时**在线状态**也要从「内存 Hub」改成「Redis + TTL」。

## 十、代码导读

| 关注点 | 文件 |
|---|---|
| 握手 + 鉴权 | `internal/ws/handler.go` |
| 连接管理 | `internal/ws/hub.go` |
| 读写泵 / 心跳 / 消息处理 | `internal/ws/client.go` |
| 消息扇出 | `internal/ws/push.go` |
| 前端客户端 | `web/js/ws.js` |

## 十一、面试问法

1. WebSocket 握手过程？101 状态码是什么？
2. WebSocket 和 HTTP 长轮询/SSE 的区别？
3. 为什么 WebSocket 要读写分离？
4. 心跳怎么做？为什么 ping 间隔要小于超时时间？
5. 断线重连怎么做？为什么要指数退避？重连后要补什么？
6. wss 和 ws 的区别？Nginx 怎么配？
7. 浏览器 WebSocket 为什么不能带自定义 header？怎么鉴权？
8. 多实例下怎么给「不在本机的用户」推消息？
9. 消息太多/太慢怎么办？（慢消费者、背压、丢弃策略）
10. WebSocket 怎么测试？（.NET ClientWebSocket / wscat / 浏览器）

---

相关：[并发编程实战](/blog/2026/11/14/02-03-并发编程实战/)、[M2 单聊与 WebSocket 详情](/blog/2026/11/14/04-10-M2详细设计/)
