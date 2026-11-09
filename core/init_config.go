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

type FlagConfig struct {
	ConfigPath string
	InitDB     bool
}

var flagConfig = new(FlagConfig)

func LoadConfigFile() {
	flag.StringVar(&flagConfig.ConfigPath, "config", "./config.yaml", "默认使用config.yaml")
	flag.BoolVar(&flagConfig.InitDB, "db", false, "是否初始化数据库")
	flag.Parse()
	global.ConfigPath = &flagConfig.ConfigPath
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
	if _, err := db.LoadMysql(); err != nil {
		fmt.Print(fmt.Errorf("数据库连接失败：%w", err))
		os.Exit(0)
	}
	if flagConfig.InitDB {
		if _, err := db.InitMysqlTables(); err != nil {
			fmt.Print(fmt.Errorf("数据库建表初始化错误：%w", err))
			os.Exit(0)
		}
	}
	if _, err := db.LoadRedis(); err != nil {
		fmt.Print(fmt.Errorf("初始化Redis失败：%w", err))
	}
	mq.LoadMq()
	jwt.LoadJWT()
	server.LoadServer()
}
