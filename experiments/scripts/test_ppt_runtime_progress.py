from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
REPORT = ROOT / "docs" / "competition" / "PPT功能落地进度-20261011.md"


class PptRuntimeProgressTest(unittest.TestCase):
    def test_report_preserves_evidence_boundaries(self):
        text = REPORT.read_text(encoding="utf-8")
        for marker in (
            "GET /api/v1/audit/traces/:trace_id",
            "GET /api/v1/evidence/summary",
            "report_eligible=false",
            "不称参数级机器遗忘",
            "auto_derived_unreviewed",
        ):
            self.assertIn(marker, text)


if __name__ == "__main__":
    unittest.main()
