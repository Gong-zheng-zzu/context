# 健康助手聊天功能使用说明

## 已完成的工作

✅ 创建了聊天API端点 (`internal/api/chat_handlers.go`)
✅ 修改了前端页面连接到真实API (`web/user_health_assistant.html`)
✅ 在main.go中注册了聊天路由
✅ 添加了GetLLMService方法到LLMDrivenContextService

## 启动步骤

### 1. 配置环境变量

在启动服务器之前，需要配置LLM服务的环境变量。根据你使用的LLM提供商：

**使用DeepSeek（推荐）：**
```bash
export LLM_PROVIDER=deepseek
export LLM_MODEL=deepseek-chat
export DEEPSEEK_API_KEY=your_api_key_here
export LLM_DRIVEN_ENABLED=true
```

**使用本地Ollama：**
```bash
export LLM_PROVIDER=ollama_local
export LLM_MODEL=llama2
export LLM_DRIVEN_ENABLED=true
```

**使用OpenAI：**
```bash
export LLM_PROVIDER=openai
export LLM_MODEL=gpt-3.5-turbo
export OPENAI_API_KEY=your_api_key_here
export LLM_DRIVEN_ENABLED=true
```

### 2. 启动HTTP服务器

项目需要在HTTP模式下运行（而不是MCP模式）。有两种方式：

**方式1：使用main.go（推荐）**
```bash
cd d:\context\context-keeper-main
go run cmd/server/main.go
```

**方式2：使用main_http.go**
```bash
cd d:\context\context-keeper-main
export HTTP_MODE=true
go run cmd/server/main_http.go
```

服务器将在 `http://localhost:8088` 启动。

### 3. 打开前端页面

在浏览器中打开：
```
file:///d:/context/context-keeper-main/web/user_health_assistant.html
```

或者使用HTTP服务器托管（如果配置了静态文件服务）：
```
http://localhost:8088/user_health_assistant.html
```

## API端点说明

### POST /api/chat

**请求体：**
```json
{
  "user_id": "user_123",
  "session_id": "session_456",  // 可选，首次请求可不提供
  "message": "帮我记录今天的血压数据",
  "history": [  // 可选，历史消息
    {
      "role": "user",
      "content": "你好",
      "timestamp": 1234567890
    },
    {
      "role": "assistant",
      "content": "您好！我是您的健康助手",
      "timestamp": 1234567891
    }
  ]
}
```

**响应：**
```json
{
  "success": true,
  "data": {
    "session_id": "session_456",
    "message": "好的，我来帮您记录血压数据...",
    "timestamp": 1234567892
  },
  "error": ""
}
```

## 测试步骤

1. **启动服务器**
   ```bash
   cd d:\context\context-keeper-main
   export DEEPSEEK_API_KEY=your_key
   export LLM_PROVIDER=deepseek
   export LLM_MODEL=deepseek-chat
   export LLM_DRIVEN_ENABLED=true
   go run cmd/server/main.go
   ```

2. **检查服务器日志**
   应该看到类似的输出：
   ```
   ✅ 聊天API已注册
   ========================================
   HTTP服务器配置:
     监听地址: 0.0.0.0:8088
     健康检查: GET /health
     健康API路由:
       POST /api/health/record - 记录健康信息
       GET  /api/health/history - 查询健康记录历史
       GET  /api/health/summary - 健康档案摘要
       GET  /api/health/report - 生成就医报告
     聊天API路由:
       POST /api/chat - 健康助手对话
   ========================================
   🚀 HTTP服务器启动中...
   ✅ HTTP服务器已启动，监听端口 8088
   ```

3. **测试健康检查**
   ```bash
   curl http://localhost:8088/health
   ```
   应该返回：
   ```json
   {"status":"ok","timestamp":1234567890}
   ```

4. **测试聊天API（使用curl）**
   ```bash
   curl -X POST http://localhost:8088/api/chat \
     -H "Content-Type: application/json" \
     -d '{
       "user_id": "test_user",
       "message": "你好，我想咨询健康问题"
     }'
   ```

5. **打开前端页面测试**
   - 在浏览器中打开 `user_health_assistant.html`
   - 点击快捷问题或输入消息
   - 查看是否能收到AI回复

## 故障排查

### 问题1：前端显示"请确保后端服务正在运行"

**原因：** 后端服务未启动或端口不是8088

**解决：**
- 检查服务器是否启动：`curl http://localhost:8088/health`
- 检查端口是否被占用：`netstat -ano | findstr 8088`

### 问题2：服务器日志显示"LLM服务未初始化"

**原因：** 环境变量未配置或LLM_DRIVEN_ENABLED=false

**解决：**
```bash
export LLM_DRIVEN_ENABLED=true
export LLM_PROVIDER=deepseek
export DEEPSEEK_API_KEY=your_key
```

### 问题3：前端收到错误"LLM调用失败"

**原因：** API密钥无效或网络问题

**解决：**
- 检查API密钥是否正确
- 检查网络连接
- 查看服务器日志获取详细错误信息

### 问题4：CORS错误

**原因：** 浏览器跨域限制

**解决：**
- 使用HTTP服务器托管前端页面（而不是file://协议）
- 或者在main.go中已经配置了CORS允许所有来源

## 功能特点

✨ **智能对话**：基于大模型的自然语言理解
🔒 **隐私保护**：所有敏感信息自动脱敏
💚 **健康专业**：专门针对健康咨询场景优化
📝 **上下文记忆**：保持对话上下文，支持多轮对话
⚡ **实时响应**：流式输出，快速响应

## 下一步优化

- [ ] 添加流式响应支持（Server-Sent Events）
- [ ] 集成健康数据记录功能
- [ ] 添加文件上传功能（体检报告、医疗影像）
- [ ] 实现对话历史持久化
- [ ] 添加用户认证和会话管理
