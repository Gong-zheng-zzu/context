#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""对同一批查询上的两次检索评测做配对统计，回答「差异是否真实」。

为什么放在离线脚本
------------------
``retrieval_eval.py`` 强制「一次运行只记录一个运行时配置」（配置数不为 1 即退出），
因为一个结果文件只能对应一个有效配置哈希。vector 基线与 RRF 对照因此必然是两次
独立运行。把跨模式配对检验放在这里读取两份过门禁产物，既不动既有的单配置约束，
也不改结果 schema。

为什么要配对
------------
两条流水线跑的是**同一批查询**。查询难度差异（有些查询本来就容易命中）是最大的
方差来源；逐查询配对后该方差被消掉，剩下的差值才反映流水线差异。忽略配对去做
「两组均值比较」会在小样本上严重高估显著性。

统计口径
--------
* MRR / P@5 / R@5 —— 逐样本差值不是 0/1 比例量，用配对 bootstrap 百分位区间；
  另外给 MRR 附 Wilcoxon 符号秩检验（正态近似，含并列与连续性修正）。
* 命中率（hit@5）—— 二值判定，用精确 McNemar 检验；其逐样本差值同时给配对
  bootstrap 区间，两者互为交叉验证。

诚实边界
--------
区间与 p 值只说明「在这批查询上差异是否与「无差异」相容」，不说明差异在其他语料
上同样成立。样本量、语料构成与标注方式都应随报告一并披露。
"""

from __future__ import annotations

import argparse
import json
import math
import sys
from collections import Counter
from pathlib import Path
from typing import Any, Dict, List, Optional, Sequence, Tuple

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import stat_tools  # noqa: E402

REPO_ROOT = SCRIPT_DIR.parent.parent

# 参与配对对照的指标：(结果字段名, 展示名, 是否为二值量)
CONTINUOUS_METRICS = (
    ("reciprocal_rank", "mrr"),
    ("precision_at_5", "precision_at_5"),
    ("recall_at_5", "recall_at_5"),
)
BINARY_METRIC = ("found_ground_truth", "hit_at_5")


class PairingError(RuntimeError):
    """配对前提不成立时抛出，调用方据此以非零退出码结束。"""


def load_result(path: Path) -> Dict[str, Any]:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise PairingError(f"cannot read result file {path}: {exc}") from exc
    if not isinstance(payload, dict) or not isinstance(payload.get("detailed_results"), list):
        raise PairingError(f"{path} is not a retrieval evaluation result (no detailed_results)")
    return payload


def scorable_by_query(payload: Dict[str, Any]) -> Dict[str, Dict[str, Any]]:
    """取出可评分响应，按 query_id 索引。

    只有「API 成功且结构可评分」的响应才进入配对：不可评分响应没有排名指标，把它们
    计入分母会把响应格式问题混成检索质量差异。
    """
    indexed: Dict[str, Dict[str, Any]] = {}
    for row in payload["detailed_results"]:
        if row.get("error_type") != "None" or not row.get("is_scorable"):
            continue
        query_id = row.get("query_id")
        if isinstance(query_id, str) and query_id:
            indexed[query_id] = row
    return indexed


def align_pairs(
    payload_a: Dict[str, Any],
    payload_b: Dict[str, Any],
) -> Tuple[List[Tuple[str, Dict[str, Any], Dict[str, Any]]], Dict[str, Any]]:
    """按 query_id 对齐两次运行，并回报未配对的查询。"""
    ground_truth_a = (payload_a.get("dataset") or {}).get("ground_truth_sha256")
    ground_truth_b = (payload_b.get("dataset") or {}).get("ground_truth_sha256")
    if ground_truth_a and ground_truth_b and ground_truth_a != ground_truth_b:
        raise PairingError(
            "the two runs used different ground truth "
            f"({ground_truth_a[:12]}… vs {ground_truth_b[:12]}…); pairing is undefined"
        )

    corpus_a = (payload_a.get("dataset") or {}).get("corpus_sha256")
    corpus_b = (payload_b.get("dataset") or {}).get("corpus_sha256")
    if corpus_a and corpus_b and corpus_a != corpus_b:
        raise PairingError(
            "the two runs were seeded with different corpora "
            f"({corpus_a[:12]}… vs {corpus_b[:12]}…); the same queries would be answered "
            "against different document sets, so the comparison is not paired"
        )

    indexed_a = scorable_by_query(payload_a)
    indexed_b = scorable_by_query(payload_b)
    shared_ids = sorted(set(indexed_a) & set(indexed_b))

    diagnostics = {
        "parseable": bool(shared_ids),
        "paired_queries": len(shared_ids),
        "scorable_only_in_a": sorted(set(indexed_a) - set(indexed_b))[:20],
        "scorable_only_in_b": sorted(set(indexed_b) - set(indexed_a))[:20],
        "scorable_only_in_a_count": len(set(indexed_a) - set(indexed_b)),
        "scorable_only_in_b_count": len(set(indexed_b) - set(indexed_a)),
        "ground_truth_sha256": ground_truth_a or "not_captured",
        "corpus_sha256": corpus_a or "not_captured",
    }
    pairs = [(query_id, indexed_a[query_id], indexed_b[query_id]) for query_id in shared_ids]
    return pairs, diagnostics


def wilcoxon_signed_rank(differences: Sequence[float]) -> Dict[str, Any]:
    """Wilcoxon 符号秩检验（双侧，正态近似）。

    精确分布需要枚举 2^n 种符号组合，n 达到数百时不可行，因此使用正态近似，并施加
    并列秩修正与连续性修正。零差值按标准做法剔除；若全部为零则检验无定义。
    """
    non_zero = [value for value in differences if value != 0.0]
    n = len(non_zero)
    if n == 0:
        return {
            "n_non_zero": 0,
            "statistic": None,
            "two_sided_p": None,
            "method": "wilcoxon_signed_rank_normal_approximation",
            "note": "all paired differences are zero; the test is undefined",
        }

    ordered = sorted(range(n), key=lambda index: abs(non_zero[index]))
    ranks = [0.0] * n
    tie_group_sizes: List[int] = []
    position = 0
    while position < n:
        end = position
        while end + 1 < n and abs(non_zero[ordered[end + 1]]) == abs(non_zero[ordered[position]]):
            end += 1
        group_size = end - position + 1
        tie_group_sizes.append(group_size)
        average_rank = (position + 1 + end + 1) / 2.0
        for offset in range(position, end + 1):
            ranks[ordered[offset]] = average_rank
        position = end + 1

    w_plus = sum(rank for rank, value in zip(ranks, non_zero) if value > 0)
    mean_w = n * (n + 1) / 4.0
    variance = n * (n + 1) * (2 * n + 1) / 24.0
    # 并列修正：每组并列使方差减少 t^3 - t（t 为该组大小）。
    variance -= sum(size ** 3 - size for size in tie_group_sizes) / 48.0
    if variance <= 0:
        return {
            "n_non_zero": n,
            "statistic": w_plus,
            "two_sided_p": None,
            "method": "wilcoxon_signed_rank_normal_approximation",
            "note": "variance collapsed to zero after tie correction; the test is undefined",
        }

    z = (abs(w_plus - mean_w) - 0.5) / math.sqrt(variance)
    z = max(z, 0.0)
    two_sided_p = 2.0 * (1.0 - 0.5 * (1.0 + math.erf(z / math.sqrt(2.0))))
    two_sided_p = min(1.0, max(0.0, two_sided_p))
    # 正态近似的尾概率在 |z| 很大时会下溢到 0.0。把下溢报成「p = 0」会暗示差异不可能
    # 由随机性产生，那是过度声称；显式标注下溢，读者才知道这只是双精度精度的下限。
    true_underflow = two_sided_p == 0.0 and w_plus != mean_w
    return {
        "n_non_zero": n,
        "statistic": w_plus,
        "two_sided_p": two_sided_p,
        "two_sided_p_underflow": true_underflow,
        "method": "wilcoxon_signed_rank_normal_approximation",
        "note": (
            "normal approximation with tie and continuity correction; "
            "two_sided_p underflowed below double precision"
            if true_underflow
            else "normal approximation with tie and continuity correction"
        ),
    }


def build_report(
    payload_a: Dict[str, Any],
    payload_b: Dict[str, Any],
    label_a: str,
    label_b: str,
    resamples: int,
    seed: int,
    confidence_level: float,
) -> Dict[str, Any]:
    pairs, diagnostics = align_pairs(payload_a, payload_b)
    if not pairs:
        raise PairingError(
            "no query is scorable in both runs; there is nothing to compare. "
            f"only_in_a={diagnostics['scorable_only_in_a_count']} "
            f"only_in_b={diagnostics['scorable_only_in_b_count']}"
        )

    def metric_intervals(field: str) -> Dict[str, Any]:
        differences = [float(row_b[field]) - float(row_a[field]) for _, row_a, row_b in pairs]
        return stat_tools.paired_bootstrap_ci(
            differences,
            resamples=resamples,
            confidence_level=confidence_level,
            seed=seed,
        )

    metrics: Dict[str, Any] = {}
    for field, name in CONTINUOUS_METRICS:
        metrics[name] = metric_intervals(field)

    # 二值指标：配对 bootstrap 区间与精确 McNemar 互为交叉验证。
    binary_field, binary_name = BINARY_METRIC
    wins = sum(1 for _, row_a, row_b in pairs if row_b[binary_field] and not row_a[binary_field])
    losses = sum(1 for _, row_a, row_b in pairs if row_a[binary_field] and not row_b[binary_field])
    metrics[binary_name] = {
        "difference": metric_intervals(binary_field),
        "mcnemar_exact": stat_tools.mcnemar_exact(wins, losses),
        "rate_a": sum(1 for _, row_a, _ in pairs if row_a[binary_field]) / len(pairs),
        "rate_b": sum(1 for _, _, row_b in pairs if row_b[binary_field]) / len(pairs),
    }

    # 按查询类型细分：只看 MRR，避免重采样次数随类型数线性膨胀。
    by_type: Dict[str, Any] = {}
    type_buckets: Dict[str, List[Tuple[Dict[str, Any], Dict[str, Any]]]] = {}
    for _, row_a, row_b in pairs:
        type_buckets.setdefault(str(row_a.get("query_type", "unknown")), []).append((row_a, row_b))
    for query_type in sorted(type_buckets):
        bucket = type_buckets[query_type]
        differences = [float(row_b["reciprocal_rank"]) - float(row_a["reciprocal_rank"]) for row_a, row_b in bucket]
        interval = stat_tools.paired_bootstrap_ci(
            differences,
            resamples=resamples,
            confidence_level=confidence_level,
            seed=seed,
        )
        interval["query_count"] = len(bucket)
        by_type[query_type] = interval

    mrr_differences = [float(row_b["reciprocal_rank"]) - float(row_a["reciprocal_rank"]) for _, row_a, row_b in pairs]

    return {
        "schema": "paired_retrieval_stats_v1",
        "pairing": diagnostics,
        "comparison": {
            "label_a": label_a,
            "label_b": label_b,
            "evaluation_mode_a": payload_a.get("evaluation_mode", "not_captured"),
            "evaluation_mode_b": payload_b.get("evaluation_mode", "not_captured"),
            "session_id_a": (payload_a.get("dataset") or {}).get("session_id", "not_captured"),
            "session_id_b": (payload_b.get("dataset") or {}).get("session_id", "not_captured"),
            "annotation_status_a": (payload_a.get("dataset") or {}).get("annotation_status", "not_captured"),
            "annotation_status_b": (payload_b.get("dataset") or {}).get("annotation_status", "not_captured"),
            "report_eligible_a": (payload_a.get("dataset") or {}).get("report_eligible", "not_captured"),
            "report_eligible_b": (payload_b.get("dataset") or {}).get("report_eligible", "not_captured"),
            "total_queries_a": payload_a.get("total_queries"),
            "total_queries_b": payload_b.get("total_queries"),
        },
        "statistics": {
            "method": "paired_bootstrap_percentile_with_exact_mcnemar",
            "resamples": resamples,
            "seed": seed,
            "confidence_level": confidence_level,
            "paired_queries": len(pairs),
        },
        "metrics": metrics,
        "wilcoxon_mrr": wilcoxon_signed_rank(mrr_differences),
        "by_query_type_mrr": by_type,
        "config_hashes_a": payload_a.get("config_hashes", {}),
        "config_hashes_b": payload_b.get("config_hashes", {}),
        "interpretation": (
            "A difference is inconsistent with 'no difference' only when the confidence "
            "interval excludes zero, or when the paired test's p-value is below the "
            "declared significance level. Both are conditional on this query set."
        ),
    }


def render_markdown(report: Dict[str, Any]) -> str:
    pairing = report["pairing"]
    comparison = report["comparison"]
    statistics = report["statistics"]
    label_a = comparison["label_a"]
    label_b = comparison["label_b"]

    lines: List[str] = []
    lines.append(f"# 配对检索对照：{label_a} → {label_b}")
    lines.append("")
    lines.append("同一批查询上的逐查询配对对照。区间不跨越 0 才说明差异与「无差异」不相容。")
    lines.append("")
    lines.append("## 配对前提")
    lines.append("")
    lines.append(f"- 可配对查询数：**{pairing['paired_queries']}**")
    lines.append(f"- 仅在 {label_a} 可评分：{pairing['scorable_only_in_a_count']}")
    lines.append(f"- 仅在 {label_b} 可评分：{pairing['scorable_only_in_b_count']}")
    lines.append(f"- 真值 sha256：`{pairing['ground_truth_sha256']}`")
    lines.append(f"- 语料 sha256：`{pairing['corpus_sha256']}`")
    lines.append(f"- 真值标注状态：{comparison['annotation_status_a']}（report_eligible={comparison['report_eligible_a']}）")
    lines.append(f"- 评测模式：{comparison['evaluation_mode_a']} → {comparison['evaluation_mode_b']}")
    lines.append(
        f"- 统计方法：{statistics['method']}，重采样={statistics['resamples']}，"
        f"seed={statistics['seed']}，置信水平={statistics['confidence_level']}"
    )
    lines.append("")
    lines.append("## 指标对照")
    lines.append("")
    lines.append(f"| 指标 | 差值（{label_b} − {label_a}） | 95% 区间 | 是否跨越 0 |")
    lines.append("| --- | --- | --- | --- |")
    for name, payload in report["metrics"].items():
        if name == BINARY_METRIC[1]:
            payload = payload["difference"]
        if payload.get("mean_diff") is None:
            lines.append(f"| {name} | n/a | n/a | n/a |")
            continue
        crosses = payload.get("crosses_zero")
        crosses_text = "是（无差异相容）" if crosses else "否（差异显著）"
        lines.append(
            f"| {name} | {payload['mean_diff']:+.4f} | "
            f"[{payload['ci_low']:+.4f}, {payload['ci_high']:+.4f}] | {crosses_text} |"
        )
    lines.append("")

    binary_payload = report["metrics"][BINARY_METRIC[1]]
    mcnemar = binary_payload["mcnemar_exact"]
    lines.append("## 命中率的精确 McNemar 检验")
    lines.append("")
    lines.append(f"- {label_a} 命中率：{binary_payload['rate_a']:.4f}")
    lines.append(f"- {label_b} 命中率：{binary_payload['rate_b']:.4f}")
    lines.append(
        f"- 2×2 不一致格：{label_b} 命中而 {label_a} 未命中 = **{mcnemar['wins']}**，"
        f"{label_a} 命中而 {label_b} 未命中 = **{mcnemar['losses']}**"
    )
    lines.append(f"- 不一致对总数：{mcnemar['discordant']}")
    lines.append(f"- 双侧精确 p 值：**{mcnemar['two_sided_exact_p']}**")
    lines.append(f"- 单侧精确 p 值：{mcnemar['one_sided_exact_p']}")
    lines.append("")

    wilcoxon = report["wilcoxon_mrr"]
    lines.append("## MRR 的 Wilcoxon 符号秩检验")
    lines.append("")
    lines.append(f"- 非零差值对数：{wilcoxon['n_non_zero']}")
    lines.append(f"- 统计量（W+）：{wilcoxon['statistic']}")
    if wilcoxon.get("two_sided_p_underflow"):
        lines.append("- 双侧 p 值：**< 1e-300**（正态近似尾概率下溢，低于双精度可表示范围）")
    else:
        lines.append(f"- 双侧 p 值：**{wilcoxon['two_sided_p']}**")
    lines.append(f"- 方法：{wilcoxon['method']}（{wilcoxon['note']}）")
    lines.append("")
    lines.append(
        "注：Wilcoxon 为**正态近似**（并列与连续性修正）。样本量大时精确分布不可行；"
        "上表中命中率的 McNemar 检验是**精确**检验，可作为更可靠的主判据。"
    )
    lines.append("")

    lines.append("## 按查询类型的 MRR 差值")
    lines.append("")
    lines.append("| 查询类型 | 查询数 | 差值 | 95% 区间 | 是否跨越 0 |")
    lines.append("| --- | --- | --- | --- | --- |")
    for query_type, payload in report["by_query_type_mrr"].items():
        if payload.get("mean_diff") is None:
            lines.append(f"| {query_type} | {payload['query_count']} | n/a | n/a | n/a |")
            continue
        crosses_text = "是" if payload.get("crosses_zero") else "否"
        lines.append(
            f"| {query_type} | {payload['query_count']} | {payload['mean_diff']:+.4f} | "
            f"[{payload['ci_low']:+.4f}, {payload['ci_high']:+.4f}] | {crosses_text} |"
        )
    lines.append("")
    lines.append("## 解读边界")
    lines.append("")
    lines.append(report["interpretation"])
    lines.append("")
    return "\n".join(lines)


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--result-a", type=Path, required=True, help="基线运行的结果 JSON")
    parser.add_argument("--result-b", type=Path, required=True, help="对照运行的结果 JSON")
    parser.add_argument("--label-a", default="mode_a", help="基线标签，仅用于展示")
    parser.add_argument("--label-b", default="mode_b", help="对照标签，仅用于展示")
    parser.add_argument("--resamples", type=int, default=stat_tools.DEFAULT_RESAMPLES)
    parser.add_argument("--seed", type=int, default=stat_tools.DEFAULT_SEED)
    parser.add_argument("--confidence-level", type=float, default=0.95)
    parser.add_argument(
        "--relative-output-dir",
        type=Path,
        default=Path("experiments/results/raw"),
        help="输出目录（相对仓库根）",
    )
    parser.add_argument("--label", default="paired_retrieval_stats", help="输出文件名前缀")
    args = parser.parse_args(argv)

    try:
        payload_a = load_result(args.result_a)
        payload_b = load_result(args.result_b)
        report = build_report(
            payload_a,
            payload_b,
            args.label_a,
            args.label_b,
            args.resamples,
            args.seed,
            args.confidence_level,
        )
    except PairingError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    output_dir = REPO_ROOT / args.relative_output_dir
    output_dir.mkdir(parents=True, exist_ok=True)
    json_path = output_dir / f"{args.label}.json"
    markdown_path = output_dir / f"{args.label}.md"
    json_path.write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    markdown_path.write_text(render_markdown(report), encoding="utf-8")

    print(f"[OK] 可配对查询数: {report['pairing']['paired_queries']}")
    print(
        f"[OK] 仅在 {args.label_a} 可评分: {report['pairing']['scorable_only_in_a_count']} | "
        f"仅在 {args.label_b} 可评分: {report['pairing']['scorable_only_in_b_count']}"
    )
    for name, payload in report["metrics"].items():
        if name == BINARY_METRIC[1]:
            payload = payload["difference"]
        if payload.get("mean_diff") is None:
            print(f"  {name}: n/a")
            continue
        crosses = "跨越0" if payload.get("crosses_zero") else "不跨越0"
        print(
            f"  {name}: {payload['mean_diff']:+.4f} "
            f"[{payload['ci_low']:+.4f}, {payload['ci_high']:+.4f}] ({crosses})"
        )
    mcnemar = report["metrics"][BINARY_METRIC[1]]["mcnemar_exact"]
    print(
        f"  McNemar: wins={mcnemar['wins']} losses={mcnemar['losses']} "
        f"two_sided_exact_p={mcnemar['two_sided_exact_p']}"
    )
    print(f"  Wilcoxon(MRR): two_sided_p={report['wilcoxon_mrr']['two_sided_p']}")
    print(f"  JSON: {json_path}")
    print(f"  Markdown: {markdown_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
