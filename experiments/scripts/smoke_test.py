#!/usr/bin/env python3
"""Non-destructive JWT smoke test for the four experiment API contracts.

This intentionally tests reachability and response contracts only. Machine
unlearning is destructive and is exercised exclusively by unlearning_eval.py.
"""

from __future__ import annotations

import argparse
import json
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


def validate_contract(name: str, response: APIResponse) -> tuple[bool, str]:
    """Validate the minimum auditable shape for each protected endpoint.

    A HTTP 200 response alone is not enough for a competition smoke test: an
    empty retrieval response must still carry an explicit fallback/audit
    status, and causal output must identify how it was produced.
    """
    if not response.is_valid_business_response():
        return False, response.error_message or "invalid HTTP/business response"
    payload = response.data
    if not isinstance(payload, dict):
        return False, "response body must be a JSON object"

    if name == "causal":
        relations = payload.get("relations")
        execution = payload.get("execution")
        persistence = payload.get("persistence")
        if not isinstance(relations, list) or not isinstance(execution, dict):
            return False, "missing causal relations/execution audit fields"
        if not isinstance(persistence, dict):
            return False, "missing causal persistence audit field"
        if not execution.get("mode") or not execution.get("model_status"):
            return False, "causal execution mode/model_status is missing"
    elif name == "retrieval":
        contexts = payload.get("contexts")
        metadata = payload.get("retrieval_metadata")
        if not isinstance(contexts, list) or not isinstance(metadata, dict):
            return False, "missing contexts/retrieval_metadata"
        required = ("retrieval_fusion_mode", "source_statuses", "source_latency_ms", "wall_clock_latency_ms")
        missing = [key for key in required if key not in metadata]
        if missing:
            return False, "retrieval audit fields missing: " + ", ".join(missing)
        doc_ids = [item.get("doc_id") for item in contexts if isinstance(item, dict)]
        if any(not isinstance(doc_id, str) or not doc_id.strip() for doc_id in doc_ids):
            return False, "retrieval context is missing a non-empty doc_id"
        if len(doc_ids) != len(set(doc_ids)):
            return False, "retrieval contexts contain duplicate doc_id values"
    elif name == "security":
        # The endpoint intentionally returns metadata only.  Require one of
        # the stable decision fields without accepting raw input echoes.
        security_payload = payload.get("data", payload)
        if not isinstance(security_payload, dict) or not any(key in security_payload for key in ("blocked", "risk_level", "decision", "is_safe")):
            return False, "security response has no decision field"
    elif name == "unlearning":
        if not any(key in payload for key in ("enabled", "config", "mode", "status")):
            return False, "unlearning config response has no configuration field"
    return True, "contract valid"


def fixture_smoke() -> int:
    """Run offline-only checks without contacting HTTP services or credentials."""
    console = Path(__file__).resolve().parents[1] / ".." / "web" / "competition_console.html"
    try:
        html = console.resolve().read_text(encoding="utf-8")
    except OSError as exc:
        print(f"[FAIL] fixture: cannot read competition console: {exc}")
        return 1
    required = ("offline_fixture", "不能模拟 RRF 证据", "固定离线夹具", "persist")
    missing = [item for item in required if item not in html]
    if missing:
        print(f"[FAIL] fixture: console contract missing {', '.join(missing)}")
        return 1
    print("[PASS] fixture: offline console contract valid; no service, credentials, model, or deletion used")
    return 0


def write_json_report(path: Path | None, payload: dict) -> None:
    """Write a redacted smoke report, including failed early exits."""
    if path is None:
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Run non-destructive authenticated API smoke checks.")
    parser.add_argument("--quick", action="store_true", help="accepted for runner compatibility; all checks are bounded")
    parser.add_argument("--base-url", default="http://localhost:8088")
    parser.add_argument("--mode", choices=("live", "fixture"), default="live", help="live protected API checks or offline console contract checks")
    parser.add_argument("--json-output", type=Path, help="optionally write a redacted machine-readable smoke report")
    args = parser.parse_args()

    if args.mode == "fixture":
        result = fixture_smoke()
        write_json_report(args.json_output, {"mode": "fixture", "passed": result == 0})
        return result

    evaluator = BaseEvaluator(args.base_url)
    healthy, health_message = evaluator.check_service_health()
    if not healthy:
        print(f"[FAIL] health: {health_message}")
        write_json_report(args.json_output, {"mode": "live", "base_url": args.base_url, "passed": False, "failure": "health", "message": health_message, "checks": []})
        return 1
    print(f"[PASS] health: {health_message}")

    auth = evaluator.authenticate_from_env()
    if not auth.is_valid_business_response():
        print(f"[FAIL] authentication: {auth.error_message}")
        write_json_report(args.json_output, {"mode": "live", "base_url": args.base_url, "passed": False, "failure": "authentication", "error_type": auth.error_type, "message": auth.error_message, "checks": []})
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
            {
                "text": "Medication omission caused elevated blood pressure.",
                "use_rules": True,
                "use_pmi": False,
                "use_llm": False,
                "min_confidence": 0.5,
            },
            headers,
        ),
        "retrieval": evaluator.call_api(
            "POST",
            "/api/mcp/tools/retrieve_context",
            {
                "sessionId": "eval_retrieval_test",
                "query": "health record",
                "maxResults": 1,
                "contextsOnly": True,
                "evaluationRetrievalOnly": True,
            },
            headers,
        ),
        "unlearning": evaluator.call_api("GET", "/api/v1/unlearning/config", headers=headers),
    }
    passed = []
    report_rows = []
    for name, response in checks.items():
        basic = report(name, response)
        contract, detail = validate_contract(name, response)
        if basic and not contract:
            print(f"[FAIL] {name} contract: {detail}")
        passed.append(basic and contract)
        report_rows.append({"name": name, "passed": basic and contract, "http_status": response.http_status, "latency_ms": response.latency_ms, "error_type": response.error_type, "error": response.error_message})
    print(f"Summary: {sum(passed)}/{len(passed)} checks passed; no destructive request was issued.")
    if args.json_output:
        write_json_report(args.json_output, {"mode": "live", "base_url": args.base_url, "passed": all(passed), "checks": report_rows})
    return 0 if all(passed) else 1


if __name__ == "__main__":
    raise SystemExit(main())
