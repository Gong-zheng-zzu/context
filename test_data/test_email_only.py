#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Email专项测试 - 找出为什么email第1轮就是0%召回率
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

def detect(content, jwt_token, sample_id):
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
                "session_id": f"email_test_{sample_id}"
            },
            headers=headers,
            timeout=5
        )

        if response.status_code == 200:
            result = response.json()
            infos = result.get("sensitive_infos", [])
            return len(infos) > 0, infos
        else:
            print(f"  HTTP {response.status_code}: {response.text[:100]}")
            return False, []
    except Exception as e:
        print(f"  检测异常: {e}")
        return False, []

def main():
    print("="*80)
    print("Email专项测试")
    print("="*80)

    # 登录
    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    # 加载测试数据
    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    email_samples = test_data.get("email", [])

    # 测试前10个should_detect=true的样本
    positive_samples = [s for s in email_samples if s["should_detect"]][:10]

    print(f"\n测试 {len(positive_samples)} 个email样本:\n")

    detected_count = 0
    for idx, sample in enumerate(positive_samples):
        value = sample["value"]
        label = sample["label"]

        detected, infos = detect(value, jwt_token, idx)

        if detected:
            detected_count += 1
            print(f"[OK] {label}: {value}")
            for info in infos:
                print(f"     类型={info['type']}, 置信度={info['confidence']:.3f}")
        else:
            print(f"[FAIL] {label}: {value} - 未检测到")

    print(f"\n{'='*80}")
    print(f"检测率: {detected_count}/{len(positive_samples)} = {detected_count/len(positive_samples)*100:.1f}%")
    print(f"{'='*80}")

if __name__ == "__main__":
    main()
