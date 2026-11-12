package router

import (
	"github.com/gin-gonic/gin"

	callapi "gochat/internal/router/call_router"
	chatapi "gochat/internal/router/chat_router"
	friendapi "gochat/internal/router/friend_router"
	groupapi "gochat/internal/router/group_router"
	"gochat/internal/router/health"
	rpcapi "gochat/internal/router/rpc_router"
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
	// M6：HTTP gateway 通过 gRPC 调用内部微服务
	rpcapi.RPCRouter(r)

	// WebSocket：鉴权在 handler 内部做（浏览器 WS 不能带 Authorization 头，用 query 传 token）
	r.GET("/ws", ws.ServeWS)

	// 前端演示页面（仿微信）：静态资源由 Go 直接托管，无需额外前端服务
	r.Static("/css", "./web/css")
	r.Static("/js", "./web/js")
	r.StaticFile("/", "./web/index.html")

	// 项目学习博客（Hexo 生成的静态站点）
	r.Static("/blog", "./blog/public")

	return r
}
