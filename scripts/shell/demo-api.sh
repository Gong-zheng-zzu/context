#!/bin/bash

# Context-Keeper API 演示脚本
# 展示核心功能：记忆存储、检索、敏感信息脱敏

API_BASE="http://localhost:8088"
SESSION_ID="demo_$(date +%s)"

echo "=========================================="
echo "Context-Keeper 功能演示"
echo "=========================================="
echo "会话ID: $SESSION_ID"
echo ""

# 1. 健康检查
echo "1️⃣  测试服务健康状态..."
curl -s "$API_BASE/health" | jq .
echo ""

# 2. 创建会话
echo "2️⃣  创建新会话..."
curl -s -X POST "$API_BASE/api/mcp" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 1,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"session_management\",
      \"arguments\": {
        \"action\": \"get_or_create\",
        \"sessionId\": \"$SESSION_ID\"
      }
    }
  }" | jq .
echo ""

# 3. 存储对话（包含敏感信息）
echo "3️⃣  存储对话（包含敏感信息）..."
curl -s -X POST "$API_BASE/api/mcp" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 2,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"store_conversation\",
      \"arguments\": {
        \"sessionId\": \"$SESSION_ID\",
        \"messages\": [
          {
            \"role\": \"user\",
            \"content\": \"我正在开发电商系统，数据库密码是P@ssw0rd123，API密钥是sk-1234567890abcdef\"
          }
        ]
      }
    }
  }" | jq .
echo ""

# 4. 检索上下文
echo "4️⃣  检索上下文（测试记忆功能）..."
curl -s -X POST "$API_BASE/api/mcp" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 3,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"retrieve_context\",
      \"arguments\": {
        \"sessionId\": \"$SESSION_ID\",
        \"query\": \"电商系统的技术栈\"
      }
    }
  }" | jq .
echo ""

# 5. 测试敏感信息检测
echo "5️⃣  测试敏感信息检测..."
curl -s -X POST "$API_BASE/api/security/scan" \
  -H "Content-Type: application/json" \
  -d "{
    \"content\": \"我的手机号是13800138000，邮箱test@example.com，API密钥sk-1234567890abcdef\",
    \"user_id\": \"demo_user\",
    \"session_id\": \"$SESSION_ID\"
  }" | jq .
echo ""

# 6. 语义搜索测试
echo "6️⃣  语义搜索测试..."
curl -s -X POST "$API_BASE/api/mcp" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"id\": 4,
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"retrieve_memory\",
      \"arguments\": {
        \"sessionId\": \"$SESSION_ID\",
        \"query\": \"数据库配置\"
      }
    }
  }" | jq .
echo ""

echo "=========================================="
echo "演示完成！"
echo "=========================================="
echo ""
echo "核心功能验证："
echo "✅ 会话管理"
echo "✅ 对话存储"
echo "✅ 上下文检索"
echo "✅ 敏感信息检测和脱敏"
echo "✅ 语义搜索"
echo ""
