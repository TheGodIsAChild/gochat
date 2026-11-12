// Package userapi 用户相关 HTTP 接口：注册/登录/刷新/登出/资料/搜索/设置。
package userapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"gochat/internal/middleware"
	"gochat/internal/router/resp"
	userservice "gochat/internal/service/user_service"
)

// ============================ 请求体 ============================

type registerReq struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=64"`
	Nickname string `json:"nickname" binding:"required,max=32"`
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type logoutReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// currentUserID 取当前登录用户 id（由 Auth 中间件写入，key 用常量避免手写字符串出错）。
func currentUserID(c *gin.Context) uint64 {
	return c.GetUint64(middleware.CtxUserID)
}

// ============================ 认证 ============================

// Register 用户注册
// POST /api/v1/register
func Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	u, err := userservice.UserRegister(req.Username, req.Password, req.Nickname)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, gin.H{"user_id": u.ID, "username": u.Username, "nickname": u.Nickname})
}

// Login 用户登录，返回双 token
// POST /api/v1/login
func Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	access, refresh, u, err := userservice.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		resp.Fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	resp.OK(c, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"user": gin.H{
			"id": u.ID, "username": u.Username, "nickname": u.Nickname, "avatar": u.Avatar,
		},
	})
}

// Refresh 用 refresh token 换新 token（rotation）
// POST /api/v1/token/refresh
func Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	access, refresh, err := userservice.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		resp.Fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	resp.OK(c, gin.H{"access_token": access, "refresh_token": refresh})
}

// Logout 登出（撤销 refresh token）
// POST /api/v1/logout
func Logout(c *gin.Context) {
	var req logoutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := userservice.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		resp.Fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 资料 ============================

// GetUserProfile 我的资料
// GET /api/v1/users/me
func GetUserProfile(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	profile, err := userservice.UserInfo(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusNotFound, err.Error())
		return
	}
	resp.OK(c, profile)
}

// UpdateUserProfile 修改我的资料（部分更新，未传字段不动）
// PUT /api/v1/users/me
func UpdateUserProfile(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	var req userservice.UpdateProfileInput
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := userservice.UpdateProfile(c.Request.Context(), userID, req); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// SearchUsers 搜索用户（按用户名/昵称模糊匹配）
// GET /api/v1/users/search?keyword=xxx
func SearchUsers(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	users, err := userservice.SearchUsers(c.Request.Context(), c.Query("keyword"), userID, 20)
	if err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	// 只返回公开字段，绝不返回 password_hash / phone / email
	type brief struct {
		ID       uint   `json:"id"`
		Username string `json:"username"`
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
		Gender   int8   `json:"gender"`
	}
	items := make([]brief, 0, len(users))
	for _, u := range users {
		items = append(items, brief{
			ID: u.ID, Username: u.Username, Nickname: u.Nickname, Avatar: u.Avatar, Gender: u.Gender,
		})
	}
	resp.OK(c, gin.H{"users": items})
}

// ============================ 设置 ============================

// settingsResp 设置响应 DTO（不直接返回 model，避免暴露 id/user_id 等内部字段）。
type settingsResp struct {
	AllowSearchByPhone  bool `json:"allow_search_by_phone"`
	AllowFriendRequest  bool `json:"allow_friend_request"`
	NeedVerify          bool `json:"need_verify"`
	ShowReadReceipt     bool `json:"show_read_receipt"`
	NotificationEnabled bool `json:"notification_enabled"`
}

// GetSettings 我的设置
// GET /api/v1/users/settings
func GetSettings(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	s, err := userservice.GetSettings(c.Request.Context(), userID)
	if err != nil {
		resp.Fail(c, http.StatusNotFound, err.Error())
		return
	}
	resp.OK(c, settingsResp{
		AllowSearchByPhone:  s.AllowSearchByPhone,
		AllowFriendRequest:  s.AllowFriendRequest,
		NeedVerify:          s.NeedVerify,
		ShowReadReceipt:     s.ShowReadReceipt,
		NotificationEnabled: s.NotificationEnabled,
	})
}

// UpdateSettings 修改我的设置
// PUT /api/v1/users/settings
func UpdateSettings(c *gin.Context) {
	userID := currentUserID(c)
	if userID == 0 {
		resp.Fail(c, http.StatusUnauthorized, "未登录")
		return
	}

	var req userservice.UpdateSettingsInput
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}

	if err := userservice.UpdateSettings(c.Request.Context(), userID, req); err != nil {
		resp.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	resp.OK(c, nil)
}

// ============================ 路由注册 ============================

// UserRouter 注册用户相关路由。
func UserRouter(r *gin.Engine) {
	// 公开组：不需要登录
	pub := r.Group("/api/v1")
	{
		// 刷新接口不严格限流（客户端可能频繁刷新，access 过期就会调）
		pub.POST("/token/refresh", Refresh)
	}

	// 登录/注册单独限流：防撞库、防批量注册（同一 IP 每分钟最多 20 次）
	authGroup := r.Group("/api/v1", middleware.RateLimit(20, time.Minute))
	{
		authGroup.POST("/register", Register)
		authGroup.POST("/login", Login)
	}

	// 鉴权组：整组走 Auth 中间件（漏挂鉴权是最常见的安全事故，用组能防住）
	priv := r.Group("/api/v1", middleware.Auth())
	{
		priv.POST("/logout", Logout)
		priv.GET("/users/me", GetUserProfile)
		priv.PUT("/users/me", UpdateUserProfile)
		// 搜索接口单独限流：防批量扫用户
		priv.GET("/users/search", middleware.RateLimit(30, time.Minute), SearchUsers)
		priv.GET("/users/settings", GetSettings)
		priv.PUT("/users/settings", UpdateSettings)
	}
}
