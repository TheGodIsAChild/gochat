// Package rpcapi 演示「HTTP gateway → gRPC 微服务」的调用链。
//
// 【微服务架构里的 gateway 模式】
//
//	客户端 ──HTTP──► gateway ──gRPC──► 用户服务 / 聊天服务
//
// gateway 负责：鉴权、限流、参数校验、把 HTTP 请求翻译成 gRPC 调用。
// 内部服务之间只暴露 gRPC，不直接对外。
//
// 注意：本项目为了能一个进程跑通演示，gateway 和 gRPC 服务在同一进程里启动；
// 真实部署时会拆成不同进程/容器，gateway 只知道服务地址。
package rpcapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gochat/internal/grpcx"
	"gochat/internal/middleware"
	"gochat/internal/router/resp"
)

func currentUserID(c *gin.Context) uint64 {
	return c.GetUint64(middleware.CtxUserID)
}

// GetUserViaRPC 通过 gRPC 查询用户
// GET /api/v1/rpc/users/:id
func GetUserViaRPC(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Fail(c, http.StatusBadRequest, "id 参数错误")
		return
	}

	u, err := grpcx.CallGetUser(c.Request.Context(), id)
	if err != nil {
		resp.Fail(c, http.StatusBadGateway, "调用用户服务失败: "+err.Error())
		return
	}
	resp.OK(c, u)
}

// BatchGetUsersViaRPC 通过 gRPC 批量查询用户
// GET /api/v1/rpc/users?ids=1,2,3
func BatchGetUsersViaRPC(c *gin.Context) {
	raw := c.Query("ids")
	if raw == "" {
		resp.Fail(c, http.StatusBadRequest, "ids 不能为空")
		return
	}
	var ids []uint64
	for _, s := range strings.Split(raw, ",") {
		if v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64); err == nil && v > 0 {
			ids = append(ids, v)
		}
	}
	if len(ids) == 0 {
		resp.Fail(c, http.StatusBadRequest, "ids 格式错误")
		return
	}

	users, err := grpcx.CallBatchGetUsers(c.Request.Context(), ids)
	if err != nil {
		resp.Fail(c, http.StatusBadGateway, "调用用户服务失败: "+err.Error())
		return
	}
	resp.OK(c, gin.H{"users": users})
}

// SendMessageViaRPC 通过 gRPC 发消息
// POST /api/v1/rpc/messages
func SendMessageViaRPC(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	var req struct {
		ConversationID uint64 `json:"conversation_id" binding:"required"`
		ClientMsgID    string `json:"client_msg_id" binding:"required,max=64"`
		Type           int32  `json:"type"`
		Content        string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	msgType := req.Type
	if msgType == 0 {
		msgType = 1
	}

	msg, err := grpcx.CallSendMessage(c.Request.Context(), &grpcx.SendMessageRequest{
		SenderID:       userID,
		ConversationID: req.ConversationID,
		ClientMsgID:    req.ClientMsgID,
		Type:           msgType,
		Content:        req.Content,
	})
	if err != nil {
		resp.Fail(c, http.StatusBadGateway, "调用聊天服务失败: "+err.Error())
		return
	}
	resp.OK(c, msg)
}

// ListMessagesViaRPC 通过 gRPC 拉历史消息
// GET /api/v1/rpc/conversations/:id/messages
func ListMessagesViaRPC(c *gin.Context) {
	convID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || convID == 0 {
		resp.Fail(c, http.StatusBadRequest, "id 参数错误")
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	msgs, err := grpcx.CallListMessages(c.Request.Context(), &grpcx.ListMessagesRequest{
		UserID:         userID,
		ConversationID: convID,
		Limit:          int32(limit),
	})
	if err != nil {
		resp.Fail(c, http.StatusBadGateway, "调用聊天服务失败: "+err.Error())
		return
	}
	resp.OK(c, gin.H{"messages": msgs})
}

// RPCRouter 注册 gateway 的 gRPC 调用路由。
func RPCRouter(r *gin.Engine) {
	priv := r.Group("/api/v1/rpc", middleware.Auth())
	{
		priv.GET("/users/:id", GetUserViaRPC)
		priv.GET("/users", BatchGetUsersViaRPC)
		priv.POST("/messages", SendMessageViaRPC)
		priv.GET("/conversations/:id/messages", ListMessagesViaRPC)
	}
}
