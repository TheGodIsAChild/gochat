---
title: Redis 在本项目的 5 种用法
date: 2026-11-14 23:49:00
categories:
  - 02-设计
tags:
  - Redis
  - 缓存
  - 原子操作
---

> Redis 不是只能做「缓存」。在这个项目里它是**认证白名单、原子操作工具、限流计数器**。
> 本文逐个讲用法、命令、为什么，以及线上要注意什么。

## 一、用法总览

| 用法 | 数据结构 | 命令 | 代码位置 |
|---|---|---|---|
| 1. refresh token 白名单 | String + TTL | `SET / GETDEL / DEL` | `auth/token_store.go` |
| 2. 批量删除用户 token | SCAN | `SCAN + DEL` | 同上 |
| 3. 原子消费（防并发刷新） | - | `GETDEL` | 同上 |
| 4. 限流（当前为内存版） | - | 对比实现 | `middleware/ratelimit.go` |
| 5. 在线状态（可扩展） | String + TTL | `SET EX / EXISTS` | `ws/hub.go`（内存版） |

## 二、用法 1：refresh token 白名单

### 为什么需要

JWT 是**无状态**的——签出去就收不回。要支持「登出立即失效」，必须有服务端记录。

### key 设计

```
refresh:{userID}:{jti}  →  "1"   TTL = 30 天
```

- 冒号分层：`业务:对象:标识`
- 带 userID：方便按用户批量清理
- 带 TTL：与 refresh token 的 `exp` 对齐，自动过期

### 命令

```
SET refresh:1:abc123 1 EX 2592000    # 登录时写
EXISTS refresh:1:abc123              # 校验
DEL refresh:1:abc123                 # 登出时删
```

### 代码

```go
func refreshKey(userID uint64, jti string) string {
    return fmt.Sprintf("refresh:%d:%s", userID, jti)
}

func SaveRefreshKeytoRedis(ctx context.Context, userID uint64, jti string, ttl time.Duration) error {
    return global.Redis.Set(ctx, refreshKey(userID, jti), "1", ttl).Err()
}
```

## 三、用法 2：GETDEL 原子消费

### 问题：Exists + Del 不是原子的

```
请求 A: EXISTS → 1
请求 B: EXISTS → 1          ← 两个都通过
请求 A: DEL → 签发新 token
请求 B: DEL → 也签发新 token   ← rotation 失效
```

### 解法：GETDEL

```go
// 取出来就删除，只有一个请求能拿到值
val, err := global.Redis.GetDel(ctx, key).Result()
if errors.Is(err, redis.Nil) {
    // jti 不存在 → 已被消费/已撤销 → 可能被盗用
}
```

`GETDEL` 是 Redis 6.2+ 的原子命令（等价的 `GET` + `DEL` 在一个操作内完成）。

## 四、用法 3：SCAN 批量删除

### 需求

「强制该用户所有设备下线」= 删除所有 `refresh:{userID}:*`。

### 为什么不能用 KEYS

```
KEYS refresh:1:*     # ❌ 一次性遍历所有 key，单线程 Redis 直接阻塞
```

Redis 是单线程的，`KEYS` 在数据量大时会让整个实例卡住（线上事故）。

### 正确写法

```go
var cursor uint64
for {
    keys, next, err := global.Redis.Scan(ctx, cursor, "refresh:1:*", 100).Result()
    if len(keys) > 0 { global.Redis.Del(ctx, keys...) }
    cursor = next
    if cursor == 0 { break }
}
```

- `SCAN` 用游标分批（每批 100 个），不阻塞
- 缺点：可能重复/漏（需容忍，或多次扫描）
- **面试标准答案**：删大量 key 用 SCAN + 批量 DEL，不要用 KEYS

## 五、用法 4：限流

### 当前实现（单机内存）

```go
type rateLimiter struct {
    mu     sync.Mutex
    hits   map[string][]time.Time   // IP -> 请求时间列表
    limit  int
    window time.Duration
}
```

滑动窗口 + 懒清理，20 次/分钟。**只适合单实例**。

### Redis 版（多实例正确做法）

```
# 方案 A：固定窗口计数
INCR rate:{ip}:{minute}
EXPIRE rate:{ip}:{minute} 60

# 方案 B：滑动窗口（ZSET 存时间戳）
ZADD rate:{ip} {now} {now}
ZREMRANGEBYSCORE rate:{ip} 0 {now-60000}
ZCARD rate:{ip}

# 方案 C：令牌桶（Lua 脚本原子执行）
```

| 方案 | 精度 | 复杂度 |
|---|---|---|
| 固定窗口 | 边界翻倍 | 最简单 |
| 滑动窗口（ZSET） | 精确 | 内存占用高 |
| 令牌桶（Lua） | 允许突发 | 需要脚本 |

> **面试点**：为什么单机限流在多实例下会失效？→ 每个实例独立计数，总量翻倍。

## 六、用法 5：在线状态（可扩展）

### 当前实现：内存 Hub

```go
func (h *Hub) IsOnline(userID uint64) bool { ... }
```

**问题**：只能反映本进程的连接。多实例时 A 在实例 1，B 在实例 2，互相看不到。

### Redis 版

```
登录/连接:  SET online:{userID} 1 EX 60
心跳续期:   EXPIRE online:{userID} 60
断开:      DEL online:{userID}
判断:      EXISTS online:{userID}
```

**TTL 的作用**：进程崩溃时，60s 后自动失效，不会出现「永久在线」的幽灵用户。

> 多端登录要改用 Set 或计数器：`SADD online:{uid} {connID}` + `SCARD`。

## 七、Redis 使用规范（线上要注意）

| 规范 | 原因 |
|---|---|
| **所有 key 都要有 TTL** | 防止内存无限增长 |
| key 用 `业务:对象:标识` | 可读、可批量匹配 |
| 禁止 `KEYS *` | 阻塞 Redis |
| 大 key 拆分 | 单个 value 过大会阻塞 |
| 用 pipeline 批量操作 | 减少 RTT |
| 分布式锁要设过期时间 | 防死锁 |
| 重要数据开 AOF 持久化 | 宕机不丢 |

## 八、Redis 在分布式场景还能做什么

| 场景 | 用法 |
|---|---|
| 分布式锁 | `SET key val NX EX 30` + Lua 校验释放 |
| 排行榜 | ZSET |
| 消息队列（轻量） | List / Stream |
| 缓存穿透/击穿/雪崩 | 布隆过滤器 / 空值缓存 / 随机 TTL |
| 限流 | 见上文 |

## 九、面试问法

1. Redis 在你的项目里用来做什么？
2. 为什么登出要删 Redis key？JWT 不是无状态吗？
3. `GETDEL` 解决什么问题？和 `GET + DEL` 有什么区别？
4. 为什么不能用 `KEYS`？`SCAN` 有什么缺点？
5. 单机限流和多实例限流有什么区别？
6. Redis 挂了你项目会怎样？（降级/失败）
7. 如何用 Redis 实现在线状态？为什么要 TTL？
8. Redis 的持久化方式？RDB vs AOF？
9. 缓存穿透/击穿/雪崩分别是什么？怎么解决？
10. 如何用 Redis 做分布式锁？有什么坑（锁过期、误删）？

---

相关：[双 Token 无感刷新](/blog/2026/11/14/04-02-认证与好友/)、[M1 详细设计](/blog/2026/11/14/04-09-M1详细设计/)
