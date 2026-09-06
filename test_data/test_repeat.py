#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
简单重复测试 - 测试同一个样本2次，看看第2次是否失效
"""

import json
import requests
import time

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
            return result.get("sensitive_infos", [])
        else:
            print(f"  HTTP {response.status_code}")
            return []
    except Exception as e:
        print(f"  异常: {e}")
        return []

def main():
    print("="*80)
    print("简单重复测试 - 测试同一个样本多次")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    test_value = "13877213192"  # 一个手机号

    print(f"\n测试样本: {test_value}")
    print(f"{'='*80}\n")

    # 测试5次
    for i in range(1, 6):
        print(f"第{i}次测试:")
        infos = detect(test_value, jwt_token, f"repeat_test_{i}")

        if infos:
            print(f"  检测到 {len(infos)} 个敏感信息")
            for info in infos:
                print(f"    - 类型: {info['type']}, 置信度: {info['confidence']:.3f}")
        else:
            print(f"  未检测到")

        time.sleep(0.5)

if __name__ == "__main__":
    main()
