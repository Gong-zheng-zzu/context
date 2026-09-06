#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试缓存问题 - 同一个样本测试两次，看结果是否一致
"""

import requests
import time

API_BASE_URL = "http://localhost:8088"

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
        detected = len(result.get("sensitive_infos", [])) > 0
        confidence = result.get("avg_confidence", 0)
        return detected, confidence
    else:
        return False, 0

def main():
    print("="*80)
    print("测试缓存问题 - 同一样本测试3次")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    test_phone = "13877213192"

    print(f"\n测试样本: {test_phone}")
    print("-"*80)

    # 测试3次，使用相同的session_id（会命中缓存）
    for i in range(3):
        detected, confidence = detect(test_phone, jwt_token, "same_session")
        print(f"第{i+1}次测试: 检测到={detected}, 置信度={confidence:.2f}")
        time.sleep(0.5)

    print("\n" + "="*80)
    print("测试3次，使用不同的session_id（不会命中缓存）")
    print("="*80)

    # 测试3次，使用不同的session_id
    for i in range(3):
        detected, confidence = detect(test_phone, jwt_token, f"different_session_{i}")
        print(f"第{i+1}次测试: 检测到={detected}, 置信度={confidence:.2f}")
        time.sleep(0.5)

if __name__ == "__main__":
    main()
