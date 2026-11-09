package global

import (
	"gochat/internal/config"

	"gorm.io/gorm"
)

var ConfigPath *string
var Config config.Config
var DB *gorm.DB // 全局 GORM 连接
