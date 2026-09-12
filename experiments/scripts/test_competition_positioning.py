#!/usr/bin/env python3
"""Keep competition copy aligned with the measured evidence boundary."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
POSITIONING = ROOT / "docs" / "competition" / "比赛定位与可信边界.md"
RUNBOOK = ROOT / "experiments" / "COMPETITION_DEMO_RUNBOOK.md"
INTERACTIVE = ROOT / "experiments" / "scripts" / "interactive_demo.sh"


class CompetitionPositioningTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.positioning = POSITIONING.read_text(encoding="utf-8")
        cls.runbook = RUNBOOK.read_text(encoding="utf-8")
        cls.interactive = INTERACTIVE.read_text(encoding="utf-8")

    def test_governance_first_title_and_boundaries_are_present(self):
        self.assertIn("护理数据智能治理与可信辅助决策平台", self.positioning)
        self.assertIn("不替代护工、医生", self.positioning)
        self.assertIn("离线夹具模式", self.positioning)
        self.assertIn("不宣称已经实现梯度投影式模型遗忘", self.positioning)

    def test_runbook_links_positioning_and_qualifies_deletion(self):
        self.assertIn("比赛定位与可信边界", self.runbook)
        self.assertIn("Qdrant 用户级向量删除验证", self.runbook)
        self.assertIn("-Mode fixture", self.runbook)
        self.assertIn("不能用于宣称实时检索、安全或因果指标", self.runbook)

    def test_interactive_copy_does_not_claim_gradient_unlearning(self):
        self.assertIn("用户级删除", self.interactive)
        self.assertIn("Qdrant 用户级向量删除", self.interactive)
        self.assertNotIn("梯度正交投影", self.interactive)


if __name__ == "__main__":
    unittest.main()
