#!/usr/bin/env python3
"""Offline contract checks for the competition console and launcher."""

from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
CONSOLE = ROOT / "web" / "competition_console.html"
LAUNCHER = ROOT / "experiments" / "scripts" / "run_competition_demo.ps1"
CONFIG = ROOT / "web" / "js" / "config.js"
AUTH_OVERRIDE = ROOT / "web" / "js" / "auth-security-override.js"
COMPOSE = ROOT / "docker-compose.yml"


class CompetitionConsoleContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.console = CONSOLE.read_text(encoding="utf-8")
        cls.launcher = LAUNCHER.read_text(encoding="utf-8")
        cls.config = CONFIG.read_text(encoding="utf-8")
        cls.auth_override = AUTH_OVERRIDE.read_text(encoding="utf-8")
        cls.compose = COMPOSE.read_text(encoding="utf-8")

    def test_console_uses_protected_live_endpoints(self):
        self.assertIn("/api/auth/login", self.console)
        self.assertIn("/api/v1/causal/extract", self.console)
        self.assertIn("/api/mcp/tools/retrieve_context", self.console)
        self.assertIn("evaluationRetrievalOnly:true", self.console)
        self.assertIn("/api/sessions/${EVAL_SESSION}?dry_run=${dryRun}", self.console)
        self.assertIn("/mcp/tools/create_context", self.console)
        self.assertIn("live-${Date.now()}", self.console)
        self.assertIn("现场新增护理记录", self.console)

    def test_console_defaults_to_analysis_and_fences_destructive_scope(self):
        self.assertIn('id="persist" type="checkbox"', self.console)
        self.assertIn("persist && $('persistPhrase').value.trim() !== 'PERSIST'", self.console)
        self.assertIn("const EVAL_USER = 'eval_user_001'", self.console)
        self.assertIn("const EVAL_SESSION = 'eval_retrieval_test'", self.console)
        self.assertIn("DELETE ${EVAL_SESSION}", self.console)
        self.assertIn("sessionDelete(true)", self.console)
        self.assertIn("sessionDelete(false)", self.console)
        self.assertIn("$('deletePhrase').value.trim() !== `DELETE ${EVAL_SESSION}`", self.console)

    def test_console_reports_request_errors_as_failed_actions(self):
        self.assertIn("请求失败：${now()}", self.console)
        self.assertIn("请求未执行", self.console)

    def test_console_does_not_persist_password_or_token(self):
        self.assertIsNone(re.search(r"(?:localStorage|sessionStorage)\s*\.", self.console))
        self.assertIn("let token = '';", self.console)
        self.assertIn("$('password').value = '';", self.console)
        self.assertIn("offline_fixture", self.console)
        self.assertIn("不能模拟 RRF 证据", self.console)
        self.assertIn("model_tier:'fixture_only'", self.console)
        self.assertIn("quality:{tuple_valid:true", self.console)
        self.assertIn("evidence_spans", self.console)

    def test_launcher_records_safe_manifest_and_checks_mode(self):
        self.assertIn('[ValidateSet("ollama", "fixture")]', self.launcher)
        self.assertIn("/api/auth/login", self.launcher)
        self.assertIn("/api/tags", self.launcher)
        self.assertIn('credential_source = "environment"', self.launcher)
        self.assertIn("token_persisted = $false", self.launcher)
        self.assertIn("run_manifest.json", self.launcher)
        self.assertIn("competition_console.html?mode=$Mode", self.launcher)
        manifest = self.launcher.split("$manifest =", 1)[1].split("$manifest |", 1)[0]
        self.assertNotIn("EVAL_PASSWORD", manifest)
        self.assertNotIn("token =", manifest)

    def test_launcher_fixture_mode_is_truly_offline(self):
        """The offline fallback must not require a service or protected credentials."""
        self.assertIn('$isFixture = $Mode -eq "fixture"', self.launcher)
        self.assertIn("Offline fixture mode: no service, credentials, model, or evaluation API required", self.launcher)
        self.assertIn('if ($isFixture) {', self.launcher)
        self.assertIn('if (-not $isFixture) {', self.launcher)
        self.assertIn('if ($RunIsolatedUnlearning -and $isFixture)', self.launcher)
        self.assertIn('artifacts = $(if ($isFixture) { ,@("run_manifest.json", "smoke.json") }', self.launcher)
        self.assertIn('--mode fixture --json-output (Join-Path $RunDir "smoke.json")', self.launcher)
        self.assertIn('$([System.Uri]::new($consolePath).AbsoluteUri)?mode=fixture', self.launcher)
        # The credential guard is inside the live-mode branch, not at script scope.
        credential_guard = 'if (-not $env:EVAL_USER_ID -or -not $env:EVAL_PASSWORD)'
        self.assertEqual(self.launcher.count(credential_guard), 1)
        self.assertLess(self.launcher.index(credential_guard), self.launcher.index('\n}\n\n$manifest'))

    def test_embedded_javascript_has_valid_syntax_when_node_is_available(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("node is not installed")
        script = re.search(r"<script>\s*(.*?)\s*</script>", self.console, re.DOTALL)
        self.assertIsNotNone(script)
        with tempfile.NamedTemporaryFile("w", suffix=".js", encoding="utf-8", delete=False) as handle:
            handle.write(script.group(1))
            script_path = handle.name
        try:
            completed = subprocess.run([node, "--check", script_path], capture_output=True, text=True, check=False)
            self.assertEqual(completed.returncode, 0, completed.stderr)
        finally:
            Path(script_path).unlink(missing_ok=True)

    def test_console_exposes_causal_audit_actions(self):
        self.assertIn('id="causalQuality"', self.console)
        self.assertIn('id="causalEvidence"', self.console)
        self.assertIn('id="causalRerun"', self.console)
        self.assertIn('id="causalCopy"', self.console)
        self.assertIn("evidence_spans", self.console)
        self.assertIn("model_tier", self.console)

    def test_local_api_override_and_connection_error_are_actionable(self):
        self.assertIn("new URLSearchParams(window.location.search).get('api')", self.config)
        self.assertIn("?api=http://127.0.0.1:8088", self.auth_override)
        self.assertIn("Failed to fetch", self.auth_override)
        self.assertIn("请确认 Docker/服务已启动", self.auth_override)

    def test_compose_allows_common_local_static_server_origins(self):
        self.assertIn("http://localhost:5500", self.compose)
        self.assertIn("http://127.0.0.1:5500", self.compose)
        self.assertIn("http://localhost:5173", self.compose)


if __name__ == "__main__":
    unittest.main()
