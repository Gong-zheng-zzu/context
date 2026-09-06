# Context-Keeper Docker 部署指南

## 快速开始

### 前置要求

1. **Docker 和 Docker Compose**
   - Docker Engine 20.10+
   - Docker Compose v2.0+

2. **Ollama 本地服务**（必需）
   - 确保 Ollama 在宿主机运行（http://localhost:11434）
   - 安装必需的模型：
     ```bash
     ollama pull nomic-embed-text    # 嵌入模型
     ollama pull qwen2.5:3b          # LLM 模型
     ```

3. **系统要求**
   - 内存: 至少 4GB 可用
   - 磁盘: 至少 10GB 可用空间
   - CPU: 2核心以上推荐

### 一键启动

**Linux/Mac:**
```bash
./docker.sh up
```

**Windows:**
```cmd
docker.bat up
```

### 手动启动

```bash
# 1. 构建镜像
docker compose build

# 2. 启动服务
docker compose up -d

# 3. 查看日志
docker compose logs -f context-keeper

# 4. 检查状态
docker compose ps
```

## 服务访问

启动成功后，可以通过以下地址访问：

- **主服务**: http://localhost:8088
- **健康检查**: http://localhost:8088/health
- **Qdrant 向量数据库**: http://localhost:6333
- **Qdrant Dashboard**: http://localhost:6333/dashboard

## 配置说明

### 环境变量配置

主要配置文件：`config/.env`

关键配置项：

```bash
# 服务配置
HTTP_SERVER_PORT=8088
LOG_LEVEL=info

# Ollama 配置（使用宿主机服务）
EMBEDDING_API_URL=http://host.docker.internal:11434/api/embeddings
EMBEDDING_MODEL=nomic-embed-text
LLM_PROVIDER=ollama_local
LLM_MODEL=qwen2.5:3b

# Qdrant 配置（使用容器内网络）
QDRANT_URL=http://qdrant:6333
QDRANT_COLLECTION=context_keeper
QDRANT_DIMENSION=768

# 安全配置
JWT_SECRET=your-secret-key-here
SECURITY_ENABLED=true
```

### 端口配置

可以通过环境变量修改端口：

```bash
# 修改主服务端口
HOST_PORT=9000 docker compose up -d

# 或在 .env 文件中设置
HOST_PORT=9000
```

### 数据持久化

数据卷挂载：

```yaml
volumes:
  - ./data:/app/data              # 应用数据
  - ./data/logs:/app/data/logs    # 日志文件
  - ./config:/app/config:ro       # 配置文件（只读）
  - ./web:/app/web:ro             # 前端文件（只读）
```

Qdrant 数据使用 Docker 卷：
```yaml
volumes:
  - qdrant_data:/qdrant/storage
```

## 常用命令

### 使用管理脚本

**Linux/Mac (docker.sh):**
```bash
./docker.sh build      # 构建镜像
./docker.sh up         # 启动服务
./docker.sh down       # 停止服务
./docker.sh restart    # 重启服务
./docker.sh logs       # 查看日志
./docker.sh status     # 查看状态
./docker.sh shell      # 进入容器
./docker.sh clean      # 清理所有
```

**Windows (docker.bat):**
```cmd
docker.bat build       # 构建镜像
docker.bat up          # 启动服务
docker.bat down        # 停止服务
docker.bat restart     # 重启服务
docker.bat logs        # 查看日志
docker.bat status      # 查看状态
docker.bat shell       # 进入容器
docker.bat clean       # 清理所有
```

### 使用 Docker Compose

```bash
# 启动服务
docker compose up -d

# 停止服务
docker compose down

# 重启服务
docker compose restart

# 查看日志
docker compose logs -f context-keeper
docker compose logs -f qdrant

# 查看状态
docker compose ps

# 进入容器
docker compose exec context-keeper /bin/bash

# 重新构建并启动
docker compose up -d --build

# 停止并删除数据卷
docker compose down -v
```

## 故障排查

### 1. 容器无法启动

**检查日志:**
```bash
docker compose logs context-keeper
```

**常见问题:**
- Ollama 服务未运行
- 端口被占用
- 配置文件错误

### 2. 无法连接 Ollama

**检查 Ollama 服务:**
```bash
curl http://localhost:11434/api/tags
```

**Windows Docker Desktop 用户:**
确保 Docker 设置中启用了 "Use the WSL 2 based engine"

### 3. Qdrant 连接失败

**检查 Qdrant 服务:**
```bash
curl http://localhost:6333/health
```

**检查网络:**
```bash
docker compose exec context-keeper ping qdrant
```

### 4. 健康检查失败

**查看健康状态:**
```bash
docker compose ps
docker inspect context-keeper | grep -A 10 Health
```

**手动测试健康检查:**
```bash
curl http://localhost:8088/health
```

### 5. 日志查看

**实时日志:**
```bash
docker compose logs -f context-keeper
```

**查看最近 100 行:**
```bash
docker compose logs --tail=100 context-keeper
```

**查看特定时间段:**
```bash
docker compose logs --since 30m context-keeper
```

## 高级配置

### 启用 GPU 支持

编辑 `docker-compose.yml`，取消注释 GPU 配置：

```yaml
services:
  context-keeper:
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ['0']
              capabilities: [gpu]
```

### 启用监控服务

```bash
# 启动监控服务
docker compose --profile monitoring up -d

# 访问
# Prometheus: http://localhost:9090
# Grafana: http://localhost:3000
```

### 启用 Nginx 反向代理

```bash
# 启动代理服务
docker compose --profile proxy up -d

# 访问
# http://localhost:80
```

### 资源限制

创建 `docker-compose.override.yml`:

```yaml
version: '3.8'

services:
  context-keeper:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

## 生产环境部署

### 安全建议

1. **修改默认密钥**
   ```bash
   # 在 config/.env 中修改
   JWT_SECRET=your-production-secret-key-here
   ```

2. **限制 CORS**
   ```bash
   ALLOWED_ORIGINS=https://yourdomain.com
   ```

3. **启用 HTTPS**
   - 使用 Nginx 反向代理
   - 配置 SSL 证书

4. **定期备份数据**
   ```bash
   # 备份数据目录
   tar -czf backup-$(date +%Y%m%d).tar.gz ./data
   
   # 备份 Qdrant 数据
   docker compose exec qdrant tar -czf /tmp/qdrant-backup.tar.gz /qdrant/storage
   docker compose cp qdrant:/tmp/qdrant-backup.tar.gz ./qdrant-backup.tar.gz
   ```

### 性能优化

1. **调整日志级别**
   ```bash
   LOG_LEVEL=warn  # 生产环境使用 warn 或 error
   ```

2. **配置日志轮转**
   ```yaml
   logging:
     driver: "json-file"
     options:
       max-size: "100m"
       max-file: "3"
   ```

3. **优化 Qdrant 配置**
   - 根据数据量调整内存
   - 配置持久化策略

## 更新和维护

### 更新镜像

```bash
# 拉取最新代码
git pull

# 重新构建镜像
docker compose build --no-cache

# 重启服务
docker compose up -d
```

### 数据迁移

```bash
# 导出数据
docker compose exec context-keeper tar -czf /tmp/data-backup.tar.gz /app/data
docker compose cp context-keeper:/tmp/data-backup.tar.gz ./

# 导入数据
docker compose cp ./data-backup.tar.gz context-keeper:/tmp/
docker compose exec context-keeper tar -xzf /tmp/data-backup.tar.gz -C /
```

### 清理旧数据

```bash
# 清理旧日志
find ./data/logs -name "*.log" -mtime +30 -delete

# 清理 Docker 缓存
docker system prune -a
```

## 开发环境

### 本地开发模式

创建 `docker-compose.override.yml`:

```yaml
version: '3.8'

services:
  context-keeper:
    volumes:
      - ./cmd:/app/cmd:ro
      - ./internal:/app/internal:ro
      - ./pkg:/app/pkg:ro
    environment:
      - LOG_LEVEL=debug
      - DEBUG=true
```

### 调试

```bash
# 进入容器
docker compose exec context-keeper /bin/bash

# 查看进程
ps aux | grep context-keeper

# 查看环境变量
env | grep -E "(OLLAMA|QDRANT|JWT)"

# 测试网络连接
curl http://qdrant:6333/health
curl http://host.docker.internal:11434/api/tags
```

## 支持

如遇问题，请：

1. 查看日志：`docker compose logs -f`
2. 检查配置：`docker compose config`
3. 查看状态：`docker compose ps`
4. 提交 Issue 并附上日志信息
