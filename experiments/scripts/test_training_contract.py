#!/usr/bin/env python3
"""Expose the training scaffold checks to the standard experiment test run."""

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MODULE_PATH = ROOT / "experiments" / "training" / "prepare_causal_dataset.py"
SPEC = importlib.util.spec_from_file_location("prepare_causal_dataset", MODULE_PATH)
assert SPEC and SPEC.loader
module = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = module
SPEC.loader.exec_module(module)


class TrainingContractTests(unittest.TestCase):
    def test_scaffold_exports_expected_split_contract(self) -> None:
        self.assertEqual({"train": 600, "validation": 100, "test": 100}, module.SPLIT_COUNTS)
        self.assertEqual("causal-ompr-v1", module.SCHEMA_VERSION)

    def test_generator_requires_explicit_overwrite(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "dataset"
            module.generate_dataset(output)
            with self.assertRaises(FileExistsError):
                module.generate_dataset(output)


if __name__ == "__main__":
    unittest.main()
