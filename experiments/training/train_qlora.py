#!/usr/bin/env python3
"""Run the bounded Qwen2.5-3B QLoRA job or validate it with --dry-run.

Dry-run performs provenance/config/dependency discovery only. It never writes a
"trained" marker and must not be cited as a completed training run.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import os
import platform
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pipeline_common import PIPELINE_SCHEMA, file_sha256, load_simple_yaml, validate_qlora_config, value_sha256, write_json_atomic
from prepare_reviewed_dataset import validate_prepared_dataset

REQUIRED_GPU_PACKAGES = ("torch", "transformers", "datasets", "peft", "bitsandbytes", "accelerate")


def dependency_probe() -> dict[str, bool]:
    return {name: importlib.util.find_spec(name) is not None for name in REQUIRED_GPU_PACKAGES}


def build_plan(dataset_dir: Path, config_path: Path) -> dict[str, Any]:
    dataset = validate_prepared_dataset(dataset_dir)
    config = load_simple_yaml(config_path)
    config_gate = validate_qlora_config(config)
    dependencies = dependency_probe()
    return {
        "schema_version": PIPELINE_SCHEMA,
        "artifact_type": "qlora_training_plan",
        "status": "validated_not_trained",
        "dataset_artifact_sha256": dataset["artifact_sha256"],
        "source_dataset_sha256": dataset["source_dataset_sha256"],
        "review_chain_head": dataset["review_chain_head"],
        "config_sha256": config_gate["config_sha256"],
        "profile": config_gate["profile"],
        "base_model": config["base_model"],
        "output_model": config["output_model"],
        "dependencies": dependencies,
        "gpu_required_for_training": True,
        "weights_in_plan": False,
        "host": {"system": platform.system(), "machine": platform.machine()},
    }


def _format_example(row: dict[str, Any]) -> tuple[str, str]:
    instruction = (
        "从合成护理记录中抽取 O-M-P-R 因果关系。仅返回 JSON，禁止补写原文不存在的医学机制。"
        "每个字段必须提供原文字符区间 spans，end 为开区间，且 text[start:end] 必须等于字段值。"
        "格式为 {\"relations\":[{\"object\":...,\"mediator\":...,\"property\":...,\"result\":...,"
        "\"spans\":{\"object\":{\"start\":...,\"end\":...},\"mediator\":{\"start\":...,\"end\":...},"
        "\"property\":{\"start\":...,\"end\":...},\"result\":{\"start\":...,\"end\":...}}}]}。\n"
        f"护理记录：{row['text']}\n"
    )
    answer = json.dumps({"relations": row["relations"]}, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return instruction, answer


def run_training(dataset_dir: Path, config_path: Path, output_dir: Path) -> dict[str, Any]:
    plan = build_plan(dataset_dir, config_path)
    missing = [name for name, available in plan["dependencies"].items() if not available]
    if missing:
        raise RuntimeError(f"GPU training dependencies are missing: {', '.join(missing)}; use --dry-run for validation")

    import torch
    from datasets import Dataset
    from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
    from transformers import (
        AutoModelForCausalLM,
        AutoTokenizer,
        BitsAndBytesConfig,
        DataCollatorForSeq2Seq,
        Trainer,
        TrainingArguments,
    )

    if not torch.cuda.is_available():
        raise RuntimeError("CUDA GPU is required; CPU training is intentionally disabled")
    device_name = torch.cuda.get_device_name(0)
    total_vram = int(torch.cuda.get_device_properties(0).total_memory)
    config = load_simple_yaml(config_path)
    output_dir.mkdir(parents=True, exist_ok=False)
    tokenizer = AutoTokenizer.from_pretrained(config["base_model"], trust_remote_code=False, use_fast=True)
    if tokenizer.pad_token_id is None:
        tokenizer.pad_token = tokenizer.eos_token
    quantization = BitsAndBytesConfig(
        load_in_4bit=True,
        bnb_4bit_quant_type="nf4",
        bnb_4bit_compute_dtype=torch.float16,
        bnb_4bit_use_double_quant=True,
    )
    model = AutoModelForCausalLM.from_pretrained(
        config["base_model"],
        quantization_config=quantization,
        torch_dtype=torch.float16,
        device_map={"": 0},
        trust_remote_code=False,
    )
    model = prepare_model_for_kbit_training(model, use_gradient_checkpointing=True)
    model.config.use_cache = False
    lora = LoraConfig(
        r=int(config["lora_r"]),
        lora_alpha=int(config["lora_alpha"]),
        lora_dropout=float(config["lora_dropout"]),
        bias="none",
        task_type="CAUSAL_LM",
        target_modules=["q_proj", "k_proj", "v_proj", "o_proj", "gate_proj", "up_proj", "down_proj"],
    )
    model = get_peft_model(model, lora)

    def tokenize(row: dict[str, Any]) -> dict[str, Any]:
        prompt, answer = _format_example(row)
        prompt_ids = tokenizer(prompt, add_special_tokens=True, truncation=True, max_length=int(config["max_seq_length"]))["input_ids"]
        remaining = max(1, int(config["max_seq_length"]) - len(prompt_ids))
        answer_ids = tokenizer(answer + tokenizer.eos_token, add_special_tokens=False, truncation=True, max_length=remaining)["input_ids"]
        input_ids = prompt_ids + answer_ids
        return {"input_ids": input_ids, "attention_mask": [1] * len(input_ids), "labels": [-100] * len(prompt_ids) + answer_ids}

    def load_split(split: str) -> Any:
        filename = "frozen_test.jsonl" if split == "test" else f"{split}.jsonl"
        rows = [json.loads(line) for line in (dataset_dir / filename).read_text(encoding="utf-8").splitlines() if line.strip()]
        return Dataset.from_list(rows).map(tokenize, remove_columns=list(rows[0].keys()))

    training_args = TrainingArguments(
        output_dir=str(output_dir / "checkpoints"),
        per_device_train_batch_size=1,
        per_device_eval_batch_size=1,
        gradient_accumulation_steps=16,
        learning_rate=float(config["learning_rate"]),
        num_train_epochs=float(config["num_train_epochs"]),
        fp16=True,
        bf16=False,
        gradient_checkpointing=True,
        eval_strategy="steps",
        eval_steps=int(config["eval_steps"]),
        save_steps=int(config["save_steps"]),
        logging_steps=10,
        save_total_limit=2,
        report_to=[],
        seed=int(config["seed"]),
        data_seed=int(config["seed"]),
        optim="paged_adamw_8bit",
        max_grad_norm=0.3,
    )
    trainer = Trainer(
        model=model,
        args=training_args,
        train_dataset=load_split("train"),
        eval_dataset=load_split("validation"),
        data_collator=DataCollatorForSeq2Seq(tokenizer=tokenizer, padding=True, label_pad_token_id=-100),
    )
    started_at = datetime.now(timezone.utc)
    trainer.train()
    adapter_dir = output_dir / "adapter"
    trainer.save_model(str(adapter_dir))
    tokenizer.save_pretrained(str(adapter_dir))
    ended_at = datetime.now(timezone.utc)
    adapter_files = sorted(path for path in adapter_dir.rglob("*") if path.is_file())
    adapter_fingerprint = value_sha256({str(path.relative_to(adapter_dir)): file_sha256(path) for path in adapter_files})
    result = {
        **plan,
        "artifact_type": "qlora_training_run",
        "status": "training_completed_unapproved",
        "started_at": started_at.isoformat(),
        "ended_at": ended_at.isoformat(),
        "gpu": {"name": device_name, "vram_bytes": total_vram},
        "adapter_fingerprint": adapter_fingerprint,
        "adapter_path": "adapter",
        "approved_for_activation": False,
        "note": "Completion is not an accuracy claim; frozen evaluation is still required.",
    }
    result["run_sha256"] = value_sha256(result)
    write_json_atomic(output_dir / "training_run.json", result)
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True, help="prepared reviewed dataset directory")
    parser.add_argument("--config", type=Path, default=Path(__file__).with_name("qlora_config.yaml"))
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    if args.dry_run:
        result = build_plan(args.dataset, args.config)
        result["plan_sha256"] = value_sha256(result)
        write_json_atomic(args.output, result)
    else:
        result = run_training(args.dataset, args.config, args.output)
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
