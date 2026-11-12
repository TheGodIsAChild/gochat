// Package grpcx 微服务化（M6）：服务间用 gRPC 通信。
//
// 【为什么用 JSON codec 而不是 protobuf】
// 标准做法是写 .proto → protoc 生成 Go 代码 → protobuf 序列化。
// 本机没有 protoc/buf（buf 安装被网络中断），所以这里用 gRPC 的
// 「自定义 codec」能力：传输层仍然是真正的 gRPC（HTTP/2、超时、状态码），
// 只是序列化用 JSON。
//
// 换成 protobuf 只需要：把下面的 struct 换成 .proto 生成的类型，
// 把 JSONCodec 换成默认 proto codec，服务实现的逻辑完全不用改。
// 这正好说明：**微服务的核心是「服务边界划分 + 接口契约」，不是序列化格式**。
package grpcx

const (
	// UserService 用户服务
	UserServiceName = "user.v1.UserService"
	// ChatService 聊天服务
	ChatServiceName = "chat.v1.ChatService"

	// 完整方法名（gRPC 用 /package.Service/Method 定位方法）
	MethodGetUser        = "/user.v1.UserService/GetUser"
	MethodBatchGetUsers  = "/user.v1.UserService/BatchGetUsers"
	MethodSendMessage    = "/chat.v1.ChatService/SendMessage"
	MethodListMessages   = "/chat.v1.ChatService/ListMessages"
)

// ---------- 用户服务契约 ----------

type GetUserRequest struct {
	ID uint64 `json:"id"`
}

type BatchGetUsersRequest struct {
	IDs []uint64 `json:"ids"`
}

type BatchGetUsersResponse struct {
	Users []User `json:"users"`
}

// User 用户服务的对外 DTO（注意：不包含密码等敏感字段）。
type User struct {
	ID        uint64 `json:"id"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Gender    int32  `json:"gender"`
	Signature string `json:"signature"`
}

// ---------- 聊天服务契约 ----------

type SendMessageRequest struct {
	SenderID       uint64 `json:"sender_id"`
	ConversationID uint64 `json:"conversation_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Type           int32  `json:"type"`
	Content        string `json:"content"`
}

type ListMessagesRequest struct {
	UserID         uint64 `json:"user_id"`
	ConversationID uint64 `json:"conversation_id"`
	BeforeID       uint64 `json:"before_id"`
	Limit          int32  `json:"limit"`
}

type ListMessagesResponse struct {
	Messages []Message `json:"messages"`
}

type Message struct {
	ID             uint64 `json:"id"`
	ConversationID uint64 `json:"conversation_id"`
	SenderID       uint64 `json:"sender_id"`
	ClientMsgID    string `json:"client_msg_id"`
	Type           int32  `json:"type"`
	Content        string `json:"content"`
	CreatedAt      int64  `json:"created_at"` // Unix 秒
}
