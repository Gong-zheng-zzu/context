#!/usr/bin/env python3
"""Validate approval, merge the approved adapter, and create an Ollama model."""
from __future__ import annotations

import argparse
import json
import shutil
import subprocess
from pathlib import Path
from typing import Any

from pipeline_common import PIPELINE_SCHEMA, file_sha256, value_sha256, write_json_atomic


def directory_fingerprint(path: Path) -> str:
    files = sorted(item for item in path.rglob("*") if item.is_file())
    if not files:
        raise ValueError(f"adapter directory has no files: {path}")
    return value_sha256({str(item.relative_to(path)): file_sha256(item) for item in files})


def validate_export(activation_path: Path, training_run_path: Path, adapter_dir: Path) -> dict[str, Any]:
    activation = json.loads(activation_path.read_text(encoding="utf-8"))
    activation_hash = activation.get("activation_sha256")
    unhashed_activation = dict(activation)
    unhashed_activation.pop("activation_sha256", None)
    if (
        activation.get("schema_version") != PIPELINE_SCHEMA
        or activation.get("artifact_type") != "model_activation_approval"
        or activation.get("approved_for_activation") is not True
        or activation_hash != value_sha256(unhashed_activation)
    ):
        raise ValueError("model activation approval is missing, invalid, or tampered")
    run = json.loads(training_run_path.read_text(encoding="utf-8"))
    run_hash = run.get("run_sha256")
    unhashed_run = dict(run)
    unhashed_run.pop("run_sha256", None)
    if run_hash != value_sha256(unhashed_run) or run_hash != activation.get("training_run_sha256"):
        raise ValueError("training run does not match activation approval")
    fingerprint = directory_fingerprint(adapter_dir)
    if fingerprint != run.get("adapter_fingerprint") or fingerprint != activation.get("adapter_fingerprint"):
        raise ValueError("adapter weights do not match the approved training run")
    return {
        "schema_version": PIPELINE_SCHEMA,
        "artifact_type": "ollama_export_plan",
        "status": "validated_not_exported",
        "model_name": activation["ollama_model"],
        "base_model": activation["base_model"],
        "activation_sha256": activation_hash,
        "training_run_sha256": run_hash,
        "adapter_fingerprint": fingerprint,
        "weights_committed": False,
    }


def export_model(plan: dict[str, Any], adapter_dir: Path, output_dir: Path, create: bool) -> dict[str, Any]:
    if output_dir.exists():
        raise FileExistsError(f"export output already exists: {output_dir}")
    try:
        import torch
        from peft import PeftModel
        from transformers import AutoModelForCausalLM, AutoTokenizer
    except ImportError as error:
        raise RuntimeError("transformers, peft and torch are required for real export; use --dry-run") from error
    output_dir.mkdir(parents=True)
    merged_dir = output_dir / "merged_model"
    base = AutoModelForCausalLM.from_pretrained(plan["base_model"], torch_dtype=torch.float16, device_map="cpu", trust_remote_code=False)
    merged = PeftModel.from_pretrained(base, str(adapter_dir)).merge_and_unload()
    merged.save_pretrained(merged_dir, safe_serialization=True, max_shard_size="2GB")
    AutoTokenizer.from_pretrained(str(adapter_dir), trust_remote_code=False).save_pretrained(merged_dir)
    modelfile = output_dir / "Modelfile"
    modelfile.write_text(
        "FROM ./merged_model\n"
        "PARAMETER temperature 0\n"
        "PARAMETER num_ctx 1024\n"
        "SYSTEM 你是护理因果候选裁决器。仅输出符合约定结构的 JSON，不提供诊断或治疗建议。\n",
        encoding="utf-8",
    )
    result = {**plan, "status": "exported_not_registered", "modelfile_sha256": file_sha256(modelfile)}
    if create:
        executable = shutil.which("ollama")
        if not executable:
            raise RuntimeError("ollama executable was not found")
        subprocess.run([executable, "create", plan["model_name"], "-f", str(modelfile)], check=True, cwd=output_dir)
        result["status"] = "registered_in_local_ollama"
    result["export_sha256"] = value_sha256(result)
    write_json_atomic(output_dir / "export_manifest.json", result)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--activation", type=Path, required=True)
    parser.add_argument("--training-run", type=Path, required=True)
    parser.add_argument("--adapter", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--create", action="store_true", help="register the merged model in the local Ollama instance")
    args = parser.parse_args()
    plan = validate_export(args.activation, args.training_run, args.adapter)
    if args.dry_run:
        plan["plan_sha256"] = value_sha256(plan)
        write_json_atomic(args.output, plan)
        result = plan
    else:
        result = export_model(plan, args.adapter, args.output, args.create)
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
