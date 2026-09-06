# 检索实验修复状态报告

**日期**: 2026-07-13  
**状态**: 🔴 阻塞中 - 需要后端修改或采用临时方案

---

## 问题根源

### 发现的核心问题

1. **后端响应缺少结构化字段**
   - 当前 `ContextResponse` 只返回4个字符串字段
   - 没有 `contexts` 或 `retrieved_contexts` 数组
   - 无法提取带doc_id的检索结果

2. **后端响应实际结构**：
```json
{
  "session_state": "会话ID: xxx",
  "short_term_memory": "【最近对话】\n...",
  "long_term_memory": "【相关历史】\n...",
  "relevant_knowledge": ""
}
```

3. **评测脚本期望的结构**（retrieval_eval.py Line 209-218）：
```json
{
  "contexts": [
    {"doc_id": "elder_025_xxx", "content": "...", "score": 0.85},
    ...
  ]
}
```

---

## 已尝试的修复

### ✅ 已完成：代码修改（但未生效）

1. **修改了 `internal/models/models.go`**
   - 添加了 `Contexts []ContextItem` 字段
   - 添加了 `ContextItem` 结构体定义

2. **修改了 `internal/services/context_service.go`**
   - 在构建响应时填充 `Contexts` 字段
   - 从 `searchResults` 转换为结构化数组

### ❌ 阻塞问题：无法编译部署

**问题**：`go mod tidy` 失败，网络无法连接 `proxy.golang.org`

```
dial tcp [2404:6800:4012:2::2011]:443: connectex: 
A connection attempt failed because the connected party 
did not properly respond after a period of time
```

**影响**：修改的代码无法编译到Docker镜像中

---

## 临时解决方案（推荐）

由于后端编译受阻，建议采用以下临时方案：

### 方案A：修改评测脚本使用替代指标

**思路**：既然无法获取doc_id，使用响应文本内容作为替代验证

**修改 retrieval_eval.py**：

```python
# Line 203-233 修改
if response.is_valid_business_response():
    data = response.data
    retrieved_docs = []
    
    # 尝试从结构化字段获取（未来后端修复后生效）
    if "contexts" in data:
        retrieved_docs = [ctx.get("doc_id", ctx.get("id", "")) 
                         for ctx in data.get("contexts", [])]
    elif "retrieved_contexts" in data:
        retrieved_docs = [ctx.get("doc_id", ctx.get("id", "")) 
                         for ctx in data.get("retrieved_contexts", [])]
    
    # 临时方案：如果没有结构化字段，从文本字段估算
    if not retrieved_docs:
        # 标记为"后端未返回结构化结果"
        result.retrieved_docs = ["BACKEND_NO_STRUCTURED_RESPONSE"]
        result.response_text = data.get("long_term_memory", "") + data.get("relevant_knowledge", "")
        
        # 使用文本匹配作为临时指标
        if result.response_text:
            # 检查是否包含ground truth中的关键词
            result.found_ground_truth = any(
                keyword in result.response_text 
                for keyword in ground_truth.get("expected_keywords", [])
            )
            # 无法计算MRR/Precision/Recall，标记为N/A
            result.reciprocal_rank = -1  # -1表示无法计算
            result.precision_at_5 = -1
            result.recall_at_5 = -1
        
        # 在结果JSON中添加警告
        result.warning = "后端未返回结构化检索结果，使用文本匹配作为临时指标"
    else:
        # 正常计算指标
        found, rr, p5, r5 = self.calculate_metrics(retrieved_docs, ground_truth)
        result.found_ground_truth = found
        result.reciprocal_rank = rr
        result.precision_at_5 = p5
        result.recall_at_5 = r5
```

**优点**：
- 可以立即运行实验
- 不依赖后端修改
- 能够部分验证检索功能

**缺点**：
- 无法计算精确的MRR/Precision/Recall
- 只能用关键词匹配作为粗略指标

---

### 方案B：修复Go依赖问题后重新编译

**步骤**：

1. **配置Go代理**（解决网络问题）
```bash
# 设置国内Go代理
export GOPROXY=https://goproxy.cn,direct
export GONOPROXY=none
export GONOSUMDB=*

cd D:/context/context-keeper-main
go mod tidy
go mod download
```

2. **本地编译测试**
```bash
go build -tags http -o bin/context-keeper.exe ./cmd/server/
```

3. **如果本地编译成功，重新构建Docker镜像**
```bash
docker-compose down
docker-compose build --no-cache context-keeper
docker-compose up -d
```

4. **验证新字段是否返回**
```bash
curl -X POST http://localhost:8088/api/mcp/tools/retrieve_context \
  -H "Content-Type: application/json" \
  -d '{"sessionId": "test", "query": "测试"}' | jq '.contexts'
```

**优点**：
- 彻底解决问题
- 可以获取精确的doc_id
- 支持正确的指标计算

**缺点**：
- 需要解决网络问题
- 需要重新编译和部署
- 耗时较长

---

## 下一步行动

### 立即可执行（推荐）

1. **采用方案A** - 修改评测脚本使用临时指标
2. **配置embedding服务** - 解决向量生成问题
3. **灌入测试数据** - 使用 `seed_vector_data.py`
4. **运行小规模测试** - 验证流程可行性

### 后续优化（答辩后）

1. **解决Go代理问题** - 配置国内镜像
2. **重新编译后端** - 部署修改后的代码
3. **切换到精确指标** - 使用真实的doc_id计算MRR

---

## 当前可执行的完整流程

```bash
# 1. 修改评测脚本（采用方案A）
# 手动编辑 experiments/scripts/retrieval_eval.py

# 2. 配置embedding服务（使用Ollama）
export GOPROXY=https://goproxy.cn,direct
ollama pull nomic-embed-text

# 修改 config/.env 或 docker-compose.yml
EMBEDDING_API_URL=http://host.docker.internal:11434
EMBEDDING_MODEL=nomic-embed-text

# 3. 重启服务
docker-compose restart context-keeper

# 4. 灌入测试数据
cd experiments/scripts
python seed_vector_data.py --session-id eval_groundtruth

# 5. 运行小规模测试
python retrieval_eval.py --queries 5 --configs baseline_rag_with_filter

# 6. 检查结果
cat ../results/raw/retrieval_*.json | grep -A 5 "warning\|found_ground_truth"
```

---

## 结论

**当前状态**：检索实验可以运行，但需要采用临时方案

**阻塞点**：后端编译依赖问题

**推荐路径**：先用方案A完成实验，后续有时间再切换到精确指标

**答辩可行性**：使用关键词匹配可以展示检索功能，但需要在报告中说明指标的局限性
