package callmodel

import (
	"time"

	"gochat/internal/model"
)

// Call 通话记录。
//
// 【WebRTC 的服务器角色】
// WebRTC 是 P2P 的：音视频流直接在两个浏览器之间传输，不经过服务器。
// 服务器只负责两件事：
//  1. 【信令】帮双方交换 SDP（offer/answer）和 ICE candidate（NAT 穿透信息）
//  2. 【状态】记录通话状态（呼叫中/已接通/已拒绝/已结束）和时长
//
// 所以本表记录的是「信令状态机」，不是传输内容。
//
// 【状态机】
//
//	1 呼叫中 ──接听──► 2 已接通 ──挂断──► 5 已结束
//	   │
//	   ├──被叫拒绝──► 3 已拒绝
//	   ├──主叫取消──► 4 已取消
//	   └──超时无人接──► 6 未接
//
// 状态迁移全部用「条件更新 + RowsAffected」保证并发安全（见 call_service）。
type Call struct {
	model.Model
	CallerID       uint       `gorm:"not null;index" json:"caller_id"`
	CalleeID       uint       `gorm:"not null;index" json:"callee_id"`
	ConversationID uint       `gorm:"not null;default:0" json:"conversation_id"` // 关联的会话（可选）
	Type           int8       `gorm:"type:tinyint;not null;default:1" json:"type"`    // 1音频 2视频
	Status         int8       `gorm:"type:tinyint;not null;default:1;index" json:"status"` // 见上面状态机
	StartedAt      *time.Time `json:"started_at,omitempty"` // 接通时间
	EndedAt        *time.Time `json:"ended_at,omitempty"`   // 结束时间
	Duration       int        `gorm:"not null;default:0" json:"duration"` // 通话时长（秒）
}

func (Call) TableName() string { return "call_records" }
