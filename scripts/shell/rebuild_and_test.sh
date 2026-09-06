#!/bin/bash

# 重新构建并测试文件上传功能

set -e

echo "=========================================="
echo "重新构建 Context-Keeper"
echo "=========================================="

cd /d/context/context-keeper-main

# 步骤1: 停止现有容器
echo ""
echo "步骤1: 停止现有容器..."
docker-compose down || true

# 步骤2: 重新构建镜像
echo ""
echo "步骤2: 重新构建Docker镜像..."
docker-compose build --no-cache

# 步骤3: 启动服务
echo ""
echo "步骤3: 启动服务..."
docker-compose up -d

# 步骤4: 等待服务启动
echo ""
echo "步骤4: 等待服务启动..."
sleep 10

# 步骤5: 检查服务状态
echo ""
echo "步骤5: 检查服务状态..."
docker-compose ps

# 步骤6: 查看日志
echo ""
echo "步骤6: 查看最近的日志..."
docker-compose logs --tail=50

# 步骤7: 运行文件上传测试
echo ""
echo "步骤7: 运行文件上传测试..."
bash /d/context/context-keeper-main/test_file_upload.sh

echo ""
echo "=========================================="
echo "构建和测试完成"
echo "=========================================="
