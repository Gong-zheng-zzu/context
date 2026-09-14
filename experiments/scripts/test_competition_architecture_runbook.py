#!/usr/bin/env python3
"""Guard the competition architecture runbook against unsafe claim drift."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
RUNBOOK = ROOT / "docs" / "competition" / "比赛架构与可信演示手册.md"
INDEX = ROOT / "docs" / "competition" / "README.md"


class CompetitionArchitectureRunbookTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.runbook = RUNBOOK.read_text(encoding="utf-8")
        cls.index = INDEX.read_text(encoding="utf-8")

    def test_documents_describe_the_auditable_runtime_contract(self):
        for required in (
            "对抗式隐私识别与可验证记忆治理平台",
            "ASDF",
            "CASIA",
            "PCCM",
            "evaluationRetrievalOnly=true",
            "retrieval_metadata",
            "doc_id",
            "authoritative_web_search",
            "POST /api/v1/agent/execute",
            "write_applied=false",
        ):
            self.assertIn(required, self.runbook)

    def test_documents_keep_agent_and_fixture_safety_boundaries(self):
        for required in (
            "不会自动写入护理记录、发送通知、改变权限或执行删除",
            "必须提供文章的显式 HTTPS URL",
            "不能改用通用搜索引擎",
            "offline_fixture",
            "不能作为算法或性能结果",
            "不是梯度投影式模型遗忘",
        ):
            self.assertIn(required, self.runbook)

    def test_documents_require_release_and_metric_evidence_gates(self):
        for required in (
            "go mod verify",
            "go vet -tags http ./...",
            "go test -tags http ./...",
            "docker compose build",
            "数据集哈希",
            "配置指纹",
            "同清理流程",
        ):
            self.assertIn(required, self.runbook)
        self.assertNotRegex(self.runbook, re.compile(r"\b\d+(?:\.\d+)?%"))

    def test_competition_index_starts_with_the_current_runbook(self):
        self.assertIn("比赛架构与可信演示手册.md", self.index)
        self.assertLess(
            self.index.index("比赛架构与可信演示手册.md"),
            self.index.index("项目策划书.md"),
        )


if __name__ == "__main__":
    unittest.main()
