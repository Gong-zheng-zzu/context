# O-M-P-R QLoRA 训练准备

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

`qlora_config.yaml` 是 RTX 2050 4GB 的候选配置：4-bit NF4、FP16、gradient checkpointing、micro-batch 1、gradient accumulation 16、LoRA rank 16、alpha 32、dropout 0.05。真正训练、验证和 Ollama 导出必须在 Linux GPU 环境单独执行并保留日志；未通过冻结集门禁前不得把 `contextkeeper-causal-3b` 当作可用模型。
