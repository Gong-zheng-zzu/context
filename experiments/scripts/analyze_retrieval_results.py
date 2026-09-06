#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
检索结果根因分析 - 按查询类型分组统计
"""

import json
from pathlib import Path
from collections import defaultdict
import sys

def load_result_json(config_name: str) -> dict:
    """加载指定配置的结果JSON"""
    results_dir = Path(__file__).parent.parent / "results" / "raw"

    # 获取所有retrieval结果文件
    all_files = list(results_dir.glob("retrieval_*.json"))

    # 按修改时间排序，取最近的文件
    all_files.sort(key=lambda p: p.stat().st_mtime, reverse=True)

    # 在最近的10个文件中查找匹配的配置
    for result_file in all_files[:10]:
        try:
            with open(result_file, 'r', encoding='utf-8') as f:
                data = json.load(f)
                # 检查configs列表中是否包含目标配置
                configs = data.get("configs", [])

                if config_name in configs:
                    print(f"[INFO] 加载文件: {result_file.name} (config: {config_name})")
                    return data
        except Exception as e:
            print(f"[WARN] 读取文件失败: {result_file.name} - {e}")
            continue

    raise FileNotFoundError(f"未找到配置 {config_name} 的结果文件")

def group_by_query_type(results: dict, config_name: str) -> dict:
    """按查询类型分组统计"""
    grouped = defaultdict(lambda: {"mrr": [], "precision": [], "recall": []})

    # 使用detailed_results字段
    query_results = results.get("detailed_results", [])

    for query_result in query_results:
        # 直接使用query_type字段
        query_type = query_result.get("query_type", "general")

        # 提取指标
        mrr = query_result.get("reciprocal_rank", 0)
        precision = query_result.get("precision_at_5", 0)
        recall = query_result.get("recall_at_5", 0)

        grouped[query_type]["mrr"].append(mrr)
        grouped[query_type]["precision"].append(precision)
        grouped[query_type]["recall"].append(recall)

    # 计算平均值
    summary = {}
    for qtype, metrics in grouped.items():
        if metrics["mrr"]:
            summary[qtype] = {
                "mrr": sum(metrics["mrr"]) / len(metrics["mrr"]),
                "precision": sum(metrics["precision"]) / len(metrics["precision"]),
                "recall": sum(metrics["recall"]) / len(metrics["recall"]),
                "count": len(metrics["mrr"])
            }

    return summary

def compare_configs():
    """对比三个配置的分组结果"""
    configs = ["baseline_naive_rag", "baseline_rag_with_filter", "full_system"]

    all_summaries = {}
    for config in configs:
        try:
            result = load_result_json(config)
            all_summaries[config] = group_by_query_type(result, config)
        except FileNotFoundError as e:
            print(f"[ERROR] {e}")
            return

    # 打印对比表
    print("\n" + "="*80)
    print("查询类型分组对比分析")
    print("="*80 + "\n")

    print("| Query Type | Config                   | MRR   | Precision@5 | Recall@5 | Count |")
    print("|------------|--------------------------|-------|-------------|----------|-------|")

    for qtype in ["temporal", "causal", "general"]:
        for config in configs:
            metrics = all_summaries[config].get(qtype, {})
            config_display = config.replace("baseline_", "")
            print(f"| {qtype:10} | {config_display:24} | {metrics.get('mrr', 0):.3f} | "
                  f"{metrics.get('precision', 0):.3f}       | {metrics.get('recall', 0):.3f}    | "
                  f"{metrics.get('count', 0):5} |")

    # 计算改进率
    print("\n" + "="*80)
    print("full_system vs baseline_naive_rag 改进率")
    print("="*80 + "\n")

    print("| Query Type | MRR改进率 | Precision改进率 | Recall改进率 | 结论 |")
    print("|------------|----------|----------------|-------------|------|")

    for qtype in ["temporal", "causal", "general"]:
        naive = all_summaries["baseline_naive_rag"].get(qtype, {})
        full = all_summaries["full_system"].get(qtype, {})

        if naive.get("mrr", 0) > 0:
            mrr_improvement = ((full.get("mrr", 0) - naive.get("mrr", 0)) / naive.get("mrr", 0)) * 100
            prec_improvement = ((full.get("precision", 0) - naive.get("precision", 0)) / naive.get("precision", 0)) * 100
            recall_improvement = ((full.get("recall", 0) - naive.get("recall", 0)) / naive.get("recall", 0)) * 100

            conclusion = "[OK]" if mrr_improvement > 0 else "[DOWN]"

            print(f"| {qtype:10} | {mrr_improvement:+7.1f}% | {prec_improvement:+13.1f}% | {recall_improvement:+10.1f}% | {conclusion:6} |")
        else:
            print(f"| {qtype:10} | N/A      | N/A            | N/A         | N/A  |")

    print("\n" + "="*80)
    print("关键发现")
    print("="*80 + "\n")

    # 自动诊断
    for qtype in ["temporal", "causal", "general"]:
        naive = all_summaries["baseline_naive_rag"].get(qtype, {})
        full = all_summaries["full_system"].get(qtype, {})

        if naive.get("mrr", 0) > 0:
            if full.get("mrr", 0) < naive.get("mrr", 0) * 0.5:
                print(f"[WARN] {qtype.upper()}: full_system严重下降（MRR仅为naive的{full.get('mrr', 0) / naive.get('mrr', 0) * 100:.0f}%）")
            elif full.get("mrr", 0) < naive.get("mrr", 0):
                print(f"[WARN] {qtype.upper()}: full_system轻微下降（MRR下降{(1 - full.get('mrr', 0) / naive.get('mrr', 0)) * 100:.0f}%）")
            else:
                print(f"[OK] {qtype.upper()}: full_system有所改善（MRR提升{(full.get('mrr', 0) / naive.get('mrr', 0) - 1) * 100:.0f}%）")

if __name__ == "__main__":
    compare_configs()
