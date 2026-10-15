"""Deterministic unit tests for the competition smoke-test gates."""

import unittest
import sys
from pathlib import Path
import json
import subprocess
import tempfile
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

from base_evaluator import APIResponse
from smoke_test import fixture_smoke, main, validate_contract


def response(payload, status=200):
    return APIResponse(True, status, payload, None, None, 1.0)


class SmokeContractTests(unittest.TestCase):
    def test_fixture_smoke_checks_local_console_only(self):
        self.assertEqual(fixture_smoke(), 0)

    @patch("smoke_test.BaseEvaluator")
    def test_live_smoke_does_not_issue_retrieval_requests(self, evaluator_type):
        evaluator = evaluator_type.return_value
        evaluator.check_service_health.return_value = (True, "healthy")
        evaluator.authenticate_from_env.return_value = response({"token": "test"})
        evaluator.get_auth_headers.return_value = {"Authorization": "Bearer test"}
        def call_api(_method, endpoint, *_args, **_kwargs):
            if endpoint == "/api/v1/security/evaluate-input":
                return response({"data": {"decision": "allow"}})
            if endpoint == "/api/v1/causal/extract":
                return response({"relations": [], "execution": {"mode": "rules", "model_status": "unavailable"}, "persistence": {"status": "analysis_only"}})
            return response({"enabled": True})

        evaluator.call_api.side_effect = call_api

        with patch.dict("os.environ", {"EVAL_USER_ID": "doctor_001", "EVAL_PASSWORD": "test"}):
            with patch("sys.argv", ["smoke_test.py", "--mode", "live"]):
                self.assertEqual(main(), 0)

        retrieval_calls = [
            call for call in evaluator.call_api.call_args_list
            if call.args[1] == "/api/mcp/tools/retrieve_context"
        ]
        self.assertEqual(retrieval_calls, [])
        self.assertEqual(
            [call.args[1] for call in evaluator.call_api.call_args_list],
            ["/api/v1/security/evaluate-input", "/api/v1/causal/extract", "/api/v1/unlearning/config"],
        )

    @patch("smoke_test.time.sleep")
    @patch("smoke_test.BaseEvaluator")
    def test_live_smoke_retries_transient_startup_health_failure(self, evaluator_type, sleep):
        evaluator = evaluator_type.return_value
        evaluator.check_service_health.side_effect = [(False, "starting"), (True, "healthy")]
        evaluator.authenticate_from_env.return_value = response({"token": "test"})
        evaluator.get_auth_headers.return_value = {"Authorization": "Bearer test"}
        evaluator.call_api.side_effect = lambda _method, endpoint, *_args, **_kwargs: {
            "/api/v1/security/evaluate-input": response({"data": {"decision": "allow"}}),
            "/api/v1/causal/extract": response({"relations": [], "execution": {"mode": "rules", "model_status": "unavailable"}, "persistence": {"status": "analysis_only"}}),
            "/api/v1/unlearning/config": response({"enabled": True}),
        }[endpoint]

        with patch.dict("os.environ", {"EVAL_USER_ID": "doctor_001", "EVAL_PASSWORD": "test"}):
            with patch("sys.argv", ["smoke_test.py", "--mode", "live", "--health-timeout", "2"]):
                self.assertEqual(main(), 0)

        self.assertEqual(evaluator.check_service_health.call_count, 2)
        sleep.assert_called_once()

    def test_causal_requires_execution_and_persistence_audit(self):
        ok, _ = validate_contract("causal", response({"relations": [], "execution": {}, "persistence": {}}))
        self.assertFalse(ok)
        ok, detail = validate_contract(
            "causal",
            response({
                "relations": [],
                "execution": {"mode": "rules", "model_status": "unavailable"},
                "persistence": {"status": "analysis_only"},
            }),
        )
        self.assertTrue(ok, detail)

    def test_retrieval_accepts_explicit_no_evidence_response(self):
        ok, detail = validate_contract(
            "retrieval",
            response({
                "contexts": [],
                "retrieval_metadata": {
                    "retrieval_fusion_mode": "no_evidence",
                    "source_statuses": {"vector": "empty", "knowledge": "skipped", "timeline": "skipped"},
                    "source_latency_ms": {"vector": 4, "knowledge": 0, "timeline": 0},
                    "wall_clock_latency_ms": 4,
                },
            }),
        )
        self.assertTrue(ok, detail)

    def test_retrieval_rejects_missing_source_status(self):
        payload = {
            "contexts": [],
            "retrieval_metadata": {
                "retrieval_fusion_mode": "no_evidence",
                "source_statuses": {"vector": "empty", "knowledge": "skipped"},
                "source_latency_ms": {"vector": 0, "knowledge": 0, "timeline": 0},
                "wall_clock_latency_ms": 0,
            },
        }
        ok, detail = validate_contract("retrieval", response(payload))
        self.assertFalse(ok)
        self.assertIn("timeline", detail)

    def test_retrieval_rejects_mode_that_disagrees_with_source_status(self):
        payload = {
            "contexts": [],
            "retrieval_metadata": {
                "retrieval_fusion_mode": "rrf_3_sources",
                "source_statuses": {"vector": "success", "knowledge": "empty", "timeline": "skipped"},
                "source_latency_ms": {"vector": 2, "knowledge": 2, "timeline": 0},
                "wall_clock_latency_ms": 2,
            },
        }
        ok, detail = validate_contract("retrieval", response(payload))
        self.assertFalse(ok)
        self.assertIn("fusion mode", detail)

    def test_retrieval_rejects_negative_latency(self):
        payload = {
            "contexts": [],
            "retrieval_metadata": {
                "retrieval_fusion_mode": "no_evidence",
                "source_statuses": {"vector": "empty", "knowledge": "skipped", "timeline": "skipped"},
                "source_latency_ms": {"vector": -1, "knowledge": 0, "timeline": 0},
                "wall_clock_latency_ms": 0,
            },
        }
        ok, detail = validate_contract("retrieval", response(payload))
        self.assertFalse(ok)
        self.assertIn("latency", detail)

    def test_retrieval_rejects_duplicate_doc_ids(self):
        payload = {
            "contexts": [{"doc_id": "same"}, {"doc_id": "same"}],
            "retrieval_metadata": {
                "retrieval_fusion_mode": "rrf_3_sources",
                "source_statuses": {"vector": "success", "knowledge": "success", "timeline": "success"},
                "source_latency_ms": {"vector": 1, "knowledge": 1, "timeline": 1},
                "wall_clock_latency_ms": 3,
            },
        }
        ok, detail = validate_contract("retrieval", response(payload))
        self.assertFalse(ok)
        self.assertIn("duplicate", detail)

    def test_http_error_never_passes_contract(self):
        failed = APIResponse(False, 503, None, "http_error", "service unavailable", 10.0)
        ok, detail = validate_contract("retrieval", failed)
        self.assertFalse(ok)
        self.assertIn("service unavailable", detail)

    def test_cli_writes_failure_report_when_service_is_unavailable(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "smoke.json"
            completed = subprocess.run(
                [sys.executable, str(Path(__file__).with_name("smoke_test.py")), "--base-url", "http://127.0.0.1:1", "--json-output", str(output), "--health-timeout", "0"],
                capture_output=True,
                text=True,
                encoding="utf-8",
                # The child inherits the Windows console encoding on some
                # machines.  Its diagnostic is not an assertion target, so
                # preserve the process result without a reader-thread crash.
                errors="replace",
                check=False,
                timeout=10,
            )
            self.assertNotEqual(completed.returncode, 0)
            self.assertIn("[FAIL] health:", completed.stdout)
            report = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(report["failure"], "health")
            self.assertFalse(report["passed"])


if __name__ == "__main__":
    unittest.main()
