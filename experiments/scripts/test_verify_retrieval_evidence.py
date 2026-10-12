"""Contract tests for the formal retrieval evidence gate."""

from __future__ import annotations

import unittest

from verify_retrieval_evidence import validate_result


def _base_result(*, report_eligible: bool = True, mode: str = "multi_source_rrf_evaluation_only") -> dict:
    metadata = {
        "retrieval_fusion_mode": "rrf_3_sources",
        "source_statuses": {"vector": "success", "knowledge": "success", "timeline": "success"},
        "source_latency_ms": {"vector": 4, "knowledge": 8, "timeline": 6},
        "wall_clock_latency_ms": 10,
        "source_rankings": {
            "vector": {"doc-1": 1},
            "knowledge": {"doc-1": 1},
            "timeline": {"doc-1": 1},
        },
    }
    return {
        "evaluation_mode": mode,
        "runtime_config": {
            "config_name": "test",
            "config_file": "test.env",
            "config_hash": "a" * 64,
            "service_start_time": "2026-10-12T00:00:00Z",
        },
        "dataset": {
            "ground_truth_sha256": "b" * 64,
            "corpus_sha256": "c" * 64,
            "report_eligible": report_eligible,
        },
        "detailed_results": [{
            "config_name": "test",
            "retrieved_docs": ["doc-1"],
            "recall_at_5": 1.0,
            "is_scorable": True,
            "error_type": "None",
            "retrieval_metadata": metadata,
        }],
        "aggregated_metrics": {
            "test": {
                "scorable_count": 1,
                "unscorable_count": 0,
                "recall_at_5": 1.0,
                "p50_latency_ms": 4,
                "p95_latency_ms": 10,
            }
        },
    }


class RetrievalEvidenceGateTest(unittest.TestCase):
    def test_rrf_contract_accepts_complete_audit(self) -> None:
        self.assertEqual(validate_result(_base_result()), [])

    def test_rrf_contract_rejects_unreviewed_dataset(self) -> None:
        errors = validate_result(_base_result(report_eligible=False))
        self.assertIn("dataset.report_eligible must be true for formal retrieval evidence", errors)

    def test_rrf_contract_rejects_missing_fusion_and_source_audit(self) -> None:
        payload = _base_result()
        payload["detailed_results"][0]["retrieval_metadata"] = {}
        errors = validate_result(payload)
        self.assertTrue(any("retrieval_metadata must be a non-empty object" in error for error in errors))

    def test_rrf_contract_rejects_success_source_without_rankings(self) -> None:
        payload = _base_result()
        payload["detailed_results"][0]["retrieval_metadata"]["source_rankings"]["timeline"] = {}
        errors = validate_result(payload)
        self.assertTrue(any("source_rankings.timeline must be non-empty" in error for error in errors))

    def test_vector_mode_still_requires_eligible_dataset_but_not_rrf_fields(self) -> None:
        payload = _base_result(mode="vector_contexts_only")
        payload["detailed_results"][0]["retrieval_metadata"] = {}
        self.assertEqual(validate_result(payload), [])


if __name__ == "__main__":
    unittest.main()
