#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""固定 ``experiments/scripts/stat_tools.py`` 与既有统计实现的一致性。

为什么需要这个测试
------------------
评测链路需要 bootstrap 区间与 McNemar 检验，但训练/激活链路早已有单侧配对精确
检验（``evaluate_activation.one_sided_paired_pvalue``）。两处若各自演化，同一批
数据会算出不同的 p 值，而「显著性」这类结论一旦口径分裂就无法辩护。

因此这里不采用跨子系统 import（那会把评测链路耦合到训练管线），而是让
``stat_tools`` 独立实现后，用本测试**逐位钉住**两者的等价性：任何一侧被单独改动
都会在此失败。

同时用不依赖外部样本的性质断言覆盖边界情形（无不一致对、单侧退化、大样本对数
空间路径），以及 bootstrap 的可复现性与退化输入。
"""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

TRAINING_DIR = Path(__file__).resolve().parent
SCRIPTS_DIR = TRAINING_DIR.parent / "scripts"
for candidate in (str(TRAINING_DIR), str(SCRIPTS_DIR)):
    if candidate not in sys.path:
        sys.path.insert(0, candidate)

import stat_tools  # noqa: E402
from evaluate_activation import one_sided_paired_pvalue  # noqa: E402


def paired_outcomes(wins: int, losses: int) -> tuple[dict[str, bool], dict[str, bool]]:
    """把 (wins, losses) 展开成两个 ``record_id -> bool`` 映射。

    ``wins`` 表示「custom 正确且 baseline 错误」，``losses`` 反之。未参与不一致对
    的样本两侧取值相同，因此不影响 discordant 计数。
    """
    custom: dict[str, bool] = {}
    baseline: dict[str, bool] = {}
    for index in range(wins):
        key = f"win_{index}"
        custom[key] = True
        baseline[key] = False
    for index in range(losses):
        key = f"loss_{index}"
        custom[key] = False
        baseline[key] = True
    return custom, baseline


class McNemarConsistencyTests(unittest.TestCase):
    """``stat_tools.mcnemar_exact`` 必须与 ``one_sided_paired_pvalue`` 逐位一致。"""

    FIXTURES = (
        (0, 0),
        (1, 0),
        (0, 1),
        (1, 1),
        (5, 2),
        (2, 5),
        (7, 0),
        (0, 7),
        (10, 10),
        (33, 20),
        (20, 33),
        (64, 1),
    )

    def test_one_sided_p_value_matches_training_pipeline_implementation(self) -> None:
        for wins, losses in self.FIXTURES:
            with self.subTest(wins=wins, losses=losses):
                custom, baseline = paired_outcomes(wins, losses)
                reference = one_sided_paired_pvalue(custom, baseline)
                ours = stat_tools.mcnemar_exact(wins, losses)

                self.assertEqual(reference["wins"], ours["wins"])
                self.assertEqual(reference["losses"], ours["losses"])
                self.assertEqual(reference["discordant"], ours["discordant"])
                # 逐位相等：同一公式、同一求和顺序，不允许有浮点漂移。
                self.assertEqual(
                    reference["one_sided_exact_p"],
                    ours["one_sided_exact_p"],
                    msg=f"one-sided p drifted for wins={wins} losses={losses}",
                )

    def test_no_discordant_pairs_yields_p_one(self) -> None:
        result = stat_tools.mcnemar_exact(0, 0)
        self.assertEqual(result["discordant"], 0)
        self.assertEqual(result["one_sided_exact_p"], 1.0)
        self.assertEqual(result["two_sided_exact_p"], 1.0)

    def test_two_sided_p_is_symmetric_in_the_pair(self) -> None:
        for wins, losses in ((5, 2), (7, 0), (10, 10), (33, 20)):
            with self.subTest(wins=wins, losses=losses):
                forward = stat_tools.mcnemar_exact(wins, losses)
                backward = stat_tools.mcnemar_exact(losses, wins)
                # 双侧检验只取决于 |b - c| 与 b + c，因此交换方向不应改变 p 值。
                self.assertEqual(
                    forward["two_sided_exact_p"], backward["two_sided_exact_p"]
                )

    def test_two_sided_p_is_capped_at_one(self) -> None:
        for wins, losses in self.FIXTURES:
            with self.subTest(wins=wins, losses=losses):
                result = stat_tools.mcnemar_exact(wins, losses)
                self.assertLessEqual(result["two_sided_exact_p"], 1.0)
                self.assertGreaterEqual(result["two_sided_exact_p"], 0.0)

    def test_favouring_tail_makes_one_sided_p_smaller(self) -> None:
        # wins 越多，上尾 p 越小；这是「新配置更好」方向应有的单调性。
        p_values = [stat_tools.mcnemar_exact(wins, 0)["one_sided_exact_p"] for wins in range(1, 9)]
        self.assertEqual(p_values, sorted(p_values, reverse=True))

    def test_negative_counts_are_rejected(self) -> None:
        with self.assertRaises(ValueError):
            stat_tools.mcnemar_exact(-1, 0)
        with self.assertRaises(ValueError):
            stat_tools.mcnemar_exact(0, -1)

    def test_non_integer_counts_are_rejected(self) -> None:
        with self.assertRaises(TypeError):
            stat_tools.mcnemar_exact(1.0, 0)  # type: ignore[arg-type]


class ExactArithmeticBoundaryTests(unittest.TestCase):
    """超出精确整数范围时退回对数空间，且不得抛错或产生非法概率。"""

    def test_log_space_fallback_stays_within_unit_interval(self) -> None:
        trials = stat_tools.MAX_EXACT_TRIALS + 500
        probability = stat_tools._binomial_tail_probability(trials, trials // 2)
        self.assertGreater(probability, 0.0)
        self.assertLess(probability, 1.0)
        # 对半分时上尾概率应略大于 0.5。
        self.assertAlmostEqual(probability, 0.5, delta=0.05)

    def test_reported_method_switches_only_beyond_the_exact_boundary(self) -> None:
        small = stat_tools.mcnemar_exact(stat_tools.MAX_EXACT_TRIALS // 2, 0)
        self.assertEqual(small["method"], "exact_binomial_integer_arithmetic")


class WilsonIntervalTests(unittest.TestCase):
    """Wilson 区间必须与 ``security_eval`` 的既有实现一致（若该模块可导入）。"""

    def test_matches_security_eval_implementation_when_available(self) -> None:
        try:
            from security_eval import SecurityAblationEvaluator
        except Exception as exc:  # pragma: no cover - 取决于环境是否装齐脚本依赖
            self.skipTest(f"security_eval not importable in this environment: {exc}")

        for numerator, denominator in ((0, 10), (1, 10), (5, 10), (9, 10), (10, 10), (7, 13)):
            with self.subTest(numerator=numerator, denominator=denominator):
                reference = SecurityAblationEvaluator._rate_estimate(numerator, denominator)
                ours = stat_tools.wilson_ci(numerator, denominator)
                self.assertIsNotNone(reference.wilson_95_ci)
                self.assertIsNotNone(ours)
                for expected, actual in zip(reference.wilson_95_ci, ours):  # type: ignore[arg-type]
                    self.assertAlmostEqual(expected, actual, places=12)

    def test_zero_denominator_has_no_interval(self) -> None:
        self.assertIsNone(stat_tools.wilson_ci(0, 0))

    def test_interval_brackets_the_point_rate(self) -> None:
        interval = stat_tools.wilson_ci(3, 10)
        self.assertIsNotNone(interval)
        low, high = interval  # type: ignore[misc]
        self.assertLessEqual(low, 0.3)
        self.assertGreaterEqual(high, 0.3)


class BootstrapTests(unittest.TestCase):
    def test_mean_interval_is_degenerate_for_constant_input(self) -> None:
        result = stat_tools.mean_bootstrap_ci([3.0, 3.0, 3.0, 3.0], resamples=200, seed=7)
        self.assertEqual(result["point"], 3.0)
        self.assertEqual(result["ci_low"], 3.0)
        self.assertEqual(result["ci_high"], 3.0)

    def test_paired_interval_is_degenerate_for_constant_input(self) -> None:
        result = stat_tools.paired_bootstrap_ci([0.5] * 6, resamples=200, seed=7)
        self.assertEqual(result["mean_diff"], 0.5)
        self.assertEqual(result["ci_low"], 0.5)
        self.assertEqual(result["ci_high"], 0.5)
        self.assertFalse(result["crosses_zero"])

    def test_zero_interval_detects_no_difference(self) -> None:
        # 正负对称的差值，其均值区间必然覆盖 0。
        result = stat_tools.paired_bootstrap_ci([1.0, -1.0] * 10, resamples=2000, seed=11)
        self.assertIsNotNone(result["mean_diff"])
        self.assertAlmostEqual(result["mean_diff"], 0.0, places=12)
        self.assertTrue(result["crosses_zero"])

    def test_same_seed_reproduces_the_same_interval(self) -> None:
        values = [0.1, 0.4, 0.9, 0.2, 0.7, 0.3]
        first = stat_tools.mean_bootstrap_ci(values, resamples=500, seed=123)
        second = stat_tools.mean_bootstrap_ci(values, resamples=500, seed=123)
        self.assertEqual(first, second)

    def test_different_seeds_do_not_change_the_point_estimate(self) -> None:
        values = [0.1, 0.4, 0.9, 0.2, 0.7, 0.3]
        first = stat_tools.mean_bootstrap_ci(values, resamples=500, seed=1)
        second = stat_tools.mean_bootstrap_ci(values, resamples=500, seed=2)
        self.assertEqual(first["point"], second["point"])
        self.assertEqual(first["sample_size"], second["sample_size"])

    def test_empty_input_yields_no_interval(self) -> None:
        result = stat_tools.paired_bootstrap_ci([])
        self.assertIsNone(result["mean_diff"])
        self.assertIsNone(result["ci_low"])
        self.assertIsNone(result["crosses_zero"])
        self.assertEqual(result["paired_sample_size"], 0)

    def test_invalid_confidence_level_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            stat_tools.mean_bootstrap_ci([1.0], confidence_level=1.0)
        with self.assertRaises(ValueError):
            stat_tools.paired_bootstrap_ci([1.0], confidence_level=0.0)

    def test_invalid_resample_count_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            stat_tools.mean_bootstrap_ci([1.0], resamples=0)


class PercentileTests(unittest.TestCase):
    def test_matches_retrieval_evaluator_interpolation(self) -> None:
        # 与 RetrievalEvaluator._percentile 的插值约定一致：按 (n-1) * q 线性插值。
        from retrieval_eval import RetrievalEvaluator

        values = [1.0, 2.0, 3.0, 4.0, 5.0]
        for percentile_value in (0, 25, 50, 75, 100):
            with self.subTest(percentile=percentile_value):
                expected = RetrievalEvaluator._percentile(list(values), percentile_value)
                actual = stat_tools.percentile(values, percentile_value / 100.0)
                self.assertAlmostEqual(expected, actual, places=12)

    def test_single_value_is_returned_unchanged(self) -> None:
        self.assertEqual(stat_tools.percentile([2.5], 0.5), 2.5)

    def test_empty_input_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            stat_tools.percentile([], 0.5)

    def test_out_of_range_quantile_is_rejected(self) -> None:
        with self.assertRaises(ValueError):
            stat_tools.percentile([1.0, 2.0], 1.5)


if __name__ == "__main__":
    unittest.main()
