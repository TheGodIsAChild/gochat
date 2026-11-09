package model

import "time"

// User 对应 users 表。
//
// gorm tag 说明：
//   - primaryKey / autoIncrement：主键、自增
//   - type:varchar(32)：指定建表字段类型
//   - uniqueIndex：唯一索引（username 不能重复）
//   - not null / default:''：非空、默认值
//   - json:"-"：序列化 JSON 时永不输出（密码哈希绝不能泄露给前端）
type User struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(32);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	Nickname     string    `gorm:"type:varchar(32);not null;default:''" json:"nickname"`
	Avatar       string    `gorm:"type:varchar(255);not null;default:''" json:"avatar"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 显式指定表名，避免 GORM 默认复数化带来的歧义。
func (User) TableName() string { return "users" }
