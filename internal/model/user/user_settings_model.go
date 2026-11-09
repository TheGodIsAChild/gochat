package user

import "gochat/internal/model"

type UserSettings struct {
	model.Model
	UserID              uint64 `gorm:"not null;uniqueIndex" json:"user_id"`
	AllowSearchByPhone  bool   `gorm:"not null;default:true" json:"allow_search_by_phone"`
	AllowFriendRequest  bool   `gorm:"not null;default:true" json:"allow_friend_request"`
	NeedVerify          bool   `gorm:"not null;default:true" json:"need_verify"`
	ShowReadReceipt     bool   `gorm:"not null;default:true" json:"show_read_receipt"`
	NotificationEnabled bool   `gorm:"not null;default:true" json:"notification_enabled"`
}

func (UserSettings) TableName() string { return "user_settings" }
