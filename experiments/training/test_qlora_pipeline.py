import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))

from annotation_review import create_review_manifest
from evaluate_activation import evaluate, load_predictions
from export_ollama import validate_export
from pipeline_common import PIPELINE_SCHEMA, file_sha256, validate_relation_evidence, value_sha256, write_json_atomic, write_jsonl
from prepare_causal_dataset import generate_dataset
from prepare_reviewed_dataset import prepare, validate_prepared_dataset
from train_qlora import _format_example, build_plan


class QLoRAPipelineTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name)
        self.config = Path(__file__).with_name("qlora_config.yaml")

    def tearDown(self):
        self.temporary.cleanup()

    def test_prepare_rejects_unreviewed_generated_dataset(self):
        dataset = self.root / "generated"
        review = self.root / "review.json"
        generate_dataset(dataset)
        create_review_manifest(dataset, review)
        with self.assertRaisesRegex(ValueError, "frozen-test gate failed"):
            prepare(dataset, review, self.root / "prepared")

    def test_prepare_materializes_only_final_review_labels(self):
        source = self.root / "source"
        source.mkdir()
        files = {}
        final_labels = {}
        for split in ("train", "validation", "test"):
            record_id = f"{split}-1"
            final_labels[record_id] = [{"object": "甲", "mediator": "乙", "property": "丙", "result": "丁"}]
            path = source / f"{split}.jsonl"
            write_jsonl(path, [{"record_id": record_id, "text": f"{split} text"}])
            files[split] = {"path": path.name}
        write_json_atomic(source / "manifest.json", {"files": files})
        gate = {
            "dataset_sha256": "d" * 64,
            "review_chain_head": "r" * 64,
            "reviewed_records": 3,
            "labels": final_labels,
        }
        output = self.root / "prepared"
        with mock.patch("prepare_reviewed_dataset.validate_fully_reviewed_dataset", return_value=gate):
            manifest = prepare(source, self.root / "unused-review.json", output)
        self.assertTrue(manifest["test_freeze_eligible"])
        self.assertEqual(validate_prepared_dataset(output)["artifact_sha256"], manifest["artifact_sha256"])
        frozen = json.loads((output / "frozen_test.jsonl").read_text(encoding="utf-8"))
        self.assertEqual(frozen["relations"], final_labels["test-1"])
        self.assertEqual(frozen["annotation_status"], "reviewed_final")

    def test_reviewed_relation_evidence_span_must_round_trip(self):
        text = "李爷爷服药后头晕并跌倒"
        relation = {
            "object": "李爷爷",
            "mediator": "服药",
            "property": "头晕",
            "result": "跌倒",
            "spans": {
                "object": {"start": 0, "end": 3},
                "mediator": {"start": 3, "end": 5},
                "property": {"start": 6, "end": 8},
                "result": {"start": 9, "end": 11},
            },
        }
        validate_relation_evidence(text, [relation], "record-1")
        relation["spans"]["result"] = {"start": 8, "end": 10}
        with self.assertRaisesRegex(ValueError, "does not match"):
            validate_relation_evidence(text, [relation], "record-1")

    def _prepared_dataset(self, count=100):
        prepared = self.root / "prepared"
        prepared.mkdir()
        files = {}
        for split in ("train", "validation", "test"):
            rows = []
            split_count = count if split == "test" else 1
            for index in range(split_count):
                negative = split == "test" and index % 10 == 0
                text = f"老人{index}未见异常。" if negative else f"老人{index}服药后出现头晕，随后跌倒。"
                subject = f"老人{index}"
                relations = [] if negative else [{
                    "object": subject,
                    "mediator": "服药",
                    "property": "头晕",
                    "result": "跌倒",
                    "spans": {
                        "object": {"start": text.index(subject), "end": text.index(subject) + len(subject)},
                        "mediator": {"start": text.index("服药"), "end": text.index("服药") + 2},
                        "property": {"start": text.index("头晕"), "end": text.index("头晕") + 2},
                        "result": {"start": text.index("跌倒"), "end": text.index("跌倒") + 2},
                    },
                }]
                rows.append(
                    {
                        "record_id": f"{split}-{index}",
                        "text": text,
                        "relations": relations,
                        "split": "frozen_test" if split == "test" else split,
                        "synthetic": True,
                        "annotation_status": "reviewed_final",
                        "source_dataset_sha256": "s" * 64,
                        "review_chain_head": "c" * 64,
                    }
                )
            name = "frozen_test.jsonl" if split == "test" else f"{split}.jsonl"
            path = prepared / name
            write_jsonl(path, rows)
            files[split] = {"path": name, "records": len(rows), "sha256": file_sha256(path)}
        manifest = {
            "schema_version": PIPELINE_SCHEMA,
            "artifact_type": "reviewed_causal_dataset",
            "source_dataset_sha256": "s" * 64,
            "review_chain_head": "c" * 64,
            "test_freeze_eligible": True,
            "reviewed_records": count + 2,
            "files": files,
        }
        manifest["artifact_sha256"] = value_sha256(manifest)
        write_json_atomic(prepared / "manifest.json", manifest)
        return prepared, manifest

    def _training_run(self, dataset_manifest, adapter=None):
        adapter = adapter or self.root / "adapter"
        adapter.mkdir(exist_ok=True)
        (adapter / "adapter.safetensors").write_bytes(b"test adapter bytes")
        fingerprint = value_sha256({"adapter.safetensors": file_sha256(adapter / "adapter.safetensors")})
        run = {
            "schema_version": PIPELINE_SCHEMA,
            "artifact_type": "qlora_training_run",
            "status": "training_completed_unapproved",
            "dataset_artifact_sha256": dataset_manifest["artifact_sha256"],
            "source_dataset_sha256": dataset_manifest["source_dataset_sha256"],
            "review_chain_head": dataset_manifest["review_chain_head"],
            "config_sha256": "q" * 64,
            "profile": "rtx2050_4gb_qwen25_3b_nf4",
            "base_model": "Qwen/Qwen2.5-3B-Instruct",
            "output_model": "contextkeeper-causal-3b",
            "dependencies": {},
            "gpu_required_for_training": True,
            "weights_in_plan": False,
            "host": {"system": "Linux", "machine": "x86_64"},
            "started_at": "2026-09-16T00:00:00+00:00",
            "ended_at": "2026-09-16T00:01:00+00:00",
            "gpu": {"name": "test GPU", "vram_bytes": 4_000_000_000},
            "adapter_fingerprint": fingerprint,
            "adapter_path": "adapter",
            "approved_for_activation": False,
            "note": "Completion is not an accuracy claim; frozen evaluation is still required.",
        }
        run["run_sha256"] = value_sha256(run)
        path = self.root / "training_run.json"
        write_json_atomic(path, run)
        return path, run, adapter

    def _predictions(self, dataset, manifest, kind, strategy):
        rows = []
        for gold in [json.loads(line) for line in (dataset / "frozen_test.jsonl").read_text(encoding="utf-8").splitlines()]:
            if strategy == "exact":
                relations = gold["relations"]
            elif strategy == "empty":
                relations = []
            else:
                raise ValueError(f"unsupported prediction strategy: {strategy}")
            rows.append({"record_id": gold["record_id"], "relations": relations, "latency_ms": 100})
        payload = {
            "schema_version": PIPELINE_SCHEMA,
            "model_kind": kind,
            "model_id": f"{kind}-test",
            "dataset_artifact_sha256": manifest["artifact_sha256"],
            "predictions": rows,
        }
        path = self.root / f"{kind}.json"
        write_json_atomic(path, payload)
        return path

    def test_training_dry_run_plan_requires_no_gpu_dependencies(self):
        dataset, manifest = self._prepared_dataset()
        with mock.patch("train_qlora.dependency_probe", return_value={name: False for name in ("torch", "transformers", "datasets", "peft", "bitsandbytes", "accelerate")}):
            plan = build_plan(dataset, self.config)
        self.assertEqual(plan["status"], "validated_not_trained")
        self.assertEqual(plan["dataset_artifact_sha256"], manifest["artifact_sha256"])
        self.assertFalse(any(plan["dependencies"].values()))

    def test_training_prompt_requires_source_spans(self):
        row = {
            "text": "李爷爷服药后头晕并跌倒",
            "relations": [{
                "object": "李爷爷", "mediator": "服药", "property": "头晕", "result": "跌倒",
                "spans": {
                    "object": {"start": 0, "end": 3}, "mediator": {"start": 3, "end": 5},
                    "property": {"start": 6, "end": 8}, "result": {"start": 9, "end": 11},
                },
            }],
        }
        prompt, answer = _format_example(row)
        self.assertIn("spans", prompt)
        self.assertIn('"spans"', answer)

    def test_prediction_without_round_trip_spans_is_rejected(self):
        dataset, manifest = self._prepared_dataset(count=2)
        path = self._predictions(dataset, manifest, "custom", "exact")
        payload = json.loads(path.read_text(encoding="utf-8"))
        payload["predictions"][1]["relations"][0].pop("spans")
        write_json_atomic(path, payload)
        gold_rows = [json.loads(line) for line in (dataset / "frozen_test.jsonl").read_text(encoding="utf-8").splitlines()]
        with self.assertRaisesRegex(ValueError, "spans"):
            load_predictions(path, "custom", manifest["artifact_sha256"], {row["record_id"]: row for row in gold_rows})

    def test_activation_is_withheld_when_custom_does_not_beat_baselines(self):
        dataset, manifest = self._prepared_dataset()
        run_path, _, _ = self._training_run(manifest)
        paths = {kind: self._predictions(dataset, manifest, kind, "empty") for kind in ("custom", "base", "rules")}
        activation = self.root / "activation.json"
        activation.write_text('{"stale":true}', encoding="utf-8")
        report = evaluate(dataset, run_path, paths, self.root / "report.json", activation)
        self.assertFalse(report["approved_for_activation"])
        self.assertFalse(activation.exists())
        self.assertIn("strict_tuple_f1", report["threshold_checks"])

    def test_activation_is_created_only_after_thresholds_and_significance(self):
        dataset, manifest = self._prepared_dataset()
        run_path, run, _ = self._training_run(manifest)
        paths = {
            "custom": self._predictions(dataset, manifest, "custom", "exact"),
            "base": self._predictions(dataset, manifest, "base", "empty"),
            "rules": self._predictions(dataset, manifest, "rules", "empty"),
        }
        activation = self.root / "activation.json"
        report = evaluate(dataset, run_path, paths, self.root / "report.json", activation)
        self.assertTrue(report["approved_for_activation"])
        self.assertTrue(all(report["threshold_checks"].values()))
        self.assertTrue(all(report["significant_improvement_checks"].values()))
        approval = json.loads(activation.read_text(encoding="utf-8"))
        self.assertEqual(approval["training_run_sha256"], run["run_sha256"])

    def test_export_rejects_tampered_approval_and_accepts_matching_adapter(self):
        dataset, manifest = self._prepared_dataset()
        run_path, run, adapter = self._training_run(manifest)
        approval = {
            "schema_version": PIPELINE_SCHEMA,
            "artifact_type": "model_activation_approval",
            "approved_for_activation": True,
            "ollama_model": "contextkeeper-causal-3b",
            "base_model": run["base_model"],
            "dataset_artifact_sha256": manifest["artifact_sha256"],
            "training_run_sha256": run["run_sha256"],
            "adapter_fingerprint": run["adapter_fingerprint"],
            "evaluation_report_sha256": "e" * 64,
            "custom_predictions_sha256": "p" * 64,
        }
        approval["activation_sha256"] = value_sha256(approval)
        activation = self.root / "activation.json"
        write_json_atomic(activation, approval)
        plan = validate_export(activation, run_path, adapter)
        self.assertEqual(plan["status"], "validated_not_exported")
        (adapter / "adapter.safetensors").write_bytes(b"tampered adapter bytes")
        with self.assertRaisesRegex(ValueError, "adapter weights"):
            validate_export(activation, run_path, adapter)
        (adapter / "adapter.safetensors").write_bytes(b"test adapter bytes")
        approval["base_model"] = "tampered/model"
        write_json_atomic(activation, approval)
        with self.assertRaisesRegex(ValueError, "tampered"):
            validate_export(activation, run_path, adapter)

    def test_prepared_manifest_tampering_is_rejected(self):
        dataset, _ = self._prepared_dataset()
        manifest_path = dataset / "manifest.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        manifest["source_dataset_sha256"] = "tampered"
        write_json_atomic(manifest_path, manifest)
        with self.assertRaisesRegex(ValueError, "hash mismatch"):
            validate_prepared_dataset(dataset)


if __name__ == "__main__":
    unittest.main()
