#!/usr/bin/env python3
"""Freeze a reviewed competition query set from the single-reviewer CSV."""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict


EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
DEFAULT_DRAFT = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_draft.json"
DEFAULT_REVIEW = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_annotation_tasks_100.csv"
DEFAULT_OUTPUT = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_reviewed.json"
DEFAULT_CORPUS = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_500.json"


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--draft", type=Path, default=DEFAULT_DRAFT)
    parser.add_argument("--review-csv", type=Path, default=DEFAULT_REVIEW)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument("--corpus", type=Path, default=DEFAULT_CORPUS)
    args = parser.parse_args()
    draft = json.loads(args.draft.read_text(encoding="utf-8"))
    queries = draft.get("queries", [])
    if not isinstance(queries, list) or len(queries) != 100:
        raise ValueError("draft must contain exactly 100 queries")
    corpus = json.loads(args.corpus.read_text(encoding="utf-8"))
    corpus_docs = corpus.get("documents", []) if isinstance(corpus, dict) else corpus
    corpus_doc_ids = {item.get("doc_id") for item in corpus_docs if isinstance(item, dict) and item.get("doc_id")}
    if len(corpus_doc_ids) != 500:
        raise ValueError("corpus must contain 500 unique doc_id values")
    with args.review_csv.open("r", encoding="utf-8-sig", newline="") as handle:
        rows = {row["query_id"]: row for row in csv.DictReader(handle)}
    if set(rows) != {query["query_id"] for query in queries}:
        raise ValueError("review CSV query IDs do not match the draft")
    for query in queries:
        row = rows[query["query_id"]]
        if row.get("review_status", "").strip().lower() != "confirmed":
            raise ValueError(f"{query['query_id']}: review_status must be confirmed")
        if not row.get("reviewer", "").strip() or not row.get("reviewed_at", "").strip() or not row.get("review_reason", "").strip():
            raise ValueError(f"{query['query_id']}: reviewer, reviewed_at, and review_reason are required")
        reviewed_doc_id = row.get("reviewed_doc_id", "").strip() or row["proposed_doc_id"].strip()
        if reviewed_doc_id not in corpus_doc_ids:
            raise ValueError(f"{query['query_id']}: reviewed_doc_id must exist in the frozen corpus")
        query["ground_truth_docs"] = [reviewed_doc_id]
        query["annotation"] = {
            "status": "approved_single_reviewer",
            "reviewer": row["reviewer"].strip(),
            "reviewed_at": row["reviewed_at"].strip(),
            "review_reason": row["review_reason"].strip(),
        }
    metadata: Dict[str, Any] = dict(draft.get("metadata", {}))
    metadata.update({
        "dataset_id": "competition_retrieval_queries_v1_reviewed",
        "annotation_status": "approved_single_reviewer",
        "report_eligible": True,
        "review_csv_sha256": sha256(args.review_csv),
        "frozen_at_utc": datetime.now(timezone.utc).isoformat(),
        "single_reviewer_note": "This ground-truth set was reviewed by one annotator; inter-annotator agreement was not measured.",
    })
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"metadata": metadata, "queries": queries}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"reviewed_queries={args.output}")
    print(f"sha256={sha256(args.output)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
