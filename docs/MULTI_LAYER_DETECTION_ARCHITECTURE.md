# 多层级联敏感信息检测系统架构

## 🏗️ 系统概述

本系统采用**6层级联检测架构**，结合正则表达式、词典匹配、NER实体识别、上下文规则、LLM智能检测和融合决策，实现高准确率、低延迟的敏感信息检测。

---

## 📊 架构图

```
用户输入
  ↓
┌─────────────────────────────────────────┐
│ Layer 1: 快速预过滤层（正则表达式）      │
│ - 速度：<5ms                             │
│ - 准确率：70-80%                         │
│ - 状态：✅ 已实现                        │
└─────────────────────────────────────────┘
  ↓ (置信度<0.9 继续)
┌─────────────────────────────────────────┐
│ Layer 2: 词典匹配层（Trie树）            │
│ - 速度：<2ms                             │
│ - 准确率：90%                            │
│ - 状态：🆕 待实现                        │
└─────────────────────────────────────────┘
  ↓ (置信度<0.9 继续)
┌─────────────────────────────────────────┐
│ Layer 3: NER实体识别层（HanLP）          │
│ - 速度：15-35ms                          │
│ - 准确率：85-92%                         │
│ - 状态：🆕 待实现                        │
└─────────────────────────────────────────┘
  ↓ (置信度<0.9 继续)
┌─────────────────────────────────────────┐
│ Layer 4: 上下文规则层                    │
│ - 速度：<10ms                            │
│ - 准确率：88%                            │
│ - 状态：🆕 待实现                        │
└─────────────────────────────────────────┘
  ↓ (置信度<0.9 继续)
┌─────────────────────────────────────────┐
│ Layer 5: LLM智能检测层（Ollama）         │
│ - 速度：<500ms                           │
│ - 准确率：95%+                           │
│ - 状态：🆕 待实现                        │
└─────────────────────────────────────────┘
  ↓
┌─────────────────────────────────────────┐
│ Layer 6: 融合决策层                      │
│ - 多层结果融合                           │
│ - 置信度加权                             │
│ - 冲突解决                               │
│ - 状态：🆕 待实现                        │
└─────────────────────────────────────────┘
  ↓
脱敏输出
```

---

## 🔍 各层详细设计

### Layer 1: 快速预过滤层（正则表达式）

**功能**：
- 快速检测结构化敏感信息
- 支持20+种敏感信息类型

**已支持类型**：
- 手机号、身份证、银行卡、信用卡
- API密钥、密码、Token、JWT
- 邮箱、IP地址、护照号
- AWS密钥、数据库连接串

**实现文件**：
- `internal/security/sensitive_detector.go`

**性能**：
- 延迟：<5ms
- 准确率：70-80%

---

### Layer 2: 词典匹配层（Trie树）

**功能**：
- 快速匹配已知敏感词
- 支持自定义黑名单
- 支持通配符和模糊匹配

**数据结构**：
```go
type TrieNode struct {
    children map[rune]*TrieNode
    isEnd    bool
    value    string
    category SensitiveType
}

type DictionaryMatcher struct {
    root     *TrieNode
    dict     map[string]SensitiveType
    mu       sync.RWMutex
}
```

**词典分类**：
- 政治敏感词
- 暴力恐怖词汇
- 色情低俗词汇
- 自定义敏感词

**实现要点**：
- AC自动机算法（Aho-Corasick）
- 支持动态更新词典
- 支持词典导入/导出

**性能目标**：
- 延迟：<2ms
- 准确率：90%

---

### Layer 3: NER实体识别层（HanLP）

**功能**：
- 识别人名、地名、组织名
- 理解语义上下文
- 补充正则无法识别的实体

**技术选型**：
- **推荐方案**：HanLP Go版
- 库：`github.com/hankcs/hanlp`
- 模型：中文NER预训练模型

**实现架构**：
```go
type NERDetector struct {
    hanlp    *hanlp.HanLP
    cache    *lru.Cache
    enabled  bool
}

type NERResult struct {
    Text       string
    Type       string  // PERSON, LOCATION, ORGANIZATION
    Start      int
    End        int
    Confidence float64
}
```

**识别类型**：
- PERSON：人名
- LOCATION：地名
- ORGANIZATION：组织机构名
- DATE：日期时间
- MONEY：金额

**优化策略**：
- 缓存NER结果（LRU缓存）
- 批量处理
- 异步预加载模型

**性能目标**：
- 延迟：15-35ms
- 准确率：85-92%

---

### Layer 4: 上下文规则层

**功能**：
- 上下文感知分析
- 消除误报
- 实体关联分析

**规则格式**：
```go
type ContextRule struct {
    ID          string
    Type        SensitiveType
    Pattern     string
    Triggers    []string  // 触发词
    Suppressors []string  // 抑制词
    WindowSize  int       // 上下文窗口大小
    Associations []Association
    BaseScore   float64
}

type Association struct {
    EntityType string   // PERSON, LOCATION, etc.
    Distance   int      // 最大距离
    Boost      float64  // 置信度加成
}
```

**典型规则示例**：

**规则1：密码上下文检测**
```json
{
  "id": "pwd_context_001",
  "type": "password",
  "pattern": "[A-Za-z0-9@#$%]{6,}",
  "triggers": ["密码", "password", "pwd", "口令"],
  "suppressors": ["字段", "变量", "单词", "英文"],
  "windowSize": 20,
  "baseScore": 0.8
}
```

**规则2：手机号关联检测**
```json
{
  "id": "phone_assoc_002",
  "type": "phone",
  "pattern": "1[3-9]\\d{9}",
  "triggers": ["手机", "电话", "联系方式"],
  "associations": [{
    "entityType": "PERSON",
    "distance": 15,
    "boost": 0.3
  }],
  "baseScore": 0.7
}
```

**置信度计算**：
```
Score = BaseScore 
      + TriggerBoost (每个触发词 +0.1)
      - SuppressorPenalty (每个抑制词 -0.2)
      + AssociationBoost (关联实体加成)
```

**实现要点**：
- 分词和词性标注
- 滑动窗口分析
- 实体距离计算
- 规则优先级排序

**性能目标**：
- 延迟：<10ms
- 准确率：88%

---

### Layer 5: LLM智能检测层

**功能**：
- 检测隐晦、复杂的敏感表达
- 理解深层语义
- 处理前4层无法识别的情况

**Prompt模板**：
```
你是敏感信息检测专家。分析文本中的隐晦敏感表达，返回JSON。

规则：
- 检测隐喻、暗示、变体表达
- 忽略已标记位置：{{.MarkedRanges}}
- 类型：个人信息/认证凭证/财务信息/其他

文本：{{.Text}}

输出格式：
{
  "items": [
    {
      "type": "类型",
      "start": 起始位置,
      "end": 结束位置,
      "confidence": 0-1,
      "reason": "简短原因"
    }
  ]
}

无敏感内容返回：{"items":[]}
```

**输入输出格式**：
```go
type LLMDetectionRequest struct {
    Text         string    `json:"text"`
    MarkedRanges [][2]int  `json:"marked_ranges"` // 已检测区域
}

type LLMDetectionResponse struct {
    Items []struct {
        Type       string  `json:"type"`
        Start      int     `json:"start"`
        End        int     `json:"end"`
        Confidence float64 `json:"confidence"`
        Reason     string  `json:"reason"`
    } `json:"items"`
}
```

**性能优化**：
1. **分段处理**：超过500字符分批
2. **并发调用**：多段并行检测
3. **置信度阈值**：>0.7才返回
4. **缓存机制**：相似文本缓存结果（hash）
5. **超时控制**：400ms超时

**不确定性处理**：
- 置信度<0.7标记为"待审核"
- 多次检测结果投票
- 人工反馈训练Few-shot示例

**性能目标**：
- 延迟：<500ms
- 准确率：95%+

---

### Layer 6: 融合决策层

**功能**：
- 综合多层检测结果
- 计算最终置信度
- 解决层间冲突

**融合算法**：

**1. 分层级联策略**（性能优先）
```
输入 → L1检测 → 置信度≥0.9? 是→输出
              ↓否
           L2检测 → 综合置信度≥0.9? 是→输出
              ↓否
           L3检测 → 应用冲突解决
              ↓
           L4检测 → 继续融合
              ↓
           L5检测 → 按需调用
              ↓
           加权融合 → 最终决策
```

**2. 置信度计算公式**
```
最终置信度 = Σ(层权重ᵢ × 层置信度ᵢ × 一致性因子)

权重分配：
- L1: 0.10 (正则)
- L2: 0.15 (词典)
- L3: 0.20 (NER)
- L4: 0.15 (上下文)
- L5: 0.40 (LLM)

一致性因子 = 1 + 0.1 × (一致层数 - 1)
```

**3. 冲突解决策略**
- **高准确率层优先**：L5 > L3 > L2 > L4 > L1
- **多数投票**：3层以上一致则采纳
- **置信度加权**：冲突时选择加权置信度最高的结果
- **阈值保护**：所有层置信度<0.6则标记为"不确定"

**4. 优化策略**
- 缓存高频模式（L1/L2命中率高）
- L5按需调用（前4层不确定时）
- 并行执行L1-L4（独立层）
- 动态阈值调整（根据历史准确率）

**实现结构**：
```go
type FusionEngine struct {
    layers       []DetectionLayer
    weights      map[int]float64
    threshold    float64
    cache        *lru.Cache
    stats        *FusionStats
}

type DetectionResult struct {
    LayerID      int
    Items        []SensitiveInfo
    Confidence   float64
    ProcessTime  time.Duration
}

type FusionDecision struct {
    FinalItems      []SensitiveInfo
    FinalConfidence float64
    LayersUsed      []int
    ConflictsResolved int
    TotalTime       time.Duration
}
```

---

## 📈 性能指标

### 整体性能目标

| 指标 | 目标值 | 说明 |
|------|--------|------|
| **平均延迟** | <100ms | 90%的请求 |
| **P99延迟** | <500ms | 包含LLM调用 |
| **准确率** | >92% | 综合准确率 |
| **召回率** | >90% | 敏感信息召回 |
| **误报率** | <5% | 假阳性率 |

### 分层性能

| 层级 | 延迟 | 准确率 | 调用率 |
|------|------|--------|--------|
| L1 | <5ms | 70-80% | 100% |
| L2 | <2ms | 90% | 80% |
| L3 | 15-35ms | 85-92% | 50% |
| L4 | <10ms | 88% | 40% |
| L5 | <500ms | 95%+ | 10% |

---

## 🔧 实施步骤

### Phase 1: 词典匹配层（1-2天）
1. 实现Trie树数据结构
2. 实现AC自动机算法
3. 加载敏感词词典
4. 集成到检测流程
5. 性能测试和优化

### Phase 2: NER实体识别层（2-3天）
1. 集成HanLP Go库
2. 加载中文NER模型
3. 实现缓存机制
4. 与正则层配合
5. 准确率测试

### Phase 3: 上下文规则层（2-3天）
1. 设计规则格式
2. 实现规则引擎
3. 编写典型规则
4. 实现置信度计算
5. 关联分析实现

### Phase 4: LLM智能检测层（2-3天）
1. 设计Prompt模板
2. 实现Ollama调用
3. 分段处理逻辑
4. 缓存机制
5. 性能优化

### Phase 5: 融合决策层（2-3天）
1. 实现融合算法
2. 置信度计算
3. 冲突解决
4. 性能优化
5. 完整测试

### Phase 6: 测试和优化（2-3天）
1. 编写测试用例
2. 准确率测试
3. 性能压测
4. 优化调整
5. 文档完善

**总计：12-18天**

---

## 🎯 竞赛优势

### 技术深度
- ✅ 6层级联检测架构
- ✅ 多种技术融合（正则+词典+NER+规则+LLM）
- ✅ 智能置信度计算
- ✅ 冲突解决机制

### 创新性
- ✅ 分层级联策略（性能优化）
- ✅ 上下文感知检测
- ✅ LLM辅助检测
- ✅ 多层融合决策

### 实用性
- ✅ 高准确率（>92%）
- ✅ 低延迟（<100ms平均）
- ✅ 可扩展架构
- ✅ 完整审计追踪

### 系统完整性
- ✅ 前端+后端全链路保护
- ✅ 多层防护架构
- ✅ 完整的审计日志
- ✅ 可视化展示

---

## 📚 参考资料

### 技术文档
- HanLP: https://github.com/hankcs/hanlp
- Ollama: https://ollama.ai/
- AC自动机: https://en.wikipedia.org/wiki/Aho%E2%80%93Corasick_algorithm

### 相关论文
- "Named Entity Recognition: A Survey"
- "Context-Aware Sensitive Information Detection"
- "Multi-Layer Fusion for Text Classification"

---

**版本**: v2.0
**更新时间**: 2026-05-10
**作者**: Context-Keeper Team
