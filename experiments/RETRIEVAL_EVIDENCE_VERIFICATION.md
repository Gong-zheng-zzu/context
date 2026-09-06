# Retrieval Evidence Verification

Validate a saved retrieval result without changing the result file or contacting the service:

```powershell
cd D:\context\context-keeper-main
python experiments\scripts\verify_retrieval_evidence.py `
  experiments\results\raw\retrieval_YYYYMMDD_HHMMSS.json
```

The command prints `PASS` and exits 0 only when the result captures the
`vector_contexts_only` mode, all four runtime configuration fields, the
ground-truth SHA-256, unique non-empty detail doc IDs, matching
scorable/unscorable counts, and Recall@5 values no greater than 1. It prints
`FAIL` and exits nonzero otherwise.
