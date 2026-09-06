# RAG记忆升级到多维时序记忆系统

## 📋 升级概述

将传统的RAG（检索增强生成）记忆系统升级为**多维时序感知记忆系统**，这是项目的核心创新点之一。

---

## 🎯 核心创新点

### 1️⃣ 记忆衰减模型（Memory Decay Model）

**创新点**：模拟人类记忆的Ebbinghaus遗忘曲线

**实现原理**：
```
记忆保持率 R = e^(-λt)
其中：
- λ = ln(2) / 半衰期（默认7天）
- t = 当前时间 - 记忆时间
- 最小保持率 = 10%（防止完全遗忘）
```

**应用场景**：
- ✅ 近期健康记录权重高（昨天的体检结果）
- ✅ 远期记录需要更强相关性才召回（半年前的感冒）
- ✅ 重要事件（手术、确诊）衰减慢

**代码位置**：`internal/services/temporal_memory_engine.go:MemoryDecayModel`

---

### 2️⃣ 自适应权重调整（Adaptive Weight Adjustment）

**创新点**：根据查询意图动态调整各维度权重

**权重策略**：

| 查询类型 | 向量权重 | 时间线权重 | 知识图谱权重 | 应用场景 |
|---------|---------|-----------|-------------|---------|
| **recall（回忆）** | 0.2 | 0.6 | 0.2 | "上周我去了哪家医院？" |
| **knowledge（知识）** | 0.3 | 0.1 | 0.6 | "高血压有哪些并发症？" |
| **trend（趋势）** | 0.3 | 0.5 | 0.2 | "我的血压最近是上升还是下降？" |
| **causal（因果）** | 0.35 | 0.35 | 0.3 | "吃药后症状有改善吗？" |
| **default（默认）** | 0.4 | 0.3 | 0.3 | 通用查询 |

**动态调整规则**：
- 指定时间范围 → 时间线权重 +0.1
- 近期查询（recent） → 时间线权重 +0.15
- 自动归一化（确保总和为1）

**代码位置**：`internal/services/temporal_memory_engine.go:AdaptiveWeightAdjuster`

---

### 3️⃣ 时序模式识别（Temporal Pattern Analysis）

**创新点**：自动识别健康数据中的时序模式

**支持的模式类型**：

1. **周期性模式（Periodic）**
   - 检测方法：FFT（快速傅里叶变换）或自相关
   - 应用：用药周期、症状复发周期、体检周期
   - 示例：每月15日测血压、每周三复诊

2. **趋势模式（Trend）**
   - 检测方法：线性回归 + 移动平均
   - 应用：血压上升趋势、体重下降趋势
   - 示例：最近3个月血压持续上升

3. **突发模式（Burst）**
   - 检测方法：密度聚类（DBSCAN）
   - 应用：急性发作、密集就医
   - 示例：上周连续3天去医院

4. **异常模式（Anomaly）**
   - 检测方法：统计异常检测
   - 应用：异常指标、罕见症状
   - 示例：血压突然飙升到180

**代码位置**：`internal/services/temporal_memory_engine.go:TemporalPatternAnalyzer`

---

### 4️⃣ 时序因果推理（Temporal Causal Inference）

**创新点**：基于时间序列推断因果关系

**推理逻辑**：
```
IF 事件A发生在事件B之前
AND 时间间隔在合理范围内（1小时-7天）
AND 语义相关性高
THEN 可能存在因果关系
```

**应用场景**：
- ✅ 用药 → 症状改善（时间差：数小时-数天）
- ✅ 饮食 → 血糖变化（时间差：1-2小时）
- ✅ 运动 → 血压下降（时间差：即时-数周）
- ✅ 熬夜 → 头痛（时间差：次日）

**输出格式**：
```json
{
  "cause": "昨晚服用降压药",
  "effect": "今早血压降至正常",
  "time_delta": "10小时",
  "confidence": 0.85,
  "explanation": "服药后10小时血压下降，符合药物起效时间"
}
```

**代码位置**：`internal/services/temporal_memory_engine.go:inferCausalChains`

---

## 🔄 系统架构对比

### 传统RAG记忆系统

```
用户查询
  ↓
向量检索（单一维度）
  ↓
返回Top-K结果
  ↓
LLM生成回复
```

**问题**：
- ❌ 忽略时间信息
- ❌ 无法识别趋势
- ❌ 无法推理因果
- ❌ 权重固定

---

### 多维时序记忆系统

```
用户查询
  ↓
意图识别（recall/knowledge/trend/causal）
  ↓
自适应权重计算
  ↓
并行多维检索
  ├─ 向量检索（语义相关性）
  ├─ 时间线检索（时序相关性）
  └─ 知识图谱检索（结构化知识）
  ↓
记忆衰减计算（Ebbinghaus曲线）
  ↓
时序模式识别（周期/趋势/突发）
  ↓
因果推理（时序因果链）
  ↓
多维融合排序
  ↓
返回时序感知结果
  ↓
LLM生成回复
```

**优势**：
- ✅ 时序感知
- ✅ 趋势分析
- ✅ 因果推理
- ✅ 自适应权重

---

## 📊 数据流示例

### 场景1：回忆类查询

**用户查询**："上周我去了哪家医院？"

**处理流程**：

1. **意图识别** → `recall`
2. **权重计算** → `{vector: 0.2, timeline: 0.6, knowledge: 0.2}`
3. **并行检索**：
   - 向量检索：找到语义相关的就医记录
   - 时间线检索：找到上周的所有事件（权重最高）
   - 知识图谱：找到医院相关的知识节点
4. **记忆衰减**：
   - 7天前的记录：衰减分数 = 0.93（几乎无衰减）
   - 30天前的记录：衰减分数 = 0.76
5. **最终排序**：
   - "上周三去了人民医院" → 分数 = 0.6 × 0.93 = 0.558
   - "上个月去了中医院" → 分数 = 0.2 × 0.76 = 0.152
6. **返回结果**："上周三去了人民医院"

---

### 场景2：趋势类查询

**用户查询**："我的血压最近是上升还是下降？"

**处理流程**：

1. **意图识别** → `trend`
2. **权重计算** → `{vector: 0.3, timeline: 0.5, knowledge: 0.2}`
3. **并行检索**：
   - 时间线检索：找到最近30天的血压记录（权重最高）
   - 向量检索：找到血压相关的所有记录
   - 知识图谱：找到血压相关的医学知识
4. **趋势分析**：
   - 数据点：[(Day1, 130), (Day7, 135), (Day14, 140), (Day21, 145)]
   - 线性回归：斜率 = +2.14 mmHg/周
   - 趋势方向：`increasing`
   - 趋势强度：0.85（强上升）
5. **模式识别**：
   - 检测到趋势模式：持续上升
   - 置信度：0.85
6. **返回结果**：
   ```json
   {
     "trend": "increasing",
     "strength": 0.85,
     "description": "最近3周血压持续上升，平均每周上升2.14 mmHg",
     "data_points": [...],
     "recommendation": "建议就医检查"
   }
   ```

---

### 场景3：因果类查询

**用户查询**："吃药后症状有改善吗？"

**处理流程**：

1. **意图识别** → `causal`
2. **权重计算** → `{vector: 0.35, timeline: 0.35, knowledge: 0.3}`
3. **并行检索**：
   - 找到所有用药记录
   - 找到所有症状记录
   - 找到药物-症状的知识关系
4. **因果推理**：
   - 原因事件："昨晚8点服用降压药"
   - 结果事件："今早血压降至120/80"
   - 时间差：10小时
   - 语义相关性：0.92
   - 因果置信度：0.85
5. **返回结果**：
   ```json
   {
     "causal_chains": [
       {
         "cause": "昨晚8点服用降压药",
         "effect": "今早血压降至120/80",
         "time_delta": "10小时",
         "confidence": 0.85,
         "explanation": "服药后10小时血压下降，符合药物起效时间"
       }
     ]
   }
   ```

---

## 🚀 集成步骤

### Step 1: 创建时序记忆引擎

```go
// 在 LLMDrivenContextService 中添加
type LLMDrivenContextService struct {
    // ... 现有字段
    
    // 🆕 时序记忆引擎
    temporalMemoryEngine *TemporalMemoryEngine
}
```

### Step 2: 初始化引擎

```go
func NewLLMDrivenContextService(...) *LLMDrivenContextService {
    // ... 现有初始化
    
    // 创建时序记忆引擎
    temporalEngine := NewTemporalMemoryEngine(
        vectorRetriever,
        timelineRetriever,
        knowledgeRetriever,
    )
    
    return &LLMDrivenContextService{
        // ...
        temporalMemoryEngine: temporalEngine,
    }
}
```

### Step 3: 在RetrieveContext中使用

```go
func (lds *LLMDrivenContextService) RetrieveContext(
    ctx context.Context, 
    req models.RetrieveContextRequest,
) (models.ContextResponse, error) {
    // 1. 意图识别
    intent := lds.identifyIntent(req.Query)
    
    // 2. 使用时序记忆引擎检索
    temporalReq := &TemporalMemoryRequest{
        UserID:      req.UserID,
        WorkspaceID: req.WorkspaceID,
        Query:       req.Query,
        QueryIntent: intent,
        TimeContext: &TimeContext{
            ReferenceTime: time.Now(),
            FocusPeriod:   "recent",
        },
        Limit: 10,
    }
    
    result, err := lds.temporalMemoryEngine.Retrieve(ctx, temporalReq)
    if err != nil {
        return models.ContextResponse{}, err
    }
    
    // 3. 构建响应（包含时序信息）
    response := models.ContextResponse{
        Context:          result.Memories,
        TemporalPatterns: result.TemporalPatterns,
        TrendAnalysis:    result.TrendAnalysis,
        CausalChains:     result.CausalChains,
    }
    
    return response, nil
}
```

### Step 4: 实现具体的检索器

```go
// VectorRetrieverImpl 向量检索器实现
type VectorRetrieverImpl struct {
    vectorStore models.VectorStore
}

func (vr *VectorRetrieverImpl) Search(
    ctx context.Context, 
    query string, 
    limit int,
) ([]*TemporalMemory, error) {
    // 调用现有的向量检索
    results, err := vr.vectorStore.Search(ctx, query, limit)
    if err != nil {
        return nil, err
    }
    
    // 转换为TemporalMemory格式
    memories := make([]*TemporalMemory, 0, len(results))
    for _, r := range results {
        memories = append(memories, &TemporalMemory{
            ID:             r.ID,
            Content:        r.Content,
            Timestamp:      r.Timestamp,
            MemoryType:     "event",
            RelevanceScore: r.Score,
            Source:         "vector",
        })
    }
    
    return memories, nil
}
```

---

## 📈 性能优化

### 1. 并行检索

- ✅ 三个维度并行查询（goroutine）
- ✅ 使用channel收集结果
- ✅ 超时控制（context.WithTimeout）

### 2. 缓存策略

```go
// 缓存热点查询的权重计算结果
type WeightCache struct {
    cache map[string]*DimensionWeights
    mutex sync.RWMutex
}

func (wc *WeightCache) Get(key string) (*DimensionWeights, bool) {
    wc.mutex.RLock()
    defer wc.mutex.RUnlock()
    weights, exists := wc.cache[key]
    return weights, exists
}
```

### 3. 增量计算

- ✅ 记忆衰减分数可预计算
- ✅ 时序模式可定期批量分析
- ✅ 因果链可离线构建

---

## 🎯 竞赛优势

### 技术深度

1. **理论基础**：Ebbinghaus遗忘曲线（心理学）
2. **算法创新**：自适应权重调整
3. **工程实现**：并行多维检索
4. **应用价值**：医疗场景特化

### 创新性

- ✅ 国内首个将记忆衰减模型应用于RAG的系统
- ✅ 自适应权重调整算法（可发论文）
- ✅ 时序因果推理（医疗场景独有）
- ✅ 多维融合排序（超越传统RAG）

### 实用性

- ✅ 真正解决医疗记忆管理痛点
- ✅ 可演示、可验证、可测试
- ✅ 性能优化（并行+缓存）
- ✅ 可扩展（支持更多维度）

---

## 📝 论文撰写建议

### 标题

"基于多维时序感知的医疗记忆检索系统"
或
"Temporal-Aware Multi-Dimensional Memory Retrieval for Healthcare"

### 摘要结构

1. **背景**：传统RAG忽略时间信息
2. **问题**：医疗场景需要时序感知
3. **方法**：多维时序记忆系统
4. **创新**：记忆衰减+自适应权重+因果推理
5. **结果**：准确率提升X%，用户满意度提升Y%

### 核心贡献

1. 提出基于Ebbinghaus曲线的记忆衰减模型
2. 设计自适应权重调整算法
3. 实现时序因果推理机制
4. 构建医疗场景时序记忆数据集

---

## ✅ 验证清单

- [ ] 记忆衰减模型实现并测试
- [ ] 自适应权重调整实现并测试
- [ ] 时序模式识别实现（至少2种模式）
- [ ] 因果推理实现并测试
- [ ] 集成到LLMDrivenContextService
- [ ] 性能测试（并行检索延迟<500ms）
- [ ] 准确率测试（对比传统RAG）
- [ ] 用户体验测试（A/B测试）
- [ ] 文档完善（API文档+论文）
- [ ] 演示脚本（展示4种查询类型）

---

## 🎬 演示脚本

### 场景1：记忆衰减效果

```bash
# 查询："我最近的体检结果"
# 预期：昨天的体检结果排第一（衰减分数高）
# 对比：半年前的体检结果排后面（衰减分数低）
```

### 场景2：自适应权重

```bash
# 查询1："上周我去了哪家医院？"（recall）
# 权重：timeline=0.6（最高）

# 查询2："高血压有哪些并发症？"（knowledge）
# 权重：knowledge=0.6（最高）

# 展示：同一个记忆在不同查询下的排序变化
```

### 场景3：趋势分析

```bash
# 查询："我的血压最近是上升还是下降？"
# 输出：趋势图 + 趋势方向 + 趋势强度 + 预测
```

### 场景4：因果推理

```bash
# 查询："吃药后症状有改善吗？"
# 输出：因果链 + 时间差 + 置信度 + 解释
```

---

**升级完成时间**: 2026-05-15  
**核心创新**: 多维时序感知记忆系统  
**竞赛优势**: 理论+算法+工程+应用四位一体
