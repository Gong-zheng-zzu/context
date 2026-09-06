# Retrieval Trace Findings - 2026-07-14

## Scope

- Target configuration: `full_system`
- Authenticated user: `eval_user_001`
- Session: `eval_retrieval_test`
- Trace file: `results/trace/trace_queries_20260714_151923.json`
- Query count: 10

## Verified Facts

1. All 10 authenticated retrieval requests completed successfully.
2. The retrieval handler received `maxResults=5`, but the service log shows
   `Limit=10` and returned 10 structured contexts for every request.
3. Each traced request generated a 768-dimensional embedding and called the
   Qdrant vector-store search path.
4. The captured logs contain no knowledge-graph retrieval, timeline retrieval,
   RRF-fusion, or fusion-weight execution records for these requests.
5. The returned top documents are UUIDs and repeat heavily across unrelated
   queries. They do not match the expected Ground Truth document IDs used by
   the retrieval evaluator.
6. Query-vector generation took about 0.3-0.4 seconds, while end-to-end trace
   latency was about 16-20 seconds. The dominant cost is the preceding Ollama
   intent-analysis prompt, not Qdrant search.

## Conclusion

The current `full_system` trace does not demonstrate three-source retrieval or
RRF fusion. Any prior interpretation of its MRR as a three-source fusion
result is unsupported by this trace. Dynamic RRF weighting must not be
implemented until graph and timeline retrieval are confirmed to be invoked.

## Required Next Fixes

1. Make the effective retrieval limit honor `maxResults` end to end.
2. Re-seed a clean evaluation session and verify returned `contexts[].doc_id`
   values match the Ground Truth corpus before rerunning metrics.
3. Add structured trace logging for vector, graph, timeline, RRF inputs, RRF
   weights, and final ranking.
4. Confirm that the runtime `full_system` configuration initializes and invokes
   graph and timeline stores. If it does not, fix configuration-to-runtime
   mapping before benchmarking fusion.
5. Add an evaluation mode that bypasses LLM intent analysis so retrieval
   latency measures retrieval rather than prompt planning.

## Follow-up Verification

After enabling TimescaleDB and Neo4j, both engines initialized successfully
and `maxResults=5` was verified end to end. However, the enhanced ingestion
path still cannot populate multi-source data:

- The configured LLM provider value `ollama` is rejected by enhanced analysis
  as unsupported, so ingestion falls back and marks timeline, graph, and
  multi-vector storage plans as false.
- Multi-dimensional retrieval invokes the timeline adapter with an empty
  `UserID`, which fails timeline query validation. Knowledge retrieval also
  receives zero queries.

The next implementation work is therefore provider normalization for enhanced
analysis and authenticated user-context propagation into the multi-dimensional
retriever. Do not benchmark RRF until both are verified by trace logs.
