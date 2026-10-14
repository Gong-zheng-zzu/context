#!/usr/bin/env python3
"""Freeze an RRF ground-truth set only after two independent reviews.

The input CSV is deliberately explicit rather than inferring agreement from
logs. Each query has two reviewer/doc-id pairs. A disagreement additionally
requires a third, independent arbitrator and its final doc id.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
DEFAULT_DRAFT = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_draft.json"
DEFAULT_REVIEW = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_annotation_tasks_100_double.csv"
DEFAULT_OUTPUT = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_reviewed.json"
DEFAULT_CORPUS = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_500.json"

REQUIRED_COLUMNS = (
    "query_id", "reviewer_a", "reviewed_doc_id_a", "reviewed_at_a", "reason_a",
    "reviewer_b", "reviewed_doc_id_b", "reviewed_at_b", "reason_b",
    "arbitrator", "arbitrated_doc_id", "arbitrated_at", "arbitration_reason",
)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def review_row(query_id: str, doc_a: str, doc_b: str, reviewer_a: str, reviewer_b: str, arbitrator: str, arb_doc: str, arb_reason: str) -> dict[str, str]:
    now = "2026-10-14T00:00:00Z"
    return {
        "query_id": query_id, "reviewer_a": reviewer_a, "reviewed_doc_id_a": doc_a,
        "reviewed_at_a": now, "reason_a": "reviewed",
        "reviewer_b": reviewer_b, "reviewed_doc_id_b": doc_b,
        "reviewed_at_b": now, "reason_b": "reviewed",
        "arbitrator": arbitrator, "arbitrated_doc_id": arb_doc,
        "arbitrated_at": now if arbitrator else "", "arbitration_reason": arb_reason,
    }


def _nonempty(row: dict[str, str], key: str, query_id: str) -> str:
    value = row.get(key, "").strip()
    if not value:
        raise ValueError(f"{query_id}: {key} is required")
    return value


def finalize(draft_path: Path, corpus_path: Path, review_path: Path, output_path: Path) -> dict[str, Any]:
    draft = json.loads(draft_path.read_text(encoding="utf-8"))
    queries = draft.get("queries", [])
    if not isinstance(queries, list) or not queries:
        raise ValueError("draft must contain a non-empty queries list")
    corpus = json.loads(corpus_path.read_text(encoding="utf-8"))
    documents = corpus.get("documents", []) if isinstance(corpus, dict) else []
    corpus_ids = {item.get("doc_id") for item in documents if isinstance(item, dict) and item.get("doc_id")}
    if not corpus_ids:
        raise ValueError("corpus must contain document IDs")

    with review_path.open("r", encoding="utf-8-sig", newline="") as handle:
        reader = csv.DictReader(handle)
        missing_columns = set(REQUIRED_COLUMNS) - set(reader.fieldnames or [])
        if missing_columns:
            raise ValueError("review CSV missing columns: " + ", ".join(sorted(missing_columns)))
        rows = {row.get("query_id", "").strip(): row for row in reader}
    query_ids = {query.get("query_id") for query in queries}
    if set(rows) != query_ids:
        raise ValueError("review CSV query IDs do not match the draft")

    finalized = []
    disagreement_count = 0
    reviewers = set()
    for query in queries:
        query_id = query["query_id"]
        row = rows[query_id]
        reviewer_a = _nonempty(row, "reviewer_a", query_id)
        reviewer_b = _nonempty(row, "reviewer_b", query_id)
        if reviewer_a == reviewer_b:
            raise ValueError(f"{query_id}: reviewer_a and reviewer_b must be distinct")
        reviewers.update((reviewer_a, reviewer_b))
        doc_a = _nonempty(row, "reviewed_doc_id_a", query_id)
        doc_b = _nonempty(row, "reviewed_doc_id_b", query_id)
        for doc_key, doc_id in (("reviewed_doc_id_a", doc_a), ("reviewed_doc_id_b", doc_b)):
            if doc_id not in corpus_ids:
                raise ValueError(f"{query_id}: {doc_key} does not exist in corpus")
        if doc_a == doc_b:
            final_doc = doc_a
            resolution = "independent_reviewers_agreed"
        else:
            disagreement_count += 1
            arbitrator = _nonempty(row, "arbitrator", query_id)
            if arbitrator in {reviewer_a, reviewer_b}:
                raise ValueError(f"{query_id}: arbitrator must be independent of both reviewers")
            final_doc = _nonempty(row, "arbitrated_doc_id", query_id)
            if final_doc not in corpus_ids:
                raise ValueError(f"{query_id}: arbitrated_doc_id does not exist in corpus")
            _nonempty(row, "arbitrated_at", query_id)
            _nonempty(row, "arbitration_reason", query_id)
            reviewers.add(arbitrator)
            resolution = "third_party_arbitration"
        query_copy = dict(query)
        query_copy["ground_truth_docs"] = [final_doc]
        query_copy["annotation"] = {"status": resolution, "reviewer_a": reviewer_a, "reviewer_b": reviewer_b, "arbitrator": row.get("arbitrator", "").strip()}
        finalized.append(query_copy)

    metadata = dict(draft.get("metadata", {}))
    metadata.update({
        "dataset_id": "competition_retrieval_queries_v2_double_reviewed",
        "annotation_status": "approved_double_reviewer",
        "report_eligible": True,
        "annotator_count": len(reviewers),
        "independent_reviewer_count": 2,
        "adjudication_status": "completed",
        "disagreement_count": disagreement_count,
        "review_csv_sha256": sha256(review_path),
        "source_corpus_sha256": sha256(corpus_path),
        "frozen_at_utc": datetime.now(timezone.utc).isoformat(),
    })
    payload = {"metadata": metadata, "queries": finalized}
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return payload


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--draft", type=Path, default=DEFAULT_DRAFT)
    parser.add_argument("--corpus", type=Path, default=DEFAULT_CORPUS)
    parser.add_argument("--review-csv", type=Path, default=DEFAULT_REVIEW)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    payload = finalize(args.draft, args.corpus, args.review_csv, args.output)
    print(f"reviewed_queries={args.output}")
    print(f"report_eligible={payload['metadata']['report_eligible']}")
    print(f"sha256={sha256(args.output)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
