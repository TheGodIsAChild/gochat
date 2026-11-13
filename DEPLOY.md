# xiaohugochat 部署指南

> 一条 `docker compose up -d --build` 就能起完整环境：**应用 + MySQL + Redis + RabbitMQ**。
> 应用镜像名：`xiaohugochat`，包含 Go 后端、仿微信前端、62 篇学习博客。

---

## 一、服务器要求

| 项 | 要求 |
|---|---|
| 系统 | Linux（Ubuntu/Debian/CentOS 均可） |
| Docker | 20.10+，带 `docker compose` 插件（v2） |
| 内存 | ≥ 2 GB（MySQL + RabbitMQ 比较吃内存） |
| 磁盘 | ≥ 5 GB |
| 端口 | 8080 对外开放（可通过 `APP_PORT` 改） |

检查：

```bash
docker --version && docker compose version
```

---

## 二、方式 A：服务器上直接构建（推荐）

```bash
# 1. 拉代码
git clone https://github.com/TheGodIsAChild/gochat.git
cd gochat

# 2. 准备配置（可选，但生产必须改密码和 JWT 密钥）
cp .env.example .env
vim .env

# 3. 构建并启动
docker compose up -d --build

# 4. 看状态（app 显示 healthy 才算成功）
docker compose ps
```

**国内服务器**若拉不到基础镜像：

```bash
REGISTRY=docker.m.daocloud.io/library docker compose up -d --build
```

或在 `.env` 里写 `REGISTRY=docker.m.daocloud.io/library`。

访问：

| 地址 | 内容 |
|---|---|
| `http://<服务器IP>:8080/` | 仿微信前端 |
| `http://<服务器IP>:8080/blog/` | 学习博客 |
| `http://<服务器IP>:8080/health` | 健康检查（返回 `{"status":"ok"}`） |

首次启动会自动建表（容器命令带 `-db` 参数执行 AutoMigrate）。

---

## 三、方式 B：本地构建 → 传到服务器（服务器无外网 / 构建慢）

```bash
# ---- 本地 ----
cd gochat
REGISTRY=docker.m.daocloud.io/library docker compose build

# 导出镜像（约 40-80 MB，压缩后更小）
docker save xiaohugochat:latest | gzip > xiaohugochat.tar.gz

# 传到服务器
scp xiaohugochat.tar.gz docker-compose.yml .env user@SERVER:/opt/xiaohugochat/

# ---- 服务器 ----
cd /opt/xiaohugochat
docker load < xiaohugochat.tar.gz
docker compose up -d          # 注意：没有 --build，直接用导入的镜像
```

> 方式 B 只需要服务器能跑 MySQL/Redis/RabbitMQ 镜像。
> 若服务器也拉不到这三个镜像，可在本地 `docker save` 一并导出：
> `docker save xiaohugochat:latest mysql:8.0 redis:7-alpine rabbitmq:3-management | gzip > all.tar.gz`

---

## 四、配置项（.env）

| 变量 | 默认 | 说明 |
|---|---|---|
| `APP_PORT` | `8080` | 对外端口 |
| `TAG` | `latest` | 镜像 tag |
| `BUILD_TARGET` | `app-with-blog` | `app` = 不含博客（构建更快） |
| `REGISTRY` | `docker.io/library` | 基础镜像源，国内改 `docker.m.daocloud.io/library` |
| `JWT_SECRET` | `please-change-...` | **必须改**：JWT 签名密钥 |
| `MYSQL_ROOT_PASSWORD` | `root` | MySQL root 密码 |
| `MYSQL_USER` / `MYSQL_PASSWORD` | `gochat` / `gochat123` | 应用连库账号 |
| `MYSQL_DATABASE` | `gochat` | 库名 |
| `RABBITMQ_USER` / `RABBITMQ_PASSWORD` | `gochat` / `gochat123` | MQ 账号 |
| `MYSQL_IMAGE` / `REDIS_IMAGE` / `RABBITMQ_IMAGE` | 官方镜像 | 换镜像源用 |

> 改完 `.env` 后：`docker compose up -d`（配置变化会重建容器）。

---

## 五、常用运维命令

```bash
# 查看状态（容器 + 健康检查）
docker compose ps

# 看应用日志（实时）
docker compose logs -f app

# 只看最近 100 行
docker compose logs --tail 100 app

# 重启应用
docker compose restart app

# 停止全部（保留数据）
docker compose stop

# 停止并删除容器/网络（保留数据卷）
docker compose down

# ⚠️ 连数据一起删（重置数据库）
docker compose down -v

# 升级：拉新代码后重建
git pull && docker compose up -d --build app

# 进 MySQL
docker compose exec mysql mysql -uroot -p

# 进 Redis
docker compose exec redis redis-cli

# 看 RabbitMQ 队列（不暴露管理台时用命令行）
docker compose exec rabbitmq rabbitmqctl list_queues
```

---

## 六、数据备份

数据都在命名卷里：`xiaohugochat_mysql_data` / `xiaohugochat_redis_data` / `xiaohugochat_rabbitmq_data`。

```bash
# 备份 MySQL（导出 SQL）
docker compose exec -T mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases gochat \
  > backup_$(date +%F).sql

# 恢复
docker compose exec -T mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" < backup_2026-11-13.sql
```

---

## 七、发布到 Docker Hub（可选）

镜像已经构建好，想让它能被 `docker pull` 直接拉取：

```bash
# 1. 登录（需要 Docker Hub 账号）
docker login

# 2. 打标签（把 <用户名> 换成你的 Docker Hub 用户名）
docker tag xiaohugochat:latest <用户名>/xiaohugochat:latest
docker tag xiaohugochat:latest <用户名>/xiaohugochat:1.0.0

# 3. 推送
docker push <用户名>/xiaohugochat:latest
docker push <用户名>/xiaohugochat:1.0.0
```

如果已发布，服务器上可以不用构建，直接把 `docker-compose.yml` 里的 `build:` 段删掉，
并把 `image:` 改成 `<用户名>/xiaohugochat:latest` 即可。

---

## 八、跨设备语音通话（重要）

> 结论：**发消息开箱即用；语音通话需要 HTTPS + STUN（严格 NAT 还要 TURN）**。

### 8.1 为什么用 `http://<IP>` 打不了电话

浏览器规定 `getUserMedia`（采集麦克风）只在**安全上下文**可用：

| 访问方式 | 能否采集麦克风 |
|---|---|
| `http://localhost` / `http://127.0.0.1` | ✅ 可以 |
| `https://任意域名` | ✅ 可以 |
| `http://<服务器IP>`（手机/另一台电脑） | ❌ **被浏览器禁止** |

所以跨设备演示时，点电话图标会提示「语音通话需要 HTTPS」。

### 8.2 场景对照表

| 场景 | 发消息 | 语音通话 |
|---|---|---|
| 本机两个窗口（localhost） | ✅ | ✅ |
| 同一局域网，`http://IP` | ✅ | ❌ 麦克风被禁 |
| 同一局域网 + HTTPS | ✅ | ✅（STUN 通常可直连） |
| 跨网络 + HTTPS + STUN | ✅ | ⚠️ 多数可通，对称 NAT 会失败 |
| 跨网络 + HTTPS + STUN + TURN | ✅ | ✅ 稳定 |

### 8.3 第一步：配 HTTPS（Nginx + Let's Encrypt）

```nginx
server {
    listen 443 ssl http2;
    server_name chat.example.com;
    ssl_certificate     /etc/letsencrypt/live/chat.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/chat.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket 必须单独配置
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}

server {
    listen 80;
    server_name chat.example.com;
    return 301 https://$host$request_uri;
}
```

申请证书：

```bash
sudo apt install certbot python3-certbot-nginx -y
sudo certbot --nginx -d chat.example.com
```

> 没有域名怎么办：用 `mkcert` 自签证书（设备需安装根证书），
> 或用 Cloudflare Tunnel / ngrok 之类给你一个临时 HTTPS 域名。

### 8.4 第二步：STUN（默认已内置）

配置文件默认值（`config.yaml`）：

```yaml
rtc:
  stun_urls:
    - "stun:stun.l.google.com:19302"
    - "stun:stun.miwifi.com:3478"
    - "stun:stun.qq.com:3478"
```

容器部署可用环境变量覆盖：

```bash
RTC_STUN_URLS=stun:stun.l.google.com:19302,stun:stun.miwifi.com:3478
```

前端会通过 `GET /api/v1/rtc/config` 自动获取，**换环境不用改前端代码**。

### 8.5 第三步：TURN（4G / 对称 NAT 必备）

打洞失败时需要有中继服务器。用 coturn 在同一台机器上起一个：

```bash
docker run -d --name coturn --network host --restart unless-stopped \
  -e TURN_USERNAME=gochat \
  -e TURN_PASSWORD=change-me-turn-pass \
  -e TURN_REALM=chat.example.com \
  instrumentisto/coturn
```

> 国内拉不到镜像时加前缀：`docker.m.daocloud.io/instrumentisto/coturn`

注入到应用（`.env`）：

```bash
RTC_TURN_URL=turn:chat.example.com:3478
RTC_TURN_USER=gochat
RTC_TURN_PASS=change-me-turn-pass
```

然后 `docker compose up -d` 重建应用。

**防火墙必须放行**：

| 端口 | 协议 | 用途 |
|---|---|---|
| 3478 | TCP + UDP | TURN 监听 |
| 49152-65535 | UDP | TURN 中继端口范围 |

> 云服务器要在**安全组**里加，光开系统防火墙不够。

### 8.6 怎么判断通话到底走通了没

1. 两台设备打开 `https://chat.example.com`
2. 互相加好友 → 点电话图标 → 接听
3. 若一直「接通中」：
   - `https://chat.example.com/api/v1/rtc/config` 是否返回了 stun/turn
   - F12 → Console 是否出现 `ice:failed`
   - 打开 `chrome://webrtc-internals`，看候选类型：
     - `host` = 局域网直连
     - `srflx` = 经过 STUN 打通
     - `relay` = 走 TURN 中继（说明 TURN 生效）
   - 页面提示「需要 HTTPS」→ 说明还没配证书

---

## 九、安全清单（公网部署必看）

- [ ] `.env` 里的 `JWT_SECRET`、MySQL/MQ 密码都改成强随机值
- [ ] **不要**给 MySQL/Redis/RabbitMQ 映射宿主机端口（本 compose 默认不映射）
- [ ] 用 Nginx 反代 + HTTPS（`wss://` 要求 HTTPS，语音通话在公网才正常）
- [ ] 只开放必要端口（80/443），8080 不要直接暴露到公网
- [ ] 定期备份 MySQL 卷
- [ ] 生产环境建议把 `CMD ["-db"]` 改为单独的迁移步骤（避免每次重启都跑 AutoMigrate）

### Nginx 反代示例（含 WebSocket）

```nginx
server {
    listen 80;
    server_name chat.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # WebSocket 必须单独配
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

---

## 九、排障

| 现象 | 原因 | 解决 |
|---|---|---|
| `app` 一直 restarting | MySQL 还没健康就启动失败了 | `docker compose logs app`；等 MySQL healthy 后自动重试 |
| 页面 502 | 应用没起来 | `docker compose logs --tail 100 app` |
| `/blog/` 404 | 构建时用了 `BUILD_TARGET=app` | 用 `app-with-blog` 重新构建 |
| 构建卡在拉镜像 | 访问不了 Docker Hub | `REGISTRY=docker.m.daocloud.io/library` |
| 构建卡在 go mod | 访问不了 proxy.golang.org | 已内置 `GOPROXY=goproxy.cn`；仍失败则本地构建后传镜像（方式 B） |
| 8080 被占用 | 端口冲突 | 改 `.env` 的 `APP_PORT=8081` |
| 登录后频繁掉线 | 多次部署导致 JWT_SECRET 变化 | 固定 `.env` 里的 `JWT_SECRET` |
