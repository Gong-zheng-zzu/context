#!/usr/bin/env python3
import importlib.util
import json
import os
import stat
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("validate_submission_package.py")
SPEC = importlib.util.spec_from_file_location("validate_submission_package", MODULE_PATH)
assert SPEC and SPEC.loader
validator = importlib.util.module_from_spec(SPEC)
sys.modules["validate_submission_package"] = validator
SPEC.loader.exec_module(validator)


FAKE_FFPROBE = r'''#!/usr/bin/env python3
import json
print(json.dumps({"format": {"duration": "87.5"}, "streams": [
    {"codec_type": "video", "codec_name": "h264", "width": 1280, "height": 720},
    {"codec_type": "audio", "codec_name": "aac"}
]}))
'''


class SubmissionPackageTests(unittest.TestCase):
    def make_package(self):
        root = Path(tempfile.mkdtemp())
        for name in ("01_报名表.pdf", "02_申报书.docx", "03_展示PPT.pptx", "04_展示视频.mp4"):
            (root / name).write_bytes(b"placeholder")
        probe = root / "fake_ffprobe.py"
        probe.write_text(FAKE_FFPROBE, encoding="utf-8")
        probe.chmod(probe.stat().st_mode | stat.S_IXUSR)
        return root, probe

    def test_valid_package_with_probe(self):
        root, probe = self.make_package()
        report = validator.validate_package(root, ffprobe=str(probe))
        self.assertTrue(report.passed, report.findings)
        self.assertEqual("h264", report.video_probe["video_codec"])

    def test_missing_artifacts_are_reported(self):
        root = Path(tempfile.mkdtemp())
        report = validator.validate_package(root)
        self.assertFalse(report.passed)
        self.assertEqual(4, sum(item.code == "missing-file" for item in report.findings))

    def test_size_limit_is_enforced(self):
        root, probe = self.make_package()
        (root / "01_报名表.pdf").write_bytes(b"x" * (validator.MAX_REGISTRATION_BYTES + 1))
        report = validator.validate_package(root, ffprobe=str(probe))
        self.assertFalse(report.passed)
        self.assertTrue(any(item.code == "file-too-large" for item in report.findings))

    def test_video_metadata_is_enforced(self):
        root, _ = self.make_package()
        probe = root / "bad_probe.py"
        probe.write_text("print('{}')\n", encoding="utf-8")
        report = validator.validate_package(root, ffprobe=str(probe))
        self.assertFalse(report.passed)
        self.assertTrue(any(item.code == "video-codec" for item in report.findings))

    def test_secret_and_sensitive_filename_are_rejected(self):
        root, probe = self.make_package()
        (root / ".env").write_text("TOKEN=secret", encoding="utf-8")
        (root / "notes_github_pat.txt").write_text("github_pat_abcdefghijklmnopqrstuvwxyz", encoding="utf-8")
        report = validator.validate_package(root, ffprobe=str(probe))
        self.assertFalse(report.passed)
        self.assertTrue(any(item.code == "sensitive-filename" for item in report.findings))
        self.assertTrue(any(item.code == "secret-content" for item in report.findings))

    def test_json_report_is_serializable(self):
        root, probe = self.make_package()
        report = validator.validate_package(root, ffprobe=str(probe))
        json.dumps(report.as_json(), ensure_ascii=False)


if __name__ == "__main__":
    unittest.main()
