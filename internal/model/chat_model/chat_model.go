package chatmodel

import (
	"time"

	"gochat/internal/model"
)

// Conversation 会话。
//
// 【设计要点：单聊和群聊共用一张会话表】
//   - 单聊：Type=1，conversation_members 里有 2 个成员
//   - 群聊：Type=2，成员多个，GroupID 关联 groups.id（M3 使用）
//
// 这样消息表不需要区分「单聊消息 / 群聊消息」，统一按 ConversationID 查询。
// 这是微信/IM 系统的通用做法：**会话（conversation）+ 消息（message）** 两层抽象。
//
// 【为什么冗余 LastMessageID / LastMessageAt】
// 「会话列表」要展示每个会话的最后一条消息和时间。如果每次列表都去 messages 表
// 按 conversation_id 查最新一条（子查询/排序），数据量大时很慢。
// 把最后消息信息冗余到会话行上，列表查询就变成一次简单查询（空间换时间）。
type Conversation struct {
	model.Model
	Type          int8       `gorm:"type:tinyint;not null;default:1;index" json:"type"` // 1单聊 2群聊
	GroupID       uint       `gorm:"not null;default:0" json:"group_id"`                // 群聊关联 groups.id（M3）
	LastMessageID uint       `gorm:"not null;default:0" json:"last_message_id"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
}

func (Conversation) TableName() string { return "conversations" }

// ConversationMember 会话成员（单聊/群聊通用）。
//
// 【关键字段 LastReadMessageID】
// 记录「我在这个会话里读到哪条消息了」，用于计算未读数：
//   未读数 = 会话里 id > LastReadMessageID 且 sender != 我 的消息数
// 比单独建一张 message_reads 表简单很多（M2 先用这个方案）。
//
// 【索引设计】
//   - uk_conv_user(conversation_id, user_id) 唯一：同一会话同一用户只能有一条成员记录
//   - idx_user(user_id)：查「我的所有会话」时用 user_id 查
type ConversationMember struct {
	model.Model
	ConversationID    uint `gorm:"not null;uniqueIndex:uk_conv_user" json:"conversation_id"`
	UserID            uint `gorm:"not null;uniqueIndex:uk_conv_user;index:idx_user" json:"user_id"`
	Role              int8 `gorm:"type:tinyint;not null;default:0" json:"role"` // 0成员 1群主（M3）
	Mute              int8 `gorm:"type:tinyint;not null;default:0" json:"mute"` // 0正常 1免打扰
	LastReadMessageID uint `gorm:"not null;default:0" json:"last_read_message_id"`
}

func (ConversationMember) TableName() string { return "conversation_members" }

// Message 消息。
//
// 【幂等设计：ClientMsgID】
// 客户端发消息时带一个自己生成的唯一 ID（如 UUID）。网络重试/用户连点会导致同一消息
// 被发送多次，服务端用 UNIQUE(sender_id, client_msg_id) 兜底：
//   - 第一次插入成功
//   - 重复插入报 Duplicate → 服务端查出已有消息直接返回（幂等）
// 这是 IM 里「消息不重复」的核心机制。
//
// 【为什么类型用 tinyint 而不是 enum】
// tinyint 扩展方便（加新类型不用 ALTER 枚举），且占用小。
//
// 【为什么内容用 text】
// 文本消息可能较长；图片/语音/文件消息存的是 URL/元信息 JSON，也能放下。
type Message struct {
	model.Model
	ConversationID uint   `gorm:"not null;index:idx_msg_conv" json:"conversation_id"`
	SenderID       uint   `gorm:"not null;index:idx_msg_sender" json:"sender_id"`
	ClientMsgID    string `gorm:"type:varchar(64);not null;uniqueIndex:uk_sender_client" json:"client_msg_id"`
	Type           int8   `gorm:"type:tinyint;not null;default:1" json:"type"` // 1文本 2图片 3语音 4文件
	Content        string `gorm:"type:text;not null" json:"content"`
	Status         int8   `gorm:"type:tinyint;not null;default:1" json:"status"` // 1正常 2撤回
}

func (Message) TableName() string { return "messages" }
