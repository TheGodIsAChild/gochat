// Package chatservice 单聊/群聊业务。
//
// M2 先做单聊（Type=1），M3 扩展群聊时这里的「会话 + 成员」模型可以直接复用。
package chatservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"gochat/global"
	chatmodel "gochat/internal/model/chat_model"
	groupmodel "gochat/internal/model/group_model"
	usermodel "gochat/internal/model/user_model"
)

// ============================ 会话 ============================

// ConversationItem 会话列表项。
type ConversationItem struct {
	ConversationID uint64     `json:"conversation_id"`
	Type           int8       `json:"type"`
	LastMessageID  uint64     `json:"last_message_id"`
	LastMessageAt  *time.Time `json:"last_message_at,omitempty"`
	PeerUserID     uint64     `json:"peer_user_id,omitempty"` // 单聊时是对方
	PeerNickname   string     `json:"peer_nickname,omitempty"`
	PeerAvatar     string     `json:"peer_avatar,omitempty"`
	// 群聊字段
	GroupID     uint64 `json:"group_id,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
	GroupAvatar string `json:"group_avatar,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
	Unread      int64  `json:"unread"`
}

// SingleConversationKey 生成单聊会话的唯一键。
//
// 【为什么要把 id 排序】
// A→B 和 B→A 必须得到同一个键，否则会建出两条会话。
// 统一用「小 id 在前」，保证幂等。
func SingleConversationKey(userA, userB uint64) string {
	if userA > userB {
		userA, userB = userB, userA
	}
	return fmt.Sprintf("%d:%d", userA, userB)
}

// CreateOrGetSingleConversation 创建（或获取已存在的）单聊会话。
//
// 【设计演进：从「先查后插」到「唯一键 + 幂等恢复」】
//
//	旧实现：用 conversation_members 自连接查两人是否已有会话，查不到就插。
//	问题：查和插之间有空窗期，两个用户同时点开聊天会各建一条会话，
//	      导致两人落在不同 conversation_id、消息互相收不到。
//
// 新实现（行业通用范式）：
//  1. 快速路径：用 single_key 走唯一索引查（O(1)）
//  2. 创建路径：事务里插会话 + 两条成员
//  3. 撞唯一索引（说明并发中别人刚建好）→ 捕获 ErrDuplicatedKey → 回查返回
//
// 【为什么这样就够了，不需要分布式锁】
// 唯一索引是数据库层的强约束，不存在"锁过期/多实例漏加锁"的问题；
// 应用层的查询只是快速路径，正确性由数据库兜底。
func CreateOrGetSingleConversation(ctx context.Context, userA, userB uint64) (*chatmodel.Conversation, error) {
	if userA == userB {
		return nil, errors.New("不能和自己创建会话")
	}

	key := SingleConversationKey(userA, userB)

	// 1. 快速路径：已经存在就直接返回
	var conv chatmodel.Conversation
	err := global.DB.WithContext(ctx).Where("single_key = ?", key).First(&conv).Error
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("查询单聊会话失败: %w", err)
	}

	// 2. 不存在 → 事务里创建会话 + 两个成员
	conv = chatmodel.Conversation{Type: 1, SingleKey: &key}
	err = global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conv).Error; err != nil {
			return err
		}
		return tx.Create(&[]chatmodel.ConversationMember{
			{ConversationID: conv.ID, UserID: uint(userA)},
			{ConversationID: conv.ID, UserID: uint(userB)},
		}).Error
	})
	if err != nil {
		// 3. 并发兜底：另一个请求抢先插入 → 唯一索引冲突 → 回查后幂等返回
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			var existing chatmodel.Conversation
			if e := global.DB.WithContext(ctx).Where("single_key = ?", key).First(&existing).Error; e == nil {
				return &existing, nil
			}
		}
		return nil, fmt.Errorf("创建单聊会话失败: %w", err)
	}
	return &conv, nil
}

// ListConversations 我的会话列表（按最后消息时间倒序）。
//
// 【实现思路】
//  1. 查我参与的所有 conversation_members
//  2. 批量查 conversations
//  3. 对单聊会话，批量查对方用户信息（避免 N+1）
//  4. 逐个算未读数
//
// 【注意点：未读数是 N 次 count 查询】
// 每个会话都要 count 一次。会话多时可以用 Redis 计数器或定期汇总优化。
// M2 先这样，M4 并发专题再优化。
func ListConversations(ctx context.Context, userID uint64) ([]ConversationItem, error) {
	// 1. 我参与的会话成员记录
	var members []chatmodel.ConversationMember
	if err := global.DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []ConversationItem{}, nil
	}

	convIDs := make([]uint, 0, len(members))
	memberMap := make(map[uint]chatmodel.ConversationMember, len(members))
	for _, m := range members {
		convIDs = append(convIDs, m.ConversationID)
		memberMap[m.ConversationID] = m
	}

	// 2. 批量查会话
	var convs []chatmodel.Conversation
	if err := global.DB.WithContext(ctx).
		Where("id IN ?", convIDs).
		Order("last_message_at DESC, id DESC").
		Find(&convs).Error; err != nil {
		return nil, err
	}

	// 3. 单聊：批量查「对方」用户信息
	//    先找出每个单聊会话的对方 user_id
	peerOf := make(map[uint]uint) // conversationID -> peer userID
	singleConvIDs := make([]uint, 0)
	for _, c := range convs {
		if c.Type == 1 {
			singleConvIDs = append(singleConvIDs, c.ID)
		}
	}
	if len(singleConvIDs) > 0 {
		var others []chatmodel.ConversationMember
		if err := global.DB.WithContext(ctx).
			Where("conversation_id IN ? AND user_id <> ?", singleConvIDs, userID).
			Find(&others).Error; err != nil {
			return nil, err
		}
		for _, m := range others {
			peerOf[m.ConversationID] = m.UserID
		}
	}
	peerIDs := make([]uint, 0, len(peerOf))
	for _, uid := range peerOf {
		peerIDs = append(peerIDs, uid)
	}
	peerMap := make(map[uint]usermodel.User, len(peerIDs))
	if len(peerIDs) > 0 {
		var users []usermodel.User
		if err := global.DB.WithContext(ctx).Where("id IN ?", peerIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, u := range users {
			peerMap[u.ID] = u
		}
	}

	// 3.5 群聊：批量查群信息（群名/头像/人数），用于会话列表展示
	groupMap := make(map[uint]groupmodel.Group)
	groupConvIDs := make([]uint, 0)
	for _, c := range convs {
		if c.Type == 2 {
			groupConvIDs = append(groupConvIDs, c.ID)
		}
	}
	if len(groupConvIDs) > 0 {
		var gs []groupmodel.Group
		if err := global.DB.WithContext(ctx).
			Where("conversation_id IN ?", groupConvIDs).
			Find(&gs).Error; err != nil {
			return nil, err
		}
		for _, g := range gs {
			groupMap[g.ConversationID] = g
		}
	}

	// 4. 组装 + 未读数
	items := make([]ConversationItem, 0, len(convs))
	for _, c := range convs {
		m, ok := memberMap[c.ID]
		if !ok {
			continue
		}
		item := ConversationItem{
			ConversationID: uint64(c.ID),
			Type:           c.Type,
			LastMessageID:  uint64(c.LastMessageID),
			LastMessageAt:  c.LastMessageAt,
		}
		if peerID, ok := peerOf[c.ID]; ok {
			item.PeerUserID = uint64(peerID)
			if u, exists := peerMap[peerID]; exists {
				item.PeerNickname = u.Nickname
				item.PeerAvatar = u.Avatar
			}
		}
		// 群聊：填群信息
		if c.Type == 2 {
			if g, exists := groupMap[c.ID]; exists {
				item.GroupID = uint64(g.ID)
				item.GroupName = g.Name
				item.GroupAvatar = g.Avatar
				item.MemberCount = g.MemberCount
			}
		}
		// 未读：id > 我最后读到的位置，且不是我自己发的
		var unread int64
		if err := global.DB.WithContext(ctx).Model(&chatmodel.Message{}).
			Where("conversation_id = ? AND id > ? AND sender_id <> ?", c.ID, m.LastReadMessageID, userID).
			Count(&unread).Error; err == nil {
			item.Unread = unread
		}
		items = append(items, item)
	}
	return items, nil
}

// MarkRead 更新「我读到的位置」（只前进，不后退）。
func MarkRead(ctx context.Context, userID, convID, messageID uint64) error {
	res := global.DB.WithContext(ctx).Model(&chatmodel.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ? AND last_read_message_id < ?", convID, userID, messageID).
		Update("last_read_message_id", messageID)
	if res.Error != nil {
		return res.Error
	}
	return nil
}
