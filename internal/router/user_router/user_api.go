// Package userapi 用户相关 HTTP 接口
package userapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gochat/internal/middleware"
	userservice "gochat/internal/service/user_service"
)

// ---------- 请求体 ----------

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

// ---------- 统一响应 ----------

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}

// ---------- Handlers ----------

func Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	u, err := userservice.UserRegister(req.Username, req.Password, req.Nickname)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, gin.H{"user_id": u.ID, "username": u.Username, "nickname": u.Nickname})
}

func Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	access, refresh, u, err := userservice.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	ok(c, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"user": gin.H{
			"id": u.ID, "username": u.Username, "nickname": u.Nickname, "avatar": u.Avatar,
		},
	})
}

func Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	access, refresh, err := userservice.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	ok(c, gin.H{"access_token": access, "refresh_token": refresh})
}

func Logout(c *gin.Context) {
	var req logoutReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	if err := userservice.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		fail(c, http.StatusUnauthorized, err.Error())
		return
	}
	ok(c, nil)
}

func GetUserProfile(c *gin.Context) {
	userId := c.GetUint64("userId")
	if userId == 0 {
		fail(c, http.StatusUnauthorized, "用户未登录")
		return
	}
	var userProfile userservice.UserProfile
	userProfile, err := userservice.UserInfo(c, uint64(userId))
	if err != nil {
		fail(c, http.StatusBadRequest, "用户信息获取错误错误: "+err.Error())
		return
	}
	ok(c, userProfile)
}

// ---------- 路由注册 ----------

func UserRouter(r *gin.Engine) {
	// 公开组：不需要登录
	pub := r.Group("/api/v1")
	{
		pub.POST("/register", Register)
		pub.POST("/login", Login)
		pub.POST("/token/refresh", Refresh)
	}

	// 鉴权组：整组走 Auth 中间件
	priv := r.Group("/api/v1", middleware.Auth())
	{
		priv.POST("/logout", Logout)
		priv.GET("/users/me", GetUserProfile)
	}
}
