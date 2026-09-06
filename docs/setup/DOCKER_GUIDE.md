# 健康助手 - Docker 部署指南

## 快速开始

### 1. 启动服务（一键启动）

**Windows用户：**
```bash
start-health-assistant.bat
```

**Linux/Mac用户：**
```bash
chmod +x start-health-assistant.sh
./start-health-assistant.sh
```

启动脚本会自动：
- ✅ 创建默认配置文件（如果不存在）
- ✅ 启动Ollama本地大模型服务
- ✅ 自动下载qwen2.5:3b模型（约2GB）
- ✅ 启动健康助手后端服务
- ✅ 启动Qdrant向量数据库

**首次启动需要10-15分钟**（下载模型），后续启动只需几秒钟。

### 2. 访问前端

在浏览器中打开：
```
http://localhost:8088/user_health_assistant.html
```

或者直接打开本地文件：
```
file:///d:/context/context-keeper-main/web/user_health_assistant.html
```

## 手动启动（高级）

### 构建镜像
```bash
docker-compose build
```

### 启动服务
```bash
docker-compose up -d
```

### 查看日志
```bash
docker-compose logs -f context-keeper
```

### 停止服务
```bash
docker-compose down
```

### 重启服务
```bash
docker-compose restart
```

## 配置说明

### 默认配置（本地部署，无需API密钥）

启动脚本会自动创建 `config/.env` 文件，默认使用本地Ollama：

```bash
# LLM配置
LLM_DRIVEN_ENABLED=true          # 启用LLM驱动
LLM_PROVIDER=ollama_local        # 使用本地Ollama
LLM_MODEL=qwen2.5:3b             # 使用qwen2.5:3b模型（轻量级，适合个人电脑）

# 服务器配置
RUN_MODE=http                    # 运行模式：http
HTTP_SERVER_PORT=8088            # 服务器端口
HOST_PORT=8088                   # 主机映射端口

# 日志配置
LOG_LEVEL=info                   # 日志级别：debug, info, warn, error
CONTEXT_KEEPER_LOG_TO_STDOUT=true  # 日志输出到标准输出

# 时区
TZ=Asia/Shanghai                 # 时区设置
```

### 切换到云端API（可选）

如果你想使用DeepSeek等云端API而不是本地Ollama：

编辑 `config/.env` 文件：
```bash
LLM_PROVIDER=deepseek
LLM_MODEL=deepseek-chat
DEEPSEEK_API_KEY=your_actual_api_key_here
```

然后重启服务：
```bash
docker-compose restart context-keeper
```

## 使用本地Ollama

### 默认已启用

启动脚本默认启用Ollama本地大模型服务，无需额外配置。

### 查看Ollama状态

```bash
# 查看已安装的模型
docker exec context-keeper-ollama ollama list

# 查看Ollama日志
docker-compose logs -f ollama
```

### 切换其他模型

如果想使用其他Ollama模型：

1. **拉取模型**
   ```bash
   # 拉取llama2模型（约4GB）
   docker exec context-keeper-ollama ollama pull llama2
   
   # 拉取qwen2.5:7b模型（更强大，约5GB）
   docker exec context-keeper-ollama ollama pull qwen2.5:7b
   ```

2. **修改config/.env**
   ```bash
   LLM_MODEL=llama2
   # 或
   LLM_MODEL=qwen2.5:7b
   ```

3. **重启服务**
   ```bash
   docker-compose restart context-keeper
   ```

### 推荐模型

| 模型 | 大小 | 速度 | 质量 | 适用场景 |
|------|------|------|------|----------|
| qwen2.5:3b | ~2GB | 快 | 良好 | 个人电脑，快速响应 |
| qwen2.5:7b | ~5GB | 中等 | 优秀 | 有GPU或高配电脑 |
| llama2 | ~4GB | 中等 | 良好 | 通用场景 |
| llama3.1:8b | ~5GB | 中等 | 优秀 | 英文场景 |

## 测试API

### 健康检查
```bash
curl http://localhost:8088/health
```

### 测试聊天
```bash
curl -X POST http://localhost:8088/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "message": "你好，我想咨询健康问题"
  }'
```

## 故障排查

### 问题1：容器无法启动

**检查日志：**
```bash
docker-compose logs context-keeper
```

**常见原因：**
- API密钥未配置或无效
- 端口8088被占用
- Docker资源不足

### 问题2：前端无法连接后端

**检查服务状态：**
```bash
docker-compose ps
```

**检查健康检查：**
```bash
curl http://localhost:8088/health
```

**检查防火墙：**
确保端口8088未被防火墙阻止

### 问题3：LLM调用失败

**检查环境变量：**
```bash
docker-compose exec context-keeper env | grep LLM
```

**检查API密钥：**
确保DEEPSEEK_API_KEY正确设置

**查看详细日志：**
```bash
docker-compose logs -f context-keeper | grep LLM
```

## 数据持久化

数据存储在以下目录：
- `./data` - 应用数据
- `./logs` - 日志文件
- `./config` - 配置文件

这些目录会自动挂载到容器中，数据不会因容器重启而丢失。

## 性能优化

### 1. 调整资源限制

在docker-compose.yml中添加：
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

### 2. 使用GPU加速

如果有NVIDIA GPU，取消docker-compose.yml中GPU配置的注释：
```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          device_ids: ['0']
          capabilities: [gpu]
```

## 生产部署建议

1. **使用环境变量文件**
   - 不要将API密钥提交到Git
   - 使用`.env.example`作为模板

2. **配置反向代理**
   - 使用Nginx进行SSL终止
   - 启用HTTPS

3. **监控和日志**
   - 配置日志轮转
   - 使用Prometheus + Grafana监控

4. **备份数据**
   - 定期备份`./data`目录
   - 备份配置文件

## 更新服务

### 拉取最新代码
```bash
git pull
```

### 重新构建并启动
```bash
docker-compose down
docker-compose up -d --build
```

## 卸载

### 停止并删除容器
```bash
docker-compose down
```

### 删除数据（可选）
```bash
rm -rf ./data ./logs
```

### 删除镜像（可选）
```bash
docker rmi context-keeper:latest
```

## 支持的LLM提供商

| 提供商 | 环境变量 | 说明 | 推荐度 |
|--------|----------|------|--------|
| Ollama (本地) | 无需密钥 | **默认选项**，本地部署，完全免费，数据不出本地 | ⭐⭐⭐⭐⭐ |
| DeepSeek | `DEEPSEEK_API_KEY` | 性价比高，云端API | ⭐⭐⭐⭐ |
| OpenAI | `OPENAI_API_KEY` | GPT-3.5/4，质量最高但价格贵 | ⭐⭐⭐ |
| 通义千问 | `QIANWEN_API_KEY` | 阿里云 | ⭐⭐⭐ |

## 常用命令

```bash
# 查看运行状态
docker-compose ps

# 查看实时日志
docker-compose logs -f

# 查看特定服务日志
docker-compose logs -f context-keeper  # 健康助手服务
docker-compose logs -f ollama          # Ollama服务
docker-compose logs -f qdrant          # 向量数据库

# 进入容器
docker-compose exec context-keeper sh
docker exec -it context-keeper-ollama bash

# 重启服务
docker-compose restart

# 重启特定服务
docker-compose restart context-keeper
docker-compose restart ollama

# 查看资源使用
docker stats context-keeper
docker stats context-keeper-ollama

# 清理未使用的镜像
docker system prune -a

# Ollama相关命令
docker exec context-keeper-ollama ollama list           # 查看已安装模型
docker exec context-keeper-ollama ollama pull llama2   # 拉取新模型
docker exec context-keeper-ollama ollama rm llama2     # 删除模型
```

## 联系支持

如有问题，请查看：
- 项目文档：`CHAT_SETUP.md`
- 日志文件：`./logs/`
- GitHub Issues
