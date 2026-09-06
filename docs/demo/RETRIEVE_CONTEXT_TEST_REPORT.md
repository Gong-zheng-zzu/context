# retrieve_context API 测试报告

## 测试概述

测试目标：调用 retrieve_context API 并记录完整的响应数据结构，特别关注 `result.content[0].text` 字段的内容格式。

测试时间：2026-05-10 11:26:33
测试环境：Context-Keeper 本地部署（Docker + Qdrant + Ollama）

## API 调用方式

### 端点
- URL: `http://localhost:8088/mcp`
- 方法: POST
- 协议: MCP (Model Context Protocol)

### 请求格式

```json
{
  "jsonrpc": "2.0",
  "id": 100,
  "method": "tools/call",
  "params": {
    "name": "retrieve_context",
    "arguments": {
      "sessionId": "session-20260510-112633-f2a9949f-ws_6a40be0d0584f974",
      "query": "技术栈",
      "limit": 5
    }
  }
}
```

## 响应数据结构

### 完整响应 JSON

```json
{
  "jsonrpc": "2.0",
  "id": 100,
  "result": {
    "content": [
      {
        "text": "{\"longTermMemory\":\"【相关历史】\\n无相关内容\",\"relevantKnowledge\":\"\",\"sessionState\":\"会话ID: session-20260510-112633-f2a9949f-ws_6a40be0d0584f974\\n创建时间: 2026-05-10 11:26:33\\n最后活动: 2026-05-10 11:26:33\\n状态: active\",\"shortTermMemory\":\"【最近对话】\\n无相关内容\",\"success\":true}",
        "type": "text"
      }
    ]
  }
}
```

### 响应结构层次

```
响应根对象
├── jsonrpc: "2.0"
├── id: 100
└── result: {}
    └── content: []
        └── [0]: {}
            ├── type: "text"
            └── text: "{...JSON字符串...}"
```

## result.content[0].text 字段详细分析

### 字段特征

- **类型**: `string` (JSON 字符串)
- **格式**: 需要进行 JSON 解析才能获取实际数据
- **编码**: UTF-8

### text 字段解析后的结构

```json
{
  "longTermMemory": "【相关历史】\n无相关内容",
  "relevantKnowledge": "",
  "sessionState": "会话ID: session-20260510-112633-f2a9949f-ws_6a40be0d0584f974\n创建时间: 2026-05-10 11:26:33\n最后活动: 2026-05-10 11:26:33\n状态: active",
  "shortTermMemory": "【最近对话】\n无相关内容",
  "success": true
}
```

### 字段说明

| 字段名 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `longTermMemory` | string | 长期记忆（向量数据库检索结果） | "【相关历史】\n无相关内容" |
| `relevantKnowledge` | string | 相关知识（知识图谱检索结果） | "" |
| `sessionState` | string | 会话状态信息 | "会话ID: xxx\n创建时间: xxx\n..." |
| `shortTermMemory` | string | 短期记忆（最近对话） | "【最近对话】\n无相关内容" |
| `success` | boolean | 操作是否成功 | true |

## 数据访问路径

要获取实际的检索结果，需要按以下步骤解析：

```python
# 1. 获取 MCP 响应
response = requests.post(url, json=mcp_request)
data = response.json()

# 2. 提取 result.content[0].text
text_field = data['result']['content'][0]['text']

# 3. 解析 text 字段（它是一个 JSON 字符串）
parsed_data = json.loads(text_field)

# 4. 访问具体字段
long_term_memory = parsed_data['longTermMemory']
short_term_memory = parsed_data['shortTermMemory']
session_state = parsed_data['sessionState']
relevant_knowledge = parsed_data['relevantKnowledge']
```

## 测试中发现的问题

### 1. 向量服务未配置

**错误信息**:
```
存储长期记忆失败: 生成嵌入向量失败: 向量服务未配置
```

**原因**:
- 容器内的应用无法访问 `host.docker.internal:11434` 的 Ollama 服务
- 或者 Ollama embedding 服务配置不正确

**影响**:
- 无法存储长期记忆到向量数据库
- `longTermMemory` 字段返回 "无相关内容"

### 2. LLM 模型未安装

**错误信息**:
```
model 'deepseek-coder-v2:16b' not found
```

**原因**:
- 配置文件中指定的 LLM 模型未在 Ollama 中安装
- 只有 `nomic-embed-text:latest` 模型可用

**影响**:
- LLM 驱动的智能分析功能降级
- 自动降级到基础 ContextService

## 正常情况下的预期响应

当向量服务正常工作且有存储的记忆时，`longTermMemory` 字段应该包含类似以下内容：

```json
{
  "longTermMemory": "【相关历史】\n1. 项目使用的技术栈包括：Go语言作为后端开发语言，使用Gin框架构建RESTful API。\n2. 前端技术栈采用React 18和TypeScript，使用Vite作为构建工具，TailwindCSS用于样式管理。\n3. 数据库使用PostgreSQL 15，向量数据库使用Qdrant进行语义搜索，Redis用于缓存。",
  "relevantKnowledge": "",
  "sessionState": "会话ID: session-xxx\n创建时间: 2026-05-10 11:26:33\n最后活动: 2026-05-10 11:26:33\n状态: active",
  "shortTermMemory": "【最近对话】\n无相关内容",
  "success": true
}
```

## 关键发现总结

1. **双层 JSON 结构**: 响应使用了双层 JSON 编码
   - 外层：MCP 协议标准响应
   - 内层：`text` 字段是 JSON 字符串，需要二次解析

2. **content 数组**: `result.content` 是一个数组，通常只有一个元素

3. **type 字段**: 每个 content 项都有 `type: "text"` 标识

4. **字符串格式**: 实际数据在 `text` 字段中以 JSON 字符串形式存储

5. **多维度检索**: 响应包含四个维度的上下文信息
   - 长期记忆（向量检索）
   - 短期记忆（会话对话）
   - 相关知识（知识图谱）
   - 会话状态（元数据）

## 建议

1. **修复向量服务配置**: 确保 Docker 容器能访问 Ollama 服务
2. **安装 LLM 模型**: 运行 `ollama pull deepseek-coder-v2:16b` 或更新配置使用其他模型
3. **客户端解析**: 客户端需要实现双层 JSON 解析逻辑
4. **错误处理**: 检查 `success` 字段和各个记忆字段是否为空

## 测试文件

- `retrieve_response_complete.json`: 完整的 API 响应
- `retrieve_text_parsed.json`: 解析后的 text 字段内容
- `test_retrieve_final.py`: 完整的测试脚本
