# Structured Three-Way Seed

`scripts/prepare_structured_threeway_seed.py` builds an audited, fixed input set from
`datasets/retrieval_corpus/retrieval_corpus_30.json`. It preserves each document's
`doc_id`, patient fields, timestamp, tags, event fields, causal fields, and structured
fields in a `/mcp/tools/create_context` request. The tool does not alter the corpus or
the existing `seed_vector_data.py` workflow.

The scope is deliberately fixed. Both scripts reject any account other than
`eval_user_001` and any session other than `eval_retrieval_test`. Passwords and JWTs are
read only from environment variables and are never written to artifacts.

## Artifacts

The default output directory is
`experiments/results/structured_threeway_seed/eval_retrieval_test/`.

- `manifest.json` records the corpus SHA-256, per-record hashes, per-request hashes,
  source metadata, scope, and the unified-ingestion contract.
- `create_context_requests.jsonl` contains the exact JSON payloads that can be sent to
  the JWT-protected API. Its SHA-256 is recorded in the manifest.
- `submission.jsonl` is created only by `--submit`; it records HTTP acceptance and
  response hashes, not credentials or response bodies.
- `verification.json` records input-integrity results and available retrieval evidence.

## Commands

From `D:\context\context-keeper-main`:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
python experiments/scripts/prepare_structured_threeway_seed.py --dry-run
python experiments/scripts/verify_structured_threeway_seed.py --dry-run
```

`--dry-run` writes the audit artifacts but makes no HTTP request and needs no password.
After reviewing the generated manifest, submit the same bounded corpus through the
existing JWT API:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = '<evaluation password>'
python experiments/scripts/prepare_structured_threeway_seed.py --submit --overwrite
python experiments/scripts/verify_structured_threeway_seed.py --require-vector-evidence
```

The submit command obtains a JWT from `/api/auth/login` and sends each request to
`/mcp/tools/create_context`. It does not initialize another session, delete data, or
accept arbitrary user/session values.

## Verification Boundary

The server's unified `create_context` handler can invoke vector, timeline, and graph
storage internally. Its HTTP `200` response is only recorded as
`accepted_by_unified_ingestion`; it is not reported as proof that all three stores
persisted data.

The documented external API has a retrieval endpoint that can provide document-level
vector evidence, so the verification script checks each `doc_id` through
`/mcp/tools/retrieve_context`. There is no documented session- and document-scoped
timeline or graph read endpoint. The verifier consequently reports those lanes as
`not_independently_verifiable`; it does not infer success from global causal statistics
or a write response. A backend contract that exposes scoped timeline and graph reads is
required before those two lanes can be promoted to independently verified.
