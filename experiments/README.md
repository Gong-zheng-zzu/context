# Context-Keeper Experiment System

This directory contains the reproducible experiment tooling for Context-Keeper.
The scripts preserve raw rows, configuration identity, dataset hashes, smoke-test
outcomes, and evaluator logs. A chart or HTML report is not evidence by itself.

Chinese execution status and presentation boundaries are maintained in
[`EXPERIMENT_EXECUTION_STATUS_20260717.md`](EXPERIMENT_EXECUTION_STATUS_20260717.md).
The full measurement contract is in
[`EXPERIMENT_MASTER_PLAN.md`](EXPERIMENT_MASTER_PLAN.md).

## Evidence Gate

`scripts/run_configured_eval.sh` is the only entry point that may mark a newly
generated raw result as report-eligible. It records:

- base, overlay, and effective configuration SHA-256 values;
- a dataset-tree hash and sanitized environment fingerprint;
- a passed smoke test, evaluator exit code, and immutable run manifest;
- one new raw JSON result for the configured evaluator.

`scripts/report_builder.py` rejects an artifact without this provenance. It
also requires 50 complete retrieval queries, 120 complete security requests,
20 complete causal records, and three passing unlearning trials before it
publishes the corresponding formal report section.

## Current Verified Material

Only the following historical observations are safe to cite until fresh
same-configuration evaluations complete:

| Capability | Verified observation | Boundary |
|---|---|---|
| Causal extraction | 20/20 API responses; strict O-M-P-R tuple accuracy 2/20 (10.0%) | Baseline only; not a high-accuracy PCCM claim. |
| Retrieval | Naive RAG 50-query baseline: MRR 0.405, P@5 0.168, R@5 0.565 | This is not a three-source RRF comparison. |
| Three-source trace | Ten authenticated queries include `rrf_3_sources` when all lanes return scoped `doc_id` values | Other rows may be two-source or vector-only fallback. |
| User unlearning | Isolated Qdrant target count changed from 30 to 0 and retrieval probe passed | This validates user-level deletion, not gradient-projection model unlearning. |
| Security | Six representative requests reached the protected path | It is a preflight, not a 600-sample attack-success metric. |

## Prerequisites

1. Start the stack and verify `http://127.0.0.1:8088/health`.
2. Set the protected evaluator credentials and only the isolated destructive
   target required by the script.
3. Confirm the appropriate dataset-review and release gates in the master
   plan before enabling a full run.

Do not put production passwords, API keys, or copied `.env` files under
`experiments/results/`. The repository ignores `*.env.backup` by design.

## Commands

### Formal evaluation

```bash
export EVAL_ALLOW_FULL_RUN=true
export EVAL_ALLOW_RETRIEVAL_COMPARISON=true
export EVAL_ALLOW_DESTRUCTIVE_UNLEARNING=true
export EVAL_UNLEARNING_USER_ID=eval_user_001
bash experiments/scripts/run_all.sh
```

The full runner executes 50 retrieval queries per configuration. It exits on a
failed smoke test, failed evaluator, invalid artifact count, or missing formal
report evidence; it does not manufacture a comparison from partial data.

### Competition demonstration

Use the PowerShell entry point for a bounded, interactive proof of execution:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = '<demo password>'
cd D:\context\context-keeper-main\experiments\scripts
.\run_competition_demo.ps1
```

It performs a health check, one source-traced retrieval, a latency gate, and a
representative security check. Destructive unlearning is off by default and
requires both `-RunIsolatedUnlearning` and
`EVAL_DEMO_ALLOW_UNLEARNING=true`.

### Report generation

```bash
python3 experiments/scripts/report_builder.py \
  --mode full \
  --require security causal retrieval unlearning \
  --output experiments/results/reports/full_report.html
```

The command intentionally fails when any requested section lacks eligible raw
evidence. Preserve that failure rather than replacing it with a template or an
old result.

## Supporting Utilities

- `verify_retrieval_evidence.py`: validates result-level retrieval provenance.
- `validate_retrieval_dataset.py`: validates corpus/query counts and review
  metadata.
- `run_trace_queries.py`: produces source and fallback traces; it is not a
  metric evaluator.
- `unlearning_eval.py`: performs a scoped before/delete/after verification.
- `interactive_demo.sh`: supports manual inspection but is not a benchmark
  runner.

Run the relevant offline tests before changing an evaluator or report contract:

```bash
python3 experiments/scripts/test_report_builder.py
python3 experiments/scripts/test_experiment_contract.py
```
