package userservice

import (
	"context"
	"errors"
	"fmt"
	"gochat/global"
	"gochat/internal/auth"
	usermodel "gochat/internal/model/user_model"
	"time"
)

// 只负责更新refreshtoken，不参与调用时机决定保证函数功能的单一性
func Refresh(ctx context.Context, refreshToken string) (newAccesstoken string, newRefreshtoken string, err error) {
	// 解析claims
	claims, err := auth.Parse(global.Config.JWT.Secret, refreshToken)
	if err != nil {
		return "", "", errors.New("refreshToken 无效或已过期")
	}

	// 判断token类型
	if claims.Type != auth.TypeRefresh {
		return "", "", errors.New("token 类型错误")
	}

	if err = auth.ConsumeRefreshKey(ctx, claims.UserID, claims.ID); err != nil {
		if errors.Is(err, auth.ErrRefreshNotFound) {
			// 如果查询不到refreshtoken信息说明登录非法，refreshtoken如果过期或不存在那么应该是掉登状态
			_ = auth.DeleteAllRefreshJtiinRedis(ctx, uint64(claims.UserID))
			return "", "", errors.New("登录状态已失效，请重新登录")
		}
		return "", "", fmt.Errorf("refresh token校验失败：%w", err)
	}

	var refreshUser usermodel.User
	if err = global.DB.First(&refreshUser, claims.UserID).Error; err != nil {
		_ = auth.DeleteAllRefreshJtiinRedis(ctx, uint64(claims.UserID))
		return "", "", errors.New("用户不存在")
	}
	if refreshUser.Status != 0 {
		_ = auth.DeleteAllRefreshJtiinRedis(ctx, uint64(claims.UserID))
		return "", "", errors.New("用户状态错误")
	}
	// 更新新jti
	jwtConfig := global.Config.JWT
	newAccesstoken, err = auth.GenerateAccess(jwtConfig.Secret, uint64(refreshUser.ID), refreshUser.Username, jwtConfig.AccessExpireMinutes)
	if err != nil {
		return "", "", errors.New("new accesstoken 签发失败")
	}
	newRefreshtoken, jti, err := auth.GenerateRefresh(jwtConfig.Secret, uint64(refreshUser.ID), jwtConfig.RefreshExpireHours)
	if err != nil {
		return "", "", errors.New("new refreshtoken签发失败")
	}
	ttl := time.Duration(jwtConfig.RefreshExpireHours) * time.Hour
	if err = auth.SaveRefreshKeytoRedis(ctx, uint64(refreshUser.ID), jti, ttl); err != nil {
		return "", "", errors.New("保存 new refreshtoken 失败")
	}

	// 返回新token
	return newAccesstoken, newRefreshtoken, nil
}
