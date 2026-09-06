# Context-Keeper 功能演示指南

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                     Context-Keeper 系统                      │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  用户请求 → MCP 协议 → Context-Keeper 服务                   │
│                              ↓                                │
│                    Ollama (nomic-embed-text)                 │
│                    生成 768 维向量                            │
│                              ↓                                │
│                    Qdrant 向量数据库                          │
│                    存储 + 语义检索                            │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## 技术栈

- **后端框架**: Go + Gin
- **向量数据库**: Qdrant (本地部署)
- **Embedding 模型**: Ollama nomic-embed-text (768 维)
- **协议**: MCP (Model Context Protocol)
- **部署方式**: Docker Compose

## 快速测试

### 1. 健康检查

```bash
curl http://localhost:8088/health
```

**预期响应**:
```json
{"status":"healthy"}
```

### 2. 创建用户

```bash
curl -X POST http://localhost:8088/api/users \
  -H "Content-Type: application/json" \
  -d '{
    "userId":"demo_user_001",
    "userName":"演示用户",
    "email":"demo@example.com"
  }'
```

**预期响应**:
```json
{
  "success": true,
  "message": "用户新增成功",
  "userId": "demo_user_001",
  "data": {
    "userId": "demo_user_001",
    "createdAt": "2026-05-09T22:00:00+08:00",
    "firstUsed": "2026-05-09T22:00:00+08:00",
    "lastActive": "2026-05-09T22:00:00+08:00"
  }
}
```

### 3. 创建会话

```bash
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"tools/call",
    "params":{
      "name":"session_management",
      "arguments":{
        "action":"get_or_create",
        "userId":"demo_user_001",
        "workspaceRoot":"/demo/workspace"
      }
    }
  }'
```

**关键字段**: 返回的 `sessionId` 用于后续操作

### 4. 存储长期记忆（核心功能）

```bash
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":2,
    "method":"tools/call",
    "params":{
      "name":"memorize_context",
      "arguments":{
        "sessionId":"<你的会话ID>",
        "content":"这是一个智能上下文管理系统，使用 Go + Gin + Qdrant + Ollama 构建。"
      }
    }
  }'
```

**预期响应**:
```json
{
  "jsonrpc":"2.0",
  "id":2,
  "result":{
    "content":[{
      "text":"{\"success\":true,\"memoryId\":\"xxx\",\"message\":\"成功将内容存储到长期记忆\"}",
      "type":"text"
    }]
  }
}
```

### 5. 验证 Qdrant 存储

```bash
curl -X POST http://localhost:6333/collections/context_keeper/points/scroll \
  -H "Content-Type: application/json" \
  -d '{
    "limit": 5,
    "with_payload": true,
    "with_vector": false
  }'
```

**预期结果**: 可以看到存储的向量点，包含 payload 数据

### 6. 语义检索

```bash
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":3,
    "method":"tools/call",
    "params":{
      "name":"retrieve_context",
      "arguments":{
        "sessionId":"<你的会话ID>",
        "query":"这个系统使用什么技术栈？"
      }
    }
  }'
```

**预期结果**: 返回相关的历史记忆，包含相似度分数

## 核心功能验证清单

- [x] ✅ 服务健康检查
- [x] ✅ 用户管理（Memory 类型）
- [x] ✅ 会话管理
- [x] ✅ 长期记忆存储到 Qdrant
- [x] ✅ Ollama Embedding 生成（768 维向量）
- [x] ✅ 语义相似度检索
- [x] ✅ LLM 服务连接（Ollama）

## 系统配置

### 当前配置

- **向量存储类型**: `qdrant`
- **用户仓库类型**: `memory`
- **Qdrant 地址**: `http://qdrant:6333`
- **Ollama 地址**: `http://host.docker.internal:11434`
- **Embedding 模型**: `nomic-embed-text`
- **向量维度**: `768`
- **相似度度量**: `cosine`

### 配置文件位置

- 主配置: `config/.env`
- LLM 配置: `config/llm_config.yaml`
- Docker 编排: `docker-compose.yml`

## 访问地址

- **Context-Keeper API**: http://localhost:8088
- **健康检查**: http://localhost:8088/health
- **MCP 端点**: http://localhost:8088/mcp
- **Qdrant Dashboard**: http://localhost:6333/dashboard
- **Qdrant API**: http://localhost:6333

## 故障排查

### 检查容器状态
```bash
docker ps | grep context
```

### 查看日志
```bash
docker logs context-keeper --tail 50
docker logs context-keeper-qdrant --tail 50
```

### 检查 Ollama 服务
```bash
curl http://localhost:11434/api/tags
```

### 检查 Qdrant 集合
```bash
curl http://localhost:6333/collections/context_keeper
```

## 演示要点

1. **本地部署** - 无需云服务，完全本地运行
2. **向量存储** - 使用 Qdrant 进行高效向量检索
3. **语义理解** - Ollama 生成高质量 embedding
4. **MCP 协议** - 标准化的上下文管理协议
5. **Docker 部署** - 一键启动，易于维护

## 性能指标

- **Embedding 生成**: ~100-500ms（取决于文本长度）
- **向量检索**: <10ms（Qdrant）
- **API 响应**: <1s（端到端）
- **向量维度**: 768
- **相似度算法**: Cosine

---

**演示准备完成！** 🚀
