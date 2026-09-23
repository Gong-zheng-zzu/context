#!/usr/bin/env python3
"""Deployment contracts for the local static-preview CORS allowlist."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
COMPOSE = ROOT / "docker-compose.yml"


class DockerComposeCORSTests(unittest.TestCase):
    def test_static_preview_origins_are_explicitly_allowlisted(self):
        compose = COMPOSE.read_text(encoding="utf-8")
        origins = next(
            line.strip()
            for line in compose.splitlines()
            if "ALLOWED_ORIGINS=" in line
        )

        self.assertIn("http://localhost:8092", origins)
        self.assertIn("http://127.0.0.1:8092", origins)
        self.assertNotIn("ALLOWED_ORIGINS=*", origins)


if __name__ == "__main__":
    unittest.main()
