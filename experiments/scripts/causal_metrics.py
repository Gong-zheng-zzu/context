#!/usr/bin/env python3
"""Deterministic metrics and evidence helpers for causal evaluations.

The functions in this module are intentionally independent from HTTP clients so
they can be regression-tested offline and reused by live and fixture evaluators.
"""

from __future__ import annotations

import hashlib
import re
import unicodedata
from typing import Any, Iterable, Mapping, Sequence


def normalized_text_hash(text: str) -> str:
    """Return a stable SHA-256 for semantically equivalent input formatting."""
    normalized = unicodedata.normalize("NFKC", text)
    normalized = re.sub(r"\s+", " ", normalized).strip()
    return hashlib.sha256(normalized.encode("utf-8")).hexdigest()


def percentile(values: Iterable[float], quantile: float) -> float | None:
    """Return a deterministic nearest-rank percentile, or ``None`` if empty."""
    if not 0.0 <= quantile <= 1.0:
        raise ValueError("quantile must be between 0 and 1")
    ordered = sorted(float(value) for value in values)
    if not ordered:
        return None
    # Nearest-rank is stable across Python versions and easy to explain in a report.
    rank = max(1, int((len(ordered) * quantile) + 0.999999999))
    return ordered[rank - 1]


def field_macro_f1(
    results: Sequence[Any], labels: Sequence[str] = ("object", "mediator", "property", "result")
) -> tuple[float | None, dict[str, float | None]]:
    """Calculate per-field F1 and their macro average from evaluator results.

    Each case has one annotated relation. The best selected relation contributes
    at most one true positive per field; additional returned relations count as
    false positives, which prevents over-generation from inflating scores.
    """
    if not results:
        return None, {label: None for label in labels}

    per_field: dict[str, float | None] = {}
    for label in labels:
        tp = sum(bool(getattr(item, "field_matches", {}).get(label)) for item in results)
        expected = len(results)
        predicted = sum(int(getattr(item, "relation_count", 0)) for item in results)
        fp = max(0, predicted - tp)
        fn = max(0, expected - tp)
        precision = tp / (tp + fp) if tp + fp else None
        recall = tp / (tp + fn) if tp + fn else None
        per_field[label] = (
            2 * precision * recall / (precision + recall)
            if precision is not None and recall is not None and precision + recall
            else None
        )

    valid = [score for score in per_field.values() if score is not None]
    return (sum(valid) / len(valid) if valid else None), per_field


def evidence_gate(metrics: Mapping[str, Any], tuple_f1_min: float = 0.75, field_f1_min: float = 0.88, p95_ms_max: float = 3000.0) -> dict[str, Any]:
    """Return a machine-readable gate result without changing measured values."""
    checks = {
        "strict_tuple_f1": metrics.get("strict_tuple_f1") is not None and metrics["strict_tuple_f1"] >= tuple_f1_min,
        "field_macro_f1": metrics.get("field_macro_f1") is not None and metrics["field_macro_f1"] >= field_f1_min,
        "p95_latency_ms": metrics.get("latency_p95_ms") is not None and metrics["latency_p95_ms"] <= p95_ms_max,
    }
    return {
        "passed": all(checks.values()),
        "thresholds": {
            "strict_tuple_f1_min": tuple_f1_min,
            "field_macro_f1_min": field_f1_min,
            "p95_latency_ms_max": p95_ms_max,
        },
        "checks": checks,
    }
