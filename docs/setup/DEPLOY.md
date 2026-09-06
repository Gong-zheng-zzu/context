# Context-Keeper 云服务器部署指南

## 📋 准备工作

### 1. 服务器要求
- **系统**: Ubuntu 20.04+ / CentOS 7+
- **配置**: 最低 2核4G，推荐 4核8G
- **端口**: 需要开放 8088（HTTP API）
- **Docker**: 需要安装 Docker 和 Docker Compose

### 2. 云服务商选择
- 阿里云 ECS
- 腾讯云 CVM
- 华为云 ECS

---

## 🚀 部署步骤

### 步骤 1: 购买并配置服务器

1. **购买云服务器**
   - 选择 Ubuntu 20.04 系统
   - 配置安全组，开放端口：
     - 8088 (Context-Keeper HTTP API)
     - 22 (SSH)

2. **登录服务器**
   ```bash
   ssh root@你的服务器IP
   ```

### 步骤 2: 安装 Docker

```bash
# 更新系统
apt update && apt upgrade -y

# 安装 Docker
curl -fsSL https://get.docker.com | bash

# 启动 Docker
systemctl start docker
systemctl enable docker

# 安装 Docker Compose
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# 验证安装
docker --version
docker-compose --version
```

### 步骤 3: 上传项目文件

**方法 1: 使用 Git（推荐）**
```bash
cd /opt
git clone https://github.com/your-repo/context-keeper.git
cd context-keeper
```

**方法 2: 使用 SCP 上传**
```bash
# 在本地执行
scp -r d:/context/context-keeper-main root@你的服务器IP:/opt/context-keeper
```

### 步骤 4: 配置环境变量

```bash
cd /opt/context-keeper
cp config/.env.example config/.env
nano config/.env
```

编辑配置文件：
```bash
# 基础配置
PORT=8088
TRANSPORT_MODE=http
NODE_ENV=production

# 数据库配置
TIMESCALE_HOST=timescaledb
TIMESCALE_PORT=5432
TIMESCALE_USER=postgres
TIMESCALE_PASSWORD=your_secure_password_here
TIMESCALE_DB=context_keeper

NEO4J_URI=bolt://neo4j:7687
NEO4J_USER=neo4j
NEO4J_PASSWORD=your_secure_password_here

# 向量数据库配置（阿里云 DashVector）
VECTOR_STORE_TYPE=aliyun
EMBEDDING_API_KEY=your_dashscope_api_key
VECTOR_DB_API_KEY=your_dashvector_api_key

# 安全配置
ALLOWED_ORIGINS=*  # 生产环境建议设置具体域名
```

### 步骤 5: 启动服务

```bash
cd /opt/context-keeper

# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f context-keeper
```

### 步骤 6: 验证部署

```bash
# 健康检查
curl http://localhost:8088/health

# 测试 MCP 端点
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

---

## 🔒 安全加固

### 1. 配置防火墙

```bash
# 安装 UFW
apt install ufw -y

# 配置规则
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 8088/tcp

# 启用防火墙
ufw enable
ufw status
```

### 2. 配置 Nginx 反向代理（可选）

```bash
# 安装 Nginx
apt install nginx -y

# 创建配置文件
nano /etc/nginx/sites-available/context-keeper
```

Nginx 配置：
```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8088;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

```bash
# 启用配置
ln -s /etc/nginx/sites-available/context-keeper /etc/nginx/sites-enabled/
nginx -t
systemctl restart nginx
```

### 3. 配置 SSL 证书（推荐）

```bash
# 安装 Certbot
apt install certbot python3-certbot-nginx -y

# 获取证书
certbot --nginx -d your-domain.com

# 自动续期
certbot renew --dry-run
```

---

## 📊 监控和维护

### 查看日志
```bash
# 实时日志
docker-compose logs -f

# 查看特定服务
docker-compose logs -f context-keeper

# 查看最近 100 行
docker-compose logs --tail=100 context-keeper
```

### 重启服务
```bash
# 重启所有服务
docker-compose restart

# 重启特定服务
docker-compose restart context-keeper
```

### 更新服务
```bash
cd /opt/context-keeper

# 拉取最新代码
git pull

# 重新构建并启动
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### 备份数据
```bash
# 备份会话文件
tar -czf backup-sessions-$(date +%Y%m%d).tar.gz data/sessions/

# 备份数据库
docker exec timescaledb pg_dump -U postgres context_keeper > backup-db-$(date +%Y%m%d).sql
```

---

## 🌐 客户端配置

### 团队成员使用方式

**方法 1: HTTP API**
```bash
# 设置环境变量
export CONTEXT_KEEPER_URL=http://your-server-ip:8088

# 创建会话
curl -X POST $CONTEXT_KEEPER_URL/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"tools/call",
    "params":{
      "name":"session_management",
      "arguments":{
        "action":"get_or_create",
        "userId":"user-name",
        "workspaceRoot":"/workspace/project"
      }
    }
  }'
```

**方法 2: MCP 客户端配置**

在 Cursor/VSCode 的 MCP 配置中：
```json
{
  "mcpServers": {
    "context-keeper": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-http-client", "http://your-server-ip:8088/mcp"]
    }
  }
}
```

---

## 🔧 故障排查

### 服务无法启动
```bash
# 查看详细日志
docker-compose logs context-keeper

# 检查端口占用
netstat -tlnp | grep 8088

# 检查 Docker 状态
docker ps -a
```

### 数据库连接失败
```bash
# 检查数据库容器
docker-compose ps timescaledb

# 进入数据库容器
docker exec -it timescaledb psql -U postgres

# 测试连接
\l
\c context_keeper
\dt
```

### 性能问题
```bash
# 查看资源使用
docker stats

# 查看磁盘空间
df -h

# 清理 Docker 缓存
docker system prune -a
```

---

## 📈 性能优化

### 1. 调整 Docker 资源限制

编辑 `docker-compose.yml`:
```yaml
services:
  context-keeper:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G
```

### 2. 配置数据库连接池

在 `config/.env` 中：
```bash
DB_POOL_MIN=5
DB_POOL_MAX=20
```

### 3. 启用缓存

```bash
CACHE_ENABLED=true
CACHE_TTL=3600
```

---

## 💰 成本估算

### 阿里云 ECS 参考价格
- **入门配置** (2核4G): ¥70-100/月
- **推荐配置** (4核8G): ¥200-300/月
- **流量费用**: ¥0.8/GB（按实际使用）

### 其他费用
- **域名**: ¥50-100/年
- **SSL 证书**: 免费（Let's Encrypt）
- **备份存储**: ¥0.12/GB/月

---

## 📞 技术支持

遇到问题？
1. 查看日志: `docker-compose logs -f`
2. 检查配置: `cat config/.env`
3. 验证网络: `curl http://localhost:8088/health`
4. 查看文档: https://github.com/your-repo/context-keeper

---

## ✅ 部署检查清单

- [ ] 服务器已购买并配置
- [ ] Docker 和 Docker Compose 已安装
- [ ] 项目文件已上传
- [ ] 环境变量已配置
- [ ] 服务已启动并运行
- [ ] 健康检查通过
- [ ] 防火墙规则已配置
- [ ] （可选）Nginx 反向代理已配置
- [ ] （可选）SSL 证书已配置
- [ ] 备份策略已设置
- [ ] 团队成员已配置客户端

---

**部署完成后，你的 Context-Keeper 服务将可以通过以下地址访问：**
- HTTP API: `http://your-server-ip:8088`
- 健康检查: `http://your-server-ip:8088/health`
- MCP 端点: `http://your-server-ip:8088/mcp`
