package groupmodel

import "gochat/internal/model"

// Group 群信息。
//
// 【为什么单独建 groups 表，而不把所有信息塞进 conversations】
// conversations 是「会话」的通用抽象（单聊/群聊共用），只放和消息相关的东西。
// 群名称、群主、公告、人数上限这些是「群」特有的，单独建表更清晰，
// 也避免 conversations 表字段越来越多、单聊行全是空字段。
//
// 【关联方式】
// groups.conversation_id ←→ conversations.id
//   - 建群时先创建一条 Type=2 的 conversation，再创建 group 指向它
//   - 群消息还是走 messages.conversation_id，不需要区分单聊/群聊
//
// 【群成员存哪】
// 复用 conversation_members（它已经有 role/mute/last_read_message_id），
// 不再单独建 group_members，避免两套成员表要同步。
type Group struct {
	model.Model
	ConversationID uint   `gorm:"not null;uniqueIndex" json:"conversation_id"` // 关联会话
	OwnerID        uint   `gorm:"not null;index" json:"owner_id"`              // 群主
	Name           string `gorm:"type:varchar(64);not null" json:"name"`       // 群名称
	Avatar         string `gorm:"type:varchar(255);not null;default:''" json:"avatar"`
	Announcement   string `gorm:"type:varchar(512);not null;default:''" json:"announcement"` // 群公告
	MemberCount    int    `gorm:"not null;default:0" json:"member_count"`                    // 冗余人数（列表展示用）
	MaxMembers     int    `gorm:"not null;default:200" json:"max_members"`                   // 人数上限
	Status         int8   `gorm:"type:tinyint;not null;default:1" json:"status"`             // 1正常 2已解散
}

func (Group) TableName() string { return "groups" }
