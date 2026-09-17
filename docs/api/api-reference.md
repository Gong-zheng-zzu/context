# Context-Keeper API参考文档

> **路由同步日期：2026-09-17**
>
> 本文档已对照 `cmd/server/main_http.go` 服务启动时的实际路由注册进行核验。
> 在死代码清理中，`internal/api/sse_handler.go` 以及 `internal/api/handlers.go` 中一批仅由已被注释的
> `handler.RegisterRoutes` 引用的处理器已被移除，因此部分历史端点不再注册。请以本文「当前启用的端点」及实现代码为准。

本文档提供Context-Keeper服务API端点的说明。

## 目录

- [当前启用的端点](#当前启用的端点)
- [公共端点](#公共端点)
- [已移除 / 未启用的端点](#已移除--未启用的端点)
- [API认证](#api认证)
- [错误处理](#错误处理)
- [数据模型](#数据模型)

## 当前启用的端点

以下端点由 `cmd/server/main_http.go` 的 `setupRoutesAndStartServer` 当前实际注册（核验日期 2026-09-17）。

### 公开端点（无需认证）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/` | 静态页面（`./web/role_selection.html`） |
| GET | `/web/*` | 静态资源（`./web`） |
| GET | `/temp/*` | 临时文件（`./data/temp`） |
| GET | `/health` | 健康检查 |
| POST | `/api/auth/login` | 账号登录 |
| POST | `/api/role/login` | 角色登录 |

### WebSocket 与连接管理（无需 JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/ws` | WebSocket 主连接端点 |
| GET | `/ws/status` | WebSocket 状态查询 |
| GET | `/ws/debug` | WebSocket 连接详情调试 |
| POST | `/api/ws/register-session` | 将会话注册到 WebSocket |

### 受保护端点（JWT 认证 + 速率限制）

| 方法 | 路径 | 说明 |
|------|------|------|
| * | `/api/chat/*` | 聊天相关路由（`RegisterChatRoutes`） |
| * | `/api/files/*` | 文件相关路由（`RegisterFileRoutes`） |
| POST | `/api/record` | 记录健康信息 |
| GET | `/api/history` | 查询健康记录历史 |
| GET | `/api/summary` | 健康档案摘要 |
| GET | `/api/report` | 生成就医报告 |
| POST | `/api/vital-signs` | 记录生命体征 |
| GET | `/api/vital-signs/history` | 查询生命体征历史 |
| GET | `/api/vital-signs/stats` | 生命体征统计（差分隐私） |
| POST | `/api/v1/nursing/records` | 护理记录写入 |
| POST | `/api/v1/agent/execute` | 受控 Agent（只读工具白名单） |
| POST | `/api/security/scan` | 安全扫描 |
| POST | `/api/security/detect` | 敏感信息检测 |
| POST | `/api/security/redact` | 敏感信息脱敏 |
| GET | `/api/security/stats` | 安全统计 |
| POST | `/api/v1/security/evaluate-input` | 安全输入链路评估 |
| POST | `/api/v1/security/ablation` | 安全消融评估 |
| POST | `/api/v1/experiments/threeway/evidence` | 三方结构化证据 |
| GET | `/api/sessions` | 查询会话列表 |
| GET | `/api/users/:userId/sessions` | 查询用户会话详情 |
| GET | `/api/dashboard` | 仪表盘 |
| GET | `/api/alerts` | 告警 |
| GET | `/api/recommendations` | 推荐 |
| POST | `/api/call-caregiver` | 呼叫护理员 |
| POST | `/api/family-message` | 家属消息 |
| * | `/api/v1/*` | 高级路由（因果推理、机器遗忘；依赖 Ollama/Neo4j，初始化失败时不注册） |

### MCP 端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| POST | `/mcp` | 否 | MCP Streamable HTTP 协议主端点 |
| GET | `/mcp/capabilities` | 否 | 能力查询 |
| POST | `/api/mcp/tools/retrieve_context` | JWT | 检索上下文工具 |
| POST | `/api/mcp/tools/programming_context` | JWT | 编程上下文工具 |
| POST | `/api/mcp/tools/associate_file` | JWT | 关联文件工具 |
| POST | `/api/mcp/tools/record_edit` | JWT | 记录编辑工具 |
| POST | `/api/mcp/tools/local_operation_callback` | JWT | 本地操作回调工具 |
| POST | `/api/mcp/tools/list` | JWT | 列出工具 |
| POST | `/api/mcp/tools/call` | JWT | 调用工具 |
| POST | `/mcp/tools/create_context` | JWT | 创建上下文（根级 MCP 路由） |
| POST | `/mcp/tools/read_context` | JWT | 读取上下文（根级 MCP 路由） |
| POST | `/mcp/tools/retrieve_context` | JWT | 检索上下文（根级 MCP 路由） |

### 管理与用户端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/management/sessions` | 查询所有会话（分页） |
| GET | `/management/users/:userId/sessions` | 查询用户会话详情 |
| POST | `/api/users` | 新增用户（唯一性校验） |
| PUT | `/api/users/:userId` | 变更用户信息 |
| GET | `/api/users/:userId` | 查询用户信息 |

> 注：批量 embedding 路由仅在 `BatchEmbeddingHandler` 已初始化时注册；session 删除路由见 `RegisterSessionDeletionRoutes`。

## 公共端点

### 健康检查

检查服务是否正常运行。

```
GET /health
```

#### 响应

```json
{
  "status": "healthy"
}
```

## 已移除 / 未启用的端点

以下端点曾在本文件中文档化，但对应的处理器在死代码清理中已被删除，或从未在服务启动时注册（核验日期 2026-09-17），因此当前不可用。

### Cursor 专用 API（未启用）

`internal/api/cursor_handlers.go` 中的 `CursorHandler.RegisterRoutes` 当前未被任何启动路径调用，因此 `/api/cursor/*` 组未注册。

| 方法 | 路径 | 状态 |
|------|------|------|
| POST | `/api/cursor/associateFile` | 未启用（处理器未注册） |
| POST | `/api/cursor/recordEdit` | 未启用（处理器未注册） |
| POST | `/api/cursor/retrieveContext` | 未启用（处理器未注册） |
| POST | `/api/cursor/programmingContext` | 未启用（处理器未注册） |

### Legacy MCP 标准 API（已移除）

以下 `/api/mcp/context-keeper/*` 端点对应的处理器已被移除。当前可用的 MCP 端点请见「当前启用的端点」中的 MCP 表。

| 方法 | 路径 | 状态 |
|------|------|------|
| POST | `/api/mcp/context-keeper/storeContext` | 已移除 |
| POST | `/api/mcp/context-keeper/retrieveContext` | 已移除 |
| POST | `/api/mcp/context-keeper/summarizeContext` | 已移除 |
| POST | `/api/mcp/context-keeper/storeMessages` | 已移除 |
| POST | `/api/mcp/context-keeper/retrieveConversation` | 已移除 |
| GET | `/api/mcp/context-keeper/sessionState` | 已移除 |
| POST | `/api/mcp/context-keeper/associateFile` | 已移除 |
| POST | `/api/mcp/context-keeper/recordEdit` | 已移除 |

### 集合管理 API（已移除）

`HandleListCollections` / `HandleCreateCollection` / `HandleGetCollection` / `HandleDeleteCollection` 已被移除，因此以下端点不再注册。

| 方法 | 路径 | 状态 |
|------|------|------|
| GET | `/api/collections` | 已移除 |
| POST | `/api/collections` | 已移除 |
| GET | `/api/collections/:name` | 已移除 |
| DELETE | `/api/collections/:name` | 已移除 |

## API认证

默认情况下，API没有启用认证。如启用认证（见配置文件），需在请求头中添加API密钥：

```
Authorization: Bearer your-api-key
```

部分受保护路由使用 JWT 认证（例如 `/api/chat/*`、`/api/files/*`、`/api/security/*` 以及 `/api/mcp/tools/*`）。

## 错误处理

所有API错误将返回适当的HTTP状态码和JSON格式的错误详情：

```json
{
  "error": "错误消息",
  "details": "详细错误信息",
  "code": "ERROR_CODE"
}
```

常见错误代码：

| 错误代码 | HTTP状态码 | 描述 |
|---------|-----------|------|
| SESSION_NOT_FOUND | 404 | 会话ID不存在 |
| INVALID_REQUEST | 400 | 请求参数无效 |
| UNAUTHORIZED | 401 | 认证失败 |
| INTERNAL_ERROR | 500 | 服务器内部错误 |
| VECTOR_DB_ERROR | 503 | 向量数据库服务错误 |
| EMBEDDING_API_ERROR | 503 | 嵌入API服务错误 |

## 数据模型

### 会话 (Session)

```json
{
  "id": "string",          // 会话ID
  "created": "timestamp",  // 创建时间
  "lastActive": "timestamp", // 最后活跃时间
  "status": "string"       // 状态(active/inactive)
}
```

### 文件 (File)

```json
{
  "sessionId": "string",   // 关联的会话ID
  "path": "string",        // 文件路径
  "language": "string",    // 编程语言
  "lastEdit": "timestamp", // 最后编辑时间
  "summary": "string"      // 文件摘要
}
```

### 编辑记录 (Edit)

```json
{
  "sessionId": "string",   // 关联的会话ID
  "filePath": "string",    // 文件路径
  "type": "string",        // 操作类型(insert/modify/delete)
  "position": "integer",   // 操作位置
  "content": "string",     // 编辑内容
  "timestamp": "timestamp" // 编辑时间
}
```

### 上下文 (Context)

```json
{
  "id": "string",          // 上下文ID
  "sessionId": "string",   // 关联的会话ID
  "content": "string",     // 上下文内容
  "type": "string",        // 内容类型
  "priority": "string",    // 优先级(P1/P2/P3)
  "metadata": "object",    // 元数据
  "timestamp": "timestamp" // 创建时间
}
```

### 消息 (Message)

```json
{
  "id": "string",          // 消息ID
  "sessionId": "string",   // 关联的会话ID
  "role": "string",        // 角色(user/assistant/system)
  "content": "string",     // 消息内容
  "contentType": "string", // 内容类型(text/code/image)
  "priority": "string",    // 优先级(P1/P2/P3)
  "timestamp": "timestamp" // 创建时间
}
```

---

有关API使用的更多示例，请参考[用法示例](examples.md)文档。
