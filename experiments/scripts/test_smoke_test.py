"""Deterministic unit tests for the competition smoke-test gates."""

import unittest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from base_evaluator import APIResponse
from smoke_test import fixture_smoke, validate_contract


def response(payload, status=200):
    return APIResponse(True, status, payload, None, None, 1.0)


class SmokeContractTests(unittest.TestCase):
    def test_fixture_smoke_checks_local_console_only(self):
        self.assertEqual(fixture_smoke(), 0)

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

    def test_retrieval_accepts_explicit_empty_fallback(self):
        ok, detail = validate_contract(
            "retrieval",
            response({
                "contexts": [],
                "retrieval_metadata": {
                    "retrieval_fusion_mode": "vector_only_fallback",
                    "source_statuses": {"vector": "empty", "knowledge": "skipped", "timeline": "skipped"},
                    "source_latency_ms": {"vector": 4},
                    "wall_clock_latency_ms": 4,
                },
            }),
        )
        self.assertTrue(ok, detail)

    def test_retrieval_rejects_duplicate_doc_ids(self):
        payload = {
            "contexts": [{"doc_id": "same"}, {"doc_id": "same"}],
            "retrieval_metadata": {
                "retrieval_fusion_mode": "rrf_3_sources",
                "source_statuses": {},
                "source_latency_ms": {},
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


if __name__ == "__main__":
    unittest.main()
