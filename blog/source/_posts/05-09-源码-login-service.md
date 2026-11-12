---
title: "源码走读 S09：login_service.go"
date: 2026-11-14 23:15:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/service/user_service/login_service.go`　**行数**：65 行
> **职责**：登录：校验密码 → 签发双 token → 存 jti → 更新登录时间

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package userservice

import (
	"context"
	"errors"
	"fmt"
	"gochat/global"
	"gochat/internal/auth"
	usermodel "gochat/internal/model/user_model"
	"time"

	"gorm.io/gorm"
)

func Login(ctx context.Context, username string, password string) (accessToken string, refreshToken string, user *usermodel.User, err error) {
	var loginUser usermodel.User
	// 用户查询
	err = global.DB.Where("username = ?", username).First(&loginUser).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil, errors.New("用户名或密码错误")
		}
		return "", "", nil, fmt.Errorf("查询用户失败：%w", err)
	}
	// 密码校验
	if !auth.Verify(loginUser.PasswordHash, password) {
		return "", "", nil, errors.New("用户名或密码错误")
	}

	// 账号状态检测
	// 1 禁用， 0 正常
	if loginUser.Status != 0 {
		return "", "", nil, errors.New("用户已禁用")
	}

	// 签发access token
	accessToken, err = auth.GenerateAccess(
		global.Config.JWT.Secret,
		uint64(loginUser.ID),
		loginUser.Username,
		global.Config.JWT.AccessExpireMinutes,
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("签发accesstoken失败: %w", err)
	}
	// 签发refresh token
	refreshToken, jti, err := auth.GenerateRefresh(
		global.Config.JWT.Secret,
		uint64(loginUser.ID),
		global.Config.JWT.RefreshExpireHours,
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("签发refreshtoken失败: %w", err)
	}
	// redis存储jti
	ttl := time.Duration(global.Config.JWT.RefreshExpireHours) * time.Hour
	if err := auth.SaveRefreshKeytoRedis(ctx, uint64(loginUser.ID), jti, ttl); err != nil {
		return "", "", nil, fmt.Errorf("保存refreshtoken失败: %w", err)
	}
	// 修改最近登录时间（暂时不用做登录记录）
	now := time.Now()
	global.DB.Model(&loginUser).Update("last_login_at", now)
	return accessToken, refreshToken, &loginUser, nil
}

```
