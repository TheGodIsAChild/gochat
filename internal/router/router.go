package router

import (
	"github.com/gin-gonic/gin"

	friendapi "gochat/internal/router/friend_router"
	"gochat/internal/router/health"
	userapi "gochat/internal/router/user_router"
)

// NewRouter 组装所有路由。
// 每个业务模块一个 *Router(r) 函数，内部自己决定公开组/鉴权组。
func NewRouter() *gin.Engine {
	r := gin.Default()
	health.HealthRouter(r)
	userapi.UserRouter(r)
	friendapi.FriendRouter(r)
	return r
}
