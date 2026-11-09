package config

import (
	"fmt"
)

type Mysql struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	Charset  string
}

func (mysql Mysql) DB_DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		mysql.User, mysql.Password, mysql.Host, mysql.Port, mysql.Database, mysql.Charset)
}
