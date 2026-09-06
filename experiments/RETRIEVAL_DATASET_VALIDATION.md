# 检索数据集验证文档

## 概述

`validate_retrieval_dataset.py` 是一个只读验证脚本，用于检查检索实验数据集的完整性和一致性。

**特点**：
- 只读操作，不调用后端
- 不修改任何数据文件
- 验证失败时返回非0退出码

## 检查项

### 1. 语料文件验证 (retrieval_corpus_30.json)

- ✅ 文件包含恰好 30 条唯一 doc_id
- ✅ 计算并输出文件 SHA-256 哈希值
- ✅ 统计实际文档数量

### 2. 查询文件验证 (query_answer_pairs.json)

- ✅ 文件包含恰好 50 条查询
- ✅ 查询类型分布：temporal=17, causal=17, general=16
- ✅ 计算并输出文件 SHA-256 哈希值

### 3. 引用完整性验证

- ✅ 每条查询的 `ground_truth_docs` 都能在30条语料中找到
- ✅ 支持通配符匹配（如 `elder_025_*`）
- ✅ 列出缺失的 doc_id（如有）

### 4. 输出信息

- 两个数据集的 SHA-256 哈希值
- 文档数量和查询数量
- 查询类型统计
- 缺失的 ground_truth_docs 列表

## 运行命令

### 基本用法（使用默认路径）

```bash
cd D:\context\context-keeper-main\experiments
python scripts\validate_retrieval_dataset.py
```

### 指定自定义路径

```bash
python scripts\validate_retrieval_dataset.py \
    --corpus datasets/retrieval_corpus/retrieval_corpus_30.json \
    --queries datasets/retrieval_groundtruth/query_answer_pairs.json
```

### 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--corpus` | 语料文件路径（相对于 experiments/ 目录） | `datasets/retrieval_corpus/retrieval_corpus_30.json` |
| `--queries` | 查询文件路径（相对于 experiments/ 目录） | `datasets/retrieval_groundtruth/query_answer_pairs.json` |

## 输出示例

### 验证通过

```
检索数据集验证
============================================================
语料文件: D:\context\context-keeper-main\experiments\datasets\retrieval_groundtruth\retrieval_corpus_30.json
查询文件: D:\context\context-keeper-main\experiments\datasets\retrieval_groundtruth\query_answer_pairs.json

[1/4] 验证语料文件: retrieval_corpus_30.json
  SHA-256: a1b2c3d4e5f6...
  ✅ 文档数量: 30 (符合预期)
  唯一 doc_id 数量: 30

[2/4] 验证查询文件: query_answer_pairs.json
  SHA-256: f6e5d4c3b2a1...
  ✅ 查询数量: 50 (符合预期)

  查询类型分布:
    ✅ temporal: 17 (预期17)
    ✅ causal: 17 (预期17)
    ✅ general: 16 (预期16)

  ✅ 所有 ground_truth_docs 都能在语料中找到

============================================================
验证摘要
============================================================

语料文件 (retrieval_corpus_30.json):
  SHA-256: a1b2c3d4e5f6...
  文档数量: 30 / 30

查询文件 (query_answer_pairs.json):
  SHA-256: f6e5d4c3b2a1...
  查询数量: 50 / 50
  查询类型:
    temporal: 17 / 17
    causal: 17 / 17
    general: 16 / 16

============================================================
✅ 所有检查通过
============================================================

退出码: 0
```

### 验证失败（示例）

```
检索数据集验证
============================================================
语料文件: D:\context\context-keeper-main\experiments\datasets\retrieval_groundtruth\retrieval_corpus_30.json
查询文件: D:\context\context-keeper-main\experiments\datasets\retrieval_groundtruth\query_answer_pairs.json

[1/4] 验证语料文件: retrieval_corpus_30.json
  SHA-256: a1b2c3d4e5f6...
  ❌ 文档数量: 28 (预期30)
  唯一 doc_id 数量: 28

[2/4] 验证查询文件: query_answer_pairs.json
  SHA-256: f6e5d4c3b2a1...
  ❌ 查询数量: 48 (预期50)

  查询类型分布:
    ✅ temporal: 17 (预期17)
    ❌ causal: 15 (预期17)
    ✅ general: 16 (预期16)

  ❌ 有 3 个 ground_truth_docs 在语料中缺失
  缺失示例（前10个）:
    - elder_099_2026-06-06T10:30:00
    - elder_100_2026-06-07T14:20:00
    - elder_025_2026-06-08T08:15:00

============================================================
验证摘要
============================================================

语料文件 (retrieval_corpus_30.json):
  SHA-256: a1b2c3d4e5f6...
  文档数量: 28 / 30

查询文件 (query_answer_pairs.json):
  SHA-256: f6e5d4c3b2a1...
  查询数量: 48 / 50
  查询类型:
    temporal: 17 / 17
    causal: 15 / 17
    general: 16 / 16
  ❌ 缺失 ground_truth_docs: 3

============================================================
❌ 验证失败
============================================================

退出码: 1
```

## 使用场景

### 1. 数据集初始化后验证

在创建或更新检索数据集后，立即运行验证：

```bash
python scripts\validate_retrieval_dataset.py
```

### 2. 实验前数据完整性检查

在运行大规模评测前，确认数据集状态：

```bash
python scripts\validate_retrieval_dataset.py
if [ $? -eq 0 ]; then
    echo "数据集验证通过，开始评测..."
    python scripts\retrieval_eval.py --queries 50 --configs all
else
    echo "数据集验证失败，请先修复数据集"
    exit 1
fi
```

### 3. CI/CD 流水线集成

在持续集成中自动验证数据集：

```yaml
# .github/workflows/validate-datasets.yml
name: Validate Datasets
on: [push, pull_request]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Validate retrieval dataset
        run: |
          cd experiments
          python scripts/validate_retrieval_dataset.py
```

### 4. 数据集版本比对

通过 SHA-256 哈希值追踪数据集变化：

```bash
# 记录当前版本
python scripts\validate_retrieval_dataset.py > dataset_v1_validation.txt

# 修改数据集后比对
python scripts\validate_retrieval_dataset.py > dataset_v2_validation.txt
diff dataset_v1_validation.txt dataset_v2_validation.txt
```

## 退出码

| 退出码 | 说明 |
|--------|------|
| `0` | 所有检查通过 |
| `1` | 至少一项检查失败 |

## 依赖

- Python 3.7+
- 标准库：`argparse`, `hashlib`, `json`, `pathlib`

无需额外安装第三方依赖。

## 限制

1. **不验证数据质量**：仅验证数量和引用完整性，不检查内容质量
2. **不调用后端**：无法验证数据是否已写入数据库
3. **通配符匹配限制**：仅支持后缀通配符（如 `elder_025_*`），不支持复杂正则表达式

## 相关脚本

- `seed_vector_data.py` - 将语料写入后端数据库
- `retrieval_eval.py` - 运行检索评测实验
- `verify_retrieval_evidence.py` - 验证评测结果中的三路检索证据

## 故障排查

### 问题1：找不到数据文件

**症状**：
```
❌ 语料文件不存在: D:\context\...\retrieval_corpus_30.json
```

**解决方案**：
1. 检查文件是否存在于 `experiments/datasets/retrieval_groundtruth/` 目录
2. 确认文件名拼写正确
3. 使用 `--corpus` 和 `--queries` 参数指定正确路径

### 问题2：JSON 解析失败

**症状**：
```
❌ 无法加载 retrieval_corpus_30.json: JSONDecodeError...
```

**解决方案**：
1. 使用在线工具验证 JSON 格式：https://jsonlint.com/
2. 检查文件编码是否为 UTF-8
3. 确认文件没有被截断或损坏

### 问题3：缺失 ground_truth_docs

**症状**：
```
❌ 有 5 个 ground_truth_docs 在语料中缺失
```

**解决方案**：
1. 检查缺失的 doc_id 是否应该存在于语料中
2. 确认通配符格式正确（`elder_025_*` 会匹配 `elder_025_` 开头的所有 doc_id）
3. 更新查询文件或补充语料文件

## 维护说明

### 修改预期数量

如果需要调整数据集规模，修改以下常量：

```python
# validate_retrieval_dataset.py

# 语料文件预期文档数
expected_count: 30  # 改为新的数量

# 查询文件预期查询数和类型分布
expected_count: 50  # 改为新的总数
expected_counts = {
    "temporal": 17,  # 改为新的数量
    "causal": 17,
    "general": 16
}
```

### 扩展验证规则

在 `validate_queries()` 函数中添加新的检查逻辑：

```python
# 示例：验证每个查询是否包含必需字段
for query in queries:
    if "question" not in query:
        print(f"  ❌ 查询 {query.get('query_id')} 缺少 question 字段")
        # ...
```

## 版本历史

- **v1.0** (2026-07-15) - 初始版本
  - 验证语料文件和查询文件的数量
  - 验证查询类型分布
  - 验证 ground_truth_docs 引用完整性
  - 计算 SHA-256 哈希值
