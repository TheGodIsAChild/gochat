package friend

import "gochat/internal/model"

type Friendship struct {
	model.Model
	UserID    uint64 `gorm:"not null;uniqueIndex:uk_user_friend;index:idx_user" json:"user_id"`
	FriendID  uint64 `gorm:"not null;uniqueIndex:uk_user_friend" json:"friend_id"`
	Remark    string `gorm:"type:varchar(32);not null;default:''" json:"remark"`
	GroupID   uint64 `gorm:"not null;default:0" json:"group_id"`
	IsStarred int8   `gorm:"type:tinyint;not null;default:0" json:"is_starred"`
	Origin    string `gorm:"type:varchar(32);not null;default:''" json:"origin"`
	Status    int8   `gorm:"type:tinyint;not null;default:1" json:"status"` // 1正常 2已删除
}

func (Friendship) TableName() string { return "friendships" }
