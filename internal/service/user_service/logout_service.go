package userservice

import (
	"context"
	"errors"
	"gochat/global"
	"gochat/internal/auth"
)

func Logout(ctx context.Context, refreshToken string) error {
	claims, err := auth.Parse(global.Config.JWT.Secret, refreshToken)
	if err != nil {
		return errors.New("token 无效")
	}
	if claims.Type != auth.TypeRefresh {
		return errors.New("token 类型错误")
	}
	// 登出幂等：不管存不存在，删了就算成功
	return auth.DeleteRefreshJtiinRedis(ctx, claims.UserID, claims.ID)
}
