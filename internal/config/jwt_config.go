package config

type JWT struct {
	Secret      string
	ExpireHours int `yaml:"expire_hours"`
}
