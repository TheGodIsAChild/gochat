package core

import (
	"os"
	"strconv"

	"gochat/global"
)

// ApplyEnvOverrides 用环境变量覆盖配置，供容器化部署使用。
//
// 【思路：配置优先级 默认值 < config.yaml < 环境变量】
// 本地开发时 config.yaml 里写的是 127.0.0.1；
// 容器里没有 127.0.0.1 这些服务，需要用环境变量指向 compose 的服务名（mysql / redis / rabbitmq）。
// 这样做的好处：
//  1. 同一份镜像可以部署到任何环境，不用为不同环境打不同镜像
//  2. 密码通过环境变量注入，不写死在镜像里
//
// 【注意点】只有环境变量「存在且非空」时才覆盖，避免 CI 里空变量把配置清空。
// 【面试点】这是 12-Factor App 的「配置与代码分离」原则：配置放环境，代码只读配置。
func ApplyEnvOverrides() {
	// ---- 服务 ----
	setString(&global.Config.Server.Addr, "SERVER_ADDR")
	setString(&global.Config.Server.Mode, "SERVER_MODE")

	// ---- MySQL ----
	setString(&global.Config.Mysql.Host, "MYSQL_HOST")
	setInt(&global.Config.Mysql.Port, "MYSQL_PORT")
	setString(&global.Config.Mysql.User, "MYSQL_USER")
	setString(&global.Config.Mysql.Password, "MYSQL_PASSWORD")
	setString(&global.Config.Mysql.Database, "MYSQL_DATABASE")
	setString(&global.Config.Mysql.Charset, "MYSQL_CHARSET")

	// ---- Redis ----
	setString(&global.Config.Redis.Addr, "REDIS_ADDR")
	setString(&global.Config.Redis.Password, "REDIS_PASSWORD")
	setInt(&global.Config.Redis.DB, "REDIS_DB")

	// ---- RabbitMQ（可选依赖） ----
	setString(&global.Config.Rabbitmq.Url, "RABBITMQ_URL")

	// ---- JWT ----
	setString(&global.Config.JWT.Secret, "JWT_SECRET")
	setInt(&global.Config.JWT.AccessExpireMinutes, "JWT_ACCESS_EXPIRE_MINUTES")
	setInt(&global.Config.JWT.RefreshExpireHours, "JWT_REFRESH_EXPIRE_HOURS")

	// ---- gRPC 服务地址（单进程部署时是 127.0.0.1，拆进程部署时可指向其他容器） ----
	setString(&global.Config.GRPC.UserAddr, "GRPC_USER_ADDR")
	setString(&global.Config.GRPC.ChatAddr, "GRPC_CHAT_ADDR")
}

func setString(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(dst *int, key string) {
	v := os.Getenv(key)
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return
	}
	*dst = n
}
