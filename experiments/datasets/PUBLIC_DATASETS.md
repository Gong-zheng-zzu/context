# 可复现实验数据源

本项目把数据分成三层：公开合成数据、合法脱敏数据、受限外部验证数据。受限数据只允许在获得资质和数据使用协议后本地导入，不进入 Git、CI、Docker 镜像或答辩材料。

## 推荐扩充路径

1. 使用仓库中的 `prepare_expanded_causal_dataset.py` 生成 2400/400/400 条可公开复现的 O-M-P-R 合成数据。
2. 需要结构化患者轨迹时，固定 Synthea 发布版本并在本机生成 FHIR/CSV；只登记版本和 SHA-256，不提交批量原始文件。
3. 只有在完成 PhysioNet credentialing 和 DUA 后，才可将 MIMIC-IV 或 eICU-CRD 作为本地外部验证集；不得与合成训练集混合调参。

## 运行扩展数据生成器

```powershell
python experiments/datasets/prepare_expanded_causal_dataset.py `
  --output experiments/datasets/generated/causal_ompr_expanded_v1 `
  --seed 20260926 --force

python experiments/datasets/prepare_expanded_causal_dataset.py `
  --output experiments/datasets/generated/causal_ompr_expanded_v1 --check
```

默认规模为训练 2400、验证 400、冻结候选测试 400。生成结果的 `annotation.status` 为 `synthetic_unreviewed`，`eligible_for_frozen_evaluation` 为 `false`。冻结测试仍必须经过两人独立复核和第三人仲裁。

## 外部来源边界

来源、许可证、用途和处理限制登记在 `public_sources.yaml`。Synthea 使用 Apache-2.0 的全合成数据，适合公开生成；MIMIC-IV 和 eICU-CRD 需要 PhysioNet 凭证，只能本地处理；纯正 PII 数据只能计算检出率，不能单独计算准确率、F1 或误报率。
