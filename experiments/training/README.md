# O-M-P-R QLoRA 训练、评测与启用门禁

本目录只提供可复现的数据切分和训练配置，不包含模型权重，也不会在没有显式依赖和 GPU 的机器上假装完成训练。

```bash
python experiments/training/prepare_causal_dataset.py \
  --output experiments/datasets/causal_ompr --force
python experiments/training/prepare_causal_dataset.py \
  --output experiments/datasets/causal_ompr --check
```

数据集按模板族切分为 600 条 train、100 条 validation、100 条候选 test。生成记录统一标记为 `synthetic_unreviewed`，`human_review_count=0`；生成器绝不代替人工填写“双人复核”或“已仲裁”。`manifest.json` 保存每个 split 的 SHA-256 和组合数据集哈希，初始数据不具备正式冻结评测资格。

## 人工复核清单

先为生成后的数据集建立哈希锚定、事件追加式复核清单：

```bash
python experiments/training/annotation_review.py init \
  --dataset experiments/datasets/causal_ompr \
  --manifest experiments/datasets/causal_ompr.annotation-review.json
python experiments/training/annotation_review.py check \
  --dataset experiments/datasets/causal_ompr \
  --manifest experiments/datasets/causal_ompr.annotation-review.json
```

复核事件通过 `append_review_event` 写入，并形成 SHA-256 哈希链。每条测试记录必须由两名不同成员独立复核；两份标签相同则状态为 `reviewed_agreed`，不一致时只能由第三名成员仲裁为 `reviewed_arbitrated`。只有全部 100 条测试记录达到上述任一状态，检查结果中的 `test_freeze_eligible` 才会为 `true`。清单会拒绝被修改的数据集、改写的历史事件、重复复核者以及由原复核者充任仲裁者。

命令行登记复核时，`relations.json` 必须是该成员独立确认后的关系数组；时间戳必须带时区：

```bash
python experiments/training/annotation_review.py record \
  --dataset experiments/datasets/causal_ompr \
  --manifest experiments/datasets/causal_ompr.annotation-review.json \
  --record-id t-070-00 --actor-id reviewer-a \
  --action independent_review --relations relations.json \
  --timestamp 2026-09-16T10:00:00+08:00
```

`test_freeze_eligible=true` 只证明流程门禁完成，并不证明标签正确率或模型性能；正式报告还必须保存数据集、规则、Prompt、模型、容器和配置指纹。

## 1. 生成可训练数据

训练不直接读取生成器标签。首先必须完成 `annotation_review`；测试集 100 条要满足冻结资格，而且本次实际消费的 train、validation、test 每一条记录都必须达到 `reviewed_agreed` 或 `reviewed_arbitrated`。随后生成只含最终复核标签、带数据集哈希与复核链头的派生数据：

```bash
python experiments/training/prepare_reviewed_dataset.py \
  --dataset experiments/datasets/causal_ompr \
  --review-manifest experiments/datasets/causal_ompr.annotation-review.json \
  --output experiments/training/artifacts/reviewed_dataset
python experiments/training/prepare_reviewed_dataset.py \
  --output experiments/training/artifacts/reviewed_dataset --check
```

派生数据及后续权重均被本目录 `.gitignore` 排除。不要把复核者身份、私有记录或模型权重提交到 Git。

非空复核关系还必须带四字段原文 span，偏移使用 Python/JSON 解码后的 Unicode 字符索引，`end` 为开区间；流水线会验证 `text[start:end] == 字段值`：

```json
{
  "object": "李爷爷",
  "mediator": "服药",
  "property": "头晕",
  "result": "跌倒",
  "spans": {
    "object": {"start": 0, "end": 3},
    "mediator": {"start": 3, "end": 5},
    "property": {"start": 6, "end": 8},
    "result": {"start": 9, "end": 11}
  }
}
```

空关系负例使用 `[]`，不要求 span。缺少 span、越界或无法回指原文的正例会被拒绝，避免模型学习人为补写的医学机制。

## 2. 无 GPU 检查与真实训练

`qlora_config.yaml` 固定 RTX 2050 4GB 的候选配置：Qwen2.5-3B、4-bit NF4、FP16、gradient checkpointing、micro-batch 1、gradient accumulation 16、LoRA rank 16、alpha 32、dropout 0.05。无 GPU、无 Transformers 的开发机可以验证全部治理门禁：

```bash
python experiments/training/train_qlora.py \
  --dataset experiments/training/artifacts/reviewed_dataset \
  --output experiments/training/artifacts/training_plan.json \
  --dry-run
```

输出状态必须是 `validated_not_trained`，它不代表已经训练，也不能作为模型效果证据。真实训练仅在 Linux NVIDIA GPU 环境执行：

```bash
python -m pip install -r experiments/training/requirements-gpu.txt
python experiments/training/train_qlora.py \
  --dataset experiments/training/artifacts/reviewed_dataset \
  --output experiments/training/artifacts/run_001
```

训练产物状态是 `training_completed_unapproved`。训练脚本固定使用 4-bit NF4、FP16 和有界序列长度；完成训练不等于通过准确率验收。

## 3. 冻结集三方比较

分别在完全相同的 `frozen_test.jsonl` 上运行 custom adapter、原始 `Qwen/Qwen2.5-3B-Instruct` 和规则引擎，保存三个预测文件。预测文件必须包含全部冻结记录，不得跳过失败样本：

```json
{
  "schema_version": "contextkeeper-qlora-pipeline-v1",
  "model_kind": "custom",
  "model_id": "adapter-run-001",
  "dataset_artifact_sha256": "<reviewed manifest artifact_sha256>",
  "predictions": [
    {"record_id": "t-070-00", "relations": [], "latency_ms": 1250.0}
  ]
}
```

`model_kind` 分别为 `custom`、`base`、`rules`。模型输出解析失败也要为该记录写入空 `relations`，并在模型运行日志中保留失败原因，禁止从评测输入中删除该条。然后执行：

```bash
python experiments/training/evaluate_activation.py \
  --dataset experiments/training/artifacts/reviewed_dataset \
  --training-run experiments/training/artifacts/run_001/training_run.json \
  --custom experiments/training/artifacts/predictions_custom.json \
  --base experiments/training/artifacts/predictions_base.json \
  --rules experiments/training/artifacts/predictions_rules.json \
  --report experiments/training/artifacts/frozen_comparison.json \
  --activation experiments/training/artifacts/model_activation.json
```

报告始终保存真实指标。只有 custom 同时满足严格 tuple F1 `>=0.75`、字段宏 F1 `>=0.88`、无因果误报率 `<=5%`、否定/时序准确率各 `>=95%`、p95 `<=3s`，且在逐记录 exact-match 的单侧精确配对检验中显著优于 base 与 rules（默认 `p<0.05`），才写出 `model_activation.json`。失败退出码为 `2`，不会生成激活凭证，并会撤销目标路径上的旧凭证。评测不会修改阈值、样本或负结果。

## 4. Ollama 导出

导出脚本同时校验激活凭证、训练运行哈希和 adapter 目录指纹。先做不依赖模型库的门禁检查：

```bash
python experiments/training/export_ollama.py \
  --activation experiments/training/artifacts/model_activation.json \
  --training-run experiments/training/artifacts/run_001/training_run.json \
  --adapter experiments/training/artifacts/run_001/adapter \
  --output experiments/training/artifacts/export_plan.json \
  --dry-run
```

通过后才允许合并权重；添加 `--create` 才会调用本机 `ollama create contextkeeper-causal-3b`：

```bash
python experiments/training/export_ollama.py \
  --activation experiments/training/artifacts/model_activation.json \
  --training-run experiments/training/artifacts/run_001/training_run.json \
  --adapter experiments/training/artifacts/run_001/adapter \
  --output experiments/training/artifacts/ollama_export \
  --create
```

生成的 `Modelfile`、合并权重和 adapter 都是本地运行产物，不提交 Git。没有有效激活凭证时，运行时继续使用基础 3B 或规则回退；不得把 `contextkeeper-causal-3b` 配置为首选模型。

## 5. 测试

```bash
python -m compileall -q experiments/training
python -m unittest discover -s experiments/training -p "test_*.py"
```

这些测试只验证流水线契约和篡改门禁，不声称已进行 GPU 训练或取得任何比赛指标。
