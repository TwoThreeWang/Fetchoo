# ============ 构建阶段 ============
FROM golang:1.23-alpine AS builder

# 安装构建依赖（chromedp 需要一些头文件）
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download 2>/dev/null || true
RUN go mod tidy && go mod download

# 复制源码并编译（静态链接，无 CGO）
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# ============ 运行阶段 ============
FROM alpine:3.20

# 安装 Chromium 及其最小运行时依赖
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    fonts-noto-cjk \
    ttf-liberation \
    && rm -rf /var/cache/apk/*

# 设置 Chromium 路径（chromedp 默认搜索路径）
ENV CHROMIUM_PATH=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/bin/chromium-browser

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /app/server .

# 创建数据目录
RUN mkdir -p /app/data /app/logs

EXPOSE 8000

CMD ["./server"]
