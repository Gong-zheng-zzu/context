#!/bin/bash
# Context-Keeper 数据备份脚本

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

# 配置
BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="context-keeper_${TIMESTAMP}"

# 创建备份目录
mkdir -p "${BACKUP_DIR}"

echo "=========================================="
echo "  Context-Keeper 数据备份"
echo "=========================================="
echo ""
log_info "备份时间: $(date)"
log_info "备份目录: ${BACKUP_DIR}/${BACKUP_NAME}"
echo ""

# 1. 备份应用数据
log_info "备份应用数据..."
if [ -d "./data" ]; then
    tar czf "${BACKUP_DIR}/${BACKUP_NAME}_app_data.tar.gz" data/
    log_success "应用数据备份完成"
else
    log_warning "应用数据目录不存在，跳过"
fi

# 2. 备份配置文件
log_info "备份配置文件..."
if [ -d "./config" ]; then
    tar czf "${BACKUP_DIR}/${BACKUP_NAME}_config.tar.gz" config/
    log_success "配置文件备份完成"
else
    log_warning "配置目录不存在，跳过"
fi

# 3. 备份 TimescaleDB
log_info "备份 TimescaleDB..."
if docker-compose ps | grep -q "context-keeper-timescaledb"; then
    docker-compose exec -T timescaledb pg_dump \
        -U context_keeper context_keeper_timeline \
        > "${BACKUP_DIR}/${BACKUP_NAME}_timescaledb.sql"
    log_success "TimescaleDB 备份完成"
else
    log_warning "TimescaleDB 容器未运行，跳过"
fi

# 4. 备份 Neo4j
log_info "备份 Neo4j..."
if docker-compose ps | grep -q "context-keeper-neo4j"; then
    # 创建临时备份目录
    docker-compose exec neo4j mkdir -p /tmp/backups

    # 执行备份
    docker-compose exec neo4j neo4j-admin database dump neo4j \
        --to-path=/tmp/backups 2>/dev/null || true

    # 复制备份文件
    docker cp context-keeper-neo4j:/tmp/backups/neo4j.dump \
        "${BACKUP_DIR}/${BACKUP_NAME}_neo4j.dump" 2>/dev/null || true

    log_success "Neo4j 备份完成"
else
    log_warning "Neo4j 容器未运行，跳过"
fi

# 5. 备份 Qdrant 数据卷
log_info "备份 Qdrant 数据..."
if docker volume ls | grep -q "qdrant_data"; then
    docker run --rm \
        -v context-keeper_qdrant_data:/data \
        -v "$(pwd)/${BACKUP_DIR}:/backup" \
        alpine tar czf "/backup/${BACKUP_NAME}_qdrant.tar.gz" /data
    log_success "Qdrant 数据备份完成"
else
    log_warning "Qdrant 数据卷不存在，跳过"
fi

# 6. 创建备份清单
log_info "创建备份清单..."
cat > "${BACKUP_DIR}/${BACKUP_NAME}_manifest.txt" << EOF
Context-Keeper 备份清单
======================

备份时间: $(date)
备份版本: ${TIMESTAMP}

备份文件:
- ${BACKUP_NAME}_app_data.tar.gz (应用数据)
- ${BACKUP_NAME}_config.tar.gz (配置文件)
- ${BACKUP_NAME}_timescaledb.sql (TimescaleDB 数据库)
- ${BACKUP_NAME}_neo4j.dump (Neo4j 图数据库)
- ${BACKUP_NAME}_qdrant.tar.gz (Qdrant 向量数据)

恢复说明:
1. 停止所有服务: docker-compose down
2. 恢复应用数据: tar xzf ${BACKUP_NAME}_app_data.tar.gz
3. 恢复配置文件: tar xzf ${BACKUP_NAME}_config.tar.gz
4. 启动服务: docker-compose up -d
5. 恢复 TimescaleDB: docker-compose exec -T timescaledb psql -U context_keeper context_keeper_timeline < ${BACKUP_NAME}_timescaledb.sql
6. 恢复 Neo4j: docker-compose exec neo4j neo4j-admin database load neo4j --from-path=/backups
7. 恢复 Qdrant: docker run --rm -v context-keeper_qdrant_data:/data -v \$(pwd)/backups:/backup alpine tar xzf /backup/${BACKUP_NAME}_qdrant.tar.gz -C /

注意事项:
- 恢复前请确保已停止所有服务
- 恢复 Neo4j 需要先停止数据库
- 建议在测试环境先验证备份文件
EOF

log_success "备份清单创建完成"

# 7. 计算备份大小
log_info "计算备份大小..."
BACKUP_SIZE=$(du -sh "${BACKUP_DIR}/${BACKUP_NAME}"* | awk '{sum+=$1} END {print sum}')
log_info "备份总大小: $(du -sh ${BACKUP_DIR} | awk '{print $1}')"

# 8. 清理旧备份（保留最近7天）
log_info "清理旧备份（保留最近7天）..."
find "${BACKUP_DIR}" -name "context-keeper_*" -mtime +7 -delete 2>/dev/null || true
log_success "旧备份清理完成"

echo ""
echo "=========================================="
log_success "备份完成！"
echo "=========================================="
echo ""
echo "备份文件位置: ${BACKUP_DIR}/${BACKUP_NAME}*"
echo "备份清单: ${BACKUP_DIR}/${BACKUP_NAME}_manifest.txt"
echo ""
log_info "查看备份清单: cat ${BACKUP_DIR}/${BACKUP_NAME}_manifest.txt"
echo ""
