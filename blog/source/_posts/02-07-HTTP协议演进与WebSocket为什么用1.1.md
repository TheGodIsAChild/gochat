---
title: HTTP 协议演进：为什么 WebSocket 必须用 HTTP/1.1
date: 2026-11-14 23:45:00
categories:
  - 02-设计
tags:
  - HTTP
  - HTTP2
  - HTTP3
  - WebSocket
  - 面试
---

> 面试高频题：「你们为什么用 HTTP/1.1？为什么不上 HTTP/2？」
> 很多人答成"因为 WebSocket 只支持 1.1"——**只对了一半**。
> 本文把两段连接、Upgrade 机制、h2/h3 的真实差异讲清楚。

## 一、先分清"两段连接"（这是关键）

线上部署真实链路是这样的：

```
浏览器 ──(① 公网)──> Nginx ──(② 内网)──> Go 后端
```

| 段 | 协议 | 说明 |
|---|---|---|
| ① 浏览器 → Nginx | 可以 h2 / h3 | Nginx 终止 TLS，对客户端协商 |
| ② Nginx → Go | **HTTP/1.1** | `proxy_http_version 1.1` 说的就是这段 |

**这两段是独立的。** 面试时先说这句，立刻显得你懂部署。

## 二、`proxy_http_version 1.1` 到底在解决什么

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;                      # ← 必须
    proxy_set_header Upgrade $http_upgrade;      # ← 必须
    proxy_set_header Connection "upgrade";       # ← 必须
    proxy_read_timeout 3600s;
}
```

### 三行为什么缺一不可

| 配置 | 作用 | 不写的后果 |
|---|---|---|
| `proxy_http_version 1.1` | Nginx 默认对上游用 **HTTP/1.0** | 1.0 没有 `Upgrade` 机制 → 握手失败 / 秒断 |
| `Upgrade $http_upgrade` | 把客户端的 `Upgrade: websocket` 透传给后端 | 后端收不到升级请求 → 400 |
| `Connection "upgrade"` | 告诉上游这是升级连接 | 同上 |

> 注意 `Connection` 写死成 `"upgrade"` 而不是 `$http_connection` 是有意为之：
> 只对 WebSocket 的 location 生效，避免影响普通请求的长连接行为。

## 三、为什么 WebSocket 必须依赖 HTTP/1.1

### 3.1 WebSocket 借用了 HTTP/1.1 的「协议升级」

```
GET /ws?token=xxx HTTP/1.1
Host: example.com
Connection: Upgrade            ← 1.1 的升级机制
Upgrade: websocket
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
```

服务端返回 `101 Switching Protocols` 后，同一条 TCP 连接开始传 WebSocket 帧。

### 3.2 HTTP/2 明确禁止这些头

RFC 7540 §8.1.2 规定：HTTP/2 消息**不得包含 connection-specific 头字段**，包括：

```
Connection、Upgrade、Keep-Alive、Transfer-Encoding、Proxy-Connection
```

收到就视为 **malformed**（格式错误）。

所以：**在 h2 连接上根本发不出 `Upgrade: websocket` 握手**。

### 3.3 那 HTTP/2 到底能不能跑 WebSocket

能，但要走另一套标准：**RFC 8441（Bootstrapping WebSockets with HTTP/2）**，
用 `Extended CONNECT` + `:protocol = websocket` 伪头代替 Upgrade。

现实约束：

| 约束 | 现状 |
|---|---|
| 浏览器支持 | 较新版本才支持，且默认开启情况不一 |
| 代理支持 | Nginx 需要较新版本 + 额外配置 |
| **服务端库支持** | 本项目的 `gorilla/websocket` **不支持** RFC 8441 |

所以工程实践仍然是：**WebSocket 走 HTTP/1.1 升级**。

## 四、HTTP/1.1 / HTTP/2 / HTTP/3 对比

| 维度 | HTTP/1.1 | HTTP/2 | HTTP/3 |
|---|---|---|---|
| 传输层 | TCP | TCP | **QUIC（基于 UDP）** |
| 多路复用 | ❌ 一个连接同时只能一个请求（靠 6 个并发连接凑） | ✅ Stream 并发 | ✅ 更彻底 |
| 队头阻塞 | HTTP 层有 | **TCP 层仍有**：一个丢包阻塞该连接所有流 | ✅ 流之间独立，无 HOL |
| 头部压缩 | ❌ 每次重复发完整头 | HPACK | QPACK |
| 传输格式 | 文本 | 二进制分帧 | 二进制分帧 |
| 建连 | TCP + TLS（2~3 RTT） | 同 1.1 | QUIC 把 TLS 集成进握手（1 RTT，可 0 RTT） |
| 服务端推送 | ❌ | 有，但**已废弃**（Chrome 106 移除） | ❌ |
| 真实用途 | WebSocket 升级、老客户端兼容 | **gRPC**、Web 资源加速 | 移动弱网、跨境 |

### 一句话记住 h2 的局限性

> h2 的多路复用只解决了「HTTP 层」的队头阻塞；
> 底层还是**一条 TCP 连接**，**丢一个包所有流一起等** → 所以才有 h3/QUIC。

## 五、那浏览器 → Nginx 能不能用 HTTP/2？

**能，而且推荐。** Nginx 同一端口同时支持 h2 和 h1.1（ALPN 协商）：

```nginx
server {
    listen 443 ssl;
    http2 on;              # Nginx ≥ 1.25.1 的写法（旧写法 listen 443 ssl http2;）
    server_name chat.example.com;

    ssl_certificate     /etc/letsencrypt/live/chat.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;

    # 普通请求：Nginx 与客户端可用 h2
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket：客户端会单独用 h1.1 建连，上游固定 1.1
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

### 为什么浏览器会"自动"用 1.1 连 WebSocket

浏览器的 `new WebSocket('wss://...')` 目前仍按 HTTP/1.1 升级实现（RFC 8441 未普遍启用），
所以它会在这条连接上协商 `http/1.1`；而普通 `fetch` 请求走 h2。Nginx 因为 ALPN 同时支持两种协议，所以能同时服务两者。

### 常见坑

| 现象 | 原因 |
|---|---|
| 配了 h2 后 WebSocket 连不上 | 有些反代/负载均衡**只允许 h2**，把 1.1 升级请求拒了；确认 ALPN 同时允许 `h2,http/1.1` |
| `listen ... http2` 报 warning | Nginx ≥ 1.25.1 已改用 `http2 on;` |
| 握手 400/502 | 漏了 `proxy_http_version 1.1` 或 `Upgrade`/`Connection` 头 |
| 连上 60 秒断 | `proxy_read_timeout` 用的是默认 60s |

## 六、本项目里 HTTP/2 其实已经用上了

| 位置 | 协议 | 说明 |
|---|---|---|
| 浏览器 → Go（HTTP API） | HTTP/1.1 | 前面有 Nginx，内网明文够用 |
| 浏览器 → Go（WebSocket） | HTTP/1.1 升级 | 受 RFC 7540/8441 限制 |
| **Go ↔ Go（gRPC，M6）** | **HTTP/2** | gRPC 强制基于 h2：多路复用 + 流式 + 二进制帧 |
| 前端静态资源（经 Nginx） | 可 h2/h3 | 多路复用省连接数 |

**面试可以这样说**：
> 对外，WebSocket 需要 1.1 的 Upgrade，所以反代到后端用 1.1；但客户端到 Nginx 那一段我开了 h2，
> 静态资源和 API 走多路复用。
> 而项目内部的 gRPC 服务间通信本身就是 HTTP/2，所以"用没用 h2"要分场景说。

## 七、什么时候该升级协议

| 场景 | 建议 |
|---|---|
| 静态资源多、API 密集的前端 | ✅ h2（省连接、省头部） |
| WebSocket 实时通信 | ❌ 仍走 h1.1 升级 |
| 移动端 / 弱网 / 跨境 | ✅ h3（QUIC 抗丢包） |
| 内部服务调用 | ✅ gRPC（h2） |
| 老旧客户端、简单内网 | 保持 h1.1，成本最低 |

### Go 服务开启 h2 的方式（了解）

```go
// 1. TLS 下自动启用 h2（标准库行为）
http.ListenAndServeTLS(":443", cert, key, handler)

// 2. 明文 h2c 需要显式包装
h2s := &http2.Server{}
http.ListenAndServe(":8080", h2c.NewHandler(handler, h2s))
```

本项目选择「Nginx 终止 TLS + 内网 h1.1」，因为：运维简单、WS 兼容、内网无需多路复用收益。

## 八、面试问答（10 题）

**Q1. 你们为什么用 HTTP/1.1，不用 HTTP/2？**

> 要分两段说：客户端到 Nginx 是可以用 h2 的；Nginx 到后端用 1.1，是因为 WebSocket 的 Upgrade 握手依赖 1.1，
> 而 HTTP/2 禁止 `Connection`/`Upgrade` 头。另外内部的 gRPC 通信本身就是 h2。

**Q2. 为什么 HTTP/2 不能跑 WebSocket？**

> RFC 7540 禁止 h2 消息携带 connection-specific 头（Connection、Upgrade 等），所以发不出握手。
> RFC 8441 定义了用 Extended CONNECT（`:protocol: websocket`）在 h2 上建 WebSocket，但浏览器和库支持不普遍，
> gorilla/websocket 就不支持。

**Q3. HTTP/2 相比 1.1 提升了什么？**

> 多路复用（一条连接并发多流）、二进制分帧、HPACK 头部压缩、可选的流优先级；服务端推送已废弃。
> 注意它没有解决 TCP 层队头阻塞。

**Q4. HTTP/3 解决什么问题？**

> h2 的队头阻塞在 TCP 层：一个丢包会阻塞整条连接的所有流。h3 把传输换成 QUIC（UDP），
> 每个流独立，丢包只影响那个流；并且把 TLS 握手集成进去，弱网/移动场景建连更快。

**Q5. 多路复用会不会有队头阻塞？**

> HTTP 层没有，TCP 层有。所以严格说 h2 是"解决了 HTTP 层 HOL，遗留 TCP 层 HOL"。

**Q6. `proxy_http_version 1.1` 是配置哪一段？**

> Nginx 到上游服务那一段。Nginx 对上游默认 1.0，而 1.0 没有 Upgrade 机制、默认短连接，
> 所以 WebSocket 必须显式设 1.1 并透传 Upgrade/Connection 头。

**Q7. gRPC 为什么基于 HTTP/2？**

> 需要多路复用（一条连接并发多个 RPC）、流式语义（server/client/bidi streaming）、二进制帧和头部压缩。
> h1.1 无法在一条连接上并发多个请求。

**Q8. 上 h2 有什么代价？**

> 运维复杂度（TLS 必须、ALPN、反代配置）、调试工具少（tcpdump 看到的是二进制帧）、
> 服务器要支持 h2；还有"一条 TCP 连接"意味着多路复用共享拥塞窗口，可能需要连接池调优。

**Q9. 什么场景不建议上 h2？**

> 内部简单调用（用 h1.1 够）、需要 WebSocket 升级的场景（仍走 1.1）、以及无法控制客户端的老系统。

**Q10. 如果给你一个高并发 IM 场景，你怎么选协议？**

> WebSocket 长连接走 h1.1 升级（或将来 RFC 8441 成熟后走 h2）；
> 客户端到网关开 h2/h3 优化静态资源与 API；
> 服务间 gRPC 走 h2；
> 推送侧用 MQ 解耦，不依赖 HTTP。

## 九、配置检查清单

- [ ] `location /ws` 有 `proxy_http_version 1.1`
- [ ] 有 `Upgrade $http_upgrade` 和 `Connection "upgrade"`
- [ ] `proxy_read_timeout` 调大（默认 60s 会断长连接）
- [ ] 客户端到 Nginx 的 `listen` 同时支持 h2 和 http/1.1（ALPN）
- [ ] 用 `curl -I --http2 https://域名/` 验证 h2 生效
- [ ] 用浏览器 F12 → Network 看 `/ws` 是 101 且协议是 `websocket`（h1.1）

---

相关：[WebSocket 全解](/blog/2026/11/14/02-04-WebSocket全解/)、[架构总览](/blog/2026/11/14/02-01-架构总览/)、[部署指南 DEPLOY.md](https://github.com/TheGodIsAChild/gochat/blob/main/DEPLOY.md)
