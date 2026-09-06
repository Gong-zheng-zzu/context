#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
自动生成检索ground truth查询-答案对
从护理记录中提取50对高质量查询
"""

import json
import random
from pathlib import Path
from datetime import datetime, timedelta
from collections import defaultdict

def load_nursing_records(file_path):
    """加载护理记录"""
    with open(file_path, 'r', encoding='utf-8') as f:
        return json.load(f)

def generate_temporal_queries(records, count=15):
    """生成时序查询（15对）"""
    queries = []

    # 按老人分组
    by_elder = defaultdict(list)
    for r in records:
        by_elder[r['elder_name']].append(r)

    # 选择有多条记录的老人
    elders_with_history = [(name, recs) for name, recs in by_elder.items() if len(recs) >= 3]
    selected = random.sample(elders_with_history, min(count, len(elders_with_history)))

    for i, (elder_name, elder_records) in enumerate(selected, 1):
        # 按时间排序
        elder_records.sort(key=lambda x: x['timestamp'])

        # 提取最近N天的记录
        latest_records = elder_records[-3:]
        doc_ids = [f"{r['elder_id']}_{r['timestamp']}" for r in latest_records]

        # 提取关键信息
        events = [r['event_type'] for r in latest_records]

        query = {
            "query_id": f"temporal_{i:03d}",
            "query_type": "temporal",
            "question": f"{elder_name}最近的健康记录和变化趋势是什么？",
            "ground_truth_docs": doc_ids,
            "expected_keywords": [elder_name, "趋势", "变化"] + events[:2],
            "metadata": {
                "elder_id": latest_records[0]['elder_id'],
                "record_count": len(latest_records),
                "time_span": "recent"
            }
        }
        queries.append(query)

    return queries

def generate_causal_queries(records, count=15):
    """生成因果查询（15对）"""
    queries = []

    # 筛选包含因果关系的记录
    causal_keywords = ['导致', '引起', '因为', '由于', '服用', '用药', '血压', '血糖']
    causal_records = [r for r in records
                     if any(kw in r.get('content', '') for kw in causal_keywords)]

    # 按事件类型分组
    by_event = defaultdict(list)
    for r in causal_records:
        by_event[r['event_type']].append(r)

    # 为每种事件类型生成查询
    event_types = list(by_event.keys())
    selected_types = random.sample(event_types, min(count, len(event_types)))

    for i, event_type in enumerate(selected_types, 1):
        event_records = by_event[event_type]
        sample_record = random.choice(event_records)

        elder_name = sample_record['elder_name']
        doc_id = f"{sample_record['elder_id']}_{sample_record['timestamp']}"

        # 生成因果问题
        causal_questions = [
            f"{elder_name}的{event_type}是什么原因引起的？",
            f"为什么{elder_name}会出现{event_type}？",
            f"{elder_name}{event_type}的诱因是什么？"
        ]

        query = {
            "query_id": f"causal_{i:03d}",
            "query_type": "causal",
            "question": random.choice(causal_questions),
            "ground_truth_docs": [doc_id],
            "expected_keywords": [elder_name, event_type, "原因", "因果"],
            "metadata": {
                "elder_id": sample_record['elder_id'],
                "event_type": event_type,
                "severity": sample_record.get('severity', 'medium')
            }
        }
        queries.append(query)

    return queries

def generate_general_queries(records, count=20):
    """生成普通查询（20对）"""
    queries = []

    # 随机选择记录
    selected = random.sample(records, min(count, len(records)))

    for i, record in enumerate(selected, 1):
        elder_name = record['elder_name']
        event_type = record['event_type']
        doc_id = f"{record['elder_id']}_{record['timestamp']}"

        # 生成多样化的问题
        general_questions = [
            f"{elder_name}的基本健康状况如何？",
            f"查询{elder_name}的护理记录",
            f"{elder_name}最近有哪些健康事件？",
            f"告诉我{elder_name}的情况",
            f"{elder_name}的{event_type}记录"
        ]

        query = {
            "query_id": f"general_{i:03d}",
            "query_type": "general",
            "question": random.choice(general_questions),
            "ground_truth_docs": [doc_id],
            "expected_keywords": [elder_name, event_type],
            "metadata": {
                "elder_id": record['elder_id'],
                "event_type": event_type
            }
        }
        queries.append(query)

    return queries

def main():
    # 路径设置
    script_dir = Path(__file__).parent
    nursing_file = script_dir.parent.parent / "tests" / "datasets" / "nursing_data" / "nursing_records.json"

    if not nursing_file.exists():
        print(f"[ERROR] 护理记录文件不存在: {nursing_file}")
        return

    print(f"[LOAD] 加载护理记录: {nursing_file}")
    records = load_nursing_records(nursing_file)
    print(f"[INFO] 加载了 {len(records)} 条记录")

    # 设置随机种子以保证可复现
    random.seed(42)

    # 生成三类查询
    print("\n[GEN] 生成时序查询...")
    temporal_queries = generate_temporal_queries(records, count=15)
    print(f"  ✓ 生成了 {len(temporal_queries)} 条时序查询")

    print("\n[GEN] 生成因果查询...")
    causal_queries = generate_causal_queries(records, count=15)
    print(f"  ✓ 生成了 {len(causal_queries)} 条因果查询")

    print("\n[GEN] 生成普通查询...")
    general_queries = generate_general_queries(records, count=20)
    print(f"  ✓ 生成了 {len(general_queries)} 条普通查询")

    # 合并所有查询
    all_queries = temporal_queries + causal_queries + general_queries

    # 添加元数据
    output = {
        "metadata": {
            "generated_at": datetime.now().isoformat(),
            "total_queries": len(all_queries),
            "source_records": len(records),
            "breakdown": {
                "temporal": len(temporal_queries),
                "causal": len(causal_queries),
                "general": len(general_queries)
            }
        },
        "queries": all_queries
    }

    # 保存到文件
    output_file = script_dir / "query_answer_pairs.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(output, f, ensure_ascii=False, indent=2)

    print(f"\n[SAVE] 已保存到: {output_file}")
    print(f"[DONE] 总共生成 {len(all_queries)} 对查询-答案")

    # 打印示例
    print("\n[示例] 前3条查询:")
    for i, q in enumerate(all_queries[:3], 1):
        print(f"\n{i}. [{q['query_type']}] {q['question']}")
        print(f"   Ground Truth: {q['ground_truth_docs'][0]}")

if __name__ == "__main__":
    main()
