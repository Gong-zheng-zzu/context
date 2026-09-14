#!/usr/bin/env python3
"""Contract tests for the deterministic causal training dataset builder."""

import tempfile
import unittest
from pathlib import Path

from prepare_causal_dataset import (
    SCHEMA_VERSION,
    SPLIT_COUNTS,
    generate_dataset,
    validate_dataset,
)


class CausalDatasetTests(unittest.TestCase):
    def test_generate_has_required_splits_and_no_family_leakage(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "causal_ompr"
            manifest = generate_dataset(output)
            self.assertEqual(SCHEMA_VERSION, manifest["schema_version"])
            self.assertEqual(SPLIT_COUNTS, manifest["split_counts"])
            validation = validate_dataset(output, expected_seed=20260914)
            self.assertTrue(validation["valid"])
            self.assertEqual(SPLIT_COUNTS, validation["counts"])

    def test_generation_is_reproducible_and_overwrite_requires_force(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "causal_ompr"
            first = generate_dataset(output)
            with self.assertRaises(FileExistsError):
                generate_dataset(output)
            second = generate_dataset(output, force=True)
            self.assertEqual(first["dataset_sha256"], second["dataset_sha256"])

    def test_frozen_test_rows_are_synthetic_and_have_review_metadata(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "causal_ompr"
            generate_dataset(output)
            rows = [
                __import__("json").loads(line)
                for line in (output / "test.jsonl").read_text(encoding="utf-8").splitlines()
            ]
            self.assertEqual(100, len(rows))
            self.assertTrue(all(row["synthetic"] is True for row in rows))
            self.assertTrue(all(row["annotation"]["reviewers"] == 2 for row in rows))
            self.assertTrue(all(row["annotation"]["arbitrated"] is True for row in rows))


if __name__ == "__main__":
    unittest.main()
