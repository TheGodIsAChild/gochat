package config

// RTC 是 WebRTC 通话的 ICE 服务器配置。
//
// 【为什么需要】
// 同一台机器的两个窗口可以靠 host candidate 直连；但两台设备跨网络时，
// 浏览器需要 STUN 来发现自己的公网地址，严格 NAT 下还需要 TURN 中继，
// 否则双方找不到可用路径，通话会一直「接通中」然后失败。
//
// 【配置方式（任选其一）】
//  1. config.yaml 里写死（本地开发）
//  2. 环境变量（容器部署，推荐）：
//     RTC_STUN_URLS=stun:stun.l.google.com:19302,stun:stun.miwifi.com:3478
//     RTC_TURN_URL=turn:turn.example.com:3478
//     RTC_TURN_USER=user
//     RTC_TURN_PASS=pass
//
// 【面试点】STUN 只解决「知道自己在公网是什么地址」，不做转发；
// 对称 NAT（常见于 4G/企业网）无法打洞，必须依赖 TURN 中继，中继会消耗服务器带宽。
type RTC struct {
	StunURLs []string `yaml:"stun_urls"`
	TurnURL  string   `yaml:"turn_url"`
	TurnUser string   `yaml:"turn_user"`
	TurnPass string   `yaml:"turn_pass"`
}
