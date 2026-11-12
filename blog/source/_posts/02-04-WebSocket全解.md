---
title: WebSocket 全解：握手、心跳、重连与 wss
date: 2026-11-14 23:48:00
categories:
  - 02-设计
tags:
  - WebSocket
  - wss
  - 心跳
  - 重连
---

> 从「为什么不用 HTTP 轮询」讲到「生产环境怎么上 wss + Nginx」。
> 服务端代码全部来自 `internal/ws/`，前端代码来自 `web/js/ws.js`。

## 一、为什么 IM 必须用 WebSocket

| 方案 | 实时性 | 连接开销 | 双向 | 适用场景 |
|---|---|---|---|---|
| 短轮询 | 差（取决于间隔） | 每次都建连接 | ❌ | 低频状态查询 |
| 长轮询 | 较好 | 每条消息重建 | 半双工 | 兼容性优先 |
| SSE | 好 | 单连接 | ❌ **只下行** | 通知推送 |
| **WebSocket** | **好** | **单 TCP 长连接** | ✅ **全双工** | **IM、协同编辑** |

IM 需要「服务端主动推 + 客户端主动发」，只有 WebSocket 同时满足。

## 二、握手：HTTP 是怎么「升级」成 WebSocket 的

WebSocket 不是凭空建立的，它**先发一个 HTTP 请求，然后升级协议**。

### 客户端发出的报文

```
GET /ws?token=eyJhbGciOiJIUzI1NiIs... HTTP/1.1
Host: 127.0.0.1:8080
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
```

### 服务端返回

```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

### 三个关键字段

| 字段 | 作用 |
|---|---|
| `Upgrade: websocket` | 告诉服务端「我想升级协议」 |
| `Connection: Upgrade` | 配合 Upgrade 使用 |
| `Sec-WebSocket-Key` / `Accept` | 防误连：服务端把 Key + 固定 GUID 做 SHA1，客户端校验 |

**`101` 状态码之后**，同一条 TCP 连接上开始传 WebSocket 帧（不再是 HTTP 报文）。

### 本项目的握手代码（`internal/ws/handler.go`）

核心逻辑（简化）：

```go
func ServeWS(c *gin.Context) {
    // ① 从 query 拿 token（浏览器 WebSocket API 不能自定义 header）
    token := c.Query("token")
    if token == "" {
        token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
    }

    // ② 先校验，再升级！非法 token 直接 401
    claims, err := auth.Parse(global.Config.JWT.Secret, token)
    if err != nil || claims.Type != auth.TypeAccess {
        c.JSON(401, gin.H{"code": 401, "message": "token 无效"})
        return
    }

    // ③ 升级连接
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }

    // ④ 注册到 Hub，启动读写双泵
    client := NewClient(DefaultHub, claims.UserID, conn)
    DefaultHub.Register(client)
    go client.WritePump()
    go client.ReadPump()
}
```

### 两个必须记住的点

**1. 先鉴权，再升级**

如果先 `Upgrade` 再校验 token，非法连接已经建立了，还要额外发 Close 帧断开——浪费资源，而且日志里会有一堆「连上又断开」的噪音。

**2. 为什么 token 放 query**

浏览器的 `new WebSocket(url)` **不能自定义请求头**。所以只能：

```js
const ws = new WebSocket(`ws://127.0.0.1:8080/ws?token=${accessToken}`);
```

> **安全提醒**：URL 里的 token 会进 Nginx 访问日志、浏览器历史。生产环境更推荐「先用 HTTP 换一个短期一次性 ticket，再用 ticket 连 WS」。

## 三、帧结构（了解即可）

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |          (16/64)              |
|N|V|V|V|       |S|             |                               |
+-+-+-+-+-------+-+-------------+-------------------------------+
```

| opcode | 含义 |
|---|---|
| 1 | 文本帧（我们用的） |
| 2 | 二进制帧 |
| 8 | Close（关闭） |
| 9 | Ping（心跳） |
| 10 | Pong（心跳响应） |

**Ping/Pong 是协议内置的**，本项目的心跳就基于它们。

## 四、服务端架构：一个连接 = 两个 goroutine

```go
type Client struct {
    UserID uint64
    conn   *websocket.Conn
    hub    *Hub
    send   chan []byte      // 发送队列（容量 64）
    once   sync.Once        // 保证 Close 只执行一次
}

func NewClient(hub *Hub, userID uint64, conn *websocket.Conn) *Client {
    return &Client{
        UserID: userID,
        conn:   conn,
        hub:    hub,
        send:   make(chan []byte, 64),   // 带缓冲：写慢时不至于立刻阻塞推送方
    }
}
```

### 为什么必须是两个 goroutine

`gorilla/websocket` 的**写操作不是并发安全的**：

```
panic: concurrent write to websocket connection
```

所以：

- **ReadPump**：唯一负责从 socket 读
- **WritePump**：唯一负责往 socket 写
- 其他 goroutine 想发消息 → 往 `send` channel 塞

这就是「**共享资源单 goroutine 独占**」模式。

## 五、心跳：怎么发现「假死」连接

TCP 连接可能「假死」：网线拔了、手机切 4G、路由器重启——双方 TCP 状态还是 ESTABLISHED，但数据永远到不了。

如果不处理：

- 服务端以为用户在线，一直往这个连接推送
- goroutine 和内存一直被占用（**泄漏**）

### 本项目的常量（`client.go`）

```go
const (
    writeWait      = 10 * time.Second        // 单次写超时
    pongWait       = 60 * time.Second        // 等 pong 的最长时间
    pingPeriod     = (pongWait * 9) / 10     // 54 秒发一次 ping
    maxMessageSize = 4096                    // 单条消息上限，防超大消息打爆内存
)
```

### `pingPeriod` 为什么是 54 秒而不是 60 秒

```
pongWait   = 60s   ← 超过 60s 没收到 pong 就认为连接死了
pingPeriod = 54s   ← 每 54s 发一次 ping
```

留 6 秒（10%）余量，是为了**网络抖动**。

如果 ping 间隔等于超时时间（都 60s），那么只要有一次网络延迟 > 0，就可能在 ping 到达前就超时 → **正常连接被误杀**。

### 读超时 + pong 续期（真实代码）

```go
func (c *Client) ReadPump() {
    defer func() {
        c.hub.Unregister(c)
        c.Close()
    }()

    c.conn.SetReadLimit(maxMessageSize)
    _ = c.conn.SetReadDeadline(time.Now().Add(pongWait))   // 60s 后超时
    c.conn.SetPongHandler(func(string) error {
        return c.conn.SetReadDeadline(time.Now().Add(pongWait))  // 收到 pong 就续期
    })

    for {
        _, data, err := c.conn.ReadMessage()
        if err != nil {
            return    // 连接断开 / 超时
        }
        c.handleMessage(data)
    }
}
```

**逐行解释**：

- `SetReadDeadline(now + 60s)`：如果 60 秒内没有读到任何数据（包括 pong），`ReadMessage` 会返回超时错误
- `SetPongHandler`：收到 pong 时把 deadline 往后推 60 秒
- 循环里 `ReadMessage` 出错 → 退出 → defer 里注销连接 + 关闭 socket

**这样一台服务器能自动清掉所有僵尸连接。**

## 六、消息分发：客户端发来的各种消息怎么处理

`handleMessage` 是一个大的 `switch`：

| 客户端发 `type` | 服务端做什么 |
|---|---|
| `text` | 发消息（落库 + 推送） |
| `read` | 更新已读位置 |
| `ping` | 回 `pong`（应用层心跳，和协议层 Ping 不同） |
| `call_invite` | 发起通话 → 回 `call_ack` + 给对方 `call_incoming` |
| `call_accept` / `call_reject` / `call_cancel` / `call_end` | 通话状态流转 |
| `webrtc_offer` / `webrtc_answer` / `webrtc_ice` | **原样转发**给对方（信令） |

### WebRTC 信令为什么是「原样转发」

```go
// 服务器不解析 SDP，只加一个 from_user_id 然后转发
var m map[string]any
json.Unmarshal(raw, &m)
m["from_user_id"] = c.UserID
c.hub.SendToUser(in.ToUserID, mustJSON(m))
```

**因为 SDP 是浏览器之间的协商内容，服务器不需要懂。** 这就是「信令服务器」的本质——只做中转。

## 七、客户端：连接、心跳、重连（`web/js/ws.js`）

### 连接

```js
function connect(token) {
  sock = new WebSocket(`ws://${location.host}/ws?token=${token}`);
  sock.onopen  = () => { setStatus('connected'); startHeartbeat(); };
  sock.onclose = () => { setStatus('disconnected'); scheduleReconnect(); };
  sock.onmessage = (e) => dispatch(JSON.parse(e.data));
}
```

### 心跳

```js
heartbeatTimer = setInterval(() => {
  if (sock && sock.readyState === WebSocket.OPEN) {
    sock.send(JSON.stringify({ type: 'ping' }));
  }
}, 25000);   // 25 秒一次，比服务端 60 秒超时短很多
```

### 重连（指数退避）

```js
let delay = 1000;   // 初始 1 秒

function scheduleReconnect() {
  setTimeout(() => {
    connect(getToken());
    delay = Math.min(delay * 2, 15000);   // 1s → 2s → 4s → 8s → 15s 封顶
  }, delay);
}
```

**为什么要指数退避**：如果服务端重启，1 万个客户端同时用 1 秒间隔重连 → 服务端刚起来就被打挂（**惊群**）。退避能把重连压力摊平。

### 重连后要补什么

1. **重新注册**：服务端 Hub 里已经没有这个连接了（连接断开时被 Unregister）
2. **拉断线期间的消息**：`GET /conversations/:id/messages?before_id=` 或按本地最后一条消息 id 增量拉
3. **刷新在线状态**

本项目前端在 `ws.onopen` 里重新拉会话列表和好友申请，就是这个道理。

## 八、wss：生产环境必须加密

| 环境 | 协议 | 说明 |
|---|---|---|
| 本地开发 | `ws://127.0.0.1:8080/ws` | localhost 是安全上下文，可采集麦克风 |
| 生产 | **`wss://example.com/ws`** | TLS 加密，防窃听/篡改 |

### Nginx 反代（关键三行）

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;                       # 必须 1.1
    proxy_set_header Upgrade $http_upgrade;       # 必须
    proxy_set_header Connection "upgrade";        # 必须
    proxy_read_timeout 3600s;                     # 长连接别被 60s 默认超时断开
}
```

### 常见 wss 问题排查

| 现象 | 原因 |
|---|---|
| 502 / 400 | 少了 `Upgrade` / `Connection` 头 |
| 连上 60 秒就断 | `proxy_read_timeout` 太短（默认 60s） |
| 浏览器报 Mixed Content | HTTPS 页面里写了 `ws://`，必须 `wss://` |
| 证书错误 | 自签证书浏览器不信任 |

## 九、多实例：内存 Hub 的天花板

```
用户 A → 实例 1 的 Hub
用户 B → 实例 2 的 Hub

A 发消息给 B → 实例 1 的 Hub 里找不到 B 的连接 → 推不到
```

| 方案 | 做法 | 本项目 |
|---|---|---|
| Redis Pub/Sub | 每个实例订阅频道，收到广播后推本机连接 | 未实现（可扩展） |
| **MQ 扇出** | 消息发到 RabbitMQ，所有实例消费后各推本机在线用户 | ✅ M7 已用 |
| 一致性哈希 | 按 userID 路由到固定实例 | 未实现 |

**在线状态也要换**：单机 `Hub.IsOnline` 只能反映本进程；多实例要改成 `Redis + TTL`。

## 十、代码导读

| 关注点 | 文件 |
|---|---|
| 握手 + 鉴权 | `internal/ws/handler.go` |
| 连接管理 | `internal/ws/hub.go` |
| 读写泵 / 心跳 / 消息处理 | `internal/ws/client.go` |
| 扇出推送 | `internal/ws/push.go` |
| 前端客户端 | `web/js/ws.js` |

## 十一、面试问答

1. WebSocket 握手过程？101 状态码是什么？
2. WebSocket 和 HTTP 长轮询、SSE 的区别？
3. 为什么 WebSocket 读写必须分离？
4. 心跳怎么做？为什么 ping 间隔要小于超时时间？
5. 断线重连怎么做？为什么要指数退避？重连后要补什么？
6. wss 和 ws 的区别？Nginx 怎么配？
7. 浏览器 WebSocket 为什么不能带自定义 header？那怎么鉴权？
8. 多实例下怎么给「不在本机的用户」推消息？
9. 慢消费者怎么处理？为什么选择断开？
10. WebSocket 怎么测试？（.NET ClientWebSocket / wscat / 浏览器 DevTools）

## 十二、面试怎么答（背这 3 句）

**问：WebSocket 握手是怎么完成的？**

> 客户端先发一个普通 HTTP 请求，带 `Upgrade: websocket`、`Connection: Upgrade`、`Sec-WebSocket-Key`；
> 服务端返回 `101 Switching Protocols` 和由 Key 算出的 `Sec-WebSocket-Accept`。
> 之后同一条 TCP 连接上开始传 WebSocket 帧，不再走 HTTP。

**问：为什么必须读写分离？心跳怎么做？**

> `gorilla/websocket` 不允许并发写，两个 goroutine 同时写会 panic，所以 ReadPump 唯一读、WritePump 唯一写，其他 goroutine 只能往 send channel 塞。
> 心跳：服务端每 54 秒发 ping，60 秒收不到 pong 就断开（ping 间隔 < 超时，留网络抖动余量），用来清理「假死」连接。

**问：多实例部署怎么给不在本机的用户推消息？**

> 单机 Hub 只管本进程连接，跨实例要：Redis Pub/Sub 广播，或 MQ 扇出让每个实例推自己的在线用户（本项目 M7 用了这个思路）。
> 在线状态也要从内存改成 Redis + TTL。

## 十三、小白读到这里的 5 个疑问

### ❓ `CheckOrigin` 是什么？为什么本地能连、部署到线上可能 403

WebSocket 握手是 HTTP 请求，浏览器会带 `Origin` 头。`Upgrader.CheckOrigin` 用来决定"允不允许这个来源连"。
默认策略只允许**同源**。本地 `127.0.0.1:8080` 打开页面、连同一个地址，同源所以能连；
一旦前后端不同域（比如前端 3000、后端 8080），默认就会被拒，需要显式放行。
生产环境不要写成"永远返回 true"，那等于允许任意网站连你的 WS（CSWSH 劫持）。

### ❓ 应用层的 `{"type":"ping"}` 和协议层的 Ping 帧有什么区别

| | 协议层 Ping/Pong（opcode 9/10） | 应用层 `type:ping` |
|---|---|---|
| 谁处理 | 浏览器/服务端底层库自动回 | 你的业务代码自己处理 |
| 用途 | 保活、探测连接是否存活 | 业务自定义（本项目前端也发了它） |

本项目**两种都有**：服务端 WritePump 发协议层 Ping 做保活；客户端还会发应用层 ping 让服务端回 pong，双向确认。

### ❓ `SetReadLimit(4096)` 会不会太小？发图片怎么办

4KB 只限制**单帧**大小。文本消息足够；图片/语音的正确做法是**先上传拿到 URL，再发 URL 文本**，不走 WS 传二进制。
如果不设上限，恶意客户端发一个 1GB 的帧就能把服务内存打爆。

### ❓ WebSocket 握手为什么用 GET 而不是 POST

因为它本质是「**请求把这条连接升级成 WebSocket 协议**」，不涉及提交数据体。
协议规定握手必须是 GET，而且不能带 body。

### ❓ 怎么在浏览器里看到 WebSocket 帧

F12 → Network（网络）→ 筛选 `WS` → 点开那条连接 → Messages（消息）标签页，
能看到收发的每一帧 JSON，调试消息格式时非常有用。

---

相关：[并发编程实战](/blog/2026/11/14/02-03-并发编程实战/)、[M2 单聊与 WebSocket 详情](/blog/2026/11/14/04-10-M2详细设计/)
