package config

// GRPC 微服务地址配置。
//
// 【M6 微服务化】
// 单体时所有业务在一个进程里；拆开后：
//   - 用户服务（user）  监听 UserAddr
//   - 聊天服务（chat）  监听 ChatAddr
//   - gateway（HTTP 入口）通过 gRPC 调用这两个服务
//
// 生产环境这些地址一般来自服务发现（Consul/Etcd/K8s Service），
// 学习项目直接写配置。
type GRPC struct {
	UserAddr string `yaml:"user_addr"`
	ChatAddr string `yaml:"chat_addr"`
}
