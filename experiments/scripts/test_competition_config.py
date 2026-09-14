#!/usr/bin/env python3
"""Regression checks for the runnable competition configuration template."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
TEMPLATE = ROOT / "config" / ".env.competition.example"


def parse_env_template(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key] = value
    return values


class CompetitionConfigTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.values = parse_env_template(TEMPLATE)

    def test_local_model_defaults_are_bounded_for_competition_hardware(self) -> None:
        self.assertEqual("qwen2.5:3b", self.values["LLM_MODEL"])
        self.assertLessEqual(int(self.values["LLM_MAX_TOKENS"]), 512)
        self.assertLessEqual(int(self.values["LLM_TIMEOUT_SECONDS"]), 30)
        self.assertEqual("10m", self.values["OLLAMA_KEEP_ALIVE"])

    def test_auditable_security_chain_is_explicitly_enabled(self) -> None:
        for key in (
            "SECURITY_ENABLE_MULTI_LAYER",
            "SECURITY_ENABLE_PCCM",
            "SECURITY_ENABLE_CASIA",
            "SECURITY_REDACT_ENABLED",
            "SECURITY_AUDIT_ENABLED",
        ):
            self.assertEqual("true", self.values[key], key)


if __name__ == "__main__":
    unittest.main()
