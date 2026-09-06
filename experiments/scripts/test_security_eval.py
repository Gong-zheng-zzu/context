"""Offline regression tests for security_eval.py statistics and dataset loading."""

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from security_eval import SecurityEvaluator, SecurityResult


def result(sample_id: str, outcome: str) -> SecurityResult:
    return SecurityResult(
        sample_id=sample_id,
        attack_type="benign",
        expected_behavior="allow",
        success_criteria="structured decision is allow",
        config_label="offline-test",
        http_status=200 if outcome != "api_error" else 503,
        error_type="http_error" if outcome == "api_error" else "",
        error_message="service unavailable" if outcome == "api_error" else "",
        latency_ms=1.0,
        input_sha256="input-digest",
        response_sha256="response-digest" if outcome != "api_error" else "",
        response_evidence={},
        oracle_evidence={},
        outcome=outcome,
        outcome_reason="offline test",
        timestamp="2026-07-24T00:00:00+00:00",
        sample_kind="benign",
    )


class SecurityEvaluationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.evaluator = SecurityEvaluator()

    def test_benign_dataset_expands_to_one_hundred_non_pii_requests(self) -> None:
        samples = self.evaluator.load_benign_samples()
        self.assertEqual(100, len(samples))
        self.assertEqual(100, len({sample["id"] for sample in samples}))
        self.assertTrue(all(sample["expected_behavior"] == "allow" for sample in samples))
        self.assertTrue(all(sample["sample_kind"] == "benign" for sample in samples))

    def test_false_positive_rate_excludes_api_errors_from_denominator(self) -> None:
        metrics = self.evaluator.aggregate_metrics(
            "offline-test",
            [],
            [result("benign-1", "allowed"), result("benign-2", "false_positive"), result("benign-3", "api_error")],
        )
        self.assertEqual(3, metrics.benign_total)
        self.assertEqual(2, metrics.benign_api_success_count)
        self.assertEqual(1, metrics.benign_allowed)
        self.assertEqual(1, metrics.false_positive_count)
        self.assertEqual(0.5, metrics.benign_pass_rate)
        self.assertEqual(0.5, metrics.false_positive_rate)
        self.assertIsNotNone(metrics.false_positive_wilson_95_ci)

    def test_blocked_benign_request_is_false_positive(self) -> None:
        outcome, _, evidence = self.evaluator.evaluate_benign_oracle({"blocked": True})
        self.assertEqual("false_positive", outcome)
        self.assertEqual("blocked", evidence["decision"])

    def test_saved_result_retains_only_digests_for_inputs_and_errors(self) -> None:
        attack = result("attack-1", "defended")
        attack.attack_type = "prompt_injection"
        attack.sample_kind = "attack"
        attack.input_sha256 = self.evaluator._sha256("sensitive test input")
        api_error = result("attack-2", "api_error")
        api_error.attack_type = "prompt_injection"
        api_error.error_message = self.evaluator._error_digest("sensitive test input")
        benign = result("benign-1", "allowed")
        metrics = self.evaluator.aggregate_metrics("offline-test", [attack, api_error], [benign])
        with tempfile.TemporaryDirectory() as directory:
            self.evaluator.results_dir = Path(directory)
            output = self.evaluator.save_results("offline-test", "offline", [attack, api_error], [benign], metrics)
            content = output.read_text(encoding="utf-8")
            parsed = json.loads(content)
        self.assertNotIn("sensitive test input", content)
        self.assertEqual(attack.input_sha256, parsed["detailed_results"][0]["input_sha256"])
        self.assertNotIn("attack_text", parsed["detailed_results"][0])


if __name__ == "__main__":
    unittest.main()
