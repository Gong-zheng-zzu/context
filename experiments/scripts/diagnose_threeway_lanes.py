#!/usr/bin/env python3
"""Read-only diagnosis of the structured three-way retrieval lanes.

Why this exists
---------------
A measured retrieval comparison showed the weighted RRF path scoring *worse*
than the plain vector baseline (MRR 0.346 -> 0.220). RRF only mixes sources it
is told about with weights, so the immediate suspicion is that the knowledge
and timeline lanes carry no persisted documents for the evaluated corpus: the
fusion would then rank on near-empty inputs while still diluting the vector
ordering.

`POST /api/v1/experiments/threeway/evidence` answers exactly that question. It
is deliberately read-only -- the handler neither ingests nor mutates vectors,
timeline rows, graph nodes or sessions -- and it is scoped to the audited
evaluation identity (`eval_user_001` / `eval_retrieval_test`).

This script only reads. It never writes a session vector or a result artifact,
so it is safe to run while other evaluations are idle and it does not need the
evidence gate.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Tuple

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from base_evaluator import BaseEvaluator  # noqa: E402

REPOSITORY_ROOT = SCRIPT_DIR.parent.parent
DEFAULT_CORPUS = REPOSITORY_ROOT / "experiments" / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json"

EVIDENCE_ENDPOINT = "/api/v1/experiments/threeway/evidence"
ALLOWED_USER_ID = "eval_user_001"
ALLOWED_SESSION_ID = "eval_retrieval_test"
LANES = ("vector", "timeline", "graph")


class ThreeWayDiagnosisError(RuntimeError):
    """Raised when the diagnosis cannot produce a trustworthy answer."""


def load_corpus_doc_ids(corpus_file: Path) -> Tuple[List[str], str]:
    """Return the corpus doc_ids in file order.

    The corpus is a JSON array of documents that must each declare a stable
    ``doc_id``; anything else would make the lane counts non-comparable.
    """
    try:
        raw = corpus_file.read_bytes()
        payload = json.loads(raw.decode("utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ThreeWayDiagnosisError(f"Cannot read corpus {corpus_file}: {exc}") from exc

    # The repository corpus is {"metadata": ..., "documents": [...]}; accept a
    # bare array too so a trimmed fixture stays usable.
    if isinstance(payload, dict):
        documents = payload.get("documents")
    else:
        documents = payload

    if not isinstance(documents, list) or not documents:
        raise ThreeWayDiagnosisError(
            f"{corpus_file.name} must contain a non-empty 'documents' array (or be a non-empty JSON array)."
        )

    doc_ids: List[str] = []
    for index, document in enumerate(documents):
        if not isinstance(document, dict):
            raise ThreeWayDiagnosisError(f"{corpus_file.name}[{index}] is not a JSON object.")
        doc_id = document.get("doc_id")
        if not isinstance(doc_id, str) or not doc_id.strip():
            raise ThreeWayDiagnosisError(f"{corpus_file.name}[{index}] has no usable 'doc_id'.")
        doc_ids.append(doc_id.strip())

    if len(set(doc_ids)) != len(doc_ids):
        raise ThreeWayDiagnosisError(f"{corpus_file.name} contains duplicate doc_id values.")
    return doc_ids, corpus_file.name


def summarise_lanes(documents: List[Dict[str, Any]]) -> Dict[str, Dict[str, Any]]:
    """Aggregate per-lane verification counts across documents."""
    summary: Dict[str, Dict[str, Any]] = {
        lane: {"present": 0, "verified": 0, "errors": 0, "statuses": {}} for lane in LANES
    }
    for document in documents:
        for lane in LANES:
            evidence = document.get(lane) or {}
            count = evidence.get("count")
            if isinstance(count, int) and count > 0:
                summary[lane]["present"] += 1
            if evidence.get("verified") is True:
                summary[lane]["verified"] += 1
            if evidence.get("error"):
                summary[lane]["errors"] += 1
            status = str(evidence.get("status") or "unknown")
            summary[lane]["statuses"][status] = summary[lane]["statuses"].get(status, 0) + 1
    return summary


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Read-only check of whether the knowledge/timeline lanes are persisted for the evaluation corpus."
    )
    parser.add_argument("--base-url", default="http://127.0.0.1:8088", help="API base URL.")
    parser.add_argument("--corpus-file", type=Path, default=DEFAULT_CORPUS, help="Corpus JSON used for the evaluated run.")
    parser.add_argument("--user-id", default=ALLOWED_USER_ID, help="Evaluation identity (must match the server scope).")
    parser.add_argument("--session-id", default=ALLOWED_SESSION_ID, help="Evaluation session (must match the server scope).")
    parser.add_argument("--json-out", type=Path, default=None, help="Optional path to also write the raw evidence JSON.")
    args = parser.parse_args(argv)

    if args.user_id != ALLOWED_USER_ID:
        print(f"ERROR: --user-id must be {ALLOWED_USER_ID} (the server scopes this endpoint).", file=sys.stderr)
        return 2
    if args.session_id != ALLOWED_SESSION_ID:
        print(f"ERROR: --session-id must be {ALLOWED_SESSION_ID} (the server scopes this endpoint).", file=sys.stderr)
        return 2

    try:
        doc_ids, corpus_name = load_corpus_doc_ids(args.corpus_file)
    except ThreeWayDiagnosisError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    evaluator = BaseEvaluator(args.base_url)

    auth_response = evaluator.authenticate_from_env()
    if not evaluator._jwt_token:
        print("ERROR: authentication failed; set EVAL_USER_ID=eval_user_001 and EVAL_PASSWORD.", file=sys.stderr)
        print(f"       server said: {auth_response.error_message}", file=sys.stderr)
        return 3

    print(f"[OK] Authenticated as {args.user_id}.")
    print(f"[OK] Corpus {corpus_name} -> {len(doc_ids)} documents.")

    response = evaluator.call_api(
        "POST",
        EVIDENCE_ENDPOINT,
        {"user_id": args.user_id, "session_id": args.session_id, "doc_ids": doc_ids},
        evaluator.get_auth_headers(),
    )

    if not response.is_valid_business_response():
        print(f"ERROR: evidence endpoint failed (HTTP {response.http_status}): {response.error_message}", file=sys.stderr)
        return 3

    data = response.data if isinstance(response.data, dict) else {}
    # The transport may hand back the APIResponse envelope verbatim
    # ({"success":..., "data": {...}}) or only its payload; accept both.
    inner = data.get("data") if isinstance(data.get("data"), dict) else data
    documents = inner.get("documents")
    if not isinstance(documents, list) or not documents:
        print(f"ERROR: evidence response carried no documents: {json.dumps(data, ensure_ascii=False)[:400]}", file=sys.stderr)
        return 3

    summary = summarise_lanes(documents)
    total = len(documents)

    print()
    print("Three-way lane persistence (per document)")
    print(f"  documents checked: {total}")
    for lane in LANES:
        entry = summary[lane]
        statuses = ", ".join(f"{name}={count}" for name, count in sorted(entry["statuses"].items()))
        print(
            f"  {lane:<9} count>0: {entry['present']}/{total}  verified: {entry['verified']}/{total}  "
            f"errors: {entry['errors']}  statuses: {statuses}"
        )

    print()
    knowledge_present = summary["graph"]["present"]
    timeline_present = summary["timeline"]["present"]
    vector_present = summary["vector"]["present"]

    if vector_present == 0:
        verdict = "VECTOR_LANE_EMPTY: the vector lane itself has no rows; re-seed before any comparison."
    elif knowledge_present == 0 and timeline_present == 0:
        verdict = (
            "BRANCH_A (both extra lanes empty): RRF was fed empty knowledge/timeline inputs while still "
            "weighting them, so the fused order diluted the vector ranking. Fix = seed the three lanes, then re-compare."
        )
    elif knowledge_present == 0 or timeline_present == 0:
        verdict = (
            "BRANCH_A_PARTIAL (one extra lane empty): a partially populated fusion still weighted an empty lane. "
            "Fix = seed the missing lane, then re-compare."
        )
    else:
        verdict = (
            "BRANCH_B (all three lanes populated): the negative result is not caused by missing data. "
            "Fix = compare RRF_SOURCE_WEIGHTS variants without changing code."
        )

    print("VERDICT")
    print(f"  {verdict}")

    if args.json_out:
        args.json_out.parent.mkdir(parents=True, exist_ok=True)
        args.json_out.write_text(
            json.dumps(
                {"user_id": args.user_id, "session_id": args.session_id, "documents": documents, "summary": summary},
                ensure_ascii=False,
                indent=2,
            )
            + "\n",
            encoding="utf-8",
        )
        print(f"  raw evidence -> {args.json_out}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
