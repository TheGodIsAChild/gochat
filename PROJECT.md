# gochat — 仿微信 + 语音通话（Go 项目主状态文档）

> **这是跨会话续接锚点。** 无论断线还是开新 chat，你只需说「继续 gochat 项目」，我（或任何会话）先读本文件 + `../gochat-study/plan.md` 就能立刻接上进度。

## 一、项目定位

从零构建一个带语音通话的仿微信即时通讯（IM）项目，作为面试项目，边做边学：

- 已会：Gin / GORM / Redis / MySQL
- 本项目重点学：**并发编程**（goroutine / channel / context / 连接管理）、**微服务**（服务拆分 / gRPC）、**消息队列**（RabbitMQ）

## 二、技术栈（默认选型，可协商调整）

| 层 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.27 | 本机已装 1.27.3 |
| HTTP 框架 | Gin | 已会 |
| ORM | GORM | 已会，MySQL 驱动 |
| 数据库 | MySQL 8 | Docker 起 |
| 缓存/在线状态/会话 | Redis 7 | Docker 起，go-redis/v9 |
| 实时消息 | WebSocket | gorilla/websocket |
| 语音通话 | WebRTC | Pion `pion/webrtc/v3`，信令走 WebSocket |
| 消息队列 | RabbitMQ | amqp091-go（默认；可选 Kafka） |
| 微服务 RPC | gRPC + protobuf | 拆分 gateway/user/message/call |
| 认证 | JWT | github.com/golang-jwt/jwt/v5 |
| 配置 | yaml | gopkg.in/yaml.v3 |

## 三、工作区约定

```
C:\Users\Mr.Du\Desktop\deepseekHarness\
  gochat\            # 项目代码（独立 git 仓库，从零写）
  gochat-study\      # 规划文档（见下）
  study\             # 旧项目 cortex 的文档，与本项目无关，别混
  reference\         # 旧项目 cortex 只读引用，与本项目无关
```

**gochat 内部结构（权威版见 `gochat-study/structure.md`）**：采用「`core/` 启动装配 + `global/` 全局状态 + `internal/` 业务」风格；`internal/model|service|router` 按域拆子包，`internal/config` 保持扁平。

**gochat-study 文档索引**：
- `structure.md` —— 项目结构 + 命名/拆分规则（权威）
- `plan.md` —— 9 个里程碑总计划
- `db-schema.md` —— 完整数据库设计（9 域约 25 表，标注里程碑）
- `m0-task.md` / `m0-checklist.md` —— M0 任务卡/自检
- `m1-design.md` / `m1-models.md` / `m1-auth.md` / `m1-task.md` —— M1 设计/模型/bcrypt+JWT/任务卡

## 四、里程碑进度

| 里程碑 | 内容 | 状态 | commit | 验收 |
|---|---|---|---|---|
| M0 | 环境 + 骨架 + 配置 | ✅ 完成（用户手写） | 待提交 | build/vet/run 全绿，curl /health → 200 |
| M1 | 用户 + 好友（GORM 关联） | ⬜ 待你从0手写 | — | — |
| M2 | 单聊文本 + WebSocket + 在线状态 | ⬜ | — | — |
| M3 | 群聊 + 广播 + Redis pub/sub | ⬜ | — | — |
| M4 | 并发专题：连接管理 / 优雅退出 / 压测 | ⬜ | — | — |
| M5 | 语音通话（Pion WebRTC + 信令） | ⬜ | — | — |
| M6 | 微服务拆分（gRPC） | ⬜ | — | — |
| M7 | 消息队列（RabbitMQ 异步落库/削峰/可靠投递） | ⬜ | — | — |
| M8 | 面试收尾：文档 / 架构图 / 难点复盘 | ⬜ | — | — |

## 五、当前状态

- **项目状态**：✅ M0 完成；✅ **M1 全部完成**（认证 + 用户 + 好友）。
  - M0：`internal/config`、`core.LoadConfig`、`global`、`internal/router`、`main.go`。
  - M1 认证：`internal/auth/`（bcrypt / 双 token HS256+jti+type / Redis jti 白名单+GETDEL 原子消费）、`internal/middleware/auth.go`、`user_service`（Register/Login/Refresh rotation/Logout）、`user_router`。
  - M1 用户：`GET/PUT /users/me`、`GET /users/search`、`GET/PUT /users/settings`。
  - M1 好友：`friend_service`（发申请含 6 条规则 / 收申请 / **接受申请事务 + 条件更新抗并发** / 拒绝 / 好友列表 / 备注 / 删除 / 黑名单）、`friend_router`。
  - **✅ 端到端实测全过**：认证 9 项 + 用户搜索/资料/设置 + 好友全流程（申请→接受→列表→重复接受拦截→删除→软删后重新加回恢复旧行）+ 黑名单。
- **下一步**：**M2 单聊** —— conversations/messages 模型、发消息/拉历史、WebSocket 长连接、在线状态、离线消息。
- **文档**：`gochat-study/learn/00-学习总览.md`、`learn/m1-认证与好友.md`、`面试集.md`、`场景集.md`。
- **数据库状态**：6 张表已建（含测试数据）
- **git**：M1 分步提交（`95bebcf` 认证 / `cb456d4` 资料 / `bb8c74d` 好友）

## 六、关键决策记录（追加式）

- 2026-11-09 确认：
  1. **前端范围**：最小 Web 页面（原生 HTML/JS）+ 浏览器真实通话（M5 能可视化演示）。
  2. **消息队列**：RabbitMQ（amqp091-go）。
- 架构节奏：先单体（M0–M5）跑通，再拆微服务（M6）+ 接 MQ（M7）。
- 2026-11-09 **辅导模式确认（重要）**：用户要求**从 0 手写代码，不接受现成实现**。我的角色固定为：设计讲解 + 项目结构 + 接口草图 + 代码审查 + 答疑；**不提供可直接复制的完整实现**。

## 七、环境速查（重要，别踩坑）

- **MySQL**：`127.0.0.1:3307`（host 3307 → 容器 3306），库 `gochat`，账号 `gochat/gochat123`，root 密码 `root`。
  - 本机原生 `mysqld` 占用 3306，**不要动它**；Docker MySQL 故意映射到 3307。
- **Redis**：`127.0.0.1:6379`，无密码。
- **RabbitMQ**：AMQP `amqp://gochat:gochat123@127.0.0.1:5672/`，管理台 `http://127.0.0.1:15672`（账号 gochat/gochat123）。
- **Docker 镜像已在本机**：`redis:7-alpine`、`mysql:8.0`、`rabbitmq:3-management`。**不要再拉**。
  - 直连 Docker Hub 在国内不通、`daemon.json` 旧镜像源已失效。若确需拉新镜像（如 M6 的 etcd/kafka），用：`docker pull docker.m.daocloud.io/library/<镜像>:<tag>` 再 `docker tag ... <镜像>:<tag>`。可用源：`docker.m.daocloud.io`、`docker.1ms.run`、`docker.xuanyuan.me`。
- 启动命令：`cd gochat && docker compose up -d --pull=never`
