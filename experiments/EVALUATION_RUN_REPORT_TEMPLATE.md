# Evaluation Run Checklist and Evidence Report Template

## Purpose and Scope

This document is the evidence gate for retrieval evaluation runs. It records
only fields that are present in a readable result JSON or trace JSON. A blank
field means the evidence was not captured; it is not a zero, a pass, or a
substitute for a measurement.

This template does not authorize service restarts. Do not invoke
`scripts/run_configured_eval.sh` while another owner may be operating the
shared stack: that script executes `docker-compose down`, replaces
`config/.env`, and rebuilds the service.

## Current Evidence Status (Read 2026-07-15)

| Evidence file | Readable facts | Status | PPT use |
|---|---|---|---|
| `results/raw/retrieval_20260714_113658.json` | 50 detail rows; `full_system`; API success/error `50/0`; MRR `0.285`; Precision@5 `0.188`; Recall@5 `0.546429`; average latency `19922.49544 ms`; config hash `6821f0...c0f805f5` | Historical single-label retrieval result. `runtime_config` is absent. | Do not use for a configuration comparison or a three-source claim. It may be cited only as an unverified historical result, with file name and limitations. |
| `results/raw/retrieval_20260714_125417.json` | 50 detail rows; `full_system`; API success/error `50/0`; MRR `0.040`; Precision@5 `0.040`; Recall@5 `0.083333`; average latency `14497.8893 ms`; same config hash | Historical single-label retrieval result. `runtime_config` is absent. | Do not select this file as a "latest successful result" without explaining why it differs from the earlier 50-query run. Not comparable for PPT. |
| `results/trace/trace_queries_20260715_131216.json` | 10 rows; `success=10`; summed `result_count=50`; mean `latency_ms=294.848` | Retrieval availability/returned-count trace only. It contains no ground-truth or scoring fields. | May support the narrowly worded availability and latency statement only. Do not derive MRR, Precision@5, Recall@5, answer correctness, or source-fusion effectiveness. |
| `TRACE_FINDINGS_20260714.md` | Documents vector-path execution and absence of graph, timeline, RRF, and fusion-weight execution records for the inspected trace | Trace finding, not a performance metric. | Explicitly blocks claims that current results demonstrate three-source retrieval or RRF fusion. |

The raw files also contain 50-query results labelled `baseline_naive_rag` and
`baseline_rag_with_filter`; no corresponding `baseline_vanilla_llm` retrieval
result is present. None of the 50-query raw files has `runtime_config`.

## Review Findings That Affect Reporting

1. `scripts/retrieval_eval.py` iterates `config_name` labels but sends the
   same retrieval request for every label. The script states that configuration
   switching is handled by service restart and does not transmit a configuration
   header. A multi-label result from one service instance is therefore not
   evidence of a multi-configuration experiment.
2. The evaluator averages MRR, Precision@5, Recall@5, and latency only across
   rows with `error_type == "None"`. Always report `api_success_count` and
   `api_error_count` with those metrics; the denominator is successful API
   responses, not necessarily all attempted queries.
3. The current metric function counts matching document IDs rather than unique
   matching document IDs. A run must be checked for duplicate `retrieved_docs`
   and for `recall_at_5 > 1` before its Precision@5 or Recall@5 values are used
   outside the raw evidence table.
4. `scripts/report_builder.py` chooses files by reverse lexical filename order,
   not by validated provenance. It also renders fixed targets such as
   `>0.80 MRR` in its report and demo slides. Treat generated HTML and
   `results/reports/demo_slides.html` as presentation templates, not evidence.

## Required Run Checklist

Complete this checklist before declaring a run comparable or using its
metrics in a presentation.

| Check | Evidence to record | Pass condition | Status |
|---|---|---|---|
| Run ID | Result filename and SHA-256 | Unique, readable JSON | [ ] |
| Run mode | `execution_mode` | `live_api` for live claims | [ ] |
| Query accounting | `total_queries`, detail-row count, per-config `total_queries` | Counts agree | [ ] |
| Dataset identity | Ground-truth file path and SHA-256 | Present and recorded | [ ] |
| Configuration identity | `runtime_config.config_name`, `runtime_config.config_hash`, service start time | Present and matches the intended single configuration | [ ] |
| Configuration isolation | One service start per configuration; one result file per start | No label-only configuration loop | [ ] |
| API accounting | `api_success_count`, `api_error_count`, and error rows | Both counts reported; errors retained | [ ] |
| Ranking data quality | Detail `retrieved_docs`, `found_ground_truth`, and per-row metrics | No duplicate IDs contributing to metrics; no `recall_at_5 > 1` | [ ] |
| Trace evidence | Trace filename, request count, and source execution logs | Required source paths match the claim | [ ] |
| Comparison set | Same dataset hash, query set, API version, and measurement method for every configuration | Complete baseline set, including `baseline_vanilla_llm` when claimed | [ ] |
| Review record | Reviewer, date, and known limitations | Filled before export | [ ] |

## Per-Run Evidence Record

Fill values directly from the result JSON. Do not calculate a replacement
value from a screenshot, console summary, or a different run.

```text
Result file:
Result SHA-256:
Execution mode (execution_mode):
Result timestamp (timestamp):
Configuration label (aggregated_metrics.<name>.config_name):
Runtime configuration name/hash/start time (runtime_config.*):
Ground-truth dataset path/SHA-256:
Total queries (top-level and per configuration):
API success/error counts:
MRR:
Precision@5:
Recall@5:
Average latency (ms):
Temporal/causal/general MRR:
Duplicate retrieved-doc rows:
Rows with recall_at_5 > 1:
Trace file and trace request count:
Trace-confirmed source paths:
Known limitations:
Reviewer/date:
```

## Allowed Report Language

Use only statements whose values appear in the cited file.

- Availability: "In `<trace file>`, `<success count>/<row count>` traced
  requests reported `success=true`; the mean recorded `latency_ms` was
  `<value>`."
- Single-run ranking: "In `<raw file>`, labelled `<config>`, the evaluator
  recorded API success/error `<success>/<error>`, MRR `<mrr>`, Precision@5
  `<precision>`, Recall@5 `<recall>`, and average latency `<latency> ms>`."
- Limitation: "This is not a configuration comparison because
  `<runtime_config or equivalent deployment evidence>` is absent."

## PPT Release Gate

All items below must pass before a slide presents a configuration ranking,
relative improvement, or a fusion-effectiveness claim.

- [ ] Every compared configuration has a separately captured `runtime_config`
  and matching configuration hash.
- [ ] The exact ground-truth dataset hash and query count match across the
  compared runs.
- [ ] Each compared result contains complete API success/error accounting.
- [ ] Duplicate-return and Recall@5 bounds were checked on detailed rows.
- [ ] A trace proves every retrieval source named on the slide actually ran.
- [ ] The slide contains the exact result filename(s), run date, and metric
  denominator.
- [ ] No hard-coded target from `report_builder.py` is presented as an
  observed result.

Until the gate passes, the following are explicitly not writable to PPT:

- A ranking or uplift between baseline configurations and `full_system`.
- A claim that `full_system` executed vector, graph, timeline, RRF, or
  three-source fusion for the scored queries.
- A claim of answer quality, grounded answer correctness, or end-to-end
  response quality. The retrieval raw result examined has empty
  `response_text` values, and the trace has no answer-quality field.
- Any target statement such as `MRR > 0.80` represented as a measured outcome.

## Safe Next Run Boundary

Do not start a 50-query run as part of routine validation. Coordinate an idle
window with the service owner, then run one configuration per service start,
capture `runtime_config`, preserve the raw and trace JSON files, and review
this checklist before proceeding to the next configuration.
