package config

// ⚠️ 参考实现：建议你先自己写一遍，卡住再对照本文件。

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 对应 config.yaml 的完整结构。
// 每个字段的 yaml tag 必须和 config.yaml 里的 key 严格一致。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	Redis    RedisConfig    `yaml:"redis"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	JWT      JWTConfig      `yaml:"jwt"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"` // 例：":8080"
	Mode string `yaml:"mode"` // gin 模式：debug / release / test
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Charset  string `yaml:"charset"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"` // 例："127.0.0.1:6379"
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type RabbitMQConfig struct {
	URL string `yaml:"url"` // 例："amqp://user:pass@host:5672/"
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

// global 保存加载后的全局配置，供 Get() 读取。
var global *Config

// Load 从 path 读取 yaml 文件并解析成 Config。
// 文件不存在、yaml 格式错误都会返回 error，由调用方决定怎么处理。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	global = &cfg
	return &cfg, nil
}

// Get 返回已加载的全局配置。必须在 Load 成功之后调用。
func Get() *Config {
	return global
}
