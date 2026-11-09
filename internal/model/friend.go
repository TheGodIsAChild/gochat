package model

import "time"

// 好友申请状态
const (
	FriendRequestPending  int8 = 0 // 待处理
	FriendRequestAccepted int8 = 1 // 已接受
	FriendRequestRejected int8 = 2 // 已拒绝
)

// FriendRequest 对应 friend_requests 表（好友申请）。
//
// 索引设计：
//   - uniqueIndex:uk_from_to：from_user_id 和 to_user_id 两个字段带同一个索引名，
//     会生成联合唯一索引 UNIQUE(from_user_id, to_user_id)，防止重复申请。
//   - index:idx_to_status：联合普通索引 (to_user_id, status)，
//     加速「查我收到的待处理申请」这个高频查询。
type FriendRequest struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	FromUserID uint64    `gorm:"not null;uniqueIndex:uk_from_to" json:"from_user_id"`
	ToUserID   uint64    `gorm:"not null;uniqueIndex:uk_from_to;index:idx_to_status,priority:1" json:"to_user_id"`
	Status     int8      `gorm:"type:tinyint;not null;default:0;index:idx_to_status,priority:2" json:"status"`
	Message    string    `gorm:"type:varchar(255);not null;default:''" json:"message"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (FriendRequest) TableName() string { return "friend_requests" }

// Friendship 对应 friendships 表（好友关系）。
//
// 注意：好友关系是双向的，A 与 B 成为好友时会存两行：(A,B) 和 (B,A)。
// 这样「查我的好友」只需 WHERE user_id = ?，不用 OR。
// 代价是接受好友申请时必须用事务同时插两行。
type Friendship struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"not null;uniqueIndex:uk_user_friend;index:idx_user" json:"user_id"`
	FriendID  uint64    `gorm:"not null;uniqueIndex:uk_user_friend" json:"friend_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (Friendship) TableName() string { return "friendships" }
