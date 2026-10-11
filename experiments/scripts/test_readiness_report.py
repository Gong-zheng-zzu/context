from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
REPORT = ROOT / "docs" / "competition" / "参赛就绪度检查-20261012.md"


class ReadinessReportTest(unittest.TestCase):
    def test_report_keeps_runtime_evidence_and_claim_boundaries(self):
        text = REPORT.read_text(encoding="utf-8")
        for marker in (
            "docker compose config --quiet",
            "results/smoke_live_20261012.json",
            "retrieval_metadata",
            "vector_only_fallback",
            "report_eligible=false",
            "Qwen2.5:3b",
            "隔离用户/会话跨存储删除验证",
            "双人复核",
        ):
            self.assertIn(marker, text)

    def test_report_does_not_promote_unverified_metrics(self):
        text = REPORT.read_text(encoding="utf-8")
        self.assertIn("不能直接当成绩页使用", text)
        self.assertIn("仍是问题基线", text)
        self.assertIn("仍处于待冻结评测状态", text)


if __name__ == "__main__":
    unittest.main()
