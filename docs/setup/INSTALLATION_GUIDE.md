# Context-Keeper 系统安装部署指南

## 📋 系统概述

Context-Keeper 是一个AI Agent安全治理平台，包含以下核心功能：
- 多维检索（向量+图谱+时序）
- PCCM因果推理
- 机器遗忘算法
- 生命体征数据可视化

---

## 🖥️ 系统要求

### 最低配置
- **CPU**: 4核以上
- **内存**: 16GB（推荐32GB）
- **存储**: 100GB可用空间（SSD推荐）
- **操作系统**: Windows 10/11, Linux, macOS

### 软件依赖
- Docker Desktop 或 Docker Engine
- Docker Compose
- Git
- Python 3.8+
- Go 1.22+（如需开发）

---

## 📦 安装步骤

### 第一步：安装Docker

#### Windows:
1. 下载并安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)
2. 启动Docker Desktop，等待完全启动
3. 验证安装：
```bash
docker --version
docker-compose --version
```

#### Linux (Ubuntu/Debian):
```bash
# 安装Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# 安装Docker Compose
sudo apt-get install docker-compose-plugin

# 添加当前用户到docker组
sudo usermod -aG docker $USER
# 重新登录使权限生效

# 验证安装
docker --version
docker compose version
```

---

### 第二步：克隆项目代码

```bash
# 克隆仓库（替换为你的实际仓库地址）
git clone <你的仓库地址> context-keeper
cd context-keeper
```

---

### 第三步：配置环境变量

创建 `.env` 文件（复制示例配置）：

```bash
# Linux/Mac
cp .env.example .env

# Windows
copy .env.example .env
```

**编辑 `.env` 文件**，修改以下关键配置：

```bash
# JWT密钥（必须修改！）
JWT_SECRET=your-secret-key-change-this-in-production

# Demo认证密码（可选，用于演示登录）
DEMO_AUTH_PASSWORD=demo123

# Neo4j图数据库配置
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=your-neo4j-password

# TimescaleDB配置
TIMESCALE_HOST=timescaledb
TIMESCALE_PORT=5432
TIMESCALE_USER=postgres
TIMESCALE_PASSWORD=your-timescale-password
TIMESCALE_DB=context_keeper

# InfluxDB配置（可选，用于时序数据）
INFLUXDB_URL=http://influxdb:8086
INFLUXDB_TOKEN=your-influxdb-token
INFLUXDB_ORG=context-keeper
INFLUXDB_BUCKET=vital_signs

# Ollama本地LLM配置
OLLAMA_HOST=http://host.docker.internal:11434
OLLAMA_MODEL=qwen2.5:7b

# 向量数据库配置（选择一种）
VECTOR_STORE_TYPE=qdrant  # 可选: qdrant, vearch, aliyun
QDRANT_URL=http://qdrant:6333
```

---

### 第四步：启动基础服务

使用Docker Compose启动所有服务：

```bash
# 启动所有容器（首次启动会下载镜像，需要一些时间）
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f context-keeper
```

**预期输出**（所有服务应该是 `Up` 状态）：
```
NAME                  STATUS
context-keeper        Up
neo4j                 Up (healthy)
timescaledb           Up (healthy)
qdrant                Up
```

---

### 第五步：初始化数据库

#### 1. 初始化Neo4j图数据库

访问 Neo4j Browser: http://localhost:7474
- 用户名: `neo4j`
- 密码: 你在`.env`中设置的密码

执行初始化脚本：
```cypher
// 创建索引
CREATE INDEX entity_name IF NOT EXISTS FOR (n:Entity) ON (n.name);
CREATE INDEX entity_type IF NOT EXISTS FOR (n:Entity) ON (n.entity_type);
```

#### 2. 初始化TimescaleDB

```bash
# 进入TimescaleDB容器
docker exec -it timescaledb psql -U postgres -d context_keeper

# 创建时序表（已在docker-compose中自动执行）
# 如需手动执行，运行：
\i /docker-entrypoint-initdb.d/init.sql
```

---

### 第六步：安装Ollama并下载模型（本地LLM）

**如果你的电脑内存 < 16GB，建议跳过此步，使用云端LLM**

#### Windows/Mac:
1. 下载安装 [Ollama](https://ollama.com/download)
2. 安装后自动启动服务
3. 下载Qwen2.5模型：
```bash
ollama pull qwen2.5:7b
```

#### Linux:
```bash
# 安装Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 下载模型
ollama pull qwen2.5:7b

# 验证模型
ollama list
```

**验证Ollama服务**：
```bash
curl http://localhost:11434/api/tags
```

---

### 第七步：启动可视化服务

```bash
# 进入tools目录
cd tools

# 安装Python依赖
pip install flask flask-cors matplotlib

# 启动可视化服务
python visualization_service.py
```

**预期输出**：
```
[OK] Visualization Service starting on http://localhost:5001
[OK] API endpoint: POST http://localhost:5001/generate
```

---

### 第八步：访问系统

#### 1. 打开Web界面
浏览器访问: **http://localhost:8088**

#### 2. 登录系统
默认演示账号：
- 用户名: `doctor_wang`
- 密码: 你在`.env`中设置的 `DEMO_AUTH_PASSWORD`

#### 3. 测试功能
- 在聊天框输入: "查询张奶奶最近的血压数据"
- 系统应该返回血压记录并生成图表

---

## 🧪 插入测试数据

为了测试可视化功能，插入一些生命体征数据：

```bash
# 方法1：使用Go脚本（推荐）
cd tools
go run insert_test_data.go

# 方法2：使用Shell脚本
cd tools
# 先编辑 insert_test_vitals.sh，替换JWT token
# 获取token: 登录系统后，打开浏览器开发者工具 -> Application -> Local Storage -> 复制 token
bash insert_test_vitals.sh
```

**如何获取JWT Token**：
1. 登录系统 http://localhost:8088
2. 打开浏览器开发者工具（F12）
3. 切换到 `Application` 标签
4. 展开 `Local Storage` -> `http://localhost:8088`
5. 复制 `token` 的值

---

## 🔧 常见问题排查

### 问题1：Docker容器启动失败

**症状**：`docker-compose ps` 显示某个服务 `Exit 1`

**解决**：
```bash
# 查看详细错误日志
docker-compose logs <服务名>

# 例如：
docker-compose logs context-keeper
docker-compose logs neo4j

# 重启特定服务
docker-compose restart <服务名>
```

### 问题2：后端API返回 "大数据服务未初始化"

**原因**：InfluxDB/TimescaleDB服务未正确初始化

**解决**：
```bash
# 检查TimescaleDB是否运行
docker-compose ps timescaledb

# 重启后端服务
docker-compose restart context-keeper

# 查看后端日志确认初始化
docker-compose logs -f context-keeper | grep "大数据"
```

### 问题3：Ollama连接失败

**症状**：因果推理功能报错 "LLM服务不可用"

**解决**：
```bash
# 验证Ollama服务
curl http://localhost:11434/api/tags

# 如果不可访问，检查Ollama是否启动
# Windows: 查看任务管理器是否有Ollama进程
# Linux: systemctl status ollama

# 验证模型已下载
ollama list
```

### 问题4：端口冲突

**症状**：`docker-compose up` 报错 "port is already allocated"

**解决**：
```bash
# 查看哪个进程占用端口
# Windows:
netstat -ano | findstr :8088

# Linux/Mac:
lsof -i :8088

# 修改 docker-compose.yml 中的端口映射
# 例如将 8088:8088 改为 8089:8088
```

### 问题5：内存不足

**症状**：服务频繁崩溃，Docker显示 OOMKilled

**解决**：
```bash
# 限制服务内存使用（编辑docker-compose.yml）
services:
  neo4j:
    mem_limit: 2g
  qdrant:
    mem_limit: 2g
  context-keeper:
    mem_limit: 1g

# 或者关闭不需要的服务
docker-compose stop qdrant  # 如果不使用向量检索
```

---

## 🚀 生产环境部署建议

### 1. 使用云端数据库（推荐）

**如果你的电脑配置不够，建议将重量级服务迁移到云端**：

```bash
# .env 配置示例（使用阿里云服务）
NEO4J_URI=bolt://your-cloud-neo4j.aliyun.com:7687
TIMESCALE_HOST=your-rds.aliyun.com
QDRANT_URL=http://your-cloud-qdrant.com:6333
```

### 2. 安全加固

```bash
# 1. 修改所有默认密码
# 2. 启用HTTPS
# 3. 限制访问IP
# 4. 定期备份数据
```

### 3. 性能优化

```bash
# docker-compose.yml 添加资源限制
services:
  context-keeper:
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 4G
```

---

## 📞 技术支持

遇到问题？尝试以下方式：

1. **查看日志**：`docker-compose logs -f`
2. **检查服务状态**：`docker-compose ps`
3. **重启所有服务**：`docker-compose restart`
4. **完全重建**：
   ```bash
   docker-compose down -v  # 删除所有数据！慎用！
   docker-compose up -d --build
   ```

---

## 📊 系统架构图

```
┌─────────────────────────────────────────┐
│         用户浏览器 (localhost:8088)      │
└────────────────┬────────────────────────┘
                 ↓
┌────────────────────────────────────────┐
│    Go后端服务 (context-keeper:8088)    │
│  - JWT认证                              │
│  - 多维检索                             │
│  - 因果推理                             │
│  - 机器遗忘                             │
└─┬──────┬──────┬──────┬──────┬──────────┘
  ↓      ↓      ↓      ↓      ↓
┌───┐  ┌───┐  ┌────┐ ┌────┐ ┌─────┐
│Neo4j│ │Qdrant│ │TimeDB│ │Ollama│ │Viz│
│图谱│ │向量│  │时序│  │LLM│  │可视化│
└───┘  └───┘  └────┘ └────┘ └─────┘
7474   6333    5432   11434  5001
```

---

## ✅ 安装检查清单

- [ ] Docker已安装并运行
- [ ] 代码已克隆到本地
- [ ] `.env`文件已配置
- [ ] `docker-compose up -d` 启动成功
- [ ] 所有容器状态为 `Up`
- [ ] Neo4j可访问 (http://localhost:7474)
- [ ] Ollama已安装并下载模型（可选）
- [ ] 可视化服务已启动 (http://localhost:5001/health)
- [ ] Web界面可访问 (http://localhost:8088)
- [ ] 测试数据已插入
- [ ] 聊天功能正常
- [ ] 图表生成正常

---

## 🎓 下一步

系统安装完成后，建议：
1. 阅读 `README.md` 了解功能详情
2. 查看 `docs/API.md` 学习API使用
3. 运行测试脚本验证功能
4. 根据实际需求调整配置

祝使用愉快！🎉
