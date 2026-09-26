#!/usr/bin/env python3
"""Tests for the reproducible larger causal corpus."""

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = ROOT / "experiments" / "datasets" / "prepare_expanded_causal_dataset.py"
SPEC = importlib.util.spec_from_file_location("prepare_expanded_causal_dataset", MODULE_PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ExpandedCausalDatasetTests(unittest.TestCase):
    def test_expanded_default_counts_and_status(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "expanded"
            manifest = MODULE.generate_dataset(output, force=False)
            self.assertEqual({"train": 2400, "validation": 400, "test": 400}, manifest["split_counts"])
            self.assertFalse(manifest["eligible_for_frozen_evaluation"])
            self.assertEqual({"valid": True, "counts": manifest["split_counts"], "dataset_sha256": manifest["dataset_sha256"]}, MODULE.validate_dataset(output))

    def test_generation_is_reproducible_and_overwrite_is_explicit(self):
        with tempfile.TemporaryDirectory() as directory:
            first = Path(directory) / "first"
            second = Path(directory) / "second"
            a = MODULE.generate_dataset(first, seed=7)
            b = MODULE.generate_dataset(second, seed=7)
            self.assertEqual(a["dataset_sha256"], b["dataset_sha256"])
            with self.assertRaises(FileExistsError):
                MODULE.generate_dataset(first, seed=7)

    def test_generated_rows_are_not_reviewed(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "expanded"
            MODULE.generate_dataset(output, counts={"train": 24, "validation": 8, "test": 8})
            rows = []
            for split in ("train", "validation", "test"):
                rows.extend(json.loads(line) for line in (output / f"{split}.jsonl").read_text(encoding="utf-8").splitlines())
            self.assertTrue(rows)
            self.assertTrue(all(row["annotation"]["human_review_count"] == 0 for row in rows))
            self.assertTrue(any(row["example_kind"] == "no_cause" for row in rows))
            self.assertTrue(any(row["example_kind"] == "multi_cause" for row in rows))


if __name__ == "__main__":
    unittest.main()
