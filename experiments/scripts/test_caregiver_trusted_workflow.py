#!/usr/bin/env python3
"""Contract tests for the caregiver-facing trusted-care bridge."""

from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
CAREGIVER = ROOT / "web" / "caregiver_chat.html"
CHAT_COMMON = ROOT / "web" / "js" / "chat-common.js"
WORKFLOW = ROOT / "web" / "js" / "caregiver-trusted-workflow.js"


class CaregiverTrustedWorkflowTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.caregiver = CAREGIVER.read_text(encoding="utf-8")
        cls.chat_common = CHAT_COMMON.read_text(encoding="utf-8")
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")

    def test_caregiver_page_loads_the_trusted_workflow_and_has_compact_controls(self):
        self.assertIn('js/caregiver-trusted-workflow.js', self.caregiver)
        for identifier in ('trustedCareAutoCausal', 'trustedCareRetrieve', 'trustedCareImage'):
            self.assertIn(f'id="{identifier}"', self.caregiver)
        self.assertIn('可信护理辅助', self.caregiver)
        self.assertIn('.input-container { align-items: stretch; flex-direction: column;', self.caregiver)
        self.assertIn('.agent-toggle-row { width: 100%; flex-wrap: wrap;', self.caregiver)

    def test_saved_records_emit_an_in_memory_event_for_optional_risk_analysis(self):
        self.assertIn("new CustomEvent('caregiver:record-saved'", self.chat_common)
        self.assertIn('content: recordContent', self.chat_common)
        self.assertNotIn('localStorage.setItem(\'caregiver:record-saved\'', self.chat_common)

    def test_agent_mode_uses_the_read_only_controlled_agent_when_available(self):
        self.assertIn("typeof window.runCaregiverTrustedAgent === 'function'", self.chat_common)
        self.assertIn('/api/v1/agent/execute', self.workflow)
        self.assertIn("payload.write_applied !== false", self.workflow)
        self.assertIn("read_only_authoritative_tools", self.workflow)

    def test_workflow_uses_authorized_results_without_exposing_audit_or_delete_controls(self):
        self.assertIn('/api/v1/causal/extract', self.workflow)
        self.assertIn('/api/mcp/tools/retrieve_context', self.workflow)
        self.assertIn('/api/v1/multimodal/image/deidentify', self.workflow)
        self.assertIn('evaluationRetrievalOnly: true', self.workflow)
        self.assertIn("original_returned !== false", self.workflow)
        for forbidden in ('/api/sessions/', 'risk-rerank', '/api/v1/evidence/episodes', 'localStorage'):
            self.assertNotIn(forbidden, self.workflow)

    def test_workflow_javascript_has_valid_syntax_when_node_is_available(self):
        node = shutil.which('node')
        if not node:
            self.skipTest('node is not installed')
        completed = subprocess.run([node, '--check', str(WORKFLOW)], capture_output=True, text=True, encoding='utf-8', errors='replace', check=False)
        self.assertEqual(completed.returncode, 0, completed.stderr)


if __name__ == '__main__':
    unittest.main()
