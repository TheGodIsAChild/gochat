// Package rtcapi 提供 WebRTC ICE 服务器配置的 HTTP 接口。
//
// 【为什么要由后端下发】
// STUN/TURN 地址可能因部署环境不同（内网/公网/TURN 账号密码），
// 写死在前端 JS 里会导致换环境就要改代码、重新构建。
// 由后端通过配置返回，前端启动时拉一次即可。
//
// 【安全提醒】本接口返回的 TURN 账号密码对所有访问者可见。
// 演示环境可接受；生产建议改为「短期临时凭证」（TURN REST API / time-limited credentials）。
package rtcapi

import (
	"github.com/gin-gonic/gin"

	"gochat/global"
	"gochat/internal/router/resp"
)

// GetRTCConfig 返回前端创建 RTCPeerConnection 所需的 iceServers。
//
// GET /api/v1/rtc/config
//
// 返回示例：
//
//	{"code":0,"data":{"ice_servers":[
//	   {"urls":["stun:stun.l.google.com:19302","stun:stun.miwifi.com:3478"]},
//	   {"urls":["turn:turn.example.com:3478"],"username":"u","credential":"p"}
//	]}}
func GetRTCConfig(c *gin.Context) {
	servers := make([]gin.H, 0, 2)

	// STUN：用于发现公网地址
	if len(global.Config.RTC.StunURLs) > 0 {
		servers = append(servers, gin.H{"urls": global.Config.RTC.StunURLs})
	}

	// TURN：打洞失败时中继（对称 NAT / 4G 必备）
	if global.Config.RTC.TurnURL != "" {
		servers = append(servers, gin.H{
			"urls":       []string{global.Config.RTC.TurnURL},
			"username":   global.Config.RTC.TurnUser,
			"credential": global.Config.RTC.TurnPass,
		})
	}

	resp.OK(c, gin.H{
		"ice_servers": servers,
		// 提示前端：是否处于安全上下文（跨设备 http://IP 访问时浏览器会禁止麦克风）
		"secure_context_required": true,
	})
}

// RTCRouter 注册 RTC 相关路由（公开接口，不需要登录）。
func RTCRouter(r *gin.Engine) {
	pub := r.Group("/api/v1")
	{
		pub.GET("/rtc/config", GetRTCConfig)
	}
}
