#!/usr/bin/env python3
"""Offline checks for report_builder eligibility and rendering rules."""

import importlib.util
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("report_builder.py")
SPEC = importlib.util.spec_from_file_location("report_builder", MODULE_PATH)
report_builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(report_builder)
RETRIEVAL_MIN_COMPLETE_QUERIES = report_builder.RETRIEVAL_MIN_COMPLETE_QUERIES
SECURITY_REQUIRED_REQUESTS = report_builder.SECURITY_REQUIRED_REQUESTS
SECURITY_BENIGN_REQUIRED_REQUESTS = report_builder.SECURITY_BENIGN_REQUIRED_REQUESTS
SECURITY_AMBIGUOUS_REQUIRED_REQUESTS = report_builder.SECURITY_AMBIGUOUS_REQUIRED_REQUESTS
CAUSAL_REQUIRED_TEST_SAMPLES = report_builder.CAUSAL_REQUIRED_TEST_SAMPLES
ReportBuilder = report_builder.ReportBuilder


SHA256 = "a" * 64


def formal_gate(experiment):
    gate = {"status": "eligible", "report_eligible": True, "manifest_sha256": SHA256}
    if experiment == "security":
        gate["groups"] = {name: {"sha256": SHA256} for name in ("attacks", "benign", "ambiguous")}
    else:
        gate["frozen_test"] = {"sample_count": CAUSAL_REQUIRED_TEST_SAMPLES, "sha256": SHA256, "review_status": "reviewed_arbitrated"}
    return gate


def provenance(config_name="baseline_naive_rag"):
    return {
        "outcome": "succeeded",
        "report_eligible": True,
        "smoke_status": "passed",
        "evaluator_exit_code": 0,
        "configuration": {
            "name": config_name,
            "base_env_sha256": SHA256,
            "overlay_env_sha256": SHA256,
            "effective_env_sha256": SHA256,
        },
        "data": {"datasets_tree_sha256": SHA256},
        "environment": {"platform": "offline-test"},
    }


def retrieval_result(total=RETRIEVAL_MIN_COMPLETE_QUERIES):
    config_name = "baseline_naive_rag"
    return {
        "execution_mode": "live_api",
        "evaluation_mode": "multi_source_rrf_evaluation_only",
        "total_queries": total,
        "configs": [config_name],
        "runtime_config": {"config_hash": SHA256},
        "dataset": {"ground_truth_sha256": SHA256, "annotation_status": "approved_double_reviewer", "report_eligible": True, "annotator_count": 2, "adjudication_status": "completed"},
        "formal_dataset_gate": formal_gate("retrieval"),
        "detailed_results": [{"query_id": str(index), "retrieval_metadata": {"retrieval_fusion_mode": "rrf_3_sources", "source_statuses": {"vector": "success", "knowledge": "success", "timeline": "success"}, "source_latency_ms": {"vector": 1, "knowledge": 2, "timeline": 3}, "wall_clock_latency_ms": 4, "source_rankings": {"vector": {"doc-1": 1}, "knowledge": {"doc-1": 1}, "timeline": {"doc-1": 1}}}} for index in range(total)],
        "aggregated_metrics": {
            config_name: {
                "total_queries": total,
                "api_success_count": total,
                "api_error_count": 0,
                "mrr": 0.417,
                "precision_at_5": 0.250,
                "avg_latency_ms": 12.5,
            }
        },
        "runner_provenance": provenance(config_name),
    }


def security_result(successes=SECURITY_REQUIRED_REQUESTS):
    rows = [{"sample_id": str(index), "sample_kind": "attack"} for index in range(SECURITY_REQUIRED_REQUESTS)]
    rows += [{"sample_id": f"benign-{index}", "sample_kind": "benign"} for index in range(SECURITY_BENIGN_REQUIRED_REQUESTS)]
    rows += [{"sample_id": f"ambiguous-{index}", "sample_kind": "ambiguous"} for index in range(SECURITY_AMBIGUOUS_REQUIRED_REQUESTS)]
    return {
        "execution_mode": "live_api",
        "dataset_manifest": [{"path": "formal.json", "sample_count": SECURITY_REQUIRED_REQUESTS}],
        "formal_dataset_gate": formal_gate("security"),
        "pccm_calibration": {"status": "validated", "calibration_source": "devset_calibrated_v1"},
        "detailed_results": rows,
        "configuration": {"label": "full_system"},
        "aggregated_metrics": {
            "total_samples": SECURITY_REQUIRED_REQUESTS,
            "api_success_count": successes,
            "api_error_count": SECURITY_REQUIRED_REQUESTS - successes,
            "metrics_status": "complete" if successes == SECURITY_REQUIRED_REQUESTS else "incomplete_api_coverage",
            "attack_success_rate": 0.125,
            "defense_success_rate": 0.96,
            "benign_total": SECURITY_BENIGN_REQUIRED_REQUESTS,
            "benign_api_success_count": SECURITY_BENIGN_REQUIRED_REQUESTS,
            "benign_api_error_count": 0,
            "benign_pass_rate": 0.99,
            "false_positive_rate": 0.01,
            "avg_latency_ms": 10.0,
            "by_attack_type": {
                name: {"attack_success": {"rate": 0.04}}
                for name in (
                    "prompt_injection", "memory_poisoning", "privilege_escalation",
                    "privacy_leakage", "hallucination_induction", "unlearning_bypass",
                )
            },
        },
        "runner_provenance": provenance("full_system"),
    }


def security_result_v4():
    data = security_result()
    data["schema_version"] = 4
    data["detailed_results"] = (
        [{"sample_id": str(index), "sample_kind": "attack"} for index in range(SECURITY_REQUIRED_REQUESTS)]
        + [{"sample_id": f"benign-{index}", "sample_kind": "benign"} for index in range(SECURITY_BENIGN_REQUIRED_REQUESTS)]
        + [{"sample_id": f"ambiguous-{index}", "sample_kind": "ambiguous"} for index in range(SECURITY_AMBIGUOUS_REQUIRED_REQUESTS)]
    )
    data["aggregated_metrics"].update({
        "benign_total": SECURITY_BENIGN_REQUIRED_REQUESTS,
        "benign_api_success_count": SECURITY_BENIGN_REQUIRED_REQUESTS,
        "benign_api_error_count": 0,
        "benign_pass_rate": 0.98,
        "false_positive_rate": 0.02,
    })
    return data


class ReportBuilderTest(unittest.TestCase):
    def setUp(self):
        self.builder = ReportBuilder()

    def test_chinese_font_fallback_is_configured_for_chart_output(self):
        configured_fonts = report_builder.matplotlib.rcParams["font.family"]
        self.assertIn("Noto Sans SC", configured_fonts)
        self.assertIn("Microsoft YaHei", configured_fonts)
        self.assertFalse(report_builder.matplotlib.rcParams["axes.unicode_minus"])

    def test_retrieval_requires_at_least_fifty_complete_raw_queries(self):
        valid, reason = self.builder._validate_result("retrieval", retrieval_result())
        self.assertTrue(valid, reason)

        valid, reason = self.builder._validate_result("retrieval", retrieval_result(RETRIEVAL_MIN_COMPLETE_QUERIES - 1))
        self.assertFalse(valid)
        self.assertIn(str(RETRIEVAL_MIN_COMPLETE_QUERIES), reason)

    def test_security_requires_120_effective_responses_from_120_requests(self):
        valid, reason = self.builder._validate_result("security", security_result())
        self.assertTrue(valid, reason)

        valid, reason = self.builder._validate_result("security", security_result(SECURITY_REQUIRED_REQUESTS - 1))
        self.assertFalse(valid)
        self.assertIn(str(SECURITY_REQUIRED_REQUESTS), reason)

    def test_security_schema_v4_requires_one_hundred_benign_responses(self):
        valid, reason = self.builder._validate_result("security", security_result_v4())
        self.assertTrue(valid, reason)

        data = security_result_v4()
        data["aggregated_metrics"]["benign_api_success_count"] -= 1
        valid, reason = self.builder._validate_result("security", data)
        self.assertFalse(valid)
        self.assertIn(str(SECURITY_BENIGN_REQUIRED_REQUESTS), reason)

    def test_raw_rows_and_runner_provenance_are_required(self):
        data = retrieval_result()
        del data["runner_provenance"]
        valid, reason = self.builder._validate_result("retrieval", data)
        self.assertFalse(valid)
        self.assertIn("runner_provenance", reason)

        data = retrieval_result()
        data["detailed_results"] = []
        valid, reason = self.builder._validate_result("retrieval", data)
        self.assertFalse(valid)
        self.assertIn("raw rows", reason)

    def test_report_renders_observations_and_missing_reason_without_targets(self):
        self.builder.rejected_results["security"] = ["security.json: incomplete API denominator"]
        html = self.builder._generate_full_report({
            "security": None,
            "causal": None,
            "retrieval": retrieval_result(),
            "unlearning": None,
        })
        self.assertIn("0.417", html)
        self.assertIn("\u672a\u8fbe\u5230\u62a5\u544a\u95e8\u69db", html)
        for forbidden in ("\u76ee\u6807", "&lt;10%", "&gt;85%", "&gt;0.80", "\u5e94\u8fbe\u5230"):
            self.assertNotIn(forbidden, html)


if __name__ == "__main__":
    unittest.main()
