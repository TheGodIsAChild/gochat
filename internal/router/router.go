package router

import (
	"github.com/gin-gonic/gin"

	callapi "gochat/internal/router/call_router"
	chatapi "gochat/internal/router/chat_router"
	friendapi "gochat/internal/router/friend_router"
	groupapi "gochat/internal/router/group_router"
	"gochat/internal/router/health"
	userapi "gochat/internal/router/user_router"
	"gochat/internal/ws"
)

// NewRouter 组装所有路由。
// 每个业务模块一个 *Router(r) 函数，内部自己决定公开组/鉴权组。
func NewRouter() *gin.Engine {
	r := gin.Default()

	health.HealthRouter(r)

	userapi.UserRouter(r)
	friendapi.FriendRouter(r)
	chatapi.ChatRouter(r)
	groupapi.GroupRouter(r)
	callapi.CallRouter(r)

	// WebSocket：鉴权在 handler 内部做（浏览器 WS 不能带 Authorization 头，用 query 传 token）
	r.GET("/ws", ws.ServeWS)

	return r
}
