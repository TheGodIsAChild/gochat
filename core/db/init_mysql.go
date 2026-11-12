package db

import (
	"fmt"
	"gochat/global"
	call "gochat/internal/model/call_model"
	chat "gochat/internal/model/chat_model"
	friend "gochat/internal/model/friend_model"
	group "gochat/internal/model/group_model"
	user "gochat/internal/model/user_model"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func LoadMysql() (*gorm.DB, error) {
	dsn := global.Config.Mysql.DB_DSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true, // 不生成物理外键，约束交给应用层
		// TranslateError: 把「驱动层错误」翻译成 GORM 的哨兵错误，
		// 例如 MySQL 的 Duplicate entry → gorm.ErrDuplicatedKey。
		// 这样业务代码可以用 errors.Is(err, gorm.ErrDuplicatedKey) 判断，而不用匹配错误字符串。
		TranslateError: true,
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
	// AutoMigrate 会：建表 / 加缺失的列 / 加索引（不会删列，所以改字段类型要小心）
	// 开发阶段用它很方便，生产环境一般用专门的迁移工具（golang-migrate 等）。
	err := global.DB.AutoMigrate(
		// M1 用户 + 好友
		&friend.Blacklist{},
		&friend.FriendGroup{},
		&friend.FriendRequest{},
		&friend.Friendship{},
		&user.User{},
		&user.UserSettings{},
		// M2 单聊
		&chat.Conversation{},
		&chat.ConversationMember{},
		&chat.Message{},
		// M3 群聊
		&group.Group{},
		// M5 通话
		&call.Call{},
	)
	if err != nil {
		return false, fmt.Errorf("初始化数据库表失败: %w", err)
	}
	return true, nil
}
