#!/usr/bin/env python3
"""Compare only pre-generation input-security decisions for 12 fixed samples."""

import argparse
import hashlib
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple
from uuid import uuid4

sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import APIResponse, BaseEvaluator


FIXED_SAMPLES: Tuple[Tuple[str, str], ...] = (
    ("prompt_injection", "prompt_injection_001"),
    ("prompt_injection", "prompt_injection_002"),
    ("memory_poisoning", "memory_poisoning_001"),
    ("memory_poisoning", "memory_poisoning_002"),
    ("privilege_escalation", "privilege_escalation_001"),
    ("privilege_escalation", "privilege_escalation_002"),
    ("privacy_leakage", "privacy_leakage_001"),
    ("privacy_leakage", "privacy_leakage_002"),
    ("hallucination_induction", "hallucination_induction_001"),
    ("hallucination_induction", "hallucination_induction_002"),
    ("unlearning_bypass", "unlearning_bypass_001"),
    ("unlearning_bypass", "unlearning_bypass_002"),
)
def sha256_text(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def error_evidence(response: APIResponse, category: str) -> Dict[str, Any]:
    message = response.error_message or ""
    return {
        "status": "error",
        "category": category,
        "http_status": response.http_status,
        "error_type": response.error_type or "invalid_envelope",
        "error_sha256": sha256_text(message),
        "latency_ms": response.latency_ms,
    }


def response_payload(response: APIResponse) -> Optional[Dict[str, Any]]:
    if not response.is_valid_business_response() or not isinstance(response.data, dict):
        return None
    if response.data.get("success") is not True:
        return None
    payload = response.data.get("data", response.data)
    return payload if isinstance(payload, dict) else None


def classify_lightweight(response: APIResponse) -> Dict[str, Any]:
    payload = response_payload(response)
    if payload is None:
        return error_evidence(response, "security_endpoint_error")

    decision = payload.get("decision")
    redacted = payload.get("redacted")
    if decision == "block":
        input_decision = "block"
    elif decision == "redact" or redacted is True:
        input_decision = "redact"
    elif decision == "allow" and isinstance(redacted, bool):
        input_decision = "allow"
    else:
        return error_evidence(response, "security_endpoint_protocol_error")

    return {
        "status": "decision",
        "input_decision": input_decision,
        "http_status": response.http_status,
        "latency_ms": response.latency_ms,
        "stage": payload.get("stage") if isinstance(payload.get("stage"), str) else None,
        "redacted": redacted,
        "sensitive_type_count": len(payload.get("sensitive_types", [])) if isinstance(payload.get("sensitive_types"), list) else 0,
    }


def classify_chat(response: APIResponse) -> Dict[str, Any]:
    payload = response_payload(response)
    if payload is None:
        return error_evidence(response, "chat_input_only_transport_or_service_error")
    if payload.get("generation_skipped") is not True or payload.get("comparison_mode") != "chat_pre_generation_input_chain":
        return error_evidence(response, "chat_input_only_protocol_error")
    decision = payload.get("decision")
    redacted = payload.get("input_redacted")
    if decision == "block":
        input_decision = "block"
    elif decision == "redact" or redacted is True:
        input_decision = "redact"
    elif decision == "allow" and isinstance(redacted, bool):
        input_decision = "allow"
    else:
        return error_evidence(response, "chat_input_only_protocol_error")
    return {
        "status": "decision",
        "input_decision": input_decision,
        "http_status": response.http_status,
        "latency_ms": response.latency_ms,
        "input_redacted": redacted,
        "stage": payload.get("stage") if isinstance(payload.get("stage"), str) else None,
        "pipeline_version": payload.get("pipeline_version") if isinstance(payload.get("pipeline_version"), str) else None,
    }


def load_fixed_samples(dataset_dir: Path) -> Tuple[List[Dict[str, str]], List[Dict[str, Any]]]:
    by_type: Dict[str, Dict[str, Dict[str, Any]]] = {}
    manifest: List[Dict[str, Any]] = []
    for attack_type in sorted({attack_type for attack_type, _ in FIXED_SAMPLES}):
        path = dataset_dir / f"{attack_type}.json"
        raw = path.read_bytes()
        values = json.loads(raw.decode("utf-8"))
        if not isinstance(values, list):
            raise ValueError(f"{path} is not a JSON array")
        by_type[attack_type] = {str(value.get("id")): value for value in values if isinstance(value, dict)}
        manifest.append({"path": str(path), "sha256": hashlib.sha256(raw).hexdigest(), "sample_count": len(values)})

    selected: List[Dict[str, str]] = []
    for attack_type, sample_id in FIXED_SAMPLES:
        sample = by_type[attack_type].get(sample_id)
        if not isinstance(sample, dict) or sample.get("type") != attack_type or not isinstance(sample.get("attack_text"), str):
            raise ValueError(f"fixed sample is missing or invalid: {attack_type}/{sample_id}")
        selected.append({"id": sample_id, "type": attack_type, "attack_text": sample["attack_text"]})
    return selected, manifest


def compare_one(evaluator: BaseEvaluator, sample: Dict[str, str], run_id: str) -> Dict[str, Any]:
    session_id = f"security-input-chain-{run_id}-{sample['id']}"
    headers = evaluator.get_auth_headers()
    security_response = evaluator.call_api("POST", "/api/v1/security/evaluate-input", {
        "session_id": session_id,
        "message": sample["attack_text"],
        "input": sample["attack_text"],
        "sample_id": sample["id"],
        "attack_type": sample["type"],
        "name": "security_input_chain_consistency",
    }, headers)
    chat_response = evaluator.call_api("POST", "/api/chat", {
        "session_id": session_id,
        "message": sample["attack_text"],
        "evaluationInputOnly": True,
        "sample_id": sample["id"],
        "attack_type": sample["type"],
        "name": "security_input_chain_consistency",
    }, headers)

    lightweight = classify_lightweight(security_response)
    chat = classify_chat(chat_response)
    if lightweight["status"] == "decision" and chat["status"] == "decision":
        comparison = "consistent" if lightweight["input_decision"] == chat["input_decision"] else "mismatch"
    else:
        comparison = "inconclusive_error"

    return {
        "sample_id": sample["id"],
        "attack_type": sample["type"],
        "input_sha256": sha256_text(sample["attack_text"]),
        "session_id_sha256": sha256_text(session_id),
        "lightweight_endpoint": lightweight,
        "chat_endpoint": chat,
        "comparison": comparison,
    }


def run_self_test() -> None:
    block = classify_chat(APIResponse(True, 200, {"success": True, "data": {
        "decision": "block", "input_redacted": False, "generation_skipped": True,
        "comparison_mode": "chat_pre_generation_input_chain",
    }}, None, None, 1.0))
    assert block["input_decision"] == "block"
    assert classify_chat(APIResponse(False, 401, None, "http_error", "unauthorized", 1.0))["status"] == "error"
    assert classify_lightweight(APIResponse(True, 200, {"success": True, "data": {"decision": "redact", "redacted": True}}, None, None, 1.0))["input_decision"] == "redact"
    print(json.dumps({"validation_mode": "pure_classifier", "passed": 3}, indent=2))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8088")
    parser.add_argument("--timeout", type=int, default=120)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        run_self_test()
        return

    project_root = Path(__file__).resolve().parent.parent
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%SZ")
    output_path = args.output or project_root / "results" / "raw" / f"security_input_chain_{timestamp}.json"
    output_path.parent.mkdir(parents=True, exist_ok=True)
    run_id = uuid4().hex
    evidence: Dict[str, Any] = {
        "schema_version": 1,
        "execution_mode": "live_api",
        "started_at": utc_now(),
        "run_id": run_id,
        "comparison_mode": "chat_pre_generation_input_chain",
        "contract": {
            "security_endpoint": "/api/v1/security/evaluate-input",
            "chat_endpoint": "/api/chat",
            "comparison_mode": "chat_pre_generation_input_chain",
            "comparison_scope": "pre-generation input security decision only: block, redact, or allow",
            "excluded": "full chat text, LLM generation, retrieval behavior, memory effects, and output filtering",
        },
        "authentication": {},
        "service": {},
        "dataset_manifest": [],
        "results": [],
    }

    try:
        samples, manifest = load_fixed_samples(project_root / "datasets" / "attack_samples")
        evidence["dataset_manifest"] = manifest
        evaluator = BaseEvaluator(args.base_url.rstrip("/"))
        evaluator.default_timeout = args.timeout
        auth = evaluator.authenticate_from_env()
        if not evaluator.get_auth_headers():
            evidence["authentication"] = error_evidence(auth, "authentication_error")
        else:
            evidence["authentication"] = {"status": "authenticated", "http_status": auth.http_status, "latency_ms": auth.latency_ms}
            healthy, health_message = evaluator.check_service_health()
            if not healthy:
                evidence["service"] = {"status": "error", "category": "health_check_error", "error_sha256": sha256_text(health_message)}
            else:
                evidence["service"] = {"status": "healthy"}
                evidence["results"] = [compare_one(evaluator, sample, run_id) for sample in samples]
    except Exception as error:
        evidence["service"] = {"status": "error", "category": "runner_error", "error_sha256": sha256_text(str(error))}

    comparisons = [row["comparison"] for row in evidence["results"]]
    errors = sum(item == "inconclusive_error" for item in comparisons)
    mismatches = sum(item == "mismatch" for item in comparisons)
    consistent = sum(item == "consistent" for item in comparisons)
    if evidence["authentication"].get("status") != "authenticated" or evidence["service"].get("status") != "healthy" or errors:
        overall = "inconclusive_due_to_auth_or_service_error"
    elif mismatches:
        overall = "mismatch_detected"
    elif consistent == len(FIXED_SAMPLES):
        overall = "consistent_for_all_fixed_samples"
    else:
        overall = "inconclusive"
    evidence["summary"] = {
        "fixed_sample_count": len(FIXED_SAMPLES),
        "consistent_count": consistent,
        "mismatch_count": mismatches,
        "inconclusive_error_count": errors,
        "overall_input_decision_consistency": overall,
    }
    evidence["ended_at"] = utc_now()
    output_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"[SAVE] {output_path}")
    print(json.dumps(evidence["summary"], ensure_ascii=False))
    if overall == "inconclusive_due_to_auth_or_service_error":
        raise SystemExit(2)


if __name__ == "__main__":
    main()
