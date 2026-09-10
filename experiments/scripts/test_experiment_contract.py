#!/usr/bin/env python3
"""Offline regression checks for evidence gates and presentation-safe docs."""

from pathlib import Path
import unittest


EXPERIMENTS = Path(__file__).resolve().parents[1]


class ExperimentContractTest(unittest.TestCase):
    def test_full_runner_meets_report_builder_retrieval_denominator(self):
        runner = (EXPERIMENTS / "scripts" / "run_all.sh").read_text(encoding="utf-8")
        self.assertIn("retrieval_eval.py --queries 50", runner)
        self.assertIn("EVAL_ALLOW_RETRIEVAL_COMPARISON", runner)
        self.assertIn("--require security causal retrieval unlearning", runner)

    def test_readme_names_verified_observations_without_target_claims(self):
        readme = (EXPERIMENTS / "README.md").read_text(encoding="utf-8")
        self.assertIn("Evidence Gate", readme)
        self.assertIn("strict O-M-P-R tuple accuracy 2/20 (10.0%)", readme)
        self.assertIn("MRR 0.405", readme)
        self.assertNotIn("full_system | >0.80", readme)
        self.assertNotIn("full_system | >85%", readme)


if __name__ == "__main__":
    unittest.main()
