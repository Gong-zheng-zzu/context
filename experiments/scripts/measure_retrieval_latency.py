#!/usr/bin/env python3
"""Measure retrieval-only latency with warm-up requests and a P95 gate."""

from __future__ import annotations

import argparse
import json
import math
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, List

sys.path.insert(0, str(Path(__file__).resolve().parent))
from base_evaluator import BaseEvaluator  # noqa: E402


def percentile(values: List[float], value: float) -> float:
    ordered = sorted(values)
    if not ordered:
        return 0.0
    index = (len(ordered) - 1) * value / 100
    lower, upper = math.floor(index), math.ceil(index)
    return ordered[lower] if lower == upper else ordered[lower] + (ordered[upper] - ordered[lower]) * (index - lower)


def load_queries(path: Path, count: int) -> List[dict[str, Any]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    queries = data.get("queries", data) if isinstance(data, dict) else data
    if not isinstance(queries, list) or len(queries) < count:
        raise ValueError(f"{path} does not contain {count} queries")
    return queries[:count]


def request(evaluator: BaseEvaluator, query: str, session_id: str, mode: str):
    payload = {"sessionId": session_id, "query": query, "maxResults": 5}
    payload["evaluationRetrievalOnly" if mode == "rrf" else "contextsOnly"] = True
    return evaluator.call_api("POST", "/api/mcp/tools/retrieve_context", payload, {"Content-Type": "application/json", **evaluator.get_auth_headers()}, timeout=20)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--queries-file", type=Path, default=Path(__file__).resolve().parents[1] / "datasets" / "retrieval_groundtruth" / "query_answer_pairs.json")
    parser.add_argument("--count", type=int, default=10)
    parser.add_argument("--warmup", type=int, default=3)
    parser.add_argument("--session-id", default="eval_retrieval_test")
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--mode", choices=("vector", "rrf"), default="rrf")
    parser.add_argument("--max-p95-ms", type=float, default=1000.0)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    if args.count < 1 or args.warmup < 0:
        parser.error("--count must be positive and --warmup must not be negative")
    queries = load_queries(args.queries_file, max(args.count, args.warmup))
    evaluator = BaseEvaluator(args.base_url)
    healthy, message = evaluator.check_service_health()
    if not healthy:
        print(f"FAIL: {message}")
        return 1
    auth = evaluator.authenticate_from_env()
    if not auth.is_valid_business_response():
        print(f"FAIL: authentication: {auth.error_message}")
        return 1
    for query in queries[:args.warmup]:
        response = request(evaluator, query["question"], args.session_id, args.mode)
        if not response.is_valid_business_response():
            print(f"FAIL: warmup query {query.get('query_id')} failed: {response.error_message}")
            return 1
    measured = []
    for query in queries[:args.count]:
        response = request(evaluator, query["question"], args.session_id, args.mode)
        if not response.is_valid_business_response():
            print(f"FAIL: measured query {query.get('query_id')} failed: {response.error_message}")
            return 1
        measured.append({
            "query_id": query.get("query_id"),
            "latency_ms": response.latency_ms,
            "retrieval_metadata": response.data.get("retrieval_metadata", {}),
        })
    latencies = [item["latency_ms"] for item in measured]
    report = {
        "timestamp_utc": datetime.now(timezone.utc).isoformat(),
        "mode": args.mode,
        "session_id": args.session_id,
        "warmup_count": args.warmup,
        "measured_count": len(measured),
        "latency_ms": {"p50": percentile(latencies, 50), "p95": percentile(latencies, 95), "max": max(latencies), "mean": sum(latencies) / len(latencies)},
        "p95_gate_ms": args.max_p95_ms,
        "passed": percentile(latencies, 95) < args.max_p95_ms,
        "samples": measured,
    }
    output = args.output or Path(__file__).resolve().parents[1] / "results" / "latency" / f"retrieval_latency_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%SZ')}.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"latency_report={output}")
    print(f"P50={report['latency_ms']['p50']:.1f}ms P95={report['latency_ms']['p95']:.1f}ms gate={args.max_p95_ms:.1f}ms")
    return 0 if report["passed"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
