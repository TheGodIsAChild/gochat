package userservice

import (
	"fmt"
	"gochat/global"
	"gochat/internal/auth"
	usermodel "gochat/internal/model/user_model"

	"gorm.io/gorm"
)

func UserRegister(username string, password, nickname string) (*usermodel.User, error) {
	var count int64
	if err := global.DB.Model(&usermodel.User{}).
		Where("username = ?", username).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("用户查询失败：%w", err)
	}
	if count > 0 {
		return nil, fmt.Errorf("用户名已存在")
	}
	hash, err := auth.Hash(password)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败：%w", err)
	}
	user := &usermodel.User{
		Username:     username,
		PasswordHash: hash,
		Nickname:     nickname,
	}

	err = global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		settings := &usermodel.UserSettings{UserID: user.ID}
		if err := tx.Create(settings).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("创建用户失败：%w", err)
	}
	return user, nil
}
