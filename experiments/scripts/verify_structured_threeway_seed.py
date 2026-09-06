#!/usr/bin/env python3
"""Verify the audited inputs and available retrieval evidence for a three-way seed."""

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Tuple

import requests


ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
PROJECT_ROOT = EXPERIMENTS_DIR.parent
DEFAULT_DIR = EXPERIMENTS_DIR / "results" / "structured_threeway_seed" / ALLOWED_SESSION_ID


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def validate_scope(user_id: str, session_id: str) -> None:
    if user_id != ALLOWED_USER_ID:
        raise ValueError("EVAL_USER_ID must be eval_user_001 for this tool")
    if session_id != ALLOWED_SESSION_ID:
        raise ValueError("session_id must be eval_retrieval_test for this tool")


def read_jsonl(path: Path) -> List[Dict[str, Any]]:
    records = []
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if line.strip():
            try:
                record = json.loads(line)
            except json.JSONDecodeError as exc:
                raise ValueError(f"invalid JSONL at {path}:{line_number}") from exc
            if not isinstance(record, dict):
                raise ValueError(f"JSONL record at {path}:{line_number} must be an object")
            records.append(record)
    return records


def audit_artifacts(manifest_path: Path) -> Tuple[Dict[str, Any], List[Dict[str, Any]], List[str]]:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    errors: List[str] = []
    if manifest.get("schema") != "structured-threeway-seed-manifest-v1":
        errors.append("unexpected manifest schema")
    scope = manifest.get("scope", {})
    if scope.get("user_id") != ALLOWED_USER_ID or scope.get("session_id") != ALLOWED_SESSION_ID:
        errors.append("manifest scope is not eval_user_001/eval_retrieval_test")
    source = manifest.get("source", {})
    corpus_documents: Dict[str, Dict[str, Any]] = {}
    try:
        source_path = PROJECT_ROOT / source["path"]
        source_bytes = source_path.read_bytes()
        if sha256_bytes(source_bytes) != source.get("sha256"):
            errors.append("source corpus SHA-256 does not match manifest")
        source_data = json.loads(source_bytes.decode("utf-8"))
        source_records = source_data.get("documents", []) if isinstance(source_data, dict) else []
        corpus_documents = {record.get("doc_id"): record for record in source_records if isinstance(record, dict)}
        expected_document_count = source.get("document_count")
        if not isinstance(expected_document_count, int) or expected_document_count < 1:
            errors.append("manifest source does not declare a positive document_count")
        elif len(corpus_documents) != expected_document_count:
            errors.append("source corpus document count does not match manifest")
    except (KeyError, OSError):
        errors.append("manifest source path is unreadable")
    except (UnicodeDecodeError, json.JSONDecodeError):
        errors.append("source corpus is not valid UTF-8 JSON")
    request_file = manifest.get("request_file", {})
    request_path = manifest_path.parent / request_file.get("path", "")
    try:
        if sha256_bytes(request_path.read_bytes()) != request_file.get("sha256"):
            errors.append("request JSONL SHA-256 does not match manifest")
        request_records = read_jsonl(request_path)
    except OSError:
        errors.append("request JSONL is unreadable")
        request_records = []
    manifest_docs = manifest.get("documents", [])
    expected_document_count = source.get("document_count") if isinstance(source, dict) else 0
    if len(manifest_docs) != expected_document_count or len(request_records) != expected_document_count:
        errors.append("manifest and request JSONL document counts do not match the source corpus")
    manifest_by_id = {item.get("doc_id"): item for item in manifest_docs if isinstance(item, dict)}
    manifest_ids = set(manifest_by_id)
    request_ids = set()
    for item in request_records:
        doc_id = item.get("doc_id")
        request = item.get("request")
        request_ids.add(doc_id)
        if not isinstance(request, dict):
            errors.append(f"{doc_id}: request is missing")
            continue
        if request.get("sessionId") != ALLOWED_SESSION_ID:
            errors.append(f"{doc_id}: request session is outside the allowed scope")
        metadata = request.get("metadata", {})
        if metadata.get("doc_id") != doc_id or metadata.get("id") != doc_id:
            errors.append(f"{doc_id}: request metadata does not preserve doc_id")
        if item.get("request_sha256") != sha256_bytes(canonical_json(request)):
            errors.append(f"{doc_id}: request SHA-256 does not match")
        source_record = corpus_documents.get(doc_id)
        manifest_record = manifest_by_id.get(doc_id)
        if source_record is None or manifest_record is None:
            errors.append(f"{doc_id}: does not exist in both source corpus and manifest")
            continue
        source_record_sha256 = sha256_bytes(canonical_json(source_record))
        if item.get("source_record_sha256") != source_record_sha256 or manifest_record.get("source_record_sha256") != source_record_sha256:
            errors.append(f"{doc_id}: source record SHA-256 does not match the corpus")
        if manifest_record.get("request_sha256") != item.get("request_sha256"):
            errors.append(f"{doc_id}: manifest request SHA-256 does not match JSONL")
        if request.get("content") != source_record.get("content"):
            errors.append(f"{doc_id}: request content differs from the corpus")
        source_metadata = source_record.get("metadata", {})
        if not isinstance(source_metadata, dict):
            errors.append(f"{doc_id}: source metadata is invalid")
            continue
        for key, expected in source_metadata.items():
            if metadata.get(key) != expected:
                errors.append(f"{doc_id}: request metadata.{key} differs from the corpus")
        if metadata.get("source_record_sha256") != source_record_sha256 or metadata.get("seed_schema") != "structured-threeway-v1":
            errors.append(f"{doc_id}: required audit metadata is missing or mismatched")
    if len(request_ids) != expected_document_count or manifest_ids != request_ids:
        errors.append("manifest and request document IDs do not match exactly")
    return manifest, request_records, errors


def authenticate(base_url: str, user_id: str, password: str, timeout: int) -> str:
    response = requests.post(
        f"{base_url.rstrip('/')}/api/auth/login",
        json={"user_id": user_id, "password": password},
        timeout=timeout,
    )
    if response.status_code != 200:
        raise RuntimeError(f"login failed with HTTP {response.status_code}")
    data = response.json()
    token = data.get("token") or data.get("data", {}).get("token")
    if not isinstance(token, str) or not token:
        raise RuntimeError("login response did not contain a token")
    return token


def contains_value(value: Any, expected: str) -> bool:
    if isinstance(value, str):
        return expected in value
    if isinstance(value, dict):
        return any(contains_value(child, expected) for child in value.values())
    if isinstance(value, list):
        return any(contains_value(child, expected) for child in value)
    return False


def verify_vector_retrieval(base_url: str, token: str, records: Iterable[Dict[str, Any]], timeout: int) -> List[Dict[str, Any]]:
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    results = []
    for record in records:
        doc_id = record["doc_id"]
        try:
            response = requests.post(
                f"{base_url.rstrip('/')}/mcp/tools/retrieve_context",
                json={
                    "sessionId": ALLOWED_SESSION_ID,
                    "query": doc_id,
                    "limit": 10,
                    "contextsOnly": True,
                    "evaluationRetrievalOnly": True,
                },
                headers=headers,
                timeout=timeout,
            )
            result = {"doc_id": doc_id, "http_status": response.status_code, "response_sha256": sha256_bytes(response.content)}
            if response.status_code == 200:
                result["status"] = "verified" if contains_value(response.json(), doc_id) else "not_found"
            else:
                result["status"] = "http_error"
        except (requests.RequestException, ValueError) as exc:
            result = {"doc_id": doc_id, "status": "transport_or_json_error", "error": str(exc)}
        results.append(result)
    return results


def verify_threeway_evidence(base_url: str, token: str, records: Iterable[Dict[str, Any]], timeout: int) -> Dict[str, Any]:
    doc_ids = [record["doc_id"] for record in records]
    response = requests.post(
        f"{base_url.rstrip('/')}/api/v1/experiments/threeway/evidence",
        json={"user_id": ALLOWED_USER_ID, "session_id": ALLOWED_SESSION_ID, "doc_ids": doc_ids},
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        timeout=timeout,
    )
    result: Dict[str, Any] = {"http_status": response.status_code, "response_sha256": sha256_bytes(response.content)}
    if response.status_code != 200:
        result.update({"status": "http_error", "error": response.text[:500]})
        return result
    payload = response.json()
    if payload.get("success") is not True or not isinstance(payload.get("data"), dict):
        result.update({"status": "invalid_envelope"})
        return result
    documents = payload["data"].get("documents")
    if not isinstance(documents, list) or len(documents) != len(doc_ids):
        result.update({"status": "invalid_document_count"})
        return result
    by_doc_id = {item.get("doc_id"): item for item in documents if isinstance(item, dict)}
    evidence = []
    for doc_id in doc_ids:
        document = by_doc_id.get(doc_id, {})
        lanes = {lane: document.get(lane, {}) for lane in ("vector", "timeline", "graph")}
        verified = all(isinstance(lane, dict) and lane.get("verified") is True and lane.get("count", 0) > 0 for lane in lanes.values())
        evidence.append({"doc_id": doc_id, "verified": verified, "lanes": lanes})
    verified_count = sum(item["verified"] for item in evidence)
    result.update({
        "status": "verified" if verified_count == len(doc_ids) else "incomplete",
        "verified": verified_count,
        "expected": len(doc_ids),
        "results": evidence,
    })
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, default=DEFAULT_DIR / "manifest.json")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--session-id", default=ALLOWED_SESSION_ID)
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--timeout", type=int, default=30)
    parser.add_argument("--dry-run", action="store_true", help="Audit files only; do not authenticate or call HTTP APIs")
    parser.add_argument("--require-vector-evidence", action="store_true", help="Return non-zero unless every document is found through retrieval")
    parser.add_argument("--require-threeway-evidence", action="store_true", help="Return non-zero unless every document has vector, timeline, and graph evidence")
    args = parser.parse_args()

    try:
        user_id = os.environ.get("EVAL_USER_ID", ALLOWED_USER_ID).strip()
        validate_scope(user_id, args.session_id)
        manifest, records, errors = audit_artifacts(args.manifest)
        report: Dict[str, Any] = {
            "schema": "structured-threeway-verification-v2",
            "verified_at": utc_now(),
            "manifest": str(args.manifest.resolve()),
            "scope": manifest.get("scope"),
            "input_audit": {"status": "passed" if not errors else "failed", "errors": errors},
            "vector": {"status": "not_run"},
            "three_way": {"status": "not_run"},
        }
        if not args.dry_run and not errors:
            password = os.environ.get("EVAL_PASSWORD", "")
            if not password:
                raise ValueError("EVAL_PASSWORD is required unless --dry-run is used")
            token = authenticate(args.base_url, user_id, password, args.timeout)
            three_way = verify_threeway_evidence(args.base_url, token, records, args.timeout)
            report["three_way"] = three_way
            vector_results = three_way.get("results", [])
            vector_verified = sum(bool(item.get("lanes", {}).get("vector", {}).get("verified")) for item in vector_results)
            report["vector"] = {"status": "verified" if vector_verified == len(records) else "incomplete", "verified": vector_verified, "expected": len(records), "results": vector_results}
        output_path = args.output or args.manifest.parent / "verification.json"
        output_path.parent.mkdir(parents=True, exist_ok=True)
        write_json(output_path, report)
        print(f"verification={output_path}")
        print(f"input_audit={report['input_audit']['status']}")
        print(f"vector={report['vector']['status']}")
        print(f"three_way={report['three_way']['status']}")
        if errors:
            return 2
        if args.require_vector_evidence and report["vector"]["status"] != "verified":
            return 1
        if args.require_threeway_evidence and report["three_way"]["status"] != "verified":
            return 1
        return 0
    except (ValueError, OSError, json.JSONDecodeError, requests.RequestException, RuntimeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
