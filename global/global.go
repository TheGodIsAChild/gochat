package global

import (
	"gochat/internal/config"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var ConfigPath *string
var Config config.Config
var DB *gorm.DB // 全局 GORM 连接
var Redis *redis.Client
