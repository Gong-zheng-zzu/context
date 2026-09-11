#!/usr/bin/env python3
"""Prepare audited three-way ingestion requests for the fixed retrieval corpus."""

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Tuple
sys.path.insert(0, str(Path(__file__).resolve().parent))
from experiment_integrity import IntegrityConfigurationError, dual_digest_bytes, dual_digest_file, utc_now

import requests


ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
PROJECT_ROOT = EXPERIMENTS_DIR.parent
DEFAULT_CORPUS = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json"
DEFAULT_OUTPUT_DIR = EXPERIMENTS_DIR / "results" / "structured_threeway_seed" / ALLOWED_SESSION_ID


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def sha256_bytes(value: bytes) -> str:
    return dual_digest_bytes(value)["sha256"]


def write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def validate_scope(user_id: str, session_id: str) -> None:
    if user_id != ALLOWED_USER_ID:
        raise ValueError("EVAL_USER_ID must be eval_user_001 for this tool")
    if session_id != ALLOWED_SESSION_ID:
        raise ValueError("session_id must be eval_retrieval_test for this tool")


def validate_timestamp(value: Any, doc_id: str) -> str:
    if not isinstance(value, str) or not value:
        raise ValueError(f"{doc_id}: metadata.timestamp must be a non-empty ISO-8601 string")
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValueError(f"{doc_id}: invalid metadata.timestamp: {value}") from exc
    return value


def build_requests(corpus_file: Path, session_id: str) -> Tuple[Dict[str, Any], List[Dict[str, Any]]]:
    raw_corpus = corpus_file.read_bytes()
    corpus = json.loads(raw_corpus.decode("utf-8"))
    documents = corpus.get("documents") if isinstance(corpus, dict) else None
    if not isinstance(documents, list) or not documents:
        raise ValueError("corpus must contain at least one document")

    requests_to_submit: List[Dict[str, Any]] = []
    document_manifest: List[Dict[str, Any]] = []
    seen_doc_ids = set()
    causal_document_count = 0

    for record in documents:
        if not isinstance(record, dict):
            raise ValueError("each corpus document must be a JSON object")
        doc_id = record.get("doc_id")
        content = record.get("content")
        metadata = record.get("metadata")
        if not isinstance(doc_id, str) or not doc_id:
            raise ValueError("each corpus document needs a non-empty doc_id")
        if doc_id in seen_doc_ids:
            raise ValueError(f"duplicate doc_id: {doc_id}")
        seen_doc_ids.add(doc_id)
        if not isinstance(content, str) or not content:
            raise ValueError(f"{doc_id}: content must be a non-empty string")
        if not isinstance(metadata, dict):
            raise ValueError(f"{doc_id}: metadata must be an object")

        for key in ("patient_id", "patient_name", "event_type", "severity", "type"):
            if not isinstance(metadata.get(key), str) or not metadata[key]:
                raise ValueError(f"{doc_id}: metadata.{key} must be a non-empty string")
        timestamp = validate_timestamp(metadata.get("timestamp"), doc_id)
        tags = metadata.get("tags")
        if not isinstance(tags, list) or not tags or not all(isinstance(tag, str) and tag for tag in tags):
            raise ValueError(f"{doc_id}: metadata.tags must be a non-empty string list")

        source_record_sha256 = sha256_bytes(canonical_json(record))
        api_metadata = dict(metadata)
        api_metadata.update(
            {
                "doc_id": doc_id,
                "id": doc_id,
                "timestamp": timestamp,
                "source_record_sha256": source_record_sha256,
                "seed_schema": "structured-threeway-v1",
				"structured_threeway": True,
            }
        )
        payload = {"sessionId": session_id, "content": content, "metadata": api_metadata}
        request_sha256 = sha256_bytes(canonical_json(payload))
        has_causal_fields = any(key in metadata for key in ("causal_factors", "causal_relations"))
        causal_document_count += int(has_causal_fields)
        requests_to_submit.append(
            {
                "doc_id": doc_id,
                "source_record_sha256": source_record_sha256,
                "request_sha256": request_sha256,
                "lanes": {
                    "vector": "unified_create_context",
                    "timeline": "unified_create_context",
                    "graph": "unified_create_context",
                },
                "request": payload,
            }
        )
        document_manifest.append(
            {
                "doc_id": doc_id,
                "patient_id": metadata["patient_id"],
                "timestamp": timestamp,
                "event_type": metadata["event_type"],
                "tags": tags,
                "has_causal_fields": has_causal_fields,
                "source_record_sha256": source_record_sha256,
                "request_sha256": request_sha256,
            }
        )

    manifest = {
        "schema": "structured-threeway-seed-manifest-v1",
        "created_at": utc_now(),
        "scope": {"user_id": ALLOWED_USER_ID, "session_id": session_id},
        "source": {
            "path": corpus_file.resolve().relative_to(PROJECT_ROOT.resolve()).as_posix(),
            "sha256": sha256_bytes(raw_corpus),
            "dual_digest": dual_digest_bytes(raw_corpus),
            "document_count": len(documents),
            "corpus_metadata": corpus.get("metadata", {}),
        },
        "three_way_contract": {
            "ingestion_mode": "deterministic_preannotated_no_llm",
            "write_endpoint": "/mcp/tools/create_context",
            "authentication": "JWT Bearer token from /api/auth/login",
            "vector": "submitted through unified ingestion; retrieval can be independently checked",
            "timeline": "submitted through unified ingestion; no documented session/doc-scoped public read API",
            "graph": "submitted through unified ingestion; no documented session/doc-scoped public read API",
        },
        "causal_document_count": causal_document_count,
        "documents": document_manifest,
    }
    return manifest, requests_to_submit


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


def submit_requests(base_url: str, token: str, requests_to_submit: List[Dict[str, Any]], output_file: Path, timeout: int) -> int:
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    accepted = 0
    with output_file.open("w", encoding="utf-8", newline="\n") as handle:
        for item in requests_to_submit:
            response_record: Dict[str, Any] = {
                "doc_id": item["doc_id"],
                "request_sha256": item["request_sha256"],
                "submitted_at": utc_now(),
                "endpoint": "/mcp/tools/create_context",
            }
            try:
                response = requests.post(
                    f"{base_url.rstrip('/')}/mcp/tools/create_context",
                    json=item["request"],
                    headers=headers,
                    timeout=timeout,
                )
                response_record["http_status"] = response.status_code
                response_record["response_sha256"] = sha256_bytes(response.content)
                try:
                    response_json = response.json() if response.content else {}
                except ValueError:
                    response_json = {}
                    response_record["response_json"] = "unavailable"
                response_record["memory_id"] = response_json.get("memoryId")
                response_record["accepted_by_unified_ingestion"] = response.status_code == 200
                accepted += int(response.status_code == 200)
            except requests.RequestException as exc:
                response_record["transport_error"] = str(exc)
                response_record["accepted_by_unified_ingestion"] = False
            handle.write(json.dumps(response_record, ensure_ascii=False, sort_keys=True) + "\n")
    return accepted


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus-file", type=Path, default=DEFAULT_CORPUS)
    parser.add_argument("--output-dir", type=Path, default=DEFAULT_OUTPUT_DIR)
    parser.add_argument("--session-id", default=ALLOWED_SESSION_ID)
    parser.add_argument("--base-url", default="http://127.0.0.1:8088")
    parser.add_argument("--timeout", type=int, default=90)
    parser.add_argument("--submit", action="store_true", help="Authenticate and submit prepared requests to the unified API")
    parser.add_argument("--dry-run", action="store_true", help="Generate artifacts without authenticating or calling HTTP APIs")
    parser.add_argument("--overwrite", action="store_true", help="Allow replacing an existing manifest in output-dir")
    args = parser.parse_args()

    try:
        user_id = os.environ.get("EVAL_USER_ID", ALLOWED_USER_ID).strip()
        validate_scope(user_id, args.session_id)
        if args.dry_run and args.submit:
            raise ValueError("--dry-run cannot be combined with --submit")
        if not args.corpus_file.is_file():
            raise ValueError(f"corpus file does not exist: {args.corpus_file}")
        manifest, requests_to_submit = build_requests(args.corpus_file, args.session_id)
        args.output_dir.mkdir(parents=True, exist_ok=True)
        manifest_path = args.output_dir / "manifest.json"
        requests_path = args.output_dir / "create_context_requests.jsonl"
        if manifest_path.exists() and not args.overwrite:
            raise ValueError(f"manifest already exists: {manifest_path}; use --overwrite to replace it")

        with requests_path.open("w", encoding="utf-8", newline="\n") as handle:
            for item in requests_to_submit:
                handle.write(json.dumps(item, ensure_ascii=False, sort_keys=True) + "\n")
        manifest["hash_algorithms"] = ["SHA-256", "SM3"]
        manifest["request_file"] = {"path": requests_path.name, "sha256": sha256_bytes(requests_path.read_bytes()), "dual_digest": dual_digest_file(requests_path)}
        manifest["mode"] = "dry_run" if args.dry_run else ("submit" if args.submit else "prepare_only")

        if args.submit:
            password = os.environ.get("EVAL_PASSWORD", "")
            if not password:
                raise ValueError("EVAL_PASSWORD is required with --submit")
            token = authenticate(args.base_url, user_id, password, args.timeout)
            submission_path = args.output_dir / "submission.jsonl"
            accepted = submit_requests(args.base_url, token, requests_to_submit, submission_path, args.timeout)
            manifest["submission"] = {
                "path": submission_path.name,
                "sha256": sha256_bytes(submission_path.read_bytes()),
                "dual_digest": dual_digest_file(submission_path),
                "accepted_by_unified_ingestion": accepted,
                "attempted": len(requests_to_submit),
                "note": "HTTP acceptance is not proof of independent vector, timeline, and graph persistence.",
            }

        write_json(manifest_path, manifest)
        print(f"manifest={manifest_path}")
        print(f"requests={requests_path}")
        print(f"source_sha256={manifest['source']['sha256']}")
        if args.submit:
            print(f"accepted_by_unified_ingestion={manifest['submission']['accepted_by_unified_ingestion']}/{len(requests_to_submit)}")
            return 0 if manifest["submission"]["accepted_by_unified_ingestion"] == len(requests_to_submit) else 1
        return 0
    except (ValueError, OSError, json.JSONDecodeError, requests.RequestException, RuntimeError, IntegrityConfigurationError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
