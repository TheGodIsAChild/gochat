package userservice

import (
	"context"
	"errors"
	"gochat/global"
	usermodel "gochat/internal/model/user_model"
	"time"

	"gorm.io/gorm"
)

type UserProfile struct {
	UserID      uint       `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Avatar      string     `json:"avatar"`
	Gender      int8       `json:"gender"`    // 0未知 1男 2女
	Signature   string     `json:"signature"` // 签名
	Region      string     `json:"region"`
	Birthday    *time.Time `json:"birthday,omitempty"`
	Status      int8       `json:"status"` // 0正常 1禁用
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

func UserInfo(ctx context.Context, userID uint64) (UserProfile, error) {
	var user usermodel.User
	if err := global.DB.WithContext(ctx).First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserProfile{}, errors.New("用户不存在")
		}
		return UserProfile{}, err
	}
	return UserProfile{
		UserID:      user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname,
		Avatar:      user.Avatar,
		Gender:      user.Gender,
		Signature:   user.Signature,
		Region:      user.Region,
		Birthday:    user.Birthday,
		Status:      user.Status,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   &user.CreatedAt,
	}, nil

}
