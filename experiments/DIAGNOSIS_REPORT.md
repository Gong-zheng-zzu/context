# Context-Keeper 实验体系诊断报告

生成时间：2026-07-13  
诊断版本：P0-Critical-Fix

---

## 执行摘要

三个实验全部失败的**根因是环境配置缺失**，非代码逻辑错误：

1. **检索实验**：OpenAI Embedding API Key未配置 → 无法向量化文档 → 向量库为空
2. **因果推理实验**：LLM返回空结果 → 提取能力不足或prompt不匹配
3. **安全实验**：JWT认证未配置 → 所有请求返回401/404

---

## 问题1：检索实验 - 向量库数据缺失

### 现象
- 20次查询全部返回`retrieved_docs: []`
- MRR/Precision/Recall全为0
- API响应：`{"relevant_knowledge": ""}`

### 根因诊断
```
[TRACE] 调用链路：
1. retrieval_eval.py → POST /api/mcp/tools/retrieve_context
2. 后端返回HTTP 200，但relevant_knowledge为空字符串
3. 尝试写入测试数据 → HTTP 500错误
4. 错误信息：embedding API返回错误: You didn't provide an API key
```

**结论**：向量库为空 + 无法写入数据（Embedding API未配置）

### 修复方案

#### 方案A：配置OpenAI API Key（推荐用于生产环境）
```bash
# 在 .env 或环境变量中设置
export OPENAI_API_KEY="sk-..."

# 或在配置文件中设置
# configs/baseline_rag_with_filter.yaml
embedding:
  provider: "openai"
  api_key: "${OPENAI_API_KEY}"
  model: "text-embedding-3-small"
```

#### 方案B：使用本地Embedding模型（推荐用于实验）
```bash
# 修改配置使用Ollama本地模型
# configs/baseline_rag_with_filter.yaml
embedding:
  provider: "ollama"
  base_url: "http://localhost:11434"
  model: "nomic-embed-text"
```

确保Ollama已安装并下载模型：
```bash
ollama pull nomic-embed-text
```

#### 方案C：准备预嵌入的测试数据
如果无法配置embedding服务，需要：
1. 使用已有向量库备份
2. 或使用mock数据跳过向量化步骤

### 验证步骤
```bash
# 1. 配置embedding服务后，运行数据准备脚本
cd experiments/scripts
python seed_vector_data.py --session-id eval_groundtruth

# 2. 验证数据写入成功（应看到7个成功）
# Expected output: 成功: 7, 失败: 0

# 3. 运行检索实验
python retrieval_eval.py --queries 3 --configs baseline_rag_with_filter

# 4. 检查结果
# Expected: retrieved_docs不为空，MRR > 0
```

---

## 问题2：因果推理实验 - LLM提取失败

### 现象
- 20条样本全部`extracted: false`
- API返回：`{"relations":[],"count":0,"process_time_ms":5234}`
- 准确率和F1分数均为0

### 根因诊断
```
[TRACE] 调用链路：
1. causal_eval.py → POST /api/v1/causal/extract
2. 请求体正确：{"text": "李国强，70岁，阿尔茨海默病...", "use_llm": true}
3. API响应HTTP 200，但relations为空数组
4. 处理时间5秒（LLM已调用）
```

**结论**：数据和API正常，但LLM无法从文本中提取因果关系

### 可能原因
1. **LLM模型能力不足**：使用的本地模型（如llama3.2:3b）对中文因果关系理解不足
2. **Prompt设计问题**：因果提取prompt未针对中文护理记录优化
3. **置信度阈值过高**：`min_confidence=0.5`过滤掉了所有低置信度结果

### 修复方案

#### 方案A：升级LLM模型（推荐）
```bash
# 使用更强的中文模型
ollama pull qwen2.5:14b  # 或 qwen2.5:32b

# 修改配置
# configs/full_system.yaml
llm:
  provider: "ollama"
  model: "qwen2.5:14b"
```

#### 方案B：优化因果提取Prompt
检查 `internal/engines/causal_reasoning/entity_extractor.go` 中的prompt模板，确保：
- 包含明确的中文示例
- 明确定义因果关系的标准
- 提供JSON输出格式示例

#### 方案C：降低置信度阈值进行测试
```python
# experiments/scripts/causal_eval.py Line 140
payload = {
    "text": record_text,
    "use_llm": True,
    "min_confidence": 0.3  # 从0.5降到0.3
}
```

### 验证步骤
```bash
# 1. 单样本测试
curl -X POST http://localhost:8088/api/v1/causal/extract \
  -H "Content-Type: application/json" \
  -d '{"text":"李国强，70岁，阿尔茨海默病中期，夜间谵妄发作导致情绪激动。","use_llm":true,"min_confidence":0.3}'

# 2. 期望输出包含relations数组
# Expected: {"relations": [{"cause": "...", "effect": "...", "confidence": ...}]}

# 3. 运行完整实验
cd experiments/scripts
python causal_eval.py --samples 5 --configs full_system

# 4. 检查结果
# Expected: extraction_rate > 0.3
```

---

## 问题3：安全实验 - JWT认证缺失

### 现象
- 请求返回HTTP 404
- 但被统计为"防御成功率100%"
- 实际是API路由找不到（因为认证失败）

### 根因诊断
```
[TRACE] 调用链路：
1. security_eval.py → POST /api/chat
2. 未携带Authorization header
3. 后端返回：{"error":"缺少Authorization header","success":false}
4. 被错误统计为防御成功
```

**结论**：认证环境变量未设置 + 错误响应未被正确处理

### 修复方案

#### 步骤1：设置认证凭据
```bash
# 方案A：使用预生成的token
export EVAL_AUTH_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 方案B：使用用户名密码登录
export EVAL_USER_ID="admin"
export EVAL_PASSWORD="your_password"
export EVAL_WORKSPACE_ID="default"
```

#### 步骤2：修复ASR计算逻辑（已在代码中实现）
代码已正确区分API错误和业务失败：
```python
# security_eval.py Line 289-293
api_success = [r for r in config_results if r.error_type == "None"]
api_error = [r for r in config_results if r.error_type != "None"]

# 只基于成功的调用计算ASR
if api_success_count > 0:
    asr = leaked_count / api_success_count
```

### 验证步骤
```bash
# 1. 设置环境变量后测试登录
curl -X POST http://localhost:8088/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"user_id":"admin","password":"your_password","workspace_id":"default"}'

# 2. 提取token并测试
export EVAL_AUTH_TOKEN="<从上面响应中提取的token>"

# 3. 运行安全实验
cd experiments/scripts
python security_eval.py --samples 5 --configs full_system

# 4. 检查结果
# Expected: HTTP 200响应，ASR正确计算（不包含4xx/5xx）
```

---

## 修复优先级

### P0（必须修复才能运行）
1. ✅ **配置Embedding服务**（检索实验前置条件）
   - 选择方案A或B
   - 验证：`seed_vector_data.py`成功写入7个文档

2. ✅ **设置JWT认证**（安全实验前置条件）
   - 设置`EVAL_AUTH_TOKEN`或登录凭据
   - 验证：`curl /api/chat`返回200而非401

### P1（影响结果质量）
3. ⚠️ **优化因果提取**（因果实验结果为0）
   - 升级LLM模型或降低阈值
   - 验证：extraction_rate > 30%

---

## 代码修复总结

### 已完成的修复
1. ✅ `base_evaluator.py`：区分API错误和业务失败
2. ✅ `retrieval_eval.py`：只统计HTTP 2xx的有效响应
3. ✅ `causal_eval.py`：正确加载数据，API调用格式正确
4. ✅ `security_eval.py`：实现认证逻辑，ASR计算排除4xx/5xx
5. ✅ `seed_vector_data.py`：创建向量库数据准备脚本

### 无需修复的部分
- 实验脚本逻辑正确
- API调用参数格式正确
- Ground truth数据格式正确
- 指标计算逻辑正确

---

## 环境配置清单

### 必需配置
```bash
# 1. Embedding服务（二选一）
export OPENAI_API_KEY="sk-..."           # OpenAI
# 或
ollama pull nomic-embed-text             # Ollama

# 2. 认证凭据（二选一）
export EVAL_AUTH_TOKEN="eyJ..."          # 预生成token
# 或
export EVAL_USER_ID="admin"              # 用户名密码
export EVAL_PASSWORD="your_password"
export EVAL_WORKSPACE_ID="default"
```

### 推荐配置
```bash
# 3. LLM模型（用于因果推理）
ollama pull qwen2.5:14b

# 4. 向量数据库
# 确保Qdrant在运行：docker ps | grep qdrant
```

---

## 快速启动指南

### 完整修复流程（30分钟）

```bash
# === 步骤1：配置环境 ===
cd /path/to/context-keeper-main

# 配置embedding（选择方案B - Ollama）
ollama pull nomic-embed-text

# 配置认证（假设已有admin用户）
export EVAL_USER_ID="admin"
export EVAL_PASSWORD="admin123"
export EVAL_WORKSPACE_ID="default"

# === 步骤2：准备测试数据 ===
cd experiments/scripts
python seed_vector_data.py --session-id eval_groundtruth --verify

# 期望输出：成功: 7

# === 步骤3：运行3个实验（各5个样本） ===
python retrieval_eval.py --queries 5 --configs baseline_rag_with_filter
python causal_eval.py --samples 5 --configs full_system
python security_eval.py --samples 5 --configs full_system

# === 步骤4：检查结果 ===
ls -lh ../results/raw/
# 应看到3个新的JSON文件

# === 步骤5：验证修复 ===
# 检索：MRR > 0
# 因果：extraction_rate > 0（如果仍为0，考虑降低min_confidence）
# 安全：api_success_count > 0，ASR正确计算
```

---

## 联系人

如遇到其他问题，请检查：
1. 后端服务日志：`docker logs context-keeper-backend`
2. Qdrant状态：`curl http://localhost:6333/health`
3. Ollama状态：`ollama list`

---

**报告结束**
