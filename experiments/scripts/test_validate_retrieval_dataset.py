from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

import validate_retrieval_dataset as validator


class RetrievalDatasetReviewGateTests(unittest.TestCase):
    def _run(self, metadata: dict) -> dict:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            corpus = root / "corpus.json"
            queries = root / "queries.json"
            corpus.write_text(json.dumps({"documents": [{"doc_id": "doc-1"}]}), encoding="utf-8")
            queries.write_text(json.dumps({
                "metadata": metadata,
                "queries": [{"query_id": "q-1", "query_type": "general", "ground_truth_docs": ["doc-1"]}],
            }), encoding="utf-8")
            args = SimpleNamespace(
                experiments_dir=root,
                corpus="corpus.json",
                queries="queries.json",
                expected_corpus_count=1,
                expected_query_count=1,
                expected_type_counts="general=1",
                require_reviewed=True,
            )
            return validator.validate(args)[1]

    def test_single_reviewer_is_not_formally_eligible(self) -> None:
        report = self._run({
            "annotation_status": "approved_single_reviewer",
            "report_eligible": True,
        })
        self.assertFalse(report["queries"]["reviewed"])
        self.assertFalse(report["passed"])

    def test_double_review_and_adjudication_are_required(self) -> None:
        report = self._run({
            "annotation_status": "approved_double_reviewer",
            "annotator_count": 2,
            "adjudication_status": "completed",
            "report_eligible": True,
        })
        self.assertTrue(report["queries"]["reviewed"])
        self.assertTrue(report["passed"])


if __name__ == "__main__":
    unittest.main()
