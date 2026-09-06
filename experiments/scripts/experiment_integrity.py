"""Dual-digest and optional external audit-signature helpers for experiments.

SHA-256 is always available through the Python standard library. SM3 uses the
runtime OpenSSL provider when available and falls back to the optional
``gmssl`` package. This module never substitutes another algorithm for SM3.
Set ``EVAL_REQUIRE_SM3=true`` to make a missing SM3 provider a configuration
error.

An external signer is deliberately opt-in. ``EVAL_AUDIT_SIGNER_CMD`` must name
a command that reads the JSON signing request from stdin and emits a JSON
object with ``signature``, ``public_key_id`` and
``verification_status=verified`` on stdout.
"""

from __future__ import annotations

import hashlib
import json
import os
import shlex
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict


class IntegrityConfigurationError(RuntimeError):
    """Raised when an explicitly required integrity capability is unavailable."""


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def _sm3(value: bytes) -> Dict[str, str]:
    try:
        digest = hashlib.new("sm3")
        digest.update(value)
        return {"status": "available", "provider": "hashlib_openssl", "value": digest.hexdigest()}
    except ValueError:
        pass
    try:
        from gmssl import sm3, func  # type: ignore[import-not-found]
    except ImportError as error:
        message = "configuration_error: Python runtime needs OpenSSL SM3 support or optional gmssl"
        if os.getenv("EVAL_REQUIRE_SM3", "").strip().lower() in {"1", "true", "yes"}:
            raise IntegrityConfigurationError(message) from error
        return {"status": "unavailable", "error": message}
    return {"status": "available", "provider": "gmssl", "value": sm3.sm3_hash(func.bytes_to_list(value))}


def dual_digest_bytes(value: bytes) -> Dict[str, Any]:
    """Return a truthful SHA-256/SM3 record without faking unavailable SM3."""
    return {
        "hash_algorithms": ["SHA-256", "SM3"],
        "sha256": hashlib.sha256(value).hexdigest(),
        "sm3": _sm3(value),
        "generated_at": utc_now(),
    }


def dual_digest_file(path: Path) -> Dict[str, Any]:
    return dual_digest_bytes(path.read_bytes())


def audit_signature(payload: Dict[str, Any]) -> Dict[str, Any]:
    """Call an explicitly configured external signer, otherwise record no claim."""
    command = os.getenv("EVAL_AUDIT_SIGNER_CMD", "").strip()
    request = {"schema": "experiment-audit-signing-request-v1", "payload": payload, "payload_digest": dual_digest_bytes(canonical_json(payload))}
    if not command:
        return {
            "status": "unsigned_not_eligible_for_signed_claim",
            "signing_requested_at": utc_now(),
            "payload_digest": request["payload_digest"],
        }
    try:
        completed = subprocess.run(
            shlex.split(command), input=json.dumps(request, ensure_ascii=False), text=True,
            capture_output=True, check=False, timeout=30,
        )
        if completed.returncode != 0:
            return {"status": "signing_failed", "error": f"external signer exited {completed.returncode}", "payload_digest": request["payload_digest"]}
        response = json.loads(completed.stdout)
        if not isinstance(response, dict) or not response.get("signature") or not response.get("public_key_id") or response.get("verification_status") != "verified":
            return {"status": "signing_failed", "error": "external signer response lacks verified SM2 signature metadata", "payload_digest": request["payload_digest"]}
        return {"status": "signed", "algorithm": "SM2-with-SM3", "signature": response["signature"], "public_key_id": response["public_key_id"], "verification_status": "verified", "payload_digest": request["payload_digest"], "signed_at": utc_now()}
    except (OSError, subprocess.TimeoutExpired, ValueError, json.JSONDecodeError) as error:
        return {"status": "signing_failed", "error": str(error), "payload_digest": request["payload_digest"]}
