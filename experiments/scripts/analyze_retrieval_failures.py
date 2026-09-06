#!/usr/bin/env python3
"""Summarize missed retrieval queries without changing raw evaluation output."""

from __future__ import annotations

import argparse
import json
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any, Dict, List


def failure_category(result: Dict[str, Any]) -> str:
    text = str(result.get("query_text", ""))
    query_type = result.get("query_type", "general")
    if query_type == "temporal" or any(token in text for token in ("最近", "前后", "趋势", "日期", "时间")):
        return "time_expression"
    if query_type == "causal" or any(token in text for token in ("原因", "因素", "导致", "为什么")):
        return "cross_event_causality"
    if any(token in text for token in ("哪些", "所有", "清单", "频率")):
        return "multi_patient_or_aggregation"
    return "patient_event_or_synonym"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("result_json", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    data = json.loads(args.result_json.read_text(encoding="utf-8"))
    results: List[Dict[str, Any]] = data.get("detailed_results", [])
    failures = [item for item in results if item.get("error_type") == "None" and item.get("is_scorable") and not item.get("found_ground_truth")]
    categories = Counter(failure_category(item) for item in failures)
    by_type = defaultdict(lambda: {"total": 0, "misses": 0})
    for item in results:
        if item.get("error_type") != "None" or not item.get("is_scorable"):
            continue
        bucket = by_type[item.get("query_type", "unknown")]
        bucket["total"] += 1
        bucket["misses"] += int(not item.get("found_ground_truth"))
    report = {
        "source_result": str(args.result_json),
        "total_scorable": sum(item["total"] for item in by_type.values()),
        "miss_count": len(failures),
        "failure_categories": dict(categories),
        "by_query_type": {name: {**value, "miss_rate": value["misses"] / value["total"] if value["total"] else None} for name, value in by_type.items()},
        "misses": [{key: item.get(key) for key in ("query_id", "query_type", "query_text", "retrieved_docs", "latency_ms")} | {"failure_category": failure_category(item)} for item in failures],
        "note": "Categories are deterministic triage labels. Confirm root causes against returned documents and retrieval trace evidence before changing ranking logic.",
    }
    output = args.output or args.result_json.with_name(args.result_json.stem + "_failure_analysis.json")
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"failure_analysis={output}")
    print(f"misses={len(failures)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
