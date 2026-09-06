#!/usr/bin/env python
"""Safely verify and clean the only destructive experiment session.

The script calls the JWT-protected cascade endpoint. It never opens database
connections and refuses all user/session values other than the fixed evaluation
scope, preventing accidental deletion of production data.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from base_evaluator import BaseEvaluator

ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
REQUIRED_STORES = {"qdrant", "timescaledb", "neo4j", "session_file_cache"}


def validate_result(payload: dict[str, Any], expected_status: str) -> list[str]:
    errors: list[str] = []
    result = payload.get("result")
    if not isinstance(result, dict):
        return ["response does not contain a structured result"]
    if result.get("user_id") != ALLOWED_USER_ID or result.get("session_id") != ALLOWED_SESSION_ID:
        return ["server response scope does not match the fixed evaluation scope"]

    by_name = {item.get("store"): item for item in result.get("stores", []) if isinstance(item, dict)}
    missing = REQUIRED_STORES - set(by_name)
    if missing:
        errors.append("missing storage results: " + ", ".join(sorted(missing)))
    for name, item in by_name.items():
        if name not in REQUIRED_STORES:
            continue
        if item.get("status") != expected_status:
            errors.append(f"{name}: status={item.get('status')!r}, expected={expected_status!r}")
        if expected_status == "deleted" and item.get("after") != 0:
            errors.append(f"{name}: post-delete count={item.get('after')!r}, expected 0")
    return errors


def write_report(output: Path, report: dict[str, Any]) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=True, indent=2), encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Clean only the fixed evaluation session through the cascade API.")
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--confirm", action="store_true", help="required before the destructive DELETE request")
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    if not args.confirm:
        parser.error("--confirm is required; this script performs a destructive cleanup in the fixed evaluation scope")

    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%SZ")
    output = args.output or Path(__file__).resolve().parents[1] / "results" / "cleanup" / f"cleanup_{timestamp}.json"
    evaluator = BaseEvaluator(args.base_url)
    auth = evaluator.authenticate_from_env()
    report: dict[str, Any] = {
        "timestamp_utc": datetime.now(timezone.utc).isoformat(),
        "scope": {"user_id": ALLOWED_USER_ID, "session_id": ALLOWED_SESSION_ID},
        "auth": {"http_status": auth.http_status, "error_type": auth.error_type, "error_message": auth.error_message},
        "passed": False,
    }
    if not auth.is_valid_business_response():
        report["failure"] = "authentication failed; no cleanup request was issued"
        write_report(output, report)
        print(f"FAIL: {report['failure']}\nreport: {output}")
        return 1

    endpoint = f"/api/sessions/{ALLOWED_SESSION_ID}"
    headers = evaluator.get_auth_headers()
    dry_run = evaluator.call_api("DELETE", endpoint + "?dry_run=true", headers=headers, timeout=args.timeout)
    report["preflight"] = {"http_status": dry_run.http_status, "data": dry_run.data, "error": dry_run.error_message}

    # A previous bounded unlearning or cleanup may have already removed this
    # exact session. Treat only this fixed-scope 404 as verified-clean rather
    # than issuing a second destructive request.
    if dry_run.http_status == 404:
        report["status"] = "already_clean"
        report["passed"] = True
        report["deletion"] = {"skipped": True, "reason": "evaluation session does not exist"}
        write_report(output, report)
        print(f"PASS: evaluation scope is already clean; report: {output}")
        return 0

    preflight_errors = [] if dry_run.is_valid_business_response() and isinstance(dry_run.data, dict) else [dry_run.error_message or "preflight failed"]
    if not preflight_errors:
        preflight_errors = validate_result(dry_run.data, "dry_run")
    if preflight_errors:
        report["failure"] = "preflight failed: " + "; ".join(preflight_errors)
        write_report(output, report)
        print(f"FAIL: {report['failure']}\nreport: {output}")
        return 1

    deletion = evaluator.call_api("DELETE", endpoint, headers=headers, timeout=args.timeout)
    report["deletion"] = {"http_status": deletion.http_status, "data": deletion.data, "error": deletion.error_message}
    deletion_errors = [] if deletion.is_valid_business_response() and isinstance(deletion.data, dict) else [deletion.error_message or "delete failed"]
    if not deletion_errors:
        if deletion.data.get("complete") is not True:
            deletion_errors.append("server did not mark the cascade complete")
        deletion_errors.extend(validate_result(deletion.data, "deleted"))

    report["passed"] = not deletion_errors
    if deletion_errors:
        report["failure"] = "cleanup failed: " + "; ".join(deletion_errors)
    write_report(output, report)
    print(("PASS" if report["passed"] else "FAIL") + f": report: {output}")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
