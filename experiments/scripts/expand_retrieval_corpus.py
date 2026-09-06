#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""Create a deterministic retrieval corpus from real nursing records."""

import argparse
import hashlib
import json
import random
from collections import defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
SOURCE_FILE = ROOT / "datasets" / "nursing_data" / "nursing_records.json"
GROUND_TRUTH_FILE = ROOT / "datasets" / "retrieval_groundtruth" / "query_answer_pairs.json"
DEFAULT_OUTPUT = ROOT / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json"


def make_doc_id(record):
    return f"{record['elder_id']}_{record['timestamp']}"


def load_json(path):
    with open(path, "r", encoding="utf-8") as file:
        return json.load(file)


def record_to_document(record):
    metadata = {
        "patient_id": record["elder_id"],
        "patient_name": record.get("elder_name", ""),
        "timestamp": record["timestamp"],
        "type": "health_record",
        "event_type": record.get("event_type", ""),
        "severity": record.get("severity", ""),
        "tags": record.get("tags", []),
    }
    if record.get("structured_data"):
        metadata["structured_data"] = record["structured_data"]
    if record.get("causal_factors"):
        metadata["causal_factors"] = record["causal_factors"]

    return {
        "doc_id": make_doc_id(record),
        "content": (
            f"患者编号: {record['elder_id']}\n"
            f"患者姓名: {record.get('elder_name', '')}\n"
            f"记录时间: {record['timestamp']}\n"
            f"事件类型: {record.get('event_type', '')}\n"
            f"严重程度: {record.get('severity', '')}\n"
            f"护理记录: {record['content']}"
        ),
        "metadata": metadata,
    }


def ground_truth_doc_ids(ground_truth):
    return {
        doc_id
        for query in ground_truth.get("queries", [])
        for doc_id in query.get("ground_truth_docs", [])
    }


def select_records(records, target_ids, count, seed):
    by_id = {make_doc_id(record): record for record in records}
    missing_ids = sorted(target_ids.difference(by_id))
    if missing_ids:
        raise ValueError(f"Ground truth 文档不在源数据中: {missing_ids}")
    if count < len(target_ids):
        raise ValueError(f"文档数量不能小于 {len(target_ids)} 条 Ground Truth 文档")

    selected = [by_id[doc_id] for doc_id in sorted(target_ids)]
    selected_ids = set(target_ids)
    grouped = defaultdict(list)
    for record in records:
        if make_doc_id(record) not in selected_ids:
            grouped[record.get("event_type", "unknown")].append(record)

    rng = random.Random(seed)
    for group in grouped.values():
        rng.shuffle(group)

    event_types = sorted(grouped)
    cursor = {event_type: 0 for event_type in event_types}
    while len(selected) < count:
        progressed = False
        for event_type in event_types:
            index = cursor[event_type]
            if index >= len(grouped[event_type]):
                continue
            selected.append(grouped[event_type][index])
            cursor[event_type] += 1
            progressed = True
            if len(selected) == count:
                break
        if not progressed:
            raise ValueError("源数据不足以构建指定规模语料")
    return selected


def main():
    parser = argparse.ArgumentParser(description="从真实护理记录构建检索实验语料")
    parser.add_argument("--count", type=int, default=30)
    parser.add_argument("--seed", type=int, default=20260715)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    records = load_json(SOURCE_FILE)
    ground_truth = load_json(GROUND_TRUTH_FILE)
    targets = ground_truth_doc_ids(ground_truth)
    selected = select_records(records, targets, args.count, args.seed)
    documents = [record_to_document(record) for record in selected]

    args.output.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "metadata": {
            "source_file": str(SOURCE_FILE.relative_to(ROOT)),
            "source_record_count": len(records),
            "selected_document_count": len(documents),
            "ground_truth_document_count": len(targets),
            "selection_seed": args.seed,
            "source_sha256": hashlib.sha256(SOURCE_FILE.read_bytes()).hexdigest(),
        },
        "documents": documents,
    }
    with open(args.output, "w", encoding="utf-8") as file:
        json.dump(payload, file, ensure_ascii=False, indent=2)

    print(f"[OK] 源记录: {len(records)}")
    print(f"[OK] Ground Truth文档: {len(targets)}")
    print(f"[OK] 已选择真实文档: {len(documents)}")
    print(f"[OK] 输出文件: {args.output}")


if __name__ == "__main__":
    main()
