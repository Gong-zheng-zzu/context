#!/bin/bash

# 文件上传测试脚本

API_BASE_URL="http://localhost:8088"
USER_ID="caregiver_wang"
PASSWORD="health_assistant_2024"
WORKSPACE_ID="default"

echo "=========================================="
echo "文件上传功能测试"
echo "=========================================="

# 步骤1: 登录获取JWT Token
echo ""
echo "步骤1: 登录获取JWT Token..."
LOGIN_RESPONSE=$(curl -s -X POST "${API_BASE_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"${USER_ID}\",
    \"password\": \"${PASSWORD}\",
    \"workspace_id\": \"${WORKSPACE_ID}\"
  }")

echo "登录响应: ${LOGIN_RESPONSE}"

JWT_TOKEN=$(echo "${LOGIN_RESPONSE}" | jq -r '.data.token')

if [ -z "${JWT_TOKEN}" ] || [ "${JWT_TOKEN}" == "null" ]; then
    echo "❌ 登录失败，无法获取JWT Token"
    exit 1
fi

echo "✅ 登录成功，JWT Token: ${JWT_TOKEN:0:20}..."

# 步骤2: 创建测试文件
echo ""
echo "步骤2: 创建测试文件..."
TEST_FILE="/tmp/test_upload.txt"
echo "这是一个测试文件，用于测试文件上传功能。" > "${TEST_FILE}"
echo "文件内容：测试数据" >> "${TEST_FILE}"
echo "上传时间：$(date)" >> "${TEST_FILE}"

echo "✅ 测试文件创建成功: ${TEST_FILE}"
ls -lh "${TEST_FILE}"

# 步骤3: 上传文件
echo ""
echo "步骤3: 上传文件..."
UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE_URL}/api/files/upload" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -F "file=@${TEST_FILE}" \
  -F "user_id=${USER_ID}")

echo "上传响应: ${UPLOAD_RESPONSE}"

# 检查上传是否成功
SUCCESS=$(echo "${UPLOAD_RESPONSE}" | jq -r '.success')
if [ "${SUCCESS}" == "true" ]; then
    FILE_ID=$(echo "${UPLOAD_RESPONSE}" | jq -r '.data.file_id')
    FILE_NAME=$(echo "${UPLOAD_RESPONSE}" | jq -r '.data.file_name')
    FILE_SIZE=$(echo "${UPLOAD_RESPONSE}" | jq -r '.data.file_size')

    echo "✅ 文件上传成功！"
    echo "   文件ID: ${FILE_ID}"
    echo "   文件名: ${FILE_NAME}"
    echo "   文件大小: ${FILE_SIZE} bytes"
else
    ERROR=$(echo "${UPLOAD_RESPONSE}" | jq -r '.error')
    echo "❌ 文件上传失败: ${ERROR}"
    exit 1
fi

# 步骤4: 测试图片上传
echo ""
echo "步骤4: 测试图片上传..."

# 创建一个简单的1x1像素PNG图片（base64编码）
TEST_IMAGE="/tmp/test_image.png"
echo "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" | base64 -d > "${TEST_IMAGE}"

echo "✅ 测试图片创建成功: ${TEST_IMAGE}"
ls -lh "${TEST_IMAGE}"

IMAGE_UPLOAD_RESPONSE=$(curl -s -X POST "${API_BASE_URL}/api/files/upload" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -F "file=@${TEST_IMAGE}" \
  -F "user_id=${USER_ID}")

echo "图片上传响应: ${IMAGE_UPLOAD_RESPONSE}"

IMAGE_SUCCESS=$(echo "${IMAGE_UPLOAD_RESPONSE}" | jq -r '.success')
if [ "${IMAGE_SUCCESS}" == "true" ]; then
    IMAGE_FILE_ID=$(echo "${IMAGE_UPLOAD_RESPONSE}" | jq -r '.data.file_id')
    echo "✅ 图片上传成功！文件ID: ${IMAGE_FILE_ID}"
else
    IMAGE_ERROR=$(echo "${IMAGE_UPLOAD_RESPONSE}" | jq -r '.error')
    echo "❌ 图片上传失败: ${IMAGE_ERROR}"
fi

# 清理测试文件
rm -f "${TEST_FILE}" "${TEST_IMAGE}"

echo ""
echo "=========================================="
echo "测试完成"
echo "=========================================="
