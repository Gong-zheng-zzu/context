# Context-Keeper 因果推理实验测试指南

本目录包含因果推理模块的实验验证脚本。

## 📁 文件说明

### 测试脚本

- **causal_reasoning_accuracy_test.py** - 因果推理准确率测试
- **causal_reasoning_performance_test.py** - 因果推理性能测试
- **attack_defense_benchmark.py** - 攻击防御效果测试（已有）
- **unlearning_benchmark.py** - 机器遗忘基准测试（已有）

### 支持文件

- **requirements.txt** - Python依赖包
- **results/** - 测试结果输出目录（自动创建）

## 🚀 快速开始

### 1. 安装依赖

```bash
cd tests/benchmark
pip install -r requirements.txt
```

### 2. 启动Context-Keeper服务

确保以下服务正在运行：

```bash
# 检查Docker服务
docker ps | grep -E "neo4j|qdrant|timescale"

# 启动Context-Keeper服务
cd ../../
docker-compose up -d

# 或直接运行
go run cmd/server/main_http.go
```

### 3. 运行测试

#### 准确率测试

```bash
# 默认测试100条样本
python causal_reasoning_accuracy_test.py

# 自定义样本数和API地址
python causal_reasoning_accuracy_test.py --url http://localhost:8088 --samples 50
```

#### 性能测试

```bash
# 完整测试（约30-60分钟）
python causal_reasoning_performance_test.py

# 快速测试模式（约5分钟）
python causal_reasoning_performance_test.py --quick

# 自定义API地址
python causal_reasoning_performance_test.py --url http://localhost:8088
```

## 📊 测试内容

### 准确率测试

测试指标：
- ✓ 抽取成功率（目标≥80%）
- ✓ 平均置信度（目标≥0.7）
- ✓ 平均响应时间（目标<2000ms）
- ✓ 四元组字段完整性（Object, Mediator, Property, Result）
- ✓ Ground Truth匹配分数（如有标注数据）

输出文件：
- `results/accuracy_test_results_YYYYMMDD_HHMMSS.json` - 详细测试数据
- `results/accuracy_test_report_YYYYMMDD_HHMMSS.md` - Markdown格式报告

### 性能测试

测试类型：

1. **单条请求延迟测试**
   - 目标：平均响应时间 < 2000ms
   - 样本数：100条（快速模式20条）

2. **并发处理能力测试**
   - 目标：支持100并发，响应时间 < 3000ms
   - 并发级别：10, 50, 100

3. **批量吞吐量测试**
   - 目标：1000条记录 < 30分钟
   - 批量大小：1000条（快速模式50条）

4. **推理查询响应时间测试**
   - 目标：平均响应时间 < 1000ms
   - 查询数：100次（快速模式20次）

输出文件：
- `results/performance_test_results_YYYYMMDD_HHMMSS.json` - 详细性能数据
- `results/performance_test_report_YYYYMMDD_HHMMSS.md` - Markdown格式报告

## 📋 测试数据

### 标注数据集

位置：`../datasets/causal_reasoning/annotated_nursing_records.json`

格式：
```json
{
  "record_id": "001",
  "text": "护理记录文本...",
  "has_ground_truth": true,
  "ground_truth": {
    "object": "对象",
    "mediator": "中介",
    "property": "属性",
    "result": "结果",
    "confidence": 0.95,
    "annotator": "标注者",
    "annotation_date": "2026-07-13"
  }
}
```

当前包含：**30条规则标注的医学因果关系数据**

### 护理记录数据

位置：`../datasets/nursing_data/nursing_records.json`

包含：**9,809条真实护理记录**

## 🔧 故障排除

### 1. API连接失败

```
[ERROR] Request timeout (30s)
```

**解决方案**：
- 检查Context-Keeper服务是否启动：`curl http://localhost:8080/health`
- 检查防火墙设置
- 检查端口是否被占用

### 2. 所有测试失败

```
[ERROR] All requests failed
```

**解决方案**：
- 确认Neo4j服务运行：`docker ps | grep neo4j`
- 确认Ollama服务运行：`curl http://localhost:11434/api/tags`
- 查看Context-Keeper日志：`docker-compose logs context-keeper`

### 3. 标注数据不足

```
[LOAD] Loaded 0 annotated samples
```

**解决方案**：
- 检查文件路径：`ls ../datasets/causal_reasoning/annotated_nursing_records.json`
- 脚本会自动从护理记录中抽取样本作为补充

### 4. 性能测试耗时过长

**解决方案**：
- 使用快速测试模式：`--quick`
- 减少样本数量
- 检查网络延迟和服务性能

## 📈 结果分析

### 查看测试报告

所有测试报告保存在 `results/` 目录：

```bash
# 查看最新的准确率报告
cat results/accuracy_test_report_*.md | tail -n 100

# 查看最新的性能报告
cat results/performance_test_report_*.md | tail -n 100
```

### 关键指标

**准确率测试**：
- 成功率 ≥ 80% ✓
- 平均置信度 ≥ 0.7 ✓
- 响应时间 < 2秒 ✓

**性能测试**：
- 单条抽取 < 2秒 ✓
- 100并发支持 ✓
- 批量1000条 < 30分钟 ✓
- 推理查询 < 1秒 ✓

## 🎯 实验目标

根据Context-Keeper策划书，因果推理模块需要达到以下指标：

1. **抽取准确率** > 85%
2. **推理查询响应时间** < 1秒
3. **支持并发** ≥ 100用户
4. **PCCM置信度融合** 误差 < 5%

## 📝 注意事项

1. **首次运行**：首次测试会比较慢，因为需要初始化LLM模型和Neo4j连接
2. **数据隔离**：测试不会修改生产数据，所有抽取的关系仅用于测试
3. **资源消耗**：批量测试会消耗大量CPU和内存，建议在非高峰期运行
4. **并发限制**：过高并发可能导致服务不稳定，建议逐步增加并发级别

## 🔗 相关文档

- [技术栈文档](../../TECH_STACK_COMPLETE.md)
- [API文档](../../internal/api/causal_reasoning_handlers.go)
- [因果推理引擎](../../internal/engines/causal_reasoning/)
- [实施计划](../../.claude/plans/stateless-shimmying-haven.md)

## 📧 问题反馈

如遇到问题，请检查：
1. Docker服务状态
2. 日志文件：`docker-compose logs`
3. 测试结果目录：`results/`
