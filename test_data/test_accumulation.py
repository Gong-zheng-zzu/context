#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试累积效应 - 先发送400个请求，然后测试检测是否正常
"""

import json
import requests
import time

API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

def login():
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
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {jwt_token}"
    }
    try:
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
            return detected, result
        else:
            return False, {"error": f"HTTP {response.status_code}", "body": response.text}
    except Exception as e:
        return False, {"error": f"Exception: {str(e)}"}

def main():
    print("="*80)
    print("测试累积效应")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    # 阶段1：发送400个请求（模拟前2轮完整测试）
    print("\n阶段1：发送400个请求（模拟前2轮）...")
    sample_id = 0
    for round_num in range(1, 3):
        for info_type in ["id_card", "phone", "bank_card", "email"]:
            samples = test_data.get(info_type, [])
            for sample in samples:
                detect(sample["value"], jwt_token, f"warmup_{sample_id}")
                sample_id += 1
                time.sleep(0.01)
        print(f"  完成第{round_num}轮（共{sample_id}个请求）")

    print(f"\n已发送{sample_id}个请求，等待2秒...")
    time.sleep(2)

    # 阶段2：测试前12个正样本
    print("\n阶段2：测试前12个正样本...")
    tp = 0
    fn = 0
    test_id = 0

    for info_type in ["id_card", "phone", "bank_card", "email"]:
        samples = test_data.get(info_type, [])
        positive_samples = [s for s in samples if s["should_detect"]][:3]

        for sample in positive_samples:
            value = sample["value"]
            label = sample.get("label", "")

            detected, result = detect(value, jwt_token, f"test_{test_id}")
            test_id += 1

            if detected:
                tp += 1
                print(f"  [OK] {info_type} - {label}: {value}")
            else:
                fn += 1
                print(f"  [FAIL] {info_type} - {label}: {value}")
                print(f"         响应: {result}")

            time.sleep(0.01)

    recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
    print(f"\n结果: TP={tp}, FN={fn}, 召回率={recall:.1f}%")

    if recall < 90:
        print("\n❌ 累积效应确认：400个请求后检测性能下降")
    else:
        print("\n✅ 无累积效应：400个请求后检测仍然正常")

if __name__ == "__main__":
    main()
