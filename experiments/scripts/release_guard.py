#!/usr/bin/env python3
"""Release-scope guard for the public competition snapshot.

The repository still contains migration and historical prototypes.  This guard
does not delete them; it prevents active competition surfaces from invoking
those paths and prevents credentials/runtime secrets from entering a release.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Sequence


ACTIVE_ROOT_FILES = {
    "README.md",
    "README-en.md",
    "Dockerfile",
    "docker-compose.yml",
}
ACTIVE_PREFIXES = (
    "web/competition_console",
    "docs/competition/",
    "experiments/COMPETITION_",
    "experiments/scripts/run_competition_demo",
    "experiments/scripts/smoke_test.py",
)

# These are intentionally executable/import patterns.  A competition runbook
# may mention a retired path to explain the boundary without becoming an entry.
LEGACY_EXECUTION_PATTERNS = (
    ("stdio-entrypoint", re.compile(r"\b(?:go\s+(?:run|build|test)|\.\/)?[^\n]*cmd/server/main\.go\b")),
    ("simulated-retrieval-import", re.compile(r"(?:import|from|require|go\s+(?:run|build)|python[^\n]*)[^\n]*multi_dimensional_retrieval[^\n]*engine\.go")),
    ("fabricated-agent-tool", re.compile(r"(?:register|tools?\s*[:=]|tool_name|toolName|execute|call|invoke)[^\n]*(?:vital_signs|resident_profile|trend_analysis|generate_chart)")),
)

PRIVATE_PATH_PATTERNS = (
    re.compile(r"(^|/)(?:\.env|\.env\.[^/]+)$", re.IGNORECASE),
    re.compile(r"(^|/)(?:effective\.env|.*\.(?:pem|key|p12|pfx|jks))$", re.IGNORECASE),
    re.compile(r"(^|/)experiments/runtime_env/", re.IGNORECASE),
    re.compile(r"(^|/)(?:credentials?|secrets?)[^/]*\.(?:json|ya?ml|toml|ini)$", re.IGNORECASE),
)
PRIVATE_PATH_ALLOWLIST = {".env.example", "docker-compose.override.yml.example"}
SECRET_PATTERNS = (
    ("github-token", re.compile(r"\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b")),
)
PRIVATE_KEY_BLOCK = re.compile(
    r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----\s*"
    r"[\r\n]+[A-Za-z0-9+/=\r\n]{40,}"
)


@dataclass(frozen=True)
class Finding:
    rule: str
    path: str
    line: int | None = None
    detail: str = ""

    def __str__(self) -> str:
        location = self.path if self.line is None else f"{self.path}:{self.line}"
        suffix = f" ({self.detail})" if self.detail else ""
        return f"[{self.rule}] {location}{suffix}"


def tracked_files(root: Path) -> list[str]:
    """Return repository-relative tracked paths, independent of shell platform."""
    result = subprocess.run(
        ["git", "ls-files", "-z"], cwd=root, check=True, capture_output=True
    )
    return [item for item in result.stdout.decode("utf-8").split("\0") if item]


def is_active_path(path: str) -> bool:
    normalized = path.replace("\\", "/")
    return normalized in ACTIVE_ROOT_FILES or normalized.startswith(ACTIVE_PREFIXES)


def scan_active_text(path: str, text: str) -> list[Finding]:
    findings: list[Finding] = []
    for number, line in enumerate(text.splitlines(), 1):
        for rule, pattern in LEGACY_EXECUTION_PATTERNS:
            if pattern.search(line):
                findings.append(Finding(rule, path, number, line.strip()[:180]))
        for rule, pattern in SECRET_PATTERNS:
            if pattern.search(line):
                findings.append(Finding(rule, path, number))
    if PRIVATE_KEY_BLOCK.search(text):
        findings.append(Finding("private-key", path))
    return findings


def scan_private_paths(paths: Iterable[str]) -> list[Finding]:
    findings: list[Finding] = []
    for path in paths:
        normalized = path.replace("\\", "/")
        name = normalized.rsplit("/", 1)[-1]
        if normalized in PRIVATE_PATH_ALLOWLIST or (
            name.startswith(".env.") and name.endswith(".example")
        ):
            continue
        if any(pattern.search(normalized) for pattern in PRIVATE_PATH_PATTERNS):
            findings.append(Finding("private-file", normalized))
    return findings


def check_repository(root: Path) -> list[Finding]:
    paths = tracked_files(root)
    findings = scan_private_paths(paths)
    for relative in paths:
        if not is_active_path(relative):
            continue
        file_path = root / relative
        try:
            text = file_path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            # Binary assets are not competition source/config surfaces.
            continue
        findings.extend(scan_active_text(relative, text))
    return findings


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args(argv)
    try:
        findings = check_repository(args.root.resolve())
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"release guard could not inspect repository: {exc}", file=sys.stderr)
        return 2
    if findings:
        print("Release guard failed:")
        for finding in findings:
            print(f"  {finding}")
        return 1
    print("Release guard passed: active competition scope and tracked secret paths are clean.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
