# Context-Keeper Docker 部署配置总结

## 已完成的配置工作

本次配置根据 README.md 的要求，完善了 Context-Keeper 的 Docker 部署环境。

---

## 1. 新增的数据库服务

### TimescaleDB (时间线存储)

**配置位置**: `docker-compose.yml`

```yaml
timescaledb:
  image: timescale/timescaledb:latest-pg16
  ports: ["5432:5432"]
  environment:
    - POSTGRES_DB=context_keeper_timeline
    - POSTGRES_USER=context_keeper
    - POSTGRES_PASSWORD=context_keeper_password
```

**初始化脚本**: `scripts/init/timescaledb/01-init-timescaledb.sql`

包含:
- 4个核心表: timeline_events, conversation_messages, context_snapshots, session_statistics
- TimescaleDB 超表配置
- 索引和约束
- 数据保留策略 (90-365天)
- 连续聚合视图

### Neo4j (知识图谱)

**配置位置**: `docker-compose.yml`

```yaml
neo4j:
  image: neo4j:5.15-community
  ports: ["7474:7474", "7687:7687"]
  environment:
    - NEO4J_AUTH=neo4j/neo4j_password
    - NEO4JLABS_PLUGINS=["apoc"]
```

**初始化脚本**: `scripts/init/neo4j/01-init-neo4j.cypher`

包含:
- 8个节点类型的唯一性约束
- 多维度索引 (时间、名称、类型等)
- 全文搜索索引
- 示例数据 (系统用户和默认工作空间)

---

## 2. 更新的配置文件

### docker-compose.yml

**主要变更**:
- 新增 `timescaledb` 服务
- 新增 `neo4j` 服务
- 更新 `context-keeper` 依赖关系 (依赖所有3个数据库)
- 新增环境变量传递 (TIMESCALEDB_HOST, NEO4J_URI)
- 新增数据卷 (timescaledb_data, neo4j_data, neo4j_logs 等)

### config/.env

**主要变更**:
```bash
# 启用时间线存储
TIMELINE_STORAGE_ENABLED=true
TIMESCALEDB_HOST=timescaledb
TIMESCALEDB_USERNAME=context_keeper
TIMESCALEDB_PASSWORD=context_keeper_password

# 启用知识图谱
KNOWLEDGE_GRAPH_ENABLED=true
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=neo4j_password
```

### config/env.template

**主要变更**:
- 更新默认值为 Docker 环境配置
- 添加详细注释说明
- 设置合理的默认密码

---

## 3. 新增的脚本和文档

### 初始化脚本

1. **scripts/init/timescaledb/01-init-timescaledb.sql**
   - TimescaleDB 数据库结构初始化
   - 自动在容器启动时执行

2. **scripts/init/neo4j/01-init-neo4j.cypher**
   - Neo4j 图数据库结构初始化
   - 需要手动执行或通过脚本执行

3. **scripts/init/neo4j/init-neo4j.sh**
   - Neo4j 初始化脚本执行器
   - 等待 Neo4j 就绪后自动执行

### 运维脚本

1. **scripts/health-check.sh**
   - 全面的健康检查脚本
   - 检查 8 个维度: 命令、端口、模型、容器、HTTP服务、数据库、Ollama、MCP协议
   - 彩色输出，易于阅读

2. **scripts/quick-start.sh**
   - 一键启动脚本
   - 自动检查前置条件
   - 引导安装 Ollama 模型
   - 自动初始化配置
   - 启动服务并验证

### 文档

1. **docs/DOCKER_DEPLOYMENT.md** (完整部署文档)
   - 系统架构图
   - 前置要求
   - 快速开始指南
   - 详细配置说明
   - 首次启动检查清单
   - 常见问题排查 (5个典型问题)
   - Ollama 模型安装指南 (3种方案)
   - 服务管理命令
   - 数据备份方案
   - 性能优化建议
   - 安全配置建议

2. **docs/STARTUP_CHECKLIST.md** (启动检查清单)
   - 启动前检查清单 (5个维度)
   - 启动服务步骤
   - 启动后验证 (4个层次)
   - 功能测试脚本
   - 常见问题快速排查
   - 数据持久化验证
   - 性能基准测试

---

## 4. 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Context-Keeper 应用                       │
│                     (端口: 8088)                             │
└────────────┬────────────┬────────────┬──────────────────────┘
             │            │            │
    ┌────────▼───┐  ┌────▼─────┐  ┌──▼──────────┐
    │  Qdrant    │  │TimescaleDB│  │   Neo4j     │
    │  向量存储   │  │ 时间线存储 │  │  知识图谱   │
    │ (6333)     │  │  (5432)   │  │(7474/7687)  │
    └────────────┘  └───────────┘  └─────────────┘
         │
    ┌────▼──────────┐
    │ Ollama (宿主机)│
    │  本地LLM服务   │
    │   (11434)     │
    └───────────────┘
```

---

## 5. 快速启动步骤

### 方式 1: 使用快速启动脚本 (推荐)

```bash
# 1. 赋予执行权限
chmod +x scripts/quick-start.sh

# 2. 运行快速启动
bash scripts/quick-start.sh
```

脚本会自动:
- 检查前置条件 (Docker, Docker Compose, Ollama)
- 检查并安装 Ollama 模型
- 初始化配置文件
- 创建必要目录
- 启动所有服务
- 等待服务就绪
- 运行健康检查
- 显示访问地址

### 方式 2: 手动启动

```bash
# 1. 安装 Ollama 模型
ollama pull nomic-embed-text
ollama pull qwen2.5:7b

# 2. 初始化配置
cp config/env.template config/.env

# 3. 启动服务
docker-compose up -d

# 4. 验证部署
bash scripts/health-check.sh
```

---

## 6. 验证部署

### 自动验证

```bash
# 运行健康检查脚本
bash scripts/health-check.sh
```

### 手动验证

```bash
# 1. 检查容器状态
docker-compose ps

# 2. 检查主服务
curl http://localhost:8088/health

# 3. 检查 Qdrant
curl http://localhost:6333/collections

# 4. 检查 TimescaleDB
docker-compose exec timescaledb psql -U context_keeper -d context_keeper_timeline -c "\dt"

# 5. 检查 Neo4j
curl -u neo4j:neo4j_password http://localhost:7474

# 6. 测试 MCP 协议
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

---

## 7. 关键配置项

### 向量存储 (Qdrant)

```bash
VECTOR_STORE_TYPE=qdrant
QDRANT_URL=http://qdrant:6333
QDRANT_DIMENSION=768  # nomic-embed-text 维度
```

### Embedding 服务 (Ollama)

```bash
EMBEDDING_API_URL=http://host.docker.internal:11434/api/embeddings
EMBEDDING_MODEL=nomic-embed-text
```

### LLM 配置 (Ollama)

```bash
LLM_PROVIDER=ollama_local
LLM_MODEL=qwen2.5:7b
```

### 时间线存储 (TimescaleDB)

```bash
TIMELINE_STORAGE_ENABLED=true
TIMESCALEDB_HOST=timescaledb
TIMESCALEDB_USERNAME=context_keeper
TIMESCALEDB_PASSWORD=context_keeper_password
```

### 知识图谱 (Neo4j)

```bash
KNOWLEDGE_GRAPH_ENABLED=true
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=neo4j_password
```

---

## 8. 数据持久化

所有数据通过 Docker 卷持久化:

- `qdrant_data` - Qdrant 向量数据
- `timescaledb_data` - TimescaleDB 时序数据
- `neo4j_data` - Neo4j 图数据
- `neo4j_logs` - Neo4j 日志
- `./data` - Context-Keeper 应用数据
- `./data/logs` - 应用日志

---

## 9. 端口映射

| 服务 | 容器端口 | 宿主机端口 | 用途 |
|------|---------|-----------|------|
| Context-Keeper | 8088 | 8088 | HTTP API |
| Qdrant | 6333 | 6333 | HTTP API |
| Qdrant | 6334 | 6334 | gRPC API |
| TimescaleDB | 5432 | 5432 | PostgreSQL |
| Neo4j | 7474 | 7474 | HTTP/Browser |
| Neo4j | 7687 | 7687 | Bolt |
| Ollama | 11434 | 11434 | API (宿主机) |

---

## 10. 资源需求

### 最低配置
- CPU: 4核心
- 内存: 8GB
- 磁盘: 20GB

### 推荐配置
- CPU: 8核心
- 内存: 16GB
- 磁盘: 50GB

### 内存分配
- Context-Keeper: 2-4GB
- Qdrant: 500MB-1GB
- TimescaleDB: 500MB-1GB
- Neo4j: 1-2GB
- Ollama: 4-8GB (取决于模型大小)

---

## 11. 常见问题

### Q1: Ollama 连接失败

**解决方案**:
```bash
# 确保 Ollama 正在运行
ollama serve

# 测试连接
curl http://localhost:11434/api/tags
```

### Q2: 数据库初始化失败

**解决方案**:
```bash
# 重新初始化
docker-compose down -v
docker-compose up -d
```

### Q3: 端口被占用

**解决方案**:
```bash
# 查找占用进程
lsof -i :8088

# 修改配置文件中的端口
vim config/.env
```

### Q4: 内存不足

**解决方案**:
```bash
# 使用更小的模型
LLM_MODEL=qwen2.5:3b

# 调整 Neo4j 内存
NEO4J_dbms_memory_heap_max__size=1G
```

---

## 12. 下一步

1. **集成到 IDE**
   - 参考 README.md 中的 Cursor/VSCode 集成指南

2. **开始使用**
   - 通过 MCP 协议与 Context-Keeper 交互

3. **监控和维护**
   - 定期运行: `bash scripts/health-check.sh`
   - 查看日志: `docker-compose logs -f`

4. **备份数据**
   - 参考 `docs/DOCKER_DEPLOYMENT.md` 中的备份章节

---

## 文件清单

### 新增文件
- `scripts/init/timescaledb/01-init-timescaledb.sql`
- `scripts/init/neo4j/01-init-neo4j.cypher`
- `scripts/init/neo4j/init-neo4j.sh`
- `scripts/health-check.sh`
- `scripts/quick-start.sh`
- `docs/DOCKER_DEPLOYMENT.md`
- `docs/STARTUP_CHECKLIST.md`

### 修改文件
- `docker-compose.yml` - 新增 TimescaleDB 和 Neo4j 服务
- `config/.env` - 更新数据库配置
- `config/env.template` - 更新默认配置

---

**配置完成！现在可以使用 `bash scripts/quick-start.sh` 快速启动 Context-Keeper。**
