---
title: "源码走读 S07：token_store.go"
date: 2026-11-14 23:15:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/auth/token_store.go`　**行数**：75 行
> **职责**：Refresh token 的 Redis 存储：GETDEL 原子消费、SCAN 批量删

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"gochat/global"
	"time"

	"github.com/redis/go-redis/v9"
)

// 生成redis Key： refresh:{userID}:{jti}
func refreshKey(userID uint64, jti string) string {
	return fmt.Sprintf("refresh:%d:%s", userID, jti)
}

// 登录时将 refresh token 的jti写入 Redis
func SaveRefreshKeytoRedis(ctx context.Context, userID uint64, jti string, ttl time.Duration) error {
	// "1" 为标识值，表示 jti还在可用状态
	return global.Redis.Set(ctx, refreshKey(userID, jti), "1", ttl).Err()
}

// 检查jti是否还有效
func ExistsRefreshKey(ctx context.Context, userID uint64, jti string) (bool, error) {
	n, err := global.Redis.Exists(ctx, refreshKey(userID, jti)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// 退出登录或登录过期时删除Redis jti
func DeleteRefreshJtiinRedis(ctx context.Context, userID uint64, jti string) error {
	return global.Redis.Del(ctx, refreshKey(userID, jti)).Err()
}

// 为什么 DeleteAllRefresh 用 SCAN 而不是 KEYS？—— 面试重要考点

// KEYS refresh:* 会一次性遍历所有 key，Redis 单线程 → 数据量大时阻塞整个 Redis，线上事故常见
// SCAN 用游标分批（每次 100 个），不阻塞；缺点是可能漏/重复（所以通常配合去重或重试）
// 面试问"Redis 怎么删大量 key"必答：SCAN + 批量 DEL，别用 KEYS
func DeleteAllRefreshJtiinRedis(ctx context.Context, userID uint64) error {
	pattern := fmt.Sprintf("refresh:%d:*", userID)
	var cursor uint64
	for {
		keys, next, err := global.Redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := global.Redis.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

var ErrRefreshNotFound error = errors.New("refresh token 不存在")

// ConsumeRefreshKey 原子地"取出并删除" jti（GETDEL）
// 返回 ErrRefreshNotFound 表示 jti 不存在（已被用/被撤销）
func ConsumeRefreshKey(ctx context.Context, userID uint64, jti string) error {
	_, err := global.Redis.GetDel(ctx, refreshKey(userID, jti)).Result()
	if errors.Is(err, redis.Nil) {
		return ErrRefreshNotFound
	}
	return err
}

```
