#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""analyze_ablation_attack_subtypes.py 的离线回归测试（全部使用合成数据）。

钉住的行为
----------
* ``parse_source_id`` 的切分规则：结果行的 ``sample_id`` 与数据集条目 ``id``
  全靠它对齐。对齐错了分子分母会一起错，而且不会报错。
* ``breakdown_by_subtype`` 遇到索引里查不到的 source id 必须整体抛
  ``ValueError``。漏检数（如 16 条 semantic 全漏）只有在分母完整时才可解释；
  静默丢样本会把「检出率 96%」变成无法追溯的数字。
* 已知事实：semantic 全漏时该 subtype 的 ``detection_rate`` 为 0.0，其余
  subtype 为 1.0，且全部漏检都落在 semantic 上。

不读取 experiments/results/raw 下的真实结果文件，也不访问网络。
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from analyze_ablation_attack_subtypes import (  # noqa: E402
    breakdown_by_subtype,
    load_dataset_index,
    main,
    parse_source_id,
)

PROFILE = "asdf_casia_pccm"
# CLI 用例让每个 profile 共用同一 decision，这样无论 main 默认挑哪个 profile，
# 分列结果都一致。
ALL_PROFILES = ("regex_only", "asdf", "asdf_casia", "asdf_casia_pccm")

SEMANTIC = "bypass_attack/semantic"
BASE64 = "bypass_attack/base64"
PHONE = "standard/phone"
ORDINARY = "normal/ordinary_prose"


def row(sample_id, decision, sample_kind="attack", profiles=ALL_PROFILES):
    """构造一条 detailed_results 行：各 profile 共用同一 decision。"""
    return {
        "sample_id": sample_id,
        "sample_kind": sample_kind,
        "profiles": {name: {"decision": decision} for name in profiles},
    }


def attack(sample_id, decision, profiles=ALL_PROFILES):
    return row(sample_id, decision, "attack", profiles)


def benign(sample_id, profiles=ALL_PROFILES):
    return row(sample_id, "allow", "benign", profiles)


def index_of(mapping):
    """把 {source_id: subtype} 变成 load_dataset_index 的返回形状。"""
    return {str(source_id): subtype for source_id, subtype in mapping.items()}


def semantic_miss_fixture():
    """合成「16 条 semantic 全漏检，其余 subtype 全检出」的 rows + index。"""
    index = {}
    rows = []
    for offset in range(16):
        source_id = str(101 + offset)
        index[source_id] = SEMANTIC
        rows.append(attack("pii_bypass_attack_%s" % source_id, "allow"))
    for offset in range(5):
        source_id = str(201 + offset)
        index[source_id] = BASE64
        rows.append(attack("pii_bypass_attack_%s" % source_id, "redact"))
    for offset in range(3):
        source_id = str(301 + offset)
        index[source_id] = PHONE
        rows.append(attack("pii_standard_%s" % source_id, "redact"))
    return rows, index


class ParseSourceIdTests(unittest.TestCase):
    def test_keeps_trailing_id_after_stripping_pii_prefix(self):
        self.assertEqual("185", parse_source_id("pii_bypass_attack_185"))
        self.assertEqual("1", parse_source_id("pii_standard_1"))
        self.assertEqual("007", parse_source_id("pii_bypass_attack_007"))

    def test_returns_none_without_pii_prefix(self):
        for sample_id in ("bypass_attack_185", "sample_185", "PII_bypass_attack_185"):
            with self.subTest(sample_id=sample_id):
                self.assertIsNone(parse_source_id(sample_id))

    def test_returns_none_without_separator(self):
        for sample_id in ("pii185", ""):
            with self.subTest(sample_id=sample_id):
                self.assertIsNone(parse_source_id(sample_id))


class LoadDatasetIndexTests(unittest.TestCase):
    def test_keys_are_stringified_dataset_ids(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "dataset.json"
            path.write_text(
                json.dumps(
                    [
                        {"id": 185, "attack_type": SEMANTIC},
                        {"id": "1", "attack_type": PHONE},
                    ],
                    ensure_ascii=False,
                ),
                encoding="utf-8",
            )
            index = load_dataset_index(str(path))
        self.assertEqual({"185": SEMANTIC, "1": PHONE}, index)

    def test_missing_or_empty_attack_type_falls_back_to_normal(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "dataset.json"
            path.write_text(
                json.dumps(
                    [
                        {"id": "1"},
                        {"id": "2", "attack_type": ""},
                        {"id": "3", "attack_type": None},
                        {"id": "4", "attack_type": BASE64},
                    ],
                    ensure_ascii=False,
                ),
                encoding="utf-8",
            )
            index = load_dataset_index(str(path))
        self.assertEqual({"1": "normal", "2": "normal", "3": "normal", "4": BASE64}, index)


class BreakdownBySubtypeTests(unittest.TestCase):
    def test_counts_attack_rows_only_and_splits_by_subtype(self):
        index = index_of(
            {"101": SEMANTIC, "102": SEMANTIC, "201": BASE64, "202": BASE64, "301": ORDINARY}
        )
        rows = [
            attack("pii_bypass_attack_101", "redact"),
            attack("pii_bypass_attack_102", "allow"),
            attack("pii_bypass_attack_201", "redact"),
            attack("pii_bypass_attack_202", "redact"),
            benign("pii_normal_301"),
        ]
        breakdown = breakdown_by_subtype(rows, index, PROFILE)

        # benign 行不进分母，所以 ORDINARY 不该出现。
        self.assertEqual({SEMANTIC, BASE64}, set(breakdown))
        semantic = breakdown[SEMANTIC]
        self.assertEqual(2, semantic["total"])
        self.assertEqual(1, semantic["detected"])
        self.assertEqual(1, semantic["missed"])
        self.assertEqual(0.5, semantic["detection_rate"])
        base64_bucket = breakdown[BASE64]
        self.assertEqual(2, base64_bucket["total"])
        self.assertEqual(2, base64_bucket["detected"])
        self.assertEqual(0, base64_bucket["missed"])
        self.assertEqual(1.0, base64_bucket["detection_rate"])

        for subtype, bucket in breakdown.items():
            with self.subTest(subtype=subtype):
                interval = bucket["wilson_95_ci"]
                self.assertEqual(2, len(interval))
                self.assertGreaterEqual(interval[0], 0.0)
                self.assertLessEqual(interval[1], 1.0)
                self.assertLessEqual(interval[0], bucket["detection_rate"])
                self.assertGreaterEqual(interval[1], bucket["detection_rate"])

    def test_unknown_source_id_raises_value_error(self):
        """关键断言：分母不完整必须整体失败，不能静默丢样本。"""
        index = index_of({"101": SEMANTIC})
        rows = [
            attack("pii_bypass_attack_101", "redact"),
            attack("pii_bypass_attack_999", "redact"),
        ]
        with self.assertRaises(ValueError):
            breakdown_by_subtype(rows, index, PROFILE)

    def test_sixteen_semantic_misses_are_the_only_misses(self):
        rows, index = semantic_miss_fixture()
        breakdown = breakdown_by_subtype(rows, index, PROFILE)

        self.assertEqual({SEMANTIC, BASE64, PHONE}, set(breakdown))
        semantic = breakdown[SEMANTIC]
        self.assertEqual(16, semantic["total"])
        self.assertEqual(0, semantic["detected"])
        self.assertEqual(16, semantic["missed"])
        self.assertEqual(0.0, semantic["detection_rate"])
        # 全漏时 Wilson 下界贴 0，上界仍 > 0：不能读成「已证明不可能检出」。
        self.assertEqual(0.0, semantic["wilson_95_ci"][0])
        self.assertGreater(semantic["wilson_95_ci"][1], 0.0)

        self.assertEqual(1.0, breakdown[BASE64]["detection_rate"])
        self.assertEqual(5, breakdown[BASE64]["total"])
        self.assertEqual(0, breakdown[BASE64]["missed"])
        self.assertEqual(1.0, breakdown[PHONE]["detection_rate"])
        self.assertEqual(3, breakdown[PHONE]["total"])
        self.assertEqual(0, breakdown[PHONE]["missed"])

        self.assertEqual(24, sum(bucket["total"] for bucket in breakdown.values()))
        self.assertEqual(8, sum(bucket["detected"] for bucket in breakdown.values()))
        self.assertEqual(16, sum(bucket["missed"] for bucket in breakdown.values()))

    def test_no_attack_rows_yields_empty_breakdown(self):
        index = index_of({"101": SEMANTIC})
        self.assertEqual({}, breakdown_by_subtype([], index, PROFILE))
        self.assertEqual({}, breakdown_by_subtype([benign("pii_normal_101")], index, PROFILE))


class MainCliTests(unittest.TestCase):
    def test_main_returns_zero_and_writes_subtype_report(self):
        rows, index = semantic_miss_fixture()
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            dataset_path = base / "dataset.json"
            dataset_path.write_text(
                json.dumps(
                    [{"id": source_id, "attack_type": subtype} for source_id, subtype in index.items()],
                    ensure_ascii=False,
                ),
                encoding="utf-8",
            )
            result_path = base / "result.json"
            result_path.write_text(
                json.dumps({"detailed_results": rows}, ensure_ascii=False), encoding="utf-8"
            )
            output_path = base / "subtypes.json"
            self.assertEqual(
                0,
                main([str(result_path), "--dataset", str(dataset_path), "--output", str(output_path)]),
            )
            payload = json.loads(output_path.read_text(encoding="utf-8"))
        serialized = json.dumps(payload, ensure_ascii=False)
        self.assertIn(SEMANTIC, serialized)
        self.assertIn(BASE64, serialized)
        self.assertIn(PHONE, serialized)


if __name__ == "__main__":
    unittest.main()
