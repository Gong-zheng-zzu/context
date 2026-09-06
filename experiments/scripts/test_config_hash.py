#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
测试配置hash功能
"""

import sys
from pathlib import Path

# 添加当前目录到Python路径
sys.path.insert(0, str(Path(__file__).parent))

from base_evaluator import BaseEvaluator

def test_config_hash():
    """测试配置hash功能"""
    print("=" * 60)
    print("测试配置hash功能")
    print("=" * 60)

    evaluator = BaseEvaluator()

    # Test 1: Get config path
    print("\n[Test 1] Get config file path...")
    config_path = evaluator.get_config_path('baseline_naive_rag')
    if config_path:
        print(f'[PASS] Config path found: {config_path}')
    else:
        print('[FAIL] Config path not found')
        return False

    # Test 2: Calculate hash
    print("\n[Test 2] Calculate config hash...")
    hash_value = evaluator.calculate_config_hash(config_path)
    print(f'[PASS] Config hash: {hash_value}')

    # Test 3: Verify hash format (64 hex chars)
    print("\n[Test 3] Verify hash format...")
    if len(hash_value) == 64 and all(c in '0123456789abcdef' for c in hash_value):
        print('[PASS] Hash format valid (64 hex characters)')
    else:
        print(f'[FAIL] Invalid hash format: {hash_value}')
        return False

    # Test 4: Test idempotency
    print("\n[Test 4] Test idempotency...")
    hash_value2 = evaluator.calculate_config_hash(config_path)
    if hash_value == hash_value2:
        print('[PASS] Hash calculation is idempotent')
    else:
        print('[FAIL] Hash not idempotent')
        return False

    # Test 5: Different configs have different hashes
    print("\n[Test 5] Test different configs have different hashes...")
    config_path2 = evaluator.get_config_path('full_system')
    if not config_path2:
        print('[FAIL] full_system config not found')
        return False

    hash_value3 = evaluator.calculate_config_hash(config_path2)
    if hash_value != hash_value3:
        print(f'[PASS] Different configs have different hashes')
        print(f'   baseline_naive_rag: {hash_value[:16]}...')
        print(f'   full_system: {hash_value3[:16]}...')
    else:
        print('[FAIL] Different configs have same hash')
        return False

    print("\n" + "=" * 60)
    print("[SUCCESS] All Python config hash tests passed!")
    print("=" * 60)
    return True

if __name__ == "__main__":
    success = test_config_hash()
    sys.exit(0 if success else 1)
