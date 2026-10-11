import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
CLAIMS = ROOT / "docs" / "competition" / "ppt_experiment_claims.json"
REPORT = ROOT / "docs" / "competition" / "PPT实验部分可信性审查报告-20261011.md"


class PptExperimentClaimsTests(unittest.TestCase):
    def test_claim_manifest_has_explicit_evidence_boundaries(self):
        payload = json.loads(CLAIMS.read_text(encoding="utf-8"))
        self.assertEqual(payload["schema_version"], "1.0")
        self.assertGreaterEqual(len(payload["claims"]), 6)
        required = {"id", "slides", "statement", "evidence_level", "formal_result", "source"}
        for claim in payload["claims"]:
            self.assertTrue(required.issubset(claim))
            self.assertTrue(claim["slides"])
            self.assertTrue(claim["source"])
        self.assertTrue(all(not c["formal_result"] for c in payload["claims"]))

    def test_report_mentions_historical_boundaries_and_gate(self):
        text = REPORT.read_text(encoding="utf-8")
        for marker in ("历史问题基线", "受控初步", "Docker Linux", "94.22%", "0.5128", "0.4172"):
            self.assertIn(marker, text)


if __name__ == "__main__":
    unittest.main()
