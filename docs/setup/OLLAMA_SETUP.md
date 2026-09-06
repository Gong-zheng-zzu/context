# Ollama 本地部署指南

## 概述

健康助手默认使用 **Ollama 本地大模型服务**，实现完全本地化部署：
- ✅ **完全免费** - 无需API密钥，无使用限制
- ✅ **数据隐私** - 所有数据在本地处理，不上传云端
- ✅ **离线可用** - 无需网络连接即可使用
- ✅ **GPU加速** - 支持NVIDIA GPU加速（可选）

## 快速开始

### 1. 一键启动

运行启动脚本，会自动配置Ollama：

**Windows:**
```bash
start-health-assistant.bat
```

**Linux/Mac:**
```bash
./start-health-assistant.sh
```

首次启动会自动：
1. 启动Ollama服务容器
2. 下载 `qwen2.5:3b` 模型（约2GB，需要5-10分钟）
3. 配置健康助手连接到Ollama

### 2. 验证安装

```bash
# 检查Ollama服务状态
docker-compose ps ollama

# 查看已安装的模型
docker exec context-keeper-ollama ollama list

# 测试模型
docker exec context-keeper-ollama ollama run qwen2.5:3b "你好"
```

## 模型选择

### 推荐模型

| 模型 | 大小 | 内存需求 | 速度 | 质量 | 适用场景 |
|------|------|----------|------|------|----------|
| **qwen2.5:3b** | 2GB | 4GB | ⚡⚡⚡ | ⭐⭐⭐ | **默认推荐**，适合个人电脑 |
| qwen2.5:7b | 5GB | 8GB | ⚡⚡ | ⭐⭐⭐⭐ | 有GPU或高配电脑 |
| llama3.1:8b | 5GB | 8GB | ⚡⚡ | ⭐⭐⭐⭐ | 英文场景优秀 |
| llama2 | 4GB | 6GB | ⚡⚡ | ⭐⭐⭐ | 通用场景 |
| gemma2:2b | 1.6GB | 3GB | ⚡⚡⚡ | ⭐⭐ | 低配电脑 |

### 切换模型

1. **拉取新模型**
   ```bash
   docker exec context-keeper-ollama ollama pull qwen2.5:7b
   ```

2. **修改配置**
   编辑 `config/.env`：
   ```bash
   LLM_MODEL=qwen2.5:7b
   ```

3. **重启服务**
   ```bash
   docker-compose restart context-keeper
   ```

## 性能优化

### GPU加速（推荐）

如果你有NVIDIA GPU，Ollama会自动使用GPU加速，速度提升5-10倍。

**检查GPU是否启用：**
```bash
docker exec context-keeper-ollama nvidia-smi
```

如果看到GPU信息，说明GPU加速已启用。

### CPU模式

如果没有GPU，Ollama会使用CPU运行，速度较慢但完全可用。

**优化建议：**
- 使用较小的模型（如 qwen2.5:3b 或 gemma2:2b）
- 增加Docker内存限制（至少4GB）
- 关闭其他占用内存的程序

### 内存配置

在 `docker-compose.yml` 中调整Ollama内存限制：

```yaml
ollama:
  deploy:
    resources:
      limits:
        memory: 8G  # 根据你的电脑配置调整
      reservations:
        memory: 4G
```

## 常见问题

### 1. 模型下载很慢

**原因：** 网络问题或Ollama官方服务器慢

**解决：**
- 使用国内镜像（如果有）
- 或者手动下载模型文件后导入

### 2. 响应速度慢

**原因：** 模型太大或没有GPU加速

**解决：**
- 切换到更小的模型（qwen2.5:3b 或 gemma2:2b）
- 启用GPU加速
- 增加内存分配

### 3. 内存不足

**错误信息：** `OOM` 或 `out of memory`

**解决：**
```bash
# 使用更小的模型
docker exec context-keeper-ollama ollama pull gemma2:2b

# 修改 config/.env
LLM_MODEL=gemma2:2b

# 重启服务
docker-compose restart context-keeper
```

### 4. Ollama服务无法启动

**检查日志：**
```bash
docker-compose logs ollama
```

**常见原因：**
- 端口11434被占用
- Docker资源不足
- GPU驱动问题（如果使用GPU）

**解决：**
```bash
# 检查端口占用
netstat -ano | findstr 11434

# 增加Docker资源限制（Docker Desktop设置）
# 或禁用GPU（修改docker-compose.yml，注释掉GPU配置）
```

### 5. 切换回云端API

如果Ollama不适合你的电脑配置，可以切换回云端API：

编辑 `config/.env`：
```bash
LLM_PROVIDER=deepseek
LLM_MODEL=deepseek-chat
DEEPSEEK_API_KEY=your_api_key_here
```

重启服务：
```bash
docker-compose restart context-keeper
```

## 高级配置

### 自定义Ollama参数

在 `docker-compose.yml` 中添加环境变量：

```yaml
ollama:
  environment:
    - OLLAMA_NUM_PARALLEL=2        # 并发请求数
    - OLLAMA_MAX_LOADED_MODELS=1   # 同时加载的模型数
    - OLLAMA_KEEP_ALIVE=5m         # 模型保持加载时间
```

### 使用外部Ollama服务

如果你已经有运行中的Ollama服务：

1. **修改 config/.env**
   ```bash
   OLLAMA_HOST=http://your-ollama-host:11434
   ```

2. **禁用Docker中的Ollama服务**
   ```bash
   # 注释掉 docker-compose.yml 中的 ollama 服务
   ```

### 模型管理

```bash
# 查看所有模型
docker exec context-keeper-ollama ollama list

# 删除不用的模型（释放空间）
docker exec context-keeper-ollama ollama rm llama2

# 查看模型详情
docker exec context-keeper-ollama ollama show qwen2.5:3b

# 复制模型（创建自定义版本）
docker exec context-keeper-ollama ollama cp qwen2.5:3b my-custom-model
```

## 数据持久化

Ollama模型存储在Docker卷中，不会因容器重启而丢失：

```bash
# 查看卷信息
docker volume inspect context-keeper-main_ollama_data

# 备份模型数据
docker run --rm -v context-keeper-main_ollama_data:/data -v $(pwd):/backup alpine tar czf /backup/ollama-backup.tar.gz /data

# 恢复模型数据
docker run --rm -v context-keeper-main_ollama_data:/data -v $(pwd):/backup alpine tar xzf /backup/ollama-backup.tar.gz -C /
```

## 性能基准

在不同配置下的响应速度参考：

| 配置 | 模型 | 首次响应 | 平均速度 |
|------|------|----------|----------|
| RTX 3060 (12GB) | qwen2.5:7b | ~1s | 50 tokens/s |
| RTX 2050 (4GB) | qwen2.5:3b | ~2s | 30 tokens/s |
| CPU (i7-12700) | qwen2.5:3b | ~5s | 10 tokens/s |
| CPU (i5-10400) | gemma2:2b | ~3s | 15 tokens/s |

## 资源链接

- [Ollama官方文档](https://ollama.ai/docs)
- [Ollama模型库](https://ollama.ai/library)
- [Qwen2.5模型介绍](https://github.com/QwenLM/Qwen2.5)
- [Docker GPU支持](https://docs.docker.com/config/containers/resource_constraints/#gpu)

## 总结

使用Ollama本地部署的优势：
- ✅ 完全免费，无使用限制
- ✅ 数据隐私，不上传云端
- ✅ 离线可用，不依赖网络
- ✅ 适合信息安全大赛的隐私保护主题

如果遇到问题，请查看 [DOCKER_GUIDE.md](DOCKER_GUIDE.md) 或提交Issue。
