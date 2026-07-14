# syntax=docker/dockerfile:1
# ============ 构建阶段 ============
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 步骤 1: 只复制依赖文件，利用 Docker 层缓存（改动极少）
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# 步骤 2: 预热构建缓存，加速后续编译
# 这一步会提前编译所有依赖，利用 Docker 的持久化缓存
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build std

# 步骤 3: 复制源码（改动频繁，但之前的缓存已预热）
COPY . .

# 步骤 4: 编译应用（完全相同的编译命令，利用预热后的缓存）
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# ============ Chromium 基础镜像层（单独层，不变就不重装）============
FROM alpine:3.20 AS chromium-base

RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    && rm -rf /var/cache/apk/*

ENV CHROMIUM_PATH=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/bin/chromium-browser

# ============ 运行阶段 ============
FROM chromium-base

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /app/server .

# 创建数据目录
RUN mkdir -p /app/data /app/logs

EXPOSE 5000

CMD ["./server"]
