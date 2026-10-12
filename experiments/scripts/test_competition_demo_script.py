"""Static contract checks for the protected competition demo launcher."""

from __future__ import annotations

import unittest
from pathlib import Path


class CompetitionDemoScriptTest(unittest.TestCase):
    def test_live_launcher_uses_explicit_operational_latency_budget(self) -> None:
        script = (Path(__file__).with_name("run_competition_demo.ps1")).read_text(encoding="utf-8")
        self.assertIn("--max-p95-ms 3000", script)
        self.assertIn("Retrieval P95 exceeded the 3s operational demo gate", script)
        self.assertNotIn("Retrieval P95 did not pass the 1s gate", script)


if __name__ == "__main__":
    unittest.main()
