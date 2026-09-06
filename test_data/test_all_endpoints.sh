#!/bin/bash

# ============================================
# Context-Keeper 全面自动化测试脚本
# ============================================
# 测试覆盖：
# 1. 所有4个角色的登录功能
# 2. JWT认证机制
# 3. 聊天API与LLM连接
# 4. 文件上传/下载功能
# 5. 安全检测功能
# 6. 速率限制机制
# 7. 无认证访问控制
# ============================================

set -o pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
BASE_URL="${BASE_URL:-http://localhost:8088}"
TEMP_DIR="/tmp/context-keeper-test-$$"
TEST_FILE="$TEMP_DIR/test_upload.txt"

# 测试统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
WARNINGS=0

# 创建临时目录
mkdir -p "$TEMP_DIR"

# 清理函数
cleanup() {
    rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# 打印函数
print_header() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
}

print_test() {
    echo -e "${BLUE}[测试 $TOTAL_TESTS] $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
    ((PASSED_TESTS++))
}

print_failure() {
    echo -e "${RED}❌ $1${NC}"
    ((FAILED_TESTS++))
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
    ((WARNINGS++))
}

print_info() {
    echo -e "   $1"
}

# 测试结果验证函数
check_http_status() {
    local expected=$1
    local actual=$2
    local test_name=$3

    ((TOTAL_TESTS++))
    if [ "$actual" = "$expected" ]; then
        print_success "$test_name (HTTP $actual)"
        return 0
    else
        print_failure "$test_name (期望: $expected, 实际: $actual)"
        return 1
    fi
}

check_json_field() {
    local json=$1
    local field=$2
    local test_name=$3

    ((TOTAL_TESTS++))
    local value=$(echo "$json" | jq -r "$field" 2>/dev/null)
    if [ -n "$value" ] && [ "$value" != "null" ]; then
        print_success "$test_name"
        echo "$value"
        return 0
    else
        print_failure "$test_name (字段 $field 不存在或为空)"
        return 1
    fi
}

# ============================================
# 测试1: 健康检查
# ============================================
test_health_check() {
    print_header "测试1: 健康检查"
    print_test "检查服务是否运行"

    RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/health" 2>/dev/null)
    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if check_http_status "200" "$HTTP_CODE" "健康检查端点"; then
        print_info "响应: $BODY"
    else
        print_failure "服务未运行或健康检查失败"
        print_info "请确保服务已启动: $BASE_URL"
        exit 1
    fi
}

# ============================================
# 测试2: 角色登录测试
# ============================================
declare -A ROLE_TOKENS
declare -A ROLE_NAMES=(
    ["caregiver"]="护工"
    ["doctor"]="医生"
    ["family"]="家属"
    ["elder"]="老人"
)

test_role_login() {
    print_header "测试2: 角色登录功能"

    for role in caregiver doctor family elder; do
        print_test "测试 ${ROLE_NAMES[$role]} ($role) 登录"

        LOGIN_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/role/login" \
            -H "Content-Type: application/json" \
            -d "{
                \"user_id\": \"${role}_test\",
                \"password\": \"health_assistant_2024\",
                \"role\": \"$role\",
                \"workspace_id\": \"default\"
            }" 2>/dev/null)

        HTTP_CODE=$(echo "$LOGIN_RESPONSE" | tail -n1)
        BODY=$(echo "$LOGIN_RESPONSE" | sed '$d')

        if check_http_status "200" "$HTTP_CODE" "${ROLE_NAMES[$role]}登录"; then
            TOKEN=$(echo "$BODY" | jq -r '.data.token // empty')
            if [ -n "$TOKEN" ]; then
                ROLE_TOKENS[$role]=$TOKEN
                print_success "获取到JWT Token: ${TOKEN:0:30}..."
                print_info "角色: $(echo "$BODY" | jq -r '.data.role_name')"
                print_info "用户ID: $(echo "$BODY" | jq -r '.data.user_id')"
                print_info "过期时间: $(echo "$BODY" | jq -r '.data.expires_in')秒"
            else
                print_failure "未能获取JWT Token"
                print_info "响应: $BODY"
            fi
        else
            print_info "响应: $BODY"
        fi
        echo ""
    done
}

# ============================================
# 测试3: 无认证访问测试
# ============================================
test_unauthorized_access() {
    print_header "测试3: 无认证访问控制"

    print_test "尝试不带Token访问聊天端点"
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/chat" \
        -H "Content-Type: application/json" \
        -d '{
            "user_id": "test_user",
            "message": "测试消息",
            "session_id": "test_session"
        }' 2>/dev/null)

    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    check_http_status "401" "$HTTP_CODE" "拒绝无认证访问"

    print_test "尝试使用无效Token访问"
    RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/chat" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer invalid_token_12345" \
        -d '{
            "user_id": "test_user",
            "message": "测试消息",
            "session_id": "test_session"
        }' 2>/dev/null)

    HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
    check_http_status "401" "$HTTP_CODE" "拒绝无效Token访问"
    echo ""
}

# ============================================
# 测试4: 聊天功能测试
# ============================================
test_chat_functionality() {
    print_header "测试4: 聊天功能与LLM连接"

    # 使用护工角色测试
    if [ -z "${ROLE_TOKENS[caregiver]}" ]; then
        print_warning "护工Token不存在，跳过聊天测试"
        return
    fi

    print_test "测试聊天API连接"
    CHAT_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/chat" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" \
        -d '{
            "user_id": "caregiver_test",
            "message": "你好，请简单介绍一下你自己",
            "session_id": "test_session_001"
        }' 2>/dev/null)

    HTTP_CODE=$(echo "$CHAT_RESPONSE" | tail -n1)
    BODY=$(echo "$CHAT_RESPONSE" | sed '$d')

    if check_http_status "200" "$HTTP_CODE" "聊天API响应"; then
        # 检查响应内容
        REPLY=$(echo "$BODY" | jq -r '.data.reply // .reply // empty' 2>/dev/null)
        if [ -n "$REPLY" ] && [ "$REPLY" != "null" ]; then
            print_success "LLM返回有效响应"
            print_info "响应长度: ${#REPLY} 字符"
            print_info "响应预览: ${REPLY:0:100}..."
        else
            print_warning "LLM响应为空或格式异常"
            print_info "完整响应: $BODY"
        fi
    else
        print_info "响应: $BODY"
    fi

    # 测试其他角色的聊天功能
    for role in doctor family elder; do
        if [ -n "${ROLE_TOKENS[$role]}" ]; then
            print_test "测试 ${ROLE_NAMES[$role]} 角色聊天"
            RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/chat" \
                -H "Content-Type: application/json" \
                -H "Authorization: Bearer ${ROLE_TOKENS[$role]}" \
                -d "{
                    \"user_id\": \"${role}_test\",
                    \"message\": \"测试消息\",
                    \"session_id\": \"test_session_${role}\"
                }" 2>/dev/null)

            HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
            check_http_status "200" "$HTTP_CODE" "${ROLE_NAMES[$role]}聊天功能"
        fi
    done
    echo ""
}

# ============================================
# 测试5: 文件上传功能
# ============================================
test_file_upload() {
    print_header "测试5: 文件上传功能"

    if [ -z "${ROLE_TOKENS[caregiver]}" ]; then
        print_warning "护工Token不存在，跳过文件上传测试"
        return
    fi

    # 创建测试文件
    print_test "创建测试文件"
    echo "这是一个测试文件，用于验证文件上传功能。" > "$TEST_FILE"
    echo "文件内容包含一些普通文本。" >> "$TEST_FILE"
    print_success "测试文件已创建: $TEST_FILE"

    print_test "上传文件到服务器"
    UPLOAD_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/files/upload" \
        -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" \
        -F "file=@$TEST_FILE" \
        -F "user_id=caregiver_test" 2>/dev/null)

    HTTP_CODE=$(echo "$UPLOAD_RESPONSE" | tail -n1)
    BODY=$(echo "$UPLOAD_RESPONSE" | sed '$d')

    if check_http_status "200" "$HTTP_CODE" "文件上传"; then
        FILE_ID=$(echo "$BODY" | jq -r '.data.file_id // empty')
        if [ -n "$FILE_ID" ] && [ "$FILE_ID" != "null" ]; then
            print_success "文件上传成功，文件ID: $FILE_ID"
            print_info "文件名: $(echo "$BODY" | jq -r '.data.file_name')"
            print_info "文件大小: $(echo "$BODY" | jq -r '.data.file_size') 字节"
            print_info "文件类型: $(echo "$BODY" | jq -r '.data.file_type')"

            # 测试文件下载
            print_test "下载已上传的文件"
            DOWNLOAD_RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/api/files/$FILE_ID" \
                -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" 2>/dev/null)

            DOWNLOAD_HTTP_CODE=$(echo "$DOWNLOAD_RESPONSE" | tail -n1)
            check_http_status "200" "$DOWNLOAD_HTTP_CODE" "文件下载"
        else
            print_failure "未能获取文件ID"
            print_info "响应: $BODY"
        fi
    else
        print_info "响应: $BODY"
    fi
    echo ""
}

# ============================================
# 测试6: 安全检测功能
# ============================================
test_security_detection() {
    print_header "测试6: 安全检测功能"

    if [ -z "${ROLE_TOKENS[caregiver]}" ]; then
        print_warning "护工Token不存在，跳过安全检测测试"
        return
    fi

    print_test "测试敏感信息检测（身份证号、手机号）"
    SECURITY_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/security/scan" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" \
        -d '{
            "content": "我的身份证号是110101199001011234，手机号是13800138000，邮箱是test@example.com",
            "user_id": "caregiver_test"
        }' 2>/dev/null)

    HTTP_CODE=$(echo "$SECURITY_RESPONSE" | tail -n1)
    BODY=$(echo "$SECURITY_RESPONSE" | sed '$d')

    if check_http_status "200" "$HTTP_CODE" "安全扫描API"; then
        SENSITIVE_COUNT=$(echo "$BODY" | jq -r '.sensitive_infos | length' 2>/dev/null)
        if [ "$SENSITIVE_COUNT" -gt 0 ] 2>/dev/null; then
            print_success "检测到 $SENSITIVE_COUNT 个敏感信息"
            echo "$BODY" | jq -r '.sensitive_infos[] | "   - 类型: \(.type), 置信度: \(.confidence)"' 2>/dev/null

            # 检查脱敏内容
            REDACTED=$(echo "$BODY" | jq -r '.redacted_content // empty')
            if [ -n "$REDACTED" ]; then
                print_success "内容已脱敏"
                print_info "脱敏后: $REDACTED"
            fi
        else
            print_warning "未检测到敏感信息（可能检测功能未启用）"
            print_info "响应: $BODY"
        fi
    else
        print_info "响应: $BODY"
    fi

    print_test "测试API密钥检测"
    API_KEY_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/security/scan" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" \
        -d '{
            "content": "我的API密钥是sk-1234567890abcdef，密码是MyPassword123!",
            "user_id": "caregiver_test"
        }' 2>/dev/null)

    HTTP_CODE=$(echo "$API_KEY_RESPONSE" | tail -n1)
    BODY=$(echo "$API_KEY_RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" = "200" ]; then
        SENSITIVE_COUNT=$(echo "$BODY" | jq -r '.sensitive_infos | length' 2>/dev/null)
        if [ "$SENSITIVE_COUNT" -gt 0 ] 2>/dev/null; then
            print_success "检测到API密钥/密码等敏感信息"
        fi
    fi
    echo ""
}

# ============================================
# 测试7: 速率限制测试
# ============================================
test_rate_limiting() {
    print_header "测试7: 速率限制功能"

    if [ -z "${ROLE_TOKENS[caregiver]}" ]; then
        print_warning "护工Token不存在，跳过速率限制测试"
        return
    fi

    print_test "快速发送多个请求测试速率限制"
    print_info "发送15个连续请求..."

    local success_count=0
    local rate_limited_count=0
    local error_count=0

    for i in {1..15}; do
        RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/chat" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${ROLE_TOKENS[caregiver]}" \
            -d "{
                \"user_id\": \"caregiver_test\",
                \"message\": \"速率测试消息 $i\",
                \"session_id\": \"rate_limit_test\"
            }" 2>/dev/null)

        HTTP_CODE=$(echo "$RESPONSE" | tail -n1)

        if [ "$HTTP_CODE" = "200" ]; then
            ((success_count++))
            echo -n "."
        elif [ "$HTTP_CODE" = "429" ]; then
            ((rate_limited_count++))
            echo -n "R"
        else
            ((error_count++))
            echo -n "E"
        fi
    done
    echo ""

    print_info "成功: $success_count, 速率限制: $rate_limited_count, 错误: $error_count"

    ((TOTAL_TESTS++))
    if [ "$rate_limited_count" -gt 0 ]; then
        print_success "速率限制功能正常工作（触发了 $rate_limited_count 次限制）"
        ((PASSED_TESTS++))
    else
        print_warning "未触发速率限制（可能限制阈值较高或未启用）"
    fi
    echo ""
}

# ============================================
# 测试8: 仪表盘数据测试
# ============================================
test_dashboard_data() {
    print_header "测试8: 角色仪表盘数据"

    for role in caregiver doctor family elder; do
        if [ -n "${ROLE_TOKENS[$role]}" ]; then
            print_test "获取 ${ROLE_NAMES[$role]} 仪表盘数据"
            RESPONSE=$(curl -s -w "\n%{http_code}" -X GET "$BASE_URL/api/dashboard" \
                -H "Authorization: Bearer ${ROLE_TOKENS[$role]}" 2>/dev/null)

            HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
            BODY=$(echo "$RESPONSE" | sed '$d')

            if check_http_status "200" "$HTTP_CODE" "${ROLE_NAMES[$role]}仪表盘"; then
                ELDER_COUNT=$(echo "$BODY" | jq -r '.data.elders | length' 2>/dev/null)
                ALERT_COUNT=$(echo "$BODY" | jq -r '.data.alerts | length' 2>/dev/null)
                print_info "老人数量: $ELDER_COUNT, 告警数量: $ALERT_COUNT"
            fi
        fi
    done
    echo ""
}

# ============================================
# 测试9: 跨角色权限测试
# ============================================
test_cross_role_permissions() {
    print_header "测试9: 跨角色权限控制"

    if [ -n "${ROLE_TOKENS[elder]}" ]; then
        print_test "测试老人角色呼叫护工功能"
        RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/call-caregiver" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${ROLE_TOKENS[elder]}" \
            -d '{
                "elder_id": "elder_test",
                "reason": "需要帮助"
            }' 2>/dev/null)

        HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
        check_http_status "200" "$HTTP_CODE" "老人呼叫护工"
    fi

    if [ -n "${ROLE_TOKENS[family]}" ]; then
        print_test "测试家属留言功能"
        RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/family-message" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${ROLE_TOKENS[family]}" \
            -d '{
                "elder_id": "elder_001",
                "message": "请多关照我的家人"
            }' 2>/dev/null)

        HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
        check_http_status "200" "$HTTP_CODE" "家属留言"
    fi
    echo ""
}

# ============================================
# 主测试流程
# ============================================
main() {
    print_header "Context-Keeper 全面自动化测试"
    echo "测试目标: $BASE_URL"
    echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""

    # 执行所有测试
    test_health_check
    test_role_login
    test_unauthorized_access
    test_chat_functionality
    test_file_upload
    test_security_detection
    test_rate_limiting
    test_dashboard_data
    test_cross_role_permissions

    # 打印测试报告
    print_header "测试报告"
    echo -e "${BLUE}总测试数:${NC} $TOTAL_TESTS"
    echo -e "${GREEN}通过:${NC} $PASSED_TESTS"
    echo -e "${RED}失败:${NC} $FAILED_TESTS"
    echo -e "${YELLOW}警告:${NC} $WARNINGS"
    echo ""

    # 计算成功率
    if [ $TOTAL_TESTS -gt 0 ]; then
        SUCCESS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
        echo -e "${BLUE}成功率:${NC} ${SUCCESS_RATE}%"
    fi

    echo ""
    echo "结束时间: $(date '+%Y-%m-%d %H:%M:%S')"

    # 返回退出码
    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "${GREEN}所有测试通过！${NC}"
        exit 0
    else
        echo -e "${RED}存在失败的测试${NC}"
        exit 1
    fi
}

# 运行主函数
main
