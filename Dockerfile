#
# xiaohugochat —— 仿微信 IM 后端（Go）+ 仿微信前端 + 学习博客
#
# 构建：
#   docker build -t xiaohugochat:latest .
#   国内网络可用镜像源加速：
#   docker build --build-arg REGISTRY=docker.m.daocloud.io/library -t xiaohugochat:latest .
#
# 只构建运行时（不含博客，构建更快）：
#   docker build --target app -t xiaohugochat:latest .
#
# 镜像源参数：默认 Docker Hub，国内可换成 docker.m.daocloud.io/library
ARG REGISTRY=docker.io/library

# ============================================================
# 阶段 1：构建学习博客（Hexo → 静态 HTML）
# ============================================================
FROM ${REGISTRY}/node:20-alpine AS blog

WORKDIR /blog
# 国内 npm 加速 + 把 lockfile 里 registry.npmjs.org 的地址重写到镜像源
ENV npm_config_registry=https://registry.npmmirror.com \
    npm_config_replace_registry_host=always

# 先只拷依赖清单，利用 Docker 层缓存：package.json 不变就不会重新装依赖
COPY blog/package.json blog/package-lock.json ./
# --ignore-scripts：此时 tools/ 还没拷进来，跳过 postinstall（后面手动执行 copy-theme）
RUN npm install --ignore-scripts --no-audit --no-fund

COPY blog/ ./
# 把 node_modules 里的主题复制到 themes/，再生成静态站点
RUN node tools/copy-theme.js && npx hexo generate

# ============================================================
# 阶段 2：编译 Go 二进制
# ============================================================
FROM ${REGISTRY}/golang:1.26-alpine AS build

WORKDIR /src
# CGO_ENABLED=0：编译静态二进制，运行时不需要 glibc（可以用 alpine）
# GOPROXY：中国大陆访问 proxy.golang.org 不通，改用 goproxy.cn
# GOTOOLCHAIN=local：禁止自动下载更高版本 toolchain（镜像内 Go 版本即最终版本）
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off \
    GOTOOLCHAIN=local

COPY . .
# 有 vendor 目录就离线构建（-mod=vendor，服务器无外网也能构建），否则走模块下载
RUN if [ -d vendor ]; then \
        echo "==> 使用 vendor 目录离线构建" && \
        go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/xiaohugochat . ; \
    else \
        echo "==> 使用模块下载构建" && \
        go build -mod=mod -trimpath -ldflags="-s -w" -o /out/xiaohugochat . ; \
    fi

# ============================================================
# 阶段 3：运行时（不含博客）
# ============================================================
FROM ${REGISTRY}/alpine:3.20 AS app

# ca-certificates：HTTPS 需要；tzdata：容器内时区（DSN 用了 loc=Local）
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app

WORKDIR /app
ENV TZ=Asia/Shanghai

COPY --from=build /out/xiaohugochat /app/xiaohugochat
COPY web/ /app/web/
COPY config.yaml /app/config.yaml
# 先建空目录：即使没有博客，静态路由也能正常返回 404 而不是启动报错
RUN mkdir -p /app/blog/public && chown -R app:app /app

# 非 root 运行（安全最佳实践）
USER app

EXPOSE 8080 9001 9002

# 容器健康检查（alpine 自带 busybox wget）
HEALTHCHECK --interval=15s --timeout=3s --start-period=25s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/xiaohugochat"]
# -db：启动时执行 AutoMigrate 建表（演示部署方便；生产可用专门的迁移任务）
CMD ["-db"]

# ============================================================
# 阶段 4：运行时 + 学习博客（默认目标）
# ============================================================
FROM app AS app-with-blog

COPY --from=blog /blog/public /app/blog/public
