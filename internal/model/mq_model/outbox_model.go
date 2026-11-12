package mqmodel

import (
	"time"

	"gochat/internal/model"
)

// Outbox 本地消息表（Transactional Outbox 模式）。
//
// 【解决什么问题】
// 「消息落库」和「发 MQ」是两个操作，无法用一个事务包住：
//   - 先落库后发 MQ：如果发 MQ 失败，消息永远不会被推送
//   - 先发 MQ 后落库：如果落库失败，推了一条不存在的消息
//
// 【Outbox 模式】
//   1. 业务事务里：写业务数据 + 写一条 outbox 记录（同一个本地事务，保证原子）
//   2. 后台 worker：轮询 outbox 里 status=0 的记录 → 发 MQ → 标记 status=1
//   3. 消费方：消费 MQ 消息，做实际处理
//
// 这样：即使 MQ 暂时不可用，outbox 记录还在，worker 会重试（至少一次投递）。
// 消费方需要做幂等（用消息 id 去重）。
//
// 【面试点】这是「最终一致性」最常用的实现，比 2PC 简单、比直接发 MQ 可靠。
type Outbox struct {
	model.Model
	EventType  string    `gorm:"type:varchar(64);not null;index" json:"event_type"` // 事件类型，如 message.created
	Payload    string    `gorm:"type:text;not null" json:"payload"`                // 事件内容（JSON）
	Status     int8      `gorm:"type:tinyint;not null;default:0;index" json:"status"` // 0待发送 1已发送 2失败
	RetryCount int       `gorm:"not null;default:0" json:"retry_count"`
	NextRetry  time.Time `gorm:"index" json:"next_retry"` // 下次重试时间
}

func (Outbox) TableName() string { return "outbox_messages" }

// 事件类型常量。
const (
	EventMessageCreated = "message.created"
)

// MessageCreatedPayload message.created 事件的内容。
//
// 【为什么不直接用 chatservice.MessageItem】
// outbox 在 service 层被写入、在 mq 层被消费。如果两边互相 import 会形成循环依赖。
// 用一个中立的「事件契约」结构体，生产方和消费方都依赖它，彼此解耦。
type MessageCreatedPayload struct {
	ID             uint64 `json:"id"`
	ConversationID uint64 `json:"conversation_id"`
	SenderID       uint64 `json:"sender_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Type           int8   `json:"type"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"`
}
