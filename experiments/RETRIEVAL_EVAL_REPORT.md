# 检索评估完整报告

**评估时间**: 2026-07-14  
**数据集**: 50条查询 (17 temporal + 17 causal + 16 general)  
**配置**: full_system (三路融合：向量+图谱+时序)  
**会话**: eval_retrieval_test (7个ground truth文档)

---

## 一、核心指标

### 1.1 整体性能

| 指标 | 数值 | 说明 |
|------|------|------|
| **MRR** | **0.285** | 平均倒数排名，ground truth平均在第3.5位 |
| **Precision@5** | **0.188** | 前5个结果中18.8%命中ground truth |
| **Recall@5** | **0.546** | 54.6%的ground truth被检索到（在前5位内） |
| **平均延迟** | **19.9秒** | 包含embedding、检索、RRF融合全流程 |
| **API成功率** | **100%** | 50/50查询全部成功 |

### 1.2 按查询类型分解

| 查询类型 | 样本数 | MRR | 典型查询 |
|---------|--------|-----|---------|
| **General** | 16 | **0.359** ✅ | "王明的基本健康状况"、"所有阿尔茨海默病患者" |
| **Temporal** | 17 | **0.303** | "最近7天认知障碍频率"、"夜间谵妄时间分布" |
| **Causal** | 17 | **0.197** ⚠️ | "睡眠质量与认知恶化因果链"、"便秘与谵妄关系" |

**关键发现**:
- ✅ **General查询效果最好** (MRR=0.359)，说明简单实体/属性查询检索准确
- ✅ **Temporal查询次之** (MRR=0.303)，时序索引有效
- ⚠️ **Causal查询效果最差** (MRR=0.197)，因果推理能力不足，需要优化

---

## 二、修复历程

### 2.1 初始问题（MRR=0.000）

**症状**: 2026-07-14 早上，MRR始终为0.000，无法计算检索质量

**根因分析**:
1. **Qdrant存储未保存metadata** - `qdrant_store.go:202-211`只存储固定字段
2. **检索未读取metadata.id** - `context_service.go:4949`只查找`doc_id`字段
3. **会话过滤失效** - `context_service.go:4902`传入空字符串导致跨会话污染

### 2.2 修复措施

#### 修复1: Qdrant存储合并metadata
**文件**: `pkg/vectorstore/qdrant_store.go:202-211`

```go
// 构建点数据
payload := map[string]interface{}{
    "content":    memory.Content,
    "session_id": memory.SessionID,
    "user_id":    memory.UserID,
    "timestamp":  memory.Timestamp,
}

// 🔥 合并metadata到payload（支持自定义字段如doc_id）
if memory.Metadata != nil {
    for k, v := range memory.Metadata {
        payload[k] = v
    }
}
```

#### 修复2: 检索fallback到metadata.id
**文件**: `internal/services/context_service.go:4949`

```go
// 提取doc_id（优先从Fields中的doc_id，其次从id，最后用UUID）
docID := result.ID
if metaDocID, ok := result.Fields["doc_id"].(string); ok && metaDocID != "" {
    docID = metaDocID
} else if metaID, ok := result.Fields["id"].(string); ok && metaID != "" {
    docID = metaID  // 🔥 fallback到metadata.id
}
```

#### 修复3: 会话过滤修复
**文件**: `internal/services/context_service.go:4902`

```go
// 🔥 修复前: 传入空字符串，导致返回所有会话数据
// searchResults, err = s.searchByVector(ctx, queryVector, "", options)

// 🔥 修复后: 传入正确的sessionID
searchResults, err = s.searchByVector(ctx, queryVector, req.SessionID, options)
```

#### 修复4: 评测端top-5截断
**文件**: `experiments/scripts/retrieval_eval.py:210-223`

```python
# 严格截断到top-5，确保MRR计算口径准确
if len(retrieved_docs) > 5:
    retrieved_docs = retrieved_docs[:5]
```

### 2.3 修复效果对比

| 阶段 | MRR | 问题 | 状态 |
|------|-----|------|------|
| 修复前 | **0.000** | metadata丢失，无法匹配ground truth | ❌ |
| 修复后（9条结果） | **0.256** | 后端返回超过5条，MRR口径不准 | ⚠️ |
| top-5截断后 | **0.285** | 口径准确，但排序质量偏低 | ✅ |

---

## 三、当前问题分析

### 3.1 检索质量偏低（MRR=0.285）

**现象**: 
- 50条查询中，ground truth平均排在第3.5位
- Causal查询尤其差（MRR=0.197，平均第5位）

**可能原因**:
1. **测试数据质量问题** - 只有7个文档，查询涉及的患者名未出现在content中
2. **Embedding模型匹配度** - `qwen2.5:3b`的语义理解能力不足
3. **RRF融合权重** - 向量/图谱/时序三路权重可能不合理
4. **缺少重排序** - 未使用reranker对初筛结果精排

**验证方法**:
```bash
# 查看具体哪些查询失败（MRR=0.0）
cd experiments/results/raw
python -c "
import json
data = json.load(open('retrieval_20260714_113658.json'))
failed = [r for r in data['detailed_results'] if r['reciprocal_rank'] == 0.0]
print(f'失败查询: {len(failed)}/50')
for r in failed[:5]:
    print(f\"  {r['query_id']}: {r['query_text']}\")
"
```

### 3.2 延迟过高（19.9秒）

**现象**: 平均每个查询19.9秒，最慢可达30秒以上

**推测耗时分解**:
- **Embedding生成**: ~2-3秒 (Ollama qwen2.5:3b)
- **向量检索**: ~0.5秒 (Qdrant)
- **图谱查询**: ~1-2秒 (Neo4j)
- **时序查询**: ~1-2秒 (TimescaleDB)
- **LLM生成answer**: ~10-15秒 (Ollama qwen2.5:3b生成长文本)

**优化方向**:
1. **关闭LLM answer生成** - 评测只需doc_id，不需要生成完整回答
2. **并行执行三路检索** - 目前可能是串行
3. **升级Embedding模型** - 考虑使用专门的中文检索模型
4. **增加批量API** - 一次请求多个查询

### 3.3 会话中存在UUID文档

**现象**: 之前10条测试返回9个文档，包含2个UUID格式的ID

**推测来源**:
1. **历史会话消息** - `/api/mcp/tools/retrieve_context`可能混入历史chat消息
2. **其他模块写入** - security/causal模块测试时写入的临时数据
3. **测试残留** - 之前实验未清理的数据

**验证方法**:
```bash
# 方案1: 通过API查询eval_retrieval_test的所有上下文
curl -X POST http://localhost:8088/api/mcp/tools/retrieve_context \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"sessionId":"eval_retrieval_test","query":"*","maxResults":20}'

# 方案2: 删除会话重新seed
cd experiments/scripts
python seed_vector_data.py --session eval_retrieval_test --force-recreate
```

---

## 四、下一步行动

### 4.1 P0 - 阻塞性问题（答辩前必须完成）

#### 任务1: 关闭LLM answer生成，降低延迟到5秒以内
**目标**: 评测只需要retrieved_docs，不需要生成完整answer

**修改方案**:
- 后端: 为`/api/mcp/tools/retrieve_context`增加参数`generate_answer=false`
- 或: 评测脚本使用专门的检索API（如`/api/v1/retrieve`）而非MCP工具API

#### 任务2: 清理eval_retrieval_test会话，确保只有7个ground truth
**目标**: 消除UUID文档干扰

**方案**:
```bash
# 删除会话
curl -X DELETE http://localhost:8088/api/sessions/eval_retrieval_test

# 重新seed
cd experiments/scripts
python seed_vector_data.py --session eval_retrieval_test
```

#### 任务3: 分析低分查询，定位检索失败原因
**目标**: 找出MRR=0.0的查询，分析是数据问题还是算法问题

**方案**:
```python
# 查看失败案例的retrieved_docs，对比ground_truth
import json
data = json.load(open('retrieval_20260714_113658.json'))
for r in data['detailed_results']:
    if r['reciprocal_rank'] == 0.0:
        print(f"\n[FAIL] {r['query_text']}")
        print(f"  Ground truth: {r['ground_truth_docs']}")  # 需要添加这个字段
        print(f"  Retrieved: {r['retrieved_docs'][:5]}")
```

### 4.2 P1 - 功能完善（提升检索质量）

#### 任务4: 运行baseline配置对比
**目标**: 验证三路融合是否优于单路检索

**方案**:
```bash
# 分别测试4个配置
export EVAL_USER_ID="eval_user_001" EVAL_PASSWORD="demo123"

# 1. vanilla_llm (无检索)
python retrieval_eval.py --queries 50 --configs baseline_vanilla_llm

# 2. naive_rag (仅向量)
python retrieval_eval.py --queries 50 --configs baseline_naive_rag

# 3. rag_with_filter (向量+规则)
python retrieval_eval.py --queries 50 --configs baseline_rag_with_filter

# 4. full_system (三路融合)
# 已完成，MRR=0.285
```

**预期对比**:
| 配置 | 预期MRR | 说明 |
|------|---------|------|
| vanilla_llm | 0.0 | 无检索，随机回答 |
| naive_rag | 0.20-0.25 | 仅向量检索 |
| rag_with_filter | 0.25-0.30 | 向量+规则过滤 |
| full_system | **0.285** | 三路融合（当前） |

#### 任务5: 扩展测试数据集规模
**目标**: 从7个文档扩展到30+文档，测试更真实场景

**方案**:
```bash
# 创建新会话eval_retrieval_large，包含30个文档
cd experiments/scripts
python seed_vector_data.py --session eval_retrieval_large --count 30
```

### 4.3 P2 - 体验优化（锦上添花）

#### 任务6: 增加延迟分解日志
**目标**: 输出embedding/vector/graph/timeline/llm各阶段耗时

#### 任务7: 生成可视化报告
**目标**: MRR柱状图、查询类型雷达图、延迟分布箱线图

---

## 五、验收标准

### 5.1 功能完整性
- [x] 50条查询全部成功（API成功率100%）
- [x] MRR计算口径准确（top-5截断生效）
- [x] 三种查询类型全覆盖（temporal/causal/general）
- [ ] 延迟降低到5秒以内
- [ ] UUID文档清理完成

### 5.2 检索质量
- [x] MRR > 0.25（当前0.285，达标）
- [ ] Causal MRR > 0.25（当前0.197，未达标）
- [x] Recall@5 > 0.50（当前0.546，达标）
- [ ] 三路融合优于单路检索（待验证）

### 5.3 可演示性
- [x] 结果JSON完整保存
- [ ] baseline对比数据齐全
- [ ] 可视化图表生成
- [ ] 答辩用3-5个典型案例

---

## 六、答辩演示方案

### 6.1 核心亮点

**1. 问题诊断能力**
- 展示MRR从0.000到0.285的修复过程
- 证明能够系统性诊断和修复bug

**2. 三路融合效果**
- 对比baseline配置，证明full_system提升明显
- 雷达图展示三种查询类型的适配性

**3. 完整评测体系**
- 50条标注查询，覆盖3种类型
- MRR/Precision/Recall标准指标
- 可复现的评测流程

### 6.2 演示脚本（3分钟）

```bash
# 1. 快速演示（30秒）
cd experiments/scripts
export EVAL_USER_ID="eval_user_001" EVAL_PASSWORD="demo123"
python retrieval_eval.py --queries 5 --sample-mode representative --configs full_system

# 2. 展示结果（30秒）
cd ../results/raw
python -c "
import json
data = json.load(open('retrieval_20260714_113658.json'))
metrics = data['aggregated_metrics']['full_system']
print(f'MRR: {metrics[\"mrr\"]:.3f}')
print(f'Recall@5: {metrics[\"recall_at_5\"]:.3f}')
"

# 3. 典型案例（2分钟）
# 展示1个成功案例（MRR=1.0）+ 1个失败案例（MRR=0.0）
# 说明检索链路和优化方向
```

---

## 七、附录

### 7.1 关键文件清单

**后端代码**:
- `pkg/vectorstore/qdrant_store.go:202-211` - metadata合并
- `internal/services/context_service.go:4902` - 会话过滤
- `internal/services/context_service.go:4949` - doc_id提取

**评测脚本**:
- `experiments/scripts/retrieval_eval.py` - 主评测脚本
- `experiments/scripts/seed_vector_data.py` - 测试数据生成
- `experiments/datasets/retrieval_groundtruth/query_answer_pairs.json` - 50条标注

**结果文件**:
- `experiments/results/raw/retrieval_20260714_113658.json` - 完整50条结果
- `experiments/results/raw/retrieval_20260714_105229.json` - 修复后首次测试（9条结果）
- `experiments/results/raw/retrieval_20260714_104142.json` - 修复前测试（MRR=0.0）

### 7.2 环境依赖

**认证配置**:
```bash
export EVAL_USER_ID="eval_user_001"  # 必须使用knownDemoUsers中的用户
export EVAL_PASSWORD="demo123"       # DEMO_AUTH_PASSWORD
```

**Docker服务**:
```bash
export DEMO_AUTH_PASSWORD=demo123
docker-compose up -d
```

**Python依赖**:
```bash
cd experiments
pip install requests tqdm
```

---

**报告生成时间**: 2026-07-14 11:40  
**评估数据版本**: retrieval_20260714_113658.json  
**下次更新**: 完成baseline对比后
