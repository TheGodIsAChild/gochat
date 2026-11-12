package db

import (
	"fmt"
	"gochat/global"
	friend "gochat/internal/model/friend_model"
	user "gochat/internal/model/user_model"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func LoadMysql() (*gorm.DB, error) {
	dsn := global.Config.Mysql.DB_DSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true, //不生成实体外键
	})
	if err != nil {
		return db, fmt.Errorf("数据库连接Open失败，请检查数据库状态：%w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return db, fmt.Errorf("数据库连接Open失败，请检查数据库状态：%w", err)
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)
	global.DB = db
	fmt.Printf("数据库连接成功")
	return db, nil
}

func InitMysqlTables() (bool, error) {
	err := global.DB.AutoMigrate(
		&friend.Blacklist{},
		&friend.FriendGroup{},
		&friend.FriendRequest{},
		&friend.Friendship{},
		&user.User{},
		&user.UserSettings{},
	)
	if err != nil {
		fmt.Print("初始化数据库表失败")
		return false, err
	}
	return true, nil
}
