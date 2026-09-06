#!/bin/bash

# 快速验证修复 - 只重启服务，不重新构建

set -e

echo "=========================================="
echo "快速验证文件上传修复"
echo "=========================================="

cd /d/context/context-keeper-main

# 步骤1: 检查服务是否运行
echo ""
echo "步骤1: 检查服务状态..."
if docker-compose ps | grep -q "Up"; then
    echo "✅ 服务正在运行"
else
    echo "⚠️ 服务未运行，正在启动..."
    docker-compose up -d
    sleep 10
fi

# 步骤2: 检查健康状态
echo ""
echo "步骤2: 检查健康状态..."
HEALTH_RESPONSE=$(curl -s http://localhost:8088/health)
echo "健康检查响应: ${HEALTH_RESPONSE}"

if echo "${HEALTH_RESPONSE}" | grep -q "ok"; then
    echo "✅ 服务健康"
else
    echo "❌ 服务不健康"
    exit 1
fi

# 步骤3: 运行文件上传测试
echo ""
echo "步骤3: 运行文件上传测试..."
bash /d/context/context-keeper-main/test_file_upload.sh

echo ""
echo "=========================================="
echo "验证完成"
echo "=========================================="
echo ""
echo "如果测试失败，请运行以下命令重新构建："
echo "  bash /d/context/context-keeper-main/rebuild_and_test.sh"
