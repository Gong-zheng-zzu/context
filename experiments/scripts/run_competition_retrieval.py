#!/usr/bin/env python3
"""Run one guarded 500-document retrieval evaluation against an already-started runtime.

Docker lifecycle remains outside this runner. Use run_configured_eval.ps1 to
restart one configuration first, then set EVAL_COMPETITION_RUNTIME_READY to
that exact configuration before invoking this command.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

sys.path.insert(0, str(Path(__file__).resolve().parent))
from experiment_integrity import IntegrityConfigurationError, audit_signature, dual_digest_file, utc_now


EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
SCRIPTS_DIR = EXPERIMENTS_DIR / "scripts"
ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"


def sha256(path: Path) -> str:
    return dual_digest_file(path)["sha256"]


def run_stage(name: str, command: List[str], env: Dict[str, str], run_dir: Path, stages: List[Dict[str, Any]]) -> None:
    log_path = run_dir / f"{name}.log"
    stage = {"name": name, "command": command, "log": str(log_path), "started_at": utc_now()}
    stages.append(stage)
    with log_path.open("w", encoding="utf-8") as log:
        process = subprocess.run(command, cwd=EXPERIMENTS_DIR, env=env, stdout=log, stderr=subprocess.STDOUT, text=True)
    stage["ended_at"] = utc_now()
    stage["exit_code"] = process.returncode
    if process.returncode:
        raise RuntimeError(f"{name} failed; see {log_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config-name", choices=("baseline_naive_rag", "baseline_rag_with_filter", "full_system"), required=True)
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--corpus-file", type=Path, default=EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_500.json")
    parser.add_argument("--ground-truth-file", type=Path, default=EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_reviewed.json")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if os.environ.get("EVAL_USER_ID", "").strip() != ALLOWED_USER_ID:
        parser.error(f"EVAL_USER_ID must be {ALLOWED_USER_ID}")
    if not args.dry_run and os.environ.get("EVAL_COMPETITION_RUNTIME_READY", "").strip() != args.config_name:
        parser.error("Set EVAL_COMPETITION_RUNTIME_READY to the already-running configuration; this runner never changes Docker state.")
    if not args.dry_run and not os.environ.get("EVAL_PASSWORD", ""):
        parser.error("EVAL_PASSWORD is required for live API seed, cleanup, trace, and evaluation stages.")
    if not args.corpus_file.is_file() or not args.ground_truth_file.is_file():
        parser.error("corpus or reviewed ground-truth file is missing")

    run_id = f"{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{args.config_name}_500x100"
    run_dir = EXPERIMENTS_DIR / "results" / "competition_runs" / run_id
    run_dir.mkdir(parents=True, exist_ok=False)
    result_path = EXPERIMENTS_DIR / "results" / "raw" / f"retrieval_{run_id}.json"
    stages: List[Dict[str, Any]] = []
    env = dict(os.environ)
    corpus_digest, queries_digest = dual_digest_file(args.corpus_file), dual_digest_file(args.ground_truth_file)
    env.update({"EVAL_RUN_ID": run_id, "EVAL_DATASET_CORPUS_SHA256": corpus_digest["sha256"], "EVAL_DATASET_QUERIES_SHA256": queries_digest["sha256"]})
    mode = "rrf" if args.config_name == "full_system" else "vector"
    manifest: Dict[str, Any] = {
        "schema": "competition-retrieval-run-v1", "run_id": run_id, "started_at": utc_now(), "config_name": args.config_name,
        "mode": mode, "scope": {"user_id": ALLOWED_USER_ID, "session_id": ALLOWED_SESSION_ID},
        "hash_algorithms": ["SHA-256", "SM3"],
        "dataset": {"corpus": str(args.corpus_file), "corpus_sha256": corpus_digest["sha256"], "corpus_dual_digest": corpus_digest, "queries": str(args.ground_truth_file), "queries_sha256": queries_digest["sha256"], "queries_dual_digest": queries_digest},
        "stages": stages, "result": str(result_path), "outcome": "running",
    }
    try:
        validation = [sys.executable, str(SCRIPTS_DIR / "validate_retrieval_dataset.py"), "--corpus", str(args.corpus_file), "--queries", str(args.ground_truth_file), "--expected-corpus-count", "500", "--expected-query-count", "100", "--expected-type-counts", "temporal=34,causal=33,general=33", "--require-reviewed"]
        run_stage("validate_reviewed_dataset", validation, env, run_dir, stages)
        seed_dir = run_dir / "seed"
        seed_command = [sys.executable, str(SCRIPTS_DIR / "prepare_structured_threeway_seed.py"), "--corpus-file", str(args.corpus_file), "--output-dir", str(seed_dir), "--overwrite"]
        if args.dry_run:
            run_stage("prepare_seed_dry_run", seed_command + ["--dry-run"], env, run_dir, stages)
            manifest["outcome"] = "dry_run_complete"
        else:
            run_stage("cleanup", [sys.executable, str(SCRIPTS_DIR / "cleanup_eval_session.py"), "--confirm", "--base-url", args.base_url, "--output", str(run_dir / "cleanup.json")], env, run_dir, stages)
            run_stage("seed", seed_command + ["--submit", "--base-url", args.base_url], env, run_dir, stages)
            trace = [sys.executable, str(SCRIPTS_DIR / "run_trace_queries.py"), "--mode", mode, "--limit", "10", "--base-url", args.base_url, "--session-id", ALLOWED_SESSION_ID, "--output", str(run_dir / "trace.json")]
            run_stage("trace", trace, env, run_dir, stages)
            evaluate = [sys.executable, str(SCRIPTS_DIR / "retrieval_eval.py"), "--queries", "100", "--configs", args.config_name, "--retrieval-mode", mode, "--base-url", args.base_url, "--ground-truth-file", str(args.ground_truth_file), "--corpus-file", str(args.corpus_file), "--require-reviewed-ground-truth", "--output", str(result_path)]
            run_stage("evaluate", evaluate, env, run_dir, stages)
            run_stage("verify_result", [sys.executable, str(SCRIPTS_DIR / "verify_retrieval_evidence.py"), str(result_path)], env, run_dir, stages)
            manifest["outcome"] = "succeeded"
    except RuntimeError as error:
        manifest["outcome"] = "failed"
        manifest["error"] = str(error)
        print(f"ERROR: {error}", file=sys.stderr)
    finally:
        manifest["ended_at"] = utc_now()
        manifest["audit_signature"] = audit_signature({key: value for key, value in manifest.items() if key != "audit_signature"})
        (run_dir / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"manifest={run_dir / 'manifest.json'}")
    return 0 if manifest["outcome"] in {"succeeded", "dry_run_complete"} else 1


if __name__ == "__main__":
    raise SystemExit(main())
