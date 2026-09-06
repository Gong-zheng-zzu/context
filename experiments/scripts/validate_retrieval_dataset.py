#!/usr/bin/env python3
"""Read-only validation of a retrieval corpus and query ground-truth set."""

import argparse
import json
import sys
from pathlib import Path
from typing import Dict, Set, Tuple

sys.path.insert(0, str(Path(__file__).resolve().parent))
from experiment_integrity import IntegrityConfigurationError, dual_digest_file


def load(path: Path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot load {path}: {error}") from error


def corpus_ids(data) -> Set[str]:
    records = data.get("documents", data.get("records", [])) if isinstance(data, dict) else data
    if not isinstance(records, list):
        return set()
    return {item.get("doc_id") or item.get("id") for item in records if isinstance(item, dict) and (item.get("doc_id") or item.get("id"))}


def queries_from(data):
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        return data.get("queries", data.get("query_answer_pairs", []))
    return []


def parse_counts(value: str) -> Dict[str, int]:
    counts = {}
    for item in value.split(","):
        name, count = item.split("=", 1)
        counts[name.strip()] = int(count)
    return counts


def validate(args) -> Tuple[bool, Dict]:
    corpus_path = args.experiments_dir / args.corpus
    queries_path = args.experiments_dir / args.queries
    corpus = load(corpus_path)
    query_data = load(queries_path)
    doc_ids = corpus_ids(corpus)
    queries = queries_from(query_data)
    expected_types = parse_counts(args.expected_type_counts)
    type_counts = {name: 0 for name in expected_types}
    missing = []
    duplicate_query_ids = set()
    query_ids = set()
    for query in queries:
        if not isinstance(query, dict):
            missing.append("invalid_query_object")
            continue
        query_id = query.get("query_id")
        if query_id in query_ids:
            duplicate_query_ids.add(query_id)
        query_ids.add(query_id)
        query_type = query.get("query_type")
        if query_type in type_counts:
            type_counts[query_type] += 1
        for expected_doc in query.get("ground_truth_docs", []):
            if "*" in expected_doc:
                if not any(doc_id.startswith(expected_doc.replace("*", "")) for doc_id in doc_ids):
                    missing.append(expected_doc)
            elif expected_doc not in doc_ids:
                missing.append(expected_doc)
    metadata = query_data.get("metadata", {}) if isinstance(query_data, dict) else {}
    reviewed = metadata.get("annotation_status") == "approved_single_reviewer" and metadata.get("report_eligible") is True
    passed = (
        len(doc_ids) == args.expected_corpus_count
        and len(queries) == args.expected_query_count
        and type_counts == expected_types
        and not missing
        and not duplicate_query_ids
        and (not args.require_reviewed or reviewed)
    )
    report = {
        "hash_algorithms": ["SHA-256", "SM3"],
        "corpus": {"path": str(corpus_path), "sha256": dual_digest_file(corpus_path)["sha256"], "dual_digest": dual_digest_file(corpus_path), "count": len(doc_ids), "expected": args.expected_corpus_count},
        "queries": {"path": str(queries_path), "sha256": dual_digest_file(queries_path)["sha256"], "dual_digest": dual_digest_file(queries_path), "count": len(queries), "expected": args.expected_query_count, "type_counts": type_counts, "expected_type_counts": expected_types, "annotation_status": metadata.get("annotation_status", "not_recorded"), "report_eligible": metadata.get("report_eligible"), "reviewed": reviewed},
        "missing_doc_ids": sorted(set(missing))[:20],
        "duplicate_query_ids": sorted(item for item in duplicate_query_ids if item),
        "passed": passed,
    }
    return passed, report


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", default="datasets/retrieval_corpus/retrieval_corpus_30.json")
    parser.add_argument("--queries", default="datasets/retrieval_groundtruth/query_answer_pairs.json")
    parser.add_argument("--expected-corpus-count", type=int, default=30)
    parser.add_argument("--expected-query-count", type=int, default=50)
    parser.add_argument("--expected-type-counts", default="temporal=17,causal=17,general=16")
    parser.add_argument("--require-reviewed", action="store_true")
    parser.add_argument("--json-output", type=Path)
    args = parser.parse_args()
    args.experiments_dir = Path(__file__).resolve().parent.parent
    try:
        passed, report = validate(args)
    except (ValueError, OSError, IntegrityConfigurationError) as error:
        print(f"FAIL: {error}")
        return 1
    print(json.dumps(report, ensure_ascii=False, indent=2))
    if args.json_output:
        args.json_output.parent.mkdir(parents=True, exist_ok=True)
        args.json_output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0 if passed else 1


if __name__ == "__main__":
    sys.exit(main())
