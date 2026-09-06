# Experiment Checkpoint - 2026-07-14

## Verified Retrieval Result

- Result file: `results/raw/retrieval_20260714_113658.json`
- Execution mode: live API
- Configuration: `full_system`
- Dataset: 50 fixed queries (17 temporal, 17 causal, 16 general)
- API success: 50/50
- MRR: 0.285
- Precision@5: 0.188
- Recall@5: 0.546
- Average latency: 19.9 s

The retrieval pipeline now returns structured contexts with stable `doc_id` values.
JWT protection is enabled on the retrieval endpoints, and the evaluator authenticates
with environment-provided credentials before requesting retrieval.

## Current Scope

The retrieval experiment is runnable and its quality metrics are calculable. This is
not yet evidence that the full system improves over baselines, because only
`full_system` has been evaluated with the repaired pipeline.

## Required Before Final Competition Results

1. Run the same 50 queries against `baseline_naive_rag`,
   `baseline_rag_with_filter`, and `full_system`, with a clean identical dataset.
2. Record MRR, Precision@5, Recall@5, and P50/P95 latency for every configuration.
3. Remove or bypass non-essential LLM generation on the retrieval path before live
   demonstration; the current average latency is too high.
4. Expand the seeded corpus from 7 documents to at least 30 semantically distinct,
   manually reviewed documents.
5. Complete separate valid runs for security, causal extraction, and unlearning.

## Decision

Do not claim that the complete four-part experiment suite is finished. Proceed with
the full retrieval comparison experiment after preparing the baseline configurations.
