#!/usr/bin/env python3
"""Fail-closed, three-trial evaluation for session-scoped machine unlearning.

Each trial must reseed an isolated target session and a non-target control
session. The evaluator deletes only eval_user_001/eval_retrieval_test and
requires the production cascade to prove deletion across Qdrant, TimescaleDB,
Neo4j, and the local session file/cache.
"""

import argparse
import json
import os
import subprocess
import sys
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Tuple

sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import APIResponse, BaseEvaluator


ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
ALLOWED_COLLECTION = "nursing_records"
REQUIRED_STORES = ("qdrant", "timescaledb", "neo4j", "session_file_cache")


@dataclass
class TrialResult:
    trial: int
    status: str
    error: Optional[str]
    seed: Dict[str, Any]
    target_pre_count: Dict[str, int]
    control_pre_count: Dict[str, int]
    delete: Dict[str, Any]
    target_post_count: Dict[str, int]
    control_post_count: Dict[str, int]
    document_evidence: Dict[str, Any]
    retrieval_probe: Dict[str, Any]
    verification: Dict[str, Any]
    timestamp_utc: str


class UnlearningEvaluator(BaseEvaluator):
    def __init__(self, base_url: str) -> None:
        super().__init__(base_url)
        self.project_root = Path(__file__).resolve().parents[2]
        self.results_dir = self.project_root / "experiments" / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)

    @staticmethod
    def _now() -> str:
        return datetime.now(timezone.utc).isoformat()

    @staticmethod
    def _payload(response: APIResponse) -> Optional[Dict[str, Any]]:
        if not response.is_valid_business_response() or not isinstance(response.data, dict):
            return None
        payload = response.data.get("result", response.data)
        return payload if isinstance(payload, dict) else None

    def session_delete(self, session_id: str, dry_run: bool) -> Tuple[Optional[Dict[str, Any]], APIResponse]:
        response = self.call_api(
            "DELETE",
            f"/api/sessions/{session_id}?dry_run={'true' if dry_run else 'false'}",
            None,
            self.get_auth_headers(),
        )
        return self._payload(response), response

    def retrieval_probe(self, session_id: str, probe_text: str, target_doc_ids: Iterable[str]) -> Dict[str, Any]:
        response = self.call_api(
            "POST",
            "/mcp/tools/retrieve_context",
            {
                "sessionId": session_id,
                "query": probe_text,
                "limit": 20,
                "contextsOnly": True,
                "evaluationRetrievalOnly": True,
            },
            self.get_auth_headers(),
        )
        payload = self._payload(response)
        if payload is None:
            return {"passed": False, "error": response.error_message, "http_status": response.http_status}

        returned_doc_ids = sorted(extract_retrieved_doc_ids(payload))
        forbidden = sorted(set(target_doc_ids).intersection(returned_doc_ids))
        return {
            "passed": not forbidden,
            "http_status": response.http_status,
            "returned_doc_ids": returned_doc_ids,
            "forbidden_doc_ids": forbidden,
        }

    def threeway_evidence(self, target_doc_ids: List[str]) -> Tuple[Optional[Dict[str, Any]], APIResponse]:
        response = self.call_api(
            "POST",
            "/api/v1/experiments/threeway/evidence",
            {"user_id": ALLOWED_USER_ID, "session_id": ALLOWED_SESSION_ID, "doc_ids": target_doc_ids},
            self.get_auth_headers(),
        )
        return unwrap_evidence_payload(response), response


def unwrap_evidence_payload(response: APIResponse) -> Optional[Dict[str, Any]]:
    """Unwrap only known API envelope keys until a documents payload is found."""
    if not response.is_valid_business_response() or not isinstance(response.data, dict):
        return None
    if response.data.get("success") is False:
        return None

    payload: Dict[str, Any] = response.data
    for _ in range(5):
        if isinstance(payload.get("documents"), list):
            return payload
        for envelope_key in ("data", "result"):
            nested = payload.get(envelope_key)
            if isinstance(nested, dict):
                payload = nested
                break
        else:
            return None
    return None


def extract_retrieved_doc_ids(payload: Any) -> set[str]:
    """Read IDs only from returned context items, never request echo fields."""
    found: set[str] = set()

    def visit(value: Any) -> None:
        if isinstance(value, dict):
            for key in ("contexts", "retrieved_contexts"):
                items = value.get(key)
                if isinstance(items, list):
                    for item in items:
                        if isinstance(item, dict):
                            for id_key in ("doc_id", "docId", "id"):
                                doc_id = item.get(id_key)
                                if isinstance(doc_id, str) and doc_id.strip():
                                    found.add(doc_id.strip())
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    visit(payload)
    return found


def parse_store_counts(payload: Optional[Dict[str, Any]], phase: str, require_positive: bool) -> Tuple[Optional[Dict[str, int]], Optional[str]]:
    if payload is None:
        return None, f"{phase}: API returned no result payload"
    stores = payload.get("stores")
    if not isinstance(stores, list):
        return None, f"{phase}: result has no stores array"

    by_name: Dict[str, Dict[str, Any]] = {}
    for store in stores:
        if not isinstance(store, dict) or not isinstance(store.get("store"), str):
            return None, f"{phase}: malformed store result"
        name = store["store"]
        if name in by_name:
            return None, f"{phase}: duplicate result for {name}"
        by_name[name] = store

    counts: Dict[str, int] = {}
    expected_status = "dry_run" if phase == "pre" else "deleted"
    for name in REQUIRED_STORES:
        store = by_name.get(name)
        if store is None:
            return None, f"{phase}: required store {name} is missing"
        if store.get("status") != expected_status:
            return None, f"{phase}: {name} status is {store.get('status')!r}, expected {expected_status!r}"
        if store.get("error"):
            return None, f"{phase}: {name} reported {store['error']}"

        before = store.get("before")
        after = store.get("after")
        deleted = store.get("deleted")
        if not isinstance(before, int) or before < 0 or not isinstance(after, int) or after < 0:
            return None, f"{phase}: {name} has invalid count values"
        if phase == "pre":
            if deleted != 0 or after != before:
                return None, f"{phase}: {name} dry-run mutated or misreported counts"
            if require_positive and before <= 0:
                return None, f"{phase}: {name} must contain seeded control data"
            counts[name] = before
        else:
            if not isinstance(deleted, int) or deleted < 0 or after != 0:
                return None, f"{phase}: {name} was not verified empty after deletion"
            counts[name] = after

    if phase == "delete" and payload.get("complete") is not True:
        return None, "delete: cascade did not report complete=true"
    return counts, None


def parse_document_evidence(payload: Optional[Dict[str, Any]], target_doc_ids: List[str], expect_present: bool) -> Tuple[Optional[Dict[str, Any]], Optional[str]]:
    if payload is None:
        return None, "document evidence API returned no result payload"
    documents = payload.get("documents")
    if not isinstance(documents, list):
        return None, "document evidence result has no documents array"
    by_doc_id = {item.get("doc_id"): item for item in documents if isinstance(item, dict) and isinstance(item.get("doc_id"), str)}
    if set(by_doc_id) != set(target_doc_ids):
        return None, "document evidence IDs do not exactly match the retrieval probe IDs"

    evidence: Dict[str, Any] = {}
    for doc_id in target_doc_ids:
        lanes = by_doc_id[doc_id]
        evidence[doc_id] = {}
        for lane_name in ("vector", "timeline", "graph"):
            lane = lanes.get(lane_name)
            if not isinstance(lane, dict):
                return None, f"{doc_id}: {lane_name} evidence is missing"
            if lane.get("status") == "error" or lane.get("error"):
                return None, f"{doc_id}: {lane_name} evidence failed: {lane.get('error', 'unknown error')}"
            count = lane.get("count", 0)
            if not isinstance(count, int) or count < 0:
                return None, f"{doc_id}: {lane_name} count is invalid"
            if expect_present and count <= 0:
                return None, f"{doc_id}: {lane_name} has no seeded provenance"
            if not expect_present and count != 0:
                return None, f"{doc_id}: {lane_name} still has {count} records after deletion"
            evidence[doc_id][lane_name] = count
    return evidence, None


def run_seed(command: str, trial: int, user_id: str, target_session: str, control_session: str) -> Dict[str, Any]:
    env = os.environ.copy()
    env.update({
        "EVAL_UNLEARNING_TRIAL": str(trial),
        "EVAL_UNLEARNING_USER_ID": user_id,
        "EVAL_UNLEARNING_SESSION_ID": target_session,
        "EVAL_UNLEARNING_CONTROL_SESSION_ID": control_session,
    })
    try:
        completed = subprocess.run(command.replace("{trial}", str(trial)), shell=True, text=True, capture_output=True, env=env)
    except OSError as exc:
        return {"command": command, "returncode": -1, "stdout": "", "stderr": str(exc)}
    return {
        "command": command,
        "returncode": completed.returncode,
        "stdout": completed.stdout[-4000:],
        "stderr": completed.stderr[-4000:],
    }


def run_trial(
    evaluator: UnlearningEvaluator,
    trial: int,
    seed_command: str,
    target_doc_ids: List[str],
    probe_text: str,
    control_session: str,
) -> TrialResult:
    seed = run_seed(seed_command, trial, ALLOWED_USER_ID, ALLOWED_SESSION_ID, control_session)
    empty: Dict[str, int] = {}
    empty_evidence: Dict[str, Any] = {}
    if seed["returncode"] != 0:
        return TrialResult(trial, "seed_failed", "seed command failed", seed, empty, empty, {}, empty, empty, empty_evidence, {}, {}, evaluator._now())

    target_pre_payload, target_pre_response = evaluator.session_delete(ALLOWED_SESSION_ID, dry_run=True)
    target_pre, error = parse_store_counts(target_pre_payload, "pre", require_positive=True)
    if error:
        return TrialResult(trial, "target_precheck_failed", error, seed, empty, empty, response_summary(target_pre_response), empty, empty, empty_evidence, {}, {}, evaluator._now())

    document_pre_payload, document_pre_response = evaluator.threeway_evidence(target_doc_ids)
    document_pre, error = parse_document_evidence(document_pre_payload, target_doc_ids, expect_present=True)
    if error:
        return TrialResult(trial, "document_precheck_failed", error, seed, target_pre or empty, empty, response_summary(document_pre_response), empty, empty, {"pre": empty_evidence}, {}, {}, evaluator._now())

    control_pre_payload, control_pre_response = evaluator.session_delete(control_session, dry_run=True)
    control_pre, error = parse_store_counts(control_pre_payload, "pre", require_positive=True)
    if error:
        return TrialResult(trial, "control_precheck_failed", error, seed, target_pre or empty, empty, response_summary(control_pre_response), empty, empty, {"pre": document_pre or empty_evidence}, {}, {}, evaluator._now())

    delete_payload, delete_response = evaluator.session_delete(ALLOWED_SESSION_ID, dry_run=False)
    target_post, error = parse_store_counts(delete_payload, "delete", require_positive=False)
    if error:
        return TrialResult(trial, "delete_failed", error, seed, target_pre or empty, control_pre or empty, response_summary(delete_response), empty, empty, {"pre": document_pre or empty_evidence}, {}, {}, evaluator._now())

    delete_stores = {item["store"]: item for item in delete_payload.get("stores", []) if isinstance(item, dict) and isinstance(item.get("store"), str)}
    for name, before in (target_pre or {}).items():
        delete_before = delete_stores.get(name, {}).get("before")
        if delete_before != before:
            return TrialResult(trial, "delete_failed", f"delete: {name} pre-count changed from {before} to {delete_before}", seed, target_pre or empty, control_pre or empty, response_summary(delete_response), target_post or empty, empty, {"pre": document_pre or empty_evidence}, {}, {}, evaluator._now())

    document_post_payload, document_post_response = evaluator.threeway_evidence(target_doc_ids)
    document_post, error = parse_document_evidence(document_post_payload, target_doc_ids, expect_present=False)
    document_evidence = {"pre": document_pre or empty_evidence, "post": document_post or empty_evidence}
    if error:
        return TrialResult(trial, "document_postcheck_failed", error, seed, target_pre or empty, control_pre or empty, response_summary(document_post_response), target_post or empty, empty, document_evidence, {}, {}, evaluator._now())

    control_post_payload, control_post_response = evaluator.session_delete(control_session, dry_run=True)
    control_post, error = parse_store_counts(control_post_payload, "pre", require_positive=True)
    if error:
        return TrialResult(trial, "control_postcheck_failed", error, seed, target_pre or empty, control_pre or empty, response_summary(delete_response), target_post or empty, empty, document_evidence, {}, {}, evaluator._now())
    if control_post != control_pre:
        return TrialResult(trial, "control_changed", "non-target control counts changed during target deletion", seed, target_pre or empty, control_pre or empty, response_summary(delete_response), target_post or empty, control_post or empty, document_evidence, {}, {}, evaluator._now())

    probe = evaluator.retrieval_probe(ALLOWED_SESSION_ID, probe_text, target_doc_ids)
    if probe.get("passed") is not True:
        return TrialResult(trial, "retrieval_probe_failed", "target document was returned after deletion", seed, target_pre or empty, control_pre or empty, response_summary(delete_response), target_post or empty, control_post or empty, document_evidence, probe, {}, evaluator._now())

    verification = {
        "passed": True,
        "required_stores": list(REQUIRED_STORES),
        "target_post_count": target_post,
        "control_post_count": control_post,
        "document_evidence": document_evidence,
    }
    return TrialResult(trial, "passed", None, seed, target_pre or empty, control_pre or empty, response_summary(delete_response), target_post or empty, control_post or empty, document_evidence, probe, verification, evaluator._now())


def response_summary(response: APIResponse) -> Dict[str, Any]:
    return {
        "http_status": response.http_status,
        "error_type": response.error_type,
        "error_message": response.error_message,
        "latency_ms": response.latency_ms,
    }


def save_report(evaluator: UnlearningEvaluator, args: argparse.Namespace, trials: List[TrialResult], health_message: str, auth: APIResponse) -> Path:
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%SZ")
    report = {
        "schema_version": 3,
        "execution_mode": "live_session_cascade",
        "timestamp_utc": evaluator._now(),
        "scope": {
            "target_user_id": ALLOWED_USER_ID,
            "target_session_id": ALLOWED_SESSION_ID,
            "control_session_id": args.control_session,
            "collection": args.collection,
            "destructive_authorization": "EVAL_ALLOW_DESTRUCTIVE_UNLEARNING=true",
        },
        "required_sequence": "seed -> pre-count -> delete -> post-count -> retrieval probe -> verify",
        "required_stores": list(REQUIRED_STORES),
        "preflight": {"health": health_message, "authentication": response_summary(auth)},
        "trials": [asdict(trial) for trial in trials],
        "passed": len(trials) == args.trials and all(trial.status == "passed" for trial in trials),
    }
    output = evaluator.results_dir / f"unlearning_{timestamp}.json"
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    return output


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8088")
    parser.add_argument("--collection", default=ALLOWED_COLLECTION)
    parser.add_argument("--control-session", default=os.getenv("EVAL_UNLEARNING_CONTROL_SESSION_ID", ""))
    parser.add_argument("--target-doc-id", action="append", default=[])
    parser.add_argument("--probe-text", default="eval_retrieval_test memory probe")
    parser.add_argument("--seed-command", default=os.getenv("EVAL_UNLEARNING_SEED_COMMAND", ""))
    parser.add_argument("--trials", type=int, default=3)
    args = parser.parse_args()

    configured_user = os.getenv("EVAL_UNLEARNING_USER_ID", os.getenv("EVAL_USER_ID", "")).strip()
    if configured_user != ALLOWED_USER_ID:
        parser.error(f"EVAL_UNLEARNING_USER_ID or EVAL_USER_ID must be exactly {ALLOWED_USER_ID!r}.")
    if args.collection != ALLOWED_COLLECTION:
        parser.error(f"--collection must be exactly {ALLOWED_COLLECTION!r}.")
    if args.control_session.strip() == "" or args.control_session == ALLOWED_SESSION_ID:
        parser.error("--control-session must name a distinct, seeded non-target session.")
    if not args.target_doc_id or any(not doc_id.strip() for doc_id in args.target_doc_id):
        parser.error("provide one or more non-empty --target-doc-id values for the retrieval probe.")
    if not args.seed_command.strip():
        parser.error("--seed-command is required so every trial is independently reseeded.")
    if args.trials != 3:
        parser.error("--trials must be exactly 3 for this evaluation.")
    if os.getenv("EVAL_ALLOW_DESTRUCTIVE_UNLEARNING", "").lower() != "true":
        parser.error("Set EVAL_ALLOW_DESTRUCTIVE_UNLEARNING=true to authorize deletion of the bounded target session.")

    evaluator = UnlearningEvaluator(args.base_url.rstrip("/"))
    healthy, health_message = evaluator.check_service_health()
    if not healthy:
        raise SystemExit(f"[FAIL] Health preflight failed: {health_message}")
    auth = evaluator.authenticate_from_env()
    if not evaluator._jwt_token:
        raise SystemExit(f"[FAIL] Authentication failed: {auth.error_message}")

    trials = [run_trial(evaluator, trial, args.seed_command, args.target_doc_id, args.probe_text, args.control_session) for trial in range(1, args.trials + 1)]
    output = save_report(evaluator, args, trials, health_message, auth)
    passed = all(trial.status == "passed" for trial in trials)
    print(f"[RESULT] {'passed' if passed else 'failed'}")
    print(f"[RAW] {output}")
    if not passed:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
