#!/usr/bin/env python3
"""Seed the bounded target/control sessions for one unlearning evaluation trial.

The tool clears only the bounded target session before writing the fixed
30-record corpus into eval_retrieval_test and eval_unlearning_control.
The server must run with EXPERIMENT_STRUCTURED_THREEWAY_ENABLED=true, which
allows deterministic direct writes only for these two sessions.
"""

import argparse
import json
import os
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

import requests

sys.path.insert(0, str(Path(__file__).parent))

from prepare_structured_threeway_seed import (
    ALLOWED_USER_ID,
    DEFAULT_CORPUS,
    PROJECT_ROOT,
    build_requests,
    canonical_json,
    sha256_bytes,
)


TARGET_SESSION_ID = "eval_retrieval_test"
CONTROL_SESSION_ID = "eval_unlearning_control"
STRUCTURED_MODE_ENV = "EXPERIMENT_STRUCTURED_THREEWAY_ENABLED"
EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
DEFAULT_OUTPUT_ROOT = EXPERIMENTS_DIR / "results" / "unlearning_trial_seed"
REQUIRED_STORES = ("qdrant", "timescaledb", "neo4j", "session_file_cache")


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def authenticate(base_url: str, timeout: int) -> str:
    token = os.getenv("EVAL_AUTH_TOKEN", "").strip()
    if token:
        return token
    password = os.getenv("EVAL_PASSWORD", "")
    if not password:
        raise ValueError("EVAL_PASSWORD or EVAL_AUTH_TOKEN is required")
    response = requests.post(
        f"{base_url}/api/auth/login",
        json={"user_id": ALLOWED_USER_ID, "password": password},
        timeout=timeout,
    )
    if response.status_code != 200:
        raise RuntimeError(f"login failed with HTTP {response.status_code}")
    payload = response.json()
    data = payload.get("data", payload) if isinstance(payload, dict) else {}
    token = data.get("token") if isinstance(data, dict) else None
    if not isinstance(token, str) or not token:
        raise RuntimeError("login response did not contain a token")
    return token


def with_trial_metadata(requests_to_submit: List[Dict[str, Any]], trial: int, role: str) -> List[Dict[str, Any]]:
    seeded: List[Dict[str, Any]] = []
    for item in requests_to_submit:
        request = json.loads(json.dumps(item["request"]))
        metadata = request["metadata"]
        metadata["unlearning_trial"] = trial
        metadata["unlearning_seed_role"] = role
        seeded.append({
            "doc_id": item["doc_id"],
            "source_record_sha256": item["source_record_sha256"],
            "request": request,
            "request_sha256": sha256_bytes(canonical_json(request)),
        })
    return seeded


def retry_after_seconds(response: Optional[requests.Response], retry_number: int) -> float:
    if response is not None:
        value = response.headers.get("Retry-After", "").strip()
        try:
            if value:
                return max(0.0, float(value))
        except ValueError:
            pass
    return min(30.0, float(2 ** (retry_number - 1)))


def unwrap_evidence_documents(payload: Any) -> Optional[Dict[str, Any]]:
    """Accept documented data/result envelopes, never a response without documents."""
    if not isinstance(payload, dict):
        return None
    if payload.get("success") is False:
        return None
    current: Dict[str, Any] = payload
    for _ in range(5):
        if isinstance(current.get("documents"), list):
            return current
        for envelope_key in ("data", "result"):
            nested = current.get(envelope_key)
            if isinstance(nested, dict):
                current = nested
                break
        else:
            return None
    return None


def response_summary(response: Optional[requests.Response], error: Optional[str] = None) -> Dict[str, Any]:
    summary: Dict[str, Any] = {"http_status": response.status_code if response is not None else 0}
    if response is None:
        summary["error"] = error or "request did not return a response"
        return summary
    summary["response_sha256"] = sha256_bytes(response.content)
    try:
        payload = response.json()
    except ValueError:
        summary["error"] = "response was not valid JSON"
        return summary
    if not isinstance(payload, dict):
        summary["error"] = "response JSON was not an object"
        return summary
    for key in ("success", "complete", "error"):
        if key in payload:
            summary[key] = payload[key]
    return summary


def cascade_result(payload: Any) -> Optional[Dict[str, Any]]:
    if not isinstance(payload, dict):
        return None
    current: Dict[str, Any] = payload
    for _ in range(3):
        result = current.get("result")
        if isinstance(result, dict):
            return result
        data = current.get("data")
        if isinstance(data, dict):
            current = data
            continue
        return None
    return None


def parse_cascade_counts(response: requests.Response, expected_status: str, require_complete: bool) -> Tuple[Optional[Dict[str, Dict[str, int]]], List[str]]:
    if response.status_code != 200:
        return None, [f"target cleanup HTTP {response.status_code}: {response.text[:300]}"]
    try:
        result = cascade_result(response.json())
    except ValueError as exc:
        return None, [f"target cleanup response JSON invalid: {exc}"]
    if not isinstance(result, dict):
        return None, ["target cleanup did not return a cascade result"]
    if require_complete and result.get("complete") is not True:
        return None, ["target cleanup cascade did not report complete=true"]
    stores = result.get("stores")
    if not isinstance(stores, list):
        return None, ["target cleanup did not return stores"]

    by_store: Dict[str, Dict[str, Any]] = {}
    for store in stores:
        if not isinstance(store, dict) or not isinstance(store.get("store"), str):
            return None, ["target cleanup returned a malformed store result"]
        name = store["store"]
        if name in by_store:
            return None, [f"target cleanup returned duplicate {name} results"]
        by_store[name] = store

    counts: Dict[str, Dict[str, int]] = {}
    errors: List[str] = []
    for name in REQUIRED_STORES:
        store = by_store.get(name)
        if not isinstance(store, dict):
            errors.append(f"target cleanup missing required store {name}")
            continue
        if store.get("status") != expected_status or store.get("error"):
            errors.append(f"target cleanup {name} status/error is invalid")
            continue
        before, after, deleted = store.get("before"), store.get("after"), store.get("deleted")
        if not all(isinstance(value, int) and value >= 0 for value in (before, after, deleted)):
            errors.append(f"target cleanup {name} counts are invalid")
            continue
        if expected_status == "dry_run" and (after != before or deleted != 0):
            errors.append(f"target cleanup {name} dry-run mutated or misreported state")
            continue
        if expected_status == "deleted" and after != 0:
            errors.append(f"target cleanup {name} was not verified empty")
            continue
        counts[name] = {"before": before, "deleted": deleted, "after": after}
    return counts if not errors else None, errors


def verify_target_threeway_absent(base_url: str, token: str, doc_ids: List[str], timeout: int) -> Tuple[Dict[str, Any], List[str]]:
    response = requests.post(
        f"{base_url}/api/v1/experiments/threeway/evidence",
        json={"user_id": ALLOWED_USER_ID, "session_id": TARGET_SESSION_ID, "doc_ids": doc_ids},
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        timeout=timeout,
    )
    result = response_summary(response)
    if response.status_code != 200:
        return result, [f"target absence evidence HTTP {response.status_code}: {response.text[:300]}"]
    try:
        data = unwrap_evidence_documents(response.json())
    except ValueError as exc:
        return result, [f"target absence evidence JSON invalid: {exc}"]
    documents = data.get("documents") if data is not None else None
    if not isinstance(documents, list):
        return result, ["target absence evidence response did not contain documents"]
    by_doc_id = {item.get("doc_id"): item for item in documents if isinstance(item, dict)}
    errors: List[str] = []
    if set(by_doc_id) != set(doc_ids):
        errors.append("target absence evidence document IDs do not exactly match the fixed corpus")
    for doc_id in doc_ids:
        document = by_doc_id.get(doc_id)
        if not isinstance(document, dict):
            errors.append(f"target absence evidence missing {doc_id}")
            continue
        for lane in ("vector", "timeline", "graph"):
            lane_result = document.get(lane)
            if not isinstance(lane_result, dict) or lane_result.get("status") == "error" or lane_result.get("error"):
                errors.append(f"target absence evidence {doc_id}/{lane} failed")
                continue
            if lane_result.get("count") != 0:
                errors.append(f"target absence evidence {doc_id}/{lane} is not empty")
    result["documents"] = documents
    return result, errors


def clear_target_session(base_url: str, token: str, doc_ids: List[str], timeout: int) -> Tuple[Dict[str, Any], List[str]]:
    """Clear target only and capture the cascade proof before any seed write."""
    headers = {"Authorization": f"Bearer {token}"}
    endpoint = f"{base_url}/api/sessions/{TARGET_SESSION_ID}"
    cleanup: Dict[str, Any] = {"target_session_id": TARGET_SESSION_ID}
    try:
        dry_run = requests.delete(f"{endpoint}?dry_run=true", headers=headers, timeout=timeout)
    except requests.RequestException as exc:
        cleanup["dry_run"] = response_summary(None, str(exc))
        return cleanup, [f"target cleanup dry-run request failed: {exc}"]
    cleanup["dry_run"] = response_summary(dry_run)

    # A successful prior trial removes the local target session. Keep the
    # cleanup call explicit, then prove the fixed corpus is absent in all three
    # durable lanes before treating the missing local cache as empty.
    if dry_run.status_code == 404:
        try:
            deletion = requests.delete(f"{endpoint}?dry_run=false", headers=headers, timeout=timeout)
        except requests.RequestException as exc:
            cleanup["delete"] = response_summary(None, str(exc))
            return cleanup, [f"target cleanup delete request failed: {exc}"]
        cleanup["delete"] = response_summary(deletion)
        if deletion.status_code != 404:
            return cleanup, [f"target cleanup changed from dry-run 404 to delete HTTP {deletion.status_code}"]
        absence_evidence, evidence_errors = verify_target_threeway_absent(base_url, token, doc_ids, timeout)
        cleanup["absence_evidence"] = absence_evidence
        if evidence_errors:
            return cleanup, evidence_errors
        cleanup["state"] = "already_absent"
        cleanup["pre_counts"] = {name: 0 for name in REQUIRED_STORES}
        cleanup["post_counts"] = {name: 0 for name in REQUIRED_STORES}
        return cleanup, []

    dry_counts, dry_errors = parse_cascade_counts(dry_run, "dry_run", require_complete=False)
    if dry_errors:
        return cleanup, dry_errors
    cleanup["pre_counts"] = {name: counts["before"] for name, counts in (dry_counts or {}).items()}

    try:
        deletion = requests.delete(f"{endpoint}?dry_run=false", headers=headers, timeout=timeout)
    except requests.RequestException as exc:
        cleanup["delete"] = response_summary(None, str(exc))
        return cleanup, [f"target cleanup delete request failed: {exc}"]
    cleanup["delete"] = response_summary(deletion)
    delete_counts, delete_errors = parse_cascade_counts(deletion, "deleted", require_complete=True)
    if delete_errors:
        return cleanup, delete_errors
    cleanup["post_counts"] = {name: counts["after"] for name, counts in (delete_counts or {}).items()}
    cleanup["state"] = "deleted"
    return cleanup, []


def submit_session(
    base_url: str,
    token: str,
    session_id: str,
    records: List[Dict[str, Any]],
    timeout: int,
    request_interval_seconds: float = 1.2,
    max_retries: int = 6,
) -> Tuple[List[Dict[str, Any]], List[str]]:
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    submissions: List[Dict[str, Any]] = []
    errors: List[str] = []
    for record in records:
        response_record: Dict[str, Any] = {
            "doc_id": record["doc_id"],
            "request_sha256": record["request_sha256"],
            "attempts": [],
        }
        for attempt in range(1, max_retries + 2):
            response: Optional[requests.Response] = None
            try:
                response = requests.post(f"{base_url}/mcp/tools/create_context", json=record["request"], headers=headers, timeout=timeout)
                attempt_record: Dict[str, Any] = {"attempt": attempt, "http_status": response.status_code}
                if response.headers.get("X-RateLimit-Limit"):
                    attempt_record["rate_limit_limit"] = response.headers["X-RateLimit-Limit"]
                response_record["attempts"].append(attempt_record)
                response_record["http_status"] = response.status_code
                response_record["response_sha256"] = sha256_bytes(response.content)
                if response.status_code == 200:
                    payload = response.json()
                    response_record["memory_id"] = payload.get("memoryId") if isinstance(payload, dict) else None
                    break
                retryable = response.status_code == 429 or response.status_code >= 500
                if not retryable or attempt > max_retries:
                    errors.append(f"{session_id}/{record['doc_id']}: HTTP {response.status_code}: {response.text[:300]}")
                    break
                delay = max(request_interval_seconds, retry_after_seconds(response, attempt))
                response_record["attempts"][-1]["retry_delay_seconds"] = delay
                time.sleep(delay)
            except requests.RequestException as exc:
                response_record["attempts"].append({"attempt": attempt, "transport_error": str(exc)})
                if attempt > max_retries:
                    errors.append(f"{session_id}/{record['doc_id']}: {exc}")
                    break
                delay = max(request_interval_seconds, retry_after_seconds(None, attempt))
                response_record["attempts"][-1]["retry_delay_seconds"] = delay
                time.sleep(delay)
            except ValueError as exc:
                errors.append(f"{session_id}/{record['doc_id']}: invalid success response: {exc}")
                break
            finally:
                # A successful write must also leave room for the next write.
                if response is not None and response.status_code == 200 and request_interval_seconds > 0:
                    time.sleep(request_interval_seconds)
        submissions.append(response_record)
    return submissions, errors


def verify_target_threeway(base_url: str, token: str, doc_ids: List[str], timeout: int) -> Tuple[Dict[str, Any], List[str]]:
    response = requests.post(
        f"{base_url}/api/v1/experiments/threeway/evidence",
        json={"user_id": ALLOWED_USER_ID, "session_id": TARGET_SESSION_ID, "doc_ids": doc_ids},
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        timeout=timeout,
    )
    result: Dict[str, Any] = {"http_status": response.status_code, "response_sha256": sha256_bytes(response.content)}
    if response.status_code != 200:
        return result, [f"target evidence HTTP {response.status_code}: {response.text[:300]}"]
    payload = response.json()
    data = unwrap_evidence_documents(payload)
    documents = data.get("documents") if data is not None else None
    if not isinstance(documents, list):
        return result, ["target evidence response did not contain documents"]
    by_doc_id = {item.get("doc_id"): item for item in documents if isinstance(item, dict)}
    errors: List[str] = []
    if set(by_doc_id) != set(doc_ids):
        errors.append("target evidence document IDs do not exactly match the fixed corpus")
    for doc_id in doc_ids:
        document = by_doc_id.get(doc_id)
        if not isinstance(document, dict):
            errors.append(f"target evidence missing {doc_id}")
            continue
        for lane in ("vector", "timeline", "graph"):
            lane_result = document.get(lane)
            if not isinstance(lane_result, dict) or lane_result.get("verified") is not True or lane_result.get("count", 0) <= 0:
                errors.append(f"target evidence {doc_id}/{lane} is not verified")
    result["documents"] = documents
    return result, errors


def verify_control_snapshot(base_url: str, token: str, timeout: int) -> Tuple[Dict[str, Any], List[str]]:
    response = requests.delete(
        f"{base_url}/api/sessions/{CONTROL_SESSION_ID}?dry_run=true",
        headers={"Authorization": f"Bearer {token}"},
        timeout=timeout,
    )
    result: Dict[str, Any] = {"http_status": response.status_code, "response_sha256": sha256_bytes(response.content)}
    if response.status_code != 200:
        return result, [f"control dry-run HTTP {response.status_code}: {response.text[:300]}"]
    payload = response.json()
    cascade = payload.get("result") if isinstance(payload, dict) else None
    stores = cascade.get("stores") if isinstance(cascade, dict) else None
    if not isinstance(stores, list):
        return result, ["control dry-run did not return stores"]
    by_store = {item.get("store"): item for item in stores if isinstance(item, dict)}
    errors: List[str] = []
    for name in REQUIRED_STORES:
        store = by_store.get(name)
        if not isinstance(store, dict) or store.get("status") != "dry_run" or store.get("error"):
            errors.append(f"control {name} dry-run is unavailable")
            continue
        if not isinstance(store.get("before"), int) or store["before"] <= 0 or store.get("after") != store["before"]:
            errors.append(f"control {name} was not seeded or dry-run count is invalid")
    result["stores"] = stores
    return result, errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--trial", type=int, required=True)
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--timeout", type=int, default=90)
    parser.add_argument("--max-requests-per-minute", type=float, default=50.0)
    parser.add_argument("--max-retries", type=int, default=6)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    try:
        if args.trial <= 0:
            raise ValueError("--trial must be positive")
        if args.max_requests_per_minute <= 0 or args.max_requests_per_minute > 50:
            raise ValueError("--max-requests-per-minute must be greater than zero and no more than 50")
        if args.max_retries < 0:
            raise ValueError("--max-retries must not be negative")
        if os.getenv("EVAL_USER_ID", ALLOWED_USER_ID).strip() != ALLOWED_USER_ID:
            raise ValueError("EVAL_USER_ID must be eval_user_001")
        if os.getenv(STRUCTURED_MODE_ENV, "").strip().lower() != "true":
            raise ValueError(f"{STRUCTURED_MODE_ENV}=true is required for bounded structured writes")
        if not DEFAULT_CORPUS.is_file():
            raise ValueError(f"fixed corpus is missing: {DEFAULT_CORPUS}")

        target_manifest, target_requests = build_requests(DEFAULT_CORPUS, TARGET_SESSION_ID)
        _, control_requests = build_requests(DEFAULT_CORPUS, CONTROL_SESSION_ID)
        if len(target_requests) != 30 or len(control_requests) != 30:
            raise ValueError("fixed corpus must produce exactly 30 requests for each session")
        target_records = with_trial_metadata(target_requests, args.trial, "target")
        control_records = with_trial_metadata(control_requests, args.trial, "control")
        doc_ids = [record["doc_id"] for record in target_records]

        output_dir = args.output_root / f"trial-{args.trial:02d}"
        output_dir.mkdir(parents=True, exist_ok=True)
        token = authenticate(args.base_url.rstrip("/"), args.timeout)
        target_cleanup, target_cleanup_errors = clear_target_session(args.base_url.rstrip("/"), token, doc_ids, args.timeout)
        if target_cleanup_errors:
            manifest = {
                "schema": "unlearning-trial-seed-v1",
                "created_at": utc_now(),
                "trial": args.trial,
                "scope": {"user_id": ALLOWED_USER_ID, "target_session_id": TARGET_SESSION_ID, "control_session_id": CONTROL_SESSION_ID},
                "source": target_manifest["source"],
                "target_doc_ids": doc_ids,
                "target_cleanup": target_cleanup,
                "writes": {"target": [], "control": []},
                "errors": target_cleanup_errors,
                "control_deleted": False,
            }
            write_json(output_dir / "manifest.json", manifest)
            write_json(output_dir / "target_doc_ids.json", doc_ids)
            print(f"manifest={output_dir / 'manifest.json'}")
            print(f"target_doc_ids={output_dir / 'target_doc_ids.json'}")
            print("seed_status=failed")
            return 1

        request_interval_seconds = 60.0 / args.max_requests_per_minute
        target_submission, target_errors = submit_session(args.base_url.rstrip("/"), token, TARGET_SESSION_ID, target_records, args.timeout, request_interval_seconds, args.max_retries)
        control_submission, control_errors = submit_session(args.base_url.rstrip("/"), token, CONTROL_SESSION_ID, control_records, args.timeout, request_interval_seconds, args.max_retries)
        target_evidence, target_evidence_errors = verify_target_threeway(args.base_url.rstrip("/"), token, doc_ids, args.timeout)
        control_snapshot, control_snapshot_errors = verify_control_snapshot(args.base_url.rstrip("/"), token, args.timeout)

        manifest = {
            "schema": "unlearning-trial-seed-v1",
            "created_at": utc_now(),
            "trial": args.trial,
            "scope": {"user_id": ALLOWED_USER_ID, "target_session_id": TARGET_SESSION_ID, "control_session_id": CONTROL_SESSION_ID},
            "source": target_manifest["source"],
            "target_doc_ids": doc_ids,
            "target_cleanup": target_cleanup,
            "writes": {"target": target_submission, "control": control_submission},
            "rate_limit": {
                "requested_max_requests_per_minute": args.max_requests_per_minute,
                "request_interval_seconds": request_interval_seconds,
                "max_retries": args.max_retries,
                "observed_limits": sorted({attempt["rate_limit_limit"] for submission in target_submission + control_submission for attempt in submission["attempts"] if "rate_limit_limit" in attempt}),
            },
            "verification": {"target_threeway": target_evidence, "control_dry_run": control_snapshot},
            "errors": target_errors + control_errors + target_evidence_errors + control_snapshot_errors,
            "control_deleted": False,
        }
        write_json(output_dir / "manifest.json", manifest)
        write_json(output_dir / "target_doc_ids.json", doc_ids)
        print(f"manifest={output_dir / 'manifest.json'}")
        print(f"target_doc_ids={output_dir / 'target_doc_ids.json'}")
        print(f"seed_status={'passed' if not manifest['errors'] else 'failed'}")
        return 0 if not manifest["errors"] else 1
    except (ValueError, OSError, json.JSONDecodeError, requests.RequestException, RuntimeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
