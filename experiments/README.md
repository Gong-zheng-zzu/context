# Context-Keeper 比赛实验体系

本目录包含Context-Keeper项目的完整实验体系，支持3个使用场景：

1. **赛前完整跑分**（20-40分钟）：生成正式实验报告和数据
2. **答辩现场快速演示**（3-5分钟）：展示核心能力
3. **评委交互验证**（即时响应）：证明系统非预设

---

## 目录结构

```
experiments/
├── configs/           # 4个对比配置（vanilla_llm、naive_rag、rag_with_filter、full_system）
├── datasets/          # 测试数据集（软链接到tests/datasets/）
├── scripts/           # 实验脚本
│   ├── security_eval.py        # 安全防护实验
│   ├── causal_eval.py         # 因果推理实验
│   ├── retrieval_eval.py      # 检索融合实验
│   ├── unlearning_eval.py     # 机器遗忘实验
│   ├── report_builder.py      # 报告生成器
│   ├── run_all.sh             # 完整实验入口
│   ├── run_demo.sh            # 快速演示入口
│   └── interactive_demo.sh    # 交互验证入口
├── results/           # 实验结果输出
│   ├── raw/          # 原始JSON数据
│   ├── figures/      # 对比图表
│   └── reports/      # HTML报告
└── README.md
```

---

## 快速开始

### 前置条件

1. **服务运行**：确保Context-Keeper服务运行在 `http://localhost:8088`
   ```bash
   cd /d/context/context-keeper-main
   docker-compose up -d
   ```

2. **Python环境**：Python 3.8+ 已安装
   ```bash
   pip install requests pyyaml matplotlib plotly pandas
   ```

### 使用方式

#### 1. 完整实验（赛前跑分）

```bash
cd /d/context/context-keeper-main/experiments
bash scripts/run_all.sh
```

**耗时**：20-40分钟  
**输出**：`results/reports/full_report.html`（5章节完整报告 + 4张关键图表）

**包含内容**：
- 安全防护：600个攻击样本 × 4配置
- 因果推理：100条样本 × 4配置
- 检索融合：50对查询 × 4配置
- 机器遗忘：5个场景 × full_system配置

---

#### 2. 快速演示（答辩现场）

```bash
cd /d/context/context-keeper-main/experiments
bash scripts/run_demo.sh
```

**耗时**：3-5分钟  
**输出**：`results/reports/demo_slides.html`（4页答辩slides）

**包含内容**：
- 安全防护：6类攻击各1条样本
- 因果推理：10条典型样本
- 检索融合：10对查询
- 机器遗忘：1个场景演示

---

#### 3. 交互验证（评委提问）

```bash
cd /d/context/context-keeper-main/experiments
bash scripts/interactive_demo.sh
```

**支持3类交互**：
1. **自定义查询**：输入任意查询，展示三路检索过程（向量+图谱+时序）
2. **攻击测试**：输入攻击文本，展示安全防护决策链（ASDF→CASIA→PCCM）
3. **数据遗忘**：输入user_id，展示遗忘前后对比

---

## 四类实验说明

### 1. 安全防护实验（security_eval.py）

**目标**：验证多层安全防护能力

**测试内容**：
- 6类攻击：提示注入、记忆投毒、权限提升、隐私泄露、幻觉诱导、遗忘绕过
- 4配置对比：vanilla_llm、naive_rag、rag_with_filter、full_system

**关键指标**：
- ASR（攻击成功率）：越低越好，目标 <10%
- 防御成功率：越高越好，目标 >90%
- 分类ASR：6类攻击的单独成功率
- 平均延迟：<2000ms

**基线对比预期**：
| 配置 | ASR | 防御成功率 |
|------|-----|-----------|
| vanilla_llm | 85-90% | 10-15% |
| naive_rag | 75-80% | 20-25% |
| rag_with_filter | 45-55% | 45-55% |
| full_system | <10% | >90% |

---

### 2. 因果推理实验（causal_eval.py）

**目标**：验证PCCM融合算法的因果抽取能力

**测试内容**：
- 30条标注样本（快速）或100条（完整）
- 四元组抽取：Object → Mediator → Property → Result

**关键指标**：
- 抽取成功率：目标 ≥80%
- 平均置信度：目标 ≥0.7
- Ground truth F1：与人工标注的匹配度
- 响应时间：目标 <2000ms

**基线对比预期**：
| 配置 | 准确率 | 置信度 |
|------|--------|--------|
| vanilla_llm | 50-60% | 0.5 |
| naive_rag | 60-70% | 0.6 |
| rag_with_filter | 70-80% | 0.65 |
| full_system | >85% | >0.7 |

---

### 3. 检索融合实验（retrieval_eval.py）

**目标**：验证三路检索融合（向量+图谱+时序）的优势

**测试内容**：
- 50对查询-答案对
- 查询类型：时序查询（15对）、因果查询（15对）、普通查询（20对）

**关键指标**：
- MRR（平均倒数排名）：目标 >0.80
- Precision@5：前5结果的准确率
- Recall@5：前5结果的召回率
- 单路 vs 融合对比

**基线对比预期**：
| 配置 | MRR | Precision@5 |
|------|-----|-------------|
| vanilla_llm | 0.3 | 0.2 |
| naive_rag | 0.55 | 0.45 |
| rag_with_filter | 0.62 | 0.52 |
| full_system | >0.80 | >0.70 |

---

### 4. 机器遗忘实验（unlearning_eval.py）

**目标**：验证梯度正交投影算法的遗忘效果

**测试内容**：
- 5个场景：用户删除、时间段遗忘、事件类型遗忘、关联遗忘、敏感信息遗忘

**关键指标**：
- 遗忘率：目标 >95%（目标用户数据不可检索）
- 副作用率：目标 <5%（其他用户数据受影响程度）
- 收敛迭代次数：算法收敛速度
- 遗忘耗时：完整遗忘过程耗时

**预期结果**：
- 遗忘前：目标用户查询命中率 >90%
- 遗忘后：目标用户查询命中率 <5%
- 其他用户：查询命中率变化 <5%

---

## 数据集说明

### 现有数据集

| 数据集 | 路径 | 规模 | 用途 |
|--------|------|------|------|
| 攻击样本 | `datasets/attack_samples/` | 600个（6类×100） | 安全实验 |
| 因果标注 | `datasets/causal_reasoning/` | 30条标注 | 因果实验 |
| 护理记录 | `datasets/nursing_data/` | 9,809条 | 因果/检索实验 |
| 老人档案 | `datasets/nursing_data/` | 100个 | 遗忘实验 |

### 待补充数据集

⚠️ **检索ground truth**（优先级P0）：
- 路径：`datasets/retrieval_groundtruth/query_answer_pairs.json`
- 规模：50对查询-答案对
- 用途：检索融合实验
- 状态：需补充

**生成方式**：
1. 自动生成30对基础查询（时序/普通查询）
2. 人工标注20对复杂查询（多跳因果/时序推理）

---

## 报告说明

### 完整报告（full_report.html）

**5个章节**：
1. **数据集说明**：测试数据集构成表
2. **安全防护评估**：4配置ASR对比 + 6类攻击雷达图
3. **因果推理评估**：4配置准确率对比 + PCCM三路贡献饼图
4. **检索融合评估**：单路vs三路MRR对比 + 查询类型适配性柱状图
5. **机器遗忘评估**：5场景对比 + 遗忘率vs副作用散点图

### 4张关键图表

用于PPT和答辩演示的核心对比图。

---

## 常见问题

### Q1: 服务启动失败怎么办？

检查服务状态：
```bash
docker-compose ps
curl http://localhost:8088/health
```

### Q2: 如何只运行单个实验？

```bash
# 只运行安全实验
python3 scripts/security_eval.py --samples 100 --configs all

# 只运行因果实验
python3 scripts/causal_eval.py --samples 30 --configs all
```

---

## 联系方式

如有问题，请查阅：
- 完整计划：`.claude/plans/stateless-shimmying-haven.md`
- P0-6实验报告：`../P0-6_因果推理实验完成报告.md`
