#!/usr/bin/env python3
"""Build a PCCM-S calibration dev set from the repository's labelled PII corpus.

Why this exists
---------------
`internal/security/calibration` ships a complete offline weight-search pipeline
(`SearchWeights`, `Evaluate`, `WriteArtifact`), but the only dev set wired into
it is the synthetic one in `synthetic_devset.go`. Calibrating on synthetic
inputs cannot support any claim about the shipped detector, so the weights stay
labelled `fixed_unvalidated`.

This script produces a dev set whose *labels come from the repository's own
labelled corpus* (`test_data/test_data_100.json`, which declares
`expected_sensitive` per sample) and whose *layer confidences come from the live
detector*, so the calibration input is traceable on both axes.

Contract notes (verified against the Go sources)
------------------------------------------------
`DevSet` requires, per sample, a non-empty `layer_confidences` map keyed by
PCCM-S layer id and a boolean `expected_sensitive`. This script therefore reads
the per-layer confidences out of the ablation endpoint response
(`POST /api/v1/security/ablation`), whose `asdf_casia_pccm` profile reports the
deterministic lab layers:

    regex_casia      -> layer 1  (regex)
    dictionary_casia -> layer 2  (dictionary)
    context_casia    -> layer 4  (context rules)

Layer 5 (LLM) is deliberately excluded by the lab profile
(`deterministic_lab_profile_excludes_llm`), so it is recorded as 0.0 rather than
invented. The labels themselves are never derived from the detector -- they are
read from the corpus -- so the resulting dev set is not circular.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from pathlib import Path
from typing import Any, Dict, List, Tuple

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from base_evaluator import BaseEvaluator  # noqa: E402

REPOSITORY_ROOT = SCRIPT_DIR.parent.parent
DEFAULT_CORPUS = REPOSITORY_ROOT / "test_data" / "test_data_100.json"
DEFAULT_OUTPUT = REPOSITORY_ROOT / "experiments" / "datasets" / "pccm_calibration" / "pccm_devset_repository_v1.json"

ABLATION_ENDPOINT = "/api/v1/security/ablation"
# 消融端点只接受 synthetic_ 前缀的样本 ID（服务端校验
# syntheticSecuritySampleIDPattern 见 internal/api/security_evaluation_handlers.go）。
# 缺少该前缀会被拒为 HTTP 400，使开发集一条样本也收不到。
ABLATION_SYNTHETIC_PREFIX = "synthetic_"
# The lab profile reports deterministic layers only; layer 5 is intentionally absent.
LAYER_NAME_TO_ID = {"regex_casia": 1, "dictionary_casia": 2, "context_casia": 4}
LAB_PROFILE = "asdf_casia_pccm"
EXCLUDED_LAYER_ID = 5

DEVSET_NAME = "pccm_calibration_repository_devset_v1"
# 如实标注来源：样本与标签均取自仓库自有标注语料，不是合成数据。
DEVSET_SOURCE = "repository"


class DevSetBuildError(RuntimeError):
    """Raised when the dev set cannot be built honestly."""


def load_labelled_corpus(corpus_file: Path) -> Tuple[List[Dict[str, Any]], str]:
    try:
        raw = corpus_file.read_bytes()
        payload = json.loads(raw.decode("utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise DevSetBuildError(f"Cannot read labelled corpus {corpus_file}: {exc}") from exc

    if not isinstance(payload, list) or not payload:
        raise DevSetBuildError(f"{corpus_file.name} must be a non-empty JSON array.")

    samples: List[Dict[str, Any]] = []
    for index, item in enumerate(payload):
        if not isinstance(item, dict):
            raise DevSetBuildError(f"{corpus_file.name}[{index}] is not a JSON object.")
        content = item.get("content")
        expected = item.get("expected_sensitive")
        if not isinstance(content, str) or not content.strip():
            raise DevSetBuildError(f"{corpus_file.name}[{index}] has no usable 'content'.")
        if not isinstance(expected, bool):
            raise DevSetBuildError(f"{corpus_file.name}[{index}] must declare a boolean 'expected_sensitive'.")
        samples.append(
            {
                "source_id": item.get("id"),
                "content": content,
                "expected_sensitive": expected,
                "sensitive_types": item.get("sensitive_types") or [],
                "category": str(item.get("category") or "unknown"),
            }
        )

    return samples, hashlib.sha256(raw).hexdigest()


def extract_layer_confidences(profiles: List[Dict[str, Any]]) -> Dict[str, float]:
    """Read deterministic layer confidences out of the ablation response.

    Missing layers are reported as 0.0 so the caller can see the difference
    between "layer answered with zero confidence" and "layer was not run".
    """
    confidences: Dict[str, float] = {str(layer_id): 0.0 for layer_id in LAYER_NAME_TO_ID.values()}
    confidences[str(EXCLUDED_LAYER_ID)] = 0.0

    for profile in profiles:
        if not isinstance(profile, dict) or profile.get("profile") != LAB_PROFILE:
            continue
        for layer in profile.get("layers") or []:
            if not isinstance(layer, dict):
                continue
            layer_id = LAYER_NAME_TO_ID.get(str(layer.get("layer") or ""))
            if layer_id is None:
                continue
            confidence = layer.get("confidence")
            if isinstance(confidence, (int, float)) and not isinstance(confidence, bool):
                confidences[str(layer_id)] = float(confidence)
    return confidences


def synthetic_sample_id(source_id: Any) -> str:
    """构造消融端点接受的样本 ID。

    端点强制 ``^synthetic_[A-Za-z0-9_-]{1,64}$``，其它形态会得到 HTTP 400，
    进而使开发集收集不到任何样本。这里对语料 ID 做字符净化并截断到允许长度。
    """
    safe = re.sub(r"[^A-Za-z0-9_-]", "_", str(source_id))
    suffix = f"pccm_devset_{safe}"[:64]
    return f"{ABLATION_SYNTHETIC_PREFIX}{suffix}"


def collect_layer_confidences(
    samples: List[Dict[str, Any]],
    evaluator: BaseEvaluator,
    progress_every: int,
) -> Tuple[List[Dict[str, Any]], List[str]]:
    """Collect layer confidences for every labelled sample.

    Returns the dev-set shaped samples plus the ids that could not be collected,
    so the caller can report them instead of silently shrinking the dev set.
    """
    collected: List[Dict[str, Any]] = []
    skipped: List[str] = []

    for index, sample in enumerate(samples, start=1):
        synthetic_id = synthetic_sample_id(sample["source_id"])
        response = evaluator.call_api(
            "POST",
            ABLATION_ENDPOINT,
            {"message": sample["content"], "sample_id": synthetic_id, "synthetic": True},
            evaluator.get_auth_headers(),
        )

        if not response.is_valid_business_response():
            skipped.append(f"{sample['source_id']}: {response.error_message or response.http_status}")
        else:
            payload = response.data if isinstance(response.data, dict) else {}
            inner = payload.get("data") if isinstance(payload.get("data"), dict) else payload
            profiles = inner.get("profiles")
            if isinstance(profiles, list) and profiles:
                collected.append(
                    {
                        "id": synthetic_id,
                        "text": sample["content"],
                        "sensitive_type": ",".join(sample["sensitive_types"]) if sample["sensitive_types"] else "",
                        "expected_sensitive": sample["expected_sensitive"],
                        "layer_confidences": extract_layer_confidences(profiles),
                    }
                )
            else:
                skipped.append(f"{sample['source_id']}: response carried no profiles")

        if progress_every > 0 and index % progress_every == 0:
            print(f"  [{index}/{len(samples)}] collected={len(collected)} skipped={len(skipped)}")

    return collected, skipped


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Build a PCCM-S calibration dev set from the repository's labelled PII corpus."
    )
    parser.add_argument("--base-url", default="http://127.0.0.1:8088", help="API base URL.")
    parser.add_argument("--corpus-file", type=Path, default=DEFAULT_CORPUS, help="Labelled corpus JSON.")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT, help="Where to write the dev set JSON.")
    parser.add_argument("--progress-every", type=int, default=20, help="Print progress every N samples.")
    args = parser.parse_args(argv)

    try:
        samples, corpus_sha256 = load_labelled_corpus(args.corpus_file)
    except DevSetBuildError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 2

    print(f"[OK] Loaded {len(samples)} labelled samples from {args.corpus_file.name} (sha256={corpus_sha256[:16]}...).")

    evaluator = BaseEvaluator(args.base_url)
    auth_response = evaluator.authenticate_from_env()
    if not evaluator._jwt_token:
        print("ERROR: authentication failed; set EVAL_USER_ID=eval_user_001 and EVAL_PASSWORD.", file=sys.stderr)
        print(f"       server said: {auth_response.error_message}", file=sys.stderr)
        return 3
    print("[OK] Authenticated for layer-confidence collection.")

    try:
        collected, skipped = collect_layer_confidences(samples, evaluator, args.progress_every)

        if not collected:
            raise DevSetBuildError(
                "No sample produced layer confidences; the ablation endpoint is required for an honest dev set."
            )

        positives = sum(1 for s in collected if s["expected_sensitive"])
        negatives = len(collected) - positives
        if positives == 0 or negatives == 0:
            raise DevSetBuildError(
                f"Dev set needs both labels to be meaningful; got positives={positives}, negatives={negatives}."
            )

        notes = (
            f"由仓库自有标注语料 {args.corpus_file.name} 构建；标签来自语料自带的 expected_sensitive 字段，"
            f"层置信度取自消融端点 {LAB_PROFILE} profile 的确定性层输出（layer 5 由 lab profile 排除，记为 0）。"
            f"样本 {len(collected)} 条（正例 {positives}、负例 {negatives}）"
            + (f"；跳过 {len(skipped)} 条" if skipped else "")
            + "。"
        )

        devset = {
            "name": DEVSET_NAME,
            "source": DEVSET_SOURCE,
            "notes": notes,
            "samples": collected,
        }
    except DevSetBuildError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 3

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(devset, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    digest = hashlib.sha256(args.output.read_bytes()).hexdigest()
    print()
    print("Dev set written")
    print(f"  path       : {args.output}")
    print(f"  name       : {devset['name']}")
    print(f"  source     : {devset['source']}")
    print(f"  samples    : {len(devset['samples'])}")
    print(f"  corpus     : {args.corpus_file.name} sha256={corpus_sha256}")
    print(f"  devset     : sha256={digest}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
