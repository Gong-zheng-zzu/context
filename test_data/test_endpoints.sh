#!/bin/bash

# Context-Keeper 端点测试脚本

BASE_URL="http://localhost:8088"
echo "=========================================="
echo "Context-Keeper 端点测试"
echo "=========================================="
echo ""

# 1. 健康检查
echo "1. 测试健康检查端点..."
curl -s "$BASE_URL/health" | jq '.' || echo "健康检查失败"
echo ""

# 2. 测试登录（护工）
echo "2. 测试护工登录..."
LOGIN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/role/login" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "caregiver_test",
    "password": "health_assistant_2024",
    "workspace_id": "default"
  }')

echo "$LOGIN_RESPONSE" | jq '.'
JWT_TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.data.token // empty')

if [ -z "$JWT_TOKEN" ]; then
    echo "❌ 登录失败，无法获取JWT Token"
    exit 1
fi

echo "✅ 登录成功，JWT Token: ${JWT_TOKEN:0:20}..."
echo ""

# 3. 测试聊天端点（需要JWT）
echo "3. 测试聊天端点..."
curl -s -X POST "$BASE_URL/api/chat" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{
    "user_id": "caregiver_test",
    "message": "你好，请介绍一下你自己",
    "session_id": "test_session_001"
  }' | jq '.'
echo ""

# 4. 测试安全扫描端点
echo "4. 测试安全扫描端点..."
curl -s -X POST "$BASE_URL/api/security/scan" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -d '{
    "content": "我的身份证号是110101199001011234，手机号是13800138000",
    "user_id": "caregiver_test"
  }' | jq '.'
echo ""

# 5. 测试速率限制（快速发送多个请求）
echo "5. 测试速率限制（发送10个快速请求）..."
for i in {1..10}; do
    RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST "$BASE_URL/api/chat" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $JWT_TOKEN" \
      -d "{\"user_id\":\"caregiver_test\",\"message\":\"测试消息$i\",\"session_id\":\"test_session_001\"}")

    HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)

    if [ "$HTTP_CODE" = "429" ]; then
        echo "  请求 $i: ⚠️ 速率限制触发 (429)"
    elif [ "$HTTP_CODE" = "200" ]; then
        echo "  请求 $i: ✅ 成功 (200)"
    else
        echo "  请求 $i: ❌ 失败 ($HTTP_CODE)"
    fi
done
echo ""

# 6. 测试其他角色登录
echo "6. 测试其他角色登录..."
for role in "doctor" "family" "elder"; do
    echo "  测试 ${role} 登录..."
    ROLE_RESPONSE=$(curl -s -X POST "$BASE_URL/api/role/login" \
      -H "Content-Type: application/json" \
      -d "{
        \"user_id\": \"${role}_test\",
        \"password\": \"health_assistant_2024\",
        \"workspace_id\": \"default\"
      }")

    ROLE_TOKEN=$(echo "$ROLE_RESPONSE" | jq -r '.data.token // empty')

    if [ -n "$ROLE_TOKEN" ]; then
        echo "    ✅ ${role} 登录成功"
    else
        echo "    ❌ ${role} 登录失败"
    fi
done
echo ""

echo "=========================================="
echo "测试完成"
echo "=========================================="
