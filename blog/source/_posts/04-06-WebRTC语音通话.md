---
title: WebRTC 语音通话实现
date: 2026-11-14 23:34:00
categories:
  - 04-里程碑
tags:
  - WebRTC
  - 信令
  - P2P
---

## 一、WebRTC 是 P2P 的

```
浏览器 A ◄══════ 音视频流（P2P，UDP）══════► 浏览器 B
     │                                          │
     └────────► 信令服务器（WebSocket）◄────────┘
               （只转发 SDP/ICE，不传音视频）
```

**服务器不传音视频**——这是 WebRTC 相比服务器混流的最大优势：延迟低、带宽成本低。

## 二、建立连接的三个步骤

1. **交换 SDP**
   - A 生成 offer（支持的编解码、网络信息）
   - B 返回 answer
2. **交换 ICE candidate**
   - 双方收集可能的网络路径
3. **P2P 打洞 / 中继**
   - 能直连就直连，失败用 TURN 中继

### STUN / TURN / ICE

| 概念 | 作用 |
|---|---|
| ICE | 框架：收集候选 + 按优先级尝试连接 |
| STUN | 发现自己的公网 IP:Port（NAT 映射） |
| TURN | 打洞失败时的中继服务器（有带宽成本） |

## 三、服务器职责：信令状态机

```
1 呼叫中 ──接听──► 2 已接通 ──挂断──► 5 已结束
   ├──被叫拒绝──► 3 已拒绝
   ├──主叫取消──► 4 已取消
   └──超时──► 6 未接
```

状态迁移全部用**条件更新**：

```go
res := tx.Model(&Call{}).
    Where("id = ? AND callee_id = ? AND status = ?", callID, userID, CallStatusCalling).
    Updates(map[string]any{"status": CallStatusAccepted, "started_at": now})
if res.RowsAffected == 0 {
    return errors.New("通话不存在或状态不允许接听")
}
```

连点接听、超时后接、已挂断再挂断，都会被拦住。

## 四、信令协议（WebSocket）

| 方向 | type | 说明 |
|---|---|---|
| C→S | call_invite | 发起通话 |
| C→S | call_accept / call_reject / call_cancel / call_end | 状态操作 |
| C→S | webrtc_offer / webrtc_answer / webrtc_ice | 转发信令 |
| S→C | call_ack / call_incoming / call_accepted / call_rejected / call_canceled / call_ended | 状态通知 |
| S→C | webrtc_offer / webrtc_answer / webrtc_ice | 原样转发 + from_user_id |

### 信令转发实现

```go
func (c *Client) handleRelay(raw []byte, in inMessage) {
    var m map[string]any
    json.Unmarshal(raw, &m)
    m["from_user_id"] = c.UserID   // 加上发送者
    c.hub.SendToUser(in.ToUserID, mustJSON(m))
}
```

**服务器不解析 SDP**，只加 `from_user_id` 原样转发——这就是信令服务器的本质。

## 五、完整呼叫流程

```
A                     服务器                      B
│  call_invite ────────►│                         │
│◄──── call_ack ────────│──── call_incoming ─────►│
│                       │◄─── call_accept ─────────│
│◄── call_accepted ─────│──── call_accepted ──────►│
│── webrtc_offer ──────►│──── webrtc_offer ───────►│
│◄──── webrtc_answer ───│◄─── webrtc_answer ───────│
│── webrtc_ice ────────►│──── webrtc_ice ─────────►│
│      ===== P2P 音视频直连 =====                   │
│── call_end ──────────►│──── call_ended ─────────►│
```

## 六、前端实现要点

- `getUserMedia({ audio: true })` 采集麦克风
- `RTCPeerConnection` 建立连接
- `onicecandidate` → 通过 WS 发给对方
- `ontrack` → 把对方音频挂到 `<audio>` 播放
- **localhost 是安全上下文**，所以本地 HTTP 也能采集麦克风
- 本机两窗口演示可不用 STUN（host candidate 直连）；跨网需 STUN/TURN

## 七、面试点

1. WebRTC 原理？为什么是 P2P？
2. 信令服务器的作用？为什么必须要信令？
3. SDP / ICE / STUN / TURN？
4. 为什么需要 TURN？成本？
5. 通话状态机怎么设计？怎么防重复接听？
6. 群通话怎么实现？（SFU/MCU）
7. 通话超时无人接怎么处理？
