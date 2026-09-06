#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
批量后重测 - 测试200个样本后，再测试一次看是否失效
"""

import json
import requests
import time

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

def detect(content, jwt_token, session_id):
    """检测"""
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
                "session_id": session_id
            },
            headers=headers,
            timeout=5
        )

        if response.status_code == 200:
            result = response.json()
            return len(result.get("sensitive_infos", [])) > 0
        else:
            return False
    except Exception as e:
        return False

def main():
    print("="*80)
    print("批量后重测 - 测试200个样本后再测试")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    # 加载测试数据
    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    # 第1轮：测试所有200个样本
    print("\n第1轮：测试所有200个样本...")
    sample_id = 0
    detected_count = 0
    total_count = 0

    for info_type in ["id_card", "phone", "bank_card", "email"]:
        samples = test_data.get(info_type, [])
        for sample in samples:
            value = sample["value"]
            should_detect = sample["should_detect"]

            detected = detect(value, jwt_token, f"batch_test_{sample_id}")
            sample_id += 1
            total_count += 1

            if should_detect and detected:
                detected_count += 1

            time.sleep(0.01)

    print(f"第1轮完成：检测到 {detected_count}/{total_count} 个样本")

    # 第2轮：重新测试前10个phone样本
    print("\n第2轮：重新测试前10个phone样本...")
    phone_samples = test_data.get("phone", [])
    positive_samples = [s for s in phone_samples if s["should_detect"]][:10]

    detected_count_2 = 0
    for idx, sample in enumerate(positive_samples):
        value = sample["value"]
        detected = detect(value, jwt_token, f"retest_{idx}")

        if detected:
            detected_count_2 += 1
            print(f"  [OK] {sample['label']}: {value}")
        else:
            print(f"  [FAIL] {sample['label']}: {value}")

        time.sleep(0.01)

    print(f"\n第2轮完成：检测到 {detected_count_2}/10 个样本")

    if detected_count_2 == 0:
        print("\n问题复现！第2轮检测完全失效！")
    elif detected_count_2 < 10:
        print(f"\n问题部分复现！第2轮检测率下降到 {detected_count_2*10}%")
    else:
        print("\n问题未复现，第2轮检测正常")

if __name__ == "__main__":
    main()
