"""Offline regression tests for the security ablation evaluation path.

These tests never touch the network. They feed a fixed, metadata-only copy of a
real ablation response shape into the parser, aggregation, and persistence
functions so the ablation evidence contract can be verified while the
Qdrant/TimescaleDB/Neo4j/Ollama stack is unavailable.
"""

import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import APIResponse
from security_eval import (
    ABLATION_MAX_MESSAGE_BYTES,
    ABLATION_PROFILE_ORDER,
    ABLATION_RESULT_FILENAME_PREFIX,
    ABLATION_SELF_TEST_RESPONSE,
    AblationSampleResult,
    SecurityAblationEvaluator,
)


def observation(decision, adversarial=False, residual=None, sensitive_types=None):
    return {
        "profile": "offline-test",
        "decision": decision,
        "reason": "offline-test",
        "confidence": 0.5,
        "sensitive_types": sensitive_types if sensitive_types is not None else (["phone"] if decision == "redact" else []),
        "layers": [],
        "asdf": {
            "is_adversarial": adversarial,
            "attack_types": ["space_separation"] if adversarial else [],
            "confidence": 0.9 if adversarial else 0.0,
            "redetection_performed": adversarial,
            "residual_attack_types": residual or [],
        },
        "early_stop_reason": "offline-test",
        "latency_ms": 1.0,
        "pipeline_version": "offline-test",
        "config_snapshot_sha256": "",
    }


def sample_result(evaluator, sample_id, sample_kind, profiles, attack_text="offline text"):
    return AblationSampleResult(
        sample_id=sample_id,
        synthetic_sample_id=evaluator._synthetic_sample_id(sample_id),
        attack_type="prompt_injection" if sample_kind == "attack" else "benign_nursing_request",
        sample_kind=sample_kind,
        expected_behavior="refuse" if sample_kind == "attack" else "allow",
        success_criteria="block" if sample_kind == "attack" else "allow",
        config_label="offline-test",
        http_status=200,
        error_type="",
        error_message="",
        latency_ms=1.0,
        input_sha256=evaluator._sha256(attack_text),
        response_sha256="offline-response",
        trace_id="offline-trace",
        pipeline_version="security-ablation-v1",
        timestamp="2026-07-24T00:00:00+00:00",
        profiles=profiles,
    )


class SecurityAblationEvaluationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.evaluator = SecurityAblationEvaluator()

    def test_response_parses_every_server_profile_with_metadata_only_evidence(self) -> None:
        observations = self.evaluator._parse_profile_observations(ABLATION_SELF_TEST_RESPONSE["data"])
        self.assertEqual(list(ABLATION_PROFILE_ORDER), list(observations))
        self.assertEqual("redact", observations["asdf"]["decision"])
        self.assertTrue(observations["asdf"]["asdf"]["is_adversarial"])
        self.assertEqual(["homophone"], observations["asdf"]["asdf"]["residual_attack_types"])
        self.assertEqual(1, observations["asdf"]["layers"][0]["match_count"])
        self.assertEqual(["normalized"], observations["asdf"]["layers"][0]["span_coordinate_spaces"])
        self.assertEqual({"exact": 1}, observations["asdf_casia"]["layers"][0]["casia_match_method_counts"])
        self.assertEqual(["casia-context-v1"], observations["asdf_casia"]["layers"][0]["casia_configuration_versions"])
        self.assertNotIn("normalized_text", json.dumps(observations))

    def test_casia_audit_fields_are_profiled_from_config_snapshot(self) -> None:
        observations = self.evaluator._parse_profile_observations(ABLATION_SELF_TEST_RESPONSE["data"])
        attack = sample_result(self.evaluator, "prompt_injection_003", "attack", observations)
        metrics, _ = self.evaluator.aggregate_ablation_metrics([attack], [])
        self.assertEqual("casia-context-v1", metrics["asdf_casia"].casia_config_version)
        self.assertEqual("builtin", metrics["asdf_casia"].casia_source)
        self.assertIsNone(metrics["asdf_casia"].pccm_calibration_source)
        self.assertEqual("fixed_unvalidated", metrics["asdf_casia_pccm"].pccm_calibration_source)
        self.assertEqual({"exact": 1}, metrics["asdf_casia"].casia_match_method_counts)
        self.assertIsNone(metrics["regex_only"].casia_source)
        self.assertEqual({}, metrics["regex_only"].casia_match_method_counts)
        self.assertEqual(["regex"], [layer.layer for layer in metrics["regex_only"].layers])

    def test_casia_file_source_changes_digest_but_stays_stable_within_a_run(self) -> None:
        file_response = json.loads(json.dumps(ABLATION_SELF_TEST_RESPONSE))
        for profile in file_response["data"]["profiles"]:
            if isinstance(profile.get("casia"), dict):
                profile["casia"]["version"] = "casia-context-v1-file"
                profile["casia"]["source"] = "file"
        evaluator = SecurityAblationEvaluator()
        first = evaluator._parse_profile_observations(file_response["data"])
        second = evaluator._parse_profile_observations(file_response["data"])
        self.assertEqual("file", evaluator.ablation_config_snapshots[first["asdf_casia"]["config_snapshot_sha256"]]["casia"]["source"])
        self.assertEqual(first["asdf_casia"]["config_snapshot_sha256"], second["asdf_casia"]["config_snapshot_sha256"])
        self.assertEqual(2, len(evaluator.ablation_config_snapshots))

    def test_config_snapshots_are_captured_and_deduplicated(self) -> None:
        observations = self.evaluator._parse_profile_observations(ABLATION_SELF_TEST_RESPONSE["data"])
        self.assertEqual("", observations["regex_only"]["config_snapshot_sha256"])
        digests = {
            observations["asdf_casia"]["config_snapshot_sha256"],
            observations["asdf_casia_pccm"]["config_snapshot_sha256"],
        }
        self.assertEqual(2, len(digests))
        self.assertEqual(2, len(self.evaluator.ablation_config_snapshots))
        full_snapshot = self.evaluator.ablation_config_snapshots[observations["asdf_casia_pccm"]["config_snapshot_sha256"]]
        self.assertEqual("fixed_unvalidated", full_snapshot["pccm_s"]["calibration_source"])
        self.assertEqual(0.45, full_snapshot["pccm_s"]["decision_threshold"])

    def test_synthetic_sample_id_matches_server_contract(self) -> None:
        for sample_id in ("prompt_injection_001", "benign-01-01", "weird id.with/slash", "x" * 200):
            synthetic = self.evaluator._synthetic_sample_id(sample_id)
            self.assertRegex(synthetic, r"^synthetic_[A-Za-z0-9_-]{1,64}$")

    def test_envelope_requires_success_and_profiles(self) -> None:
        self.assertTrue(self.evaluator._validate_ablation_envelope(
            APIResponse(True, 200, ABLATION_SELF_TEST_RESPONSE, None, None, 1.0))[0])
        self.assertFalse(self.evaluator._validate_ablation_envelope(
            APIResponse(True, 200, {"success": True, "data": {"profiles": []}}, None, None, 1.0))[0])
        self.assertFalse(self.evaluator._validate_ablation_envelope(
            APIResponse(True, 200, {"success": True, "data": {}}, None, None, 1.0))[0])
        self.assertFalse(self.evaluator._validate_ablation_envelope(
            APIResponse(False, 403, {"success": False, "error": "out of scope"}, None, None, 1.0))[0])

    def test_ablation_guard_rejects_oversized_input_without_network(self) -> None:
        oversized = {
            "id": "oversized",
            "type": "prompt_injection",
            "attack_text": "a" * (ABLATION_MAX_MESSAGE_BYTES + 1),
            "expected_behavior": "refuse",
            "success_criteria": "block",
        }
        response = self.evaluator.send_ablation_request(oversized)
        self.assertIsNotNone(response.error_type)
        self.assertEqual(0, response.http_status)

    def test_detection_rate_and_delta_use_paired_per_sample_decisions(self) -> None:
        all_allow = {profile: observation("allow") for profile in ABLATION_PROFILE_ORDER}
        attacks = [
            sample_result(self.evaluator, "prompt_injection_001", "attack", {
                "regex_only": observation("allow"),
                "asdf": observation("redact", adversarial=True, residual=["homophone"]),
                "asdf_casia": observation("redact", adversarial=True, residual=["homophone"]),
                "asdf_casia_pccm": observation("redact", adversarial=True, residual=[]),
            }),
            sample_result(self.evaluator, "prompt_injection_002", "attack", dict(all_allow)),
        ]
        benign = [
            sample_result(self.evaluator, "benign-01-01", "benign", dict(all_allow)),
            sample_result(self.evaluator, "benign-01-02", "benign", {
                **all_allow, "regex_only": observation("redact"),
            }),
        ]
        metrics, deltas = self.evaluator.aggregate_ablation_metrics(attacks, benign)
        self.assertEqual(0.0, metrics["regex_only"].detection_rate)
        self.assertEqual(0.5, metrics["asdf"].detection_rate)
        self.assertEqual(2, metrics["asdf"].attack_api_success_count)
        self.assertEqual(0.0, metrics["asdf"].benign_false_positive_rate)
        self.assertEqual(0.5, metrics["regex_only"].benign_false_positive_rate)
        self.assertEqual(1, metrics["asdf"].asdf_residual_observation_count)
        first_delta = deltas[0]
        self.assertEqual("regex_only", first_delta.baseline_profile)
        self.assertEqual("asdf", first_delta.profile)
        self.assertEqual(1, first_delta.newly_detected_count)
        self.assertEqual(1, first_delta.stable_allowed_count)
        self.assertEqual(["phone"], first_delta.newly_covered_sensitive_types)
        self.assertEqual(0.5, first_delta.detection_rate_delta)

    def test_saved_ablation_result_keeps_hashes_and_skips_report_builder_glob(self) -> None:
        all_allow = {profile: observation("allow") for profile in ABLATION_PROFILE_ORDER}
        attack = sample_result(
            self.evaluator, "prompt_injection_001", "attack",
            {**all_allow, "asdf": observation("redact", adversarial=True)}, attack_text="offline secret input",
        )
        benign = sample_result(self.evaluator, "benign-01-01", "benign", dict(all_allow))
        metrics, deltas = self.evaluator.aggregate_ablation_metrics([attack], [benign])
        self.evaluator.dataset_manifest = [{"path": "x", "sample_count": 1, "sha256": "digest"}]
        with tempfile.TemporaryDirectory() as directory:
            self.evaluator.results_dir = Path(directory)
            output = self.evaluator.save_ablation_results("full_system", "offline", [attack], [benign], metrics, deltas)
            content = output.read_text(encoding="utf-8")
            parsed = json.loads(content)
        self.assertFalse(Path(output.name).match("security_*.json"))
        self.assertTrue(output.name.startswith(ABLATION_RESULT_FILENAME_PREFIX))
        self.assertEqual("live_api_ablation", parsed["execution_mode"])
        self.assertEqual("security_ablation", parsed["evaluation_mode"])
        self.assertIn("config_hashes", parsed)
        self.assertIn("dataset_manifest", parsed)
        self.assertIn("timestamp_utc", parsed)
        self.assertIn("profile_metrics", parsed)
        self.assertIn("ablation_deltas", parsed)
        self.assertNotIn("offline secret input", content)
        self.assertNotIn("offline_text", content)
        self.assertEqual(attack.input_sha256, parsed["detailed_results"][0]["input_sha256"])
        self.assertNotIn("attack_text", parsed["detailed_results"][0])


if __name__ == "__main__":
    unittest.main()
