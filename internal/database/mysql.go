package database

import (
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gochat/internal/config"
)

// db 保存全局数据库连接，供其他包通过 Get() 使用。
var db *gorm.DB

// Init 建立 MySQL 连接并配置连接池。
// 返回的 *gorm.DB 可以继续用于 AutoMigrate、事务等。
func Init(cfg *config.Config) (*gorm.DB, error) {
	// DSN（Data Source Name）格式：
	// user:password@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	// parseTime=True：把 MySQL 的 DATETIME 自动转成 Go 的 time.Time
	// loc=Local：使用本地时区
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.MySQL.User,
		cfg.MySQL.Password,
		cfg.MySQL.Host,
		cfg.MySQL.Port,
		cfg.MySQL.Database,
		cfg.MySQL.Charset,
	)

	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		// Info 级别会打印执行的 SQL，学习阶段很方便；生产环境可调成 Warn。
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 连接池配置：
	// MaxOpenConns 最大打开连接数；MaxIdleConns 最大空闲连接数；
	// ConnMaxLifetime 连接最长存活时间（避免 MySQL 踢掉长时间空闲连接）。
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// Get 返回全局 *gorm.DB。
func Get() *gorm.DB {
	return db
}
