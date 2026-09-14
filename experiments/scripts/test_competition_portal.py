"""Static contract checks for the synthetic multi-role competition portal."""
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
PORTAL = ROOT / "web" / "competition_portal.html"


class CompetitionPortalContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.html = PORTAL.read_text(encoding="utf-8")

    def test_role_views_and_protected_contracts_are_present(self):
        for role in ("caregiver", "doctor", "family", "elder"):
            self.assertIn(f'data-role="{role}"', self.html)
        for endpoint in ("/api/role/login", "/api/dashboard", "/api/v1/causal/extract", "/api/v1/agent/execute", "/api/sessions/"):
            self.assertIn(endpoint, self.html)

    def test_browser_does_not_persist_credentials(self):
        self.assertNotRegex(self.html, r"(?:localStorage|sessionStorage)")
        self.assertIn("let token = ''", self.html)
        self.assertIn("token='';", self.html)
        self.assertIn('autocomplete="current-password"', self.html)

    def test_role_and_scope_are_server_authoritative(self):
        self.assertIn("serverRole=data.role || role", self.html)
        self.assertIn("角色由 JWT 决定，不能在前端切换权限", self.html)
        self.assertIn("前端不自行扩大权限", self.html)
        self.assertIn("encodeURIComponent(id)", self.html)
        self.assertIn("id !== 'eval_retrieval_test'", self.html)

    def test_fixture_is_explicit_and_does_not_claim_live_evidence(self):
        self.assertIn("离线夹具（合成）", self.html)
        self.assertIn("离线夹具不执行联网研究，不能模拟权威来源证据。", self.html)
        self.assertIn("离线夹具不执行删除", self.html)
        self.assertIn("fixture_only", self.html)
        self.assertIn("不代表服务端授权结果", self.html)

    def test_sensitive_views_are_minimized(self):
        self.assertIn("授权住民（脱敏）", self.html)
        self.assertIn("不提供诊断、处方或自动通知", self.html)
        self.assertIn("默认不写入", self.html)
        self.assertIn("删除前 dry-run", self.html)
        self.assertIn("function approvedSourceURL", self.html)
        self.assertIn("parsed.protocol === 'https:'", self.html)

    def test_embedded_javascript_syntax(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("node is not installed")
        script = re.search(r"<script>\s*(.*?)\s*</script>", self.html, re.DOTALL)
        self.assertIsNotNone(script)
        with tempfile.NamedTemporaryFile("w", suffix=".js", encoding="utf-8", delete=False) as handle:
            handle.write(script.group(1))
            path = handle.name
        try:
            result = subprocess.run([node, "--check", path], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        finally:
            Path(path).unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
