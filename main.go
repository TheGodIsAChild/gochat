package main

import (
	"fmt"
	"gochat/core"
	"gochat/global"
	"gochat/internal/router"
)

func main() {
	global.ConfigPath = core.LoadConfigFile()
	core.LoadConfig()
	core.InitBaseServices()
	fmt.Println("Config：", global.Config)
	r := router.NewRouter()
	r.Run(global.Config.Server.Addr)
}
