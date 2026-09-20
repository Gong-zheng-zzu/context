#!/usr/bin/env python3
"""Build the competition retrieval corpus plus its query set and review queue.

语料与查询**一起生成**，因此每条查询的真值文档必然存在于语料内（闭合性由
``source_corpus_sha256`` 绑定）。查询文本由目标文档构造、真值直接取该文档的
doc_id，所以相关性是**定义性**的（由构造得出），而不是人工相关性判定。

这带来一个必须如实声明的边界：这套查询集**不是**人工标注的相关性集。脚本据此
提供两种标注模式：

* ``review_draft``（默认）—— 标注状态为 ``pending_single_reviewer``，配套 CSV 供
  评审人逐条确认，确认后才可能升级为可报告数据集。这是既有 100 条草稿集的口径。
* ``auto_derived`` —— 标注状态为 ``auto_derived_unreviewed``，显式声明标签由机器
  推导、未经人工复核。它**不得**被当作已复核数据使用；``retrieval_eval.py`` 对这两种
  状态走两个互斥的门禁分支。

无论哪种模式都会产出待复核 CSV，作为升级为可报告数据集的通道。
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
from typing import Any, Dict, Iterable, List, Optional, Sequence


EXPERIMENTS_DIR = Path(__file__).resolve().parent.parent
SOURCE_FILE = EXPERIMENTS_DIR / "datasets" / "nursing_data" / "nursing_records.json"
DEFAULT_CORPUS = EXPERIMENTS_DIR / "datasets" / "retrieval_corpus" / "retrieval_corpus_500.json"
DEFAULT_QUERIES = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_queries_100_draft.json"
DEFAULT_REVIEW = EXPERIMENTS_DIR / "datasets" / "retrieval_groundtruth" / "competition_annotation_tasks_100.csv"
EVENT_TYPES = ("pain", "mood", "vital_signs", "fall", "sleep", "medication", "meal", "confusion")

# 既有 100 条草稿集的分类配额。--per-type-queries 为 0 时沿用该配额，使历史产物可复现。
LEGACY_QUERY_COUNTS = {"temporal": 34, "causal": 33, "general": 33}
QUERY_TYPES = ("temporal", "causal", "general")

REVIEW_DRAFT_STATUS = "pending_single_reviewer"
AUTO_DERIVED_STATUS = "auto_derived_unreviewed"
ANNOTATION_MODES = ("review_draft", "auto_derived")

# 相关性来源的自描述。两种模式共用同一推导方式，差别只在是否已完成人工复核。
DERIVATION_METHOD = "query_constructed_from_source_document"
AUTO_DERIVED_CAVEAT = (
    "Relevance is definitional: each query is generated from its ground-truth "
    "document. No human relevance judgement was performed, and the label was not "
    "reviewed. Do not describe metrics from this set as human-verified."
)

# 语料必须为每位患者保留一篇带因果字段的文档，否则 causal 查询无法覆盖全部患者
# （实测既有 500 语料仅 63 篇因果文档、覆盖 16 个患者，而 build_queries 按患者去重，
# 于是 causal 查询上限只有 16 条）。该覆盖是查询集能达到声明配额的前提。
CAUSAL_FIELDS = ("causal_factors", "causal_relations")


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


def has_causal_fields(record: Dict[str, Any]) -> bool:
    """记录是否带因果字段——判定与 ``build_queries`` 的 causal 入池条件一致。"""
    return any(record.get(field) for field in CAUSAL_FIELDS)


def stratified_select(
    records: List[Dict[str, Any]],
    count: int,
    seed: int,
    require_causal_coverage: bool = False,
) -> List[Dict[str, Any]]:
    """按患者与事件类型分层选样，可选地保证每位患者都有一篇因果文档。

    ``require_causal_coverage`` 的理由：``build_queries`` 对查询按患者去重，因此
    每种查询类型的配额上限等于「拥有该类型候选文档的患者数」。若语料中大量患者没有
    因果文档，causal 配额就无法达成，只能靠兜底重复患者来凑数——那会让查询集与其真值
    的关系变得不透明。开启该选项后，抽样会为每位患者保留一篇因果文档。
    """
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

    # 第 0 趟：为每位患者保留一篇因果文档。只有开启覆盖要求时才执行，且必须在
    # 第 1 趟之前，否则患者首篇名额会被非因果文档占满。
    if require_causal_coverage:
        causal_by_patient: Dict[str, List[Dict[str, Any]]] = defaultdict(list)
        for record in records:
            patient_id = record.get("elder_id")
            if patient_id and has_causal_fields(record) and record.get("event_type") in EVENT_TYPES:
                causal_by_patient[patient_id].append(record)
        missing = [patient_id for patient_id in patients if not causal_by_patient.get(patient_id)]
        if missing:
            raise ValueError(
                "source data cannot give every patient a causal document; "
                f"{len(missing)} patient(s) have none, e.g. {missing[:3]}"
            )
        for patient_id in patients:
            bucket = causal_by_patient[patient_id]
            candidate = next((item for item in bucket if doc_id(item) not in seen), None)
            if candidate is None:
                raise ValueError(f"causal document lookup failed for patient {patient_id}")
            selected.append(candidate)
            seen.add(doc_id(candidate))
        if len(selected) > count:
            raise ValueError("count is too small to hold one causal document per patient")

    # 第 1 趟：保证每位患者都出现，同时轮转事件类型。
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

    # 第 2 趟：把八类事件补齐到请求的语料规模。
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


def build_queries(
    records: Iterable[Dict[str, Any]],
    seed: int,
    counts: Dict[str, int],
    annotation_status: str = REVIEW_DRAFT_STATUS,
) -> tuple[List[Dict[str, Any]], Dict[str, Dict[str, int]]]:
    """构造查询集，并回报每类的患者与文档覆盖情况。

    相关性由构造定义：查询文本由目标文档生成，真值即该文档的 doc_id。这里对每条查询
    按患者去重，使同一患者的多个事件不会同时占据配额；去重不足以填满配额时才退回剩余
    候选，并把兜底条数如实计入覆盖统计——否则「配额达标」会掩盖患者覆盖不足。
    """
    by_type: Dict[str, List[Dict[str, Any]]] = defaultdict(list)
    for record in records:
        if has_causal_fields(record):
            by_type["causal"].append(record)
        by_type["temporal"].append(record)
        by_type["general"].append(record)

    rng = random.Random(seed + 1)
    queries: List[Dict[str, Any]] = []
    coverage: Dict[str, Dict[str, int]] = {}

    for query_type in QUERY_TYPES:
        expected = counts[query_type]
        candidates = sorted(by_type[query_type], key=lambda item: (item["elder_id"], item["timestamp"]))
        rng.shuffle(candidates)

        selected: List[Dict[str, Any]] = []
        patient_seen = set()
        seen_documents = set()
        for record in candidates:
            if record["elder_id"] not in patient_seen and doc_id(record) not in seen_documents:
                selected.append(record)
                patient_seen.add(record["elder_id"])
                seen_documents.add(doc_id(record))
            if len(selected) == expected:
                break

        filled_beyond_patient_dedup = 0
        if len(selected) < expected:
            # 患者去重不足时的兜底。旧实现写作 candidates[len(selected):expected]，
            # 把「已选条数」误当切片下标，会静默取到任意记录且无法察觉覆盖不足；
            # 这里改为按 doc_id 去重地取剩余候选，并把兜底条数记录在案。
            for record in candidates:
                if len(selected) >= expected:
                    break
                if doc_id(record) in seen_documents:
                    continue
                selected.append(record)
                seen_documents.add(doc_id(record))
                filled_beyond_patient_dedup += 1

        if len(selected) != expected:
            raise ValueError(
                f"not enough {query_type} records for {expected} queries (got {len(selected)})"
            )

        coverage[query_type] = {
            "queries": len(selected),
            "distinct_patients": len({record["elder_id"] for record in selected}),
            "distinct_documents": len({doc_id(record) for record in selected}),
            "filled_beyond_patient_dedup": filled_beyond_patient_dedup,
        }

        for index, record in enumerate(selected, 1):
            text, keywords, reason = query_text(record, query_type)
            queries.append({
                "query_id": f"competition_{query_type}_{index:03d}",
                "query_type": query_type,
                "question": text,
                "ground_truth_docs": [doc_id(record)],
                "expected_keywords": keywords,
                "annotation": {
                    "status": annotation_status,
                    "proposed_relevance": 1,
                    "proposed_reason": reason,
                    "reviewer": "",
                    "reviewed_at": "",
                },
            })
    return queries, coverage


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
    parser.add_argument(
        "--per-type-queries", type=int, default=0,
        help="每个查询类型的条数；0 表示沿用既有 34/33/33 配额，使历史产物可复现",
    )
    parser.add_argument(
        "--annotation-mode", choices=ANNOTATION_MODES, default="review_draft",
        help="review_draft=待人工复核；auto_derived=机器推导且明确未复核",
    )
    parser.add_argument("--corpus-dataset-id", default="competition_retrieval_corpus_v1")
    parser.add_argument("--queries-dataset-id", default="competition_retrieval_queries_v1_draft")
    parser.add_argument(
        "--require-causal-coverage", action="store_true",
        help="保证每位患者都有一篇因果文档，使 causal 配额能覆盖全部患者",
    )
    args = parser.parse_args()

    counts = (
        {query_type: args.per_type_queries for query_type in QUERY_TYPES}
        if args.per_type_queries > 0
        else dict(LEGACY_QUERY_COUNTS)
    )
    annotation_status = (
        AUTO_DERIVED_STATUS if args.annotation_mode == "auto_derived" else REVIEW_DRAFT_STATUS
    )

    records = json.loads(SOURCE_FILE.read_text(encoding="utf-8"))
    if not isinstance(records, list) or len(records) < args.count:
        raise ValueError("nursing source data is insufficient")
    selected = stratified_select(records, args.count, args.seed, args.require_causal_coverage)
    documents = [document(record) for record in selected]
    source_sha = sha256_file(SOURCE_FILE)
    event_counts = {event: sum(record["event_type"] == event for record in selected) for event in EVENT_TYPES}
    patient_count = len({record["elder_id"] for record in selected})
    causal_documents = [record for record in selected if has_causal_fields(record)]
    causal_patient_count = len({record["elder_id"] for record in causal_documents})
    if len(documents) != args.count or len({item["doc_id"] for item in documents}) != args.count:
        raise ValueError("generated corpus does not have unique document identifiers")
    if patient_count != 100 or any(count == 0 for count in event_counts.values()):
        raise ValueError("generated corpus does not preserve patient and event-type coverage")
    if args.require_causal_coverage and causal_patient_count != patient_count:
        raise ValueError(
            "requested causal coverage but the corpus does not give every patient a causal document"
        )

    corpus = {
        "metadata": {
            "dataset_id": args.corpus_dataset_id,
            "dataset_status": "frozen_candidate_corpus",
            "generated_at_utc": datetime.now(timezone.utc).isoformat(),
            "source_file": "datasets/nursing_data/nursing_records.json",
            "source_record_count": len(records),
            "source_sha256": source_sha,
            "selected_document_count": len(documents),
            "patient_count": patient_count,
            "event_type_counts": event_counts,
            "causal_document_count": len(causal_documents),
            "causal_patient_count": causal_patient_count,
            "selection_seed": args.seed,
            "selection_method": (
                "deterministic patient-first and event-balanced stratified sample"
                + (
                    ", with one causal document reserved per patient so that "
                    "causal queries can cover every patient"
                    if args.require_causal_coverage
                    else ""
                )
            ),
        },
        "documents": documents,
    }
    write_json(args.corpus_output, corpus)
    queries, coverage = build_queries(selected, args.seed, counts, annotation_status)
    query_metadata: Dict[str, Any] = {
        "dataset_id": args.queries_dataset_id,
        "annotation_status": annotation_status,
        # 两种模式都保持 False：机器推导的标签在通过人工复核前都不是可报告真值。
        "report_eligible": False,
        "derivation_method": DERIVATION_METHOD,
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "source_corpus_file": args.corpus_output.name,
        "source_corpus_sha256": sha256_file(args.corpus_output),
        "total_queries": len(queries),
        "breakdown": counts,
        "coverage": coverage,
    }
    if annotation_status == AUTO_DERIVED_STATUS:
        query_metadata["annotation_caveat"] = AUTO_DERIVED_CAVEAT
    else:
        query_metadata["annotation_instruction"] = (
            "Complete the companion CSV and finalize it before using this file for formal metrics."
        )
    query_payload = {"metadata": query_metadata, "queries": queries}
    write_json(args.queries_output, query_payload)
    write_review_csv(args.review_output, queries, query_payload["metadata"]["source_corpus_sha256"])
    print(f"corpus={args.corpus_output}")
    print(f"queries={args.queries_output}")
    print(f"review_queue={args.review_output}")
    print(
        f"documents={len(documents)} patients={patient_count} "
        f"causal_documents={len(causal_documents)} causal_patients={causal_patient_count} "
        f"queries={len(queries)} annotation_status={annotation_status}"
    )
    for query_type in QUERY_TYPES:
        stats = coverage[query_type]
        print(
            f"  {query_type}: queries={stats['queries']} "
            f"distinct_patients={stats['distinct_patients']} "
            f"distinct_documents={stats['distinct_documents']} "
            f"filled_beyond_patient_dedup={stats['filled_beyond_patient_dedup']}"
        )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
