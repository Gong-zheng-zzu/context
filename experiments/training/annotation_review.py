#!/usr/bin/env python3
"""Append-only review manifest for the synthetic O-M-P-R dataset.

The review log is anchored to the generated dataset hash and hash-chained. It
does not turn generated labels into ground truth by itself: two independent
reviewers must agree, or a third person must arbitrate a disagreement.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
from typing import Any

from prepare_causal_dataset import SPLIT_COUNTS, validate_dataset

REVIEW_SCHEMA_VERSION = "causal-annotation-review-v1"
ZERO_HASH = "0" * 64
VALID_ACTIONS = {"independent_review", "arbitration"}


def _canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def _hash(value: Any) -> str:
    return hashlib.sha256(_canonical(value)).hexdigest()


def _write_json_atomic(path: Path, value: Any) -> None:
    temporary = path.with_name(path.name + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    os.replace(temporary, path)


def _dataset_records(dataset_dir: Path) -> dict[str, dict[str, Any]]:
    manifest = json.loads((dataset_dir / "manifest.json").read_text(encoding="utf-8"))
    records: dict[str, dict[str, Any]] = {}
    for split in SPLIT_COUNTS:
        path = dataset_dir / manifest["files"][split]["path"]
        for line in path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            row = json.loads(line)
            record_id = row["record_id"]
            if record_id in records:
                raise ValueError(f"duplicate record_id: {record_id}")
            records[record_id] = row
    return records


def create_review_manifest(dataset_dir: Path, output: Path) -> dict[str, Any]:
    if output.exists():
        raise FileExistsError(f"review manifest already exists: {output}")
    validation = validate_dataset(dataset_dir)
    dataset_manifest = json.loads((dataset_dir / "manifest.json").read_text(encoding="utf-8"))
    manifest = {
        "schema_version": REVIEW_SCHEMA_VERSION,
        "dataset_schema_version": dataset_manifest["schema_version"],
        "dataset_sha256": validation["dataset_sha256"],
        "split_sha256": {
            split: dataset_manifest["files"][split]["sha256"] for split in SPLIT_COUNTS
        },
        "workflow": {
            "minimum_independent_reviewers": 2,
            "reviewers_must_be_distinct": True,
            "third_party_arbitration_on_disagreement": True,
        },
        "events": [],
        "chain_head": ZERO_HASH,
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    _write_json_atomic(output, manifest)
    return manifest


def _validate_timestamp(timestamp: str) -> None:
    if not timestamp or "T" not in timestamp or not (timestamp.endswith("Z") or "+" in timestamp[10:]):
        raise ValueError("timestamp must be an explicit ISO-8601 value with timezone")


def append_review_event(
    dataset_dir: Path,
    manifest_path: Path,
    *,
    record_id: str,
    actor_id: str,
    action: str,
    relations: list[dict[str, str]],
    timestamp: str,
) -> dict[str, Any]:
    if action not in VALID_ACTIONS:
        raise ValueError(f"unsupported review action: {action}")
    if not actor_id.strip():
        raise ValueError("actor_id is required")
    _validate_timestamp(timestamp)
    records = _dataset_records(dataset_dir)
    if record_id not in records:
        raise ValueError(f"unknown record_id: {record_id}")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    state = validate_review_manifest(dataset_dir, manifest_path)
    record_state = state["records"].get(record_id, {"reviewers": [], "status": "synthetic_unreviewed"})
    reviewers = set(record_state["reviewers"])
    if action == "independent_review":
        if actor_id in reviewers:
            raise ValueError("the same reviewer cannot review one record twice")
        if len(reviewers) >= 2:
            raise ValueError("two independent reviews already exist")
    else:
        if record_state["status"] != "needs_arbitration":
            raise ValueError("arbitration is allowed only after two reviewers disagree")
        if actor_id in reviewers:
            raise ValueError("the arbitrator must be independent of both reviewers")

    event = {
        "sequence": len(manifest["events"]) + 1,
        "previous_hash": manifest["chain_head"],
        "record_id": record_id,
        "actor_id": actor_id,
        "action": action,
        "timestamp": timestamp,
        "relations": relations,
        "labels_sha256": _hash(relations),
    }
    event["event_hash"] = _hash(event)
    manifest["events"].append(event)
    manifest["chain_head"] = event["event_hash"]
    _write_json_atomic(manifest_path, manifest)
    return event


def validate_review_manifest(dataset_dir: Path, manifest_path: Path) -> dict[str, Any]:
    dataset = validate_dataset(dataset_dir)
    records = _dataset_records(dataset_dir)
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("schema_version") != REVIEW_SCHEMA_VERSION:
        raise ValueError("unsupported review manifest schema")
    if manifest.get("dataset_sha256") != dataset["dataset_sha256"]:
        raise ValueError("review manifest is not anchored to this dataset")
    dataset_manifest = json.loads((dataset_dir / "manifest.json").read_text(encoding="utf-8"))
    expected_split_hashes = {split: dataset_manifest["files"][split]["sha256"] for split in SPLIT_COUNTS}
    if manifest.get("split_sha256") != expected_split_hashes:
        raise ValueError("review manifest split hashes do not match the dataset")
    expected_workflow = {
        "minimum_independent_reviewers": 2,
        "reviewers_must_be_distinct": True,
        "third_party_arbitration_on_disagreement": True,
    }
    if manifest.get("workflow") != expected_workflow:
        raise ValueError("review workflow policy was modified")

    previous_hash = ZERO_HASH
    states: dict[str, dict[str, Any]] = {}
    for expected_sequence, stored in enumerate(manifest.get("events", []), start=1):
        event = dict(stored)
        event_hash = event.pop("event_hash", None)
        if event.get("sequence") != expected_sequence or event.get("previous_hash") != previous_hash:
            raise ValueError("review event sequence or previous hash is invalid")
        if event_hash != _hash(event):
            raise ValueError("review event hash mismatch")
        record_id = event.get("record_id")
        if record_id not in records:
            raise ValueError(f"review event references unknown record: {record_id}")
        if event.get("action") not in VALID_ACTIONS or event.get("labels_sha256") != _hash(event.get("relations")):
            raise ValueError("invalid review event payload")
        if not isinstance(event.get("relations"), list) or not event.get("actor_id", "").strip():
            raise ValueError("review event relations and actor_id are required")
        _validate_timestamp(event.get("timestamp", ""))
        state = states.setdefault(
            record_id,
            {"reviewers": [], "review_hashes": [], "status": "synthetic_unreviewed", "final_labels_sha256": None},
        )
        actor = event.get("actor_id", "")
        if event["action"] == "independent_review":
            if actor in state["reviewers"] or len(state["reviewers"]) >= 2:
                raise ValueError("reviewer independence violated")
            state["reviewers"].append(actor)
            state["review_hashes"].append(event["labels_sha256"])
            if len(state["reviewers"]) == 1:
                state["status"] = "review_in_progress"
            elif len(set(state["review_hashes"])) == 1:
                state["status"] = "reviewed_agreed"
                state["final_labels_sha256"] = event["labels_sha256"]
            else:
                state["status"] = "needs_arbitration"
        else:
            if state["status"] != "needs_arbitration" or actor in state["reviewers"]:
                raise ValueError("invalid arbitration event")
            state["status"] = "reviewed_arbitrated"
            state["arbitrator"] = actor
            state["final_labels_sha256"] = event["labels_sha256"]
        previous_hash = event_hash

    if manifest.get("chain_head") != previous_hash:
        raise ValueError("review manifest chain_head mismatch")
    frozen_ids = {record_id for record_id, row in records.items() if row["split"] == "test"}
    finalized = {record_id for record_id, state in states.items() if state["status"] in {"reviewed_agreed", "reviewed_arbitrated"}}
    return {
        "valid": True,
        "records": states,
        "reviewed_records": len(finalized),
        "test_records": len(frozen_ids),
        "test_freeze_eligible": frozen_ids <= finalized,
        "chain_head": previous_hash,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    init = subparsers.add_parser("init")
    init.add_argument("--dataset", type=Path, required=True)
    init.add_argument("--manifest", type=Path, required=True)
    check = subparsers.add_parser("check")
    check.add_argument("--dataset", type=Path, required=True)
    check.add_argument("--manifest", type=Path, required=True)
    record = subparsers.add_parser("record")
    record.add_argument("--dataset", type=Path, required=True)
    record.add_argument("--manifest", type=Path, required=True)
    record.add_argument("--record-id", required=True)
    record.add_argument("--actor-id", required=True)
    record.add_argument("--action", choices=sorted(VALID_ACTIONS), required=True)
    record.add_argument("--relations", type=Path, required=True, help="JSON file containing the reviewed relation list")
    record.add_argument("--timestamp", required=True, help="explicit ISO-8601 timestamp with timezone")
    args = parser.parse_args()
    if args.command == "init":
        result = create_review_manifest(args.dataset, args.manifest)
    elif args.command == "check":
        result = validate_review_manifest(args.dataset, args.manifest)
    else:
        relations = json.loads(args.relations.read_text(encoding="utf-8"))
        result = append_review_event(
            args.dataset,
            args.manifest,
            record_id=args.record_id,
            actor_id=args.actor_id,
            action=args.action,
            relations=relations,
            timestamp=args.timestamp,
        )
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
