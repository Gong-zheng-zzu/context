#!/usr/bin/env python3
"""Shared, dependency-free integrity helpers for the QLoRA pipeline."""
from __future__ import annotations

import hashlib
import json
import math
from pathlib import Path
from typing import Any, Iterable

from annotation_review import validate_review_manifest
from prepare_causal_dataset import SPLIT_COUNTS, validate_dataset

LABELS = ("object", "mediator", "property", "result")
FINAL_REVIEW_STATES = {"reviewed_agreed", "reviewed_arbitrated"}
PIPELINE_SCHEMA = "contextkeeper-qlora-pipeline-v1"


def canonical_bytes(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def value_sha256(value: Any) -> str:
    return hashlib.sha256(canonical_bytes(value)).hexdigest()


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def read_jsonl(path: Path) -> list[dict[str, Any]]:
    rows = []
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        value = json.loads(line)
        if not isinstance(value, dict):
            raise ValueError(f"{path}:{line_number} must contain a JSON object")
        rows.append(value)
    return rows


def write_json_atomic(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    temporary.replace(path)


def write_jsonl(path: Path, rows: Iterable[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + ".tmp")
    with temporary.open("w", encoding="utf-8", newline="\n") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=False, sort_keys=True) + "\n")
    temporary.replace(path)


def load_simple_yaml(path: Path) -> dict[str, Any]:
    """Load the flat scalar-only YAML used by qlora_config without PyYAML."""
    result: dict[str, Any] = {}
    for line_number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if ":" not in line:
            raise ValueError(f"{path}:{line_number}: expected key: value")
        key, raw_value = (part.strip() for part in line.split(":", 1))
        if not key or not raw_value:
            raise ValueError(f"{path}:{line_number}: empty key or value")
        lowered = raw_value.lower()
        if lowered in {"true", "false"}:
            value: Any = lowered == "true"
        elif raw_value.startswith(('"', "'")):
            value = raw_value.strip('"\'')
        else:
            try:
                value = float(raw_value) if any(mark in raw_value for mark in (".", "e", "E")) else int(raw_value)
            except ValueError:
                value = raw_value
        result[key] = value
    return result


def validate_qlora_config(config: dict[str, Any]) -> dict[str, Any]:
    required = {
        "base_model": "Qwen/Qwen2.5-3B-Instruct",
        "use_4bit": True,
        "quantization": "nf4",
        "compute_dtype": "float16",
        "gradient_checkpointing": True,
        "per_device_train_batch_size": 1,
        "gradient_accumulation_steps": 16,
        "lora_r": 16,
        "lora_alpha": 32,
        "lora_dropout": 0.05,
    }
    mismatches = {key: {"required": expected, "actual": config.get(key)} for key, expected in required.items() if config.get(key) != expected}
    if mismatches:
        raise ValueError(f"QLoRA configuration violates the RTX 2050 bounded profile: {mismatches}")
    if not 128 <= int(config.get("max_seq_length", 0)) <= 768:
        raise ValueError("max_seq_length must be between 128 and 768 for the 4 GB profile")
    return {"valid": True, "config_sha256": value_sha256(config), "profile": "rtx2050_4gb_qwen25_3b_nf4"}


def final_review_labels(review_manifest: dict[str, Any]) -> dict[str, list[dict[str, Any]]]:
    independent: dict[str, list[dict[str, Any]]] = {}
    final: dict[str, list[dict[str, Any]]] = {}
    for event in review_manifest.get("events", []):
        record_id = event["record_id"]
        if event["action"] == "independent_review":
            independent.setdefault(record_id, []).append(event)
            reviews = independent[record_id]
            if len(reviews) == 2 and reviews[0]["labels_sha256"] == reviews[1]["labels_sha256"]:
                final[record_id] = reviews[1]["relations"]
        elif event["action"] == "arbitration":
            final[record_id] = event["relations"]
    return final


def validate_relation_evidence(text: str, relations: Any, record_id: str) -> None:
    if not isinstance(relations, list):
        raise ValueError(f"{record_id}: reviewed relations must be a list")
    for relation_index, relation in enumerate(relations):
        if not isinstance(relation, dict):
            raise ValueError(f"{record_id}: relation {relation_index} must be an object")
        spans = relation.get("spans")
        if not isinstance(spans, dict):
            raise ValueError(f"{record_id}: relation {relation_index} requires O-M-P-R evidence spans")
        for label in LABELS:
            value = relation.get(label)
            span = spans.get(label)
            if not isinstance(value, str) or not value.strip():
                raise ValueError(f"{record_id}: relation {relation_index} has invalid {label}")
            if not isinstance(span, dict) or not isinstance(span.get("start"), int) or not isinstance(span.get("end"), int):
                raise ValueError(f"{record_id}: relation {relation_index} has invalid {label} span")
            start, end = span["start"], span["end"]
            if start < 0 or end <= start or end > len(text) or text[start:end] != value:
                raise ValueError(f"{record_id}: relation {relation_index} {label} span does not match source text")


def validate_fully_reviewed_dataset(dataset_dir: Path, review_manifest_path: Path) -> dict[str, Any]:
    dataset = validate_dataset(dataset_dir)
    review = validate_review_manifest(dataset_dir, review_manifest_path)
    if not review["test_freeze_eligible"]:
        raise ValueError("frozen-test gate failed: every test record requires completed independent review")
    manifest = json.loads((dataset_dir / "manifest.json").read_text(encoding="utf-8"))
    review_manifest = json.loads(review_manifest_path.read_text(encoding="utf-8"))
    labels = final_review_labels(review_manifest)
    missing: dict[str, list[str]] = {}
    total = 0
    for split in SPLIT_COUNTS:
        rows = read_jsonl(dataset_dir / manifest["files"][split]["path"])
        total += len(rows)
        missing[split] = [row["record_id"] for row in rows if row["record_id"] not in labels]
        for row in rows:
            if row["record_id"] in labels:
                validate_relation_evidence(row["text"], labels[row["record_id"]], row["record_id"])
    missing = {split: ids for split, ids in missing.items() if ids}
    if missing:
        summary = {split: len(ids) for split, ids in missing.items()}
        raise ValueError(f"training gate failed: unreviewed records remain by split: {summary}")
    if len(labels) != total:
        raise ValueError("review manifest contains an unexpected reviewed-record count")
    return {
        "valid": True,
        "dataset_sha256": dataset["dataset_sha256"],
        "review_chain_head": review["chain_head"],
        "reviewed_records": total,
        "test_freeze_eligible": True,
        "labels": labels,
    }


def percentile(values: list[float], quantile: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * quantile) - 1)]
