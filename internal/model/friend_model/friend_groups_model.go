package friendmodel

import "gochat/internal/model"

type FriendGroup struct {
	model.Model
	UserID uint   `gorm:"not null;uniqueIndex:uk_user_name" json:"user_id"`
	Name   string `gorm:"type:varchar(32);not null;uniqueIndex:uk_user_name" json:"name"`
	Sort   int    `gorm:"not null;default:0" json:"sort"`
}

func (FriendGroup) TableName() string { return "friend_groups" }
