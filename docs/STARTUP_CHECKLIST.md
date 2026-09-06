# Context-Keeper Docker 部署 - 首次启动检查清单

## 启动前检查清单

### 1. 系统环境检查

- [ ] 操作系统: Linux / macOS / Windows (WSL2)
- [ ] 内存: 至少 8GB 可用
- [ ] 磁盘空间: 至少 20GB 可用
- [ ] CPU: 4核心以上

### 2. 必需软件检查

```bash
# 检查 Docker
docker --version
# 需要: Docker 20.10+

# 检查 Docker Compose
docker-compose --version
# 需要: Docker Compose 2.0+

# 检查 Ollama
ollama --version
# 需要: Ollama 最新版本
```

### 3. 端口可用性检查

确保以下端口未被占用:

- [ ] 8088 - Context-Keeper 主服务
- [ ] 6333 - Qdrant HTTP API
- [ ] 6334 - Qdrant gRPC API
- [ ] 5432 - TimescaleDB/PostgreSQL
- [ ] 7474 - Neo4j HTTP
- [ ] 7687 - Neo4j Bolt
- [ ] 11434 - Ollama (宿主机)

```bash
# Linux/macOS 检查端口
netstat -tuln | grep -E '8088|6333|5432|7474|7687|11434'

# Windows 检查端口
netstat -ano | findstr "8088 6333 5432 7474 7687 11434"
```

### 4. Ollama 模型安装

```bash
# 必需: Embedding 模型
ollama pull nomic-embed-text

# 必需: LLM 模型（选择一个）
ollama pull qwen2.5:7b              # 推荐
# 或
ollama pull qwen2.5:3b              # 轻量级
# 或
ollama pull deepseek-coder-v2:16b   # 高性能

# 验证安装
ollama list
```

### 5. 配置文件检查

```bash
# 确保配置文件存在
ls -l config/.env

# 如果不存在，从模板创建
cp config/env.template config/.env

# 检查关键配置项
cat config/.env | grep -E "VECTOR_STORE_TYPE|LLM_PROVIDER|TIMELINE_STORAGE_ENABLED|KNOWLEDGE_GRAPH_ENABLED"
```

---

## 启动服务

### 方式 1: 使用快速启动脚本（推荐）

```bash
# 赋予执行权限
chmod +x scripts/quick-start.sh

# 运行快速启动
bash scripts/quick-start.sh
```

### 方式 2: 手动启动

```bash
# 1. 启动所有服务
docker-compose up -d

# 2. 查看启动状态
docker-compose ps

# 3. 查看日志
docker-compose logs -f
```

---

## 启动后验证

### 1. 容器状态检查

```bash
# 检查所有容器状态
docker-compose ps

# 预期输出: 所有服务状态为 "Up (healthy)"
```

**预期结果:**
```
NAME                        STATUS
context-keeper              Up (healthy)
context-keeper-qdrant       Up (healthy)
context-keeper-timescaledb  Up (healthy)
context-keeper-neo4j        Up (healthy)
```

### 2. 服务健康检查

```bash
# 运行健康检查脚本
chmod +x scripts/health-check.sh
bash scripts/health-check.sh
```

### 3. 手动验证各服务

#### Context-Keeper 主服务

```bash
# 健康检查
curl http://localhost:8088/health

# 预期输出:
# {"status":"healthy","version":"v2.0.0"}
```

#### Qdrant 向量数据库

```bash
# 检查连接
curl http://localhost:6333/collections

# 访问 Dashboard
# 浏览器打开: http://localhost:6333/dashboard
```

#### TimescaleDB 时序数据库

```bash
# 检查连接
docker-compose exec timescaledb pg_isready -U context_keeper

# 查看表结构
docker-compose exec timescaledb psql -U context_keeper -d context_keeper_timeline -c "\dt"

# 预期输出: timeline_events, conversation_messages, context_snapshots, session_statistics
```

#### Neo4j 图数据库

```bash
# 检查连接
curl -u neo4j:neo4j_password http://localhost:7474

# 访问 Neo4j Browser
# 浏览器打开: http://localhost:7474
# 用户名: neo4j
# 密码: neo4j_password

# 查看约束
docker-compose exec neo4j cypher-shell -u neo4j -p neo4j_password "SHOW CONSTRAINTS;"
```

#### Ollama 连接测试

```bash
# 从宿主机测试
curl http://localhost:11434/api/tags

# 从容器内测试
docker-compose exec context-keeper curl http://host.docker.internal:11434/api/tags
```

### 4. 功能测试

#### 测试 MCP 协议

```bash
# 列出可用工具
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

#### 测试记忆存储

```bash
# 存储记忆
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","id":2,"method":"tools/call",
    "params":{
      "name":"memorize_context",
      "arguments":{
        "sessionId":"test_session_001",
        "content":"这是一个测试记忆：项目采用微服务架构"
      }
    }
  }'
```

#### 测试记忆检索

```bash
# 检索记忆
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0","id":3,"method":"tools/call",
    "params":{
      "name":"retrieve_context",
      "arguments":{
        "sessionId":"test_session_001",
        "query":"项目架构"
      }
    }
  }'
```

---

## 常见问题快速排查

### 问题 1: 容器启动失败

```bash
# 查看详细日志
docker-compose logs

# 重新启动
docker-compose down
docker-compose up -d
```

### 问题 2: Ollama 连接失败

```bash
# 检查 Ollama 是否运行
ollama list

# 启动 Ollama 服务
ollama serve

# 测试连接
curl http://localhost:11434/api/tags
```

### 问题 3: 数据库初始化失败

```bash
# 查看数据库日志
docker-compose logs timescaledb
docker-compose logs neo4j

# 重新初始化
docker-compose down -v
docker-compose up -d
```

### 问题 4: 端口被占用

```bash
# 查找占用端口的进程
lsof -i :8088

# 停止占用进程或修改配置文件中的端口
vim config/.env
# 修改 PORT=8088 为其他端口
```

---

## 数据持久化验证

### 检查数据卷

```bash
# 列出所有数据卷
docker volume ls | grep context-keeper

# 预期输出:
# context-keeper-main_qdrant_data
# context-keeper-main_timescaledb_data
# context-keeper-main_neo4j_data
# context-keeper-main_neo4j_logs
```

### 检查数据目录

```bash
# 检查应用数据目录
ls -la ./data

# 预期输出:
# drwxr-xr-x  logs/
# -rw-r--r--  其他数据文件
```

---

## 性能基准测试

### 内存使用检查

```bash
# 查看容器内存使用
docker stats --no-stream

# 预期内存使用:
# context-keeper:     2-4GB
# qdrant:            500MB-1GB
# timescaledb:       500MB-1GB
# neo4j:             1-2GB
# 总计:              4-8GB
```

### 响应时间测试

```bash
# 测试健康检查响应时间
time curl http://localhost:8088/health

# 预期: < 100ms

# 测试向量检索响应时间
time curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"retrieve_context","arguments":{"sessionId":"test","query":"test"}}}'

# 预期: < 2s
```

---

## 下一步

启动成功后，您可以:

1. **集成到 IDE**
   - 参考 README.md 中的 Cursor/VSCode 集成指南

2. **查看完整文档**
   - 部署文档: `docs/DOCKER_DEPLOYMENT.md`
   - 项目主页: https://github.com/redleaves/context-keeper

3. **开始使用**
   - 通过 MCP 协议与 Context-Keeper 交互
   - 在 IDE 中配置 MCP 服务器

4. **监控和维护**
   - 定期运行健康检查: `bash scripts/health-check.sh`
   - 查看日志: `docker-compose logs -f`
   - 备份数据: 参考部署文档中的备份章节

---

## 获取帮助

如遇到问题:

1. 查看日志: `docker-compose logs -f`
2. 运行健康检查: `bash scripts/health-check.sh`
3. 查看完整文档: `docs/DOCKER_DEPLOYMENT.md`
4. 提交 Issue: https://github.com/redleaves/context-keeper/issues

---

**祝您使用愉快！**
