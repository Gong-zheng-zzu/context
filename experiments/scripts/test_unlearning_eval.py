import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).parent))
import unlearning_eval as evaluator
from base_evaluator import APIResponse


def stores(status, before, after, deleted=0):
    return {
        "complete": status == "deleted",
        "stores": [
            {"store": name, "status": status, "before": before, "after": after, "deleted": deleted}
            for name in evaluator.REQUIRED_STORES
        ],
    }


class UnlearningEvaluationTests(unittest.TestCase):
    def test_precount_requires_each_storage_lane(self):
        counts, error = evaluator.parse_store_counts(stores("dry_run", 2, 2), "pre", require_positive=True)
        self.assertIsNone(error)
        self.assertEqual(set(counts), set(evaluator.REQUIRED_STORES))

        missing = stores("dry_run", 2, 2)
        missing["stores"] = missing["stores"][:-1]
        _, error = evaluator.parse_store_counts(missing, "pre", require_positive=True)
        self.assertIn("session_file_cache", error)

    def test_delete_postcount_rejects_residual_records(self):
        payload = stores("deleted", 2, 1, deleted=1)
        _, error = evaluator.parse_store_counts(payload, "delete", require_positive=False)
        self.assertIn("not verified empty", error)

    def test_document_evidence_fails_closed_on_storage_error(self):
        payload = {
            "documents": [{
                "doc_id": "doc-1",
                "vector": {"count": 1, "status": "verified"},
                "timeline": {"count": 1, "status": "verified"},
                "graph": {"count": 0, "status": "error", "error": "Neo4j unavailable"},
            }],
        }
        _, error = evaluator.parse_document_evidence(payload, ["doc-1"], expect_present=True)
        self.assertIn("Neo4j unavailable", error)

    def test_retrieval_ids_ignore_request_echoes(self):
        payload = {
            "request": {"doc_id": "target-doc"},
            "contexts": [{"doc_id": "returned-doc"}],
        }
        self.assertEqual(evaluator.extract_retrieved_doc_ids(payload), {"returned-doc"})

    def test_evidence_payload_unwraps_nested_data_envelopes_only(self):
        response = APIResponse(
            success=True,
            http_status=200,
            data={"success": True, "data": {"data": {"documents": [{"doc_id": "doc-1"}]}}},
            error_type=None,
            error_message=None,
            latency_ms=0,
        )
        self.assertEqual(evaluator.unwrap_evidence_payload(response), {"documents": [{"doc_id": "doc-1"}]})

        malformed = APIResponse(True, 200, {"data": {"status": "ok"}}, None, None, 0)
        self.assertIsNone(evaluator.unwrap_evidence_payload(malformed))


if __name__ == "__main__":
    unittest.main()
