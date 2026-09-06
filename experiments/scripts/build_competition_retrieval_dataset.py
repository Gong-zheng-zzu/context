#!/usr/bin/env python3
"""Build the 500-document competition retrieval corpus and review queue.

The generated query file is deliberately a review draft. It cannot be used as
report-eligible ground truth until a reviewer completes the companion CSV and
the finalization tool records an approved single-reviewer annotation status.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import random
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List


EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
SOURCE_FILE = EXPERIMENTS_DIR / "datasets" / "nursing_data" / "nursing_records.json"
DEFAULT_CORPUS = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_500.json"
DEFAULT_QUERIES = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_draft.json"
DEFAULT_REVIEW = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_annotation_tasks_100.csv"
EVENT_TYPES = ("pain", "mood", "vital_signs", "fall", "sleep", "medication", "meal", "confusion")
QUERY_COUNTS = {"temporal": 34, "causal": 33, "general": 33}


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def doc_id(record: Dict[str, Any]) -> str:
    return f"{record['elder_id']}_{record['timestamp']}"


def document(record: Dict[str, Any]) -> Dict[str, Any]:
    metadata = {
        "patient_id": record["elder_id"],
        "patient_name": record.get("elder_name", ""),
        "timestamp": record["timestamp"],
        "type": "health_record",
        "event_type": record.get("event_type", ""),
        "severity": record.get("severity", ""),
        "tags": record.get("tags", []),
        "raw_source_record_sha256": hashlib.sha256(canonical_json(record)).hexdigest(),
    }
    for field in ("structured_data", "causal_factors", "causal_relations"):
        if record.get(field):
            metadata[field] = record[field]
    return {
        "doc_id": doc_id(record),
        "content": (
            f"患者编号: {record['elder_id']}\n患者姓名: {record.get('elder_name', '')}\n"
            f"记录时间: {record['timestamp']}\n事件类型: {record.get('event_type', '')}\n"
            f"严重程度: {record.get('severity', '')}\n护理记录: {record['content']}"
        ),
        "metadata": metadata,
    }


def stratified_select(records: List[Dict[str, Any]], count: int, seed: int) -> List[Dict[str, Any]]:
    if count < len(EVENT_TYPES) * 2:
        raise ValueError("count is too small to represent all event types")
    grouped: Dict[str, Dict[str, List[Dict[str, Any]]]] = defaultdict(lambda: defaultdict(list))
    for record in records:
        event_type = record.get("event_type")
        patient_id = record.get("elder_id")
        if event_type in EVENT_TYPES and patient_id:
            grouped[event_type][patient_id].append(record)

    rng = random.Random(seed)
    for by_patient in grouped.values():
        for bucket in by_patient.values():
            rng.shuffle(bucket)

    selected: List[Dict[str, Any]] = []
    seen = set()
    patients = sorted({record["elder_id"] for record in records})
    rng.shuffle(patients)
    # First pass guarantees all patients appear, while rotating event types.
    for patient_index, patient_id in enumerate(patients):
        for offset in range(len(EVENT_TYPES)):
            event_type = EVENT_TYPES[(patient_index + offset) % len(EVENT_TYPES)]
            bucket = grouped[event_type].get(patient_id, [])
            candidate = next((item for item in bucket if doc_id(item) not in seen), None)
            if candidate:
                selected.append(candidate)
                seen.add(doc_id(candidate))
                break
        if len(selected) >= count:
            return selected[:count]

    # Second pass balances the eight event types to the requested corpus size.
    target_per_event = count // len(EVENT_TYPES)
    remainder = count % len(EVENT_TYPES)
    desired = {event: target_per_event + int(index < remainder) for index, event in enumerate(EVENT_TYPES)}
    by_event_selected = defaultdict(int)
    for item in selected:
        by_event_selected[item["event_type"]] += 1

    while len(selected) < count:
        progressed = False
        for event_type in EVENT_TYPES:
            if by_event_selected[event_type] >= desired[event_type]:
                continue
            for patient_id in patients:
                candidate = next((item for item in grouped[event_type].get(patient_id, []) if doc_id(item) not in seen), None)
                if candidate:
                    selected.append(candidate)
                    seen.add(doc_id(candidate))
                    by_event_selected[event_type] += 1
                    progressed = True
                    break
            if len(selected) >= count:
                break
        if not progressed:
            raise ValueError("source data cannot satisfy the requested stratified corpus")
    return selected


def query_text(record: Dict[str, Any], query_type: str) -> tuple[str, List[str], str]:
    name = record.get("elder_name") or record["elder_id"]
    event = record.get("event_type", "护理")
    timestamp = str(record["timestamp"])[:10]
    tags = record.get("tags", [])
    keyword = tags[0] if tags else event
    if query_type == "temporal":
        return f"请查询{name}在{timestamp}前后的{event}护理记录与变化。", [name, timestamp, event], "按患者、日期和事件类型核验是否为相关记录。"
    if query_type == "causal":
        factor = (record.get("causal_factors") or [keyword])[0]
        return f"{name}在{timestamp}发生{event}与哪些护理因素有关？", [name, event, factor], "核验记录是否包含该患者同日事件及对应诱因或护理因素。"
    return f"{name}本次{event}护理记录的主要情况和处置是什么？", [name, event, keyword], "核验记录是否描述该患者的目标事件和护理处置。"


def build_queries(records: Iterable[Dict[str, Any]], seed: int) -> List[Dict[str, Any]]:
    by_type: Dict[str, List[Dict[str, Any]]] = defaultdict(list)
    for record in records:
        if record.get("causal_factors") or record.get("causal_relations"):
            by_type["causal"].append(record)
        by_type["temporal"].append(record)
        by_type["general"].append(record)

    rng = random.Random(seed + 1)
    queries: List[Dict[str, Any]] = []
    for query_type, expected in QUERY_COUNTS.items():
        candidates = sorted(by_type[query_type], key=lambda item: (item["elder_id"], item["timestamp"]))
        rng.shuffle(candidates)
        patient_seen = set()
        selected = []
        for record in candidates:
            if record["elder_id"] not in patient_seen:
                selected.append(record)
                patient_seen.add(record["elder_id"])
            if len(selected) == expected:
                break
        if len(selected) < expected:
            selected.extend(candidates[len(selected):expected])
        if len(selected) != expected:
            raise ValueError(f"not enough {query_type} records for {expected} queries")
        for index, record in enumerate(selected, 1):
            text, keywords, reason = query_text(record, query_type)
            queries.append({
                "query_id": f"competition_{query_type}_{index:03d}",
                "query_type": query_type,
                "question": text,
                "ground_truth_docs": [doc_id(record)],
                "expected_keywords": keywords,
                "annotation": {
                    "status": "pending_single_reviewer",
                    "proposed_relevance": 1,
                    "proposed_reason": reason,
                    "reviewer": "",
                    "reviewed_at": "",
                },
            })
    return queries


def write_json(path: Path, payload: Dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_review_csv(path: Path, queries: List[Dict[str, Any]], source_sha: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=[
            "query_id", "query_type", "question", "proposed_doc_id", "proposed_relevance",
            "proposed_reason", "review_status", "reviewed_doc_id", "reviewer", "reviewed_at", "review_reason", "source_sha256",
        ])
        writer.writeheader()
        for query in queries:
            annotation = query["annotation"]
            writer.writerow({
                "query_id": query["query_id"],
                "query_type": query["query_type"],
                "question": query["question"],
                "proposed_doc_id": query["ground_truth_docs"][0],
                "proposed_relevance": annotation["proposed_relevance"],
                "proposed_reason": annotation["proposed_reason"],
                "review_status": "pending",
                "reviewed_doc_id": "",
                "reviewer": "",
                "reviewed_at": "",
                "review_reason": "",
                "source_sha256": source_sha,
            })


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--count", type=int, default=500)
    parser.add_argument("--seed", type=int, default=20260722)
    parser.add_argument("--corpus-output", type=Path, default=DEFAULT_CORPUS)
    parser.add_argument("--queries-output", type=Path, default=DEFAULT_QUERIES)
    parser.add_argument("--review-output", type=Path, default=DEFAULT_REVIEW)
    args = parser.parse_args()

    records = json.loads(SOURCE_FILE.read_text(encoding="utf-8"))
    if not isinstance(records, list) or len(records) < args.count:
        raise ValueError("nursing source data is insufficient")
    selected = stratified_select(records, args.count, args.seed)
    documents = [document(record) for record in selected]
    source_sha = sha256_file(SOURCE_FILE)
    event_counts = {event: sum(record["event_type"] == event for record in selected) for event in EVENT_TYPES}
    patient_count = len({record["elder_id"] for record in selected})
    if len(documents) != args.count or len({item["doc_id"] for item in documents}) != args.count:
        raise ValueError("generated corpus does not have unique document identifiers")
    if patient_count != 100 or any(count == 0 for count in event_counts.values()):
        raise ValueError("generated corpus does not preserve patient and event-type coverage")

    corpus = {
        "metadata": {
            "dataset_id": "competition_retrieval_corpus_v1",
            "dataset_status": "frozen_candidate_corpus",
            "generated_at_utc": datetime.now(timezone.utc).isoformat(),
            "source_file": "datasets/nursing_data/nursing_records.json",
            "source_record_count": len(records),
            "source_sha256": source_sha,
            "selected_document_count": len(documents),
            "patient_count": patient_count,
            "event_type_counts": event_counts,
            "selection_seed": args.seed,
            "selection_method": "deterministic patient-first and event-balanced stratified sample",
        },
        "documents": documents,
    }
    write_json(args.corpus_output, corpus)
    queries = build_queries(selected, args.seed)
    query_payload = {
        "metadata": {
            "dataset_id": "competition_retrieval_queries_v1_draft",
            "annotation_status": "pending_single_reviewer",
            "report_eligible": False,
            "generated_at_utc": datetime.now(timezone.utc).isoformat(),
            "source_corpus_file": args.corpus_output.name,
            "source_corpus_sha256": sha256_file(args.corpus_output),
            "total_queries": len(queries),
            "breakdown": QUERY_COUNTS,
            "annotation_instruction": "Complete the companion CSV and finalize it before using this file for formal metrics.",
        },
        "queries": queries,
    }
    write_json(args.queries_output, query_payload)
    write_review_csv(args.review_output, queries, query_payload["metadata"]["source_corpus_sha256"])
    print(f"corpus={args.corpus_output}")
    print(f"queries_draft={args.queries_output}")
    print(f"review_queue={args.review_output}")
    print(f"documents={len(documents)} patients={patient_count} queries={len(queries)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
