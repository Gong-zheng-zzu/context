"""Regression tests for the live retrieval runner's fail-closed preflight."""

from __future__ import annotations

import unittest
from pathlib import Path
from unittest.mock import patch

import run_competition_retrieval as runner


class _Response:
    def __init__(self, status_code: int, payload: dict | None = None):
        self.status_code = status_code
        self._payload = payload or {}

    def json(self):
        return self._payload


class CompetitionRetrievalRunnerTests(unittest.TestCase):
    def _env(self) -> dict[str, str]:
        return {
            "EVAL_COMPETITION_RUNTIME_READY": "full_system",
            "EVAL_RUNTIME_CONFIG_NAME": "full_system",
            "EVAL_RUNTIME_CONFIG_HASH": "a" * 64,
            "EVAL_PASSWORD": "test-password",
        }

    @patch("run_competition_retrieval.requests.post")
    @patch("run_competition_retrieval.requests.get")
    def test_live_preflight_requires_health_and_auth(self, get, post):
        get.return_value = _Response(200)
        post.return_value = _Response(200, {"token": "redacted-test-token"})
        result = runner.probe_live_runtime("http://127.0.0.1:8088", "full_system", self._env())
        self.assertEqual(result["health_status"], 200)
        self.assertEqual(result["authentication_status"], 200)
        get.assert_called_once()
        post.assert_called_once()

    def test_live_preflight_rejects_missing_config_hash_before_network(self):
        env = self._env()
        env.pop("EVAL_RUNTIME_CONFIG_HASH")
        with patch("run_competition_retrieval.requests.get") as get:
            with self.assertRaisesRegex(RuntimeError, "config SHA-256"):
                runner.probe_live_runtime("http://127.0.0.1:8088", "full_system", env)
        get.assert_not_called()

    @patch("run_competition_retrieval.requests.post")
    @patch("run_competition_retrieval.requests.get")
    def test_live_preflight_rejects_unhealthy_service(self, get, post):
        get.return_value = _Response(503)
        with self.assertRaisesRegex(RuntimeError, "HTTP 503"):
            runner.probe_live_runtime("http://127.0.0.1:8088", "full_system", self._env())
        post.assert_not_called()

    @patch("run_competition_retrieval.requests.post")
    @patch("run_competition_retrieval.requests.get")
    def test_live_preflight_rejects_auth_failure(self, get, post):
        get.return_value = _Response(200)
        post.return_value = _Response(401)
        with self.assertRaisesRegex(RuntimeError, "HTTP 401"):
            runner.probe_live_runtime("http://127.0.0.1:8088", "full_system", self._env())

    def test_seed_verification_requires_vector_and_all_three_lanes(self):
        command = runner.seed_verification_command(Path("seed"), Path("run"), "http://localhost:8088")
        self.assertIn("--require-vector-evidence", command)
        self.assertIn("--require-threeway-evidence", command)
        self.assertIn(str(Path("seed") / "manifest.json"), command)

    def test_default_ground_truth_prefers_reviewed_and_falls_back_to_draft(self):
        with patch.object(runner, "DEFAULT_REVIEWED_GROUND_TRUTH", Path("reviewed.json")):
            with patch.object(runner, "DEFAULT_DRAFT_GROUND_TRUTH", Path("draft.json")):
                with patch.object(Path, "is_file", autospec=True, side_effect=lambda path: str(path) == "reviewed.json"):
                    self.assertEqual(Path("reviewed.json"), runner.select_ground_truth_file(None))
                with patch.object(Path, "is_file", autospec=True, return_value=False):
                    self.assertEqual(Path("draft.json"), runner.select_ground_truth_file(None))

    def test_validation_requires_review_only_for_live_runs(self):
        source = Path(runner.__file__).read_text(encoding="utf-8")
        self.assertIn('if not args.dry_run:', source)
        self.assertIn('validation.append("--require-reviewed")', source)

    def test_runner_orders_seed_verification_before_trace_and_evaluation(self):
        source = Path(runner.__file__).read_text(encoding="utf-8")
        seed = source.index('run_stage("seed",')
        verify = source.index('"verify_seed_threeway"', seed)
        trace = source.index('run_stage("trace",', verify)
        evaluate = source.index('run_stage("evaluate",', trace)
        self.assertLess(seed, verify)
        self.assertLess(verify, trace)
        self.assertLess(trace, evaluate)


if __name__ == "__main__":
    unittest.main()
