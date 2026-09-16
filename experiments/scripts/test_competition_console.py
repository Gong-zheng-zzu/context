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
AUDIT_HELPER = ROOT / "web" / "js" / "competition-audit.js"
AUTH_OVERRIDE = ROOT / "web" / "js" / "auth-security-override.js"
COMPOSE = ROOT / "docker-compose.yml"


class CompetitionConsoleContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.console = CONSOLE.read_text(encoding="utf-8")
        cls.launcher = LAUNCHER.read_text(encoding="utf-8")
        cls.config = CONFIG.read_text(encoding="utf-8")
        cls.audit_helper = AUDIT_HELPER.read_text(encoding="utf-8")
        cls.auth_override = AUTH_OVERRIDE.read_text(encoding="utf-8")
        cls.compose = COMPOSE.read_text(encoding="utf-8")

    def test_console_uses_protected_live_endpoints(self):
        self.assertIn("/api/auth/login", self.console)
        self.assertIn("/api/v1/security/ablation", self.console)
        self.assertIn("/api/v1/causal/extract", self.console)
        self.assertIn("/api/v1/causal/reviews", self.console)
        self.assertIn("/api/mcp/tools/retrieve_context", self.console)
        self.assertIn("evaluationRetrievalOnly:true", self.console)
        self.assertIn("/api/sessions/${EVAL_SESSION}?dry_run=${dryRun}", self.console)
        self.assertIn("/mcp/tools/create_context", self.console)
        self.assertIn("live-${Date.now()}", self.console)
        self.assertIn("现场新增护理记录", self.console)

    def test_console_uses_one_validated_trace_across_lifecycle_calls(self):
        self.assertIn("let lifecycleTraceID = createTraceID();", self.console)
        self.assertIn("'X-Trace-ID':lifecycleTraceID", self.console)
        self.assertIn("function setStage(stage, state, message)", self.console)
        for stage in ("security", "causal", "storage", "retrieval", "deletion"):
            self.assertIn(f'data-stage="{stage}"', self.console)

    def test_security_ablation_is_server_owned_and_complete(self):
        self.assertIn('id="securityProfiles"', self.console)
        self.assertIn("['regex_only','asdf','asdf_casia','asdf_casia_pccm']", self.console)
        self.assertIn("sample_id:`synthetic_${Date.now()}`", self.console)
        self.assertIn("synthetic:true", self.console)
        self.assertIn("不代表达到正式指标", self.console)

    def test_causal_review_and_counterfactual_remain_governed(self):
        self.assertIn('id="reviewSubmit"', self.console)
        self.assertIn("training_applied", self.console)
        self.assertIn("未直接进入训练集", self.console)
        self.assertIn("synthetic:true", self.console)
        self.assertIn("mode:'research_preview'", self.console)
        self.assertIn("persisted:false", self.console)
        self.assertIn("clinical_advice:false", self.console)
        self.assertIn("不生成新的医学机制、诊断或治疗建议", self.console)

    def test_deletion_proof_runs_probes_and_idempotency_without_raw_text(self):
        self.assertIn("const beforeProbe = retrievalProbe(await runRRF(probeQuery))", self.console)
        self.assertIn("const afterProbe = retrievalProbe(await runRRF(probeQuery))", self.console)
        self.assertIn("const idempotent = deletionEvidence(await sessionDelete(false))", self.console)
        self.assertIn("evidence_sha256", self.console)
        self.assertIn("scope_sha256", self.console)
        self.assertIn("certificate_sha256", self.console)
        self.assertIn("不代表梯度或模型参数遗忘", self.console)
        certificate_builder = self.console.split("const certificate =", 1)[1].split("certificate.certificate_sha256", 1)[0]
        self.assertNotIn("causalText", certificate_builder)
        self.assertNotIn("liveRecord", certificate_builder)
        self.assertNotIn("content:", certificate_builder)

    def test_fixture_mode_has_no_network_or_mutating_controls(self):
        self.assertIn("严格离线：不联网、不写入、不检索、不删除", self.console)
        self.assertIn("for (const id of ['securityRun','researchRun','liveSeedRun','rrfRun','unlearningVerify','unlearningRun','reviewSubmit'])", self.console)
        fixture_branch = self.console.split("async function refreshStatus()", 1)[1].split("function requireEvalScope", 1)[0]
        self.assertLess(fixture_branch.index("if (fixtureMode)"), fixture_branch.index("fetch(API_BASE + '/health'"))

    def test_console_research_agent_uses_protected_read_only_contract(self):
        self.assertIn('id="researchTopic"', self.console)
        self.assertIn('id="researchSourceUrl"', self.console)
        self.assertIn('id="researchRun"', self.console)
        self.assertIn("/api/v1/agent/execute", self.console)
        self.assertIn("authoritative_web_search", self.console)
        self.assertIn("authoritative_source_fetch", self.console)
        self.assertIn("source_evidence", self.console)
        self.assertIn("content_sha256", self.console)
        self.assertIn("retrieved_at", self.console)
        self.assertIn("Array.isArray(raw) ? raw[0] : raw", self.console)
        self.assertIn("write_applied", self.console)
        self.assertIn("不能模拟权威来源证据", self.console)

    def test_console_research_agent_fences_untrusted_sources_and_personal_data(self):
        self.assertIn("const allowed = ['who.int', 'nhc.gov.cn', 'chinacdc.cn', 'nmpa.gov.cn']", self.console)
        self.assertIn("parsed.protocol === 'https:'", self.console)
        self.assertIn("不要输入护理记录、姓名、病历号、联系方式", self.console)
        self.assertIn("不得编造", self.console)
        self.assertIn("不得写入、归档、通知、删除或修改任何记录", self.console)

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
        self.assertIn('autocomplete="off"', self.console)

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

    def test_console_exposes_structured_rrf_source_audit(self):
        self.assertIn('id="rrfAuditStatus"', self.console)
        self.assertIn('id="rrfSources"', self.console)
        self.assertIn('id="rrfResults"', self.console)
        self.assertIn('js/competition-audit.js', self.console)
        self.assertIn("三路融合完成", self.console)
        self.assertIn("来源审计不完整", self.console)
        self.assertIn("本次响应没有可追溯的检索结果", self.console)

    def test_rrf_audit_normalizer_uses_only_response_evidence(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("node is not installed")
        script = r'''
const audit = require(process.argv[1]);
const normalized = audit.normalizeRRFResponse({data:{
  retrieval_metadata:{
    retrieval_fusion_mode:'rrf_2_sources',
    retrieval_active_sources:['vector','timeline'],
    retrieval_empty_sources:['knowledge'],
    source_statuses:{vector:'success',knowledge:'timeout',timeline:'success'},
    source_latency_ms:{vector:17,knowledge:500,timeline:24},
    source_candidate_counts:{vector:3,knowledge:0,timeline:2},
    wall_clock_latency_ms:503
  },
  contexts:[{doc_id:'doc-7',content:'真实返回记录',metadata:{
    rrf_score:0.031,
    rrf_sources:['timeline','vector'],
    rrf_ranks:{vector:1,timeline:2}
  }}]
}});
if (normalized.fusionMode !== 'rrf_2_sources') process.exit(10);
if (!normalized.auditComplete || !normalized.evidencePresent) process.exit(11);
if (normalized.sources[1].status !== 'timeout' || normalized.sources[1].candidateCount !== 0) process.exit(12);
if (normalized.results[0].docId !== 'doc-7' || normalized.results[0].score !== 0.031) process.exit(13);
const missing = audit.normalizeRRFResponse({contexts:[]});
if (missing.auditComplete || missing.evidencePresent || missing.fusionMode !== 'unknown') process.exit(14);
'''
        completed = subprocess.run(
            [node, "-e", script, str(AUDIT_HELPER)],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(completed.returncode, 0, completed.stderr)

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
