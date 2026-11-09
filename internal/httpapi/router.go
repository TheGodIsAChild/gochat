package httpapi

// ⚠️ 参考实现：建议你先自己写一遍，卡住再对照本文件。

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewRouter 构建并返回 Gin 路由引擎。
// M0 只有 /health；M1 起会在这里注册用户/好友路由。
func NewRouter() *gin.Engine {
	r := gin.New()
	// Logger 记录请求日志，Recovery 防止 panic 打挂整个服务。
	r.Use(gin.Logger(), gin.Recovery())

	// 健康检查：服务活着就返回 200 + {"status":"ok"}。
	// 它不查数据库，只证明 HTTP 层工作正常。
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return r
}
