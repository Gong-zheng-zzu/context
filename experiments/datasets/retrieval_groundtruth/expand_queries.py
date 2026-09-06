#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Ground Truth扩展脚本
将现有30对查询扩展到50对（+20对）
"""

import json
from pathlib import Path

def generate_additional_20_queries():
    """生成额外的20对查询"""
    additional = {
        "temporal": [
            {"query_id": "temporal_011", "query_type": "temporal",
             "question": "最近一个月血压波动超过20mmHg的患者统计？",
             "ground_truth_docs": ["elder_025_*", "elder_084_*"],
             "expected_keywords": ["血压", "波动", "一个月"]},
            {"query_id": "temporal_012", "query_type": "temporal",
             "question": "夜间2-4点谵妄事件的时间分布规律？",
             "ground_truth_docs": ["elder_025_2026-06-06T02:40:43.194500"],
             "expected_keywords": ["夜间", "谵妄", "分布"]},
            {"query_id": "temporal_013", "query_type": "temporal",
             "question": "用药后48小时内副作用监测记录汇总？",
             "ground_truth_docs": ["elder_047_*", "elder_082_*"],
             "expected_keywords": ["用药", "48小时", "副作用"]},
            {"query_id": "temporal_014", "query_type": "temporal",
             "question": "跌倒风险评分的3个月变化趋势？",
             "ground_truth_docs": ["elder_080_*", "elder_033_*"],
             "expected_keywords": ["跌倒", "风险", "趋势"]},
            {"query_id": "temporal_015", "query_type": "temporal",
             "question": "情绪波动与时段关联分析（早中晚夜）？",
             "ground_truth_docs": ["elder_084_*", "elder_082_*"],
             "expected_keywords": ["情绪", "时段", "关联"]},
            {"query_id": "temporal_016", "query_type": "temporal",
             "question": "认知功能MMSE评分的月度对比？",
             "ground_truth_docs": ["elder_025_*", "elder_020_*"],
             "expected_keywords": ["认知", "MMSE", "月度"]},
            {"query_id": "temporal_017", "query_type": "temporal",
             "question": "护理干预前后7天的行为改善对比？",
             "ground_truth_docs": ["elder_025_*", "elder_020_*"],
             "expected_keywords": ["干预", "7天", "对比"]}
        ],
        "causal": [
            {"query_id": "causal_011", "query_type": "causal",
             "question": "睡眠质量下降导致认知功能恶化的因果链？",
             "ground_truth_docs": ["elder_025_*", "elder_080_*"],
             "expected_keywords": ["睡眠", "认知", "因果"]},
            {"query_id": "causal_012", "query_type": "causal",
             "question": "室温过高与夜间躁动的关联分析？",
             "ground_truth_docs": ["elder_084_*"],
             "expected_keywords": ["室温", "躁动", "关联"]},
            {"query_id": "causal_013", "query_type": "causal",
             "question": "家属探访频率对患者情绪稳定性的影响？",
             "ground_truth_docs": ["elder_082_*", "elder_047_*"],
             "expected_keywords": ["探访", "情绪", "影响"]},
            {"query_id": "causal_014", "query_type": "causal",
             "question": "阿尔茨海默症患者便秘与谵妄发作的因果关系？",
             "ground_truth_docs": ["elder_025_*"],
             "expected_keywords": ["便秘", "谵妄", "因果"]},
            {"query_id": "causal_015", "query_type": "causal",
             "question": "社交活动参与度对认知衰退速度的影响？",
             "ground_truth_docs": ["elder_020_*", "elder_033_*"],
             "expected_keywords": ["社交", "认知", "衰退"]},
            {"query_id": "causal_016", "query_type": "causal",
             "question": "护士换班时段（交接班）患者不安行为增加的原因？",
             "ground_truth_docs": ["elder_025_*", "elder_084_*"],
             "expected_keywords": ["换班", "不安", "原因"]},
            {"query_id": "causal_017", "query_type": "causal",
             "question": "多重用药（5种以上）与跌倒风险的因果分析？",
             "ground_truth_docs": ["elder_080_*"],
             "expected_keywords": ["多重用药", "跌倒", "风险"]}
        ],
        "general": [
            {"query_id": "general_011", "query_type": "general",
             "question": "所有患者的年龄段分布和性别比例统计？",
             "ground_truth_docs": ["elder_*"],
             "expected_keywords": ["年龄", "性别", "统计"]},
            {"query_id": "general_012", "query_type": "general",
             "question": "当前所有在用药物清单及使用频率？",
             "ground_truth_docs": ["elder_*"],
             "expected_keywords": ["药物", "清单", "频率"]},
            {"query_id": "general_013", "query_type": "general",
             "question": "护理活动类型统计（喂食、翻身、清洁等）？",
             "ground_truth_docs": ["elder_*"],
             "expected_keywords": ["护理", "活动", "统计"]},
            {"query_id": "general_014", "query_type": "general",
             "question": "最高频的5种症状及其发生次数排名？",
             "ground_truth_docs": ["elder_*"],
             "expected_keywords": ["症状", "频率", "排名"]},
            {"query_id": "general_015", "query_type": "general",
             "question": "年龄>80岁且有跌倒史的患者筛选？",
             "ground_truth_docs": ["elder_080_*", "elder_084_*"],
             "expected_keywords": ["80岁", "跌倒", "筛选"]},
            {"query_id": "general_016", "query_type": "general",
             "question": "非药物干预（音乐疗法、芳香疗法）的使用记录？",
             "ground_truth_docs": ["elder_033_*", "elder_047_*"],
             "expected_keywords": ["非药物", "疗法", "记录"]}
        ]
    }
    return additional

def append_to_json():
    """追加到现有JSON文件"""
    json_path = Path(__file__).parent / "query_answer_pairs.json"

    print(f"[INFO] 正在加载现有数据: {json_path}")

    # 读取现有数据
    with open(json_path, 'r', encoding='utf-8') as f:
        data = json.load(f)

    current_count = len(data["queries"])
    print(f"[INFO] 当前查询数量: {current_count}")

    # 生成新查询
    additional = generate_additional_20_queries()

    # 追加
    data["queries"].extend(additional["temporal"])
    data["queries"].extend(additional["causal"])
    data["queries"].extend(additional["general"])

    # 更新metadata
    data["metadata"]["total_pairs"] = 50
    data["metadata"]["temporal_count"] = 17
    data["metadata"]["causal_count"] = 17
    data["metadata"]["general_count"] = 16
    data["metadata"]["last_updated"] = "2026-07-13"

    # 保存
    with open(json_path, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)

    print(f"[SUCCESS] 成功扩展到50对查询")
    print(f"   - Temporal: 17对")
    print(f"   - Causal: 17对")
    print(f"   - General: 16对")

if __name__ == "__main__":
    append_to_json()
