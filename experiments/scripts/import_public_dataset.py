#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把一个外部公开数据集导入为本项目消融评测可用的样本集。

为什么需要外部数据
------------------
本项目此前的评测语料全部由自己生成。自己生成、自己评测的闭环无法回答「换一个团队的
语料，结论是否仍成立」。本脚本引入一个**第三方独立生成**的数据集，使敏感信息识别的
召回结论不再只依赖自建语料。

选定的数据集
------------
``wan9yu/pii-bench-zh``（Hugging Face）

* 许可证：**Apache 2.0**（允许使用、修改、再分发，须保留版权与许可声明）
* 规模：8,000 条样本 / 23,206 个 PII 实体
* 标签：person, phone, id_number, address, email, bank_card, passport, license_plate
* 两个子集：``formal``（正式文本）与 ``chat``（含 emoji、口语化、把号码重排为空格或连字符的噪声文本）

映射规则（以及为什么只映射两类）
--------------------------------
本项目自有的敏感类型是 ``id_card`` / ``phone`` / ``medical_record`` / ``blood_pressure``。
外部标签中只有两类有**结构等价**的对应：

* ``id_number`` -> ``id_card``（18 位，含 MOD 11-2 校验位）
* ``phone``     -> ``phone``（11 位手机号）

``medical_record`` 与 ``blood_pressure`` 在外部数据集中没有对应标签，因此外部评测**只能**
覆盖两类敏感信息。这一点会写入产物清单，不得被表述为覆盖全部类型。

收录判定：一条样本只要含至少一个上述目标类型的实体就收录，并标记为「应脱敏」。样本里
同时出现的 address / email 等非目标实体不影响该判定——检测器把它们一并标出也不算错。

为什么没有负样本
----------------
外部数据集几乎全部由 PII 模板生成，且其非目标类型（person / address / email / bank_card /
passport / license_plate）在本项目的类型体系里**本身就是敏感信息**。用它们当「不应报警」
的负样本会得出错误结论：检测器标出银行卡号并不是误报。因此本导入器**只产出正样本**，
其测量结果只能用于**召回率**，不能用于误报率。
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, Iterable, List, Tuple

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent.parent

DEFAULT_CACHE_DIR = REPO_ROOT / "experiments" / "datasets" / "external" / "pii_bench_zh"

# 外部标签 -> 本项目敏感类型。未列出的标签不参与映射。
LABEL_MAP = {"id_number": "id_card", "phone": "phone"}
TARGET_LABELS = tuple(LABEL_MAP)

# 外部子集 -> 本项目消融类别。chat 子集注入了空格/连字符重排等噪声，与 bypass_attack
# 的语义一致；formal 子集是规范写法，对应 standard。
SUBSET_PLAN = {
    "formal": {
        "url": "https://huggingface.co/datasets/wan9yu/pii-bench-zh/resolve/main/data/pii_bench_zh.jsonl",
        "filename": "pii_bench_zh.jsonl",
        "category": "standard",
        "attack_type": "external_formal",
    },
    "chat": {
        "url": "https://huggingface.co/datasets/wan9yu/pii-bench-zh/resolve/main/data/pii_bench_zh_chat.jsonl",
        "filename": "pii_bench_zh_chat.jsonl",
        "category": "bypass_attack",
        "attack_type": "external_chat_noisy",
    },
}

DATASET_INFO = {
    "name": "pii-bench-zh",
    "publisher": "wan9yu (Hugging Face)",
    "url": "https://huggingface.co/datasets/wan9yu/pii-bench-zh",
    "license": "Apache-2.0",
    "license_url": "https://www.apache.org/licenses/LICENSE-2.0",
    "citation": (
        "@dataset{pii_bench_zh_2026,\n"
        "  title={PII Bench ZH: Chinese PII Detection Benchmark},\n"
        "  author={wan9yu},\n"
        "  year={2026},\n"
        "  url={https://huggingface.co/datasets/wan9yu/pii-bench-zh},\n"
        "  publisher={Hugging Face}\n"
        "}"
    ),
    "attribution_required": True,
    "provenance_note": (
        "Third-party, template-generated synthetic data. Independent of this repository's "
        "own generators, but still synthetic: it does not substitute for a human-annotated "
        "corpus when arguing external validity."
    ),
}


def sha256_bytes(payload: bytes) -> str:
    return hashlib.sha256(payload).hexdigest()


def download(url: str, destination: Path, timeout: int) -> bytes:
    """下载到本地缓存。缓存命中时直接复用，避免重复下载。"""
    if destination.exists():
        return destination.read_bytes()
    import requests  # 与仓库其它实验脚本一致，已是既有依赖

    response = requests.get(url, timeout=timeout)
    response.raise_for_status()
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(response.content)
    return response.content


def parse_jsonl(payload: bytes) -> List[Dict[str, Any]]:
    records: List[Dict[str, Any]] = []
    for line_number, line in enumerate(payload.decode("utf-8").splitlines(), 1):
        stripped = line.strip()
        if not stripped:
            continue
        try:
            records.append(json.loads(stripped))
        except json.JSONDecodeError as exc:
            raise ValueError(f"line {line_number} is not valid JSON: {exc}") from exc
    return records


def normalise_spans(record: Dict[str, Any]) -> List[Dict[str, Any]]:
    """把一条记录的实体列表规范化为 ``[{label, text, start, end}]``。

    不同版本的外部数据可能用不同键名承载实体列表，这里同时接受几种常见写法，并对缺
    字段的记录直接报错——静默跳过会让「收录了多少条」失去依据。
    """
    entities = record.get("entities") or record.get("spans") or record.get("labels")
    if entities is None:
        raise ValueError("record has no entities/spans/labels field")
    if isinstance(entities, dict):
        candidates = entities.get("entities") or entities.get("spans") or []
    else:
        candidates = entities

    spans: List[Dict[str, Any]] = []
    for item in candidates:
        if not isinstance(item, dict):
            continue
        label = item.get("label") or item.get("type") or item.get("entity")
        text = item.get("text") or item.get("value") or ""
        start = item.get("start")
        end = item.get("end")
        if label is None:
            continue
        spans.append({"label": str(label), "text": str(text), "start": start, "end": end})
    return spans


def build_samples(
    records: Iterable[Dict[str, Any]],
    subset: str,
    category: str,
    attack_type: str,
    start_id: int,
) -> Tuple[List[Dict[str, Any]], Dict[str, Any]]:
    """把外部记录映射为本项目的消融样本，并回报映射统计。"""
    samples: List[Dict[str, Any]] = []
    next_id = start_id
    scanned = 0
    skipped_no_target = 0
    observed_labels: Counter = Counter()
    mapped_type_counts: Counter = Counter()

    for record in records:
        scanned += 1
        text = record.get("text") or record.get("content") or ""
        if not isinstance(text, str) or not text.strip():
            continue
        spans = normalise_spans(record)
        for span in spans:
            observed_labels[span["label"]] += 1

        target_types = sorted({LABEL_MAP[span["label"]] for span in spans if span["label"] in LABEL_MAP})
        if not target_types:
            skipped_no_target += 1
            continue
        for mapped in target_types:
            mapped_type_counts[mapped] += 1

        samples.append(
            {
                "id": next_id,
                "content": text,
                "expected_sensitive": True,
                "sensitive_types": target_types,
                "attack_type": attack_type,
                "category": category,
                "description": (
                    f"外部公开数据集 pii-bench-zh（{subset} 子集）第 {scanned} 条；"
                    f"命中本项目的目标类型：{', '.join(target_types)}"
                ),
            }
        )
        next_id += 1

    report = {
        "subset": subset,
        "records_scanned": scanned,
        "records_without_target_type": skipped_no_target,
        "samples_emitted": len(samples),
        "observed_external_labels": dict(sorted(observed_labels.items())),
        "mapped_type_counts": dict(sorted(mapped_type_counts.items())),
    }
    return samples, report


def main(argv: List[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cache-dir", type=Path, default=DEFAULT_CACHE_DIR,
                        help="下载缓存与产物的目录")
    parser.add_argument("--timeout", type=int, default=120)
    parser.add_argument("--subsets", default="formal,chat",
                        help="要导入的子集，逗号分隔")
    args = parser.parse_args(argv)

    selected = [name.strip() for name in args.subsets.split(",") if name.strip()]
    unknown = [name for name in selected if name not in SUBSET_PLAN]
    if unknown:
        print(f"ERROR: unknown subset(s): {', '.join(unknown)}", file=sys.stderr)
        return 2

    cache_dir = args.cache_dir
    raw_dir = cache_dir / "raw"
    all_samples: List[Dict[str, Any]] = []
    subset_reports: List[Dict[str, Any]] = []
    source_files: List[Dict[str, Any]] = []
    next_id = 1

    for subset in selected:
        plan = SUBSET_PLAN[subset]
        try:
            payload = download(plan["url"], raw_dir / plan["filename"], args.timeout)
        except Exception as exc:  # 网络失败要明确报错，不能静默产出空数据集
            print(f"ERROR: cannot fetch {subset} from {plan['url']}: {exc}", file=sys.stderr)
            return 3

        records = parse_jsonl(payload)
        samples, report = build_samples(
            records, subset, plan["category"], plan["attack_type"], next_id
        )
        next_id += len(samples)
        all_samples.extend(samples)
        subset_reports.append(report)
        source_files.append(
            {
                "subset": subset,
                "url": plan["url"],
                "filename": plan["filename"],
                "bytes": len(payload),
                "sha256": sha256_bytes(payload),
                "source_records": len(records),
            }
        )
        print(
            f"[OK] {subset}: 读取 {len(records)} 条，收录 {len(samples)} 条"
            f"（跳过 {report['records_without_target_type']} 条无目标类型）"
        )

    if not all_samples:
        print("ERROR: no sample carried a mapped target type; refusing to emit an empty dataset",
              file=sys.stderr)
        return 4

    dataset_path = cache_dir / "external_pii_positive.json"
    dataset_path.parent.mkdir(parents=True, exist_ok=True)
    dataset_bytes = (json.dumps(all_samples, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
    dataset_path.write_bytes(dataset_bytes)

    category_counts = Counter(sample["category"] for sample in all_samples)
    type_counts = Counter(t for sample in all_samples for t in sample["sensitive_types"])
    manifest = {
        "schema": "external-dataset-import-manifest-v1",
        "imported_at_utc": datetime.now(timezone.utc).isoformat(),
        "importer": "experiments/scripts/import_public_dataset.py",
        "dataset": DATASET_INFO,
        "source_files": source_files,
        "cache_policy": (
            "Raw files are cached under the cache directory and reused on later runs. The "
            "recorded sha256 is a first-observation digest: verify it against the upstream "
            "release before treating it as an integrity guarantee."
        ),
        "mapping": {
            "label_map": LABEL_MAP,
            "target_labels": list(TARGET_LABELS),
            "unmapped_external_labels": [
                label
                for label in sorted(
                    {label for report in subset_reports for label in report["observed_external_labels"]}
                )
                if label not in LABEL_MAP
            ],
            "inclusion_rule": (
                "A record is included when it contains at least one entity whose label maps to a "
                "target type. Co-occurring non-target entities do not change the expectation."
            ),
        },
        "counts": {
            "samples": len(all_samples),
            "by_category": dict(sorted(category_counts.items())),
            "by_target_type": dict(sorted(type_counts.items())),
            "per_subset": subset_reports,
        },
        "produced_dataset": {
            "path": dataset_path.relative_to(REPO_ROOT).as_posix(),
            "sha256": sha256_bytes(dataset_bytes),
            "sample_count": len(all_samples),
        },
        "known_limits": [
            "Positive-only: the file contains no negative samples, so it can measure recall but "
            "must never be used to report a false-positive rate.",
            "Covers only the target types id_card and phone; medical_record and blood_pressure "
            "have no counterpart in this dataset.",
            "The dataset is synthetic and template-generated, so it is not a human-annotated "
            "reference and does not by itself establish external validity.",
            "About the external dataset's remaining labels (person, address, email, bank_card, "
            "passport, license_plate): several are sensitive under this project's own taxonomy, "
            "so they are deliberately not converted into negative samples.",
        ],
    }
    manifest_path = cache_dir / "external_pii_positive.manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    print(f"dataset={dataset_path}")
    print(f"manifest={manifest_path}")
    print(f"samples={len(all_samples)} categories={dict(sorted(category_counts.items()))}")
    print(f"target_types={dict(sorted(type_counts.items()))}")
    print(f"sha256={manifest['produced_dataset']['sha256']}")
    print(f"license={DATASET_INFO['license']}  attribution_required=True")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
