# Experiment Execution Status - 2026-07-17

## Scope And Integrity Rules

- Docker Compose services were healthy after execution.
- Destructive scope was limited to `eval_user_001` and `eval_retrieval_test`.
- Historical results without dataset provenance, configuration evidence, or valid API denominators are excluded from presentation material.
- HTTP, JWT, timeout, and protocol failures are not counted as business success, defense, or extraction success.
- `run_configured_eval.sh` now records an isolated run manifest with base/overlay/effective configuration hashes, a dataset-tree hash, sanitized environment fingerprints, evaluator logs, and outcome. Only a newly created raw JSON with a passed smoke test and zero evaluator exit code is marked report-eligible.
- `run_all.sh` is intentionally gated by explicit full-run, retrieval-comparison, and destructive-unlearning approvals. It does not authorize the blocked retrieval comparison described below.

## Validated Infrastructure

| Check | Evidence | Status |
|---|---|---|
| Protected causal endpoint | Unauthenticated `401`; authenticated `200` | Pass |
| Protected unlearning endpoint | Unauthenticated `401`; authenticated `200` | Pass |
| Cross-store cleanup | `results/cleanup/cleanup_20260717_041438Z.json` | Pass |
| Retrieval corpus | 30 docs / 50 queries; corpus and query SHA-256 validated | Pass |
| Retrieval corpus exact Qdrant seed | 30 scoped `doc_id` values matched fixed corpus before unlearning | Pass |

## Executed Live Experiments

### Causal Extraction

- Raw result: `results/raw/causal_20260717_132701.json`
- Fixture: 20 annotated O-M-P-R cases.
- Valid API responses: 20/20.
- Relation-found rate: 55.0%.
- Strict O-M-P-R tuple accuracy: 10.0% (2/20).
- Average valid-response latency: 15053.3 ms.

This is a valid baseline result. It demonstrates that the current `qwen2.5:3b` setup is insufficient for a high-accuracy causal claim. Do not place a high accuracy or PCCM-threshold-passing claim in the PPT.

### Security Representative Preflight

- Raw result: `results/raw/security_20260717_053505Z.json`
- Dataset validation: 600 canonical samples across six attack categories.
- Executed samples: one representative sample per category (6 total), not 600.
- Valid API responses: 6/6.
- Explicit defenses: 0.
- Observable PII-leak attack successes: 0.
- Inconclusive responses: 6.
- Average latency: 49049.8 ms.

The representative preflight confirms the authenticated chat path and the conservative error/metric accounting. It does **not** establish a 600-sample ASR or defense-success result and must not be charted as one.

### Machine Unlearning

- Raw result: `results/raw/unlearning_20260717_053554Z.json`
- Collection: `nursing_records`.
- Pre-delete vector count: 30.
- Removed vectors: 30.
- Post-delete vector count: 0.
- Retrieval probe: passed.
- Verification: `is_fully_unlearned=true`.
- Delete latency: 56.193 ms.

This is valid evidence for the configured Qdrant user-level unlearning path. It intentionally deleted the isolated evaluation user's vectors; reseeding is required before another retrieval run.

## Retrieval Evidence Status

- Deterministic patient-name graph ingestion is deployed. It persists `user_id`, `session_id`, `workspace`, and real `doc_id` provenance.
- Full-system trace: `results/trace/trace_rrf_queries_20260717_140126.json`.
  - 10/10 requests succeeded with unique, nonempty `doc_id` values.
  - Patient queries produced actual `knowledge + timeline + vector` evidence and `rrf_3_sources` metadata.
  - Queries without graph or timeline matches explicitly report a vector-only or two-source fallback; they are not labelled as three-source fusion.
- Official Naive RAG result: `results/raw/retrieval_20260717_142118.json`.
  - 50/50 valid API responses; 50/50 scorable responses.
  - MRR `0.405`, Precision@5 `0.168`, Recall@5 `0.565`, average retrieval latency `113.7 ms`.
  - The result passed `verify_retrieval_evidence.py` and has runner provenance, config hashes, and a dataset-tree hash.
- Baseline configurations now use a real vector fallback for `evaluationRetrievalOnly`, with explicit metadata: active source `vector`; empty sources `knowledge`, `timeline`; fusion mode `vector_only_fallback`.
- An attempted full-system 50-query cycle started at `results/runs/20260717T062142Z_full_system_29884` exceeded the execution environment limit during full LLM graph/timeline corpus preparation. It produced no raw result and is not report eligible. The bounded evaluation scope was subsequently cleaned.
- `create_context` now persists the JWT user ID into session metadata before storage. A single-document write followed by `results/cleanup/cleanup_20260717_073137Z.json` verified that cascade cleanup can enforce ownership without relying on a later retrieval request.

## PPT-Safe Material

Use:

- Cross-store cleanup evidence and JWT route protection.
- The causal baseline table with its actual 10.0% strict tuple result and 20/20 API validity.
- The machine-unlearning before/after count table: `30 -> 0`.
- A three-source retrieval trace table labeled "10-query source-provenance trace", including explicit fallback rows.
- The single official Naive RAG 50-query metric table, labelled as a baseline only.

Do not use:

- Historical MRR values produced before strict source and scope validation.
- Any 600-sample security ASR/defense percentage.
- Any claimed three-source retrieval improvement until the full-system 50-query cycle completes.
- Any unlearning percentage inferred from API errors or fabricated side-effect values.

## Remaining Release Gates

1. Complete the full-system 50-query cycle after bounding or accelerating full LLM graph/timeline corpus preparation; rerun from a clean scope.
2. Run `baseline_rag_with_filter` in the same independent restart/clean/seed/trace/50-query process before any multi-configuration comparison chart.
3. Reduce authenticated chat latency before considering the 600-sample security evaluation; current representative latency would make it multi-hour and unsuitable for an interactive demo.
