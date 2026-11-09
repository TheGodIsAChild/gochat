package db

import (
	"context"
	"fmt"
	"gochat/global"

	"github.com/redis/go-redis/v9"
)

func LoadRedis() (*redis.Client, error) {
	redisCfg := global.Config.Redis

	client := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})
	global.Redis = client
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis 连接失败: %w", err)
	}
	return client, nil
}
