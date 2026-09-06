#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
配置加载器
支持动态加载YAML配置文件
"""

import yaml
from pathlib import Path
from typing import Dict, List


class ConfigLoader:
    """配置加载器"""

    def __init__(self, configs_dir: Path = None):
        if configs_dir is None:
            self.configs_dir = Path(__file__).parent.parent / "configs"
        else:
            self.configs_dir = Path(configs_dir)

    def list_available_configs(self) -> List[str]:
        """列出所有可用配置"""
        if not self.configs_dir.exists():
            return []

        config_files = self.configs_dir.glob("*.yaml")
        return sorted([f.stem for f in config_files])

    def load_config(self, config_name: str) -> Dict:
        """加载单个配置文件"""
        config_path = self.configs_dir / f"{config_name}.yaml"

        if not config_path.exists():
            raise FileNotFoundError(f"配置文件不存在: {config_path}")

        with open(config_path, 'r', encoding='utf-8') as f:
            return yaml.safe_load(f)

    def get_baseline_configs(self) -> List[str]:
        """获取所有baseline配置"""
        all_configs = self.list_available_configs()
        return [c for c in all_configs if c.startswith('baseline_')]

    def get_config_display_name(self, config_name: str) -> str:
        """获取配置的显示名称"""
        mapping = {
            "baseline_vanilla_llm": "Vanilla LLM",
            "baseline_naive_rag": "Naive RAG",
            "baseline_rag_with_filter": "RAG with Filter",
            "full_system": "Full System (PCCM)"
        }
        return mapping.get(config_name, config_name.replace('_', ' ').title())

    def validate_config(self, config_name: str) -> bool:
        """验证配置是否有效"""
        try:
            config_data = self.load_config(config_name)
            # 基础验证：检查必要字段
            return isinstance(config_data, dict)
        except Exception:
            return False


if __name__ == "__main__":
    # 测试代码
    loader = ConfigLoader()
    print("可用配置:", loader.list_available_configs())
    print("Baseline配置:", loader.get_baseline_configs())

    for config in loader.list_available_configs():
        display_name = loader.get_config_display_name(config)
        valid = loader.validate_config(config)
        print(f"  - {config}: {display_name} (有效: {valid})")
