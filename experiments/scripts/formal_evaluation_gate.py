#!/usr/bin/env python3
"""Fail-closed gate for formal competition evaluation artifacts.

This command only inspects already-produced JSON.  It never labels a fixture,
machine-derived ground truth, or historical result as a formal score.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from report_builder import ReportBuilder


REQUIRED_EXPERIMENTS = ("security", "causal", "retrieval")


def evaluate(root: Path) -> dict:
    builder = ReportBuilder()
    builder.project_root = root
    builder.results_dir = root / "results" / "raw"
    results = builder.load_latest_results()
    statuses = {}
    for experiment in REQUIRED_EXPERIMENTS:
        if results.get(experiment) is not None:
            statuses[experiment] = {"status": "eligible", "artifact": builder.selected_results.get(experiment)}
        else:
            statuses[experiment] = {
                "status": "blocked",
                "reason": (builder.rejected_results.get(experiment) or ["no candidate artifact"])[0],
            }
    passed = all(item["status"] == "eligible" for item in statuses.values())
    return {
        "schema": "formal-evaluation-gate-v1",
        "passed": passed,
        "required_experiments": list(REQUIRED_EXPERIMENTS),
        "experiments": statuses,
        "report_policy": "Only eligible artifacts may replace historical or controlled-preliminary results.",
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument("--output", type=Path)
    args = parser.parse_args(argv)
    report = evaluate(args.root.resolve())
    rendered = json.dumps(report, ensure_ascii=False, indent=2) + "\n"
    print(rendered, end="")
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
