# 配对检索对照：vector → rrf

同一批查询上的逐查询配对对照。区间不跨越 0 才说明差异与「无差异」不相容。

## 配对前提

- 可配对查询数：**300**
- 仅在 vector 可评分：0
- 仅在 rrf 可评分：0
- 真值 sha256：`d5463deae0bf82d8b1d21c38cc7f5e4eae3a97219a5a1bca2e8f018058cf5ca0`
- 语料 sha256：`6766cc1e958381e4df0b5bcaf8af03e167e07f7c6b0caa61a6092cbc3422b3b8`
- 真值标注状态：auto_derived_unreviewed（report_eligible=False）
- 评测模式：vector_contexts_only → multi_source_rrf_evaluation_only
- 统计方法：paired_bootstrap_percentile_with_exact_mcnemar，重采样=10000，seed=20260920，置信水平=0.95

## 指标对照

| 指标 | 差值（rrf − vector） | 95% 区间 | 是否跨越 0 |
| --- | --- | --- | --- |
| mrr | +0.3482 | [+0.2979, +0.3971] | 否（差异显著） |
| precision_at_5 | +0.0900 | [+0.0787, +0.1013] | 否（差异显著） |
| recall_at_5 | +0.4500 | [+0.3933, +0.5067] | 否（差异显著） |
| hit_at_5 | +0.4500 | [+0.3933, +0.5067] | 否（差异显著） |

## 命中率的精确 McNemar 检验

- vector 命中率：0.3433
- rrf 命中率：0.7933
- 2×2 不一致格：rrf 命中而 vector 未命中 = **136**，vector 命中而 rrf 未命中 = **1**
- 不一致对总数：137
- 双侧精确 p 值：**1.5841623087253484e-39**
- 单侧精确 p 值：7.920811543626742e-40

## MRR 的 Wilcoxon 符号秩检验

- 非零差值对数：199
- 统计量（W+）：18452.5
- 双侧 p 值：**< 1e-300**（正态近似尾概率下溢，低于双精度可表示范围）
- 方法：wilcoxon_signed_rank_normal_approximation（normal approximation with tie and continuity correction; two_sided_p underflowed below double precision）

注：Wilcoxon 为**正态近似**（并列与连续性修正）。样本量大时精确分布不可行；上表中命中率的 McNemar 检验是**精确**检验，可作为更可靠的主判据。

## 按查询类型的 MRR 差值

| 查询类型 | 查询数 | 差值 | 95% 区间 | 是否跨越 0 |
| --- | --- | --- | --- | --- |
| causal | 100 | +0.2337 | [+0.1647, +0.3037] | 否 |
| general | 100 | +0.4322 | [+0.3508, +0.5167] | 否 |
| temporal | 100 | +0.3787 | [+0.2833, +0.4735] | 否 |

## 解读边界

A difference is inconsistent with 'no difference' only when the confidence interval excludes zero, or when the paired test's p-value is below the declared significance level. Both are conditional on this query set.
