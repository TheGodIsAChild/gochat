package ws

import (
	"context"
	"encoding/json"
	"log"

	chatservice "gochat/internal/service/chat_service"
)

// PushMessage 把一条消息推送给会话里的所有在线成员。
//
// 【为什么放在 ws 包】
// HTTP 通道发消息、WebSocket 通道发消息，最终都要走「推送给会话成员」这一步。
// 抽成一个函数，两个入口共用，避免逻辑重复。
//
// 【为什么不放 service 层】
// service 层不应该依赖 WebSocket（M6 拆微服务时 service 可能被 gRPC 调用，没有 WS）。
// 推送是「表现层」职责，放在 ws 包由 handler 调用更合理。
func PushMessage(convID uint64, item chatservice.MessageItem) {
	memberIDs, err := chatservice.ListMemberIDs(context.Background(), convID)
	if err != nil {
		log.Printf("推送消息时查询会话成员失败: %v", err)
		return
	}

	frame, err := json.Marshal(map[string]any{
		"type": "message",
		"data": item,
	})
	if err != nil {
		return
	}

	for _, uid := range memberIDs {
		// 返回 false 说明该用户不在线，无需处理：
		// 消息已经落库，用户下次上线拉历史/未读数就能看到（这就是"离线消息"）
		DefaultHub.SendToUser(uid, frame)
	}
}
