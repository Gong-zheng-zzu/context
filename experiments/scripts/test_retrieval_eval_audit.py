"""Unit tests for retrieval evaluator provenance aggregation."""

from __future__ import annotations

import unittest

from retrieval_eval import collect_source_rankings


class RetrievalEvaluatorAuditTest(unittest.TestCase):
    def test_collects_server_provided_ranks_by_source(self) -> None:
        contexts = [
            {
                "doc_id": "doc-1",
                "metadata": {"rrf_ranks": {"vector": 2, "knowledge": 1}},
            },
            {
                "doc_id": "doc-2",
                "metadata": {"rrf_ranks": {"vector": 3, "timeline": 4}},
            },
        ]
        self.assertEqual(
            collect_source_rankings(contexts),
            {
                "vector": {"doc-1": 2, "doc-2": 3},
                "knowledge": {"doc-1": 1},
                "timeline": {"doc-2": 4},
            },
        )

    def test_does_not_infer_missing_ranks_from_fused_order(self) -> None:
        contexts = [
            {"doc_id": "doc-1", "metadata": {"rrf_sources": ["vector"]}},
            {"doc_id": "doc-2", "metadata": {}},
        ]
        self.assertEqual(collect_source_rankings(contexts), {})

    def test_rejects_invalid_rank_values(self) -> None:
        contexts = [
            {
                "doc_id": "doc-1",
                "metadata": {
                    "rrf_ranks": {
                        "vector": 0,
                        "knowledge": True,
                        "timeline": "1",
                        "": 2,
                    }
                },
            }
        ]
        self.assertEqual(collect_source_rankings(contexts), {})


if __name__ == "__main__":
    unittest.main()
