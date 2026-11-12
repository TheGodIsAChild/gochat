package global

import (
	"gochat/internal/config"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ConfigPath *string
var Config config.Config
var DB *gorm.DB // 全局 GORM 连接
var Redis *redis.Client

// MQClient RabbitMQ 客户端（M7）。
//
// 【为什么要有 Enabled 字段】
// RabbitMQ 是可选依赖：连不上时应用要能降级运行（消息改为同步推送），
// 而不是直接启动失败。所以用 Enabled 标记是否真正可用。
type MQClient struct {
	Conn      *amqp.Connection
	Channel   *amqp.Channel
	Enabled   bool
	QueueName string
}

var MQ *MQClient
