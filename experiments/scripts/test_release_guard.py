#!/usr/bin/env python3
"""Tests for the public competition release guard."""

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("release_guard.py")
SPEC = importlib.util.spec_from_file_location("release_guard", MODULE_PATH)
assert SPEC and SPEC.loader
release_guard = importlib.util.module_from_spec(SPEC)
sys.modules["release_guard"] = release_guard
SPEC.loader.exec_module(release_guard)


class ReleaseGuardTests(unittest.TestCase):
    def test_retired_command_is_blocked_on_active_surface(self):
        findings = release_guard.scan_active_text(
            "README.md", "go run cmd/server/main.go\n"
        )
        self.assertTrue(any(item.rule == "stdio-entrypoint" for item in findings))

    def test_explanatory_legacy_reference_is_not_an_execution_match(self):
        findings = release_guard.scan_active_text(
            "docs/competition/guide.md",
            "engine.go 是已弃用的原型；HTTP 主链路不使用它。",
        )
        self.assertEqual([], findings)

    def test_legacy_tool_registration_is_blocked(self):
        findings = release_guard.scan_active_text(
            "web/competition_console.html",
            'tools: ["generate_chart"]',
        )
        self.assertTrue(any(item.rule == "fabricated-agent-tool" for item in findings))

    def test_secret_path_detection_allows_templates(self):
        findings = release_guard.scan_private_paths(
            [".env", ".env.example", "experiments/runtime_env/live.env", "docs/key.pem"]
        )
        paths = {item.path for item in findings}
        self.assertEqual({".env", "experiments/runtime_env/live.env", "docs/key.pem"}, paths)

    def test_private_key_example_is_not_a_private_key(self):
        findings = release_guard.scan_active_text(
            "docs/competition/guide.md",
            "格式示例：-----BEGIN PRIVATE KEY-----",
        )
        self.assertEqual([], findings)

    def test_private_key_block_is_blocked(self):
        findings = release_guard.scan_active_text(
            "README.md",
            "-----BEGIN PRIVATE KEY-----\n" +
            ("A" * 48)
            + "\n-----END PRIVATE KEY-----",
        )
        self.assertTrue(any(item.rule == "private-key" for item in findings))

    def test_repository_current_scope_passes(self):
        findings = release_guard.check_repository(Path(__file__).resolve().parents[2])
        self.assertEqual([], findings, "active release scope has unsafe references")


if __name__ == "__main__":
    unittest.main()
