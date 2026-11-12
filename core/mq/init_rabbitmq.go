package mq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"gochat/global"
)

// QueueName 统一的事件队列名。
const QueueName = "gochat.events"

// LoadMq 连接 RabbitMQ 并声明队列。
//
// 【持久化设置】
//   - QueueDeclare 的 durable=true：队列在 RabbitMQ 重启后还在
//   - Publish 时 DeliveryMode=2：消息持久化到磁盘
//
// 注意：RabbitMQ 默认是「至少一次」投递，不是「恰好一次」。
// 所以消费方必须做幂等（本项目消费消息是「推送给在线用户」，重复推问题不大；
// 如果是「加积分」这类操作就必须去重）。
func LoadMq() error {
	conn, err := amqp.Dial(global.Config.Rabbitmq.Url)
	if err != nil {
		return fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("打开 RabbitMQ channel 失败: %w", err)
	}

	// 声明持久化队列（不存在则创建）
	if _, err := ch.QueueDeclare(
		QueueName,
		true,  // durable：持久化
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("声明队列失败: %w", err)
	}

	global.MQ = &global.MQClient{
		Conn:      conn,
		Channel:   ch,
		Enabled:   true,
		QueueName: QueueName,
	}
	return nil
}

// CloseMq 关闭连接（优雅退出时调用）。
func CloseMq() {
	if global.MQ == nil {
		return
	}
	if global.MQ.Channel != nil {
		_ = global.MQ.Channel.Close()
	}
	if global.MQ.Conn != nil {
		_ = global.MQ.Conn.Close()
	}
	global.MQ = nil
}
