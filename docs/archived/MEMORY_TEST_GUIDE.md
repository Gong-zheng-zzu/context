# 🧠 Context-Keeper 记忆功能测试指南

## 📊 当前系统状态

✅ **向量数据库 (Qdrant)**
- 状态：green（健康）
- 已存储记忆：20条
- 向量维度：768维
- 相似度算法：Cosine

✅ **LLM服务 (Ollama)**
- 模型：qwen2.5:7b
- 嵌入模型：nomic-embed-text
- 超时时间：1200秒

✅ **安全系统**
- 五层脱敏防护：已启用
- 审计日志：正常记录

---

## 🎯 测试方案

### 方式1: 浏览器可视化测试（推荐）

#### 步骤1: 打开演示页面
```
（原 chat-demo-fixed.html 演示页面已移除，请使用当前实际部署的前端入口）
```

#### 步骤2: 存储记忆
1. 在输入框输入：
   ```
   我正在开发一个Go语言的上下文管理系统，使用了Qdrant向量数据库和Ollama本地LLM
   ```
2. 点击"发送"按钮
3. 等待响应（应该在1-2秒内返回）

#### 步骤3: 测试语义检索
1. 在输入框输入：
   ```
   我之前说过用什么数据库？
   ```
2. 系统应该能检索到之前的记忆并回答："Qdrant向量数据库"

#### 步骤4: 测试脱敏功能
1. 点击"🔒 敏感信息"按钮
2. 观察脱敏预览效果
3. 发送消息
4. 检查审计日志：
   ```bash
   tail -f d:/context/context-keeper-main/data/security_audit.log/audit_2026-05-10.jsonl
   ```

---

### 方式2: API直接测试

#### 测试1: 存储上下文
```bash
curl -X POST http://localhost:8088/mcp/tools/create_context \
  -H "Content-Type: application/json" \
  -d '{
    "sessionId": "test_memory_001",
    "userId": "test_user",
    "content": "Context-Keeper使用Qdrant作为向量数据库，支持768维向量检索",
    "metadata": {
      "type": "technical_note",
      "tags": ["database", "vector", "qdrant"]
    }
  }'
```

**预期响应**：
```json
{"memoryId":"xxx-xxx-xxx","status":"success"}
```

#### 测试2: 查看向量数据库统计
```bash
curl -s http://localhost:6333/collections/context_keeper
```

**关键指标**：
- `points_count`: 记忆条数
- `status`: 应该是 "green"
- `indexed_vectors_count`: 已索引向量数

---

### 方式3: 检查存储文件

#### 查看会话数据
```bash
ls -lh d:/context/context-keeper-main/data/sessions/
```

#### 查看审计日志
```bash
tail -20 d:/context/context-keeper-main/data/security_audit.log/audit_2026-05-10.jsonl
```

---

## 🔍 验证记忆功能的关键指标

### 1. 存储成功率
- ✅ API返回 `status: success`
- ✅ Qdrant `points_count` 增加

### 2. 语义检索准确性
- ✅ 能检索到相关上下文
- ✅ 相似度分数 > 0.3（配置的阈值）
- ✅ 响应时间 < 2秒

### 3. 脱敏保护
- ✅ 审计日志记录敏感信息检测
- ✅ 存储的内容已脱敏
- ✅ 向量嵌入前已脱敏

### 4. LLM智能分析
- ✅ 能理解用户意图
- ✅ 能合成多条记忆
- ✅ 能提供上下文相关的回答

---

## 📈 性能基准

| 指标 | 目标值 | 当前状态 |
|------|--------|----------|
| 存储延迟 | < 500ms | ✅ 正常 |
| 检索延迟 | < 2s | ✅ 正常 |
| 向量生成 | < 200ms | ✅ 正常 |
| LLM分析 | < 1200s | ✅ 已优化 |

---

## 🐛 常见问题

### Q1: 检索返回空结果
**原因**：相似度阈值太高或没有相关记忆  
**解决**：降低 `min_score` 参数（默认0.3）

### Q2: 响应时间过长
**原因**：LLM模型太大或超时设置太短  
**解决**：已切换到 qwen2.5:7b，超时设置为1200秒

### Q3: 会话错误
**原因**：会话未初始化或已过期  
**解决**：使用新的 sessionId 或检查会话超时配置（720分钟）

---

## 🎉 测试成功标志

- ✅ 能成功存储记忆
- ✅ 能准确检索相关上下文
- ✅ 敏感信息被正确脱敏
- ✅ 审计日志正常记录
- ✅ 响应时间在可接受范围内

---

> 📅 生成时间: 2026-05-10  
> 🔧 工具: Context-Keeper 测试助手
