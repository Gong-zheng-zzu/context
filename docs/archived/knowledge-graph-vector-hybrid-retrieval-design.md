# 知识图谱与向量混合检索架构设计方案

> 版本: v1.0
> 日期: 2026-01-13
> 作者: Context-Keeper架构组

## 一、背景与问题分析

### 1.1 当前架构问题

通过对知识图谱检索链路的深度分析，发现以下核心问题：

#### 问题1：实体提取粒度不当
- **现象**：LLM提取实体时过度拆分，如将"Redis连接池"拆分为"Redis"和"连接池"
- **影响**：检索时无法精确匹配，"Redis"匹配到大量无关内容，"连接池"匹配到各类连接池

#### 问题2：全文索引查询精度低
- **现象**：用户查询"Redis连接池常见问题"被分词后OR匹配
- **影响**：召回精度仅约40%，大量无关实体被返回

#### 问题3：近义词/同义词无法匹配
- **现象**：存储时用"Redis连接池"，检索时用"Redis连接池配置"
- **影响**：图谱字符串匹配失败，相关知识无法召回

#### 问题4：向量与图谱未协同
- **现象**：向量检索和图谱检索独立运行，各自为战
- **影响**：向量的语义泛化能力和图谱的结构化推理能力未能互补

### 1.2 核心设计目标

1. **语义精准**：实体提取保持"语义最小完整单元"，不过度拆分
2. **近义词覆盖**：通过向量语义相似度解决近义词匹配问题
3. **结构化推理**：利用图谱关系进行知识展开和推理
4. **最小改动**：兼容现有架构，增量式优化

---

## 二、核心概念定义

### 2.1 语义最小完整单元

**定义**：一个语义完整、在当前上下文中不可再分的概念单元。

**判断标准**：
```
组合是否产生新语义？
├── 是 → 保持整体（如"Redis连接池"是特定机制，不等于"Redis"+"连接池"）
└── 否 → 可以分开（如"Redis和MySQL"是两个独立概念的并列）
```

**示例对比**：

| 原文 | 正确提取 | 错误提取 | 原因 |
|-----|---------|---------|------|
| Redis连接池配置 | "Redis连接池", "配置" | "Redis", "连接池", "配置" | "Redis连接池"是特定机制 |
| Redis和MySQL对比 | "Redis", "MySQL" | "Redis和MySQL" | 两个独立概念的并列 |
| Spring Boot应用 | "Spring Boot", "应用" | "Spring", "Boot", "应用" | "Spring Boot"是特定框架 |
| 前端和后端分离 | "前端", "后端", "分离架构" | "前端和后端分离" | 三个独立概念 |

### 2.2 语义边界与近似度

**重要区分**：

```
❌ 错误理解：
"数据库连接池" ≈ "Redis连接池"  （错！不同限定词 = 不同语义边界）

✅ 正确理解：
"Redis连接池" ≈ "Redis连接池配置"  （对！研究的问题边界相近）
"Redis连接池" ≈ "Redis连接池优化"  （对！同一技术对象的不同方面）
"Redis连接池" ≠ "MySQL连接池"      （不同！不同数据库的连接池）
```

**语义边界判断规则**：
1. **限定词改变语义边界**：Redis连接池 ≠ MySQL连接池 ≠ 数据库连接池（泛指）
2. **修饰词不改变核心边界**：Redis连接池 ≈ Redis连接池配置 ≈ Redis连接池优化
3. **上下位关系需区分**："连接池"是上位概念，"Redis连接池"是下位概念，不能等同

---

## 三、向量存储架构设计

### 3.1 当前架构

```
┌─────────────────────────────────────────┐
│           当前向量存储结构                │
├─────────────────────────────────────────┤
│                                         │
│  DashVector/Vearch Collection           │
│  ┌───────────────────────────────────┐  │
│  │ id: "memory-uuid-xxx"             │  │
│  │ content: "完整的摘要文本..."       │  │
│  │ vector: [0.1, 0.2, ...]          │  │  ← 仅摘要级向量
│  │ metadata: {                       │  │
│  │   user_id, session_id,           │  │
│  │   priority, timestamp            │  │
│  │ }                                │  │
│  └───────────────────────────────────┘  │
│                                         │
│  问题：没有实体级别的向量索引             │
│        无法解决近义词匹配问题             │
│                                         │
└─────────────────────────────────────────┘
```

### 3.2 目标架构：混合向量空间

**核心思想**：在同一向量Collection中存储两种类型的向量记录

```
┌─────────────────────────────────────────────────────────────────┐
│                    混合向量空间架构                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  同一个Collection，通过 metadata.type 区分两种记录               │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                                                         │   │
│  │  Type: "memory" （原有摘要向量，完全不变）                │   │
│  │  ──────────────────────────────────────────────────────│   │
│  │  {                                                      │   │
│  │    id: "memory-uuid-xxx",                              │   │
│  │    content: "完整的摘要文本...",                        │   │
│  │    vector: [0.1, 0.2, ...],                            │   │
│  │    metadata: {                                          │   │
│  │      type: "memory",          // 标识为memory类型       │   │
│  │      user_id: "xxx",                                   │   │
│  │      session_id: "xxx",                                │   │
│  │      priority: "P1",                                   │   │
│  │      entity_ids: ["ent-1", "ent-2"]  // 新增：关联实体  │   │
│  │    }                                                   │   │
│  │  }                                                      │   │
│  │                                                         │   │
│  │  Type: "entity" （新增实体向量）                         │   │
│  │  ──────────────────────────────────────────────────────│   │
│  │  {                                                      │   │
│  │    id: "entity-uuid-yyy",                              │   │
│  │    content: "Redis连接池",      // 实体名作为content    │   │
│  │    vector: [0.3, 0.4, ...],     // 实体名的embedding   │   │
│  │    metadata: {                                          │   │
│  │      type: "entity",            // 标识为entity类型     │   │
│  │      entity_type: "Technical",  // 实体类型            │   │
│  │      user_id: "xxx",                                   │   │
│  │      memory_ids: ["mem-1", "mem-2"],  // 来源memory列表│   │
│  │      neo4j_node_id: "xxx"       // 关联的图谱节点ID    │   │
│  │    }                                                   │   │
│  │  }                                                      │   │
│  │                                                         │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 3.3 数据关联设计

**双向关联机制**：

```
┌─────────────────────────────────────────────────────────────────┐
│                       双向关联设计                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   Memory记录                          Entity记录                 │
│   ┌───────────────┐                  ┌───────────────┐          │
│   │ id: "mem-1"   │                  │ id: "ent-1"   │          │
│   │               │    entity_ids    │               │          │
│   │ metadata: {   │ ───────────────► │ content:      │          │
│   │   entity_ids: │                  │ "Redis连接池" │          │
│   │   ["ent-1",   │ ◄─────────────── │               │          │
│   │    "ent-2"]   │    memory_ids    │ metadata: {   │          │
│   │ }             │                  │   memory_ids: │          │
│   └───────────────┘                  │   ["mem-1",   │          │
│                                      │    "mem-3"]   │          │
│                                      │ }             │          │
│                                      └───────────────┘          │
│                                             │                   │
│                                             │ neo4j_node_id     │
│                                             ▼                   │
│                                      ┌───────────────┐          │
│                                      │  Neo4j节点    │          │
│                                      │ (Entity)      │          │
│                                      │               │          │
│                                      │ 图谱关系展开  │          │
│                                      └───────────────┘          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**关联作用**：
- `memory.entity_ids` → 从memory找到其包含的实体
- `entity.memory_ids` → 从实体找到它出现在哪些memory中（跨memory聚合）
- `entity.neo4j_node_id` → 从向量实体跳转到图谱进行关系展开

### 3.4 实体去重策略

**问题**：同一实体可能在多个memory中出现，如何避免重复存储？

**策略**：基于 (entity_name + user_id) 唯一性约束

```go
// 伪代码：实体向量存储逻辑
func storeEntityVector(entity Entity, memoryID string, userID string) {
    // 1. 构建唯一标识
    entityKey := hash(entity.Name + userID)

    // 2. 检查是否已存在
    existingEntity := vectorDB.Get(filter: {
        "metadata.type": "entity",
        "content": entity.Name,
        "metadata.user_id": userID
    })

    if existingEntity != nil {
        // 3a. 已存在：追加memory_id到列表
        existingEntity.metadata.memory_ids = append(
            existingEntity.metadata.memory_ids,
            memoryID
        )
        vectorDB.Update(existingEntity)
    } else {
        // 3b. 不存在：新建实体向量记录
        entityVector := embedding.Generate(entity.Name)
        vectorDB.Insert({
            id: generateUUID(),
            content: entity.Name,
            vector: entityVector,
            metadata: {
                type: "entity",
                entity_type: entity.Type,
                user_id: userID,
                memory_ids: [memoryID],
                neo4j_node_id: entity.Neo4jID
            }
        })
    }
}
```

---

## 四、检索链路设计

### 4.1 整体检索流程

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         向量+图谱联合检索流程                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  用户查询: "Redis连接池配置优化"                                          │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Step 1: 计算查询向量                                             │   │
│  │ query_vector = embedding("Redis连接池配置优化")                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Step 2: 双路并行向量检索                                          │   │
│  │ ─────────────────────────────────────────────────────────────── │   │
│  │                                                                 │   │
│  │  路径A: Memory向量检索（原有逻辑，不变）                          │   │
│  │  ┌───────────────────────────────────────────────────────────┐ │   │
│  │  │ search(vector=query_vector,                               │ │   │
│  │  │        filter="type='memory' AND user_id='xxx'",          │ │   │
│  │  │        top_k=10)                                          │ │   │
│  │  │ → 返回语义相关的memory摘要                                 │ │   │
│  │  └───────────────────────────────────────────────────────────┘ │   │
│  │                                                                 │   │
│  │  路径B: Entity向量检索（新增）                                   │   │
│  │  ┌───────────────────────────────────────────────────────────┐ │   │
│  │  │ search(vector=query_vector,                               │ │   │
│  │  │        filter="type='entity' AND user_id='xxx'",          │ │   │
│  │  │        top_k=10)                                          │ │   │
│  │  │ → 返回语义相似的实体：                                     │ │   │
│  │  │   - "Redis连接池" (similarity: 0.92)                      │ │   │
│  │  │   - "Redis连接池配置" (similarity: 0.95)  ← 近义词命中    │ │   │
│  │  │   - "连接池优化" (similarity: 0.78)                       │ │   │
│  │  └───────────────────────────────────────────────────────────┘ │   │
│  │                                                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Step 3: 图谱关系展开（以Entity向量检索结果为入口）                 │   │
│  │ ─────────────────────────────────────────────────────────────── │   │
│  │                                                                 │   │
│  │  以"Redis连接池"为入口，在Neo4j中展开1-2跳关系：                  │   │
│  │                                                                 │   │
│  │  (Redis连接池)                                                  │   │
│  │       │                                                         │   │
│  │       ├──[HAS_CONFIG]──► (maxTotal配置)                         │   │
│  │       │                                                         │   │
│  │       ├──[HAS_CONFIG]──► (maxIdle配置)                          │   │
│  │       │                                                         │   │
│  │       ├──[HAS_ISSUE]──► (连接超时)                              │   │
│  │       │                     │                                   │   │
│  │       │                     └──[SOLVED_BY]──► (调整maxTotal)    │   │
│  │       │                                                         │   │
│  │       └──[RELATED_TO]──► (Jedis客户端)                          │   │
│  │                                                                 │   │
│  │  关系展开带来的实体 → 通过neo4j_node_id关联的memory_ids          │   │
│  │                                                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Step 4: 多源结果融合                                              │   │
│  │ ─────────────────────────────────────────────────────────────── │   │
│  │                                                                 │   │
│  │  来源1: Memory向量直接检索结果                                   │   │
│  │  来源2: Entity向量检索 → entity.memory_ids                      │   │
│  │  来源3: 图谱展开实体 → 实体关联的memory_ids                      │   │
│  │                                                                 │   │
│  │  融合策略: RRF (Reciprocal Rank Fusion)                         │   │
│  │  去重: 基于memory_id去重                                         │   │
│  │  排序: 综合相似度 + 来源权重                                     │   │
│  │                                                                 │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       │                                                                 │
│       ▼                                                                 │
│  最终返回: 排序后的memory列表 + 相关实体 + 关系路径                       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 4.2 Entity向量检索的价值

**解决的核心问题：近义词/同义词匹配**

| 用户查询 | 存储的实体 | 字符串匹配 | 向量匹配 |
|---------|-----------|-----------|---------|
| "Redis连接池配置" | "Redis连接池" | ❌ 失败 | ✅ 0.92相似 |
| "Redis连接池优化" | "Redis连接池" | ❌ 失败 | ✅ 0.90相似 |
| "Redis连接池问题" | "Redis连接池" | ❌ 失败 | ✅ 0.88相似 |
| "Jedis连接池" | "Redis连接池" | ❌ 失败 | ✅ 0.75相似 |

**关键点**：
- 向量相似度能捕捉语义相近性
- "Redis连接池配置" 和 "Redis连接池" 在向量空间中距离很近
- 即使表述不完全相同，也能召回相关实体

### 4.3 图谱展开的价值

**解决的核心问题：结构化知识推理**

```
场景：用户问"Redis连接池超时怎么解决"

1. Entity向量检索找到入口实体: "Redis连接池" (similarity: 0.85)

2. 图谱展开发现关系链:
   (Redis连接池) --[HAS_ISSUE]--> (连接超时) --[SOLVED_BY]--> (maxTotal调优)

3. 通过关系推理，找到解决方案实体: "maxTotal调优"

4. 该实体关联的memory包含具体的解决方案描述
```

**图谱的独特价值**：
- 不仅找到语义相关的内容，还能找到"问题→解决方案"的因果链
- 向量只能做相似度匹配，图谱能做关系推理

---

## 五、实体提取Prompt设计

### 5.1 设计原则

1. **语义完整性优先**：实体应是"语义最小完整单元"，不过度拆分
2. **限定词敏感**：不同限定词产生不同语义边界
3. **Few-shot精准**：示例要覆盖典型场景和边界情况
4. **输出结构化**：便于程序解析

### 5.2 完整Prompt模板

```markdown
## 知识实体提取任务

你是一个专业的知识图谱构建专家，负责从技术文本中提取实体和关系。

### 核心原则：语义最小完整单元

提取的每个实体必须是一个**语义完整、边界清晰**的概念单元。

### 语义边界判断规则

**规则1：限定词改变语义边界**
- 不同的限定词 = 不同的实体
- "Redis连接池" ≠ "MySQL连接池" ≠ "数据库连接池"（三个不同的实体）
- "Redis连接池" 是特指，"数据库连接池" 是泛指

**规则2：修饰词不改变核心边界**
- 同一核心概念的不同方面 = 可合并或分开
- "Redis连接池" ≈ "Redis连接池配置" ≈ "Redis连接池优化"
- 这些在语义上是相近的，但如果上下文中分别强调，可以作为独立实体

**规则3：组合产生新语义时保持整体**
- "Redis" + "连接池" 组合后产生新含义（特指Redis的连接池机制）
- 应提取为 "Redis连接池"，而非拆分为 "Redis" 和 "连接池"

**规则4：简单并列时可分开**
- "Redis和MySQL" → 应分开为 "Redis", "MySQL"
- 它们是两个独立概念的并列关系

### 实体类型定义

| 类型 | 英文 | 说明 | 示例 |
|-----|------|------|------|
| 技术实体 | Technical | 技术、工具、框架、系统、具体技术方案 | Redis连接池、Spring Boot、Kafka消息队列 |
| 问题实体 | Issue | 具体问题、故障现象、异常情况 | 连接超时、OOM异常、死锁问题 |
| 解决方案 | Solution | 解决问题的方法、优化措施 | 连接池调优、分库分表、缓存预热 |
| 配置实体 | Config | 配置项、参数、设置 | maxTotal配置、timeout设置 |
| 概念实体 | Concept | 抽象概念、设计模式、架构理念 | 微服务架构、CQRS模式、领域驱动设计 |

### 关系类型定义

| 关系 | 说明 | 示例 |
|------|------|------|
| HAS_ISSUE | 技术/系统存在的问题 | (Redis连接池)--[HAS_ISSUE]-->(连接超时) |
| SOLVED_BY | 问题的解决方案 | (连接超时)--[SOLVED_BY]-->(maxTotal调优) |
| HAS_CONFIG | 技术的配置项 | (Redis连接池)--[HAS_CONFIG]-->(maxTotal) |
| DEPENDS_ON | 依赖关系 | (订单服务)--[DEPENDS_ON]-->(Redis) |
| RELATED_TO | 相关关系 | (Redis连接池)--[RELATED_TO]-->(Jedis客户端) |
| CAUSES | 导致关系 | (配置不当)--[CAUSES]-->(连接超时) |

---

### Few-Shot 示例（重要！请仔细学习）

#### 示例1：技术问题场景（正确的语义完整提取）

**输入**：
```
Redis连接池配置不当导致生产环境连接超时，分析发现maxTotal设置为8太小，
高并发时连接池耗尽。将maxTotal调整为200，maxIdle调整为50后问题解决。
```

**正确输出**：
```json
{
  "entities": [
    {"name": "Redis连接池", "type": "Technical", "description": "Redis客户端连接池机制"},
    {"name": "连接超时", "type": "Issue", "description": "连接池获取连接超时问题"},
    {"name": "连接池耗尽", "type": "Issue", "description": "高并发下连接池资源用完"},
    {"name": "maxTotal配置", "type": "Config", "description": "连接池最大连接数配置"},
    {"name": "maxIdle配置", "type": "Config", "description": "连接池最大空闲连接数配置"},
    {"name": "生产环境", "type": "Technical", "description": "生产部署环境"}
  ],
  "relationships": [
    {"source": "Redis连接池", "target": "连接超时", "type": "HAS_ISSUE"},
    {"source": "Redis连接池", "target": "连接池耗尽", "type": "HAS_ISSUE"},
    {"source": "连接池耗尽", "target": "连接超时", "type": "CAUSES"},
    {"source": "Redis连接池", "target": "maxTotal配置", "type": "HAS_CONFIG"},
    {"source": "Redis连接池", "target": "maxIdle配置", "type": "HAS_CONFIG"},
    {"source": "maxTotal配置", "target": "连接超时", "type": "SOLVED_BY"},
    {"source": "maxIdle配置", "target": "连接超时", "type": "SOLVED_BY"}
  ]
}
```

**为什么这样提取**：
- ✅ "Redis连接池" 作为整体，不拆分为 "Redis" + "连接池"
- ✅ "连接超时" 是具体问题，不是泛化的 "超时"
- ✅ "maxTotal配置" 是具体配置项，包含了配置项名称
- ✅ 关系链完整：问题→原因→解决方案

**错误示范（避免）**：
```json
{
  "entities": [
    {"name": "Redis", "type": "Technical"},      // ❌ 过度拆分
    {"name": "连接池", "type": "Technical"},      // ❌ 过度拆分，语义不完整
    {"name": "超时", "type": "Issue"},           // ❌ 太泛化
    {"name": "maxTotal", "type": "Config"}       // ❌ 缺少"配置"说明
  ]
}
```

---

#### 示例2：多技术对比场景（正确识别并列关系）

**输入**：
```
对比了Redis和MySQL作为缓存层的性能差异。Redis读取QPS可达10万，
MySQL只有1万。最终选择Redis作为一级缓存，MySQL作为持久化存储。
```

**正确输出**：
```json
{
  "entities": [
    {"name": "Redis", "type": "Technical", "description": "NoSQL内存数据库"},
    {"name": "MySQL", "type": "Technical", "description": "关系型数据库"},
    {"name": "缓存层", "type": "Concept", "description": "系统缓存架构层"},
    {"name": "一级缓存", "type": "Concept", "description": "最快访问的缓存层"},
    {"name": "持久化存储", "type": "Concept", "description": "数据持久化层"}
  ],
  "relationships": [
    {"source": "Redis", "target": "一级缓存", "type": "USED_AS"},
    {"source": "MySQL", "target": "持久化存储", "type": "USED_AS"},
    {"source": "Redis", "target": "MySQL", "type": "RELATED_TO", "context": "性能对比"}
  ]
}
```

**为什么这样提取**：
- ✅ "Redis" 和 "MySQL" 是两个独立技术，并列关系，分开提取
- ✅ 没有错误地合并为 "Redis和MySQL"
- ✅ 抽象概念如 "缓存层"、"一级缓存" 也作为实体提取

---

#### 示例3：复杂业务场景（识别语义边界）

**输入**：
```
订单服务调用库存服务时出现分布式事务问题。采用Saga模式解决，
通过Kafka消息队列实现最终一致性。涉及订单数据库和库存数据库的数据同步。
```

**正确输出**：
```json
{
  "entities": [
    {"name": "订单服务", "type": "Technical", "description": "订单业务微服务"},
    {"name": "库存服务", "type": "Technical", "description": "库存业务微服务"},
    {"name": "分布式事务问题", "type": "Issue", "description": "跨服务事务一致性问题"},
    {"name": "Saga模式", "type": "Solution", "description": "分布式事务解决模式"},
    {"name": "Kafka消息队列", "type": "Technical", "description": "消息中间件"},
    {"name": "最终一致性", "type": "Concept", "description": "分布式一致性模型"},
    {"name": "订单数据库", "type": "Technical", "description": "订单服务数据存储"},
    {"name": "库存数据库", "type": "Technical", "description": "库存服务数据存储"}
  ],
  "relationships": [
    {"source": "订单服务", "target": "库存服务", "type": "DEPENDS_ON"},
    {"source": "订单服务", "target": "分布式事务问题", "type": "HAS_ISSUE"},
    {"source": "Saga模式", "target": "分布式事务问题", "type": "SOLVED_BY"},
    {"source": "Saga模式", "target": "Kafka消息队列", "type": "DEPENDS_ON"},
    {"source": "Saga模式", "target": "最终一致性", "type": "RELATED_TO"}
  ]
}
```

**为什么这样提取**：
- ✅ "订单服务" 和 "库存服务" 是两个不同的服务实体
- ✅ "分布式事务问题" 是具体问题，不是泛化的 "事务问题"
- ✅ "Saga模式" 是解决方案实体
- ✅ "Kafka消息队列" 保持完整，不拆分为 "Kafka" + "消息队列"

---

#### 示例4：边界情况（区分泛指和特指）

**输入**：
```
优化了数据库连接池的配置，主要调整了MySQL连接池和Redis连接池的参数。
MySQL连接池maxActive从20增加到100，Redis连接池maxTotal从50增加到200。
```

**正确输出**：
```json
{
  "entities": [
    {"name": "数据库连接池", "type": "Concept", "description": "连接池概念（泛指）"},
    {"name": "MySQL连接池", "type": "Technical", "description": "MySQL数据库连接池"},
    {"name": "Redis连接池", "type": "Technical", "description": "Redis连接池"},
    {"name": "maxActive配置", "type": "Config", "description": "MySQL连接池最大活跃连接"},
    {"name": "maxTotal配置", "type": "Config", "description": "Redis连接池最大连接数"}
  ],
  "relationships": [
    {"source": "MySQL连接池", "target": "数据库连接池", "type": "IS_A"},
    {"source": "Redis连接池", "target": "数据库连接池", "type": "IS_A"},
    {"source": "MySQL连接池", "target": "maxActive配置", "type": "HAS_CONFIG"},
    {"source": "Redis连接池", "target": "maxTotal配置", "type": "HAS_CONFIG"}
  ]
}
```

**为什么这样提取**：
- ✅ "数据库连接池" 是泛指概念，作为独立实体
- ✅ "MySQL连接池" 和 "Redis连接池" 是特指，各自独立
- ✅ 三者关系通过 IS_A（上下位关系）连接
- ✅ 不同连接池的配置参数名不同（maxActive vs maxTotal），分别提取

---

### 输出格式要求

请严格按照以下JSON格式输出：

```json
{
  "entities": [
    {
      "name": "实体名称（语义完整单元）",
      "type": "Technical|Issue|Solution|Config|Concept",
      "description": "简短描述"
    }
  ],
  "relationships": [
    {
      "source": "源实体名",
      "target": "目标实体名",
      "type": "HAS_ISSUE|SOLVED_BY|HAS_CONFIG|DEPENDS_ON|RELATED_TO|CAUSES|IS_A",
      "context": "可选：关系的具体上下文说明"
    }
  ]
}
```

---

### 现在请分析以下文本，提取实体和关系：

{用户输入的文本内容}
```

---

## 六、代码实现方案

### 6.1 存储链路改造

#### 6.1.1 实体向量存储服务（新增）

**文件位置**：`internal/services/entity_vector_service.go`

```go
package services

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "log"
    "time"
)

// EntityVectorService 实体向量存储服务
type EntityVectorService struct {
    vectorService  *VectorService    // 复用现有向量服务
    embeddingService EmbeddingService // embedding生成服务
}

// EntityVectorRecord 实体向量记录
type EntityVectorRecord struct {
    ID          string   `json:"id"`
    Content     string   `json:"content"`      // 实体名作为content
    Vector      []float32 `json:"vector"`
    Metadata    EntityMetadata `json:"metadata"`
}

// EntityMetadata 实体元数据
type EntityMetadata struct {
    Type        string   `json:"type"`          // 固定为 "entity"
    EntityType  string   `json:"entity_type"`   // Technical/Issue/Solution等
    UserID      string   `json:"user_id"`
    MemoryIDs   []string `json:"memory_ids"`    // 关联的memory列表
    Neo4jNodeID string   `json:"neo4j_node_id"` // 图谱节点ID
    Description string   `json:"description"`   // 实体描述
    CreatedAt   int64    `json:"created_at"`
    UpdatedAt   int64    `json:"updated_at"`
}

// StoreEntityVectors 批量存储实体向量
func (s *EntityVectorService) StoreEntityVectors(
    ctx context.Context,
    entities []ExtractedEntity,
    memoryID string,
    userID string,
) error {
    log.Printf("🔄 [实体向量] 开始存储 %d 个实体向量", len(entities))

    for _, entity := range entities {
        // 1. 检查实体是否已存在
        existingID, exists := s.findExistingEntity(ctx, entity.Name, userID)

        if exists {
            // 2a. 已存在：追加memory_id
            err := s.appendMemoryID(ctx, existingID, memoryID)
            if err != nil {
                log.Printf("⚠️ [实体向量] 追加memory_id失败: %v", err)
                continue
            }
            log.Printf("✅ [实体向量] 实体已存在，追加关联: %s -> %s", entity.Name, memoryID)
        } else {
            // 2b. 不存在：新建实体向量记录
            err := s.createEntityVector(ctx, entity, memoryID, userID)
            if err != nil {
                log.Printf("⚠️ [实体向量] 创建实体向量失败: %v", err)
                continue
            }
            log.Printf("✅ [实体向量] 创建新实体向量: %s", entity.Name)
        }
    }

    return nil
}

// findExistingEntity 查找已存在的实体
func (s *EntityVectorService) findExistingEntity(
    ctx context.Context,
    entityName string,
    userID string,
) (string, bool) {
    // 通过向量搜索 + 精确过滤找到已存在的实体
    filter := fmt.Sprintf(`type="entity" AND user_id="%s"`, userID)

    // 计算实体名的向量
    entityVector, err := s.embeddingService.GenerateEmbedding(entityName)
    if err != nil {
        return "", false
    }

    // 高相似度搜索（阈值0.98以上认为是同一实体）
    results, err := s.vectorService.SearchWithFilter(ctx, entityVector, filter, 5)
    if err != nil {
        return "", false
    }

    for _, result := range results {
        if result.Score >= 0.98 && result.Content == entityName {
            return result.ID, true
        }
    }

    return "", false
}

// createEntityVector 创建新的实体向量记录
func (s *EntityVectorService) createEntityVector(
    ctx context.Context,
    entity ExtractedEntity,
    memoryID string,
    userID string,
) error {
    // 1. 生成实体名的向量
    entityVector, err := s.embeddingService.GenerateEmbedding(entity.Name)
    if err != nil {
        return fmt.Errorf("生成实体向量失败: %w", err)
    }

    // 2. 构建记录
    entityID := s.generateEntityID(entity.Name, userID)
    now := time.Now().Unix()

    record := EntityVectorRecord{
        ID:      entityID,
        Content: entity.Name,
        Vector:  entityVector,
        Metadata: EntityMetadata{
            Type:        "entity",
            EntityType:  entity.Type,
            UserID:      userID,
            MemoryIDs:   []string{memoryID},
            Neo4jNodeID: entity.Neo4jNodeID,
            Description: entity.Description,
            CreatedAt:   now,
            UpdatedAt:   now,
        },
    }

    // 3. 插入向量数据库
    return s.vectorService.InsertEntityRecord(ctx, record)
}

// generateEntityID 生成实体ID（基于实体名+用户ID的哈希）
func (s *EntityVectorService) generateEntityID(entityName, userID string) string {
    hash := sha256.Sum256([]byte(entityName + userID))
    return "entity-" + hex.EncodeToString(hash[:8])
}

// appendMemoryID 追加memory关联
func (s *EntityVectorService) appendMemoryID(
    ctx context.Context,
    entityID string,
    memoryID string,
) error {
    // 更新实体记录，追加memory_id到列表
    return s.vectorService.AppendEntityMemoryID(ctx, entityID, memoryID)
}
```

#### 6.1.2 存储链路集成点

**修改文件**：`internal/services/context_service.go`

在 `storeKnowledgeDataToNeo4j` 函数后增加实体向量存储：

```go
// executeParallelStorage 执行并行存储（修改版）
func (s *ContextService) executeParallelStorage(
    ctx context.Context,
    analysisResult *models.SmartAnalysisResult,
    req models.StoreContextRequest,
    memoryID string,
) error {
    var wg sync.WaitGroup
    var storageErrors []error
    var mutex sync.Mutex

    // 1. 向量存储 - Memory级别（原有逻辑）
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := s.storeMemoryVector(ctx, req, memoryID); err != nil {
            mutex.Lock()
            storageErrors = append(storageErrors, err)
            mutex.Unlock()
        }
    }()

    // 2. Neo4j图谱存储（原有逻辑）
    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := s.storeKnowledgeDataToNeo4j(ctx, analysisResult, req, memoryID); err != nil {
            mutex.Lock()
            storageErrors = append(storageErrors, err)
            mutex.Unlock()
        }
    }()

    // 3. 【新增】实体向量存储
    wg.Add(1)
    go func() {
        defer wg.Done()
        if analysisResult.KnowledgeGraphExtraction != nil {
            entities := s.convertToExtractedEntities(analysisResult.KnowledgeGraphExtraction.Entities)
            if err := s.entityVectorService.StoreEntityVectors(ctx, entities, memoryID, req.UserID); err != nil {
                log.Printf("⚠️ [实体向量存储] 存储失败: %v", err)
                // 实体向量存储失败不阻断主流程
            }
        }
    }()

    wg.Wait()

    if len(storageErrors) > 0 {
        return fmt.Errorf("存储错误: %v", storageErrors)
    }
    return nil
}
```

### 6.2 检索链路改造

#### 6.2.1 实体向量检索（新增）

**文件位置**：`internal/services/llm_driven_context_service.go`

```go
// EntityVectorSearchResult 实体向量检索结果
type EntityVectorSearchResult struct {
    EntityName  string   `json:"entity_name"`
    EntityType  string   `json:"entity_type"`
    Similarity  float32  `json:"similarity"`
    MemoryIDs   []string `json:"memory_ids"`
    Neo4jNodeID string   `json:"neo4j_node_id"`
}

// searchEntityVectors 实体向量检索
func (s *LLMDrivenContextService) searchEntityVectors(
    ctx context.Context,
    query string,
    userID string,
    limit int,
) ([]EntityVectorSearchResult, error) {
    log.Printf("🔍 [实体向量检索] 开始检索: %s", query)

    // 1. 计算查询向量
    queryVector, err := s.embeddingService.GenerateEmbedding(query)
    if err != nil {
        return nil, fmt.Errorf("生成查询向量失败: %w", err)
    }

    // 2. 仅检索entity类型的记录
    filter := fmt.Sprintf(`type="entity" AND user_id="%s"`, userID)

    // 3. 执行向量检索
    results, err := s.vectorService.SearchWithFilter(ctx, queryVector, filter, limit)
    if err != nil {
        return nil, fmt.Errorf("实体向量检索失败: %w", err)
    }

    // 4. 转换结果
    entityResults := make([]EntityVectorSearchResult, 0, len(results))
    for _, r := range results {
        // 设置相似度阈值（0.7以上才认为相关）
        if r.Score < 0.7 {
            continue
        }

        entityResults = append(entityResults, EntityVectorSearchResult{
            EntityName:  r.Content,
            EntityType:  r.Metadata.EntityType,
            Similarity:  r.Score,
            MemoryIDs:   r.Metadata.MemoryIDs,
            Neo4jNodeID: r.Metadata.Neo4jNodeID,
        })
    }

    log.Printf("✅ [实体向量检索] 找到 %d 个相关实体", len(entityResults))
    return entityResults, nil
}
```

#### 6.2.2 图谱关系展开

```go
// expandGraphRelations 以实体为入口展开图谱关系
func (s *LLMDrivenContextService) expandGraphRelations(
    ctx context.Context,
    entityNodeIDs []string,
    userID string,
    maxDepth int,
) ([]GraphExpandedResult, error) {
    log.Printf("🕸️ [图谱展开] 开始展开 %d 个入口实体", len(entityNodeIDs))

    if s.knowledgeEngine == nil {
        return nil, fmt.Errorf("知识图谱引擎未初始化")
    }

    results := make([]GraphExpandedResult, 0)

    for _, nodeID := range entityNodeIDs {
        // 执行1-2跳关系展开
        cypher := `
            MATCH (e:Entity {id: $node_id, user_id: $user_id})
            OPTIONAL MATCH path = (e)-[r*1..2]-(related)
            WHERE related:Entity OR related:Event OR related:Solution
            RETURN e.name AS entry_name,
                   related.name AS related_name,
                   related.vector_id AS related_vector_id,
                   [rel in r | type(rel)] AS relation_types
            LIMIT $limit`

        params := map[string]interface{}{
            "node_id": nodeID,
            "user_id": userID,
            "limit":   20,
        }

        expanded, err := s.knowledgeEngine.ExecuteQuery(ctx, cypher, params)
        if err != nil {
            log.Printf("⚠️ [图谱展开] 展开失败: %v", err)
            continue
        }

        results = append(results, expanded...)
    }

    log.Printf("✅ [图谱展开] 展开完成，获得 %d 个关联结果", len(results))
    return results, nil
}
```

#### 6.2.3 多源结果融合

```go
// HybridRetrievalResult 混合检索结果
type HybridRetrievalResult struct {
    MemoryID    string  `json:"memory_id"`
    Content     string  `json:"content"`
    Score       float32 `json:"score"`
    Source      string  `json:"source"`  // "memory_vector" | "entity_vector" | "graph_expand"
    EntityPath  string  `json:"entity_path,omitempty"` // 图谱展开路径
}

// hybridRetrieval 混合检索（向量+图谱）
func (s *LLMDrivenContextService) hybridRetrieval(
    ctx context.Context,
    query string,
    userID string,
    limit int,
) ([]HybridRetrievalResult, error) {
    log.Printf("🔄 [混合检索] 开始执行: %s", query)

    var wg sync.WaitGroup
    var mutex sync.Mutex
    allResults := make([]HybridRetrievalResult, 0)

    // 路径A: Memory向量检索（原有逻辑）
    wg.Add(1)
    go func() {
        defer wg.Done()
        memoryResults, err := s.searchMemoryVectors(ctx, query, userID, limit)
        if err != nil {
            log.Printf("⚠️ [Memory向量检索] 失败: %v", err)
            return
        }

        mutex.Lock()
        for _, r := range memoryResults {
            allResults = append(allResults, HybridRetrievalResult{
                MemoryID: r.MemoryID,
                Content:  r.Content,
                Score:    r.Score,
                Source:   "memory_vector",
            })
        }
        mutex.Unlock()
    }()

    // 路径B: Entity向量检索 + 图谱展开
    wg.Add(1)
    go func() {
        defer wg.Done()

        // B1: 实体向量检索
        entityResults, err := s.searchEntityVectors(ctx, query, userID, 10)
        if err != nil {
            log.Printf("⚠️ [Entity向量检索] 失败: %v", err)
            return
        }

        // B2: 收集实体关联的memory_ids
        mutex.Lock()
        for _, e := range entityResults {
            for _, memID := range e.MemoryIDs {
                allResults = append(allResults, HybridRetrievalResult{
                    MemoryID:   memID,
                    Score:      e.Similarity * 0.9, // 略微降权
                    Source:     "entity_vector",
                    EntityPath: e.EntityName,
                })
            }
        }
        mutex.Unlock()

        // B3: 图谱关系展开
        nodeIDs := make([]string, 0)
        for _, e := range entityResults {
            if e.Neo4jNodeID != "" {
                nodeIDs = append(nodeIDs, e.Neo4jNodeID)
            }
        }

        if len(nodeIDs) > 0 {
            expandedResults, err := s.expandGraphRelations(ctx, nodeIDs, userID, 2)
            if err != nil {
                log.Printf("⚠️ [图谱展开] 失败: %v", err)
                return
            }

            mutex.Lock()
            for _, r := range expandedResults {
                // 通过展开实体找到关联的memory
                for _, memID := range r.RelatedMemoryIDs {
                    allResults = append(allResults, HybridRetrievalResult{
                        MemoryID:   memID,
                        Score:      0.7, // 图谱展开结果统一权重
                        Source:     "graph_expand",
                        EntityPath: r.RelationPath,
                    })
                }
            }
            mutex.Unlock()
        }
    }()

    wg.Wait()

    // 去重 + RRF融合排序
    finalResults := s.deduplicateAndRank(allResults, limit)

    log.Printf("✅ [混合检索] 完成，返回 %d 条结果", len(finalResults))
    return finalResults, nil
}

// deduplicateAndRank 去重并排序
func (s *LLMDrivenContextService) deduplicateAndRank(
    results []HybridRetrievalResult,
    limit int,
) []HybridRetrievalResult {
    // 按memory_id去重，保留最高分
    seen := make(map[string]HybridRetrievalResult)
    for _, r := range results {
        if existing, ok := seen[r.MemoryID]; ok {
            if r.Score > existing.Score {
                seen[r.MemoryID] = r
            }
        } else {
            seen[r.MemoryID] = r
        }
    }

    // 转换为切片并排序
    deduped := make([]HybridRetrievalResult, 0, len(seen))
    for _, r := range seen {
        deduped = append(deduped, r)
    }

    sort.Slice(deduped, func(i, j int) bool {
        return deduped[i].Score > deduped[j].Score
    })

    if len(deduped) > limit {
        return deduped[:limit]
    }
    return deduped
}
```

---

## 七、改动评估与实施计划

### 7.1 改动范围评估

| 模块 | 文件 | 改动类型 | 代码量 | 风险等级 |
|-----|------|---------|-------|---------|
| 实体向量服务 | `entity_vector_service.go` | 新增 | ~200行 | 低 |
| 向量服务扩展 | `vector_service.go` | 修改 | ~50行 | 中 |
| 存储链路集成 | `context_service.go` | 修改 | ~30行 | 低 |
| 检索链路改造 | `llm_driven_context_service.go` | 修改 | ~150行 | 中 |
| 图谱展开 | `engine.go` | 修改 | ~50行 | 低 |
| Prompt优化 | `context_service.go` | 修改 | ~100行 | 低 |
| **总计** | | | **~580行** | |

### 7.2 向量数据库改动

**DashVector/Vearch改动**：

需要确保向量数据库支持以下能力：
1. **metadata过滤**：`type="entity"` 过滤
2. **数组字段更新**：追加memory_id到memory_ids数组
3. **混合查询**：向量相似度 + 元数据过滤

**DashVector适配**（已支持）：
```go
// 过滤查询示例
filter := `type = "entity" AND user_id = "xxx"`
results, _ := collection.Query(vector, topK, filter)
```

### 7.3 兼容性保证

**原有逻辑不变**：
1. Memory向量存储/检索逻辑完全保留
2. Neo4j图谱存储逻辑完全保留
3. 现有API接口不变

**新增能力**：
1. 实体向量存储（并行执行，失败不阻断）
2. 实体向量检索（作为额外检索路径）
3. 混合结果融合

### 7.4 实施阶段

#### 阶段一：基础能力建设（1-2天）

- [ ] 新建 `entity_vector_service.go`
- [ ] 扩展 `vector_service.go` 支持entity类型
- [ ] 单元测试覆盖

#### 阶段二：存储链路集成（1天）

- [ ] 修改 `context_service.go` 集成实体向量存储
- [ ] 更新实体提取Prompt
- [ ] 集成测试

#### 阶段三：检索链路改造（1-2天）

- [ ] 实现实体向量检索
- [ ] 实现图谱关系展开
- [ ] 实现多源结果融合
- [ ] 端到端测试

#### 阶段四：效果验证与调优（1天）

- [ ] 召回率/精确率测试
- [ ] 相似度阈值调优
- [ ] 性能基准测试

### 7.5 预期效果

| 指标 | 当前值 | 预期值 | 提升幅度 |
|-----|-------|-------|---------|
| 知识图谱召回精度 | ~40% | ~75% | +35% |
| 近义词覆盖率 | ~20% | ~80% | +60% |
| 检索延迟 | 50ms | 80ms | +30ms（可接受） |
| 结果相关性评分 | 0.6 | 0.85 | +0.25 |

---

## 八、总结

### 8.1 核心创新点

1. **语义最小完整单元**：重新定义实体提取标准，保持语义完整性
2. **混合向量空间**：同一Collection存储Memory和Entity两种向量
3. **向量先行+图谱增强**：向量解决近义词，图谱解决结构化推理
4. **双向关联设计**：Memory←→Entity←→Neo4j三方关联

### 8.2 关键设计决策

| 决策点 | 选择 | 原因 |
|-------|------|------|
| 向量存储架构 | 混合空间 | 最小改动，复用现有基础设施 |
| 实体去重 | 基于(name+user_id)哈希 | 简单高效，避免重复存储 |
| 检索策略 | 双路并行 | Memory和Entity独立检索，不相互阻塞 |
| 结果融合 | RRF排序 | 平衡不同来源的贡献 |

### 8.3 后续优化方向

1. **实体聚类**：相似实体自动聚类，减少冗余
2. **关系权重学习**：根据使用反馈调整关系权重
3. **增量更新**：实体向量的增量更新机制
4. **缓存优化**：热门实体的向量缓存

---

*文档版本*：v1.0
*最后更新*：2026-01-13
*作者*：Context-Keeper架构组
