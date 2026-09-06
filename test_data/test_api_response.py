#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
API响应调试 - 查看API实际返回了什么
"""

import json
import requests

API_BASE_URL = "http://localhost:8088"

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

def test_samples():
    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {jwt_token}"
    }

    # 测试几个样本
    test_cases = [
        ("phone", "13877213192"),
        ("phone", "18637857092"),
        ("bank_card", "6217005734308785"),
        ("email", "user8976@163.com"),
    ]

    for idx, (type_name, value) in enumerate(test_cases):
        print(f"\n{'='*80}")
        print(f"测试 {type_name}: {value}")
        print(f"{'='*80}")

        response = requests.post(
            f"{API_BASE_URL}/api/security/detect",
            json={
                "content": value,
                "user_id": "test_user",
                "session_id": f"api_debug_{idx}"
            },
            headers=headers,
            timeout=5
        )

        print(f"HTTP状态码: {response.status_code}")
        print(f"响应内容:")

        if response.status_code == 200:
            result = response.json()
            print(json.dumps(result, indent=2, ensure_ascii=False))
        else:
            print(response.text)

if __name__ == "__main__":
    test_samples()
