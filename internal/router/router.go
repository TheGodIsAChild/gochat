package router

import (
	"gochat/internal/router/health"
	userapi "gochat/internal/router/user_router"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.Default()
	health.HealthRouter(r)
	userapi.UserRouter(r)
	return r
}
