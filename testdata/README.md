# Context-Keeper 测试数据集说明

## 概述

本目录包含Context-Keeper项目的测试数据集，用于开发和验证核心功能模块。

## 数据集文件

### 1. medical_rules.json
**用途**: PCCM因果推理模块的医学规则库

**内容**:
- 12条医学因果规则
- 涵盖药物副作用、慢性疾病、生活方式等类别
- 每条规则包含：
  - `mediator`: 中介因素（如"服用降压药"）
  - `property`: 属性变化（如"体位性低血压"）
  - `result`: 最终结果（如"跌倒"）
  - `confidence`: 规则置信度（0.78-0.95）

**使用场景**:
- 因果关系抽取时的规则匹配
- PCCM置信度融合的规则权重计算
- 验证抽取结果的合理性

### 2. nursing_records.json
**用途**: 护理记录样本，用于因果关系抽取测试

**内容**:
- 20条真实场景的护理记录
- 每条记录包含：
  - 完整的护理描述文本
  - 标注的因果四元组（O→C→P→R）
  - 时间戳和患者信息

**使用场景**:
- LLM实体抽取的输入样本
- 端到端因果推理流程测试
- 抽取准确率的基准数据集

### 3. causal_test_cases.json
**用途**: 因果关系抽取准确率验证

**内容**:
- 20个标注测试用例
- 每个用例包含：
  - 输入文本
  - 期望输出（O/C/P/R）
  - 置信度阈值

**验证标准**:
- 抽取准确率 > 85%
- 语义匹配度 > 80%

### 4. machine_unlearning_test.json
**用途**: 梯度正交投影机器遗忘算法测试

**内容**:
- 5个测试场景（单用户、大批量、敏感数据等）
- 性能基准（小/中/大规模）
- 验证标准：
  - 遗忘后相似度 < 0.1
  - 公平性损失变化 < 0.05
  - 收敛迭代 < 50次

**使用场景**:
- 验证梯度正交投影算法正确性
- 性能压力测试
- GDPR合规性验证

### 5. rrf_fusion_test.json
**用途**: RRF三路检索融合算法测试

**内容**:
- RRF参数配置（k=60，权重设置）
- 3个查询场景的模拟检索结果
- RRF计算示例和期望排序
- 性能对比基准

**验证标准**:
- RRF分数计算误差 < 1%
- 融合延迟 < 50ms
- 检索质量提升 > 15%

## 数据集使用指南

### 开发阶段
```bash
# 因果推理模块开发
go test -v ./internal/engines/causal_reasoning/ -testdata=../../testdata/

# 机器遗忘模块开发
go test -v ./internal/services/machine_unlearning_service_test.go

# RRF融合算法开发
go test -v ./internal/engines/multi_dimensional_retrieval/
```

### 集成测试
```bash
# 端到端测试：护理记录 → 因果抽取 → 图谱构建 → 推理查询
make test-causal-reasoning

# 机器遗忘完整流程测试
make test-machine-unlearning

# 三路检索融合测试
make test-rrf-fusion
```

### 性能测试
```bash
# 批量处理1000条护理记录
go test -bench=BenchmarkCausalExtraction -benchtime=1000x

# 机器遗忘性能测试（5000向量）
go test -bench=BenchmarkMachineUnlearning -benchtime=10x

# RRF融合延迟测试
go test -bench=BenchmarkRRFFusion -benchtime=1000x
```

## 数据质量保证

### 医学准确性
- 所有医学规则和护理记录已经过医学专业人员审核
- 置信度数值基于医学文献和临床经验

### 数据多样性
- 涵盖养老院常见的20类健康问题
- 包含药物、疾病、生活方式、环境等多维度因素

### 隐私保护
- 所有患者姓名均为虚构
- 不包含真实个人身份信息

## 数据更新

### 版本控制
- 当前版本: v1.0
- 更新日期: 2026-07-07

### 扩展计划
- 增加护理记录样本至100条（目标：2026-07-15）
- 扩充医学规则库至50条（目标：2026-07-20）
- 添加负样本测试用例（目标：2026-07-25）

## 相关文档

- [PCCM因果推理实施计划](../docs/causal_reasoning_plan.md)
- [机器遗忘算法设计](../docs/machine_unlearning_design.md)
- [RRF融合算法说明](../docs/rrf_fusion_algorithm.md)

## 问题反馈

如发现数据质量问题或需要补充测试场景，请联系开发团队。
