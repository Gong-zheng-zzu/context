#!/bin/bash
# Context-Keeper 快速启动脚本

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# 显示欢迎信息
show_welcome() {
    echo ""
    echo "=========================================="
    echo "  Context-Keeper 快速启动向导"
    echo "=========================================="
    echo ""
}

# 检查前置条件
check_prerequisites() {
    log_info "检查前置条件..."

    local all_ok=true

    # 检查 Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        all_ok=false
    else
        log_success "Docker 已安装: $(docker --version)"
    fi

    # 检查 Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose 未安装，请先安装 Docker Compose"
        all_ok=false
    else
        log_success "Docker Compose 已安装: $(docker-compose --version)"
    fi

    # 检查 Ollama
    if ! command -v ollama &> /dev/null; then
        log_error "Ollama 未安装，请先安装 Ollama"
        log_info "安装命令: curl -fsSL https://ollama.ai/install.sh | sh"
        all_ok=false
    else
        log_success "Ollama 已安装: $(ollama --version)"
    fi

    if [ "$all_ok" = false ]; then
        log_error "前置条件检查失败，请安装缺失的软件"
        exit 1
    fi

    echo ""
}

# 检查并安装 Ollama 模型
check_ollama_models() {
    log_info "检查 Ollama 模型..."

    local embedding_model="nomic-embed-text"
    local llm_model="qwen2.5:7b"

    # 检查 Embedding 模型
    if ollama list | grep -q "$embedding_model"; then
        log_success "Embedding 模型 $embedding_model 已安装"
    else
        log_warning "Embedding 模型 $embedding_model 未安装"
        read -p "是否现在安装? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "正在安装 $embedding_model..."
            ollama pull $embedding_model
            log_success "模型安装完成"
        else
            log_error "Embedding 模型是必需的，无法继续"
            exit 1
        fi
    fi

    # 检查 LLM 模型
    if ollama list | grep -q "qwen2.5"; then
        log_success "LLM 模型已安装"
    else
        log_warning "推荐的 LLM 模型未安装"
        echo ""
        echo "请选择要安装的 LLM 模型:"
        echo "1) qwen2.5:3b  (轻量级, ~2GB, 推荐新手)"
        echo "2) qwen2.5:7b  (平衡型, ~4.7GB, 推荐)"
        echo "3) deepseek-coder-v2:16b  (高性能, ~9GB, 推荐生产)"
        echo "4) 跳过（稍后手动安装）"
        read -p "请选择 (1-4): " choice

        case $choice in
            1)
                llm_model="qwen2.5:3b"
                ;;
            2)
                llm_model="qwen2.5:7b"
                ;;
            3)
                llm_model="deepseek-coder-v2:16b"
                ;;
            4)
                log_warning "跳过 LLM 模型安装，请稍后手动安装"
                return
                ;;
            *)
                log_error "无效选择"
                exit 1
                ;;
        esac

        log_info "正在安装 $llm_model..."
        ollama pull $llm_model
        log_success "模型安装完成"

        # 更新配置文件
        if [ -f "config/.env" ]; then
            sed -i.bak "s/LLM_MODEL=.*/LLM_MODEL=$llm_model/" config/.env
            log_success "已更新配置文件中的 LLM 模型"
        fi
    fi

    echo ""
}

# 初始化配置文件
init_config() {
    log_info "初始化配置文件..."

    if [ ! -f "config/.env" ]; then
        if [ -f "config/env.template" ]; then
            cp config/env.template config/.env
            log_success "已创建配置文件 config/.env"
        else
            log_error "配置模板文件 config/env.template 不存在"
            exit 1
        fi
    else
        log_info "配置文件已存在，跳过创建"
    fi

    echo ""
}

# 创建必要的目录
create_directories() {
    log_info "创建必要的目录..."

    mkdir -p data/logs
    mkdir -p backups

    log_success "目录创建完成"
    echo ""
}

# 启动服务
start_services() {
    log_info "启动 Docker 服务..."

    # 停止旧容器（如果存在）
    if docker-compose ps | grep -q "Up"; then
        log_info "检测到运行中的容器，正在停止..."
        docker-compose down
    fi

    # 启动服务
    log_info "正在启动所有服务（这可能需要几分钟）..."
    docker-compose up -d

    log_success "服务启动命令已执行"
    echo ""
}

# 等待服务就绪
wait_for_services() {
    log_info "等待服务就绪..."

    local max_wait=120
    local waited=0

    while [ $waited -lt $max_wait ]; do
        if curl -f -s http://localhost:8088/health >/dev/null 2>&1; then
            log_success "Context-Keeper 服务已就绪"
            return 0
        fi

        echo -n "."
        sleep 2
        waited=$((waited + 2))
    done

    echo ""
    log_warning "服务启动超时，请检查日志"
    return 1
}

# 运行健康检查
run_health_check() {
    log_info "运行健康检查..."
    echo ""

    if [ -f "scripts/health-check.sh" ]; then
        bash scripts/health-check.sh
    else
        log_warning "健康检查脚本不存在，跳过"
    fi
}

# 显示完成信息
show_completion() {
    echo ""
    echo "=========================================="
    log_success "Context-Keeper 启动完成！"
    echo "=========================================="
    echo ""
    echo "访问地址:"
    echo "  - Context-Keeper API: http://localhost:8088"
    echo "  - Qdrant Dashboard:   http://localhost:6333/dashboard"
    echo "  - Neo4j Browser:      http://localhost:7474"
    echo "    (用户名: neo4j, 密码: neo4j_password)"
    echo ""
    echo "常用命令:"
    echo "  - 查看日志:   docker-compose logs -f context-keeper"
    echo "  - 停止服务:   docker-compose stop"
    echo "  - 重启服务:   docker-compose restart"
    echo "  - 健康检查:   bash scripts/health-check.sh"
    echo ""
    echo "文档:"
    echo "  - 部署文档:   docs/DOCKER_DEPLOYMENT.md"
    echo "  - 项目主页:   https://github.com/redleaves/context-keeper"
    echo ""
}

# 主函数
main() {
    show_welcome
    check_prerequisites
    check_ollama_models
    init_config
    create_directories
    start_services

    if wait_for_services; then
        run_health_check
        show_completion
    else
        log_error "服务启动失败，请查看日志: docker-compose logs"
        exit 1
    fi
}

# 运行主函数
main "$@"
