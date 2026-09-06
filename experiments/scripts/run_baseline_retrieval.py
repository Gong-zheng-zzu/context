#!/usr/bin/env python3
"""Run one prepared vector-retrieval baseline without managing Docker.

The operator must start the requested baseline runtime before invoking this
script. This runner only acts on the fixed evaluation session and records a
manifest for the clean -> seed -> trace -> evaluate -> evidence sequence.
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List


ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
BASELINES = {
    "baseline_naive_rag": False,
    "baseline_rag_with_filter": True,
}
EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
PROJECT_ROOT = EXPERIMENTS_DIR.parent
SCRIPTS_DIR = EXPERIMENTS_DIR / "scripts"
RUNTIME_ENV_DIR = EXPERIMENTS_DIR / "runtime_env"


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def parse_env_file(path: Path) -> Dict[str, str]:
    values: Dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def validate_overlay(config_name: str, path: Path) -> None:
    values = parse_env_file(path)
    expected_filter = BASELINES[config_name]
    required = {
        "VECTOR_STORE_ENABLED": "true",
        "GRAPH_STORE_ENABLED": "false",
        "TIMELINE_ENABLED": "false",
        "RETRIEVAL_RULE_FILTER_ENABLED": str(expected_filter).lower(),
    }
    mismatches = [
        f"{key}={values.get(key)!r} (expected {expected!r})"
        for key, expected in required.items()
        if values.get(key, "").lower() != expected
    ]
    if mismatches:
        raise ValueError("runtime overlay is not the requested vector baseline: " + "; ".join(mismatches))


def write_manifest(path: Path, manifest: Dict[str, Any]) -> None:
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def run_stage(name: str, command: List[str], env: Dict[str, str], run_dir: Path, stages: List[Dict[str, Any]]) -> None:
    log_path = run_dir / f"{name}.log"
    stage = {"name": name, "command": command, "log": str(log_path), "started_at": utc_now()}
    stages.append(stage)
    with log_path.open("w", encoding="utf-8") as log_file:
        process = subprocess.run(
            command,
            cwd=PROJECT_ROOT,
            env=env,
            stdout=log_file,
            stderr=subprocess.STDOUT,
            text=True,
        )
    stage["ended_at"] = utc_now()
    stage["exit_code"] = process.returncode
    if process.returncode != 0:
        raise RuntimeError(f"stage {name} failed with exit code {process.returncode}; see {log_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("config_name", choices=sorted(BASELINES))
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--dry-run", action="store_true", help="Validate data and prepare the seed manifest without API requests.")
    args = parser.parse_args()

    config_path = RUNTIME_ENV_DIR / f"{args.config_name}.env"
    if not config_path.is_file():
        parser.error(f"missing runtime overlay: {config_path}")
    try:
        validate_overlay(args.config_name, config_path)
    except (OSError, ValueError) as error:
        parser.error(str(error))

    if os.environ.get("EVAL_USER_ID", "").strip() != ALLOWED_USER_ID:
        parser.error(f"EVAL_USER_ID must be {ALLOWED_USER_ID!r}")
    if not args.dry_run and os.environ.get("EVAL_BASELINE_RUNTIME_READY", "").strip() != args.config_name:
        parser.error(
            "Set EVAL_BASELINE_RUNTIME_READY to the selected config after an operator has started that baseline. "
            "This runner never starts or restarts Docker."
        )
    if not args.dry_run and not os.environ.get("EVAL_PASSWORD", ""):
        parser.error(
            "Set EVAL_PASSWORD. The audited seed and seed-evidence stages authenticate by login, "
            "so EVAL_AUTH_TOKEN alone is insufficient."
        )

    started_at = utc_now()
    run_id = f"{started_at.replace(':', '').replace('-', '')}_{args.config_name}"
    run_dir = EXPERIMENTS_DIR / "results" / "baseline_runs" / run_id
    run_dir.mkdir(parents=True, exist_ok=False)
    manifest_path = run_dir / "manifest.json"
    seed_dir = run_dir / "seed"
    trace_path = run_dir / "trace.json"
    # The configured runtime runner treats a new raw result as the only
    # report-eligible artifact. Keep supporting evidence in this run folder,
    # but write the evaluator output directly to the shared raw-results area
    # so the outer runner can attach its configuration provenance.
    raw_results_dir = EXPERIMENTS_DIR / "results" / "raw"
    raw_results_dir.mkdir(parents=True, exist_ok=True)
    result_path = raw_results_dir / f"retrieval_{run_id}.json"
    cleanup_path = run_dir / "cleanup.json"
    seed_verification_path = run_dir / "seed_verification.json"
    stages: List[Dict[str, Any]] = []
    outcome = "running"
    error_message = ""

    env = dict(os.environ)
    # The seed stages log in with EVAL_PASSWORD. Keep every stage on that same
    # credential path instead of allowing a stale inherited JWT to split a run.
    env.pop("EVAL_AUTH_TOKEN", None)
    env.update(
        {
            "EVAL_RUNTIME_CONFIG_NAME": args.config_name,
            "EVAL_RUNTIME_CONFIG_FILE": str(config_path),
            "EVAL_RUNTIME_CONFIG_HASH": sha256_file(config_path),
            "EVAL_SERVICE_START_TIME": started_at,
            "EVAL_RUN_ID": run_id,
        }
    )

    manifest: Dict[str, Any] = {
        "schema_version": 1,
        "run_id": run_id,
        "started_at": started_at,
        "outcome": outcome,
        "execution_mode": "dry_run" if args.dry_run else "live_api",
        "runtime": {
            "config_name": args.config_name,
            "runtime_env_file": str(config_path),
            "runtime_env_sha256": sha256_file(config_path),
            "operator_attestation": os.environ.get("EVAL_BASELINE_RUNTIME_READY", ""),
            "docker_managed_by_runner": False,
            "authentication_strategy": "password_login",
        },
        "evidence_requirements": {
            "trace_query_count": 10,
            "trace_requests_must_succeed_for_filter_baseline": BASELINES[args.config_name],
            "trace_candidate_filter_required": BASELINES[args.config_name],
            "retrieval_result_must_have_sourced_doc_ids": True,
        },
        "scope": {"user_id": ALLOWED_USER_ID, "session_id": ALLOWED_SESSION_ID},
        "data": {
            "corpus": str(EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json"),
            "ground_truth": str(EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "query_answer_pairs.json"),
            "expected_document_count": 30,
            "expected_query_count": 50,
        },
        "artifacts": {
            "cleanup": str(cleanup_path),
            "seed_manifest": str(seed_dir / "manifest.json"),
            "seed_verification": str(seed_verification_path),
            "trace": str(trace_path),
            "retrieval_result": str(result_path),
        },
        "stages": stages,
    }

    try:
        run_stage(
            "validate_dataset",
            [sys.executable, str(SCRIPTS_DIR / "validate_retrieval_dataset.py")],
            env,
            run_dir,
            stages,
        )
        if args.dry_run:
            run_stage(
                "prepare_seed_dry_run",
                [
                    sys.executable,
                    str(SCRIPTS_DIR / "prepare_structured_threeway_seed.py"),
                    "--dry-run",
                    "--overwrite",
                    "--output-dir",
                    str(seed_dir),
                ],
                env,
                run_dir,
                stages,
            )
            outcome = "dry_run_complete"
        else:
            run_stage(
                "cleanup",
                [
                    sys.executable,
                    str(SCRIPTS_DIR / "cleanup_eval_session.py"),
                    "--confirm",
                    "--base-url",
                    args.base_url,
                    "--output",
                    str(cleanup_path),
                ],
                env,
                run_dir,
                stages,
            )
            run_stage(
                "seed_30_documents",
                [
                    sys.executable,
                    str(SCRIPTS_DIR / "prepare_structured_threeway_seed.py"),
                    "--submit",
                    "--overwrite",
                    "--base-url",
                    args.base_url,
                    "--output-dir",
                    str(seed_dir),
                ],
                env,
                run_dir,
                stages,
            )
            run_stage(
                "verify_seed_evidence",
                [
                    sys.executable,
                    str(SCRIPTS_DIR / "verify_structured_threeway_seed.py"),
                    "--manifest",
                    str(seed_dir / "manifest.json"),
                    "--base-url",
                    args.base_url,
                    "--output",
                    str(seed_verification_path),
                    "--require-vector-evidence",
                ],
                env,
                run_dir,
                stages,
            )
            trace_command = [
                sys.executable,
                str(SCRIPTS_DIR / "run_trace_queries.py"),
                "--mode",
                "vector",
                "--limit",
                "10",
                "--base-url",
                args.base_url,
                "--session-id",
                ALLOWED_SESSION_ID,
                "--output",
                str(trace_path),
            ]
            if BASELINES[args.config_name]:
                trace_command.append("--require-candidate-filter")
            run_stage("trace", trace_command, env, run_dir, stages)
            run_stage(
                "evaluate_50_queries",
                [
                    sys.executable,
                    str(SCRIPTS_DIR / "retrieval_eval.py"),
                    "--queries",
                    "50",
                    "--configs",
                    args.config_name,
                    "--retrieval-mode",
                    "vector",
                    "--base-url",
                    args.base_url,
                    "--output",
                    str(result_path),
                ],
                env,
                run_dir,
                stages,
            )
            run_stage(
                "verify_retrieval_evidence",
                [sys.executable, str(SCRIPTS_DIR / "verify_retrieval_evidence.py"), str(result_path)],
                env,
                run_dir,
                stages,
            )
            outcome = "succeeded"
    except RuntimeError as error:
        outcome = "failed"
        error_message = str(error)
        print(f"ERROR: {error}", file=sys.stderr)
    finally:
        manifest["outcome"] = outcome
        manifest["error"] = error_message or None
        manifest["ended_at"] = utc_now()
        for name, path in list(manifest["artifacts"].items()):
            artifact_path = Path(path)
            if artifact_path.is_file():
                manifest["artifacts"][name] = {"path": str(artifact_path), "sha256": sha256_file(artifact_path)}
        write_manifest(manifest_path, manifest)
        print(f"manifest={manifest_path}")

    return 0 if outcome in {"succeeded", "dry_run_complete"} else 1


if __name__ == "__main__":
    sys.exit(main())
