package grpcx

import "encoding/json"

// JSONCodec 用 JSON 做 gRPC 的序列化格式。
//
// gRPC 允许自定义 codec（encoding.Codec 接口：Marshal / Unmarshal / Name）。
// 传输层（HTTP/2、流控、超时、状态码）完全不变，只是 body 的编解码方式换了。
//
// 生产环境用 protobuf（体积小、跨语言、强 schema）；这里用 JSON 是为了
// 在「没有 protoc」的环境下也能完整演示 gRPC 的服务端/客户端/网关。
type JSONCodec struct{}

// Marshal 序列化。
func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal 反序列化。
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Name 返回 codec 名字（gRPC 内部用）。
func (JSONCodec) Name() string {
	return "json"
}
