# Retrieval Evaluation Session Cleanup

`experiments/scripts/cleanup_eval_retrieval_session.py` is intentionally bounded to the only disposable evaluation identity:

- User: `eval_user_001`
- Session: `eval_retrieval_test`

It refuses any other `EVAL_USER_ID`, has no user or session override flags, authenticates through the production JWT login endpoint, and runs a mandatory dry-run before deletion. The API reports per-store `before`, `deleted`, and `after` counts for `timescaledb`, `neo4j`, and `session_file_cache`.

Run the non-destructive verification:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = 'demo123'
python experiments/scripts/cleanup_eval_retrieval_session.py
```

Perform the bounded cleanup and post-delete verification:

```powershell
$env:EVAL_USER_ID = 'eval_user_001'
$env:EVAL_PASSWORD = 'demo123'
python experiments/scripts/cleanup_eval_retrieval_session.py --apply
```

Set `API_BASE_URL` or pass `--base-url` only to select the server. The target identity and session remain fixed. A failed store or a nonzero post-delete count is an error; do not treat a partial response as a successful cleanup.
