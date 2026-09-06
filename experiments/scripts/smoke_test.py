#!/usr/bin/env python3
"""Non-destructive JWT smoke test for the four experiment API contracts.

This intentionally tests reachability and response contracts only. Machine
unlearning is destructive and is exercised exclusively by unlearning_eval.py.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import APIResponse, BaseEvaluator


def report(name: str, response: APIResponse) -> bool:
    valid = response.is_valid_business_response()
    state = "PASS" if valid else "FAIL"
    detail = "valid business response" if valid else (response.error_message or "invalid response")
    print(f"[{state}] {name}: HTTP {response.http_status}, {detail}, {response.latency_ms:.0f}ms")
    return valid


def main() -> int:
    parser = argparse.ArgumentParser(description="Run non-destructive authenticated API smoke checks.")
    parser.add_argument("--quick", action="store_true", help="accepted for runner compatibility; all checks are bounded")
    parser.add_argument("--base-url", default="http://localhost:8088")
    args = parser.parse_args()

    evaluator = BaseEvaluator(args.base_url)
    healthy, health_message = evaluator.check_service_health()
    if not healthy:
        print(f"[FAIL] health: {health_message}")
        return 1
    print(f"[PASS] health: {health_message}")

    auth = evaluator.authenticate_from_env()
    if not auth.is_valid_business_response():
        print(f"[FAIL] authentication: {auth.error_message}")
        return 1
    print("[PASS] authentication: JWT acquired from environment credentials")

    headers = evaluator.get_auth_headers()
    checks = {
        "security": evaluator.call_api(
            "POST",
            "/api/v1/security/evaluate-input",
            {"input": "health check", "endpoint_name": "smoke_test"},
            headers,
        ),
        "causal": evaluator.call_api(
            "POST",
            "/api/v1/causal/extract",
            {"text": "Medication omission caused elevated blood pressure.", "use_llm": False, "min_confidence": 0.5},
            headers,
        ),
        "retrieval": evaluator.call_api(
            "POST",
            "/api/mcp/tools/retrieve_context",
            {"sessionId": "eval_retrieval_test", "query": "health record", "maxResults": 1, "contextsOnly": True},
            headers,
        ),
        "unlearning": evaluator.call_api("GET", "/api/v1/unlearning/config", headers=headers),
    }
    passed = [report(name, response) for name, response in checks.items()]
    print(f"Summary: {sum(passed)}/{len(passed)} checks passed; no destructive request was issued.")
    return 0 if all(passed) else 1


if __name__ == "__main__":
    raise SystemExit(main())
