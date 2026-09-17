# Context-Keeper 完全手动安装指南

## 📋 目录
1. [安装Docker](#1-安装docker)
2. [下载项目代码](#2-下载项目代码)
3. [配置环境变量](#3-配置环境变量)
4. [启动数据库服务](#4-启动数据库服务)
5. [初始化数据库](#5-初始化数据库)
6. [启动后端服务](#6-启动后端服务)
7. [启动可视化服务](#7-启动可视化服务)
8. [验证安装](#8-验证安装)
9. [插入测试数据](#9-插入测试数据)

---

## 1. 安装Docker

### Windows系统

**步骤1：下载Docker Desktop**
- 访问：https://www.docker.com/products/docker-desktop/
- 点击 "Download for Windows"
- 下载完成后双击安装包

**步骤2：安装**
- 勾选 "Use WSL 2 instead of Hyper-V"（推荐）
- 点击 "OK" 开始安装
- 安装完成后重启电脑

**步骤3：启动Docker**
- 打开 Docker Desktop
- 等待底部显示 "Docker Desktop is running"

**步骤4：验证安装**
打开命令提示符（CMD）或PowerShell，输入：
```cmd
docker --version
docker-compose --version
```

应该看到版本号，例如：
```
Docker version 24.0.6
Docker Compose version v2.23.0
```

### Linux系统（Ubuntu/Debian）

**步骤1：更新系统**
```bash
sudo apt-get update
sudo apt-get upgrade -y
```

**步骤2：安装Docker**
```bash
# 安装必要的工具
sudo apt-get install -y \
    ca-certificates \
    curl \
    gnupg \
    lsb-release

# 添加Docker官方GPG密钥
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg

# 添加Docker仓库
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# 安装Docker引擎
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

**步骤3：配置用户权限**
```bash
# 添加当前用户到docker组
sudo usermod -aG docker $USER

# 重新登录或运行
newgrp docker
```

**步骤4：启动Docker服务**
```bash
sudo systemctl start docker
sudo systemctl enable docker
```

**步骤5：验证安装**
```bash
docker --version
docker compose version
```

### macOS系统

**步骤1：下载Docker Desktop**
- 访问：https://www.docker.com/products/docker-desktop/
- 选择 "Download for Mac"
  - Intel芯片选择 "Mac with Intel chip"
  - Apple芯片选择 "Mac with Apple chip"

**步骤2：安装**
- 打开下载的 `.dmg` 文件
- 拖动Docker图标到Applications文件夹
- 打开Applications，双击Docker

**步骤3：验证安装**
打开终端（Terminal），输入：
```bash
docker --version
docker compose version
```

---

## 2. 下载项目代码

### 方法1：使用Git（推荐）

**步骤1：安装Git**

**Windows**:
- 下载：https://git-scm.com/download/win
- 双击安装，全部使用默认选项

**Linux**:
```bash
sudo apt-get install git
```

**macOS**:
```bash
brew install git
# 或者直接使用系统自带的git
```

**步骤2：克隆仓库**
```bash
# 替换<仓库地址>为实际地址
git clone <仓库地址> context-keeper
cd context-keeper
```

### 方法2：直接下载压缩包

1. 访问项目仓库页面
2. 点击 "Code" -> "Download ZIP"
3. 解压到任意目录
4. 进入解压后的目录

---

## 3. 配置环境变量

### Windows系统

**步骤1：创建.env文件**
```cmd
cd d:\context-keeper
copy .env.example .env
```

**步骤2：编辑.env文件**
用记事本打开 `.env` 文件：
```cmd
notepad .env
```

**步骤3：修改以下配置**

```bash
# ==================== 认证配置 ====================
# JWT密钥（必须修改！建议使用随机字符串）
JWT_SECRET=your-super-secret-key-change-this-12345

# Demo认证密码（登录用）
DEMO_AUTH_PASSWORD=demo123456

# ==================== Neo4j 图数据库 ====================
NEO4J_URI=bolt://neo4j:7687
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=neo4j_password_12345

# ==================== TimescaleDB 时序数据库 ====================
TIMESCALE_HOST=timescaledb
TIMESCALE_PORT=5432
TIMESCALE_USER=postgres
TIMESCALE_PASSWORD=timescale_password_12345
TIMESCALE_DB=context_keeper

# ==================== InfluxDB 配置（可选）====================
INFLUXDB_URL=http://influxdb:8086
INFLUXDB_TOKEN=my-super-secret-auth-token
INFLUXDB_ORG=context-keeper
INFLUXDB_BUCKET=vital_signs

# ==================== Ollama 本地LLM ====================
OLLAMA_HOST=http://host.docker.internal:11434
OLLAMA_MODEL=qwen2.5:7b

# ==================== 向量数据库 ====================
VECTOR_STORE_TYPE=qdrant
QDRANT_URL=http://qdrant:6333
QDRANT_API_KEY=

# ==================== 后端配置 ====================
PORT=8088
GIN_MODE=release
```

**重点修改项**：
- `JWT_SECRET`: 改成复杂的随机字符串
- `DEMO_AUTH_PASSWORD`: 登录密码
- `NEO4J_PASSWORD`: Neo4j数据库密码
- `TIMESCALE_PASSWORD`: TimescaleDB密码

保存并关闭文件。

### Linux/macOS系统

```bash
cd ~/context-keeper

# 复制配置文件
cp .env.example .env

# 编辑配置
nano .env
# 或者
vi .env
```

按照上面Windows的配置修改相应内容。

---

## 4. 启动数据库服务

### 步骤1：检查docker-compose.yml文件

确保项目根目录有 `docker-compose.yml` 文件：
```bash
ls docker-compose.yml
```

### 步骤2：逐个启动服务

**启动Neo4j图数据库**
```bash
docker-compose up -d neo4j
```

等待30秒，检查状态：
```bash
docker-compose ps neo4j
```

应该显示 `Up` 状态。

**启动TimescaleDB时序数据库**
```bash
docker-compose up -d timescaledb
```

等待20秒，检查状态：
```bash
docker-compose ps timescaledb
```

**启动Qdrant向量数据库**
```bash
docker-compose up -d qdrant
```

检查状态：
```bash
docker-compose ps qdrant
```

### 步骤3：验证数据库连接

**验证Neo4j**
- 访问：http://localhost:7474
- 应该看到Neo4j Browser登录界面
- 输入：
  - Connect URL: `bolt://localhost:7687`
  - Username: `neo4j`
  - Password: 你在`.env`中设置的`NEO4J_PASSWORD`

**验证Qdrant**
- 访问：http://localhost:6333/dashboard
- 应该看到Qdrant Dashboard

**验证TimescaleDB**
```bash
# Windows (PowerShell)
docker exec -it timescaledb psql -U postgres -d context_keeper -c "\dt"

# Linux/Mac
docker exec -it timescaledb psql -U postgres -d context_keeper -c '\dt'
```

应该显示数据表列表（或提示数据库为空）。

---

## 5. 初始化数据库

### 初始化Neo4j

**步骤1：访问Neo4j Browser**
http://localhost:7474

**步骤2：执行初始化脚本**
在查询框中逐条执行：

```cypher
// 创建实体名称索引
CREATE INDEX entity_name IF NOT EXISTS FOR (n:Entity) ON (n.name);

// 创建实体类型索引
CREATE INDEX entity_type IF NOT EXISTS FOR (n:Entity) ON (n.entity_type);

// 验证索引创建
SHOW INDEXES;
```

应该看到两个索引被创建。

### 初始化TimescaleDB

TimescaleDB会在启动时自动运行初始化脚本（如果存在 `init.sql`）。

**手动验证**：
```bash
docker exec -it timescaledb psql -U postgres -d context_keeper
```

在psql提示符下执行：
```sql
-- 查看已创建的表
\dt

-- 查看vital_signs表结构（如果存在）
\d vital_signs

-- 退出
\q
```

**如果表不存在，手动创建**：
```sql
CREATE TABLE IF NOT EXISTS vital_signs (
    id SERIAL PRIMARY KEY,
    resident_id VARCHAR(100) NOT NULL,
    type VARCHAR(50) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    value2 DOUBLE PRECISION,
    unit VARCHAR(20),
    room_number VARCHAR(20),
    recorded_by VARCHAR(100),
    notes TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 创建时序表（TimescaleDB特有）
SELECT create_hypertable('vital_signs', 'timestamp', if_not_exists => TRUE);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_vital_signs_resident ON vital_signs(resident_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_vital_signs_type ON vital_signs(type, timestamp DESC);
```

---

## 6. 启动后端服务

### 方法1：使用Docker（推荐）

```bash
docker-compose up -d context-keeper
```

**查看启动日志**：
```bash
docker-compose logs -f context-keeper
```

应该看到类似输出：
```
[GIN-debug] Listening and serving HTTP on :8088
[INFO] Server started successfully
```

按 `Ctrl+C` 退出日志查看（服务继续运行）。

### 方法2：本地编译运行（开发环境）

**步骤1：安装Go**
- 下载：https://go.dev/dl/
- 选择对应系统的安装包
- 安装后验证：
  ```bash
  go version
  ```

**步骤2：安装依赖**
```bash
cd context-keeper
go mod download
```

**步骤3：编译并运行**
```bash
# Windows
cd cmd\server
go build -o context-keeper.exe main_http.go
context-keeper.exe

# Linux/Mac
cd cmd/server
go build -o context-keeper main_http.go
./context-keeper
```

应该看到：
```
[GIN] Listening and serving HTTP on :8088
```

---

## 7. 启动可视化服务

### 步骤1：安装Python

**Windows**:
- 下载：https://www.python.org/downloads/
- 安装时勾选 "Add Python to PATH"

**Linux**:
```bash
sudo apt-get install python3 python3-pip
```

**macOS**:
```bash
brew install python3
```

**验证安装**：
```bash
python --version
# 或
python3 --version
```

### 步骤2：安装依赖

```bash
cd tools

# Windows
pip install flask flask-cors matplotlib

# Linux/Mac
pip3 install flask flask-cors matplotlib
```

### 步骤3：启动服务

**前台运行（查看日志）**：
```bash
# Windows
python visualization_service.py

# Linux/Mac
python3 visualization_service.py
```

应该看到：
```
[OK] Visualization Service starting on http://localhost:5001
[OK] API endpoint: POST http://localhost:5001/generate
```

**后台运行**：

**Linux/Mac**:
```bash
nohup python3 visualization_service.py > visualization.log 2>&1 &
echo $! > visualization.pid
```

**Windows**:
```cmd
start /b python visualization_service.py > visualization.log 2>&1
```

---

## 8. 验证安装

### 步骤1：检查所有服务状态

```bash
docker-compose ps
```

应该看到：
```
NAME                STATUS
context-keeper      Up
neo4j               Up (healthy)
timescaledb         Up (healthy)
qdrant              Up
```

### 步骤2：访问Web界面

打开浏览器，访问：**http://localhost:8088**

应该看到登录页面。

### 步骤3：登录系统

- 用户名：`doctor_wang`
- 密码：你在`.env`中设置的`DEMO_AUTH_PASSWORD`

### 步骤4：测试基础功能

登录后，在聊天框输入：
```
你好
```

应该收到AI回复。

---

## 9. 插入测试数据

### 方法1：使用Go脚本

**步骤1：获取JWT Token**
1. 登录系统 http://localhost:8088
2. 按 `F12` 打开开发者工具
3. 切换到 `Application` 标签
4. 展开 `Local Storage` -> `http://localhost:8088`
5. 找到 `token`，复制其值

**步骤2：编辑测试脚本**
```bash
cd tools
```

打开 `insert_test_data.go`，找到第61行：
```go
token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

替换为你复制的token。

**步骤3：运行脚本**
```bash
go run insert_test_data.go
```

应该看到：
```
开始插入张奶奶的血压测试数据...
✅ 插入成功
✅ 插入成功
...
✅ 所有数据插入完成！
```

### 方法2：使用Shell脚本

**Linux/Mac**:
```bash
cd tools

# 编辑脚本，替换TOKEN
nano insert_test_vitals.sh

# 修改第5行的TOKEN值
TOKEN="你的JWT-token"

# 运行脚本
chmod +x insert_test_vitals.sh
./insert_test_vitals.sh
```

### 方法3：手动使用curl

```bash
# 替换<YOUR_TOKEN>为实际token
curl -X POST http://localhost:8088/api/vital-signs \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <YOUR_TOKEN>" \
  -d '{
    "resident_id": "张奶奶",
    "type": "blood_pressure",
    "value": 136,
    "value2": 85,
    "unit": "mmHg",
    "room_number": "201",
    "recorded_by": "护士小王",
    "notes": "晨间测量"
  }'
```

### 验证数据插入

在聊天界面输入：
```
查询张奶奶最近7天的血压数据
```

应该显示血压数据并生成图表。

---

## 10. 常见问题处理

### 问题1：Docker容器无法启动

**检查日志**：
```bash
docker-compose logs <服务名>
```

**常见原因**：
- 端口被占用
- 配置文件错误
- 内存不足

### 问题2：Neo4j无法连接

**解决步骤**：
```bash
# 重启Neo4j
docker-compose restart neo4j

# 查看日志
docker-compose logs neo4j

# 等待完全启动（约30秒）
```

### 问题3：后端API返回500错误

**检查后端日志**：
```bash
docker-compose logs context-keeper | tail -50
```

**常见原因**：
- 数据库连接失败
- 环境变量配置错误
- JWT_SECRET未设置

### 问题4：可视化服务无法访问

**验证服务运行**：
```bash
curl http://localhost:5001/health
```

应该返回：
```json
{"status":"ok","service":"visualization-service"}
```

**如果失败**：
```bash
# 查看日志
cat tools/visualization.log

# 重新启动
cd tools
python3 visualization_service.py
```

---

## 11. 停止服务

### 停止所有Docker容器

```bash
docker-compose down
```

### 停止可视化服务

**找到进程**：
```bash
# Linux/Mac
cat tools/visualization.pid
kill $(cat tools/visualization.pid)

# Windows
tasklist | findstr python
taskkill /PID <进程ID> /F
```

---

## 12. 完全重置（慎用）

**删除所有数据和容器**：
```bash
docker-compose down -v
```

**注意**：这会删除所有数据库数据！

---

## ✅ 安装完成检查清单

- [ ] Docker已安装并运行
- [ ] 项目代码已下载
- [ ] `.env`文件已配置
- [ ] Neo4j容器运行中（http://localhost:7474）
- [ ] TimescaleDB容器运行中
- [ ] Qdrant容器运行中（http://localhost:6333/dashboard）
- [ ] 后端服务运行中（docker-compose ps显示Up）
- [ ] 可视化服务运行中（http://localhost:5001/health）
- [ ] Web界面可访问（http://localhost:8088）
- [ ] 能够成功登录
- [ ] 测试数据已插入
- [ ] 图表功能正常

---

安装完成！🎉

如有问题，查看各服务日志排查：
```bash
docker-compose logs -f
```
