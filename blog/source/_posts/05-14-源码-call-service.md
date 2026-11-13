---
title: "源码走读 S14：call_service.go"
date: 2026-11-14 23:06:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/service/call_service/call_service.go`　**行数**：247 行
> **职责**：通话状态机：invite/accept/reject/cancel/end + 时长计算

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
// Package callservice 语音/视频通话的信令状态机。
//
// 【重要：服务器不传音视频】
// WebRTC 的音视频是 P2P 直连的，服务器只做「信令转发」和「状态记录」。
// 信令（offer/answer/ice）通过 WebSocket 转发，见 internal/ws/client.go。
package callservice

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"gochat/global"
	callmodel "gochat/internal/model/call_model"
	usermodel "gochat/internal/model/user_model"
)

const (
	CallStatusCalling  int8 = 1 // 呼叫中
	CallStatusAccepted int8 = 2 // 已接通
	CallStatusRejected int8 = 3 // 已拒绝
	CallStatusCanceled int8 = 4 // 已取消
	CallStatusEnded    int8 = 5 // 已结束
	CallStatusMissed   int8 = 6 // 未接

	CallTypeAudio int8 = 1
	CallTypeVideo int8 = 2
)

// Invite 主叫发起通话。
//
// 【校验】
//  1. 不能打给自己
//  2. 被叫必须存在
//  3. 双方之间不能已有进行中的通话（呼叫中/已接通）
func Invite(ctx context.Context, callerID, calleeID uint64, callType int8) (*callmodel.Call, error) {
	if callerID == calleeID {
		return nil, errors.New("不能呼叫自己")
	}
	if callType != CallTypeAudio && callType != CallTypeVideo {
		callType = CallTypeAudio
	}

	var callee usermodel.User
	if err := global.DB.WithContext(ctx).First(&callee, calleeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("用户不存在")
		}
		return nil, err
	}

	// 是否已有进行中的通话
	var ongoing int64
	if err := global.DB.WithContext(ctx).Model(&callmodel.Call{}).
		Where("status IN ?", []int8{CallStatusCalling, CallStatusAccepted}).
		Where("(caller_id = ? AND callee_id = ?) OR (caller_id = ? AND callee_id = ?)",
			callerID, calleeID, calleeID, callerID).
		Count(&ongoing).Error; err != nil {
		return nil, err
	}
	if ongoing > 0 {
		return nil, errors.New("你们之间已有进行中的通话")
	}

	call := &callmodel.Call{
		CallerID: uint(callerID),
		CalleeID: uint(calleeID),
		Type:     callType,
		Status:   CallStatusCalling,
	}
	if err := global.DB.WithContext(ctx).Create(call).Error; err != nil {
		return nil, err
	}
	return call, nil
}

// Accept 被叫接听。
//
// 【并发安全】用条件更新：只有「呼叫我、且仍在呼叫中」才能接通。
// 被叫连点两次、或超时后又被接，都会被 RowsAffected=0 拦住。
func Accept(ctx context.Context, callID, userID uint64) (*callmodel.Call, error) {
	now := time.Now()
	res := global.DB.WithContext(ctx).Model(&callmodel.Call{}).
		Where("id = ? AND callee_id = ? AND status = ?", callID, userID, CallStatusCalling).
		Updates(map[string]any{
			"status":     CallStatusAccepted,
			"started_at": now,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("通话不存在或状态不允许接听")
	}

	var call callmodel.Call
	if err := global.DB.WithContext(ctx).First(&call, callID).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

// Reject 被叫拒绝。
func Reject(ctx context.Context, callID, userID uint64) error {
	now := time.Now()
	res := global.DB.WithContext(ctx).Model(&callmodel.Call{}).
		Where("id = ? AND callee_id = ? AND status = ?", callID, userID, CallStatusCalling).
		Updates(map[string]any{
			"status":   CallStatusRejected,
			"ended_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("通话不存在或状态不允许拒绝")
	}
	return nil
}

// Cancel 主叫取消（还没接通时）。
func Cancel(ctx context.Context, callID, userID uint64) error {
	now := time.Now()
	res := global.DB.WithContext(ctx).Model(&callmodel.Call{}).
		Where("id = ? AND caller_id = ? AND status = ?", callID, userID, CallStatusCalling).
		Updates(map[string]any{
			"status":   CallStatusCanceled,
			"ended_at": now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("通话不存在或状态不允许取消")
	}
	return nil
}

// End 挂断（双方都可以，只有已接通的通话才能挂断）。
//
// 【时长计算】duration = ended_at - started_at，单位秒。
func End(ctx context.Context, callID, userID uint64) (*callmodel.Call, error) {
	var call callmodel.Call
	if err := global.DB.WithContext(ctx).First(&call, callID).Error; err != nil {
		return nil, errors.New("通话不存在")
	}
	if uint64(call.CallerID) != userID && uint64(call.CalleeID) != userID {
		return nil, errors.New("无权操作该通话")
	}

	now := time.Now()
	duration := 0
	if call.StartedAt != nil {
		duration = int(now.Sub(*call.StartedAt).Seconds())
		if duration < 0 {
			duration = 0
		}
	}

	res := global.DB.WithContext(ctx).Model(&callmodel.Call{}).
		Where("id = ? AND status = ?", callID, CallStatusAccepted).
		Updates(map[string]any{
			"status":   CallStatusEnded,
			"ended_at": now,
			"duration": duration,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("通话状态不允许挂断")
	}

	if err := global.DB.WithContext(ctx).First(&call, callID).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

// CallItem 通话记录 DTO。
//
// 【为什么要 DTO 而不是直接返回 model】
// model.Model 嵌入了 gorm.Model（ID/CreatedAt/UpdatedAt/DeletedAt），
// 这些字段没有 json tag，直接序列化会输出 "ID"、"DeletedAt" 这种大写字段，
// 还会把 DeletedAt 暴露给前端。用 DTO 明确控制返回哪些字段。
type CallItem struct {
	ID        uint64     `json:"id"`
	CallerID  uint64     `json:"caller_id"`
	CalleeID  uint64     `json:"callee_id"`
	Type      int8       `json:"type"`
	Status    int8       `json:"status"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Duration  int        `json:"duration"`
	CreatedAt time.Time  `json:"created_at"`
}

// ToCallItem 模型 → DTO。
func ToCallItem(c *callmodel.Call) CallItem {
	return CallItem{
		ID:        uint64(c.ID),
		CallerID:  uint64(c.CallerID),
		CalleeID:  uint64(c.CalleeID),
		Type:      c.Type,
		Status:    c.Status,
		StartedAt: c.StartedAt,
		EndedAt:   c.EndedAt,
		Duration:  c.Duration,
		CreatedAt: c.CreatedAt,
	}
}

// Get 查询通话详情（只允许通话双方查看）。
func Get(ctx context.Context, callID, userID uint64) (*CallItem, error) {
	var call callmodel.Call
	if err := global.DB.WithContext(ctx).First(&call, callID).Error; err != nil {
		return nil, errors.New("通话不存在")
	}
	if uint64(call.CallerID) != userID && uint64(call.CalleeID) != userID {
		return nil, errors.New("无权查看该通话")
	}
	item := ToCallItem(&call)
	return &item, nil
}

// ListHistory 我的通话记录（作为主叫或被叫）。
func ListHistory(ctx context.Context, userID uint64, limit int) ([]CallItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var calls []callmodel.Call
	if err := global.DB.WithContext(ctx).
		Where("caller_id = ? OR callee_id = ?", userID, userID).
		Order("id DESC").
		Limit(limit).
		Find(&calls).Error; err != nil {
		return nil, err
	}
	items := make([]CallItem, 0, len(calls))
	for i := range calls {
		items = append(items, ToCallItem(&calls[i]))
	}
	return items, nil
}

```
