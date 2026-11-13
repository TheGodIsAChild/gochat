---
title: "源码走读 S19：server.go"
date: 2026-11-14 23:03:00
categories:
  - 05-源码
tags:
  - 源码
---
> **文件路径**：`internal/grpcx/server.go`　**行数**：265 行
> **职责**：gRPC 微服务：服务端分发、客户端 Invoke、生命周期

本文是项目的**完整源码**（未删减）。项目源码里保留了
**【思路】【注意点】【面试点】** 三类中文注释——建议边读边对照，
遇到不理解的函数名，回看对应里程碑的设计文章。

---

```go
package grpcx

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"gochat/global"
	chatservice "gochat/internal/service/chat_service"
	usermodel "gochat/internal/model/user_model"
)

// ============================ 服务端 ============================

// userServiceHandler 用户服务的 gRPC 处理器。
//
// 【实现方式说明】
// 标准 protobuf 流程会生成 `RegisterUserServiceServer(s, impl)`，由 gRPC 框架按方法分发。
// 这里没有 protoc，所以用 `grpc.UnknownServiceHandler` 自己按「完整方法名」分发——
// 效果等价：收到 /user.v1.UserService/GetUser → 解码请求 → 调业务 → 编码响应。
func userServiceHandler(_ any, stream grpc.ServerStream) error {
	method, _ := grpc.MethodFromServerStream(stream)
	ctx := stream.Context()

	switch method {
	case MethodGetUser:
		var req GetUserRequest
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		var u usermodel.User
		if err := global.DB.WithContext(ctx).First(&u, req.ID).Error; err != nil {
			return status.Errorf(codes.NotFound, "用户不存在")
		}
		return stream.SendMsg(&User{
			ID:        uint64(u.ID),
			Username:  u.Username,
			Nickname:  u.Nickname,
			Avatar:    u.Avatar,
			Gender:    int32(u.Gender),
			Signature: u.Signature,
		})

	case MethodBatchGetUsers:
		var req BatchGetUsersRequest
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		var users []usermodel.User
		if len(req.IDs) > 0 {
			if err := global.DB.WithContext(ctx).Where("id IN ?", req.IDs).Find(&users).Error; err != nil {
				return status.Errorf(codes.Internal, "查询失败: %v", err)
			}
		}
		out := make([]User, 0, len(users))
		for _, u := range users {
			out = append(out, User{
				ID:        uint64(u.ID),
				Username:  u.Username,
				Nickname:  u.Nickname,
				Avatar:    u.Avatar,
				Gender:    int32(u.Gender),
				Signature: u.Signature,
			})
		}
		return stream.SendMsg(&BatchGetUsersResponse{Users: out})

	default:
		return status.Errorf(codes.Unimplemented, "未知方法 %s", method)
	}
}

// chatServiceHandler 聊天服务的 gRPC 处理器。
func chatServiceHandler(_ any, stream grpc.ServerStream) error {
	method, _ := grpc.MethodFromServerStream(stream)
	ctx := stream.Context()

	switch method {
	case MethodSendMessage:
		var req SendMessageRequest
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		msg, err := chatservice.SendMessage(ctx, req.SenderID, req.ConversationID, req.ClientMsgID, int8(req.Type), req.Content)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return stream.SendMsg(&Message{
			ID:             uint64(msg.ID),
			ConversationID: uint64(msg.ConversationID),
			SenderID:       uint64(msg.SenderID),
			ClientMsgID:    msg.ClientMsgID,
			Type:           int32(msg.Type),
			Content:        msg.Content,
			CreatedAt:      msg.CreatedAt.Unix(),
		})

	case MethodListMessages:
		var req ListMessagesRequest
		if err := stream.RecvMsg(&req); err != nil {
			return err
		}
		items, err := chatservice.ListMessages(ctx, req.UserID, req.ConversationID, req.BeforeID, int(req.Limit))
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "%v", err)
		}
		out := make([]Message, 0, len(items))
		for _, it := range items {
			out = append(out, Message{
				ID:             it.ID,
				ConversationID: it.ConversationID,
				SenderID:       it.SenderID,
				ClientMsgID:    it.ClientMsgID,
				Type:           int32(it.Type),
				Content:        it.Content,
				CreatedAt:      it.CreatedAt.Unix(),
			})
		}
		return stream.SendMsg(&ListMessagesResponse{Messages: out})

	default:
		return status.Errorf(codes.Unimplemented, "未知方法 %s", method)
	}
}

// Servers 持有两个 gRPC 服务。
type Servers struct {
	user *grpc.Server
	chat *grpc.Server
}

// StartServers 启动用户服务和聊天服务（两个独立端口）。
//
// 【这就是微服务的雏形】
// 用户服务和聊天服务各自监听端口、各自独立部署；调用方通过 gRPC 访问。
// 单体时它们只是同一个进程里的两个包，拆开后就是两个可独立扩缩容的服务。
func StartServers(userAddr, chatAddr string) (*Servers, error) {
	userSrv := grpc.NewServer(
		grpc.ForceServerCodec(JSONCodec{}),
		grpc.UnknownServiceHandler(userServiceHandler),
	)
	chatSrv := grpc.NewServer(
		grpc.ForceServerCodec(JSONCodec{}),
		grpc.UnknownServiceHandler(chatServiceHandler),
	)

	userLis, err := net.Listen("tcp", userAddr)
	if err != nil {
		return nil, err
	}
	chatLis, err := net.Listen("tcp", chatAddr)
	if err != nil {
		_ = userLis.Close()
		return nil, err
	}

	go func() { _ = userSrv.Serve(userLis) }()
	go func() { _ = chatSrv.Serve(chatLis) }()

	return &Servers{user: userSrv, chat: chatSrv}, nil
}

// Stop 优雅停止所有 gRPC 服务。
func (s *Servers) Stop() {
	if s == nil {
		return
	}
	if s.user != nil {
		s.user.GracefulStop()
	}
	if s.chat != nil {
		s.chat.GracefulStop()
	}
}

// ============================ 客户端（gateway 用）============================

var (
	userConn *grpc.ClientConn
	chatConn *grpc.ClientConn
)

// Dial 建立到两个 gRPC 服务的客户端连接。
func Dial(userAddr, chatAddr string) error {
	uc, err := grpc.NewClient(userAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	cc, err := grpc.NewClient(chatAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = uc.Close()
		return err
	}
	userConn = uc
	chatConn = cc
	return nil
}

// Close 关闭客户端连接。
func Close() {
	if userConn != nil {
		_ = userConn.Close()
	}
	if chatConn != nil {
		_ = chatConn.Close()
	}
}

// CallGetUser 调用用户服务：查用户。
func CallGetUser(ctx context.Context, id uint64) (*User, error) {
	if userConn == nil {
		return nil, errors.New("gRPC 用户服务未连接")
	}
	var resp User
	err := userConn.Invoke(ctx, MethodGetUser, &GetUserRequest{ID: id}, &resp, grpc.ForceCodec(JSONCodec{}))
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CallBatchGetUsers 调用用户服务：批量查用户。
func CallBatchGetUsers(ctx context.Context, ids []uint64) ([]User, error) {
	if userConn == nil {
		return nil, errors.New("gRPC 用户服务未连接")
	}
	var resp BatchGetUsersResponse
	err := userConn.Invoke(ctx, MethodBatchGetUsers, &BatchGetUsersRequest{IDs: ids}, &resp, grpc.ForceCodec(JSONCodec{}))
	if err != nil {
		return nil, err
	}
	return resp.Users, nil
}

// CallSendMessage 调用聊天服务：发消息。
func CallSendMessage(ctx context.Context, req *SendMessageRequest) (*Message, error) {
	if chatConn == nil {
		return nil, errors.New("gRPC 聊天服务未连接")
	}
	var resp Message
	err := chatConn.Invoke(ctx, MethodSendMessage, req, &resp, grpc.ForceCodec(JSONCodec{}))
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// CallListMessages 调用聊天服务：拉历史消息。
func CallListMessages(ctx context.Context, req *ListMessagesRequest) ([]Message, error) {
	if chatConn == nil {
		return nil, errors.New("gRPC 聊天服务未连接")
	}
	var resp ListMessagesResponse
	err := chatConn.Invoke(ctx, MethodListMessages, req, &resp, grpc.ForceCodec(JSONCodec{}))
	if err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

```
