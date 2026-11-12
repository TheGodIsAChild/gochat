// Package chatapi 单聊 HTTP 接口（会话 + 消息历史）。
//
// 注意：发消息的主通道是 WebSocket；这里的 HTTP 发消息接口用于
// 「不支持 WS 的客户端」或「兜底/调试」，两者共用同一个 service 逻辑。
package chatapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gochat/global"
	"gochat/internal/middleware"
	"gochat/internal/router/resp"
	chatservice "gochat/internal/service/chat_service"
	"gochat/internal/ws"
)

func currentUserID(c *gin.Context) uint64 {
	return c.GetUint64(middleware.CtxUserID)
}

func parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Fail(c, http.StatusBadRequest, "id 参数错误")
		return 0, false
	}
	return id, true
}

// ---------- 请求体 ----------

type createConversationReq struct {
	PeerUserID uint64 `json:"peer_user_id" binding:"required"`
}

type sendMessageReq struct {
	ClientMsgID string `json:"client_msg_id" binding:"required,max=64"`
	Type        int8   `json:"type"`
	Content     string `json:"content" binding:"required"`
}

type markReadReq struct {
	MessageID uint64 `json:"message_id" binding:"required"`
}

// ============================ 会话 ============================

// CreateConversation 创建或获取单聊会话
// POST /api/v1/conversations
func CreateConversation(c *gin.Context) {
	var req createConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	conv, err := chatservice.CreateOrGetSingleConversation(c.Request.Context(), userID, req.PeerUserID)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, gin.H{"conversation_id": conv.ID, "type": conv.Type})
}

// ListConversations 我的会话列表
// GET /api/v1/conversations
func ListConversations(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := chatservice.ListConversations(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"conversations": items})
}

// ============================ 消息 ============================

// SendMessage 发送消息（HTTP 兜底通道）
// POST /api/v1/conversations/:id/messages
func SendMessage(c *gin.Context) {
	convID, ok := parseID(c)
	if !ok {
		return
	}
	var req sendMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	msgType := req.Type
	if msgType == 0 {
		msgType = 1 // 默认文本
	}
	msg, err := chatservice.SendMessage(c.Request.Context(), userID, convID, req.ClientMsgID, msgType, req.Content)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// M7：启用 MQ 时由异步消费者推送（解耦 + 削峰）；未启用则同步推送
	if global.MQ == nil || !global.MQ.Enabled {
		ws.PushMessage(convID, chatservice.ToItem(msg))
	}

	resp.OK(c, chatservice.ToItem(msg))
}

// ListMessages 拉取历史消息
// GET /api/v1/conversations/:id/messages?before_id=&limit=
func ListMessages(c *gin.Context) {
	convID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	var beforeID uint64
	if v := c.Query("before_id"); v != "" {
		if parsed, err := strconv.ParseUint(v, 10, 64); err == nil {
			beforeID = parsed
		}
	}
	limit := 20
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	items, err := chatservice.ListMessages(c.Request.Context(), userID, convID, beforeID, limit)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, gin.H{"messages": items})
}

// MarkRead 标记已读
// POST /api/v1/conversations/:id/read
func MarkRead(c *gin.Context) {
	convID, ok := parseID(c)
	if !ok {
		return
	}
	var req markReadReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := chatservice.MarkRead(c.Request.Context(), userID, convID, req.MessageID); err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 在线状态 ============================

// GetOnlineStatus 查询一批用户里哪些在线
// GET /api/v1/online?ids=1,2,3
//
// 【说明】M2 的在线状态存在「单机内存 Hub」里，只能反映本进程的连接。
// M4/M6 多实例部署时会改成 Redis（每个实例把在线用户写入 Redis，用 TTL 续期）。
func GetOnlineStatus(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	raw := c.Query("ids")
	var rawIDs []string
	if raw != "" {
		rawIDs = strings.Split(raw, ",")
	}

	online := make([]uint64, 0, len(rawIDs))
	for _, s := range rawIDs {
		id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if ws.DefaultHub.IsOnline(id) {
			online = append(online, id)
		}
	}
	resp.OK(c, gin.H{"online_ids": online})
}

// ============================ 路由注册 ============================

// ChatRouter 注册单聊相关路由（全部需要登录）。
func ChatRouter(r *gin.Engine) {
	priv := r.Group("/api/v1", middleware.Auth())
	{
		priv.POST("/conversations", CreateConversation)
		priv.GET("/conversations", ListConversations)
		priv.POST("/conversations/:id/messages", SendMessage)
		priv.GET("/conversations/:id/messages", ListMessages)
		priv.POST("/conversations/:id/read", MarkRead)
		priv.GET("/online", GetOnlineStatus)
	}
}
