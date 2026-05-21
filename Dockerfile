# ============ 构建阶段 ============
# syntax 声明启用 BuildKit 缓存挂载
# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# 先复制依赖文件，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制源码并用缓存编译
COPY . .
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

EXPOSE 8000

CMD ["./server"]
