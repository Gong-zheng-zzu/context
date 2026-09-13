#!/usr/bin/env python3
"""Evaluate the causal extraction API against the 20 annotated O-M-P-R cases.

This evaluator deliberately separates API/protocol failures from extraction
quality. Only successful, contract-valid responses contribute to business
metrics. A successful response with no relations is a scored extraction miss.
"""

import argparse
import json
import sys
import unicodedata
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import APIResponse, BaseEvaluator
from causal_metrics import evidence_gate, field_macro_f1, normalized_text_hash, percentile


LABELS = ("object", "mediator", "property", "result")
MEDICAL_TERM_CANONICAL = {
    "甲减": "甲状腺功能减退",
    "copd": "慢性阻塞性肺疾病",
    "nsaids": "非甾体抗炎药",
}


@dataclass
class CausalResult:
    case_id: str
    run_label: str
    http_status: int
    error_type: Optional[str]
    error_message: Optional[str]
    latency_ms: float
    relation_count: int
    extracted: bool
    best_relation: Optional[Dict[str, Any]]
    field_matches: Dict[str, bool]
    tuple_match: bool
    confidence_threshold: float
    confidence_threshold_met: bool
    expected_tuple: List[str]
    predicted_tuples: List[List[str]]
    input_sha256: str
    quality: Dict[str, Any]
    execution: Dict[str, Any]
    timestamp: str


@dataclass
class CausalMetrics:
    run_label: str
    total_samples: int
    api_success_count: int
    api_error_count: int
    relation_found_count: int
    tuple_match_count: int
    tuple_and_confidence_pass_count: int
    object_match_count: int
    mediator_match_count: int
    property_match_count: int
    result_match_count: int
    relation_found_rate: Optional[float]
    tuple_accuracy: Optional[float]
    tuple_and_confidence_accuracy: Optional[float]
    object_accuracy: Optional[float]
    mediator_accuracy: Optional[float]
    property_accuracy: Optional[float]
    result_accuracy: Optional[float]
    predicted_tuple_count: int
    expected_tuple_count: int
    matched_tuple_count: int
    false_positive_tuple_count: int
    false_negative_tuple_count: int
    strict_tuple_precision: Optional[float]
    strict_tuple_recall: Optional[float]
    strict_tuple_f1: Optional[float]
    avg_best_relation_confidence: Optional[float]
    avg_latency_ms: Optional[float]
    latency_p50_ms: Optional[float]
    latency_p95_ms: Optional[float]
    field_macro_f1: Optional[float]
    field_f1: Dict[str, Optional[float]]
    evidence_gate: Dict[str, Any]


class CausalEvaluator(BaseEvaluator):
    """Score API relations against the annotated object-mediator-property-result labels."""

    def __init__(self, base_url: str = "http://localhost:8088"):
        super().__init__(base_url)
        self.repo_root = Path(__file__).resolve().parents[2]
        self.default_case_file = self.repo_root / "testdata" / "causal_test_cases.json"
        self.results_dir = self.repo_root / "experiments" / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)

    def load_annotated_cases(self, case_file: Path) -> List[Dict[str, Any]]:
        """Load and validate the canonical 20-case O-M-P-R fixture."""
        try:
            with case_file.open("r", encoding="utf-8") as handle:
                payload = json.load(handle)
        except (OSError, json.JSONDecodeError) as error:
            raise ValueError(f"Could not load annotated case file {case_file}: {error}") from error

        cases = payload.get("test_cases") if isinstance(payload, dict) else None
        if not isinstance(cases, list) or not cases:
            raise ValueError("Annotated case file must contain a non-empty test_cases list.")

        validated_cases: List[Dict[str, Any]] = []
        for index, case in enumerate(cases, start=1):
            if not isinstance(case, dict):
                raise ValueError(f"Case {index} must be an object.")
            case_id = case.get("case_id")
            input_text = case.get("input_text")
            expected = case.get("expected_output")
            if not isinstance(case_id, str) or not case_id:
                raise ValueError(f"Case {index} has no case_id.")
            if not isinstance(input_text, str) or not input_text:
                raise ValueError(f"Case {case_id} has no input_text.")
            if not isinstance(expected, dict):
                raise ValueError(f"Case {case_id} has no expected_output object.")
            missing = [label for label in LABELS if not isinstance(expected.get(label), str) or not expected[label]]
            if missing:
                raise ValueError(f"Case {case_id} has invalid expected labels: {', '.join(missing)}.")
            threshold = expected.get("confidence_threshold")
            if not isinstance(threshold, (int, float)) or isinstance(threshold, bool):
                raise ValueError(f"Case {case_id} has no numeric confidence_threshold.")
            validated_cases.append(case)

        return validated_cases

    def extract_causal_chain(self, record_text: str, min_confidence: float) -> APIResponse:
        """Call the endpoint using the documented extraction request fields."""
        return self.call_api(
            "POST",
            "/api/v1/causal/extract",
            {
                "text": record_text,
                "use_rules": True,
                "use_pmi": True,
                "use_llm": True,
                "min_confidence": min_confidence,
            },
            self.get_auth_headers(),
        )

    @staticmethod
    def _normalise(value: str) -> str:
        """Normalise formatting and established clinical abbreviations only."""
        normalized = unicodedata.normalize("NFKC", value).casefold()
        for abbreviation, canonical in MEDICAL_TERM_CANONICAL.items():
            normalized = normalized.replace(abbreviation, canonical)
        return "".join(character for character in normalized if character.isalnum())

    @classmethod
    def _label_matches(cls, actual: str, expected: str) -> bool:
        actual_normalized = cls._normalise(actual)
        expected_normalized = cls._normalise(expected)
        if not actual_normalized or not expected_normalized:
            return False
        return (
            actual_normalized == expected_normalized
            or actual_normalized in expected_normalized
            or expected_normalized in actual_normalized
        )

    @staticmethod
    def _validated_relations(data: Any) -> Tuple[Optional[List[Dict[str, Any]]], Optional[str]]:
        """Return relations or a protocol error; an empty relation list is valid."""
        if not isinstance(data, dict) or "relations" not in data:
            return None, "Response does not contain the required relations field."
        relations = data["relations"]
        if relations is None:
            return [], None
        if not isinstance(relations, list):
            return None, "Response relations field is not a list."
        for index, relation in enumerate(relations):
            if not isinstance(relation, dict):
                return None, f"Relation {index} is not an object."
            for label in LABELS:
                if not isinstance(relation.get(label), str):
                    return None, f"Relation {index} has no string {label} field."
            confidence = relation.get("confidence")
            if not isinstance(confidence, (int, float)) or isinstance(confidence, bool):
                return None, f"Relation {index} has no numeric confidence field."
        return relations, None

    def _score_relations(
        self, relations: List[Dict[str, Any]], expected: Dict[str, Any]
    ) -> Tuple[Optional[Dict[str, Any]], Dict[str, bool]]:
        best_relation: Optional[Dict[str, Any]] = None
        best_matches = {label: False for label in LABELS}
        best_score = -1
        best_confidence = -1.0

        for relation in relations:
            matches = {
                label: self._label_matches(relation[label], expected[label])
                for label in LABELS
            }
            score = sum(matches.values())
            confidence = float(relation["confidence"])
            if score > best_score or (score == best_score and confidence > best_confidence):
                best_relation = relation
                best_matches = matches
                best_score = score
                best_confidence = confidence

        return best_relation, best_matches

    @classmethod
    def _strict_tuple(cls, relation: Dict[str, Any]) -> Tuple[str, str, str, str]:
        return tuple(cls._normalise(str(relation[label])) for label in LABELS)  # type: ignore[return-value]

    def test_single_case(self, case: Dict[str, Any], run_label: str, min_confidence: float) -> CausalResult:
        expected = case["expected_output"]
        response = self.extract_causal_chain(case["input_text"], min_confidence)
        result = CausalResult(
            case_id=case["case_id"],
            run_label=run_label,
            http_status=response.http_status,
            error_type=response.error_type,
            error_message=response.error_message,
            latency_ms=response.latency_ms,
            relation_count=0,
            extracted=False,
            best_relation=None,
            field_matches={label: False for label in LABELS},
            tuple_match=False,
            confidence_threshold=float(expected["confidence_threshold"]),
            confidence_threshold_met=False,
            expected_tuple=list(self._strict_tuple(expected)),
            predicted_tuples=[],
            input_sha256=normalized_text_hash(case["input_text"]),
            quality={},
            execution={},
            timestamp=datetime.now().isoformat(),
        )

        if not response.is_valid_business_response():
            return result

        relations, contract_error = self._validated_relations(response.data)
        if contract_error:
            result.error_type = "contract_error"
            result.error_message = contract_error
            return result

        result.relation_count = len(relations)
        result.extracted = bool(relations)
        if isinstance(response.data, dict):
            quality = response.data.get("quality")
            execution = response.data.get("execution")
            if isinstance(quality, dict):
                result.quality = quality
            if isinstance(execution, dict):
                result.execution = execution
        result.predicted_tuples = [list(item) for item in sorted({self._strict_tuple(relation) for relation in relations})]
        best_relation, field_matches = self._score_relations(relations, expected)
        result.best_relation = best_relation
        result.field_matches = field_matches
        result.tuple_match = all(field_matches.values())
        result.confidence_threshold_met = (
            result.tuple_match
            and best_relation is not None
            and float(best_relation["confidence"]) >= result.confidence_threshold
        )
        return result

    def run_experiment(
        self, cases: List[Dict[str, Any]], run_label: str, min_confidence: float
    ) -> List[CausalResult]:
        results: List[CausalResult] = []
        for index, case in enumerate(cases, start=1):
            print(f"[{index}/{len(cases)}] Testing {case['case_id']}")
            result = self.test_single_case(case, run_label, min_confidence)
            results.append(result)
            if result.error_type:
                print(f"  API/contract error: {result.error_type}: {result.error_message}")
            else:
                matched = sum(result.field_matches.values())
                print(
                    f"  relations={result.relation_count} fields={matched}/4 "
                    f"tuple_match={result.tuple_match} latency={result.latency_ms:.0f}ms"
                )
        return results

    @staticmethod
    def calculate_metrics(results: List[CausalResult], run_label: str) -> CausalMetrics:
        total = len(results)
        business_results = [result for result in results if result.error_type is None]
        api_success_count = len(business_results)
        denominator = api_success_count or None

        def rate(value: int) -> Optional[float]:
            return value / denominator if denominator else None

        relation_found_count = sum(result.extracted for result in business_results)
        tuple_match_count = sum(result.tuple_match for result in business_results)
        tuple_and_confidence_pass_count = sum(
            result.confidence_threshold_met for result in business_results
        )
        label_counts = {
            label: sum(result.field_matches[label] for result in business_results)
            for label in LABELS
        }
        confidences = [
            float(result.best_relation["confidence"])
            for result in business_results
            if result.best_relation is not None
        ]
        expected_tuple_count = api_success_count
        matched_tuple_count = sum(
            tuple(result.expected_tuple) in {tuple(item) for item in result.predicted_tuples}
            for result in business_results
        )
        predicted_tuple_count = sum(len(result.predicted_tuples) for result in business_results)
        false_positive_tuple_count = predicted_tuple_count - matched_tuple_count
        false_negative_tuple_count = expected_tuple_count - matched_tuple_count
        strict_precision = matched_tuple_count / predicted_tuple_count if predicted_tuple_count else None
        strict_recall = matched_tuple_count / expected_tuple_count if expected_tuple_count else None
        strict_f1 = (
            2 * strict_precision * strict_recall / (strict_precision + strict_recall)
            if strict_precision is not None and strict_recall is not None and strict_precision + strict_recall
            else None
        )
        field_f1_macro, field_f1 = field_macro_f1(business_results)
        latency_values = [result.latency_ms for result in business_results]
        latency_p50 = percentile(latency_values, 0.50)
        latency_p95 = percentile(latency_values, 0.95)

        metric_values = dict(
            run_label=run_label,
            total_samples=total,
            api_success_count=api_success_count,
            api_error_count=total - api_success_count,
            relation_found_count=relation_found_count,
            tuple_match_count=tuple_match_count,
            tuple_and_confidence_pass_count=tuple_and_confidence_pass_count,
            object_match_count=label_counts["object"],
            mediator_match_count=label_counts["mediator"],
            property_match_count=label_counts["property"],
            result_match_count=label_counts["result"],
            relation_found_rate=rate(relation_found_count),
            tuple_accuracy=rate(tuple_match_count),
            tuple_and_confidence_accuracy=rate(tuple_and_confidence_pass_count),
            object_accuracy=rate(label_counts["object"]),
            mediator_accuracy=rate(label_counts["mediator"]),
            property_accuracy=rate(label_counts["property"]),
            result_accuracy=rate(label_counts["result"]),
            predicted_tuple_count=predicted_tuple_count,
            expected_tuple_count=expected_tuple_count,
            matched_tuple_count=matched_tuple_count,
            false_positive_tuple_count=false_positive_tuple_count,
            false_negative_tuple_count=false_negative_tuple_count,
            strict_tuple_precision=strict_precision,
            strict_tuple_recall=strict_recall,
            strict_tuple_f1=strict_f1,
            avg_best_relation_confidence=(sum(confidences) / len(confidences) if confidences else None),
            avg_latency_ms=(
                sum(result.latency_ms for result in business_results) / api_success_count
                if api_success_count
                else None
            ),
            latency_p50_ms=latency_p50,
            latency_p95_ms=latency_p95,
            field_macro_f1=field_f1_macro,
            field_f1=field_f1,
        )
        metric_values["evidence_gate"] = evidence_gate(metric_values)
        return CausalMetrics(**metric_values)

    def save_results(
        self,
        results: List[CausalResult],
        metrics: CausalMetrics,
        case_file: Path,
        min_confidence: float,
        health_message: str,
        auth_response: APIResponse,
    ) -> Path:
        timestamp = datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%SZ")
        output_file = self.results_dir / f"causal_{timestamp}.json"
        output = {
            "schema_version": 3,
            "execution_mode": "live_api",
            "timestamp_utc": datetime.now(timezone.utc).isoformat(),
            "base_url": self.base_url,
            "run_label": metrics.run_label,
            "case_file": str(case_file),
            "case_file_sha256": self.calculate_config_hash(str(case_file)),
            "requested_min_confidence": min_confidence,
            "api_contract": {
                "login_endpoint": "/api/auth/login",
                "extraction_endpoint": "/api/v1/causal/extract",
                "authentication": "Bearer JWT",
            },
            "preflight": {
                "health": health_message,
                "authentication": {
                    "http_status": auth_response.http_status,
                    "error_type": auth_response.error_type,
                },
            },
            "scoring": {
                "dataset": f"{len(results)} annotated O-M-P-R causal test cases",
                "matching": "NFKC/case normalization plus whole-label containment; no semantic similarity model",
                "strict_tuple_metrics": "Precision/Recall/F1 use case-scoped, exact normalized O-M-P-R tuples across every returned relation; API/contract failures are excluded.",
                "business_metric_denominator": "only HTTP-successful, contract-valid responses",
                "empty_relations": "valid business response scored as an extraction miss",
                "latency_percentiles": "nearest-rank p50/p95 over HTTP-successful, contract-valid responses",
                "field_f1": "per-field F1 penalizes extra returned relations as false positives",
            },
            "detailed_results": [asdict(result) for result in results],
            "metrics": asdict(metrics),
        }
        with output_file.open("w", encoding="utf-8") as handle:
            json.dump(output, handle, ensure_ascii=False, indent=2)
        print(f"Saved results: {output_file}")
        return output_file

    @staticmethod
    def print_summary(metrics: CausalMetrics) -> None:
        def display_rate(value: Optional[float]) -> str:
            return "n/a" if value is None else f"{value * 100:.1f}%"

        print("\nCausal extraction evaluation")
        print(f"  total annotated cases: {metrics.total_samples}")
        print(f"  API/contract-valid responses: {metrics.api_success_count}")
        print(f"  API/contract errors (excluded from business metrics): {metrics.api_error_count}")
        print(f"  relation found: {metrics.relation_found_count} ({display_rate(metrics.relation_found_rate)})")
        print(f"  object match: {metrics.object_match_count} ({display_rate(metrics.object_accuracy)})")
        print(f"  mediator match: {metrics.mediator_match_count} ({display_rate(metrics.mediator_accuracy)})")
        print(f"  property match: {metrics.property_match_count} ({display_rate(metrics.property_accuracy)})")
        print(f"  result match: {metrics.result_match_count} ({display_rate(metrics.result_accuracy)})")
        print(f"  strict O-M-P-R tuple match: {metrics.tuple_match_count} ({display_rate(metrics.tuple_accuracy)})")
        print(
            "  strict tuple P/R/F1: "
            f"{display_rate(metrics.strict_tuple_precision)} / "
            f"{display_rate(metrics.strict_tuple_recall)} / "
            f"{display_rate(metrics.strict_tuple_f1)} "
            f"(matched={metrics.matched_tuple_count}, predicted={metrics.predicted_tuple_count}, expected={metrics.expected_tuple_count})"
        )
        print(
            "  strict tuple plus annotated confidence threshold: "
            f"{metrics.tuple_and_confidence_pass_count} "
            f"({display_rate(metrics.tuple_and_confidence_accuracy)})"
        )
        if metrics.avg_best_relation_confidence is not None:
            print(f"  average best-relation confidence: {metrics.avg_best_relation_confidence:.3f}")
        if metrics.avg_latency_ms is not None:
            print(f"  average latency (valid responses): {metrics.avg_latency_ms:.1f}ms")
        if metrics.latency_p95_ms is not None:
            print(f"  latency p50/p95 (valid responses): {metrics.latency_p50_ms:.1f}/{metrics.latency_p95_ms:.1f}ms")
        if metrics.field_macro_f1 is not None:
            print(f"  field macro F1: {metrics.field_macro_f1:.3f}")
        print(f"  evidence gate: {'PASS' if metrics.evidence_gate['passed'] else 'FAIL'}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate causal O-M-P-R extraction against annotated cases.")
    parser.add_argument("--samples", type=int, default=20, help="Number of annotated cases to evaluate (1-N).")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API base URL.")
    parser.add_argument("--run-label", default="causal_extract", help="Label saved with this run; it does not change API behavior.")
    parser.add_argument(
        "--case-file",
        type=Path,
        default=None,
        help="Path to a causal fixture with test_cases (defaults to testdata/causal_test_cases.json).",
    )
    parser.add_argument(
        "--min-confidence",
        type=float,
        default=0.01,
        help="Endpoint filter threshold. Use a low positive value to score confidence separately.",
    )
    args = parser.parse_args()

    if args.samples < 1:
        parser.error("--samples must be at least 1.")
    if not 0.0 < args.min_confidence <= 1.0:
        parser.error("--min-confidence must be greater than 0 and at most 1.")

    evaluator = CausalEvaluator(base_url=args.base_url)
    case_file = args.case_file or evaluator.default_case_file
    try:
        cases = evaluator.load_annotated_cases(case_file)
    except ValueError as error:
        print(f"[ERROR] {error}", file=sys.stderr)
        sys.exit(2)

    if args.samples > len(cases):
        parser.error(f"--samples cannot exceed the {len(cases)} annotated cases in {case_file}.")

    healthy, message = evaluator.check_service_health()
    if not healthy:
        print(f"[ERROR] Service health check failed: {message}", file=sys.stderr)
        sys.exit(1)

    print(f"[OK] Service health check: {message}")
    auth_response = evaluator.authenticate_from_env()
    if not evaluator._jwt_token:
        error = auth_response.error_message or "Protected causal evaluation requires JWT credentials."
        print(f"[ERROR] Authentication failed: {error}", file=sys.stderr)
        sys.exit(1)
    print("[OK] JWT authentication preflight passed.")
    results = evaluator.run_experiment(cases[:args.samples], args.run_label, args.min_confidence)
    metrics = evaluator.calculate_metrics(results, args.run_label)
    evaluator.save_results(results, metrics, case_file, args.min_confidence, message, auth_response)
    evaluator.print_summary(metrics)


if __name__ == "__main__":
    main()
