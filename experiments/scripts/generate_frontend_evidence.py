#!/usr/bin/env python3
"""Publish a minimal, non-sensitive experiment summary for the static demo UI."""

from __future__ import annotations

import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
RAW_DIR = ROOT / "experiments" / "results" / "raw"
OUTPUT_PATH = ROOT / "web" / "data" / "experiment_evidence.json"
SECURITY_FILE = RAW_DIR / "security_20260726_080832Z.json"
CAUSAL_FILE = RAW_DIR / "causal_20260726_081306Z.json"


def load_json(path: Path) -> tuple[dict, str]:
    if not path.is_file():
        raise RuntimeError(f"required source result does not exist: {path}")
    raw = path.read_bytes()
    try:
        value = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"invalid JSON source: {path}") from exc
    if not isinstance(value, dict):
        raise RuntimeError(f"source JSON must be an object: {path}")
    return value, hashlib.sha256(raw).hexdigest()


def require(value: dict, key: str, expected_type: type):
    result = value.get(key)
    if not isinstance(result, expected_type):
        raise RuntimeError(f"missing or invalid {key}")
    return result


def percent(value: float) -> float:
    return round(value * 100, 2)


def build_security(source: dict, source_hash: str) -> dict:
    metrics = require(source, "aggregated_metrics", dict)
    if metrics.get("metrics_status") != "complete":
        raise RuntimeError("security result is not complete")
    total = require(metrics, "total_samples", int)
    valid = require(metrics, "api_success_count", int)
    benign_total = require(metrics, "benign_total", int)
    benign_valid = require(metrics, "benign_api_success_count", int)
    if total != 120 or valid != total or benign_total != 100 or benign_valid != benign_total:
        raise RuntimeError("security result did not pass the publication validity gate")

    attack_ci = require(metrics, "attack_success_wilson_95_ci", list)
    defense_ci = require(metrics, "defense_success_wilson_95_ci", list)
    false_positive_ci = require(metrics, "false_positive_wilson_95_ci", list)
    if not all(isinstance(item, (int, float)) for item in attack_ci + defense_ci + false_positive_ci):
        raise RuntimeError("security confidence intervals are invalid")

    return {
        "status": "verified",
        "source": SECURITY_FILE.name,
        "source_sha256": source_hash,
        "run_at_utc": require(source, "timestamp_utc", str),
        "attack_valid": {"numerator": valid, "denominator": total},
        "benign_valid": {"numerator": benign_valid, "denominator": benign_total},
        "attack_success_rate_pct": percent(require(metrics, "attack_success_rate", (int, float))),
        "block_rate_pct": percent(require(metrics, "defense_success_rate", (int, float))),
        "false_positive_rate_pct": percent(require(metrics, "false_positive_rate", (int, float))),
        "block_rate_ci95_pct": [percent(defense_ci[0]), percent(defense_ci[1])],
        "attack_success_ci95_pct": [percent(attack_ci[0]), percent(attack_ci[1])],
        "false_positive_ci95_pct": [percent(false_positive_ci[0]), percent(false_positive_ci[1])],
        "avg_latency_ms": round(require(metrics, "avg_latency_ms", (int, float)), 1),
    }


def build_causal(source: dict, source_hash: str) -> dict:
    metrics = require(source, "metrics", dict)
    preflight = require(source, "preflight", dict)
    if preflight.get("health") != "服务正常":
        raise RuntimeError("causal health preflight did not pass")
    total = require(metrics, "total_samples", int)
    valid = require(metrics, "api_success_count", int)
    if total != 20 or valid != total:
        raise RuntimeError("causal result did not pass the publication validity gate")

    return {
        "status": "verified",
        "source": CAUSAL_FILE.name,
        "source_sha256": source_hash,
        "run_at_utc": require(source, "timestamp_utc", str),
        "valid": {"numerator": valid, "denominator": total},
        "relation_found_rate_pct": percent(require(metrics, "relation_found_rate", (int, float))),
        "strict_precision_pct": percent(require(metrics, "strict_tuple_precision", (int, float))),
        "strict_recall_pct": percent(require(metrics, "strict_tuple_recall", (int, float))),
        "strict_f1_pct": percent(require(metrics, "strict_tuple_f1", (int, float))),
        "avg_confidence_pct": percent(require(metrics, "avg_best_relation_confidence", (int, float))),
        "avg_latency_ms": round(require(metrics, "avg_latency_ms", (int, float)), 1),
    }


def main() -> None:
    security_source, security_hash = load_json(SECURITY_FILE)
    causal_source, causal_hash = load_json(CAUSAL_FILE)
    output = {
        "schema_version": 1,
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "publication_policy": "Whitelist summary only. No raw requests, responses, patients, credentials, or tokens.",
        "security": build_security(security_source, security_hash),
        "causal": build_causal(causal_source, causal_hash),
        "retrieval": {
            "status": "in_progress",
            "message": "100 条人工复核查询与正式规模检索验证进行中，不展示历史或未复核指标。",
        },
        "unlearning": {
            "status": "in_progress",
            "message": "跨会话隔离验证修复中，不形成效果结论。",
        },
    }
    OUTPUT_PATH.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT_PATH.write_text(json.dumps(output, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"published safe evidence summary: {OUTPUT_PATH}")


if __name__ == "__main__":
    main()
