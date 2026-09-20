#!/usr/bin/env python3
"""Adversarial sensitive-data ablation evaluation over test_data_100.json.

The canonical ablation runner (``security_eval.py --ablation-mode``) drives the
six semantic attack categories. Those samples try to steer the assistant into
wrong behaviour and mostly carry no sensitive span, so the layered detectors
(ASDF / CASIA / PCCM) have nothing to act on and the paired deltas collapse to
zero.

The layered architecture targets a different question: can obfuscated or
context-ambiguous sensitive data still be recognised, without flagging lookalike
non-sensitive values? The repository already ships a dataset for exactly that
question -- ``test_data/test_data_100.json`` with the categories ``standard``,
``bypass_attack``, ``confusing`` and ``normal`` -- but no evaluation path
consumed it.

This runner closes that gap. It reuses ``SecurityEvaluator`` for the request,
aggregation and persistence paths so the layered delta semantics stay identical
to the canonical ablation run, and it only swaps the sample source and the
attack/benign split:

* ``expected_sensitive == true``  -> attack sample (a redact decision is correct)
* ``expected_sensitive == false`` -> benign sample (an allow decision is correct)

Every reported number comes from the live endpoint; nothing is synthesised. The
run is intended to be launched through
``experiments/scripts/run_configured_eval.ps1`` so the effective configuration
hash and dataset digest are recorded alongside the result.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Tuple

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from security_eval import SecurityAblationEvaluator  # noqa: E402

REPOSITORY_ROOT = SCRIPT_DIR.parent.parent
DEFAULT_DATASET = REPOSITORY_ROOT / "test_data" / "test_data_100.json"

# Kept distinct from ABLATION_RESULT_FILENAME_PREFIX so the report builder can
# tell an adversarial-PII ablation apart from a canonical attack-set one.
ADVERSARIAL_RESULT_PREFIX = "ablation_adversarial_security_"

CATEGORY_ORDER = ("standard", "bypass_attack", "confusing", "normal")


class AdversarialDatasetError(RuntimeError):
    """Raised when the adversarial dataset cannot be used as declared."""


def load_adversarial_samples(dataset_path: Path) -> Tuple[List[Dict[str, Any]], str]:
    """Load and validate the adversarial sensitive-data dataset.

    Returns the samples in the shape ``SecurityEvaluator`` expects plus the
    SHA-256 of the exact bytes that were read, so the digest can be recorded
    with the result.
    """
    try:
        raw = dataset_path.read_bytes()
        dataset = json.loads(raw.decode("utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise AdversarialDatasetError(f"Cannot read adversarial dataset {dataset_path}: {exc}") from exc

    if not isinstance(dataset, list) or not dataset:
        raise AdversarialDatasetError(f"{dataset_path.name} must be a non-empty JSON array.")

    samples: List[Dict[str, Any]] = []
    seen_ids: set[str] = set()
    for index, item in enumerate(dataset):
        if not isinstance(item, dict):
            raise AdversarialDatasetError(f"{dataset_path.name}[{index}] is not a JSON object.")

        content = item.get("content")
        if not isinstance(content, str) or not content.strip():
            raise AdversarialDatasetError(f"{dataset_path.name}[{index}] has no usable 'content'.")

        category = str(item.get("category") or "unknown").strip()
        if category not in CATEGORY_ORDER:
            raise AdversarialDatasetError(
                f"{dataset_path.name}[{index}] has unexpected category {category!r}; "
                f"expected one of {', '.join(CATEGORY_ORDER)}."
            )

        expected_sensitive = item.get("expected_sensitive")
        if not isinstance(expected_sensitive, bool):
            raise AdversarialDatasetError(
                f"{dataset_path.name}[{index}] must declare a boolean 'expected_sensitive'."
            )

        source_id = item.get("id")
        sample_id = f"pii_{category}_{source_id}"
        if sample_id in seen_ids:
            raise AdversarialDatasetError(f"Duplicate sample id derived from {source_id!r}.")
        seen_ids.add(sample_id)

        samples.append(
            {
                "id": sample_id,
                "type": category,
                "attack_text": content,
                "expected_behavior": "redact" if expected_sensitive else "allow",
                "success_criteria": (
                    "structured decision is redact" if expected_sensitive else "structured decision is allow"
                ),
                "sample_kind": "attack" if expected_sensitive else "benign",
                "expected_sensitive": expected_sensitive,
                "sensitive_types": item.get("sensitive_types") or [],
                "source_id": source_id,
                "description": str(item.get("description") or ""),
            }
        )

    digest = hashlib.sha256(raw).hexdigest()
    return samples, digest


def summarise(samples: List[Dict[str, Any]], digest: str, dataset_path: Path) -> None:
    counts: Dict[str, int] = {name: 0 for name in CATEGORY_ORDER}
    sensitive = 0
    for sample in samples:
        counts[sample["type"]] = counts.get(sample["type"], 0) + 1
        if sample["expected_sensitive"]:
            sensitive += 1
    breakdown = ", ".join(f"{name}={counts[name]}" for name in CATEGORY_ORDER)
    print(f"[OK] Loaded {len(samples)} adversarial samples ({breakdown}).")
    print(f"[OK] Sensitive={sensitive} expected redact, non-sensitive={len(samples) - sensitive} expected allow.")
    print(f"[OK] Dataset {dataset_path.name} sha256={digest}")


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Adversarial sensitive-data ablation evaluation (reuses the server-owned ablation endpoint)."
    )
    parser.add_argument("--base-url", default="http://localhost:8088", help="API base URL.")
    parser.add_argument("--dataset", type=Path, default=DEFAULT_DATASET, help="Adversarial dataset JSON path.")
    parser.add_argument("--config-label", default="full_system", help="Label of the already-running configuration.")
    parser.add_argument(
        "--config-evidence",
        default="",
        help="Deployment/configuration evidence recorded verbatim.",
    )
    parser.add_argument(
        "--request-interval-ms",
        type=int,
        default=0,
        help="Delay between measured requests; pacing time is excluded from per-request latency.",
    )
    parser.add_argument(
        "--max-samples",
        type=int,
        default=0,
        help="Optional cap per category for a bounded smoke run; 0 means every sample.",
    )
    args = parser.parse_args(argv)
    # Resolve before use. The default dataset is absolute while a caller-supplied
    # --dataset is typically relative, and the manifest records the path relative
    # to the repository root: without resolving, a relative path makes
    # Path.relative_to() raise "not in the subpath of" and the run aborts after the
    # dataset has already been read.
    args.dataset = args.dataset.resolve()

    evaluator = SecurityAblationEvaluator(base_url=args.base_url)

    try:
        samples, digest = load_adversarial_samples(args.dataset)
    except AdversarialDatasetError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    if args.max_samples > 0:
        capped: List[Dict[str, Any]] = []
        for name in CATEGORY_ORDER:
            capped.extend([sample for sample in samples if sample["type"] == name][: args.max_samples])
        samples = capped
        print(f"[OK] --max-samples={args.max_samples} applied; running {len(samples)} samples.")

    summarise(samples, digest, args.dataset)

    # Prefer a repository-relative path so the manifest stays portable, but a
    # dataset outside the repository is still usable and is recorded as-is.
    try:
        recorded_dataset_path = args.dataset.relative_to(REPOSITORY_ROOT).as_posix()
    except ValueError:
        recorded_dataset_path = args.dataset.as_posix()

    evaluator.dataset_manifest.append(
        {
            "path": recorded_dataset_path,
            "sample_count": len(samples),
            "sha256": digest,
            "kind": "adversarial_sensitive_data",
            "categories": list(CATEGORY_ORDER),
        }
    )

    ok, message = evaluator.authenticate_for_lightweight_evaluation()
    print(f"[{'OK' if ok else 'FAIL'}] {message}")
    if not ok:
        return 3

    ok, message = evaluator.ablation_endpoint_preflight()
    print(f"[{'OK' if ok else 'FAIL'}] {message}")
    if not ok:
        return 3

    interval_seconds = max(args.request_interval_ms, 0) / 1000.0
    attacks = [sample for sample in samples if sample["sample_kind"] == "attack"]
    benign = [sample for sample in samples if sample["sample_kind"] == "benign"]

    attack_results = evaluator.run_ablation_experiment(
        args.config_label, attacks, False, interval_seconds, "attack"
    )
    benign_results = evaluator.run_ablation_experiment(
        args.config_label, benign, False, interval_seconds, "benign"
    )

    profile_metrics, deltas = evaluator.aggregate_ablation_metrics(attack_results, benign_results)

    def rate(value: Any) -> str:
        return "n/a" if value is None else f"{value:.4f}"

    print("\nAdversarial sensitive-data ablation summary")
    for profile_name, metrics in profile_metrics.items():
        mean_latency = (metrics.latency_stats_ms or {}).get("mean")
        print(
            f"  {profile_name}: detection={rate(metrics.detection_rate)} "
            f"benign_false_positive={rate(metrics.benign_false_positive_rate)} "
            f"latency_mean_ms={mean_latency} "
            f"early_stop={metrics.early_stop_reason_counts}"
        )
    for delta in deltas:
        print(
            f"  delta {delta.baseline_profile}->{delta.profile}: "
            f"newly_detected={delta.newly_detected_count} newly_missed={delta.newly_missed_count} "
            f"detection_rate_delta={rate(delta.detection_rate_delta)} "
            f"layers_added={delta.layers_added}"
        )

    output_path = evaluator.save_ablation_results(
        args.config_label,
        args.config_evidence,
        attack_results,
        benign_results,
        profile_metrics,
        deltas,
        # 记录**实际施加**的间隔。此前这里写死为 max(args.request_interval_ms, 1100)，
        # 于是默认（0，即完全不配速）的运行也会在结果里声称做了 1100ms 配速，使「遵守
        # 限流的运行」与「撞上限流但被记为合规的运行」无法从结果中分辨。
        request_interval_ms=int(round(interval_seconds * 1000)),
    )

    # Rename to the adversarial-specific prefix so report tooling can tell this
    # run apart from a canonical attack-set ablation while keeping the exact
    # result payload produced by the shared persistence path.
    renamed = output_path.with_name(output_path.name.replace("ablation_security_", ADVERSARIAL_RESULT_PREFIX, 1))
    if renamed != output_path:
        output_path.replace(renamed)
        output_path = renamed

    api_error_count = sum(result.error_type != "" for result in attack_results + benign_results)
    if api_error_count:
        print(f"\n  API error samples: {api_error_count}")
        # 分母不完整时比率既不可比也不可报。以非零码退出，使门禁把该次运行标为
        # evaluator_failed 而不是 report-eligible——否则一份只覆盖部分样本的比率会被
        # 当成合格证据进入材料（实测一次 6677 条的运行只有 999 条被评分，其余全是
        # HTTP 429，而脚本当时仍以 0 退出）。
        print(
            "  ERROR: this run did not score every sample, so its rates rest on an incomplete "
            "denominator. Raise --request-interval-ms (the protected API allows about "
            f"{max(args.request_interval_ms, 0)} ms pacing) and rerun. The raw result has been "
            "written for diagnosis but must not be reported.",
            file=sys.stderr,
        )
        print(f"  Raw result: {output_path}")
        return 5
    print(f"  Raw result: {output_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
