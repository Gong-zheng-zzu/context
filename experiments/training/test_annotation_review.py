#!/usr/bin/env python3
"""Tests for the hash-chained annotation review manifest."""

import json
import tempfile
import unittest
from pathlib import Path

from annotation_review import append_review_event, create_review_manifest, validate_review_manifest
from prepare_causal_dataset import generate_dataset


class AnnotationReviewTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.dataset = Path(self.temporary.name) / "dataset"
        self.review_manifest = Path(self.temporary.name) / "annotation_review_manifest.json"
        generate_dataset(self.dataset)
        create_review_manifest(self.dataset, self.review_manifest)
        self.record = json.loads((self.dataset / "test.jsonl").read_text(encoding="utf-8").splitlines()[0])

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def _review(self, actor: str, relations=None, action="independent_review", minute=0) -> None:
        append_review_event(
            self.dataset,
            self.review_manifest,
            record_id=self.record["record_id"],
            actor_id=actor,
            action=action,
            relations=self.record["relations"] if relations is None else relations,
            timestamp=f"2026-09-16T10:{minute:02d}:00+08:00",
        )

    def test_two_distinct_agreeing_reviewers_finalize_record(self) -> None:
        self._review("reviewer-a", minute=1)
        self._review("reviewer-b", minute=2)
        result = validate_review_manifest(self.dataset, self.review_manifest)
        state = result["records"][self.record["record_id"]]
        self.assertEqual("reviewed_agreed", state["status"])
        self.assertEqual(["reviewer-a", "reviewer-b"], state["reviewers"])
        self.assertFalse(result["test_freeze_eligible"])

    def test_existing_manifest_cannot_be_silently_reinitialized(self) -> None:
        with self.assertRaisesRegex(FileExistsError, "already exists"):
            create_review_manifest(self.dataset, self.review_manifest)

    def test_disagreement_requires_independent_arbitrator(self) -> None:
        self._review("reviewer-a", minute=1)
        conflicting = [{"object": "测试对象", "mediator": "冲突", "property": "待仲裁", "result": "不一致"}]
        self._review("reviewer-b", relations=conflicting, minute=2)
        result = validate_review_manifest(self.dataset, self.review_manifest)
        self.assertEqual("needs_arbitration", result["records"][self.record["record_id"]]["status"])
        with self.assertRaisesRegex(ValueError, "arbitrator must be independent"):
            self._review("reviewer-a", action="arbitration", minute=3)
        self._review("arbitrator-c", action="arbitration", minute=3)
        result = validate_review_manifest(self.dataset, self.review_manifest)
        self.assertEqual("reviewed_arbitrated", result["records"][self.record["record_id"]]["status"])

    def test_same_reviewer_cannot_supply_both_reviews(self) -> None:
        self._review("reviewer-a", minute=1)
        with self.assertRaisesRegex(ValueError, "same reviewer"):
            self._review("reviewer-a", minute=2)

    def test_manifest_detects_event_tampering(self) -> None:
        self._review("reviewer-a", minute=1)
        manifest = json.loads(self.review_manifest.read_text(encoding="utf-8"))
        manifest["events"][0]["actor_id"] = "rewritten-reviewer"
        self.review_manifest.write_text(json.dumps(manifest, ensure_ascii=False), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "event hash mismatch"):
            validate_review_manifest(self.dataset, self.review_manifest)

    def test_manifest_detects_workflow_policy_tampering(self) -> None:
        manifest = json.loads(self.review_manifest.read_text(encoding="utf-8"))
        manifest["workflow"]["minimum_independent_reviewers"] = 1
        self.review_manifest.write_text(json.dumps(manifest, ensure_ascii=False), encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "workflow policy"):
            validate_review_manifest(self.dataset, self.review_manifest)

    def test_manifest_rejects_dataset_modified_after_initialization(self) -> None:
        path = self.dataset / "test.jsonl"
        path.write_text(path.read_text(encoding="utf-8") + "\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "sha256 mismatch"):
            validate_review_manifest(self.dataset, self.review_manifest)


if __name__ == "__main__":
    unittest.main()
