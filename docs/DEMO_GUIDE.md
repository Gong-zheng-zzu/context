# Context-Keeper 现场演示指南

## 演示目标
展示"忆安智护"AI Agent安全治理平台的四大核心创新功能，突出技术亮点和实用价值。

---

## 一、演示环境准备清单（提前1天完成）

### 1.1 基础环境检查
```bash
# 检查依赖服务状态
docker ps | grep -E "neo4j|vearch|ollama"
curl http://localhost:11434/api/tags  # 确认Ollama服务
curl http://localhost:7474  # 确认Neo4j Web界面
```

### 1.2 准备Demo数据
```bash
# 创建测试数据目录
mkdir -p demo_data/{nursing_records,user_vectors,retrieval_queries}

# 示例护理记录（用于因果推理演示）
cat > demo_data/nursing_records/sample1.txt << 'EOF'
患者李爷爷，78岁，长期服用降压药物（硝苯地平缓释片30mg，每日一次）。
今日上午10:30，护工小王发现李爷爷在洗手间摔倒，意识清醒，诉头晕。
测量血压90/60mmHg，较平时偏低。询问得知患者早餐后立即服药，随后起身如厕时突然眩晕失去平衡。
初步判断：体位性低血压导致跌倒风险。
处理措施：协助患者卧床休息，抬高下肢，监测血压变化，通知家属及主治医生。
EOF
```

### 1.3 初始化演示数据库
```bash
# 清空并初始化Neo4j图谱（仅演示环境）
cypher-shell -u neo4j -p your_password << 'EOF'
MATCH (n) DETACH DELETE n;
CREATE INDEX entity_name IF NOT EXISTS FOR (e:Entity) ON (e.name);
CREATE INDEX causal_role IF NOT EXISTS FOR (e:Entity) ON (e.causal_role);
EOF
```

### 1.4 预热LLM模型
```bash
# 预加载Qwen2.5模型到内存，避免首次调用延迟
ollama run qwen2.5:7b "你好，这是测试。" > /dev/null
```

---

## 二、演示流程设计（20分钟）

### 🎬 开场白（1分钟）
> "我们的系统面向养老护理数据治理：
> 1️⃣ 护理记录结构化为可审计的 O-M-P-R 因果证据
> 2️⃣ 用户和会话数据按范围隔离，并提供 Qdrant 用户级向量删除验证
> 3️⃣ 多源检索保留来源、排名、分数、延迟和降级状态
> 4️⃣ 敏感信息防护与检索决策日志支持追溯
> 下面我将演示这四项核心技术。"

---

### 📊 演示1：PCCM因果推理（5分钟）

**场景**：从护理记录自动发现"服用降压药→低血压→摔倒"的隐藏因果链

#### 步骤1：抽取因果关系
```bash
curl -X POST http://localhost:8080/api/v1/causal/extract \
  -H "Content-Type: application/json" \
  -d '{
    "text": "患者李爷爷，78岁，长期服用降压药...摔倒",
    "user_id": "demo_nurse_001"
  }'
```

**预期输出**（提前准备截图）：
```json
{
  "relations": [{
    "object": "李爷爷",
    "mediator": "服用降压药",
    "property": "体位性低血压",
    "result": "洗手间摔倒",
    "confidence": 0.87,
    "evidence": ["早餐后立即服药", "起身如厕时突然眩晕"]
  }]
}
```

**演示重点**：
- 📌 打开Neo4j浏览器，可视化展示O→C→P→R节点链
- 📌 强调PCCM置信度融合：规则85% + PMI 10% + LLM 5%
- 📌 对比传统关键词检索 vs 因果推理的差异

---

### 🔒 演示2：用户级向量删除验证（5分钟）

**场景**：在隔离评测用户范围内验证 Qdrant/副本向量删除，不宣称梯度投影式模型遗忘。

#### 步骤1：验证遗忘前的查询结果
```bash
curl -X POST http://localhost:8080/api/v1/search \
  -d '{"query": "user_12345的护理记录", "top_k": 5}'
# 记录返回结果数：假设返回8条
```

#### 步骤2：执行用户级删除
```bash
curl -X DELETE http://localhost:8080/api/v1/users/user_12345/forget \
  -H "Content-Type: application/json" \
  -d '{"epsilon": 1.0, "max_iterations": 50}'
```

**预期输出**（以现场真实响应为准，不预填指标）：
```json
{
  "result": {
    "target_user": "eval_user_001",
    "before_count": 30,
    "after_count": 0,
    "probe": "passed"
  }
}
```

**演示重点**：
- 📌 展示删除前数量、删除结果、删除后数量和检索探针
- 📌 强调删除操作有隔离用户范围和双重确认
- 📌 不把删除验证表述为模型参数遗忘或 GDPR 合规认证

---

### 🔍 演示3：RRF三路融合（4分钟）

**场景**：查询"阿尔茨海默患者夜间异常行为"

```bash
curl -X POST http://localhost:8080/api/v1/retrieval/multi-dimensional \
  -d '{
    "query": "阿尔茨海默患者夜间异常行为",
    "enable_rrf": true,
    "rrf_parameter": 60
  }'
```

**演示重点**：
- 📌 可视化对比：RRF vs 简单加权的排序差异
- 📌 强调RRF优势：对尾部排名更宽容

---

### 📝 演示4：审计日志（3分钟）

```bash
curl -X GET http://localhost:8080/api/v1/audit/retrieval/statistics
```

**预期输出**：
```json
{
  "total_queries": 47,
  "avg_total_latency": 234.5,
  "avg_knowledge_latency": 89.3,
  "avg_relevance": 0.78
}
```

---

## 三、风险应对预案

### 3.1 常见问题处理

| 问题 | 应对方案 |
|------|----------|
| LLM首次调用慢 | 提前运行预热命令 |
| Neo4j查询超时 | 限制演示数据<100节点 |
| 向量检索返回空 | 硬编码使用已验证的collection |
| 网络断线 | **使用本地环境**，避免依赖外网 |

### 3.2 备用方案（离线演示）

准备：
1. **录屏视频**：提前录制完整演示流程（5分钟）
2. **静态截图**：关键API响应的JSON输出
3. **Neo4j图谱导出**：保存可视化PNG图片

---

## 四、演示检查清单（演示前30分钟）

```bash
# 1. 启动所有服务
docker-compose up -d

# 2. 健康检查
curl http://localhost:8080/health

# 3. 初始化演示数据
bash scripts/init_demo_data.sh

# 4. 预执行一次完整流程
bash scripts/smoke_test.sh
```

---

## 附录：演示物料清单

- [ ] 笔记本电脑（16GB内存以上）
- [ ] Postman Collection（预配置所有API）
- [ ] 备用U盘（离线演示包）
- [ ] Neo4j浏览器书签
- [ ] PPT演示文稿（架构图、算法公式）
- [ ] 一页纸技术白皮书
