#!/usr/bin/env python3
"""Keep competition copy aligned with the measured evidence boundary."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
POSITIONING = ROOT / "docs" / "competition" / "比赛定位与可信边界.md"
RUNBOOK = ROOT / "experiments" / "COMPETITION_DEMO_RUNBOOK.md"
INTERACTIVE = ROOT / "experiments" / "scripts" / "interactive_demo.sh"
DEMO_GUIDE = ROOT / "docs" / "DEMO_GUIDE.md"
MEMORY_GRAPH = ROOT / "web" / "三维协同记忆图.html"
LEGACY_COMPETITION_COPY = (
    ROOT / "docs" / "competition" / "核心创新点总结.md",
    ROOT / "docs" / "competition" / "策划书-核心技术清单.md",
    ROOT / "docs" / "competition" / "对比实验报告.md",
    ROOT / "web" / "隐私保护.html",
)


class CompetitionPositioningTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.positioning = POSITIONING.read_text(encoding="utf-8")
        cls.runbook = RUNBOOK.read_text(encoding="utf-8")
        cls.interactive = INTERACTIVE.read_text(encoding="utf-8")
        cls.demo_guide = DEMO_GUIDE.read_text(encoding="utf-8")
        cls.memory_graph = MEMORY_GRAPH.read_text(encoding="utf-8")

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

    def test_legacy_demo_material_uses_verified_deletion_boundary(self):
        self.assertIn("用户级向量删除验证", self.demo_guide)
        self.assertNotIn("梯度正交投影遗忘", self.demo_guide)
        self.assertNotIn("final_fairness_loss", self.demo_guide)

    def test_memory_graph_does_not_show_unverified_accuracy_target(self):
        self.assertIn("来源可审计", self.memory_graph)
        self.assertNotIn("87.3%", self.memory_graph)

    def test_competition_copy_does_not_publish_legacy_metrics(self):
        forbidden = ("67.3%", "62.4%", "85.4%", "防御成功率95%", "准确率提升40%")
        for path in LEGACY_COMPETITION_COPY:
            text = path.read_text(encoding="utf-8")
            for phrase in forbidden:
                self.assertNotIn(phrase, text, f"legacy metric {phrase!r} remains in {path}")

    def test_competition_copy_qualifies_unlearning_claims(self):
        text = (ROOT / "docs" / "competition" / "对比实验报告.md").read_text(encoding="utf-8")
        self.assertIn("Qdrant/副本向量删除验证", text)
        self.assertIn("不称作梯度投影式模型遗忘", text)


if __name__ == "__main__":
    unittest.main()
