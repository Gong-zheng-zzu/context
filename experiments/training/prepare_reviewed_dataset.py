#!/usr/bin/env python3
"""Materialize reviewed train/validation/frozen-test data after integrity gates."""
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from pipeline_common import (
    PIPELINE_SCHEMA,
    file_sha256,
    read_jsonl,
    validate_fully_reviewed_dataset,
    value_sha256,
    write_json_atomic,
    write_jsonl,
)


def prepare(dataset_dir: Path, review_manifest_path: Path, output_dir: Path) -> dict[str, Any]:
    if output_dir.exists() and any(output_dir.iterdir()):
        raise FileExistsError(f"output must be empty to preserve provenance: {output_dir}")
    gate = validate_fully_reviewed_dataset(dataset_dir, review_manifest_path)
    source_manifest = json.loads((dataset_dir / "manifest.json").read_text(encoding="utf-8"))
    output_files: dict[str, Any] = {}
    for split in ("train", "validation", "test"):
        source = read_jsonl(dataset_dir / source_manifest["files"][split]["path"])
        rows = []
        for row in source:
            rows.append(
                {
                    "record_id": row["record_id"],
                    "text": row["text"],
                    "relations": gate["labels"][row["record_id"]],
                    "split": "frozen_test" if split == "test" else split,
                    "synthetic": True,
                    "annotation_status": "reviewed_final",
                    "source_dataset_sha256": gate["dataset_sha256"],
                    "review_chain_head": gate["review_chain_head"],
                }
            )
        name = "frozen_test.jsonl" if split == "test" else f"{split}.jsonl"
        destination = output_dir / name
        write_jsonl(destination, rows)
        output_files[split] = {"path": name, "records": len(rows), "sha256": file_sha256(destination)}
    manifest = {
        "schema_version": PIPELINE_SCHEMA,
        "artifact_type": "reviewed_causal_dataset",
        "source_dataset_sha256": gate["dataset_sha256"],
        "review_chain_head": gate["review_chain_head"],
        "test_freeze_eligible": True,
        "reviewed_records": gate["reviewed_records"],
        "files": output_files,
    }
    manifest["artifact_sha256"] = value_sha256(manifest)
    write_json_atomic(output_dir / "manifest.json", manifest)
    return manifest


def validate_prepared_dataset(output_dir: Path) -> dict[str, Any]:
    manifest_path = output_dir / "manifest.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    stored_hash = manifest.pop("artifact_sha256", None)
    if manifest.get("schema_version") != PIPELINE_SCHEMA or manifest.get("artifact_type") != "reviewed_causal_dataset":
        raise ValueError("unsupported reviewed dataset manifest")
    if manifest.get("test_freeze_eligible") is not True:
        raise ValueError("reviewed dataset is not frozen-test eligible")
    if stored_hash != value_sha256(manifest):
        raise ValueError("reviewed dataset manifest hash mismatch")
    for split in ("train", "validation", "test"):
        metadata = manifest["files"][split]
        path = output_dir / metadata["path"]
        if file_sha256(path) != metadata["sha256"]:
            raise ValueError(f"prepared {split} sha256 mismatch")
        rows = read_jsonl(path)
        if len(rows) != metadata["records"] or any(row.get("annotation_status") != "reviewed_final" for row in rows):
            raise ValueError(f"prepared {split} content is invalid")
    manifest["artifact_sha256"] = stored_hash
    return manifest


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path)
    parser.add_argument("--review-manifest", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if args.check:
        result = validate_prepared_dataset(args.output)
    else:
        if args.dataset is None or args.review_manifest is None:
            parser.error("--dataset and --review-manifest are required unless --check is used")
        result = prepare(args.dataset, args.review_manifest, args.output)
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
