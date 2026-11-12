// Package chatservice 单聊/群聊业务。
//
// M2 先做单聊（Type=1），M3 扩展群聊时这里的「会话 + 成员」模型可以直接复用。
package chatservice

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"gochat/global"
	chatmodel "gochat/internal/model/chat_model"
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
	Unread         int64      `json:"unread"`
}

// CreateOrGetSingleConversation 创建（或获取已存在的）单聊会话。
//
// 【为什么需要「获取已存在」】
// 两个用户之间只能有一个单聊会话。A 点开和 B 的聊天、B 也点开，必须落到同一个
// conversation_id 上，否则两边各发各的、消息对不上。
//
// 【怎么判断两人已有会话】
// conversations 表本身不存两个 user_id（因为单聊/群聊共用表），
// 所以用 conversation_members 自连接：同一个 conversation_id 下同时存在这两个 user。
//
// 【并发创建问题】
// 两个用户同时点开聊天，可能同时判定「不存在」然后各创建一条。
// M2 先接受这个极小概率问题（会多出一条会话），M4 可以用 Redis 分布式锁或
// 唯一键（如 single_key='min:max' 唯一索引）彻底解决。
func CreateOrGetSingleConversation(ctx context.Context, userA, userB uint64) (*chatmodel.Conversation, error) {
	if userA == userB {
		return nil, errors.New("不能和自己创建会话")
	}

	// 1. 查是否已存在单聊会话（两个成员 join）
	var conv chatmodel.Conversation
	err := global.DB.WithContext(ctx).Raw(`
		SELECT c.* FROM conversations c
		JOIN conversation_members m1 ON m1.conversation_id = c.id AND m1.user_id = ?
		JOIN conversation_members m2 ON m2.conversation_id = c.id AND m2.user_id = ?
		WHERE c.type = 1 AND c.deleted_at IS NULL
		LIMIT 1`, userA, userB).Scan(&conv).Error
	if err != nil {
		return nil, err
	}
	if conv.ID != 0 {
		return &conv, nil
	}

	// 2. 不存在 → 事务里创建会话 + 两个成员
	conv = chatmodel.Conversation{Type: 1}
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
		return nil, err
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
