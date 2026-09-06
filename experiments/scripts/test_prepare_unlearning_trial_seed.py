import sys
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))

import prepare_unlearning_trial_seed as seed


class PrepareUnlearningTrialSeedTests(unittest.TestCase):
    def test_fixed_corpus_builds_two_scoped_thirty_document_requests(self):
        _, target_requests = seed.build_requests(seed.DEFAULT_CORPUS, seed.TARGET_SESSION_ID)
        _, control_requests = seed.build_requests(seed.DEFAULT_CORPUS, seed.CONTROL_SESSION_ID)

        target = seed.with_trial_metadata(target_requests, 2, "target")
        control = seed.with_trial_metadata(control_requests, 2, "control")

        self.assertEqual(len(target), 30)
        self.assertEqual(len(control), 30)
        self.assertEqual({item["request"]["sessionId"] for item in target}, {seed.TARGET_SESSION_ID})
        self.assertEqual({item["request"]["sessionId"] for item in control}, {seed.CONTROL_SESSION_ID})
        self.assertTrue(all(item["request"]["metadata"]["unlearning_trial"] == 2 for item in target + control))
        self.assertTrue(all(item["request"]["metadata"]["seed_schema"] == "structured-threeway-v1" for item in target + control))

    def test_control_scope_is_distinct_from_target(self):
        self.assertEqual(seed.ALLOWED_USER_ID, "eval_user_001")
        self.assertNotEqual(seed.TARGET_SESSION_ID, seed.CONTROL_SESSION_ID)

    def test_evidence_documents_accepts_double_data_envelope(self):
        payload = {"success": True, "data": {"data": {"documents": [{"doc_id": "doc-1"}]}}}
        self.assertEqual(seed.unwrap_evidence_documents(payload), {"documents": [{"doc_id": "doc-1"}]})
        self.assertIsNone(seed.unwrap_evidence_documents({"data": {"ok": True}}))

    def test_submit_session_retries_rate_limit_and_records_attempts(self):
        throttled = mock.Mock(status_code=429, headers={"Retry-After": "0"}, content=b"busy", text="busy")
        accepted = mock.Mock(status_code=200, headers={"X-RateLimit-Limit": "60"}, content=b"ok", text="ok")
        accepted.json.return_value = {"memoryId": "memory-1"}
        records = [{"doc_id": "doc-1", "request_sha256": "request", "request": {"content": "test"}}]

        with mock.patch.object(seed.requests, "post", side_effect=[throttled, accepted]), mock.patch.object(seed.time, "sleep") as sleep:
            submissions, errors = seed.submit_session("http://example.test", "token", seed.TARGET_SESSION_ID, records, 1, 0, 1)

        self.assertEqual(errors, [])
        self.assertEqual(submissions[0]["http_status"], 200)
        self.assertEqual([item["http_status"] for item in submissions[0]["attempts"]], [429, 200])
        self.assertEqual(submissions[0]["attempts"][0]["retry_delay_seconds"], 0)
        sleep.assert_called_once_with(0)


if __name__ == "__main__":
    unittest.main()
