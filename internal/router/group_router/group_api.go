// Package groupapi 群聊 HTTP 接口。
package groupapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gochat/internal/middleware"
	"gochat/internal/router/resp"
	groupservice "gochat/internal/service/group_service"
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

type createGroupReq struct {
	Name         string   `json:"name" binding:"required,max=64"`
	Announcement string   `json:"announcement" binding:"max=512"`
	MemberIDs    []uint64 `json:"member_ids"`
}

type addMembersReq struct {
	MemberIDs []uint64 `json:"member_ids" binding:"required"`
}

type updateGroupReq struct {
	Name         *string `json:"name"`
	Announcement *string `json:"announcement"`
}

// ============================ 群 ============================

// CreateGroup 建群
// POST /api/v1/groups
func CreateGroup(c *gin.Context) {
	var req createGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	info, err := groupservice.CreateGroup(c.Request.Context(), userID, req.Name, req.Announcement, req.MemberIDs)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, info)
}

// GetGroup 群详情
// GET /api/v1/groups/:id
func GetGroup(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	info, err := groupservice.GetGroup(c.Request.Context(), groupID, userID)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, info)
}

// ListGroups 我的群列表
// GET /api/v1/groups
func ListGroups(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := groupservice.ListMyGroups(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.OK(c, gin.H{"groups": items})
}

// ListGroupMembers 群成员列表
// GET /api/v1/groups/:id/members
func ListGroupMembers(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	items, err := groupservice.ListMembers(c.Request.Context(), groupID, userID)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, gin.H{"members": items})
}

// AddMembers 拉人进群
// POST /api/v1/groups/:id/members
func AddMembers(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	var req addMembersReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := groupservice.AddMembers(c.Request.Context(), userID, groupID, req.MemberIDs); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// RemoveMember 踢人 / 退群
// DELETE /api/v1/groups/:id/members/:user_id
func RemoveMember(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	targetID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil || targetID == 0 {
		resp.Fail(c, http.StatusBadRequest, "user_id 参数错误")
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := groupservice.RemoveMember(c.Request.Context(), userID, groupID, targetID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// UpdateGroup 改群资料
// PUT /api/v1/groups/:id
func UpdateGroup(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	var req updateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := groupservice.UpdateGroup(c.Request.Context(), userID, groupID, req.Name, req.Announcement); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// DissolveGroup 解散群
// POST /api/v1/groups/:id/dissolve
func DissolveGroup(c *gin.Context) {
	groupID, ok := parseID(c)
	if !ok {
		return
	}
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	if err := groupservice.DissolveGroup(c.Request.Context(), userID, groupID); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 路由注册 ============================

// GroupRouter 注册群聊路由（全部需要登录）。
func GroupRouter(r *gin.Engine) {
	priv := r.Group("/api/v1", middleware.Auth())
	{
		priv.POST("/groups", CreateGroup)
		priv.GET("/groups", ListGroups)
		priv.GET("/groups/:id", GetGroup)
		priv.PUT("/groups/:id", UpdateGroup)
		priv.POST("/groups/:id/members", AddMembers)
		priv.GET("/groups/:id/members", ListGroupMembers)
		priv.DELETE("/groups/:id/members/:user_id", RemoveMember)
		priv.POST("/groups/:id/dissolve", DissolveGroup)
	}
}
