package core

import (
	"flag"
	"fmt"
	"gochat/core/db"
	"gochat/core/jwt"
	"gochat/core/mq"
	"gochat/core/server"
	"gochat/global"
	"os"

	"github.com/goccy/go-yaml"
)

func LoadConfigFile() *string {
	configPath := flag.String("config", "config.yaml", "./config.yaml")
	flag.Parse()
	return configPath
}

func LoadConfig() (bool, error) {
	bytes, err := os.ReadFile(*global.ConfigPath)
	if err != nil {
		return false, fmt.Errorf("config 加载失败，ERROR：%w", err)
	}
	if err := yaml.Unmarshal(bytes, &global.Config); err != nil {
		return false, fmt.Errorf("yaml 配置解析错误：%w", err)
	}
	return true, nil

}

func InitBaseServices() {
	db.LoadMysql()
	db.LoadRedis()
	mq.LoadMq()
	jwt.LoadJWT()
	server.LoadServer()
}
