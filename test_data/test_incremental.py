#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
增量测试 - 每10个样本测试一次，找出在哪个点开始失效
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
        if result and "sensitive_infos" in result:
            return len(result.get("sensitive_infos", [])) > 0
        else:
            print(f"      [WARNING] API返回异常: {result}")
            return False
    else:
        print(f"      [ERROR] HTTP {response.status_code}")
        return False

def main():
    print("="*80)
    print("增量测试 - 找出失效点")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    # 收集所有应该被检测的样本
    all_positive_samples = []
    for info_type in ["id_card", "phone", "bank_card", "email"]:
        samples = test_data.get(info_type, [])
        for sample in samples:
            if sample["should_detect"]:
                all_positive_samples.append({
                    "type": info_type,
                    "value": sample["value"],
                    "label": sample.get("label", "")
                })

    print(f"\n总共{len(all_positive_samples)}个正样本")
    print("-"*80)

    # 每10个样本测试一次标记样本
    marker_phone = "13877213192"  # 已知能检测到的phone

    for batch_num in range(0, len(all_positive_samples), 10):
        # 测试这批10个样本
        batch_samples = all_positive_samples[batch_num:batch_num+10]
        detected_count = 0

        for idx, sample in enumerate(batch_samples):
            detected = detect(sample["value"], jwt_token, f"batch_{batch_num}_{idx}")
            if detected:
                detected_count += 1
            time.sleep(0.01)

        # 测试标记样本
        marker_detected = detect(marker_phone, jwt_token, f"marker_after_{batch_num}")

        print(f"批次 {batch_num:3d}-{batch_num+len(batch_samples):3d}: "
              f"检测到 {detected_count}/{len(batch_samples)}, "
              f"标记样本: {'OK' if marker_detected else 'FAIL'}")

        if not marker_detected:
            print(f"\n!!! 在第{batch_num+len(batch_samples)}个样本后，标记样本检测失效 !!!")
            break

if __name__ == "__main__":
    main()
