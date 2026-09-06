#!/bin/bash

# 健康助手Docker快速启动脚本

echo "=========================================="
echo "健康助手 - Docker 启动脚本"
echo "=========================================="

# 检查.env文件
if [ ! -f "config/.env" ]; then
    echo "⚠️  未找到 config/.env 文件"
    echo "正在创建默认配置..."

    mkdir -p config
    cat > config/.env << 'EOF'
# LLM配置
LLM_DRIVEN_ENABLED=true
LLM_PROVIDER=ollama_local
LLM_MODEL=qwen2.5:3b
# DEEPSEEK_API_KEY=your_api_key_here

# 如果使用DeepSeek云端API，取消注释下面的配置
# LLM_PROVIDER=deepseek
# LLM_MODEL=deepseek-chat
# DEEPSEEK_API_KEY=your_api_key_here

# 服务器配置
RUN_MODE=http
HTTP_SERVER_PORT=8088
HOST_PORT=8088

# 日志配置
LOG_LEVEL=info
CONTEXT_KEEPER_LOG_TO_STDOUT=true

# 时区
TZ=Asia/Shanghai
EOF

    echo "✅ 已创建 config/.env 文件"
    echo "⚠️  请编辑 config/.env 文件，设置你的 DEEPSEEK_API_KEY"
    echo ""
    read -p "按回车键继续..."
fi

# 检查Docker是否运行
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker未运行，请先启动Docker"
    exit 1
fi

echo ""
echo "🚀 启动健康助手服务..."
echo ""

# 构建并启动
docker-compose up -d --build

# 等待Ollama服务启动
echo ""
echo "⏳ 等待Ollama服务启动..."
sleep 10

# 拉取Ollama模型
echo ""
echo "📥 拉取Ollama模型 (qwen2.5:3b)..."
docker exec context-keeper-ollama ollama pull qwen2.5:3b

# 等待服务启动
echo ""
echo "⏳ 等待健康助手服务启动..."
sleep 5

# 检查服务状态
if docker-compose ps | grep -q "Up"; then
    echo ""
    echo "=========================================="
    echo "✅ 健康助手服务已启动！"
    echo "=========================================="
    echo ""
    echo "📝 访问地址："
    echo "   前端页面: file://$(pwd)/web/user_health_assistant.html"
    echo "   或者: http://localhost:8088/user_health_assistant.html"
    echo ""
    echo "🔧 API端点："
    echo "   健康检查: http://localhost:8088/health"
    echo "   聊天API: http://localhost:8088/api/chat"
    echo ""
    echo "📊 查看日志："
    echo "   docker-compose logs -f context-keeper"
    echo ""
    echo "🛑 停止服务："
    echo "   docker-compose down"
    echo ""
else
    echo ""
    echo "❌ 服务启动失败，请查看日志："
    echo "   docker-compose logs context-keeper"
    echo ""
fi
