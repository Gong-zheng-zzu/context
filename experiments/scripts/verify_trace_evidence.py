#!/usr/bin/env python3
"""Validate that a retrieval trace has usable source and doc_id evidence."""

import argparse
import json
import sys
from pathlib import Path


def validate(trace: dict) -> list[str]:
    errors: list[str] = []
    if trace.get("mode") != "rrf":
        errors.append("trace mode must be 'rrf'")
    results = trace.get("results")
    if not isinstance(results, list) or len(results) != 10:
        return errors + ["trace must contain exactly 10 result records"]

    for index, result in enumerate(results):
        label = f"results[{index}]"
        if not result.get("success"):
            errors.append(f"{label} failed: {result.get('error', 'unknown error')}")
            continue
        if not result.get("valid_doc_ids"):
            errors.append(f"{label} contains a missing doc_id")
        if not result.get("unique_doc_ids"):
            errors.append(f"{label} contains duplicate doc_ids")
        evidence = result.get("evidence")
        if not isinstance(evidence, dict):
            errors.append(f"{label} is missing retrieval evidence")
            continue
        active = evidence.get("active_sources")
        empty = evidence.get("empty_sources")
        mode = evidence.get("fusion_mode")
        if not isinstance(active, list) or not isinstance(empty, list):
            errors.append(f"{label} must record active and empty sources")
            continue
        if set(active) | set(empty) != {"vector", "knowledge", "timeline"}:
            errors.append(f"{label} has incomplete source accounting: active={active}, empty={empty}")
        if len(active) == 1 and mode != f"{active[0]}_only_fallback":
            errors.append(f"{label} mislabels a single-source result as {mode!r}")
        if len(active) > 1 and mode != f"rrf_{len(active)}_sources":
            errors.append(f"{label} has inconsistent RRF mode {mode!r}")
        if not active and mode != "no_evidence":
            errors.append(f"{label} has inconsistent empty-source mode {mode!r}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trace_json", type=Path)
    args = parser.parse_args()
    try:
        trace = json.loads(args.trace_json.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        print(f"FAIL: cannot read trace: {error}")
        return 1
    errors = validate(trace)
    if errors:
        print(f"FAIL: {args.trace_json}")
        for error in errors:
            print(f"  - {error}")
        return 1
    print(f"PASS: {args.trace_json} has 10 evidence-backed RRF trace results")
    return 0


if __name__ == "__main__":
    sys.exit(main())
