package friend

import "gochat/internal/model"

type FriendRequest struct {
	model.Model
	FromUserID uint64 `gorm:"not null;uniqueIndex:uk_from_to" json:"from_user_id"`
	ToUserID   uint64 `gorm:"not null;uniqueIndex:uk_from_to;index:idx_to_status,priority:1" json:"to_user_id"`
	Status     int8   `gorm:"type:tinyint;not null;default:0;index:idx_to_status,priority:2" json:"status"`
	Message    string `gorm:"type:varchar(255);not null;default:''" json:"message"`
	Source     string `gorm:"type:varchar(32);not null;default:''" json:"source"`
}

func (FriendRequest) TableName() string { return "friend_requests" }
