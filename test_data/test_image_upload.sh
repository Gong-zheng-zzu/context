#!/bin/bash

echo "=== Context-Keeper 图片上传诊断脚本 ==="
echo ""

# 1. 检查Ollama服务
echo "1. 检查Ollama服务状态..."
curl -s http://localhost:11434/api/version && echo "✅ Ollama服务正常" || echo "❌ Ollama服务异常"
echo ""

# 2. 检查可用模型
echo "2. 检查视觉模型..."
curl -s http://localhost:11434/api/tags | grep -q "llava" && echo "✅ llava模型已安装" || echo "❌ llava模型未安装"
echo ""

# 3. 检查后端服务
echo "3. 检查后端服务..."
curl -s http://localhost:8088/health && echo "✅ 后端服务正常" || echo "❌ 后端服务异常"
echo ""

# 4. 测试登录
echo "4. 测试登录..."
TOKEN=$(curl -s -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test_user","password":"health_assistant_2024","workspace_id":"default","role":"caregiver"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TOKEN" ]; then
    echo "✅ 登录成功，获取到Token"
    echo "Token前20字符: ${TOKEN:0:20}..."
else
    echo "❌ 登录失败"
    exit 1
fi
echo ""

# 5. 创建测试图片
echo "5. 创建测试图片..."
mkdir -p ./test_data
# 创建一个简单的文本图片（实际应该是真实图片）
echo "这是一个测试图片" > ./test_data/test.txt
echo "✅ 测试文件已创建"
echo ""

# 6. 测试文件上传
echo "6. 测试文件上传..."
UPLOAD_RESULT=$(curl -s -X POST http://localhost:8088/api/files/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@./test_data/test.txt" \
  -F "user_id=test_user")

echo "上传响应: $UPLOAD_RESULT"
echo ""

# 7. 提取文件ID
FILE_ID=$(echo $UPLOAD_RESULT | grep -o '"file_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$FILE_ID" ]; then
    echo "✅ 文件上传成功，文件ID: $FILE_ID"

    # 8. 测试聊天（带文件）
    echo ""
    echo "7. 测试聊天（带文件）..."
    CHAT_RESULT=$(curl -s -X POST http://localhost:8088/api/chat \
      -H "Content-Type: application/json" \
      -H "Authorization: Bearer $TOKEN" \
      -d "{\"user_id\":\"test_user\",\"message\":\"你能看到内容吗\",\"session_id\":\"test_session\",\"files\":[{\"file_id\":\"$FILE_ID\"}]}")

    echo "聊天响应: $CHAT_RESULT"
else
    echo "❌ 文件上传失败"
fi

echo ""
echo "=== 诊断完成 ==="
