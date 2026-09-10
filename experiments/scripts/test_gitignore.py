#!/usr/bin/env python3
"""Regression checks for repository rules that protect runtime environment files."""

from pathlib import Path
import subprocess
import unittest


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]


class GitIgnoreTest(unittest.TestCase):
    def test_environment_backups_are_ignored_and_untracked(self):
        ignore_rules = (REPOSITORY_ROOT / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("*.env.backup", ignore_rules)

        tracked_files = subprocess.run(
            ["git", "ls-files"],
            cwd=REPOSITORY_ROOT,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.splitlines()
        backups = [path for path in tracked_files if path.endswith(".env.backup")]
        self.assertEqual([], backups, f"runtime environment backups are tracked: {backups}")


if __name__ == "__main__":
    unittest.main()
