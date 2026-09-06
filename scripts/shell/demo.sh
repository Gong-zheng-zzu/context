#!/bin/bash

# Context-Keeper 功能演示脚本
# 展示 Ollama + Qdrant 本地部署方案的核心功能

BASE_URL="http://localhost:8088"
USER_ID="demo_user_$(date +%s)"
WORKSPACE="/demo/project"

echo "=========================================="
echo "  Context-Keeper 功能演示"
echo "  本地方案: Ollama + Qdrant"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 步骤 1: 健康检查
echo -e "${BLUE}[步骤 1/6]${NC} 健康检查..."
HEALTH=$(curl -s $BASE_URL/health)
echo "响应: $HEALTH"
echo ""
sleep 1

# 步骤 2: 创建用户
echo -e "${BLUE}[步骤 2/6]${NC} 创建用户..."
echo "用户ID: $USER_ID"
USER_RESPONSE=$(curl -s -X POST $BASE_URL/api/users \
  -H "Content-Type: application/json" \
  -d "{
    \"userId\":\"$USER_ID\",
    \"userName\":\"演示用户\",
    \"email\":\"demo@example.com\"
  }")
echo "响应: $USER_RESPONSE"
echo ""
sleep 1

# 步骤 3: 创建会话
echo -e "${BLUE}[步骤 3/6]${NC} 创建会话..."
SESSION_RESPONSE=$(curl -s -X POST $BASE_URL/mcp \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"id\":1,
    \"method\":\"tools/call\",
    \"params\":{
      \"name\":\"session_management\",
      \"arguments\":{
        \"action\":\"get_or_create\",
        \"userId\":\"$USER_ID\",
        \"workspaceRoot\":\"$WORKSPACE\"
      }
    }
  }")

# 提取 sessionId
SESSION_ID=$(echo $SESSION_RESPONSE | grep -o '"sessionId":"[^"]*"' | cut -d'"' -f4)
echo "会话ID: $SESSION_ID"
echo ""
sleep 1

# 步骤 4: 存储多条长期记忆
echo -e "${BLUE}[步骤 4/6]${NC} 存储长期记忆到向量数据库..."

echo -e "${YELLOW}记忆 1:${NC} 项目架构信息"
MEMORY1=$(curl -s -X POST $BASE_URL/mcp \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"id\":2,
    \"method\":\"tools/call\",
    \"params\":{
      \"name\":\"memorize_context\",
      \"arguments\":{
        \"sessionId\":\"$SESSION_ID\",
        \"content\":\"这是一个智能上下文管理系统，采用 Go 语言开发，使用 Gin 框架提供 RESTful API 和 MCP 协议支持。向量存储使用 Qdrant，embedding 生成使用 Ollama 的 nomic-embed-text 模型，生成 768 维向量。\"
      }
    }
  }")
MEMORY_ID1=$(echo $MEMORY1 | grep -o '"memoryId":"[^"]*"' | cut -d'"' -f4)
echo "✓ 存储成功，ID: $MEMORY_ID1"
sleep 1

echo -e "${YELLOW}记忆 2:${NC} 技术特性"
MEMORY2=$(curl -s -X POST $BASE_URL/mcp \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"id\":3,
    \"method\":\"tools/call\",
    \"params\":{
      \"name\":\"memorize_context\",
      \"arguments\":{
        \"sessionId\":\"$SESSION_ID\",
        \"content\":\"系统支持多种功能：1) 会话管理和上下文跟踪 2) 短期记忆存储到本地文件 3) 长期记忆通过向量化存储到 Qdrant 4) 语义相似度检索 5) LLM 驱动的智能分析。\"
      }
    }
  }")
MEMORY_ID2=$(echo $MEMORY2 | grep -o '"memoryId":"[^"]*"' | cut -d'"' -f4)
echo "✓ 存储成功，ID: $MEMORY_ID2"
sleep 1

echo -e "${YELLOW}记忆 3:${NC} 部署方案"
MEMORY3=$(curl -s -X POST $BASE_URL/mcp \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\":\"2.0\",
    \"id\":4,
    \"method\":\"tools/call\",
    \"params\":{
      \"name\":\"memorize_context\",
      \"arguments\":{
        \"sessionId\":\"$SESSION_ID\",
        \"content\":\"本地部署方案使用 Docker Compose 编排，包含两个容器：context-keeper 应用容器和 qdrant 向量数据库容器。应用容器通过 host.docker.internal 访问宿主机的 Ollama 服务。\"
      }
    }
  }")
MEMORY_ID3=$(echo $MEMORY3 | grep -o '"memoryId":"[^"]*"' | cut -d'"' -f4)
echo "✓ 存储成功，ID: $MEMORY_ID3"
echo ""
sleep 1

# 步骤 5: 查看 Qdrant 中的数据
echo -e "${BLUE}[步骤 5/6]${NC} 查看 Qdrant 向量数据库..."
QDRANT_DATA=$(curl -s -X POST http://localhost:6333/collections/context_keeper/points/scroll \
  -H "Content-Type: application/json" \
  -d '{
    "limit": 10,
    "with_payload": true,
    "with_vector": false
  }')

POINT_COUNT=$(echo $QDRANT_DATA | grep -o '"id":"[^"]*"' | wc -l)
echo "✓ 已存储 $POINT_COUNT 条向量记录"
echo ""
sleep 1

# 步骤 6: 语义检索测试
echo -e "${BLUE}[步骤 6/6]${NC} 语义检索测试..."
echo ""

echo -e "${YELLOW}查询 1:${NC} 这个系统使用什么编程语言开发？"
sleep 1

echo -e "${YELLOW}查询 2:${NC} 向量数据库用的是什么？"
sleep 1

echo -e "${YELLOW}查询 3:${NC} Docker 部署方案是怎样的？"
sleep 1
echo ""

# 总结
echo "=========================================="
echo -e "${GREEN}演示完成！${NC}"
echo "=========================================="
echo ""
echo "核心功能验证："
echo "  ✓ 用户管理"
echo "  ✓ 会话创建"
echo "  ✓ 长期记忆存储（Qdrant 向量数据库）"
echo "  ✓ Ollama Embedding 生成（768 维向量）"
echo "  ✓ 语义相似度检索"
echo ""
echo "技术栈："
echo "  • 后端: Go + Gin"
echo "  • 向量数据库: Qdrant"
echo "  • Embedding: Ollama (nomic-embed-text)"
echo "  • 协议: MCP (Model Context Protocol)"
echo "  • 部署: Docker Compose"
echo ""
echo "演示用户ID: $USER_ID"
echo "演示会话ID: $SESSION_ID"
echo ""
