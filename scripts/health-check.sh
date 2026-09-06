#!/bin/bash
# Context-Keeper 健康检查和初始化验证脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查命令是否存在
check_command() {
    if ! command -v $1 &> /dev/null; then
        log_error "$1 未安装，请先安装"
        return 1
    fi
    return 0
}

# 检查端口是否被占用
check_port() {
    local port=$1
    local service=$2

    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1 || netstat -tuln 2>/dev/null | grep -q ":$port "; then
        log_warning "端口 $port ($service) 已被占用"
        return 1
    else
        log_success "端口 $port ($service) 可用"
        return 0
    fi
}

# 检查 Docker 容器状态
check_container() {
    local container=$1
    local status=$(docker inspect -f '{{.State.Status}}' $container 2>/dev/null)
    local health=$(docker inspect -f '{{.State.Health.Status}}' $container 2>/dev/null)

    if [ "$status" = "running" ]; then
        if [ "$health" = "healthy" ] || [ "$health" = "" ]; then
            log_success "容器 $container 运行正常"
            return 0
        else
            log_warning "容器 $container 运行中但健康检查失败: $health"
            return 1
        fi
    else
        log_error "容器 $container 未运行: $status"
        return 1
    fi
}

# 检查 HTTP 服务
check_http_service() {
    local url=$1
    local service=$2
    local max_retries=5
    local retry=0

    while [ $retry -lt $max_retries ]; do
        if curl -f -s -o /dev/null "$url"; then
            log_success "$service HTTP 服务正常"
            return 0
        fi
        retry=$((retry + 1))
        if [ $retry -lt $max_retries ]; then
            log_info "$service 未就绪，等待重试 ($retry/$max_retries)..."
            sleep 2
        fi
    done

    log_error "$service HTTP 服务检查失败"
    return 1
}

# 检查 Ollama 模型
check_ollama_models() {
    log_info "检查 Ollama 模型..."

    if ! check_command ollama; then
        return 1
    fi

    local required_models=("nomic-embed-text")
    local missing_models=()

    for model in "${required_models[@]}"; do
        if ollama list | grep -q "$model"; then
            log_success "模型 $model 已安装"
        else
            log_error "模型 $model 未安装"
            missing_models+=("$model")
        fi
    done

    if [ ${#missing_models[@]} -gt 0 ]; then
        log_warning "缺失模型: ${missing_models[*]}"
        log_info "请运行: ollama pull ${missing_models[0]}"
        return 1
    fi

    return 0
}

# 检查数据库连接
check_database_connection() {
    local db_type=$1

    case $db_type in
        "qdrant")
            log_info "检查 Qdrant 连接..."
            if curl -f -s http://localhost:6333/collections >/dev/null; then
                log_success "Qdrant 连接正常"

                # 检查集合是否存在
                if curl -s http://localhost:6333/collections/context_keeper | grep -q "context_keeper"; then
                    log_success "Qdrant 集合 context_keeper 已创建"
                else
                    log_warning "Qdrant 集合 context_keeper 未创建（首次运行时会自动创建）"
                fi
                return 0
            else
                log_error "Qdrant 连接失败"
                return 1
            fi
            ;;

        "timescaledb")
            log_info "检查 TimescaleDB 连接..."
            if docker-compose exec -T timescaledb pg_isready -U context_keeper >/dev/null 2>&1; then
                log_success "TimescaleDB 连接正常"

                # 检查表是否存在
                local tables=$(docker-compose exec -T timescaledb psql -U context_keeper -d context_keeper_timeline -t -c "SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public';" 2>/dev/null | tr -d ' ')
                if [ "$tables" -gt 0 ]; then
                    log_success "TimescaleDB 表已初始化 ($tables 个表)"
                else
                    log_warning "TimescaleDB 表未初始化"
                fi
                return 0
            else
                log_error "TimescaleDB 连接失败"
                return 1
            fi
            ;;

        "neo4j")
            log_info "检查 Neo4j 连接..."
            if curl -f -s -u neo4j:neo4j_password http://localhost:7474 >/dev/null 2>&1; then
                log_success "Neo4j 连接正常"

                # 检查约束是否存在
                local constraints=$(docker-compose exec -T neo4j cypher-shell -u neo4j -p neo4j_password "SHOW CONSTRAINTS;" 2>/dev/null | wc -l)
                if [ "$constraints" -gt 1 ]; then
                    log_success "Neo4j 约束已初始化"
                else
                    log_warning "Neo4j 约束未初始化"
                fi
                return 0
            else
                log_error "Neo4j 连接失败"
                return 1
            fi
            ;;
    esac
}

# 主检查流程
main() {
    echo "=========================================="
    echo "  Context-Keeper 健康检查"
    echo "=========================================="
    echo ""

    local all_passed=true

    # 1. 检查必需命令
    log_info "步骤 1: 检查必需命令..."
    check_command docker || all_passed=false
    check_command docker-compose || all_passed=false
    check_command curl || all_passed=false
    check_command ollama || all_passed=false
    echo ""

    # 2. 检查端口可用性（仅在容器未运行时检查）
    log_info "步骤 2: 检查端口可用性..."
    if ! docker ps | grep -q context-keeper; then
        check_port 8088 "Context-Keeper"
        check_port 6333 "Qdrant"
        check_port 5432 "TimescaleDB"
        check_port 7474 "Neo4j HTTP"
        check_port 7687 "Neo4j Bolt"
    else
        log_info "容器已运行，跳过端口检查"
    fi
    echo ""

    # 3. 检查 Ollama 模型
    log_info "步骤 3: 检查 Ollama 模型..."
    check_ollama_models || all_passed=false
    echo ""

    # 4. 检查 Docker 容器状态
    log_info "步骤 4: 检查 Docker 容器状态..."
    check_container "context-keeper-qdrant" || all_passed=false
    check_container "context-keeper-timescaledb" || all_passed=false
    check_container "context-keeper-neo4j" || all_passed=false
    check_container "context-keeper" || all_passed=false
    echo ""

    # 5. 检查 HTTP 服务
    log_info "步骤 5: 检查 HTTP 服务..."
    check_http_service "http://localhost:8088/health" "Context-Keeper" || all_passed=false
    check_http_service "http://localhost:6333/health" "Qdrant" || all_passed=false
    check_http_service "http://localhost:7474" "Neo4j" || all_passed=false
    echo ""

    # 6. 检查数据库连接
    log_info "步骤 6: 检查数据库连接..."
    check_database_connection "qdrant" || all_passed=false
    check_database_connection "timescaledb" || all_passed=false
    check_database_connection "neo4j" || all_passed=false
    echo ""

    # 7. 检查 Ollama 连接（从容器内）
    log_info "步骤 7: 检查 Ollama 连接（从容器内）..."
    if docker-compose exec -T context-keeper curl -f -s http://host.docker.internal:11434/api/tags >/dev/null 2>&1; then
        log_success "容器可以访问宿主机 Ollama 服务"
    else
        log_error "容器无法访问宿主机 Ollama 服务"
        log_info "请确保 Ollama 正在运行: ollama serve"
        all_passed=false
    fi
    echo ""

    # 8. 测试 MCP 协议
    log_info "步骤 8: 测试 MCP 协议..."
    if curl -f -s -X POST http://localhost:8088/mcp \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | grep -q "tools"; then
        log_success "MCP 协议测试通过"
    else
        log_error "MCP 协议测试失败"
        all_passed=false
    fi
    echo ""

    # 总结
    echo "=========================================="
    if [ "$all_passed" = true ]; then
        log_success "所有检查通过！Context-Keeper 运行正常"
        echo ""
        log_info "访问地址: http://localhost:8088"
        log_info "Qdrant UI: http://localhost:6333/dashboard"
        log_info "Neo4j Browser: http://localhost:7474"
        echo ""
        exit 0
    else
        log_error "部分检查失败，请查看上述错误信息"
        echo ""
        log_info "常见问题排查:"
        log_info "1. 确保所有容器正在运行: docker-compose ps"
        log_info "2. 查看容器日志: docker-compose logs -f"
        log_info "3. 确保 Ollama 正在运行: ollama list"
        log_info "4. 查看完整文档: docs/DOCKER_DEPLOYMENT.md"
        echo ""
        exit 1
    fi
}

# 运行主函数
main "$@"
