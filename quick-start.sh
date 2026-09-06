#!/bin/bash
# Context-Keeper 快速启动脚本
# 用途：一键启动所有服务并检查健康状态

set -e

echo "=========================================="
echo "   Context-Keeper 快速启动脚本"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查Docker是否安装
echo "🔍 检查Docker环境..."
if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker未安装，请先安装Docker Desktop${NC}"
    echo "下载地址: https://www.docker.com/products/docker-desktop/"
    exit 1
fi

if ! docker info &> /dev/null; then
    echo -e "${RED}❌ Docker服务未启动，请启动Docker Desktop${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Docker环境正常${NC}"
echo ""

# 检查config/.env文件
echo "🔍 检查配置文件..."
if [ ! -f config/.env ]; then
    if [ -f .env.example ]; then
        echo -e "${YELLOW}⚠️  未找到config/.env文件，正在从.env.example复制...${NC}"
        mkdir -p config
        cp .env.example config/.env
        echo -e "${GREEN}✅ 已创建config/.env文件${NC}"
        echo ""
        echo -e "${YELLOW}重要：请编辑 config/.env 文件，修改以下配置：${NC}"
        echo "  - JWT_SECRET (必须修改)"
        echo "  - NEO4J_PASSWORD"
        echo "  - TIMESCALEDB_PASSWORD"
        echo "  - VECTOR_DB_API_KEY (如使用阿里云)"
        echo "  - EMBEDDING_API_KEY (如使用阿里云)"
        echo ""
        read -p "配置完成后按回车继续..." -r
    else
        echo -e "${RED}❌ 未找到.env.example文件${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}✅ 配置文件存在${NC}"
echo ""

# 启动服务
echo "🚀 启动Docker容器..."
docker compose up -d

echo ""
echo "⏳ 等待服务启动（30秒）..."
sleep 30

# 检查服务状态
echo ""
echo "🔍 检查服务健康状态..."
echo ""

# 检查context-keeper
if docker compose ps | grep context-keeper | grep -q "Up"; then
    echo -e "${GREEN}✅ Context-Keeper 后端服务 [Running]${NC}"
else
    echo -e "${RED}❌ Context-Keeper 后端服务 [Failed]${NC}"
    echo "查看日志: docker compose logs context-keeper | tail -50"
fi

# 检查Neo4j
if docker compose ps | grep neo4j | grep -q "Up"; then
    echo -e "${GREEN}✅ Neo4j 图数据库 [Running]${NC}"
else
    echo -e "${RED}❌ Neo4j 图数据库 [Failed]${NC}"
fi

# 检查TimescaleDB
if docker compose ps | grep timescaledb | grep -q "Up"; then
    echo -e "${GREEN}✅ TimescaleDB 时序数据库 [Running]${NC}"
else
    echo -e "${RED}❌ TimescaleDB 时序数据库 [Failed]${NC}"
fi

# 检查Qdrant
if docker compose ps | grep qdrant | grep -q "Up"; then
    echo -e "${GREEN}✅ Qdrant 向量数据库 [Running]${NC}"
else
    echo -e "${YELLOW}⚠️  Qdrant 向量数据库 [Not Running]${NC}"
fi

# 检查Ollama
echo ""
echo "🔍 检查Ollama本地LLM..."
if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Ollama服务 [Running]${NC}"

    # 检查qwen2.5模型
    if curl -s http://localhost:11434/api/tags | grep -q "qwen2.5"; then
        echo -e "${GREEN}✅ Qwen2.5 模型已下载${NC}"
    else
        echo -e "${YELLOW}⚠️  Qwen2.5 模型未下载${NC}"
        echo "   运行以下命令下载: ollama pull qwen2.5:3b"
    fi
else
    echo -e "${YELLOW}⚠️  Ollama服务未运行（可选）${NC}"
    echo "   如需本地LLM功能，请安装Ollama: https://ollama.com"
fi

echo ""
echo "=========================================="
echo "   服务访问地址"
echo "=========================================="
echo ""
echo -e "🌐 Web界面:          ${GREEN}http://localhost:8088${NC}"
echo -e "🔐 默认账号:         doctor_wang"
echo -e "🔑 默认密码:         (查看config/.env中的DEMO_AUTH_PASSWORD)"
echo ""
echo -e "📊 Neo4j Browser:    ${GREEN}http://localhost:7474${NC}"
echo -e "🔍 Qdrant Dashboard: ${GREEN}http://localhost:6333/dashboard${NC}"
echo -e "🤖 Ollama API:       ${GREEN}http://localhost:11434${NC}"
echo ""
echo "=========================================="
echo ""

# 询问是否启动可视化服务
echo "🎨 是否启动可视化服务？(用于生成血压图表)"
read -p "启动可视化服务？(y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "🚀 启动可视化服务..."

    # 检查Python和依赖
    if ! command -v python3 &> /dev/null; then
        echo -e "${RED}❌ Python3未安装${NC}"
    else
        cd tools

        # 检查依赖
        if ! python3 -c "import flask" 2>/dev/null; then
            echo "📦 安装Python依赖..."
            pip3 install flask flask-cors matplotlib
        fi

        echo -e "${GREEN}✅ 启动可视化服务 (后台运行)${NC}"
        nohup python3 visualization_service.py > visualization.log 2>&1 &
        echo $! > visualization.pid

        sleep 2

        if curl -s http://localhost:5001/health > /dev/null 2>&1; then
            echo -e "${GREEN}✅ 可视化服务已启动 http://localhost:5001${NC}"
        else
            echo -e "${RED}❌ 可视化服务启动失败，查看 tools/visualization.log${NC}"
        fi

        cd ..
    fi
fi

echo ""
echo "=========================================="
echo "   快速测试"
echo "=========================================="
echo ""
echo "运行以下命令插入测试数据:"
echo "  cd tools && go run insert_test_data.go"
echo ""
echo "查看服务日志:"
echo "  docker compose logs -f context-keeper"
echo ""
echo "停止所有服务:"
echo "  docker compose down"
echo ""
echo -e "${GREEN}✅ 启动完成！请访问 http://localhost:8088${NC}"
echo ""
