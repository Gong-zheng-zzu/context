import unittest
from unittest.mock import patch
from pathlib import Path

import formal_evaluation_gate as gate


class _FakeBuilder:
    def __init__(self):
        self.project_root = Path("unused")
        self.results_dir = Path("unused")
        self.selected_results = {"security": "security.json", "causal": "causal.json", "retrieval": "retrieval.json"}
        self.rejected_results = {}

    def load_latest_results(self):
        return {"security": {}, "causal": {}, "retrieval": {}, "unlearning": None}


class FormalEvaluationGateTests(unittest.TestCase):
    def test_all_required_artifacts_must_be_eligible(self):
        with patch.object(gate, "ReportBuilder", _FakeBuilder):
            report = gate.evaluate(Path("unused"))
        self.assertTrue(report["passed"])
        self.assertEqual(report["experiments"]["retrieval"]["status"], "eligible")

    def test_missing_artifact_is_blocked(self):
        class Missing(_FakeBuilder):
            def load_latest_results(self):
                return {"security": None, "causal": {}, "retrieval": None, "unlearning": None}

        with patch.object(gate, "ReportBuilder", Missing):
            report = gate.evaluate(Path("unused"))
        self.assertFalse(report["passed"])
        self.assertEqual(report["experiments"]["security"]["status"], "blocked")


if __name__ == "__main__":
    unittest.main()
