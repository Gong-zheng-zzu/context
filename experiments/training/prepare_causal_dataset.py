#!/usr/bin/env python3
"""Generate and validate the public, synthetic O-M-P-R dataset.

The generator is deterministic and deliberately uses template *families* as
the split unit.  This prevents a paraphrase of one causal chain leaking from
train into validation or the frozen test set.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import random
import shutil
from pathlib import Path
from typing import Any

SCHEMA_VERSION = "causal-ompr-v1"
ANNOTATION_STATUS = "synthetic_unreviewed"
SPLIT_COUNTS = {"train": 600, "validation": 100, "test": 100}
GROUPS_PER_SPLIT = {"train": 60, "validation": 10, "test": 10}

# Each family has its own wording and is assigned to exactly one split.
SCENARIOS = (
    ("medication_fall", "降压药", "体位性低血压", "洗手间滑倒"),
    ("bedsores", "长期卧床", "局部压力增加", "骶尾部压疮形成"),
    ("infection", "留置导尿管", "细菌侵入", "尿路感染"),
    ("hypoglycemia", "胰岛素剂量偏高", "血糖下降", "出冷汗并头晕"),
    ("aspiration", "吞咽功能减退", "误吸风险增加", "进食时呛咳"),
    ("delirium", "阿尔茨海默病", "夜间谵妄", "情绪激动"),
    ("pressure", "活动量减少", "局部受压", "皮肤破溃"),
    ("respiratory", "慢阻肺急性加重", "气道阻力增加", "呼吸困难"),
)
NAMES = ("李爷爷", "张奶奶", "王爷爷", "赵奶奶", "陈阿姨", "周叔叔")


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _record(group: int, index: int, split: str, rng: random.Random) -> dict[str, Any]:
    scenario, mediator, prop, result = SCENARIOS[group % len(SCENARIOS)]
    subject = NAMES[(group + index) % len(NAMES)]
    # Include an explicit no-causal and multi-causal slice while retaining
    # deterministic labels. These are still synthetic and contain no PII.
    mode = (group * 10 + index) % 10
    if mode == 0:
        text = f"{subject}今日精神状态平稳，完成晨间护理，未见头晕、跌倒或感染表现。"
        relations: list[dict[str, str]] = []
    elif mode == 1:
        text = (
            f"{subject}{mediator}后出现{prop}，随后发生{result}；"
            f"夜间复查记录该变化。"
        )
        relations = [{"object": subject, "mediator": mediator, "property": prop, "result": result}]
        # A second edge makes multi-causal parsing part of the test contract.
        relations.append(
            {"object": subject, "mediator": "夜间活动", "property": "注意力下降", "result": "需要陪护"}
        )
    elif mode == 2:
        text = f"{subject}否认{result}，目前未发现{prop}，仅按计划完成护理。"
        relations = []
    else:
        temporal = ("当日", "夜间", "最近", "持续三天")[mode % 4]
        text = f"{subject}{temporal}{mediator}，导致{prop}，最终{result}。"
        relations = [{"object": subject, "mediator": mediator, "property": prop, "result": result}]
    return {
        "record_id": f"{split[:1]}-{group:03d}-{index:02d}",
        "text": text,
        "template_family": f"{scenario}-family-{group:03d}",
        "split": split,
        "synthetic": True,
        "data_origin": "synthetic",
        "relations": relations,
        "annotation": {
            "schema_version": SCHEMA_VERSION,
            "status": ANNOTATION_STATUS,
            "human_review_count": 0,
            "arbitrated": False,
        },
    }


def generate_dataset(output_dir: Path, seed: int = 20260914, force: bool = False) -> dict[str, Any]:
    """Write deterministic JSONL splits and a manifest, returning the manifest."""
    if output_dir.exists():
        if not force:
            raise FileExistsError(f"output directory already exists; pass --force to replace: {output_dir}")
        shutil.rmtree(output_dir)
    output_dir.mkdir(parents=True)
    rng = random.Random(seed)
    manifest: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "seed": seed,
        "annotation_status": ANNOTATION_STATUS,
        "eligible_for_frozen_evaluation": False,
        "split_counts": SPLIT_COUNTS.copy(),
        "files": {},
    }
    group_base = 0
    for split, count in SPLIT_COUNTS.items():
        groups = GROUPS_PER_SPLIT[split]
        per_group, remainder = divmod(count, groups)
        rows: list[dict[str, Any]] = []
        for local_group in range(groups):
            n = per_group + (1 if local_group < remainder else 0)
            for index in range(n):
                rows.append(_record(group_base + local_group, index, split, rng))
        group_base += groups
        path = output_dir / f"{split}.jsonl"
        with path.open("w", encoding="utf-8", newline="\n") as handle:
            for row in rows:
                handle.write(json.dumps(row, ensure_ascii=False, sort_keys=True) + "\n")
        manifest["files"][split] = {"path": path.name, "records": len(rows), "sha256": _sha256(path)}
    manifest["dataset_sha256"] = hashlib.sha256(
        "".join(manifest["files"][split]["sha256"] for split in SPLIT_COUNTS).encode()
    ).hexdigest()
    (output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    return manifest


def validate_dataset(output_dir: Path, expected_seed: int | None = None) -> dict[str, Any]:
    manifest_path = output_dir / "manifest.json"
    if not manifest_path.exists():
        raise ValueError(f"missing manifest: {manifest_path}")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    if manifest.get("schema_version") != SCHEMA_VERSION:
        raise ValueError("unsupported dataset schema")
    if expected_seed is not None and manifest.get("seed") != expected_seed:
        raise ValueError("dataset seed does not match requested seed")
    families: dict[str, str] = {}
    for split, expected in SPLIT_COUNTS.items():
        path = output_dir / manifest["files"].get(split, {}).get("path", f"{split}.jsonl")
        if not path.exists():
            raise ValueError(f"missing split file: {path}")
        rows = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
        if len(rows) != expected:
            raise ValueError(f"{split} has {len(rows)} rows, expected {expected}")
        if _sha256(path) != manifest["files"][split]["sha256"]:
            raise ValueError(f"{split} sha256 mismatch")
        for row in rows:
            if row.get("split") != split or row.get("synthetic") is not True:
                raise ValueError(f"invalid row metadata in {split}")
            annotation = row.get("annotation", {})
            if (
                row.get("data_origin") != "synthetic"
                or annotation.get("status") != ANNOTATION_STATUS
                or annotation.get("human_review_count") != 0
                or annotation.get("arbitrated") is not False
            ):
                raise ValueError(f"generated row falsely claims human review in {split}")
            family = row.get("template_family")
            if not family:
                raise ValueError("missing template_family")
            if family in families and families[family] != split:
                raise ValueError(f"template family leaks across splits: {family}")
            families[family] = split
    expected_hash = hashlib.sha256(
        "".join(manifest["files"][split]["sha256"] for split in SPLIT_COUNTS).encode()
    ).hexdigest()
    if manifest.get("dataset_sha256") != expected_hash:
        raise ValueError("dataset sha256 mismatch")
    if (
        manifest.get("annotation_status") != ANNOTATION_STATUS
        or manifest.get("eligible_for_frozen_evaluation") is not False
    ):
        raise ValueError("generated manifest must remain synthetic_unreviewed")
    return {"valid": True, "counts": SPLIT_COUNTS.copy(), "dataset_sha256": expected_hash}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("experiments/datasets/causal_ompr"))
    parser.add_argument("--seed", type=int, default=20260914)
    parser.add_argument("--check", action="store_true", help="validate an existing dataset")
    parser.add_argument("--force", action="store_true", help="replace an existing output directory")
    args = parser.parse_args()
    if args.check:
        print(json.dumps(validate_dataset(args.output, args.seed), ensure_ascii=False))
    else:
        print(json.dumps(generate_dataset(args.output, args.seed, args.force), ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
