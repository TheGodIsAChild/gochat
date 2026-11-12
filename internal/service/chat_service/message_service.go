package chatservice

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"gochat/global"
	chatmodel "gochat/internal/model/chat_model"
)

// MessageItem 消息响应 DTO。
type MessageItem struct {
	ID             uint64    `json:"id"`
	ConversationID uint64    `json:"conversation_id"`
	SenderID       uint64    `json:"sender_id"`
	ClientMsgID    string    `json:"client_msg_id"`
	Type           int8      `json:"type"`
	Content        string    `json:"content"`
	Status         int8      `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// ToItem 把数据库模型转成响应 DTO（WS 推送也用这个，保证 HTTP 和 WS 返回结构一致）。
func ToItem(m *chatmodel.Message) MessageItem {
	return MessageItem{
		ID:             uint64(m.ID),
		ConversationID: uint64(m.ConversationID),
		SenderID:       uint64(m.SenderID),
		ClientMsgID:    m.ClientMsgID,
		Type:           m.Type,
		Content:        m.Content,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
	}
}

// IsMember 判断用户是否是会话成员。
// 所有「读会话 / 发消息」的接口都要先过这一关，防止越权访问别人的会话。
func IsMember(ctx context.Context, convID, userID uint64) (bool, error) {
	var cnt int64
	if err := global.DB.WithContext(ctx).Model(&chatmodel.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// ListMemberIDs 查会话里所有成员的 userID（WS 推送时用）。
func ListMemberIDs(ctx context.Context, convID uint64) ([]uint64, error) {
	var members []chatmodel.ConversationMember
	if err := global.DB.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Find(&members).Error; err != nil {
		return nil, err
	}
	ids := make([]uint64, 0, len(members))
	for _, m := range members {
		ids = append(ids, uint64(m.UserID))
	}
	return ids, nil
}

// SendMessage 发送消息（幂等 + 事务）。
//
// 【幂等设计】
// 客户端每次发消息带一个 client_msg_id（如 UUID）。
//   1. 先查这个 (sender_id, client_msg_id) 是否已存在 → 存在直接返回（重试不会产生重复消息）
//   2. 不存在则插入；并发下若两个请求同时插入，唯一索引会让第二个报 Duplicate
//   3. 捕获 Duplicate → 再查一次返回已有消息（依然是幂等的）
//
// 【为什么插入消息和更新会话要在同一个事务】
// 消息插入成功、但更新 conversations.last_message_id 失败 → 会话列表里看不到最新消息。
// 两者必须原子。
func SendMessage(ctx context.Context, senderID, convID uint64, clientMsgID string, msgType int8, content string) (*chatmodel.Message, error) {
	if clientMsgID == "" {
		return nil, errors.New("client_msg_id 不能为空")
	}

	// 0. 越权校验
	ok, err := IsMember(ctx, convID, senderID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("你不是该会话的成员")
	}

	// 1. 幂等预查
	var existing chatmodel.Message
	err = global.DB.WithContext(ctx).
		Where("sender_id = ? AND client_msg_id = ?", senderID, clientMsgID).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 2. 事务：插消息 + 更新会话冗余字段
	msg := &chatmodel.Message{
		ConversationID: uint(convID),
		SenderID:       uint(senderID),
		ClientMsgID:    clientMsgID,
		Type:           msgType,
		Content:        content,
		Status:         1,
	}
	err = global.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		now := time.Now()
		return tx.Model(&chatmodel.Conversation{}).
			Where("id = ?", convID).
			Updates(map[string]any{
				"last_message_id": msg.ID,
				"last_message_at": now,
			}).Error
	})
	if err != nil {
		// 并发重复：唯一索引 uk_sender_client 拦住了第二个，重新查出来返回
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if e := global.DB.WithContext(ctx).
				Where("sender_id = ? AND client_msg_id = ?", senderID, clientMsgID).
				First(&existing).Error; e == nil {
				return &existing, nil
			}
		}
		return nil, err
	}
	return msg, nil
}

// ListMessages 拉取历史消息（游标分页）。
//
// 【为什么用游标（before_id）而不是 offset 分页】
// offset 分页在数据量大时越翻越慢（数据库要跳过 N 行），而且新消息插入会导致
// 翻页错位/重复。游标分页用「上一页最后一条的 id」作为锚点，性能稳定、不会错位。
//
// 返回按时间正序（老 → 新），方便前端直接渲染。
func ListMessages(ctx context.Context, userID, convID, beforeID uint64, limit int) ([]MessageItem, error) {
	ok, err := IsMember(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("你不是该会话的成员")
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	q := global.DB.WithContext(ctx).
		Where("conversation_id = ?", convID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}

	var msgs []chatmodel.Message
	if err := q.Order("id DESC").Limit(limit).Find(&msgs).Error; err != nil {
		return nil, err
	}

	// 查询结果是倒序（新 → 老），翻转为正序
	items := make([]MessageItem, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		items = append(items, MessageItem{
			ID:             uint64(m.ID),
			ConversationID: uint64(m.ConversationID),
			SenderID:       uint64(m.SenderID),
			ClientMsgID:    m.ClientMsgID,
			Type:           m.Type,
			Content:        m.Content,
			Status:         m.Status,
			CreatedAt:      m.CreatedAt,
		})
	}
	return items, nil
}
