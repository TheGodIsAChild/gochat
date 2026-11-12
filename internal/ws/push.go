package ws

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	chatservice "gochat/internal/service/chat_service"
)

// pushConcurrency 群消息扇出的最大并发数。
//
// 【为什么需要限制并发】
// 群成员 200 人时，如果无脑起 200 个 goroutine 推送：
//   - goroutine 数量瞬间暴涨（虽然 Go 很轻，但不是免费的）
//   - 对下游（Hub 的锁、网络）造成瞬时压力
// 用「带缓冲的信号量」限制同时最多 N 个推送 goroutine，既并行又可控。
const pushConcurrency = 16

// pushSem 信号量（容量 = pushConcurrency）。
var pushSem = make(chan struct{}, pushConcurrency)

// PushToConversation 把任意 JSON 对象推送给会话所有在线成员。
// M7 的 MQ 消费者用它推送异步消息。
func PushToConversation(convID uint64, payload any) {
	frame, err := json.Marshal(payload)
	if err != nil {
		return
	}
	memberIDs, err := chatservice.ListMemberIDs(context.Background(), convID)
	if err != nil {
		log.Printf("推送消息时查询会话成员失败: %v", err)
		return
	}
	fanout(memberIDs, frame)
}

// PushMessage 把一条消息推送给会话里的所有在线成员。
//
// 【为什么放在 ws 包】
// HTTP 通道发消息、WebSocket 通道发消息，最终都要走「推送给会话成员」这一步。
// 抽成一个函数，两个入口共用，避免逻辑重复。
//
// 【为什么不放 service 层】
// service 层不应该依赖 WebSocket（M6 拆微服务时 service 可能被 gRPC 调用，没有 WS）。
// 推送是「表现层」职责，放在 ws 包由 handler 调用更合理。
//
// 【扇出策略】
//   - 成员少（<= 10）：顺序推送，简单直接
//   - 成员多：并发推送，但用信号量把并发数限制在 pushConcurrency
func PushMessage(convID uint64, item chatservice.MessageItem) {
	PushToConversation(convID, map[string]any{
		"type": "message",
		"data": item,
	})
}

// fanout 向一批用户推送同一帧数据。
func fanout(memberIDs []uint64, frame []byte) {
	// 小会话：顺序推送
	if len(memberIDs) <= 10 {
		for _, uid := range memberIDs {
			DefaultHub.SendToUser(uid, frame)
		}
		return
	}

	// 大会话：并发推送 + 信号量限流
	var wg sync.WaitGroup
	for _, uid := range memberIDs {
		wg.Add(1)
		pushSem <- struct{}{}
		go func(uid uint64) {
			defer wg.Done()
			defer func() { <-pushSem }()
			// 返回 false 说明该用户不在线：消息已落库，上线拉历史即可
			DefaultHub.SendToUser(uid, frame)
		}(uid)
	}
	wg.Wait()
}
