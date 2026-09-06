#!/bin/bash

# Context-Keeper Docker 快速启动脚本
# 用于快速启动和管理 Docker 服务

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_ROOT"

# 显示帮助信息
show_help() {
    echo -e "${BLUE}Context-Keeper Docker 管理脚本${NC}"
    echo ""
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  build       构建 Docker 镜像"
    echo "  up          启动所有服务"
    echo "  down        停止所有服务"
    echo "  restart     重启所有服务"
    echo "  logs        查看服务日志"
    echo "  status      查看服务状态"
    echo "  clean       清理所有容器和数据卷"
    echo "  shell       进入容器 shell"
    echo "  help        显示此帮助信息"
    echo ""
    echo "选项:"
    echo "  --build     启动时重新构建镜像"
    echo "  --gpu       启用 GPU 支持"
    echo "  --monitor   启用监控服务（Prometheus + Grafana）"
    echo "  --proxy     启用 Nginx 反向代理"
    echo ""
    echo "示例:"
    echo "  $0 up                    # 启动服务"
    echo "  $0 up --build            # 重新构建并启动"
    echo "  $0 up --monitor          # 启动服务和监控"
    echo "  $0 logs context-keeper   # 查看主服务日志"
    echo "  $0 shell                 # 进入主容器"
}

# 检查 Docker 和 Docker Compose
check_requirements() {
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}错误: 未安装 Docker${NC}"
        exit 1
    fi

    if ! docker compose version &> /dev/null; then
        echo -e "${RED}错误: 未安装 Docker Compose 或版本过低${NC}"
        echo -e "${YELLOW}请安装 Docker Compose v2.0+${NC}"
        exit 1
    fi
}

# 检查 Ollama 服务
check_ollama() {
    echo -e "${YELLOW}检查 Ollama 服务...${NC}"
    if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Ollama 服务运行正常${NC}"

        # 检查必需的模型
        if curl -s http://localhost:11434/api/tags | grep -q "nomic-embed-text"; then
            echo -e "${GREEN}✓ 嵌入模型 nomic-embed-text 已安装${NC}"
        else
            echo -e "${YELLOW}⚠ 嵌入模型 nomic-embed-text 未安装${NC}"
            echo -e "${YELLOW}  请运行: ollama pull nomic-embed-text${NC}"
        fi

        if curl -s http://localhost:11434/api/tags | grep -q "qwen2.5:3b"; then
            echo -e "${GREEN}✓ LLM 模型 qwen2.5:3b 已安装${NC}"
        else
            echo -e "${YELLOW}⚠ LLM 模型 qwen2.5:3b 未安装${NC}"
            echo -e "${YELLOW}  请运行: ollama pull qwen2.5:3b${NC}"
        fi
    else
        echo -e "${RED}✗ Ollama 服务未运行${NC}"
        echo -e "${YELLOW}  请先启动 Ollama 服务${NC}"
        exit 1
    fi
}

# 构建镜像
build_image() {
    echo -e "${BLUE}构建 Docker 镜像...${NC}"

    export VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "latest")
    export BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    export COMMIT_HASH=$(git rev-parse HEAD 2>/dev/null || echo "unknown")

    echo -e "${YELLOW}构建信息:${NC}"
    echo "  版本: $VERSION"
    echo "  构建时间: $BUILD_TIME"
    echo "  提交哈希: $COMMIT_HASH"

    docker compose build --no-cache
    echo -e "${GREEN}✓ 镜像构建完成${NC}"
}

# 启动服务
start_services() {
    local BUILD_FLAG=""
    local PROFILES=""

    # 解析选项
    while [[ $# -gt 0 ]]; do
        case $1 in
            --build)
                BUILD_FLAG="--build"
                shift
                ;;
            --monitor)
                PROFILES="$PROFILES --profile monitoring"
                shift
                ;;
            --proxy)
                PROFILES="$PROFILES --profile proxy"
                shift
                ;;
            *)
                shift
                ;;
        esac
    done

    echo -e "${BLUE}启动 Context-Keeper 服务...${NC}"

    # 检查 Ollama
    check_ollama

    # 启动服务
    docker compose $PROFILES up -d $BUILD_FLAG

    echo -e "${GREEN}✓ 服务启动成功${NC}"
    echo ""
    echo -e "${YELLOW}服务访问地址:${NC}"
    echo "  主服务: http://localhost:8088"
    echo "  健康检查: http://localhost:8088/health"
    echo "  Qdrant: http://localhost:6333"

    if [[ $PROFILES == *"monitoring"* ]]; then
        echo "  Prometheus: http://localhost:9090"
        echo "  Grafana: http://localhost:3000"
    fi

    if [[ $PROFILES == *"proxy"* ]]; then
        echo "  Nginx: http://localhost:80"
    fi

    echo ""
    echo -e "${YELLOW}查看日志: $0 logs${NC}"
}

# 停止服务
stop_services() {
    echo -e "${BLUE}停止服务...${NC}"
    docker compose down
    echo -e "${GREEN}✓ 服务已停止${NC}"
}

# 重启服务
restart_services() {
    echo -e "${BLUE}重启服务...${NC}"
    docker compose restart
    echo -e "${GREEN}✓ 服务已重启${NC}"
}

# 查看日志
view_logs() {
    local SERVICE="${1:-context-keeper}"
    echo -e "${BLUE}查看 $SERVICE 日志 (Ctrl+C 退出)...${NC}"
    docker compose logs -f "$SERVICE"
}

# 查看状态
show_status() {
    echo -e "${BLUE}服务状态:${NC}"
    docker compose ps
    echo ""
    echo -e "${BLUE}资源使用:${NC}"
    docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" \
        $(docker compose ps -q 2>/dev/null)
}

# 清理
clean_all() {
    echo -e "${RED}警告: 此操作将删除所有容器、镜像和数据卷${NC}"
    read -p "确认继续? (yes/no): " confirm

    if [ "$confirm" = "yes" ]; then
        echo -e "${BLUE}清理中...${NC}"
        docker compose down -v --rmi all
        echo -e "${GREEN}✓ 清理完成${NC}"
    else
        echo -e "${YELLOW}已取消${NC}"
    fi
}

# 进入容器 shell
enter_shell() {
    local SERVICE="${1:-context-keeper}"
    echo -e "${BLUE}进入 $SERVICE 容器...${NC}"
    docker compose exec "$SERVICE" /bin/bash
}

# 主逻辑
main() {
    check_requirements

    case "${1:-help}" in
        build)
            build_image
            ;;
        up|start)
            shift
            start_services "$@"
            ;;
        down|stop)
            stop_services
            ;;
        restart)
            restart_services
            ;;
        logs)
            shift
            view_logs "$@"
            ;;
        status|ps)
            show_status
            ;;
        clean)
            clean_all
            ;;
        shell|exec)
            shift
            enter_shell "$@"
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            echo -e "${RED}未知命令: $1${NC}"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

main "$@"
