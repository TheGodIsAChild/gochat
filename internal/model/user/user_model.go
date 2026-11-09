package user

import (
	"gochat/internal/model"
	"time"
)

type User struct {
	model.Model
	Username     string     `gorm:"type:varchar(32);uniqueIndex;not null" json:"username"`
	Phone        *string    `gorm:"type:varchar(20);uniqueIndex" json:"phone,omitempty"`
	Email        *string    `gorm:"type:varchar(128);uniqueIndex" json:"email,omitempty"`
	PasswordHash string     `gorm:"type:varchar(255);not null" json:"-"`
	Nickname     string     `gorm:"type:varchar(32);not null;default:''" json:"nickname"`
	Avatar       string     `gorm:"type:varchar(255);not null;default:''" json:"avatar"`
	Gender       int8       `gorm:"type:tinyint;not null;default:0" json:"gender"`          // 0未知 1男 2女
	Signature    string     `gorm:"type:varchar(128);not null;default:''" json:"signature"` // 签名
	Region       string     `gorm:"type:varchar(64);not null;default:''" json:"region"`
	Birthday     *time.Time `gorm:"type:date" json:"birthday,omitempty"`
	Status       int8       `gorm:"type:tinyint;not null;default:0" json:"status"` // 0正常 1禁用
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

func (User) TableName() string { return "users" }
