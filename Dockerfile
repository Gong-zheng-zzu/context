# Context-Keeper 云端部署 Dockerfile
# 支持 HTTP 和 STDIO 模式，优化用于生产环境
# 完全模拟 ./scripts/manage.sh deploy http 的部署流程
# 🔥 新增：解决云端日志查看问题，将业务日志重定向到标准输出

# ================================
# 第一阶段：构建阶段
# ================================
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    bash \
    file \
    gcc \
    g++ \
    musl-dev \
    tesseract-ocr-dev \
    leptonica-dev \
    && update-ca-certificates

# 设置 Go 环境变量（模拟build.sh的设置）
ENV GO111MODULE=on \
    CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=https://goproxy.cn,direct \
    GOTOOLCHAIN=auto

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download && go mod verify

# 复制必要的源代码和构建脚本
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY scripts/ ./scripts/
COPY web/ ./web/

# 构建信息（模拟build.sh的构建信息设置）
ARG VERSION=docker
ARG BUILD_TIME
ARG COMMIT_HASH

# 🔥 关键修复：使用build tag编译HTTP模式
RUN go mod tidy && \
    go mod download && \
    # 设置构建信息
    export VERSION=${VERSION:-docker} && \
    export BUILD_TIME=${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")} && \
    export COMMIT_HASH=${COMMIT_HASH:-docker-build} && \
    # 使用http build tag编译HTTP模式
    mkdir -p ./bin && \
    go build -tags http \
        -ldflags="-s -w -X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X main.CommitHash=$COMMIT_HASH" \
        -o ./bin/context-keeper \
        ./cmd/server/

# 验证构建产物
RUN ls -la ./bin/ && \
    file ./bin/context-keeper

# ================================
# 第二阶段：运行时镜像
# ================================
FROM alpine:3.19

# 🔥 新增：安装日志处理工具和运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    jq \
    bash \
    procps \
    coreutils \
    util-linux \
    tesseract-ocr \
    leptonica \
    libstdc++ \
    && rm -rf /var/cache/apk/*

# 创建非特权用户
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件（只有stdio模式）
COPY --from=builder /app/bin/context-keeper /app/bin/context-keeper

# 从构建阶段复制脚本
COPY --from=builder /app/scripts/manage.sh /app/scripts/manage.sh
COPY --from=builder /app/scripts/docker-entrypoint.sh /app/docker-entrypoint.sh
COPY --from=builder /app/scripts/docker-log-monitor.sh /app/log-monitor.sh
COPY --from=builder /app/scripts/build/ /app/scripts/build/

# 从构建阶段复制前端文件
COPY --from=builder /app/web/ /app/web/

# 🔥 修改：创建必要的目录，包括统一的日志目录
RUN mkdir -p /app/data /app/data/logs /app/config /app/logs && \
    # 设置所有权
    chown -R appuser:appgroup /app

# 设置执行权限（模拟manage.sh的权限设置）
RUN chmod +x /app/bin/context-keeper \
    /app/scripts/manage.sh /app/docker-entrypoint.sh \
    /app/log-monitor.sh

# 🔥 修改：设置环境变量，包括日志路径重定向
ENV RUN_MODE=http \
    HTTP_SERVER_PORT=8088 \
    STORAGE_PATH=/app/data \
    LOG_LEVEL=info \
    TZ=Asia/Shanghai \
    # 模拟manage.sh的项目环境
    PROJECT_ROOT=/app \
    PID_DIR=/app/logs \
    # 🔥 新增：重定向日志到统一目录
    CONTEXT_KEEPER_LOG_DIR=/app/data/logs \
    CONTEXT_KEEPER_LOG_TO_STDOUT=true

# 健康检查（模拟manage.sh status检查）
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD if [ "$RUN_MODE" = "http" ]; then \
            curl -f http://localhost:${HTTP_SERVER_PORT}/health || exit 1; \
        else \
            pgrep -f context-keeper > /dev/null || exit 1; \
        fi

# 暴露端口
EXPOSE 8088

# 切换到非特权用户
USER appuser

# 🔥 关键修复：设置入口点支持完整的manage.sh功能
ENTRYPOINT ["/app/docker-entrypoint.sh"]

# 默认命令：模拟 ./scripts/manage.sh deploy http
CMD ["http"] 