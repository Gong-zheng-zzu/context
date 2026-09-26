#!/usr/bin/env python3
"""Generate a larger, deterministic, explicitly unreviewed O-M-P-R corpus.

The generated corpus is for training/development and annotation preparation.
It is never marked as an eligible frozen test set until independent review and
adjudication are completed.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import random
import shutil
from pathlib import Path
from typing import Any


SCHEMA_VERSION = "causal-ompr-expanded-v1"
ANNOTATION_STATUS = "synthetic_unreviewed"
DEFAULT_COUNTS = {"train": 2400, "validation": 400, "test": 400}
SPLIT_ORDER = ("train", "validation", "test")
GROUPS = {"train": 240, "validation": 40, "test": 40}
NAMES = ("李爷爷", "张奶奶", "王爷爷", "赵奶奶", "陈阿姨", "周叔叔", "刘奶奶", "孙爷爷")
SCENARIOS = (
    ("medication_fall", "服用降压药", "体位性低血压", "洗手间滑倒风险"),
    ("insulin_hypoglycemia", "胰岛素剂量偏高", "血糖下降", "出冷汗并头晕"),
    ("bedsores", "长期卧床", "局部压力增加", "骶尾部压疮风险"),
    ("catheter_infection", "留置导尿管", "细菌侵入", "尿路感染风险"),
    ("aspiration", "吞咽功能减退", "误吸风险增加", "进食时呛咳"),
    ("delirium", "阿尔茨海默病", "夜间谵妄", "情绪激动"),
    ("respiratory", "慢阻肺急性加重", "气道阻力增加", "呼吸困难"),
    ("dehydration", "饮水量不足", "循环血容量下降", "站立时头晕"),
    ("infection_fever", "伤口护理不充分", "局部炎症反应", "体温升高"),
    ("constipation", "活动量减少", "肠蠕动减慢", "排便困难"),
    ("sleep_risk", "夜间频繁醒来", "白天疲劳", "注意力下降"),
    ("fall_environment", "地面有积水", "足底摩擦力下降", "行走时滑倒"),
)
TEMPORAL = ("当日", "夜间", "最近三天", "持续一周", "服用后两小时")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def build_row(group: int, index: int, split: str) -> dict[str, Any]:
    scenario, mediator, prop, result = SCENARIOS[group % len(SCENARIOS)]
    subject = NAMES[(group + index) % len(NAMES)]
    mode = (group * 11 + index) % 12
    temporal = TEMPORAL[(group + index) % len(TEMPORAL)]
    if mode == 0:
        text = f"{subject}{temporal}状态平稳，完成常规护理，未见头晕、跌倒或感染表现。"
        relations: list[dict[str, str]] = []
        kind = "no_cause"
    elif mode == 1:
        text = f"{subject}否认{result}，目前未发现{prop}，仅按计划完成护理。"
        relations = []
        kind = "negation"
    elif mode == 2:
        text = f"{subject}{temporal}{mediator}后出现{prop}，随后发生{result}；同时夜间活动减少，需要陪护。"
        relations = [
            {"object": subject, "mediator": mediator, "property": prop, "result": result},
            {"object": subject, "mediator": "夜间活动减少", "property": "注意力下降", "result": "需要陪护"},
        ]
        kind = "multi_cause"
    else:
        text = f"{subject}{temporal}{mediator}，导致{prop}，最终表现为{result}。护理记录保留了该因果链。"
        relations = [{"object": subject, "mediator": mediator, "property": prop, "result": result}]
        kind = "single_cause"
    return {
        "record_id": f"{split[:1]}-{group:04d}-{index:04d}",
        "text": text,
        "template_family": f"{scenario}-family-{group:04d}",
        "split": split,
        "synthetic": True,
        "data_origin": "synthetic",
        "example_kind": kind,
        "relations": relations,
        "annotation": {
            "schema_version": SCHEMA_VERSION,
            "status": ANNOTATION_STATUS,
            "human_review_count": 0,
            "arbitrated": False,
        },
    }


def generate_dataset(output: Path, seed: int = 20260926, counts: dict[str, int] | None = None, force: bool = False) -> dict[str, Any]:
    counts = counts or DEFAULT_COUNTS.copy()
    if output.exists():
        if not force:
            raise FileExistsError(f"output directory exists; pass --force: {output}")
        shutil.rmtree(output)
    output.mkdir(parents=True)
    rng = random.Random(seed)
    manifest: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "seed": seed,
        "annotation_status": ANNOTATION_STATUS,
        "eligible_for_frozen_evaluation": False,
        "split_counts": counts.copy(),
        "template_families_are_split_disjoint": True,
        "files": {},
    }
    group_base = 0
    for split in SPLIT_ORDER:
        count = counts[split]
        groups = GROUPS[split]
        per_group, remainder = divmod(count, groups)
        rows: list[dict[str, Any]] = []
        for local_group in range(groups):
            n = per_group + (1 if local_group < remainder else 0)
            for index in range(n):
                rows.append(build_row(group_base + local_group, index, split))
        rng.shuffle(rows)
        group_base += groups
        path = output / f"{split}.jsonl"
        with path.open("w", encoding="utf-8", newline="\n") as handle:
            for row in rows:
                handle.write(json.dumps(row, ensure_ascii=False, sort_keys=True) + "\n")
        manifest["files"][split] = {"path": path.name, "records": len(rows), "sha256": sha256(path)}
    manifest["dataset_sha256"] = hashlib.sha256(
        "".join(manifest["files"][split]["sha256"] for split in SPLIT_ORDER).encode()
    ).hexdigest()
    (output / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return manifest


def validate_dataset(output: Path) -> dict[str, Any]:
    manifest = json.loads((output / "manifest.json").read_text(encoding="utf-8"))
    if manifest.get("schema_version") != SCHEMA_VERSION or manifest.get("eligible_for_frozen_evaluation") is not False:
        raise ValueError("expanded dataset must remain synthetic_unreviewed")
    families: dict[str, str] = {}
    for split in SPLIT_ORDER:
        expected = manifest["split_counts"][split]
        path = output / manifest["files"][split]["path"]
        rows = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
        if len(rows) != expected or sha256(path) != manifest["files"][split]["sha256"]:
            raise ValueError(f"invalid split {split}")
        for row in rows:
            if row["split"] != split or row["annotation"]["status"] != ANNOTATION_STATUS:
                raise ValueError(f"invalid annotation status in {split}")
            family = row["template_family"]
            if family in families and families[family] != split:
                raise ValueError(f"template leakage: {family}")
            families[family] = split
    expected_hash = hashlib.sha256("".join(manifest["files"][split]["sha256"] for split in SPLIT_ORDER).encode()).hexdigest()
    if manifest.get("dataset_sha256") != expected_hash:
        raise ValueError("dataset hash mismatch")
    return {"valid": True, "counts": manifest["split_counts"], "dataset_sha256": expected_hash}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("experiments/datasets/generated/causal_ompr_expanded_v1"))
    parser.add_argument("--seed", type=int, default=20260926)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()
    result = validate_dataset(args.output) if args.check else generate_dataset(args.output, args.seed, force=args.force)
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
