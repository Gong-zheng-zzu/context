#!/usr/bin/env python3
"""Contract checks for the competition experiment protocol."""

import json
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]


class CompetitionExperimentProtocolTests(unittest.TestCase):
    def test_protocol_declares_truthful_claim_boundaries(self):
        protocol = (ROOT / "docs/competition/强网杯实验协议与证据门禁.md").read_text(encoding="utf-8")
        for phrase in ("历史基线", "不可宣称事项", "删除证明", "研究 Agent"):
            self.assertIn(phrase, protocol)
        self.assertIn("是检出率，不是准确率或 F1", protocol)

    def test_config_and_manifest_are_not_formal_results(self):
        config = (ROOT / "experiments/configs/competition_experiment_v1.yaml").read_text(encoding="utf-8")
        self.assertIn("historical_results_are_formal: false", config)
        manifest = json.loads(
            (ROOT / "experiments/datasets/annotation_manifest_v1.json").read_text(encoding="utf-8")
        )
        self.assertFalse(manifest["eligible_for_formal_report"])
        self.assertEqual(2, manifest["review_policy"]["minimum_independent_annotators"])


if __name__ == "__main__":
    unittest.main()
