#!/usr/bin/env python3
"""Offline regression checks for evidence gates and presentation-safe docs."""

from pathlib import Path
import re
import unittest


EXPERIMENTS = Path(__file__).resolve().parents[1]


class ExperimentContractTest(unittest.TestCase):
    def test_docker_builder_matches_declared_go_version(self):
        go_mod = (EXPERIMENTS.parent / "go.mod").read_text(encoding="utf-8")
        dockerfile = (EXPERIMENTS.parent / "Dockerfile").read_text(encoding="utf-8")
        declared = re.search(r"^go\s+(\d+\.\d+)", go_mod, re.MULTILINE)
        builder = re.search(r"^FROM golang:(\d+\.\d+)-alpine", dockerfile, re.MULTILINE)
        self.assertIsNotNone(declared)
        self.assertIsNotNone(builder)
        self.assertEqual(builder.group(1), declared.group(1))

    def test_full_runner_meets_report_builder_retrieval_denominator(self):
        runner = (EXPERIMENTS / "scripts" / "run_all.sh").read_text(encoding="utf-8")
        self.assertIn("retrieval_eval.py --queries 50", runner)
        self.assertIn('if [[ "$config" == "full_system" ]]', runner)
        self.assertIn('retrieval_mode="rrf"', runner)
        self.assertIn('--retrieval-mode $retrieval_mode', runner)
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
