# 500/100 正式检索实验

## 前置条件

- 已完成 `competition_annotation_tasks_100.csv` 的单人复核：每行 `review_status=confirmed`，并填写 reviewer、reviewed_at、review_reason。若自动候选不合适，在 `reviewed_doc_id` 填写冻结 500 条语料中的正确 `doc_id`；留空则确认原候选。
- 已用 `finalize_single_reviewer_annotations.py` 生成 `competition_queries_100_reviewed.json`。
- 500 条实验会话仅使用 `eval_user_001 / eval_retrieval_test`；运行会清理该范围。

## 冻结与验证

```powershell
cd D:\context\context-keeper-main\experiments
python scripts\finalize_single_reviewer_annotations.py
python scripts\validate_retrieval_dataset.py `
  --corpus datasets/retrieval_corpus/retrieval_corpus_500.json `
  --queries datasets/retrieval_groundtruth/competition_queries_100_reviewed.json `
  --expected-corpus-count 500 --expected-query-count 100 `
  --expected-type-counts temporal=34,causal=33,general=33 --require-reviewed
```

## 分配置运行

每个配置必须由 `run_configured_eval.ps1` 独立重启，并在同一个运行窗口内调用正式 runner：

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = '<演示环境密码>'
.\scripts\run_configured_eval.ps1 -ConfigName baseline_naive_rag `
  -EvalCommand "python experiments/scripts/run_competition_retrieval.py --config-name baseline_naive_rag"
```

按相同方式运行 `baseline_rag_with_filter` 和 `full_system`。全系统使用 RRF，两个 baseline 使用向量检索；每次运行均包含清理、seed、10 条 trace、100 条评测和原始 JSON 校验。配置切换器会自动注入与当前配置一致的运行时证明，不需要手工设置该变量。

不要在草案查询集上执行正式 runner，也不要把草案指标放进 PPT。
