#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
详细测试第1轮 - 看看哪些类型检测失败
"""

import json
import requests
import time

API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

def login():
    """登录获取JWT Token"""
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
    return None

def detect(content, jwt_token, session_id):
    """检测"""
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {jwt_token}"
    }

    response = requests.post(
        f"{API_BASE_URL}/api/security/detect",
        json={
            "content": content,
            "user_id": "test_user",
            "session_id": session_id
        },
        headers=headers,
        timeout=5
    )

    if response.status_code == 200:
        result = response.json()
        detected = len(result.get("sensitive_infos", [])) > 0
        confidence = result.get("avg_confidence", 0)
        return detected, confidence, result
    else:
        return False, 0, {"error": f"HTTP {response.status_code}"}

def main():
    print("="*80)
    print("详细测试第1轮 - 分析各类型检测情况")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    # 加载测试数据
    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    # 测试每种类型的前5个正样本
    for info_type in ["id_card", "phone", "bank_card", "email"]:
        print(f"\n{'='*80}")
        print(f"测试类型: {info_type}")
        print(f"{'='*80}")

        samples = test_data.get(info_type, [])
        positive_samples = [s for s in samples if s["should_detect"]][:5]

        tp = 0
        fn = 0

        for idx, sample in enumerate(positive_samples):
            value = sample["value"]
            label = sample.get("label", "")

            detected, confidence, result = detect(value, jwt_token, f"test_{info_type}_{idx}")

            if detected:
                tp += 1
                print(f"  [OK] [{label}] {value} - 置信度: {confidence:.2f}")
            else:
                fn += 1
                print(f"  [FAIL] [{label}] {value} - 未检测到")
                print(f"      API响应: {result}")

            time.sleep(0.01)

        print(f"\n  统计: TP={tp}, FN={fn}, 召回率={tp/(tp+fn)*100:.1f}%")

if __name__ == "__main__":
    main()
