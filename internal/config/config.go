package config

type Config struct {
	Mysql    Mysql
	Redis    Redis
	Rabbitmq Rabbitmq
	JWT      JWT
	Server   Server
	GRPC     GRPC
	RTC      RTC
}
