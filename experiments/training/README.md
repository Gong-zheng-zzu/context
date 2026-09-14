# O-M-P-R QLoRA 训练准备

本目录只提供可复现的数据切分和训练配置，不包含模型权重，也不会在没有显式依赖和 GPU 的机器上假装完成训练。

```bash
python experiments/training/prepare_causal_dataset.py \
  --output experiments/datasets/causal_ompr --force
python experiments/training/prepare_causal_dataset.py \
  --output experiments/datasets/causal_ompr --check
```

数据集按模板族切分为 600 条 train、100 条 validation、100 条冻结 test。每条记录标记为 synthetic，并记录双人复核/仲裁元数据；`manifest.json` 保存每个 split 的 SHA-256 和组合数据集哈希。

`qlora_config.yaml` 是 RTX 2050 4GB 的候选配置：4-bit NF4、FP16、gradient checkpointing、micro-batch 1、gradient accumulation 16、LoRA rank 16、alpha 32、dropout 0.05。真正训练、验证和 Ollama 导出必须在 Linux GPU 环境单独执行并保留日志；未通过冻结集门禁前不得把 `contextkeeper-causal-3b` 当作可用模型。
