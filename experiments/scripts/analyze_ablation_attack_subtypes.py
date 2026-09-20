#!/usr/bin/env python3
"""按 attack subtype 分列消融检出率，不改动门禁产物本身。

为什么需要它
------------
完整 profile 在内部 400 条上有 16 条漏检，全部来自 ``bypass_attack/semantic``
这一类：文本只是**描述**了敏感值（「手机号是 170 开头的十一位号码」），文中并不
存在可被模式匹配的完整值。把它们与「值被混淆」（空格分隔、同音字、Base64 等）
混在同一个检出率里，0.92 这个数字既不能说明能力、也不能指导改进。

本脚本从**已过门禁**的 raw 结果里做确定性重算：用 ``sample_id = pii_{category}_{id}``
回连数据集取 ``attack_type``（subtype），再按 subtype × profile 统计 ``decision == "redact"``，
输出 total / detected / missed / detection_rate 与 Wilson 95% 区间。

诚实边界
--------
* 输入必须是门禁产物；输出记录 ``source_result`` 与数据集 sha256，属**确定性重算**，
  不是新造数字（与 ``analyze_retrieval_failures.py`` 同构）。
* ``semantic`` 的 0/16 应读作「确定性管线（按设计排除 LLM 层）对语义指代**结构上**检不出」，
  **不是**「检测器有 bug」。要覆盖它需要另开含 LLM 的 profile 单独测量。
* 分母不完整（有样本回连不到 subtype）时整体抛 ``ValueError``——静默丢样本会让检出率无法追溯。
"""

from __future__ import annotations

import argparse
import hashlib
import io
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import stat_tools  # noqa: E402


def parse_source_id(sample_id: str) -> Optional[str]:
    """``pii_bypass_attack_185`` -> ``185``；``pii_standard_1`` -> ``1``。

    去掉 ``pii_`` 前缀后按最后一个 ``_`` 右切（category 自身含下划线，如
    ``bypass_attack``）。不符合该形状时返回 None，由调用方决定如何处理。
    """
    if not isinstance(sample_id, str) or not sample_id.startswith("pii_"):
        return None
    remainder = sample_id[len("pii_"):]
    if "_" not in remainder:
        return None
    return remainder.rsplit("_", 1)[1]


def load_dataset_index(dataset_path: str) -> Dict[str, str]:
    """读数据集 JSON 数组，返回 ``{str(id): attack_type}``，缺失字段兜底为 ``normal``。"""
    payload = json.loads(io.open(dataset_path, encoding="utf-8").read())
    if not isinstance(payload, list):
        raise ValueError("dataset must be a JSON array of samples")
    index: Dict[str, str] = {}
    for item in payload:
        if not isinstance(item, dict):
            continue
        source_id = item.get("id")
        if source_id is None:
            continue
        index[str(source_id)] = str(item.get("attack_type") or "normal")
    return index


def breakdown_by_subtype(rows, index, profile) -> Dict[str, Dict[str, Any]]:
    """按 subtype 统计某个 profile 在攻击样本上的检出/漏检。

    只统计 ``sample_kind == "attack"`` 的行；``decision == "redact"`` 记检出。
    任一攻击行回连不到 subtype 时抛 ``ValueError``，避免产出分母不完整的比率。
    """
    totals: Dict[str, int] = {}
    detected: Dict[str, int] = {}
    for row in rows:
        if row.get("sample_kind") != "attack":
            continue
        source_id = parse_source_id(row.get("sample_id") or "")
        if source_id is None or source_id not in index:
            raise ValueError(
                "cannot map sample %r to a dataset attack subtype; refusing to report "
                "a detection rate with an incomplete denominator" % (row.get("sample_id"),)
            )
        subtype = index[source_id]
        observation = (row.get("profiles") or {}).get(profile) or {}
        decision = str(observation.get("decision") or "").strip().casefold()
        totals[subtype] = totals.get(subtype, 0) + 1
        if decision == "redact":
            detected[subtype] = detected.get(subtype, 0) + 1

    breakdown: Dict[str, Dict[str, Any]] = {}
    for subtype in sorted(totals):
        total = totals[subtype]
        hits = detected.get(subtype, 0)
        rate = hits / total if total else 0.0
        breakdown[subtype] = {
            "total": total,
            "detected": hits,
            "missed": total - hits,
            "detection_rate": rate,
            "wilson_95_ci": list(stat_tools.wilson_ci(hits, total)) if total else None,
        }
    return breakdown


def _dataset_sha256(path: str) -> str:
    return hashlib.sha256(io.open(path, "rb").read()).hexdigest()


def _profiles_of(rows) -> List[str]:
    for row in rows:
        profiles = row.get("profiles")
        if isinstance(profiles, dict) and profiles:
            return sorted(profiles.keys())
    return []


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("result_json", help="门禁产出的 raw 消融结果 JSON")
    parser.add_argument("--dataset", default="test_data/test_data_400.json", help="用于回连 attack subtype 的数据集")
    parser.add_argument("--profile", default="", help="只报告该 profile；默认报告结果中出现的全部 profile")
    parser.add_argument("--output", default="", help="输出路径；默认 <结果文件名>_subtype_breakdown.json")
    args = parser.parse_args(argv)

    result = json.loads(io.open(args.result_json, encoding="utf-8").read())
    rows = result.get("detailed_results") or []
    if not rows:
        raise ValueError("result has no detailed_results")
    index = load_dataset_index(args.dataset)
    profiles = [args.profile] if args.profile else _profiles_of(rows)
    if not profiles:
        raise ValueError("no profiles found in the result")

    report: Dict[str, Any] = {
        "source_result": str(args.result_json),
        "dataset": {"path": str(args.dataset), "sha256": _dataset_sha256(args.dataset)},
        "profiles": {},
        "note": (
            "Deterministic re-computation over a gated raw result; nothing in the raw "
            "artifact is modified. 'semantic' rows describe a sensitive value without "
            "containing one, so a pattern-based pipeline (which excludes the LLM layer "
            "by design) cannot detect them -- read 0/N as a structural boundary, not a bug."
        ),
    }
    for profile in profiles:
        breakdown = breakdown_by_subtype(rows, index, profile)
        total = sum(bucket["total"] for bucket in breakdown.values())
        hits = sum(bucket["detected"] for bucket in breakdown.values())
        report["profiles"][profile] = {
            "by_attack_subtype": breakdown,
            "overall": {
                "total": total,
                "detected": hits,
                "missed": total - hits,
                "detection_rate": (hits / total) if total else 0.0,
            },
        }

    output = args.output or (str(Path(args.result_json).with_suffix("")) + "_subtype_breakdown.json")
    io.open(output, "w", encoding="utf-8").write(json.dumps(report, ensure_ascii=False, indent=2) + "\n")

    for profile in profiles:
        block = report["profiles"][profile]
        print("[%s] overall %d/%d = %.4f" % (
            profile, block["overall"]["detected"], block["overall"]["total"], block["overall"]["detection_rate"]))
        for subtype, bucket in block["by_attack_subtype"].items():
            print("    %-22s %3d/%-3d = %.4f  missed=%d" % (
                subtype, bucket["detected"], bucket["total"], bucket["detection_rate"], bucket["missed"]))
    print("wrote " + output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
