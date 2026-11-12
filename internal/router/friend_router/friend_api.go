// Package friendapi 好友相关 HTTP 接口。
//
// 路由全部挂在「鉴权组」下（/api/v1 + Auth 中间件），所以每个 handler
// 都可以直接从 gin context 里拿当前登录用户 id。
package friendapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gochat/internal/middleware"
	"gochat/internal/router/resp"
	friendservice "gochat/internal/service/friend_service"
)

// ---------- 请求体 ----------

type sendRequestReq struct {
	ToUserID uint64 `json:"to_user_id" binding:"required"`
	Message  string `json:"message" binding:"max=255"`
}

type remarkReq struct {
	Remark string `json:"remark" binding:"max=32"`
}

type blacklistReq struct {
	UserID uint64 `json:"user_id" binding:"required"`
}

// currentUserID 从中间件写入的 context 里取当前用户 id。
// 抽成小函数，避免每个 handler 重复写 key 字符串（写错不报错是高频 bug）。
func currentUserID(c *gin.Context) uint64 {
	return c.GetUint64(middleware.CtxUserID)
}

// parseID 解析路径参数 :id
func parseID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		resp.Fail(c, http.StatusBadRequest, "id 参数错误")
		return 0, false
	}
	return id, true
}

// ============================ 好友申请 ============================

// SendRequest 发送好友申请
// POST /api/v1/friends/requests
func SendRequest(c *gin.Context) {
	var req sendRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	fromID := currentUserID(c)
	if fromID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.SendRequest(c.Request.Context(), fromID, req.ToUserID, req.Message, "search"); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ListRequests 我收到的待处理好友申请
// GET /api/v1/friends/requests
func ListRequests(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := friendservice.ListIncomingRequests(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"requests": items})
}

// AcceptRequest 接受好友申请
// POST /api/v1/friends/requests/:id/accept
func AcceptRequest(c *gin.Context) {
	reqID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.AcceptRequest(c.Request.Context(), userID, reqID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// RejectRequest 拒绝好友申请
// POST /api/v1/friends/requests/:id/reject
func RejectRequest(c *gin.Context) {
	reqID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.RejectRequest(c.Request.Context(), userID, reqID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 好友列表 / 删除 / 备注 ============================

// ListFriends 好友列表
// GET /api/v1/friends
func ListFriends(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := friendservice.ListFriends(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"friends": items})
}

// DeleteFriend 删除好友
// DELETE /api/v1/friends/:id
func DeleteFriend(c *gin.Context) {
	friendID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.DeleteFriend(c.Request.Context(), userID, friendID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// SetRemark 设置好友备注
// PUT /api/v1/friends/:id/remark
func SetRemark(c *gin.Context) {
	friendID, ok := parseID(c)
	if !ok {
		return
	}
	var req remarkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.SetRemark(c.Request.Context(), userID, friendID, req.Remark); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 黑名单 ============================

// AddBlacklist 拉黑
// POST /api/v1/blacklist
func AddBlacklist(c *gin.Context) {
	var req blacklistReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.AddBlacklist(c.Request.Context(), userID, req.UserID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// RemoveBlacklist 移除黑名单
// DELETE /api/v1/blacklist/:id
func RemoveBlacklist(c *gin.Context) {
	blockedID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := friendservice.RemoveBlacklist(c.Request.Context(), userID, blockedID); err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ListBlacklist 黑名单列表
// GET /api/v1/blacklist
func ListBlacklist(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := friendservice.ListBlacklist(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"blacklist": items})
}

// ============================ 路由注册 ============================

// FriendRouter 注册好友相关路由（全部需要登录）。
func FriendRouter(r *gin.Engine) {
	priv := r.Group("/api/v1", middleware.Auth())
	{
		// 好友申请
		priv.POST("/friends/requests", SendRequest)
		priv.GET("/friends/requests", ListRequests)
		priv.POST("/friends/requests/:id/accept", AcceptRequest)
		priv.POST("/friends/requests/:id/reject", RejectRequest)

		// 好友
		priv.GET("/friends", ListFriends)
		priv.DELETE("/friends/:id", DeleteFriend)
		priv.PUT("/friends/:id/remark", SetRemark)

		// 黑名单
		priv.POST("/blacklist", AddBlacklist)
		priv.DELETE("/blacklist/:id", RemoveBlacklist)
		priv.GET("/blacklist", ListBlacklist)
	}
}
