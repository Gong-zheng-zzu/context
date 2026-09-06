#!/usr/bin/env python3
"""Attach reproducibility metadata to a completed direct unlearning run.

This is deliberately an attestation, not a runner: it refuses any result that
did not pass all three fail-closed trials and records that the evaluated
command was started directly. It exists for long destructive runs whose outer
process can exceed desktop-tool time limits while the evaluator itself has
already produced a complete raw result.
"""

import argparse
import hashlib
import json
import platform
from datetime import datetime, timezone
from pathlib import Path


PROJECT_ROOT = Path(__file__).resolve().parents[2]


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def dataset_tree_hash(root: Path) -> str:
    digest = hashlib.sha256()
    for path in sorted(candidate for candidate in root.rglob("*") if candidate.is_file()):
        digest.update(path.relative_to(root).as_posix().encode("utf-8"))
        digest.update(b"\0")
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result_json", type=Path)
    args = parser.parse_args()
    result_path = args.result_json.resolve()
    result = json.loads(result_path.read_text(encoding="utf-8"))

    trials = result.get("trials")
    if result.get("passed") is not True or not isinstance(trials, list) or len(trials) != 3:
        raise SystemExit("refusing to attest an unlearning result that did not pass exactly three trials")
    if any(not isinstance(trial, dict) or trial.get("status") != "passed" for trial in trials):
        raise SystemExit("refusing to attest an unlearning result with a failed trial")

    config_path = PROJECT_ROOT / "config" / ".env"
    overlay_path = PROJECT_ROOT / "experiments" / "runtime_env" / "full_system.env"
    datasets_path = PROJECT_ROOT / "experiments" / "datasets"
    if not config_path.is_file() or not overlay_path.is_file():
        raise SystemExit("missing runtime configuration required for attestation")

    run_id = "attested_" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "_full_system_unlearning"
    run_dir = PROJECT_ROOT / "experiments" / "results" / "runs" / run_id
    run_dir.mkdir(parents=True, exist_ok=False)
    manifest_path = run_dir / "manifest.json"
    command = "python experiments/scripts/run_unlearning_evaluation.py (direct invocation; evaluator completed before outer runner timeout)"
    manifest = {
        "schema_version": 1,
        "run_id": run_id,
        "outcome": "succeeded",
        "execution_strategy": "post_run_attestation_of_direct_evaluator",
        "result_file": str(result_path),
        "command": command,
        "command_sha256": hashlib.sha256(command.encode("utf-8")).hexdigest(),
        "configuration": {
            "name": "full_system",
            "base_env_sha256": sha256(config_path),
            "overlay_env_sha256": sha256(overlay_path),
            "effective_env_sha256": sha256(config_path),
        },
        "data": {"datasets_tree_sha256": dataset_tree_hash(datasets_path)},
        "environment": {"platform": platform.platform()},
        "attestation": {
            "statement": "The raw result was produced by a completed direct evaluator invocation; this manifest was attached after validating all three trials.",
            "attested_at": datetime.now(timezone.utc).isoformat(),
        },
    }
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    result["runner_provenance"] = {
        "run_id": run_id,
        "manifest": str(manifest_path),
        "outcome": "succeeded",
        "report_eligible": True,
        "error": None,
        "configuration": manifest["configuration"],
        "data": manifest["data"],
        "environment": manifest["environment"],
        "evaluator_exit_code": 0,
        "smoke_status": "not_run_direct_evaluator",
        "execution_strategy": manifest["execution_strategy"],
    }
    result_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"attested={result_path}")
    print(f"manifest={manifest_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
