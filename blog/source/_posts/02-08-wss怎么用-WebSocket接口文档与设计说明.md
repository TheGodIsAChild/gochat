---
title: wss 怎么用：WebSocket 接口文档 + 设计说明（新手向）
date: 2026-11-14 23:44:00
categories:
  - 02-设计
tags:
  - WebSocket
  - wss
  - 接口文档
  - 新手
---

> 如果你第一次看这个项目，会疑惑：**wss 到底怎么连？连上以后发什么？服务端回什么？**
> 这篇就是答案——**接口文档 + 设计说明**，所有字段都来自真实代码，可以直接照着调。

![WebSocket 生命周期](/blog/images/wss-lifecycle.svg)

## 一、30 秒看懂：wss 在本项目里干什么

一句话：**wss 是一条"常开的双向管道"，服务端可以主动把消息推给你。**

| 用 HTTP 能做吗 | 用 wss 的理由 |
|---|---|
| 能拉历史消息 | 但**不能"被推送"**：新消息来了，HTTP 没法主动通知你 |
| 可以每 3 秒轮询一次 | 浪费请求、有延迟；IM 要的是"秒收" |

本项目的分工：

| 通道 | 用途 |
|---|---|
| **HTTP**（`/api/v1/*`） | 拉历史消息、加好友、建群、登录……**一次性请求-响应** |
| **wss**（`/ws`） | 收新消息、发消息、已读回执、通话信令、心跳——**持续双向** |

> `ws://` 是明文，`wss://` 是 TLS 加密版。**生产环境必须用 wss**（HTTPS 页面里也只能连 wss）。

## 二、怎么连上（第一步）

### 2.1 连接地址

```
ws://127.0.0.1:8080/ws?token=<access_token>
wss://chat.example.com/ws?token=<access_token>     ← 生产
```

**为什么 token 放 query**：浏览器的 `new WebSocket()` **不能自定义请求头**，带不了 `Authorization`。
（非浏览器客户端也可以用 `Authorization: Bearer xxx` 头，服务端两种都支持。）

### 2.2 token 从哪来

先用 HTTP 登录拿 `access_token`：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"123456"}'
```

响应里的 `data.access_token` 就是连 wss 用的 token。

### 2.3 握手发生了什么

```
① 浏览器发 HTTP 请求：GET /ws?token=xxx  +  Upgrade: websocket
② 服务端先验 token（无效直接 401，不建立连接）
③ 返回 101 Switching Protocols
④ 之后同一连接上传 WebSocket 帧（不再走 HTTP）
```

**返回 101 就成功了**；如果返回 401，说明 token 缺失/无效/类型不对（必须是 **access** token，不能用 refresh）。

### 2.4 最小可跑代码（浏览器控制台粘贴即可验证）

```js
// 1. 登录拿 token（真实项目里在前端 api.js 做）
const login = await fetch('/api/v1/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'alice', password: '123456' }),
}).then(r => r.json());
const token = login.data.access_token;

// 2. 连 wss
const ws = new WebSocket(`ws://127.0.0.1:8080/ws?token=${token}`);

ws.onopen = () => console.log('已连接');
ws.onmessage = (e) => console.log('收到:', JSON.parse(e.data));
ws.onclose = () => console.log('断开');

// 3. 发一条"心跳"验证链路
ws.send(JSON.stringify({ type: 'ping' }));
// 期望收到 {"type":"pong"}
```

> 打开浏览器 F12 → Console 粘贴，能收到 `pong` 说明链路完全通了。

## 三、消息长什么样（协议设计）

### 3.1 统一信封：`type` + 业务字段

所有消息都是 **一个 JSON 对象**，用 `type` 区分类型：

```json
{ "type": "text", "conversation_id": 1, "client_msg_id": "c-abc123", "content": "你好" }
```

**为什么不设计成多个 URL/端点**（比如 `/ws/text`、`/ws/call`）？
因为 WebSocket 只有**一条连接**，没有 URL 路由概念，只能自己在消息里做分发——
所以用 `type` 当"路由"，服务端一个 `switch` 分发（见 `internal/ws/client.go` 的 `handleMessage`）。

### 3.2 三条设计约定

| 约定 | 说明 |
|---|---|
| **type 必填** | 服务端靠它分发；未知 type 会回 `error` |
| **字段可空** | 不同 type 用不同字段，没用到的不传即可（Go 结构体零值兜底） |
| **单条上限 4KB** | 服务端 `SetReadLimit(4096)`，图片/语音请先上传拿 URL 再发文本 |

## 四、完整消息清单（API）

### 4.1 客户端 → 服务端（上行）

| type | 必填字段 | 作用 | 示例 |
|---|---|---|---|
| `ping` | — | 应用层心跳，服务端回 `pong` | `{"type":"ping"}` |
| `text` | `conversation_id`、`client_msg_id`、`content` | 发消息（落库 + 推送） | 见下 |
| `read` | `conversation_id`、`message_id` | 标记已读位置 | `{"type":"read","conversation_id":1,"message_id":20}` |
| `call_invite` | `to_user_id`、`call_type` | 发起通话（1 音频 / 2 视频） | `{"type":"call_invite","to_user_id":2,"call_type":1}` |
| `call_accept` | `call_id` | 被叫接听 | `{"type":"call_accept","call_id":5}` |
| `call_reject` | `call_id` | 被叫拒绝 | `{"type":"call_reject","call_id":5}` |
| `call_cancel` | `call_id` | 主叫取消 | `{"type":"call_cancel","call_id":5}` |
| `call_end` | `call_id` | 任意一方挂断 | `{"type":"call_end","call_id":5}` |
| `webrtc_offer` | `call_id`、`to_user_id`、`sdp` | 转发 SDP offer | 见下 |
| `webrtc_answer` | `call_id`、`to_user_id`、`sdp` | 转发 SDP answer | 见下 |
| `webrtc_ice` | `call_id`、`to_user_id`、`candidate` | 转发 ICE 候选 | 见下 |

**发消息示例**：

```json
{
  "type": "text",
  "conversation_id": 1,
  "client_msg_id": "c-1720000000-ab12cd",
  "content": "你好，这是第一条消息"
}
```

> `client_msg_id` 由**客户端生成**（UUID/时间戳+随机串），用于幂等：重发不会产生两条消息。

**WebRTC 信令示例**：

```json
{ "type": "webrtc_offer", "call_id": 5, "to_user_id": 2, "sdp": { "type": "offer", "sdp": "v=0\r\n..." } }
{ "type": "webrtc_ice",   "call_id": 5, "to_user_id": 2, "candidate": { "candidate": "candidate:...", "sdpMid": "0" } }
```

> 服务端**不解析 SDP**，只加一个 `from_user_id` 原样转发给对方——这就是"信令服务器"。

### 4.2 服务端 → 客户端（下行）

| type | 字段 | 含义 |
|---|---|---|
| `pong` | — | 对 `ping` 的回应 |
| `ack` | `client_msg_id`、`message_id`、`conversation_id` | 消息已落库（客户端据此把"发送中"改为"已发送"） |
| `message` | `data`（消息对象） | 新消息推送（也可能推给发送者自己，用于多端同步） |
| `error` | `message`（可能带 `client_msg_id` / `call_id`） | 处理失败 |
| `call_ack` | `call_id` | 主叫收到：通话记录已创建 |
| `call_incoming` | `call_id`、`from_user_id`、`call_type` | 被叫收到：有人来电 |
| `call_accepted` | `call_id` | 双方收到：已接听 |
| `call_rejected` | `call_id` | 双方收到：已拒绝 |
| `call_canceled` | `call_id` | 双方收到：已取消 |
| `call_ended` | `call_id`、`duration`（秒） | 双方收到：已挂断，含时长 |

**`message` 推送里的 `data` 字段**（与 HTTP 历史消息结构一致）：

```json
{
  "type": "message",
  "data": {
    "id": 21,
    "conversation_id": 1,
    "sender_id": 1,
    "client_msg_id": "c-1720000000-ab12cd",
    "type": 1,
    "content": "你好，这是第一条消息",
    "status": 1,
    "created_at": "2026-11-13T23:00:00+08:00"
  }
}
```

> **注意**：`created_at` 在 HTTP 接口是 ISO 字符串；走 MQ 异步推送时可能是 **Unix 秒**。
> 前端已做归一化（`toDate()`），自己写客户端时要注意这个差异。

## 五、四个典型流程

### 5.1 发送消息（最常用）

```
客户端 A                服务端                      客户端 B
   │  {type:text}         │                            │
   ├─────────────────────>│ 1. 落库（幂等）              │
   │                      │ 2. 更新会话最后消息          │
   │<──── {type:ack} ─────┤                            │
   │                      │ 3. 查会话成员并推送          │
   │                      ├────── {type:message} ─────>│
```

- **A 也会收到 `message`**（多端同步用），客户端要用 `client_msg_id` 去重
- 开启 MQ（M7）时，第 3 步由消费者异步推送，**会有约 0~2 秒延迟**

### 5.2 标记已读

```
客户端 → {"type":"read","conversation_id":1,"message_id":21}
服务端 → 更新 conversation_members.last_read_message_id（只前进不后退，无回包）
```

未读数 = 会话里 `id > last_read_message_id` 且 `sender != 我` 的消息数。

### 5.3 心跳（保持连接 + 发现假死）

```
客户端（每 25s）→ {"type":"ping"}
服务端         → {"type":"pong"}
```

同时服务端还有**协议层心跳**：每 54 秒发 WebSocket Ping 帧，60 秒收不到 Pong 就断开。

### 5.4 语音通话（完整信令）

```
A（主叫）                 服务端                     B（被叫）
   │ call_invite ──────────>│                          │
   │<──── call_ack ─────────┤                          │
   │                        ├──── call_incoming ──────>│
   │                        │<─── call_accept ─────────┤
   │<──── call_accepted ────┤──── call_accepted ──────>│
   │ webrtc_offer ─────────>│──── webrtc_offer ───────>│
   │<──── webrtc_answer ────┤<─── webrtc_answer ───────┤
   │ webrtc_ice ───────────>│──── webrtc_ice ─────────>│
   │        ═══════ P2P 音视频直连（不经服务器）═══════ │
   │ call_end ─────────────>│──── call_ended ─────────>│
```

## 六、服务端是怎么设计的（为什么这么写）

### 6.1 连接管理：Hub

```go
type Hub struct {
    mu      sync.RWMutex
    clients map[uint64]map[*Client]struct{}   // userID → 多个连接（多端登录）
}
```

- 一个用户可能有手机 + 电脑 + 网页多个连接 → 所以是"用户 → 连接集合"
- 推送时给该用户**所有在线连接**都发

### 6.2 一个连接 = 两个 goroutine

```go
go client.WritePump()   // 只管往 socket 写
go client.ReadPump()    // 只管从 socket 读
```

**为什么必须分开**：`gorilla/websocket` 不允许并发写，两个 goroutine 同时写会 panic。
所以其他 goroutine 想发消息 → 只能往 `send chan []byte` 里塞，由 WritePump 统一写出。

### 6.3 消息分发：一个 switch

```go
switch in.Type {
case "ping":        c.SendJSON({"type":"pong"})
case "text":        c.handleText(in)
case "read":        chatservice.MarkRead(...)
case "call_invite": c.handleCallInvite(in)
case "webrtc_offer", "webrtc_answer", "webrtc_ice": c.handleRelay(data, in)
default:            c.SendJSON({"type":"error","message":"未知消息类型"})
}
```

**加一种新消息只需要改这个 switch**（下面有步骤）。

### 6.4 推送：查成员 → 扇出

```
PushMessage(convID, item)
  → ListMemberIDs(convID)          查会话所有成员
  → fanout(memberIDs, frame)        小会话顺序推；大会话并发 + 信号量限 16
  → hub.SendToUser(uid, frame)      给该用户所有连接入队
```

### 6.5 为什么不直接用 HTTP 发消息？

| | HTTP | wss |
|---|---|---|
| 能发 | ✅ | ✅ |
| 能收推送 | ❌ | ✅ |
| 连接成本 | 每次请求 | 一次长连接 |
| 适合 | 历史消息、操作类 | **实时收发** |

本项目**两个通道都保留了**：HTTP 发消息（降级用、方便 curl 测试），wss 发消息（前端实际用）。

## 七、前端是怎么用的（`web/js/ws.js` 的结构）

```js
// 连接
function connect(token) {
  sock = new WebSocket(`${proto}//${location.host}/ws?token=${token}`);
  sock.onopen  = () => { startHeartbeat(); emit('open'); };
  sock.onmessage = (e) => {
    const msg = JSON.parse(e.data);
    emit(msg.type, msg);        // 按 type 分发到注册的 handler
  };
  sock.onclose = () => scheduleReconnect();   // 指数退避重连
}

// 发
function send(obj) {
  if (sock && sock.readyState === WebSocket.OPEN) { sock.send(JSON.stringify(obj)); return true; }
  return false;   // 返回 false 时上层降级用 HTTP
}

// 收（业务方注册）
WS.on('message', (msg) => appendMessage(msg.data));
WS.on('ack',     (msg) => markSent(msg.client_msg_id));
WS.on('call_incoming', (msg) => showIncomingCall(msg));
```

**要点**：
- 心跳 25 秒一次（服务端 60 秒超时）
- 断线自动重连（1s→2s→4s→8s→15s 封顶）
- 重连后要重新拉会话列表（断线期间的消息靠 HTTP 补）

## 八、后端加一种新消息（5 步）

需求：加一个「对方正在输入」`typing`。

**① 在 `inMessage` 里加字段**（`internal/ws/client.go`）

```go
type inMessage struct {
    // ... 已有字段
    Typing bool `json:"typing"`
}
```

**② 在 `handleMessage` 的 switch 里加分支**

```go
case "typing":
    c.handleTyping(in)
```

**③ 实现处理函数**

```go
func (c *Client) handleTyping(in inMessage) {
    // 转发给会话其他成员（不落库）
    ids, err := chatservice.ListMemberIDs(context.Background(), in.ConversationID)
    if err != nil { return }
    frame := mustJSON(map[string]any{
        "type": "typing", "conversation_id": in.ConversationID, "from_user_id": c.UserID,
    })
    for _, uid := range ids {
        if uid != c.UserID { c.hub.SendToUser(uid, frame) }
    }
}
```

**④ 前端注册 handler**

```js
WS.on('typing', (msg) => showTypingIndicator(msg.from_user_id));
```

**⑤ 自测**

```js
ws.send(JSON.stringify({ type: 'typing', conversation_id: 1 }));
// 另一个窗口应收到 {"type":"typing", ...}
```

> 注意：**带状态变更的消息要落库 + 幂等**，像 `typing` 这种瞬时状态就不用落库。

## 九、调试手册

| 想做什么 | 怎么做 |
|---|---|
| 看收发帧 | F12 → Network → 筛选 `WS` → 点连接 → **Messages** |
| 看握手失败原因 | 直接浏览器访问 `/ws?token=xxx`，非浏览器会返回 JSON 错误 |
| 命令行连 | `npm i -g wscat` 然后 `wscat -c "ws://127.0.0.1:8080/ws?token=xxx"` |
| 验证鉴权 | `curl -i "http://127.0.0.1:8080/ws?token=bad"` → 期望 401 |
| 验证推送 | 两个浏览器窗口，A 发消息，B 的 Network 里应立刻出现 `message` 帧 |
| 看服务端错 | 服务端控制台日志 + `docker logs xiaohugochat` |

## 十、常见问题

| 现象 | 原因 | 解决 |
|---|---|---|
| 连接立刻断开（401） | token 缺失/过期/类型错 | 用 **access** token；过期先调 `/token/refresh` |
| 连上但收不到消息 | 连接没注册成功 / 会话成员判断错 | 看服务端日志有无 `Register`；确认是会话成员 |
| 消息重复显示 | 发送者自己也会收到推送 | 前端用 `client_msg_id` 去重（本项目已做） |
| 发送没反应 | 连接已断但没触发 onclose | 检查心跳与重连；`send()` 返回 false 时走 HTTP 降级 |
| 60 秒左右断 | 心跳没发 / 反代超时 | 客户端心跳 25s；Nginx `proxy_read_timeout 3600s` |
| 生产环境连不上 | HTTPS 页面连了 `ws://` | 必须用 `wss://`（否则被浏览器拦成混合内容） |
| 一直不推送（延迟几秒） | MQ 异步推送 | 正常，outbox worker 每 2 秒轮询 |

## 十一、面试问答

1. **wss 鉴权怎么做？** → token 放 query（浏览器 WS 不能带 header），升级前校验，非法直接 401。
2. **为什么不给每种消息一个 URL？** → WS 只有一条连接，没有路由概念，靠消息里的 `type` 分发。
3. **消息怎么保证不重复？** → 客户端生成 `client_msg_id` + 数据库唯一索引；推送时双方都收到，客户端去重。
4. **为什么一个连接要两个 goroutine？** → gorilla 不允许并发写，读写分离后写操作单点执行。
5. **服务端怎么知道推给谁？** → 按 conversation_id 查成员，再按 userID 找该用户所有在线连接。
6. **多端登录怎么处理？** → `map[userID]map[*Client]struct{}`，一个用户多个连接，全部推送。
7. **断线重连后要做什么？** → 重新注册连接 + 拉断线期间的消息（HTTP 补历史）。
8. **wss 和 http 怎么配合？** → HTTP 负责"拉"和"操作"，wss 负责"推"和"实时发"，HTTP 还是 wss 的降级通道。

---

相关：[WebSocket 全解（原理深入）](/blog/2026/11/14/02-04-WebSocket全解/)、[HTTP 协议演进](/blog/2026/11/14/02-07-HTTP协议演进与WebSocket为什么用1.1/)、[源码走读：client.go](/blog/2026/11/14/05-16-源码-client/)
