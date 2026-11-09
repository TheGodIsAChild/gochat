package friend

import "gochat/internal/model"

type Blacklist struct {
	model.Model
	UserID        uint64 `gorm:"not null;uniqueIndex:uk_user_blocked" json:"user_id"`
	BlockedUserID uint64 `gorm:"not null;uniqueIndex:uk_user_blocked" json:"blocked_user_id"`
}

func (Blacklist) TableName() string { return "blacklist" }
