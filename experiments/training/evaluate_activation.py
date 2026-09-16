#!/usr/bin/env python3
"""Evaluate custom/base/rules predictions and issue an activation artifact only on success."""
from __future__ import annotations

import argparse
import json
import math
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pipeline_common import (
    LABELS,
    PIPELINE_SCHEMA,
    percentile,
    read_jsonl,
    validate_relation_evidence,
    value_sha256,
    write_json_atomic,
)
from prepare_reviewed_dataset import validate_prepared_dataset

KINDS = ("custom", "base", "rules")
NEGATION_MARKERS = ("无", "未", "否认", "排除")
TEMPORAL_MARKERS = ("随后", "之前", "之后", "当日", "夜间", "最近", "持续", "最终")


def relation_tuple(relation: dict[str, Any]) -> tuple[str, str, str, str]:
    values = []
    for label in LABELS:
        value = relation.get(label)
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"relation field {label} must be a non-empty string")
        values.append(value.strip())
    return tuple(values)  # type: ignore[return-value]


def load_predictions(path: Path, kind: str, dataset_hash: str, records: dict[str, dict[str, Any]]) -> dict[str, Any]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if payload.get("schema_version") != PIPELINE_SCHEMA or payload.get("model_kind") != kind:
        raise ValueError(f"{path} has an invalid prediction schema or model_kind")
    if payload.get("dataset_artifact_sha256") != dataset_hash:
        raise ValueError(f"{path} predictions do not belong to this frozen dataset")
    if not isinstance(payload.get("model_id"), str) or not payload["model_id"].strip():
        raise ValueError(f"{path} requires model_id")
    indexed: dict[str, Any] = {}
    for row in payload.get("predictions", []):
        record_id = row.get("record_id")
        if record_id in indexed:
            raise ValueError(f"{path} contains duplicate record_id {record_id}")
        if record_id not in records:
            raise ValueError(f"{path} contains unknown record_id {record_id}")
        latency = row.get("latency_ms")
        if not isinstance(latency, (int, float)) or latency < 0:
            raise ValueError(f"{path} record {record_id} has invalid latency_ms")
        relations = row.get("relations")
        if not isinstance(relations, list):
            raise ValueError(f"{path} record {record_id} relations must be a list")
        validate_relation_evidence(records[record_id]["text"], relations, record_id)
        indexed[record_id] = {"relations": {relation_tuple(item) for item in relations}, "latency_ms": float(latency)}
    missing = records.keys() - indexed.keys()
    if missing:
        raise ValueError(f"{path} is missing {len(missing)} frozen records")
    payload["indexed"] = indexed
    payload["predictions_sha256"] = value_sha256(payload["predictions"])
    return payload


def _f1(tp: int, fp: int, fn: int) -> float:
    denominator = 2 * tp + fp + fn
    return (2 * tp / denominator) if denominator else 1.0


def score(gold_rows: list[dict[str, Any]], prediction: dict[str, Any]) -> dict[str, Any]:
    tuple_tp = tuple_fp = tuple_fn = 0
    field_counts = {label: [0, 0, 0] for label in LABELS}
    negative_total = negative_fp = 0
    exact: dict[str, bool] = {}
    negation_correct = temporal_correct = negation_total = temporal_total = 0
    latencies = []
    for row in gold_rows:
        record_id = row["record_id"]
        gold = {relation_tuple(item) for item in row["relations"]}
        predicted = prediction["indexed"][record_id]["relations"]
        latencies.append(prediction["indexed"][record_id]["latency_ms"])
        tuple_tp += len(gold & predicted)
        tuple_fp += len(predicted - gold)
        tuple_fn += len(gold - predicted)
        exact[record_id] = gold == predicted
        for index, label in enumerate(LABELS):
            gold_values = {item[index] for item in gold}
            predicted_values = {item[index] for item in predicted}
            field_counts[label][0] += len(gold_values & predicted_values)
            field_counts[label][1] += len(predicted_values - gold_values)
            field_counts[label][2] += len(gold_values - predicted_values)
        if not gold:
            negative_total += 1
            negative_fp += int(bool(predicted))
        if any(marker in row["text"] for marker in NEGATION_MARKERS):
            negation_total += 1
            negation_correct += int(exact[record_id])
        if any(marker in row["text"] for marker in TEMPORAL_MARKERS):
            temporal_total += 1
            temporal_correct += int(exact[record_id])
    field_f1 = {label: _f1(*counts) for label, counts in field_counts.items()}
    return {
        "model_id": prediction["model_id"],
        "model_kind": prediction["model_kind"],
        "predictions_sha256": prediction["predictions_sha256"],
        "records": len(gold_rows),
        "strict_tuple_f1": _f1(tuple_tp, tuple_fp, tuple_fn),
        "strict_tuple_counts": {"tp": tuple_tp, "fp": tuple_fp, "fn": tuple_fn},
        "field_f1": field_f1,
        "field_macro_f1": sum(field_f1.values()) / len(field_f1),
        "no_causal_false_positive_rate": negative_fp / negative_total if negative_total else None,
        "negation_accuracy": negation_correct / negation_total if negation_total else None,
        "temporal_accuracy": temporal_correct / temporal_total if temporal_total else None,
        "latency_p50_ms": percentile(latencies, 0.50),
        "latency_p95_ms": percentile(latencies, 0.95),
        "record_exact": exact,
    }


def one_sided_paired_pvalue(custom: dict[str, bool], baseline: dict[str, bool]) -> dict[str, Any]:
    wins = sum(custom[key] and not baseline[key] for key in custom)
    losses = sum(not custom[key] and baseline[key] for key in custom)
    discordant = wins + losses
    if discordant == 0:
        p_value = 1.0
    else:
        p_value = sum(math.comb(discordant, k) for k in range(wins, discordant + 1)) / (2**discordant)
    return {"wins": wins, "losses": losses, "discordant": discordant, "one_sided_exact_p": p_value}


def evaluate(
    dataset_dir: Path,
    training_run_path: Path,
    prediction_paths: dict[str, Path],
    report_path: Path,
    activation_path: Path,
    alpha: float = 0.05,
) -> dict[str, Any]:
    if not 0 < alpha < 0.5:
        raise ValueError("alpha must be between 0 and 0.5")
    if set(prediction_paths) != set(KINDS):
        raise ValueError(f"prediction paths must contain exactly: {', '.join(KINDS)}")
    # A failed rerun must revoke a stale approval at the requested destination.
    dataset = validate_prepared_dataset(dataset_dir)
    test_path = dataset_dir / dataset["files"]["test"]["path"]
    gold_rows = read_jsonl(test_path)
    records = {row["record_id"]: row for row in gold_rows}
    training_run = json.loads(training_run_path.read_text(encoding="utf-8"))
    stored_run_hash = training_run.get("run_sha256")
    unhashed_run = dict(training_run)
    unhashed_run.pop("run_sha256", None)
    if training_run.get("status") != "training_completed_unapproved" or stored_run_hash != value_sha256(unhashed_run):
        raise ValueError("custom predictions require an intact, completed-but-unapproved training run")
    if training_run.get("dataset_artifact_sha256") != dataset["artifact_sha256"]:
        raise ValueError("training run and frozen evaluation dataset do not match")
    predictions = {kind: load_predictions(path, kind, dataset["artifact_sha256"], records) for kind, path in prediction_paths.items()}
    scores = {kind: score(gold_rows, predictions[kind]) for kind in KINDS}
    significance = {
        baseline: one_sided_paired_pvalue(scores["custom"]["record_exact"], scores[baseline]["record_exact"])
        for baseline in ("base", "rules")
    }
    thresholds = {
        "strict_tuple_f1": scores["custom"]["strict_tuple_f1"] >= 0.75,
        "field_macro_f1": scores["custom"]["field_macro_f1"] >= 0.88,
        "no_causal_false_positive_rate": scores["custom"]["no_causal_false_positive_rate"] is not None
        and scores["custom"]["no_causal_false_positive_rate"] <= 0.05,
        "negation_accuracy": scores["custom"]["negation_accuracy"] is not None and scores["custom"]["negation_accuracy"] >= 0.95,
        "temporal_accuracy": scores["custom"]["temporal_accuracy"] is not None and scores["custom"]["temporal_accuracy"] >= 0.95,
        "latency_p95_ms": scores["custom"]["latency_p95_ms"] is not None and scores["custom"]["latency_p95_ms"] <= 3000,
    }
    comparisons = {
        baseline: scores["custom"]["strict_tuple_f1"] > scores[baseline]["strict_tuple_f1"]
        and significance[baseline]["wins"] > significance[baseline]["losses"]
        and significance[baseline]["one_sided_exact_p"] < alpha
        for baseline in ("base", "rules")
    }
    approved = all(thresholds.values()) and all(comparisons.values())
    public_scores = {kind: {key: value for key, value in result.items() if key != "record_exact"} for kind, result in scores.items()}
    report = {
        "schema_version": PIPELINE_SCHEMA,
        "artifact_type": "frozen_model_comparison",
        "dataset_artifact_sha256": dataset["artifact_sha256"],
        "source_dataset_sha256": dataset["source_dataset_sha256"],
        "review_chain_head": dataset["review_chain_head"],
        "training_run_sha256": stored_run_hash,
        "evaluated_at": datetime.now(timezone.utc).isoformat(),
        "scores": public_scores,
        "significance": significance,
        "alpha": alpha,
        "threshold_checks": thresholds,
        "significant_improvement_checks": comparisons,
        "approved_for_activation": approved,
        "note": "Measured values are retained even when the activation gate fails.",
    }
    report["report_sha256"] = value_sha256(report)
    # A completed failed rerun revokes a stale approval at this destination.
    activation_path.unlink(missing_ok=True)
    write_json_atomic(report_path, report)
    if approved:
        activation = {
            "schema_version": PIPELINE_SCHEMA,
            "artifact_type": "model_activation_approval",
            "approved_for_activation": True,
            "ollama_model": "contextkeeper-causal-3b",
            "base_model": training_run["base_model"],
            "dataset_artifact_sha256": dataset["artifact_sha256"],
            "training_run_sha256": stored_run_hash,
            "adapter_fingerprint": training_run["adapter_fingerprint"],
            "evaluation_report_sha256": report["report_sha256"],
            "custom_predictions_sha256": scores["custom"]["predictions_sha256"],
        }
        activation["activation_sha256"] = value_sha256(activation)
        write_json_atomic(activation_path, activation)
    return report


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--training-run", type=Path, required=True)
    parser.add_argument("--custom", type=Path, required=True)
    parser.add_argument("--base", type=Path, required=True)
    parser.add_argument("--rules", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--activation", type=Path, required=True)
    parser.add_argument("--alpha", type=float, default=0.05)
    args = parser.parse_args()
    result = evaluate(
        args.dataset,
        args.training_run,
        {"custom": args.custom, "base": args.base, "rules": args.rules},
        args.report,
        args.activation,
        args.alpha,
    )
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0 if result["approved_for_activation"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
