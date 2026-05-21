#!/bin/bash
# deploy.sh — 一键拉取最新代码 + Docker 重新部署
# 使用: chmod +x deploy.sh && ./deploy.sh

set -e

echo "=============================="
echo " 🦝 Fetchoo Deploy"
echo "=============================="

# 1. 进入项目目录（脚本所在目录）
cd "$(dirname "$0")"

# 2. 拉取最新代码
echo ""
echo "[1/4] 拉取最新代码..."
git pull --ff-only
echo "  ✓ 代码已更新"

# 3. 重新构建并启动
echo ""
echo "[2/4] 重新构建 Docker 镜像..."
DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 docker compose build
echo "  ✓ 构建完成"

echo ""
echo "[3/4] 重启服务..."
docker compose down
docker compose up -d
echo "  ✓ 服务已启动"

# 4. 健康检查
echo ""
echo "[4/4] 健康检查..."
sleep 3
if curl -sf http://localhost:5000/ > /dev/null 2>&1; then
    echo "  ✓ Fetchoo 已就绪: http://localhost:5000"
else
    echo "  ⚠ 服务可能尚未完全启动，请稍后检查"
fi

echo ""
echo "=============================="
echo " ✅ 部署完成"
echo "=============================="
