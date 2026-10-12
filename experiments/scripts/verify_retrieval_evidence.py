#!/usr/bin/env python3
"""Read-only validation for retrieval evaluation result JSON files."""

import argparse
import json
import math
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any, Dict, List


VALID_EVALUATION_MODES = {
    "vector_contexts_only",
    "multi_source_rrf_evaluation_only",
}
RUNTIME_CONFIG_FIELDS = (
    "config_name",
    "config_file",
    "config_hash",
    "service_start_time",
)
UNCAPTURED_VALUES = {"", "not_captured"}
REQUIRED_RRF_SOURCES = ("vector", "knowledge", "timeline")


def is_nonempty_string(value: Any) -> bool:
    return isinstance(value, str) and value.strip() != ""


def is_number(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _validate_rrf_audit(label: str, result: Dict[str, Any]) -> List[str]:
    """Validate the response-level evidence required for a formal RRF run.

    A retrieval score by itself cannot prove that three sources participated.
    Keep this gate deliberately strict: a missing audit object is a failed
    evidence run rather than an implicit vector-only success.
    """
    errors: List[str] = []
    metadata = result.get("retrieval_metadata")
    if not isinstance(metadata, dict) or not metadata:
        return [f"{label}.retrieval_metadata must be a non-empty object for RRF evidence"]

    fusion_mode = metadata.get("retrieval_fusion_mode")
    if not is_nonempty_string(fusion_mode) or fusion_mode == "not_captured":
        errors.append(f"{label}.retrieval_metadata.retrieval_fusion_mode must be captured")

    statuses = metadata.get("source_statuses")
    if not isinstance(statuses, dict):
        errors.append(f"{label}.retrieval_metadata.source_statuses must be an object")
    else:
        for source in REQUIRED_RRF_SOURCES:
            if not is_nonempty_string(statuses.get(source)):
                errors.append(f"{label}.retrieval_metadata.source_statuses.{source} is required")

    latencies = metadata.get("source_latency_ms")
    if not isinstance(latencies, dict):
        errors.append(f"{label}.retrieval_metadata.source_latency_ms must be an object")
    else:
        for source in REQUIRED_RRF_SOURCES:
            value = latencies.get(source)
            if not is_number(value) or not math.isfinite(float(value)) or value < 0:
                errors.append(
                    f"{label}.retrieval_metadata.source_latency_ms.{source} must be non-negative"
                )

    wall_clock = metadata.get("wall_clock_latency_ms")
    if not is_number(wall_clock) or not math.isfinite(float(wall_clock)) or wall_clock < 0:
        errors.append(f"{label}.retrieval_metadata.wall_clock_latency_ms must be non-negative")

    rankings = metadata.get("source_rankings")
    if not isinstance(rankings, dict):
        errors.append(
            f"{label}.retrieval_metadata.source_rankings must map every source to doc_id ranks"
        )
    else:
        for source in REQUIRED_RRF_SOURCES:
            source_ranks = rankings.get(source)
            if not isinstance(source_ranks, dict):
                errors.append(f"{label}.retrieval_metadata.source_rankings.{source} is required")
                continue
            if isinstance(statuses, dict) and statuses.get(source) == "success" and not source_ranks:
                errors.append(
                    f"{label}.retrieval_metadata.source_rankings.{source} must be non-empty when source status is success"
                )
            for doc_id, rank in source_ranks.items():
                if not is_nonempty_string(doc_id) or not isinstance(rank, int) or isinstance(rank, bool) or rank < 1:
                    errors.append(
                        f"{label}.retrieval_metadata.source_rankings.{source} must contain positive integer ranks"
                    )
                    break
    return errors


def validate_result(data: Any) -> List[str]:
    """Return evidence-contract violations without modifying the result file."""
    errors: List[str] = []
    if not isinstance(data, dict):
        return ["root JSON value must be an object"]

    evaluation_mode = data.get("evaluation_mode")
    if evaluation_mode not in VALID_EVALUATION_MODES:
        errors.append(
            "evaluation_mode must be one of "
            f"{sorted(VALID_EVALUATION_MODES)!r}, got {evaluation_mode!r}"
        )

    runtime_config = data.get("runtime_config")
    if not isinstance(runtime_config, dict):
        errors.append("runtime_config must be an object")
    else:
        for field in RUNTIME_CONFIG_FIELDS:
            value = runtime_config.get(field)
            if not is_nonempty_string(value) or value.strip().lower() in UNCAPTURED_VALUES:
                errors.append(
                    f"runtime_config.{field} must be captured as a non-empty value"
                )

    dataset = data.get("dataset")
    dataset_hash = dataset.get("ground_truth_sha256") if isinstance(dataset, dict) else None
    if (
        not is_nonempty_string(dataset_hash)
        or len(dataset_hash) != 64
        or any(character not in "0123456789abcdefABCDEF" for character in dataset_hash)
    ):
        errors.append("dataset.ground_truth_sha256 must be a 64-character SHA-256 hex string")
    corpus_hash = dataset.get("corpus_sha256") if isinstance(dataset, dict) else None
    if corpus_hash not in (None, "not_captured") and (
        not is_nonempty_string(corpus_hash) or len(corpus_hash) != 64
    ):
        errors.append("dataset.corpus_sha256 must be a 64-character SHA-256 hex string when captured")

    if not isinstance(dataset, dict) or dataset.get("report_eligible") is not True:
        errors.append("dataset.report_eligible must be true for formal retrieval evidence")

    detailed_results = data.get("detailed_results")
    if not isinstance(detailed_results, list) or not detailed_results:
        errors.append("detailed_results must be a non-empty array")
        return errors

    expected_counts: Dict[str, Dict[str, int]] = defaultdict(
        lambda: {"scorable": 0, "unscorable": 0}
    )
    for index, result in enumerate(detailed_results):
        label = f"detailed_results[{index}]"
        if not isinstance(result, dict):
            errors.append(f"{label} must be an object")
            continue

        config_name = result.get("config_name")
        if not is_nonempty_string(config_name):
            errors.append(f"{label}.config_name must be a non-empty string")
            continue

        doc_ids = result.get("retrieved_docs")
        if not isinstance(doc_ids, list) or not doc_ids:
            errors.append(f"{label}.retrieved_docs must contain at least one doc_id")
        else:
            normalized_doc_ids = []
            for doc_index, doc_id in enumerate(doc_ids):
                if not is_nonempty_string(doc_id):
                    errors.append(
                        f"{label}.retrieved_docs[{doc_index}] must be a non-empty doc_id"
                    )
                else:
                    normalized_doc_ids.append(doc_id.strip())
            if len(normalized_doc_ids) != len(set(normalized_doc_ids)):
                errors.append(f"{label}.retrieved_docs contains duplicate doc_id values")

        recall = result.get("recall_at_5")
        if not is_number(recall) or not math.isfinite(recall):
            errors.append(f"{label}.recall_at_5 must be a finite number")
        elif recall > 1:
            errors.append(f"{label}.recall_at_5 must not exceed 1 (got {recall})")

        is_scorable = result.get("is_scorable")
        if not isinstance(is_scorable, bool):
            errors.append(f"{label}.is_scorable must be a boolean")
            continue

        if result.get("error_type") == "None":
            count_key = "scorable" if is_scorable else "unscorable"
            expected_counts[config_name][count_key] += 1

        if evaluation_mode == "multi_source_rrf_evaluation_only":
            errors.extend(_validate_rrf_audit(label, result))

    metrics = data.get("aggregated_metrics")
    if not isinstance(metrics, dict) or not metrics:
        errors.append("aggregated_metrics must be a non-empty object")
        return errors

    expected_config_names = set(expected_counts)
    metric_config_names = set(metrics)
    missing_metrics = expected_config_names - metric_config_names
    extra_metrics = metric_config_names - expected_config_names
    for config_name in sorted(missing_metrics):
        errors.append(f"aggregated_metrics is missing config {config_name!r}")
    for config_name in sorted(extra_metrics):
        errors.append(f"aggregated_metrics has no detailed results for config {config_name!r}")

    for config_name, expected in expected_counts.items():
        metric = metrics.get(config_name)
        if not isinstance(metric, dict):
            continue
        for field, expected_value in (
            ("scorable_count", expected["scorable"]),
            ("unscorable_count", expected["unscorable"]),
        ):
            actual_value = metric.get(field)
            if not isinstance(actual_value, int) or isinstance(actual_value, bool):
                errors.append(f"aggregated_metrics.{config_name}.{field} must be an integer")
            elif actual_value != expected_value:
                errors.append(
                    f"aggregated_metrics.{config_name}.{field} is {actual_value}, "
                    f"expected {expected_value} from detailed_results"
                )

        recall = metric.get("recall_at_5")
        if not is_number(recall) or not math.isfinite(recall):
            errors.append(
                f"aggregated_metrics.{config_name}.recall_at_5 must be a finite number"
            )
        elif recall > 1:
            errors.append(
                f"aggregated_metrics.{config_name}.recall_at_5 must not exceed 1 "
                f"(got {recall})"
            )
        for latency_field in ("p50_latency_ms", "p95_latency_ms"):
            latency = metric.get(latency_field)
            if latency is not None and (not is_number(latency) or latency < 0):
                errors.append(f"aggregated_metrics.{config_name}.{latency_field} must be a non-negative number")

    return errors


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Read-only validation of a retrieval evaluation result JSON file."
    )
    parser.add_argument("result_json", type=Path, help="path to a retrieval result JSON file")
    args = parser.parse_args()

    try:
        with args.result_json.open("r", encoding="utf-8") as result_file:
            data = json.load(result_file)
    except (OSError, json.JSONDecodeError) as error:
        print(f"FAIL: could not read {args.result_json}: {error}")
        return 1

    errors = validate_result(data)
    if errors:
        print(f"FAIL: {args.result_json}")
        for error in errors:
            print(f"  - {error}")
        return 1

    print(
        f"PASS: {args.result_json} "
        f"({len(data['detailed_results'])} detailed result(s), "
        f"{len(data['aggregated_metrics'])} configuration(s))"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
