from __future__ import annotations

import csv
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import finalize_double_reviewer_annotations as finalizer


class DoubleReviewerFinalizerTests(unittest.TestCase):
    def _fixture(self, rows: list[dict[str, str]]) -> tuple[Path, Path, Path, Path]:
        directory = Path(tempfile.mkdtemp())
        draft = directory / "draft.json"
        corpus = directory / "corpus.json"
        review = directory / "review.csv"
        output = directory / "reviewed.json"
        queries = [
            {"query_id": "q-1", "query_type": "general", "question": "q1", "ground_truth_docs": ["doc-1"]},
            {"query_id": "q-2", "query_type": "causal", "question": "q2", "ground_truth_docs": ["doc-2"]},
        ]
        corpus.write_text(json.dumps({"documents": [{"doc_id": "doc-1"}, {"doc_id": "doc-2"}]}), encoding="utf-8")
        draft.write_text(json.dumps({"metadata": {"source_corpus_sha256": hashlib.sha256(corpus.read_bytes()).hexdigest()}, "queries": queries}), encoding="utf-8")
        fields = list(finalizer.REQUIRED_COLUMNS)
        with review.open("w", encoding="utf-8-sig", newline="") as handle:
            writer = csv.DictWriter(handle, fieldnames=fields)
            writer.writeheader()
            writer.writerows(rows)
        return draft, corpus, review, output

    def test_agreement_and_arbitration_produce_formal_metadata(self) -> None:
        rows = [
            finalizer.review_row("q-1", "doc-1", "doc-1", "reviewer-a", "reviewer-b", "", "", ""),
            finalizer.review_row("q-2", "doc-1", "doc-2", "reviewer-a", "reviewer-b", "arbitrator-c", "doc-2", "disagreement resolved"),
        ]
        draft, corpus, review, output = self._fixture(rows)
        finalizer.finalize(draft, corpus, review, output)
        payload = json.loads(output.read_text(encoding="utf-8"))
        self.assertTrue(payload["metadata"]["report_eligible"])
        self.assertEqual("approved_double_reviewer", payload["metadata"]["annotation_status"])
        self.assertEqual(3, payload["metadata"]["annotator_count"])
        self.assertEqual("completed", payload["metadata"]["adjudication_status"])
        self.assertEqual(["doc-1", "doc-2"], [row["ground_truth_docs"][0] for row in payload["queries"]])

    def test_disagreement_without_independent_arbitrator_is_rejected(self) -> None:
        rows = [
            finalizer.review_row("q-1", "doc-1", "doc-2", "reviewer-a", "reviewer-b", "", "", ""),
            finalizer.review_row("q-2", "doc-2", "doc-2", "reviewer-a", "reviewer-a", "", "", ""),
        ]
        draft, corpus, review, output = self._fixture(rows)
        with self.assertRaisesRegex(ValueError, "arbitrator|distinct"):
            finalizer.finalize(draft, corpus, review, output)


if __name__ == "__main__":
    unittest.main()
