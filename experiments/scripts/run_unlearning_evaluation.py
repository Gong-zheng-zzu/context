#!/usr/bin/env python3
"""Run the bounded unlearning evaluator against the fixed 30-document corpus.

This wrapper only assembles audited command-line arguments. The evaluator still
owns authentication, destructive-operation authorization, seeding, deletion,
and all fail-closed verification. It is intentionally suitable for wrapping
with run_configured_eval.ps1 so the resulting raw JSON receives runner
provenance.
"""

import json
import subprocess
import sys
from pathlib import Path


SCRIPTS_DIR = Path(__file__).resolve().parent
EXPERIMENTS_DIR = SCRIPTS_DIR.parent
CORPUS_PATH = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json"


def main() -> int:
    corpus = json.loads(CORPUS_PATH.read_text(encoding="utf-8"))
    documents = corpus.get("documents")
    if not isinstance(documents, list) or len(documents) != 30:
        raise SystemExit("The fixed unlearning corpus must contain exactly 30 documents.")

    doc_ids = [item.get("doc_id") for item in documents if isinstance(item, dict)]
    if len(doc_ids) != 30 or any(not isinstance(doc_id, str) or not doc_id for doc_id in doc_ids):
        raise SystemExit("The fixed unlearning corpus has invalid doc_id values.")

    command = [
        sys.executable,
        str(SCRIPTS_DIR / "unlearning_eval.py"),
        "--control-session",
        "eval_unlearning_control",
        "--seed-command",
        "python experiments/scripts/prepare_unlearning_trial_seed.py --trial {trial} --base-url http://127.0.0.1:8088",
        "--trials",
        "3",
    ]
    for doc_id in doc_ids:
        command.extend(["--target-doc-id", doc_id])
    return subprocess.run(command, cwd=EXPERIMENTS_DIR.parent).returncode


if __name__ == "__main__":
    raise SystemExit(main())
