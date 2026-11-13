---
title: "源码走读 S08：register_service.go"
date: 2026-11-14 23:14:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/service/user_service/register_service.go`　**行数**：47 行
> **职责**：注册：查重 + bcrypt + 事务（用户 + 默认设置）

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
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

```
