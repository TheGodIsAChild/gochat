---
title: "源码走读 S13：group_service.go"
date: 2026-11-14 23:13:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/service/group_service/group_service.go`　**行数**：461 行
> **职责**：群聊：建群事务、成员管理、群主权限、解散

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
// Package groupservice 群聊业务。
//
// 【M3 复用 M2 的抽象】
// 群聊 = Type=2 的会话 + groups 表（群信息）+ conversation_members（成员）。
// 所以群消息发送完全复用 chatservice.SendMessage / ws.PushMessage，不需要新逻辑。
// M3 新增的只是「群的生命周期管理」：建群、拉人、踢人、退群、改群资料、解散。
package groupservice

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"gochat/global"
	chatmodel "gochat/internal/model/chat_model"
	groupmodel "gochat/internal/model/group_model"
	usermodel "gochat/internal/model/user_model"
)

const (
	GroupStatusNormal    int8 = 1 // 正常
	GroupStatusDissolved int8 = 2 // 已解散

	MemberRoleNormal int8 = 0 // 普通成员
	MemberRoleOwner  int8 = 1 // 群主

	maxGroupMembers = 200
)

// GroupInfo 群信息 DTO。
type GroupInfo struct {
	ID             uint64 `json:"id"`
	ConversationID uint64 `json:"conversation_id"`
	OwnerID        uint64 `json:"owner_id"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Announcement   string `json:"announcement"`
	MemberCount    int    `json:"member_count"`
	MaxMembers     int    `json:"max_members"`
}

func toInfo(g *groupmodel.Group) GroupInfo {
	return GroupInfo{
		ID:             uint64(g.ID),
		ConversationID: uint64(g.ConversationID),
		OwnerID:        uint64(g.OwnerID),
		Name:           g.Name,
		Avatar:         g.Avatar,
		Announcement:   g.Announcement,
		MemberCount:    g.MemberCount,
		MaxMembers:     g.MaxMembers,
	}
}

// getGroupByID 查群（含校验存在 + 未解散）。
func getGroupByID(ctx context.Context, groupID uint64) (*groupmodel.Group, error) {
	var g groupmodel.Group
	if err := global.DB.WithContext(ctx).
		Where("id = ? AND status = ?", groupID, GroupStatusNormal).
		First(&g).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("群不存在或已解散")
		}
		return nil, err
	}
	return &g, nil
}

// isMember 判断用户是否是群成员。
func isMember(ctx context.Context, convID, userID uint64) (bool, error) {
	var cnt int64
	if err := global.DB.WithContext(ctx).Model(&chatmodel.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ============================ 建群 ============================

// CreateGroup 建群。
//
// 【事务里要做 4 件事】
//  1. 创建会话（Type=2）
//  2. 创建 groups 行（关联 conversation_id）
//  3. 插入群主（Role=1）
//  4. 插入其他成员
//
// 任何一步失败都要整体回滚，否则会出现「有群没会话」或「有会话没群主」的脏数据。
func CreateGroup(ctx context.Context, ownerID uint64, name, announcement string, memberIDs []uint64) (*GroupInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("群名称不能为空")
	}

	// 去重 + 排除群主自己
	set := make(map[uint64]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		if id != 0 && id != ownerID {
			set[id] = struct{}{}
		}
	}
	ids := make([]uint64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	if len(ids)+1 > maxGroupMembers {
		return nil, errors.New("群成员超出上限")
	}

	// 校验被拉的人是否都存在
	if len(ids) > 0 {
		var cnt int64
		if err := global.DB.WithContext(ctx).Model(&usermodel.User{}).
			Where("id IN ?", ids).Count(&cnt).Error; err != nil {
			return nil, err
		}
		if int(cnt) != len(ids) {
			return nil, errors.New("部分用户不存在")
		}
	}

	var conv chatmodel.Conversation
	var g groupmodel.Group

	err := global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 会话
		conv = chatmodel.Conversation{Type: 2}
		if err := tx.Create(&conv).Error; err != nil {
			return err
		}

		// 2. 群
		g = groupmodel.Group{
			ConversationID: conv.ID,
			OwnerID:        uint(ownerID),
			Name:           name,
			Announcement:   announcement,
			MemberCount:    len(ids) + 1,
			MaxMembers:     maxGroupMembers,
			Status:         GroupStatusNormal,
		}
		if err := tx.Create(&g).Error; err != nil {
			return err
		}

		// 3. 成员（群主 + 其他人）
		members := make([]chatmodel.ConversationMember, 0, len(ids)+1)
		members = append(members, chatmodel.ConversationMember{
			ConversationID: conv.ID,
			UserID:         uint(ownerID),
			Role:           MemberRoleOwner,
		})
		for _, id := range ids {
			members = append(members, chatmodel.ConversationMember{
				ConversationID: conv.ID,
				UserID:         uint(id),
				Role:           MemberRoleNormal,
			})
		}
		return tx.Create(&members).Error
	})
	if err != nil {
		return nil, err
	}

	info := toInfo(&g)
	return &info, nil
}

// ============================ 查询 ============================

// GetGroup 查群详情（必须是群成员才能看）。
func GetGroup(ctx context.Context, groupID, userID uint64) (*GroupInfo, error) {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ok, err := isMember(ctx, uint64(g.ConversationID), userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("你不是该群成员")
	}
	info := toInfo(g)
	return &info, nil
}

// ListMyGroups 我加入的群列表。
//
// 【实现】先查「我的会话成员」，再按 conversation_id 批量查 groups。
// 注意只查 status=1（未解散）的群。
func ListMyGroups(ctx context.Context, userID uint64) ([]GroupInfo, error) {
	var members []chatmodel.ConversationMember
	if err := global.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []GroupInfo{}, nil
	}

	convIDs := make([]uint, 0, len(members))
	for _, m := range members {
		convIDs = append(convIDs, m.ConversationID)
	}

	var groups []groupmodel.Group
	if err := global.DB.WithContext(ctx).
		Where("conversation_id IN ? AND status = ?", convIDs, GroupStatusNormal).
		Order("id DESC").
		Find(&groups).Error; err != nil {
		return nil, err
	}

	items := make([]GroupInfo, 0, len(groups))
	for i := range groups {
		items = append(items, toInfo(&groups[i]))
	}
	return items, nil
}

// ============================ 成员管理 ============================

// AddMembers 拉人进群（任何群成员都可以拉，类似微信）。
// 已在群里的成员会被跳过；超过人数上限则拒绝。
func AddMembers(ctx context.Context, operatorID, groupID uint64, memberIDs []uint64) error {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	ok, err := isMember(ctx, uint64(g.ConversationID), operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("你不是该群成员")
	}

	// 已存在的成员
	var existing []chatmodel.ConversationMember
	if err := global.DB.WithContext(ctx).
		Where("conversation_id = ?", g.ConversationID).
		Find(&existing).Error; err != nil {
		return err
	}
	existingSet := make(map[uint64]struct{}, len(existing))
	for _, m := range existing {
		existingSet[uint64(m.UserID)] = struct{}{}
	}

	// 过滤：去掉已存在/重复/自己
	addSet := make(map[uint64]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		if id == 0 {
			continue
		}
		if _, exists := existingSet[id]; exists {
			continue
		}
		addSet[id] = struct{}{}
	}
	if len(addSet) == 0 {
		return nil // 没有需要新增的
	}
	if g.MemberCount+len(addSet) > g.MaxMembers {
		return errors.New("群成员超出上限")
	}

	addIDs := make([]uint64, 0, len(addSet))
	for id := range addSet {
		addIDs = append(addIDs, id)
	}

	// 校验用户存在
	var cnt int64
	if err := global.DB.WithContext(ctx).Model(&usermodel.User{}).
		Where("id IN ?", addIDs).Count(&cnt).Error; err != nil {
		return err
	}
	if int(cnt) != len(addIDs) {
		return errors.New("部分用户不存在")
	}

	// 事务：插成员 + 更新人数冗余
	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		members := make([]chatmodel.ConversationMember, 0, len(addIDs))
		for _, id := range addIDs {
			members = append(members, chatmodel.ConversationMember{
				ConversationID: g.ConversationID,
				UserID:         uint(id),
				Role:           MemberRoleNormal,
			})
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}
		return tx.Model(&groupmodel.Group{}).
			Where("id = ?", g.ID).
			Update("member_count", gorm.Expr("member_count + ?", len(addIDs))).Error
	})
}

// RemoveMember 踢人 / 退群。
//   - targetID == operatorID：自己退群（群主不能直接退，必须先解散或转让）
//   - targetID != operatorID：踢人（只有群主可以）
func RemoveMember(ctx context.Context, operatorID, groupID, targetID uint64) error {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return err
	}

	if targetID == operatorID {
		// 退群
		if uint64(g.OwnerID) == operatorID {
			return errors.New("群主不能退群，请先解散群")
		}
	} else {
		// 踢人：只有群主
		if uint64(g.OwnerID) != operatorID {
			return errors.New("只有群主可以移除成员")
		}
	}

	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("conversation_id = ? AND user_id = ?", g.ConversationID, targetID).
			Delete(&chatmodel.ConversationMember{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("该用户不在群里")
		}
		return tx.Model(&groupmodel.Group{}).
			Where("id = ? AND member_count > 0", g.ID).
			Update("member_count", gorm.Expr("member_count - 1")).Error
	})
}

// MemberItem 群成员列表项。
type MemberItem struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Role     int8   `json:"role"` // 0成员 1群主
}

// ListMembers 查询群成员列表（必须是群成员才能看）。
func ListMembers(ctx context.Context, groupID, userID uint64) ([]MemberItem, error) {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ok, err := isMember(ctx, uint64(g.ConversationID), userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("你不是该群成员")
	}

	var members []chatmodel.ConversationMember
	if err := global.DB.WithContext(ctx).
		Where("conversation_id = ?", g.ConversationID).
		Order("role DESC, id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []MemberItem{}, nil
	}

	ids := make([]uint, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	var users []usermodel.User
	if err := global.DB.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	userMap := make(map[uint]usermodel.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]MemberItem, 0, len(members))
	for _, m := range members {
		u := userMap[m.UserID]
		items = append(items, MemberItem{
			UserID:   uint64(m.UserID),
			Username: u.Username,
			Nickname: u.Nickname,
			Avatar:   u.Avatar,
			Role:     m.Role,
		})
	}
	return items, nil
}

// ============================ 群资料 ============================

// UpdateGroup 改群名/公告（只有群主）。
func UpdateGroup(ctx context.Context, operatorID, groupID uint64, name, announcement *string) error {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if uint64(g.OwnerID) != operatorID {
		return errors.New("只有群主可以修改群资料")
	}

	updates := map[string]any{}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return errors.New("群名称不能为空")
		}
		updates["name"] = n
	}
	if announcement != nil {
		updates["announcement"] = *announcement
	}
	if len(updates) == 0 {
		return errors.New("没有需要更新的字段")
	}

	return global.DB.WithContext(ctx).
		Model(&groupmodel.Group{}).
		Where("id = ?", g.ID).
		Updates(updates).Error
}

// DissolveGroup 解散群（只有群主）。
// 把群标记为已解散，并软删除对应会话（消息历史和群记录保留，便于审计）。
func DissolveGroup(ctx context.Context, operatorID, groupID uint64) error {
	g, err := getGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if uint64(g.OwnerID) != operatorID {
		return errors.New("只有群主可以解散群")
	}

	return global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&groupmodel.Group{}).
			Where("id = ?", g.ID).
			Update("status", GroupStatusDissolved).Error; err != nil {
			return err
		}
		// 软删除会话（gorm.Model 自带 DeletedAt）
		return tx.Where("id = ?", g.ConversationID).
			Delete(&chatmodel.Conversation{}).Error
	})
}

```
