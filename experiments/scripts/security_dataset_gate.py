#!/usr/bin/env python3
"""Fail-closed validation for the security evaluation dataset manifest.

The evaluator must never turn a positive-only corpus into a headline
defense-rate result.  A manifest is eligible only when attack, benign, and
ambiguous groups have independent files, hashes, and enough labelled rows.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


REQUIRED_GROUPS = ("attacks", "benign", "ambiguous")


class DatasetGateError(ValueError):
    """Raised when a dataset is not eligible for a formal report."""


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _read_rows(path: Path) -> list[Any]:
    if path.suffix.lower() == ".jsonl":
        return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
    payload = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(payload, list):
        return payload
    if isinstance(payload, dict):
        for key in ("samples", "records", "items", "data"):
            if isinstance(payload.get(key), list):
                return payload[key]
    raise DatasetGateError(f"{path}: expected a JSON array or a list field")


def validate_manifest(manifest_path: Path, minimum_benign: int = 100) -> dict[str, Any]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    groups = manifest.get("groups")
    if not isinstance(groups, dict):
        raise DatasetGateError("manifest.groups is required")

    checked: dict[str, Any] = {}
    errors: list[str] = []
    for group in REQUIRED_GROUPS:
        spec = groups.get(group)
        if not isinstance(spec, dict):
            errors.append(f"missing group: {group}")
            continue
        relative = spec.get("path")
        expected_hash = spec.get("sha256")
        if not isinstance(relative, str) or not relative:
            errors.append(f"{group}: path is required")
            continue
        if not isinstance(expected_hash, str) or len(expected_hash) != 64:
            errors.append(f"{group}: sha256 is required")
            continue
        data_path = (manifest_path.parent / relative).resolve()
        if not data_path.is_file():
            errors.append(f"{group}: file does not exist: {relative}")
            continue
        actual_hash = sha256_file(data_path)
        rows = _read_rows(data_path)
        checked[group] = {"path": relative, "rows": len(rows), "sha256": actual_hash}
        if actual_hash != expected_hash:
            errors.append(f"{group}: sha256 mismatch")
        if group == "benign" and len(rows) < minimum_benign:
            errors.append(f"benign: requires at least {minimum_benign} rows")
        if group != "benign" and not rows:
            errors.append(f"{group}: cannot be empty")

    status = "eligible" if not errors else "ineligible"
    result = {
        "schema_version": manifest.get("schema_version", "security-dataset-gate-v1"),
        "status": status,
        "report_eligible": status == "eligible",
        "manifest": str(manifest_path),
        "groups": checked,
        "errors": errors,
    }
    if errors:
        raise DatasetGateError(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--minimum-benign", type=int, default=100)
    args = parser.parse_args()
    try:
        result = validate_manifest(args.manifest, args.minimum_benign)
    except (OSError, ValueError, json.JSONDecodeError, DatasetGateError) as exc:
        print(str(exc))
        return 2
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
