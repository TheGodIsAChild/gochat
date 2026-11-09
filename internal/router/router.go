package router

import (
	"gochat/internal/router/health"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.Default()
	health.HealthRouter(r)
	return r
}
