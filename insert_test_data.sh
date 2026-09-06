#!/bin/bash

# 插入测试血压数据到后端API

API_URL="http://localhost:8088"
TOKEN=""

# 先登录获取token（使用医生账号）
echo "正在登录..."
LOGIN_RESPONSE=$(curl -s -X POST "${API_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"doctor_li","password":"health_assistant_2024"}')

TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo "❌ 登录失败"
    echo "响应: $LOGIN_RESPONSE"
    exit 1
fi

echo "✅ 登录成功，Token: ${TOKEN:0:20}..."

# 插入张奶奶的血压数据（最近7天）
echo ""
echo "正在插入张奶奶的血压数据..."

for i in {0..6}; do
    DATE=$(date -d "$i days ago" +%Y-%m-%dT%H:%M:%S)
    SYSTOLIC=$((135 + RANDOM % 15))  # 135-150
    DIASTOLIC=$((82 + RANDOM % 10))  # 82-92

    echo "插入数据: $DATE - $SYSTOLIC/$DIASTOLIC mmHg"

    curl -s -X POST "${API_URL}/api/vital-signs/record" \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "{
        \"resident_id\": \"张奶奶\",
        \"type\": \"blood_pressure\",
        \"value\": $SYSTOLIC,
        \"unit\": \"mmHg\",
        \"measured_at\": \"$DATE\",
        \"metadata\": {
          \"systolic\": $SYSTOLIC,
          \"diastolic\": $DIASTOLIC,
          \"location\": \"养老院\"
        }
      }" > /dev/null

    if [ $? -eq 0 ]; then
        echo "  ✅ 成功"
    else
        echo "  ❌ 失败"
    fi
done

echo ""
echo "✅ 测试数据插入完成！"
echo ""
echo "现在可以在医生端输入：请帮张奶奶生成血压折线图"
