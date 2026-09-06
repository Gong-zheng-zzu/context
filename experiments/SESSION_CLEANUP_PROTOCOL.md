# Evaluation Session Cleanup Protocol

`scripts/cleanup_eval_session.py` is the only supported destructive cleanup
tool for retrieval experiments. Its scope is hard-coded to:

- user: `eval_user_001`
- session: `eval_retrieval_test`

It authenticates with `EVAL_AUTH_TOKEN`, or with `EVAL_USER_ID` and
`EVAL_PASSWORD`, then invokes the JWT-protected endpoint:

```text
DELETE /api/sessions/eval_retrieval_test?dry_run=true
DELETE /api/sessions/eval_retrieval_test
```

The endpoint must return a result for `qdrant`, `timescaledb`, `neo4j`, and
`session_file_cache`. The tool records every pre-delete, deleted, and
post-delete count in `experiments/results/cleanup/`. It fails unless every
post-delete count is zero and the server reports `complete: true`.

Run after the protected route has been mounted:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = '<evaluation password>'
python experiments/scripts/cleanup_eval_session.py --confirm
```

The operation must not be replaced with direct database deletion in formal
experiments, because doing so would bypass the audited cross-store cascade.
