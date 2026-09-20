#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""共享统计原语，用于让评测结论带上可判定的不确定度与显著性。

背景
----
评测规模有限时，点估计之间的小差距无法说明「差异是否真实」。本模块提供三类
原语来回答这个问题：

* :func:`mcnemar_exact` —— 配对二分类结果的精确 McNemar 检验。
  适用于同一批样本上的二值判定（命中/未命中、redact/allow）。它正是回答
  「两种配置在同一批样本上是否真的不同」的检验。
* :func:`paired_bootstrap_ci` —— 配对差值均值的百分位 bootstrap 区间。
  适用于像倒数排名（MRR）这类非比例量：它们的逐样本取值不服从伯努利分布，
  比例检验不适用。
* :func:`mean_bootstrap_ci` / :func:`wilson_ci` —— 单组均值的区间与单组比例的
  Wilson 区间。

与既有实现的关系
----------------
``experiments/training/evaluate_activation.py`` 的 ``one_sided_paired_pvalue``
一直是本项目的单侧配对精确检验实现，其公式为
``sum(C(n, k) for k in range(wins, n + 1)) / 2 ** n``。本模块的
:func:`mcnemar_exact` 的 ``one_sided_exact_p`` 字段**刻意复刻同一公式与同一求和
顺序**，因此两者在小样本上逐位一致；``experiments/training/test_stat_tools_consistency.py``
固定了这项一致性，防止任何一侧被单独改动而悄悄漂移。

为什么不让评测链路直接 import 训练管线
--------------------------------------
``evaluate_activation`` 属于训练/激活审批链路，评测链路 import 它会把两个子系统
耦合起来。这里改为「独立实现 + 交叉测试」：语义一致由测试保证，耦合由测试避免。

仅使用标准库，不新增依赖。
"""

from __future__ import annotations

import math
import random
from statistics import fmean
from typing import Dict, List, Optional, Sequence

__all__ = [
    "MAX_EXACT_TRIALS",
    "DEFAULT_RESAMPLES",
    "DEFAULT_SEED",
    "WILSON_Z_95",
    "percentile",
    "mcnemar_exact",
    "wilson_ci",
    "mean_bootstrap_ci",
    "paired_bootstrap_ci",
]

# 精确整数算术的上限。超过它时 2 ** trials 会溢出为浮点，需要退回对数空间。
# 本项目最大评测规模为数百个样本，正常路径永远不会触碰到这个分支。
MAX_EXACT_TRIALS = 1000

DEFAULT_RESAMPLES = 10000
DEFAULT_SEED = 20260920

# 与 security_eval.py 的 WILSON_Z_95 保持一致，使两处区间在同一批数据上可比。
WILSON_Z_95 = 1.959963984540054


def percentile(values: Sequence[float], quantile: float) -> float:
    """线性插值分位数，``quantile`` 取 0 到 1。

    与 ``RetrievalEvaluator._percentile`` 的插值约定一致（按 ``(n - 1) * q`` 定位
    并在相邻两点间线性插值），使新旧延迟分位数不会因为算法差异而漂移。

    ``values`` 必须已升序排序。
    """
    if not values:
        raise ValueError("percentile requires at least one value")
    if not 0.0 <= quantile <= 1.0:
        raise ValueError("quantile must be within [0, 1]")
    if len(values) == 1:
        return float(values[0])
    position = (len(values) - 1) * quantile
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return float(values[lower])
    weight = position - lower
    return float(values[lower]) + (float(values[upper]) - float(values[lower])) * weight


def _binomial_tail_probability(trials: int, start: int) -> float:
    """``P(X >= start)``，其中 ``X ~ Binomial(trials, 0.5)``。

    小样本走精确整数求和（与 ``one_sided_paired_pvalue`` 完全一致）；超出
    :data:`MAX_EXACT_TRIALS` 时改为对数空间求和，避免 ``2 ** trials`` 溢出。
    """
    if trials < 0:
        raise ValueError("trials must be non-negative")
    if start <= 0:
        return 1.0
    if start > trials:
        return 0.0
    if trials <= MAX_EXACT_TRIALS:
        return sum(math.comb(trials, k) for k in range(start, trials + 1)) / (2 ** trials)
    log_two = math.log(2.0)
    total = math.fsum(
        math.exp(
            math.lgamma(trials + 1)
            - math.lgamma(k + 1)
            - math.lgamma(trials - k + 1)
            - trials * log_two
        )
        for k in range(start, trials + 1)
    )
    return min(1.0, max(0.0, total))


def mcnemar_exact(wins: int, losses: int) -> Dict[str, object]:
    """配对二分类结果的精确 McNemar 检验。

    参数语义（沿用 McNemar 的经典记号）：
      * ``wins``   —— 传统 2x2 表的 **b**：新配置正确、基线错误的对数；
      * ``losses`` —— 传统 2x2 表的 **c**：新配置错误、基线正确的对数。

    返回字段：
      * ``wins`` / ``losses`` / ``discordant``（= b + c）；
      * ``one_sided_exact_p`` —— 上尾 p 值 ``P(X >= wins)``，与
        ``one_sided_paired_pvalue`` 的 ``one_sided_exact_p`` 逐位一致；
      * ``two_sided_exact_p`` —— 以 ``max(wins, losses)`` 为上尾起点的双侧 p 值，
        上限截断到 1.0，即 ``2 * P(X >= max(b, c))``；
      * ``method`` —— 实际使用的算术路径，便于结果自描述。
    """
    if not isinstance(wins, int) or not isinstance(losses, int):
        raise TypeError("wins and losses must be integers")
    if wins < 0 or losses < 0:
        raise ValueError("wins and losses must be non-negative")

    discordant = wins + losses
    if discordant == 0:
        return {
            "wins": wins,
            "losses": losses,
            "discordant": 0,
            "one_sided_exact_p": 1.0,
            "two_sided_exact_p": 1.0,
            "method": "exact_binomial_no_discordant_pairs",
        }

    one_sided = _binomial_tail_probability(discordant, wins)
    two_sided = min(1.0, 2.0 * _binomial_tail_probability(discordant, max(wins, losses)))
    return {
        "wins": wins,
        "losses": losses,
        "discordant": discordant,
        "one_sided_exact_p": one_sided,
        "two_sided_exact_p": two_sided,
        "method": (
            "exact_binomial_integer_arithmetic"
            if discordant <= MAX_EXACT_TRIALS
            else "exact_binomial_log_space"
        ),
    }


def wilson_ci(
    numerator: int,
    denominator: int,
    z: float = WILSON_Z_95,
) -> Optional[List[float]]:
    """比例的双侧 Wilson 得分区间，公式与 ``security_eval._rate_estimate`` 相同。

    ``denominator`` 为 0 时返回 ``None``（区间无定义），与既有实现一致。
    """
    if denominator < 0 or numerator < 0:
        raise ValueError("numerator and denominator must be non-negative")
    if denominator == 0:
        return None
    if numerator > denominator:
        raise ValueError("numerator cannot exceed denominator")

    rate = numerator / denominator
    denominator_float = float(denominator)
    z_squared = z ** 2
    center = (rate + z_squared / (2 * denominator_float)) / (1 + z_squared / denominator_float)
    margin = (
        z
        * (
            rate * (1 - rate) / denominator_float
            + z_squared / (4 * denominator_float ** 2)
        ) ** 0.5
        / (1 + z_squared / denominator_float)
    )
    return [max(0.0, center - margin), min(1.0, center + margin)]


def _interval_from_samples(
    samples: List[float],
    confidence_level: float,
) -> Dict[str, Optional[float]]:
    if not samples:
        return {"ci_low": None, "ci_high": None}
    samples.sort()
    alpha = 1.0 - confidence_level
    return {
        "ci_low": percentile(samples, alpha / 2.0),
        "ci_high": percentile(samples, 1.0 - alpha / 2.0),
    }


def mean_bootstrap_ci(
    values: Sequence[float],
    resamples: int = DEFAULT_RESAMPLES,
    confidence_level: float = 0.95,
    seed: int = DEFAULT_SEED,
) -> Dict[str, object]:
    """单组均值（或比例）的百分位 bootstrap 区间。

    使用 ``random.Random(seed)`` 与重采样序列，因此同一输入与同一 seed 必然产生
    同一区间——评测结果可复现。
    """
    if not 0.0 < confidence_level < 1.0:
        raise ValueError("confidence_level must be within (0, 1)")
    if resamples <= 0:
        raise ValueError("resamples must be positive")

    observations = [float(value) for value in values]
    point = fmean(observations) if observations else None
    if not observations:
        return {
            "point": None,
            "ci_low": None,
            "ci_high": None,
            "sample_size": 0,
            "resamples": resamples,
            "seed": seed,
            "confidence_level": confidence_level,
        }

    rng = random.Random(seed)
    size = len(observations)
    statistics_of_resamples = [
        fmean(rng.choices(observations, k=size)) for _ in range(resamples)
    ]
    interval = _interval_from_samples(statistics_of_resamples, confidence_level)
    return {
        "point": point,
        "ci_low": interval["ci_low"],
        "ci_high": interval["ci_high"],
        "sample_size": size,
        "resamples": resamples,
        "seed": seed,
        "confidence_level": confidence_level,
    }


def paired_bootstrap_ci(
    differences: Sequence[float],
    resamples: int = DEFAULT_RESAMPLES,
    confidence_level: float = 0.95,
    seed: int = DEFAULT_SEED,
) -> Dict[str, object]:
    """配对差值均值的百分位 bootstrap 区间。

    ``differences`` 应当是**逐样本配对**后的差值（新配置指标减基线指标），长度等于
    两侧共同可评分的样本数。配对是这里的要点：只有逐样本配对才能消掉样本难度带来
    的方差，从而在样本量有限时把「真实差异」与「样本波动」区分开。

    区间不跨越 0 才说明差异在给定置信水平下不与「无差异」相容。
    """
    if not 0.0 < confidence_level < 1.0:
        raise ValueError("confidence_level must be within (0, 1)")
    if resamples <= 0:
        raise ValueError("resamples must be positive")

    deltas = [float(value) for value in differences]
    if not deltas:
        return {
            "mean_diff": None,
            "ci_low": None,
            "ci_high": None,
            "paired_sample_size": 0,
            "resamples": resamples,
            "seed": seed,
            "confidence_level": confidence_level,
            "crosses_zero": None,
        }

    rng = random.Random(seed)
    size = len(deltas)
    statistics_of_resamples = [
        fmean(rng.choices(deltas, k=size)) for _ in range(resamples)
    ]
    interval = _interval_from_samples(statistics_of_resamples, confidence_level)
    mean_diff = fmean(deltas)
    ci_low = interval["ci_low"]
    ci_high = interval["ci_high"]
    return {
        "mean_diff": mean_diff,
        "ci_low": ci_low,
        "ci_high": ci_high,
        "paired_sample_size": size,
        "resamples": resamples,
        "seed": seed,
        "confidence_level": confidence_level,
        "crosses_zero": (
            None if ci_low is None or ci_high is None else (ci_low <= 0.0 <= ci_high)
        ),
    }
