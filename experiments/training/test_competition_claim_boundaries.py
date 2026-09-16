#!/usr/bin/env python3
"""Regression tests for evidence boundaries in competition material."""

import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


class CompetitionClaimBoundaryTests(unittest.TestCase):
    def test_direct_competition_material_avoids_unsupported_claims(self) -> None:
        targets = [
            ROOT / "docs" / "competition" / "核心创新点总结.md",
            ROOT / "docs" / "competition" / "策划书-核心技术清单.md",
            ROOT / "docs" / "competition" / "项目完善清单.md",
        ]
        unsupported = [
            "首次将AI安全领域的对抗学习思想应用于敏感信息检测",
            "防御成功率：95%+",
            "可发表论文",
            "博士级别",
            "降维打击",
            "PCCM的权重通过梯度下降法自适应优化",
        ]
        for path in targets:
            content = path.read_text(encoding="utf-8")
            for claim in unsupported:
                with self.subTest(path=path.name, claim=claim):
                    self.assertNotIn(claim, content)

    def test_modification_report_labels_metrics_as_historical(self) -> None:
        report = (ROOT / "docs" / "competition" / "比赛主链创新增强改造报告.md").read_text(encoding="utf-8")
        for value in ["93.33%", "80%", "F1 0.513", "10613.65 ms"]:
            self.assertIn(value, report)
        self.assertIn("历史运行", report)
        self.assertIn("不能视为本轮代码的当前成绩", report)
        self.assertIn("synthetic_unreviewed", report)


if __name__ == "__main__":
    unittest.main()
