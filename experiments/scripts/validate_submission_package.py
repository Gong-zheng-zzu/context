#!/usr/bin/env python3
"""Validate the Strong Cyber Cup submission package without mutating it.

The validator checks the official size/format constraints, probes video
metadata with ffprobe, and rejects obvious credentials or private material.
It intentionally does not create, convert, or rename any submission artifact.
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Iterable, Sequence


MB = 1024 * 1024
MAX_REGISTRATION_BYTES = 1 * MB
MAX_DOCUMENT_BYTES = 20 * MB
MAX_VIDEO_BYTES = 20 * MB
MAX_VIDEO_SECONDS = 90.0
MIN_VIDEO_WIDTH = 800
MIN_VIDEO_HEIGHT = 600


@dataclass(frozen=True)
class Finding:
    severity: str
    code: str
    path: str = ""
    detail: str = ""

    def __str__(self) -> str:
        location = f" {self.path}" if self.path else ""
        detail = f": {self.detail}" if self.detail else ""
        return f"[{self.severity}] {self.code}{location}{detail}"


@dataclass
class ValidationReport:
    passed: bool
    findings: list[Finding] = field(default_factory=list)
    files: dict[str, str | None] = field(default_factory=dict)
    video_probe: dict[str, Any] | None = None

    def as_json(self) -> dict[str, Any]:
        return {
            "passed": self.passed,
            "findings": [asdict(item) for item in self.findings],
            "files": self.files,
            "video_probe": self.video_probe,
        }


ROLE_SPECS: dict[str, tuple[str, tuple[str, ...]]] = {
    "registration": ("01_报名表", (".pdf",)),
    "proposal": ("02_申报书", (".pdf", ".docx")),
    "ppt": ("03_展示PPT", (".ppt", ".pptx")),
    "video": ("04_展示视频", (".mp4",)),
}

SENSITIVE_NAME = re.compile(
    r"(?:^|[._-])(?:\.env|env|secret|credential|password|passwd|jwt|token|pat)(?:$|[._-])",
    re.IGNORECASE,
)
SECRET_PATTERNS = (
    re.compile(r"\b(?:ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b"),
    re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
    re.compile(r"\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b"),
)


def _candidate_files(package: Path, prefix: str, extensions: Iterable[str]) -> list[Path]:
    allowed = {extension.lower() for extension in extensions}
    return sorted(
        item
        for item in package.iterdir()
        if item.is_file()
        and item.name.startswith(prefix)
        and item.suffix.lower() in allowed
    )


def discover_files(package: Path, explicit: dict[str, Path | None]) -> tuple[dict[str, Path | None], list[Finding]]:
    findings: list[Finding] = []
    selected: dict[str, Path | None] = {}
    for role, (prefix, extensions) in ROLE_SPECS.items():
        supplied = explicit.get(role)
        if supplied is not None:
            path = supplied if supplied.is_absolute() else package / supplied
            selected[role] = path
            continue
        candidates = _candidate_files(package, prefix, extensions)
        if len(candidates) > 1:
            findings.append(Finding("error", "ambiguous-file", ", ".join(str(item.name) for item in candidates), role))
            selected[role] = None
        else:
            selected[role] = candidates[0] if candidates else None
    return selected, findings


def _check_file(path: Path | None, role: str, findings: list[Finding]) -> None:
    if path is None:
        findings.append(Finding("error", "missing-file", role, "required submission artifact was not found"))
        return
    if not path.is_file():
        findings.append(Finding("error", "missing-file", str(path), role))
        return
    expected_ext = ROLE_SPECS[role][1]
    if path.suffix.lower() not in expected_ext:
        findings.append(Finding("error", "wrong-extension", str(path), ", ".join(expected_ext)))
    limit = MAX_REGISTRATION_BYTES if role == "registration" else MAX_VIDEO_BYTES if role == "video" else MAX_DOCUMENT_BYTES
    if path.stat().st_size > limit:
        findings.append(Finding("error", "file-too-large", str(path), f"{path.stat().st_size} > {limit} bytes"))


def _probe_video(path: Path, ffprobe: str | None) -> tuple[dict[str, Any] | None, list[Finding]]:
    findings: list[Finding] = []
    executable = ffprobe or shutil.which("ffprobe")
    if not executable:
        findings.append(Finding("error", "ffprobe-unavailable", str(path), "install ffmpeg or pass --ffprobe"))
        return None, findings
    executable_path = Path(executable)
    command = [
        sys.executable if executable_path.suffix.lower() == ".py" else executable,
    ]
    if executable_path.suffix.lower() == ".py":
        command.append(executable)
    command.extend([
        "-v",
        "error",
        "-show_entries",
        "format=duration:stream=codec_type,codec_name,width,height",
        "-of",
        "json",
        str(path),
    ])
    try:
        completed = subprocess.run(command, check=True, capture_output=True, text=True, encoding="utf-8")
        payload = json.loads(completed.stdout)
    except (OSError, subprocess.CalledProcessError, json.JSONDecodeError) as exc:
        findings.append(Finding("error", "video-probe-failed", str(path), str(exc)))
        return None, findings

    streams = payload.get("streams") or []
    video_stream = next((item for item in streams if item.get("codec_type") == "video"), None)
    audio_stream = next((item for item in streams if item.get("codec_type") == "audio"), None)
    duration = float((payload.get("format") or {}).get("duration") or 0)
    probe = {
        "duration_seconds": duration,
        "video_codec": (video_stream or {}).get("codec_name"),
        "audio_codec": (audio_stream or {}).get("codec_name"),
        "width": (video_stream or {}).get("width"),
        "height": (video_stream or {}).get("height"),
    }
    if probe["video_codec"] != "h264":
        findings.append(Finding("error", "video-codec", str(path), f"expected h264, got {probe['video_codec']}"))
    if probe["audio_codec"] != "aac":
        findings.append(Finding("error", "audio-codec", str(path), f"expected aac, got {probe['audio_codec']}"))
    if duration <= 0 or duration > MAX_VIDEO_SECONDS:
        findings.append(Finding("error", "video-duration", str(path), f"expected 0 < duration <= {MAX_VIDEO_SECONDS}s, got {duration}s"))
    if not isinstance(probe["width"], int) or probe["width"] < MIN_VIDEO_WIDTH:
        findings.append(Finding("error", "video-width", str(path), f"expected >= {MIN_VIDEO_WIDTH}, got {probe['width']}"))
    if not isinstance(probe["height"], int) or probe["height"] < MIN_VIDEO_HEIGHT:
        findings.append(Finding("error", "video-height", str(path), f"expected >= {MIN_VIDEO_HEIGHT}, got {probe['height']}"))
    return probe, findings


def _scan_package(package: Path) -> list[Finding]:
    findings: list[Finding] = []
    for path in package.rglob("*"):
        if not path.is_file():
            continue
        relative = path.relative_to(package).as_posix()
        if SENSITIVE_NAME.search(path.name):
            findings.append(Finding("error", "sensitive-filename", relative))
        try:
            data = path.read_bytes()
        except OSError as exc:
            findings.append(Finding("error", "read-failed", relative, str(exc)))
            continue
        if len(data) > 50 * MB:
            continue
        text = data.decode("utf-8", errors="ignore")
        for pattern in SECRET_PATTERNS:
            if pattern.search(text):
                findings.append(Finding("error", "secret-content", relative, pattern.pattern))
    return findings


def validate_package(
    package: Path,
    explicit: dict[str, Path | None] | None = None,
    ffprobe: str | None = None,
) -> ValidationReport:
    findings: list[Finding] = []
    package = package.resolve()
    if not package.is_dir():
        return ValidationReport(False, [Finding("error", "package-missing", str(package))])
    selected, discovery_findings = discover_files(package, explicit or {})
    findings.extend(discovery_findings)
    for role, path in selected.items():
        _check_file(path, role, findings)
    video_probe = None
    video = selected.get("video")
    if video is not None and video.is_file() and video.suffix.lower() == ".mp4":
        video_probe, probe_findings = _probe_video(video, ffprobe)
        findings.extend(probe_findings)
    findings.extend(_scan_package(package))
    return ValidationReport(not any(item.severity == "error" for item in findings), findings, {
        role: str(path) if path is not None else None for role, path in selected.items()
    }, video_probe)


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", type=Path, help="directory containing the final submission artifacts")
    parser.add_argument("--registration", type=Path)
    parser.add_argument("--proposal", type=Path)
    parser.add_argument("--ppt", type=Path)
    parser.add_argument("--video", type=Path)
    parser.add_argument("--ffprobe", help="path to ffprobe; defaults to PATH lookup")
    parser.add_argument("--json-out", type=Path, help="write a machine-readable report")
    args = parser.parse_args(argv)
    report = validate_package(args.package, {
        "registration": args.registration,
        "proposal": args.proposal,
        "ppt": args.ppt,
        "video": args.video,
    }, args.ffprobe)
    for finding in report.findings:
        print(finding)
    print("Submission package: " + ("PASS" if report.passed else "FAIL"))
    if args.json_out:
        args.json_out.parent.mkdir(parents=True, exist_ok=True)
        args.json_out.write_text(json.dumps(report.as_json(), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0 if report.passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
