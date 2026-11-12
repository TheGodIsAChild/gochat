---
title: MySQL 索引与查询优化实战
date: 2026-11-14 23:46:00
categories:
  - 02-设计
tags:
  - MySQL
  - 索引
  - 查询优化
  - 事务
---

> 为什么有的查询快、有的慢？索引到底怎么建？
> 本文结合本项目的真实 SQL，讲清楚索引原理和优化手段。

## 一、先理解索引是什么

MySQL InnoDB 的主索引是 **B+ 树**：

```
              [ 10 | 20 ]
             /     |      \
     [ 1 5 8 ]  [ 12 15 ]  [ 25 30 ]
        ↓          ↓           ↓
     叶子节点（存整行数据，且按主键有序）
```

关键结论：

1. **主键索引的叶子节点就是数据行**（聚簇索引）
2. **二级索引的叶子节点存的是主键值** → 查非索引列需要**回表**
3. **索引天然有序** → 范围查询/排序能用到
4. **最左前缀原则**：复合索引 `(a,b,c)` 只能从 a 开始用

## 二、本项目的索引清单

```sql
SHOW INDEX FROM messages;
```

| 表 | 索引 | 类型 | 支撑的查询 |
|---|---|---|---|
| users | `uk_username` | 唯一 | 登录 |
| users | `uk_phone` / `uk_email` | 唯一 | 手机/邮箱登录 |
| friend_requests | `uk_from_to` | 唯一 | 防重复申请 |
| friend_requests | `idx_to_status` | 普通 | 收申请列表 |
| friendships | `uk_user_friend` | 唯一 | 防重复好友 |
| friendships | `idx_user` | 普通 | 我的好友 |
| conversation_members | `uk_conv_user` | 唯一 | 会话成员 |
| conversation_members | `idx_user` | 普通 | 我的会话 |
| messages | `idx_msg_conv` | 普通 | 拉历史 |
| messages | `uk_sender_client` | 唯一 | **幂等** |

## 三、唯一索引：不只是约束，还是并发防线

```go
// 业务层查重（给用户友好提示）
if count > 0 { return errors.New("用户名已存在") }

// 但真正防并发的是数据库唯一索引：
// 两个请求同时通过查重 → 第二个 INSERT 报 Duplicate entry
```

```go
if errors.Is(err, gorm.ErrDuplicatedKey) {
    // 并发重复，返回已有数据（幂等）
}
```

**面试点**：「先查后写」有 TOCTOU 竞态，唯一索引是最后防线。

## 四、复合索引与最左前缀

```sql
-- 索引 idx_to_status (to_user_id, status)
SELECT * FROM friend_requests
WHERE to_user_id = 10 AND status = 0;    -- ✅ 用上索引
WHERE to_user_id = 10;                   -- ✅ 用上（最左前缀）
WHERE status = 0;                        -- ❌ 用不上（缺最左列）
```

**建索引的顺序原则**：等值查询列在前，范围/排序列在后；选择性高的列在前。

## 五、消息分页为什么这么写

```sql
SELECT * FROM messages
WHERE conversation_id = ? AND id < ?
ORDER BY id DESC LIMIT 20;
```

- `conversation_id` 走 `idx_msg_conv`
- `id` 是主键，二级索引叶子节点带主键 → 天然有序
- `ORDER BY id DESC` 不需要额外排序

**对比 offset 分页**：

```sql
-- ❌ 越翻越慢：要扫描并跳过前 1000 行
... LIMIT 20 OFFSET 1000;
```

## 六、回表与覆盖索引

```sql
-- user_id 上有索引，但查 nickname 需要回表
SELECT nickname FROM friendships WHERE user_id = 1;
```

如果写成：

```sql
-- 覆盖索引（索引里就有要的列，不回表）
SELECT user_id, friend_id FROM friendships WHERE user_id = 1;
```

**优化思路**：列表页只查索引里有的列（id/外键），再批量查详情。

## 七、N+1 查询怎么发现和优化

```go
// ❌ N+1：查 20 个好友 → 又查 20 次用户表
for _, f := range friends {
    db.First(&user, f.FriendID)
}

// ✅ 2 次查询：批量 IN
ids := collectFriendIDs(friends)
db.Where("id IN ?", ids).Find(&users)
```

**怎么发现**：GORM 日志里看 SQL 条数；生产用慢查询日志 / APM。

## 八、`LIKE` 与模糊搜索

```sql
SELECT * FROM users WHERE username LIKE '%alice%';   -- ❌ 全表扫描
SELECT * FROM users WHERE username LIKE 'alice%';    -- ✅ 前缀匹配能用索引
```

**为什么**：B+ 树只能从**前缀**定位，前面加 `%` 就没法定位起点。

**生产方案**：

| 量级 | 方案 |
|---|---|
| 小 | 前缀匹配 + LIMIT |
| 中 | MySQL 全文索引（ngram） |
| 大 | Elasticsearch |

## 九、事务与锁

### 本项目的两个事务

```go
// ① 注册：users + user_settings 必须成对
tx.Transaction(func(tx *gorm.DB) error {
    tx.Create(&user)
    tx.Create(&settings)
})

// ② 接受好友申请：改状态 + 插两条好友关系
tx.Transaction(func(tx *gorm.DB) error {
    // 条件更新（原子判断 + 修改）
    tx.Model(&FriendRequest{}).Where("id=? AND status=0", id).Update("status", 1)
    // 双向好友
    upsertFriendship(tx, a, b)
    upsertFriendship(tx, b, a)
})
```

### 乐观锁 vs 悲观锁

| 方式 | 实现 | 适用 |
|---|---|---|
| **乐观锁** | `WHERE version = ?` 或条件更新 | 冲突少（本项目用条件更新） |
| **悲观锁** | `SELECT ... FOR UPDATE` | 冲突多、必须串行 |

本项目的「条件更新 + RowsAffected」本质上是一种乐观并发控制：
不加锁，靠 `WHERE status=0` 判断是否被抢先处理。

## 十、慢查询排查流程

```
1. 开慢查询日志
   SET GLOBAL slow_query_log = 1;
   SET GLOBAL long_query_time = 0.2;

2. 找到慢 SQL → EXPLAIN 看执行计划
   EXPLAIN SELECT ...;

3. 看关键列
   type: ALL(全表) / index / range / ref / const(最好)
   key:  实际用的索引
   rows: 预估扫描行数
   Extra: Using filesort / Using temporary（尽量消除）

4. 优化
   加索引 / 改 SQL / 减少回表 / 分页改游标 / 冗余字段
```

## 十一、本项目可以怎么继续优化

| 现状 | 优化方向 |
|---|---|
| 未读数是 N 次 count | Redis 计数器 |
| 会话列表每次查群信息 | 会话表冗余群名 |
| 好友搜索 `LIKE '%x%'` | 前缀匹配或 ES |
| 消息表单表 | 按 conversation_id 分表/分库（长期） |
| count 大表 | 近似值 / 单独计数表 |

## 十二、面试问法

1. 为什么用 B+ 树而不是 B 树/哈希？
2. 聚簇索引和二级索引的区别？什么是回表？
3. 最左前缀原则？复合索引怎么设计顺序？
4. 唯一索引在并发中起什么作用？
5. 为什么 `LIKE '%x%'` 不走索引？
6. 游标分页 vs offset 分页？
7. N+1 查询怎么发现和解决？
8. 事务的 ACID？隔离级别？幻读怎么解决？
9. 乐观锁和悲观锁的区别？各自适用场景？
10. `EXPLAIN` 里你关注哪些字段？

---

相关：[数据库表设计](/blog/2026/11/14/02-02-数据库表设计/)、[并发编程实战](/blog/2026/11/14/02-03-并发编程实战/)
