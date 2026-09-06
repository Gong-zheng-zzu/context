# Experiment Master Plan

## Scope

This plan defines reproducible experiments for Context-Keeper. A feature is
reported as effective only after its API path, data provenance, and metric
denominator have been verified in the same run.

## System Logic

1. Authenticated requests enter the HTTP or MCP handlers.
2. Chat input passes adversarial-sample and multi-layer sensitive-information
   checks before it reaches memory storage or model generation.
3. Context storage applies security scanning, then writes memory data to the
   vector store and, when enabled, timeline and knowledge-graph stores.
4. Retrieval uses session and user information to select relevant memory.
   The current `contextsOnly` endpoint is a vector-only evaluation path.
5. Causal extraction exposes a separate `/api/v1/causal/*` API and uses PCCM
   confidence fusion.
6. Machine unlearning exposes `/api/v1/unlearning/*` and must be verified by
   a before/after retrieval check, not only by a DELETE response.

## Evidence Levels

| Level | Meaning | Allowed claim |
|---|---|---|
| Code | Module exists and compiles | Implementation exists |
| API | Authenticated request returns an expected response | Capability is callable |
| Metric | Fixed data, configuration, raw output and denominator are recorded | Measured effectiveness |

## Experiment Matrix

| Experiment | Dataset | Baselines | Required metrics | Release gate |
|---|---|---|---|---|
| Retrieval | 30 real nursing records, 50 fixed queries, 7 ground-truth docs | Naive vector, vector plus filter, full system | MRR, Precision@5, Recall@5, success/error count, P95 latency | Every returned item has unique `doc_id`; each configuration runs after a separate service start; all claimed sources appear in trace logs |
| Input/output security | Fixed benign and attack samples | Protection disabled, protection enabled | Attack success rate, benign pass rate, false-positive rate, latency | HTTP/auth failures excluded from ASR; identical samples and model/config evidence used for both groups |
| Causal reasoning | Human-annotated nursing records | Rule, LLM, PCCM fusion | Precision, Recall, F1, extraction rate, latency | API produces structured relations; labels and matching rules are versioned |
| Machine unlearning | Seeded isolated test users | No unlearning, requested unlearning | residual retrieval rate, verification pass rate, duration, collateral impact | Before/delete/after/verify sequence succeeds; destructive target is explicitly supplied through environment variables |

## Current Retrieval Status

- Dataset validation passes: 30 documents, 50 queries, temporal/causal/general
  distribution of 17/17/16, and no missing ground-truth reference.
- Vector-only `contextsOnly` responses can return unique source `doc_id`
  values and may be used only as a vector retrieval baseline.
- Timeline and Neo4j source-document fields have been added to the build but
  require a service restart and a clean reseed before they are experimental
  evidence.
- Full-system RRF metrics are blocked until graph and timeline results are
  returned as scoped source `doc_id` values and the RRF branch is traced.

## Formal Run Order

1. Validate the retrieval dataset and record both SHA-256 values.
2. Build and restart one configuration.
3. Seed the same corpus into a clean evaluation session.
4. Run a 10-query trace and check source execution, scope isolation, unique
   document IDs, and no generated answer path.
5. Run one 50-query evaluation for that configuration.
6. Run the evidence validator on the raw result.
7. Repeat steps 2-6 for the remaining configurations.
8. Generate figures only from validated raw results.

## Claude Code Boundary

Claude Code may build/restart containers, seed evaluation data, run fixed
scripts, collect logs, and validate generated evidence. It must not select
metrics, alter raw results, replace failed calls with offline samples, or
declare a configuration comparison complete.
