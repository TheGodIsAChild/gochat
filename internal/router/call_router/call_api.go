// Package callapi 通话记录的 HTTP 接口（信令走 WebSocket，这里只查记录）。
package callapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gochat/internal/middleware"
	"gochat/internal/router/resp"
	callservice "gochat/internal/service/call_service"
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

// ListCalls 我的通话记录
// GET /api/v1/calls
func ListCalls(c *gin.Context) {
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

	calls, err := callservice.ListHistory(c.Request.Context(), userID, limit)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"calls": calls})
}

// GetCall 通话详情
// GET /api/v1/calls/:id
func GetCall(c *gin.Context) {
	callID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	call, err := callservice.Get(c.Request.Context(), callID, userID)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, call)
}

// CallRouter 注册通话相关路由（全部需要登录）。
func CallRouter(r *gin.Engine) {
	priv := r.Group("/api/v1", middleware.Auth())
	{
		priv.GET("/calls", ListCalls)
		priv.GET("/calls/:id", GetCall)
	}
}
