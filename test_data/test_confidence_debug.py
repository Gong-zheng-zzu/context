#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
置信度调试脚本 - 检查各类型样本的置信度
"""

import json
import requests

API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

def login():
    """登录获取JWT Token"""
    try:
        response = requests.post(
            f"{API_BASE_URL}/api/role/login",
            json={
                "user_id": "test_user",
                "password": "health_assistant_2024",
                "role": "caregiver",
                "workspace_id": "default"
            },
            timeout=5
        )
        if response.status_code == 200:
            data = response.json()
            if data.get("success") and "data" in data and "token" in data["data"]:
                return data["data"]["token"]
    except Exception as e:
        print(f"登录失败: {e}")
    return None

def detect_with_details(content, jwt_token, sample_id):
    """检测并返回详细信息"""
    try:
        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {jwt_token}"
        }

        response = requests.post(
            f"{API_BASE_URL}/api/security/detect",
            json={
                "content": content,
                "user_id": "test_user",
                "session_id": f"conf_debug_{sample_id}"
            },
            headers=headers,
            timeout=5
        )

        if response.status_code == 200:
            result = response.json()
            return result.get("sensitive_infos", []), result.get("avg_confidence", 0)
        else:
            return [], 0
    except Exception as e:
        print(f"检测异常: {e}")
        return [], 0

def main():
    print("="*80)
    print("置信度调试测试")
    print("="*80)

    # 登录
    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    # 加载测试数据
    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    # 测试每种类型的前5个should_detect=true的样本
    for info_type in ["phone", "bank_card", "email"]:
        print(f"\n{'='*80}")
        print(f"测试类型: {info_type.upper()}")
        print(f"{'='*80}")

        samples = test_data.get(info_type, [])
        positive_samples = [s for s in samples if s["should_detect"]][:5]

        for idx, sample in enumerate(positive_samples):
            value = sample["value"]
            label = sample["label"]

            infos, avg_conf = detect_with_details(value, jwt_token, f"{info_type}_{idx}")

            if infos:
                print(f"\n{label}: {value}")
                print(f"  检测到 {len(infos)} 个敏感信息:")
                for info in infos:
                    print(f"    - 类型: {info['type']}, 置信度: {info['confidence']:.3f}")
            else:
                print(f"\n{label}: {value}")
                print(f"  未检测到 (可能置信度低于阈值0.45)")

if __name__ == "__main__":
    main()
