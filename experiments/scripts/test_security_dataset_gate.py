import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from security_dataset_gate import DatasetGateError, validate_manifest


class SecurityDatasetGateTests(unittest.TestCase):
    def _manifest(self, root: Path, benign_count: int = 2) -> Path:
        groups = {}
        for name, rows in {
            "attacks": [{"text": "攻击", "pii_spans": ["A"]}],
            "benign": [{"text": f"护理记录-{i}", "pii_spans": []} for i in range(benign_count)],
            "ambiguous": [{"text": "上下文歧义", "pii_spans": []}],
        }.items():
            path = root / f"{name}.json"
            path.write_text(json.dumps(rows, ensure_ascii=False), encoding="utf-8")
            groups[name] = {
                "path": path.name,
                "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
            }
        manifest = root / "manifest.json"
        manifest.write_text(json.dumps({"schema_version": "test-v1", "groups": groups}), encoding="utf-8")
        return manifest

    def test_accepts_hashed_three_way_dataset(self):
        with tempfile.TemporaryDirectory() as directory:
            result = validate_manifest(self._manifest(Path(directory), benign_count=2), minimum_benign=2)
            self.assertTrue(result["report_eligible"])
            self.assertEqual(result["groups"]["benign"]["rows"], 2)

    def test_rejects_positive_only_or_small_benign_group(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(DatasetGateError) as caught:
                validate_manifest(self._manifest(Path(directory), benign_count=1), minimum_benign=2)
            self.assertIn("benign: requires at least 2 rows", str(caught.exception))

    def test_rejects_hash_mismatch(self):
        with tempfile.TemporaryDirectory() as directory:
            manifest = self._manifest(Path(directory), benign_count=2)
            payload = json.loads(manifest.read_text(encoding="utf-8"))
            payload["groups"]["attacks"]["sha256"] = "0" * 64
            manifest.write_text(json.dumps(payload), encoding="utf-8")
            with self.assertRaises(DatasetGateError) as caught:
                validate_manifest(manifest, minimum_benign=2)
            self.assertIn("attacks: sha256 mismatch", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
