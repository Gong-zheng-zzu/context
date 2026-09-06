==========================================
   Context-Keeper 安装包
   AI Agent 安全治理平台
==========================================

【推荐安装流程】

1. 先尝试自动安装（最简单）：

   Windows用户：
   - 双击运行 quick-start.bat

   Linux/Mac用户：
   - 打开终端，进入项目目录
   - 运行：chmod +x quick-start.sh
   - 运行：./quick-start.sh

2. 如果自动安装失败，使用手动安装：
   - 打开 MANUAL_INSTALLATION.md
   - 按步骤一步步操作
   - 每一步都有详细说明和验证方法

3. 快速查看（5分钟速览）：
   - 阅读 QUICK_START.md

==========================================

【最低配置要求】

- CPU: 4核心
- 内存: 16GB（推荐32GB）
- 磁盘: 100GB可用空间
- 操作系统: Windows 10/11, Linux, macOS

【必须预装软件】

- Docker Desktop (Windows/Mac)
  下载: https://www.docker.com/products/docker-desktop/

- Docker Engine (Linux)
  安装: curl -fsSL https://get.docker.com | sh

==========================================

【预计安装时间】

- 首次安装: 20-30分钟
  （包括下载Docker镜像，约2-3GB）

- 二次安装: 5-10分钟
  （镜像已缓存）

- 自动脚本: 节省50%时间

==========================================

【关键步骤说明】

步骤1: 确保Docker Desktop已启动
  - Windows: 系统托盘看到Docker图标
  - 底部显示 "Docker Desktop is running"

步骤2: 配置环境变量 (config/.env)
  - 脚本会自动从.env.example复制
  - 必须修改 JWT_SECRET
  - 建议修改数据库密码

步骤3: 等待服务启动（约30秒）
  - Neo4j 图数据库
  - TimescaleDB 时序数据库
  - Qdrant 向量数据库
  - Context-Keeper 后端服务

步骤4: 访问系统
  - 打开浏览器: http://localhost:8088
  - 默认账号: doctor_wang
  - 默认密码: 查看 config/.env 文件

==========================================

【安装后验证】

1. 访问以下地址，确认服务正常：

   ✓ 主界面
     http://localhost:8088

   ✓ Neo4j Browser
     http://localhost:7474

   ✓ Qdrant Dashboard
     http://localhost:6333/dashboard

2. 插入测试数据：

   cd tools
   go run insert_test_data.go

3. 测试功能：

   在聊天框输入：
   "查询张奶奶最近7天的血压数据"

   应该显示血压记录并生成图表

==========================================

【常见问题】

Q1: Docker容器无法启动？
A1: 检查日志
    docker compose logs <服务名>
    常见原因：端口被占用、内存不足

Q2: 端口8088被占用？
A2: 修改 docker-compose.yml
    将 8088:8088 改为 8089:8088

Q3: Neo4j连接失败？
A3: 等待30秒完全启动
    docker compose restart neo4j

Q4: Ollama无法连接？
A4: 这是可选服务，可以跳过
    如需使用：https://ollama.com

Q5: 内存不足导致崩溃？
A5: 关闭不需要的服务
    docker compose stop qdrant

==========================================

【完整文档】

- QUICK_START.md
  5分钟快速安装指南

- MANUAL_INSTALLATION.md
  完全手动安装指南（672行详细步骤）

- INSTALLATION_GUIDE.md
  生产环境部署指南

==========================================

【技术支持】

1. 查看日志排查问题：
   docker compose logs -f

2. 查看服务状态：
   docker compose ps

3. 重启所有服务：
   docker compose restart

4. 完全重置（删除所有数据）：
   docker compose down -v
   （慎用！会删除数据库数据）

==========================================

【停止服务】

# 停止但保留数据
docker compose down

# 停止并删除所有数据（慎用）
docker compose down -v

==========================================

祝安装顺利！

如有问题，请查看对应的详细文档。

Context-Keeper Team
2026-07-12
==========================================
