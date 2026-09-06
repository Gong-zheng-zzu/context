#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
快速验证脱敏配置
"""

import requests

API_BASE_URL = "http://localhost:8088"

def login():
    response = requests.post(
        f"{API_BASE_URL}/api/role/login",
        json={
            "user_id": "test_user",
            "password": "health_assistant_2024",
            "role": "caregiver",
            "workspace_id": "default"
        }
    )
    if response.status_code == 200:
        data = response.json()
        return data["data"]["token"]
    return None

def test_detection():
    token = login()
    if not token:
        print("❌ 登录失败")
        return

    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}"
    }

    # 测试内容：包含身份证和电话
    test_content = "张奶奶的电话是23425925899，身份证号是610123453324334213"

    print("="*80)
    print("测试敏感信息检测和脱敏")
    print("="*80)
    print(f"\n原始内容: {test_content}")

    response = requests.post(
        f"{API_BASE_URL}/api/security/detect",
        json={
            "content": test_content,
            "user_id": "test_user",
            "session_id": "quick_test"
        },
        headers=headers,
        timeout=5
    )

    if response.status_code == 200:
        result = response.json()
        print(f"\nAPI响应: {result}")
        sensitive_infos = result.get("sensitive_infos", [])
        if sensitive_infos is None:
            sensitive_infos = []

        print(f"\n检测结果: {len(sensitive_infos)}个敏感信息")
        for info in sensitive_infos:
            print(f"  - 类型: {info['type']}, 值: {info['value']}, 置信度: {info['confidence']}")

        # 检查是否检测到身份证和电话
        types = [info['type'] for info in sensitive_infos]
        has_id_card = 'id_card' in types
        has_phone = 'phone' in types

        print(f"\n检测到身份证: {has_id_card}")
        print(f"检测到电话: {has_phone}")

        if has_id_card and has_phone:
            print("\n✅ 成功: 身份证和电话都被检测到")
            print("✅ 修复生效: 安全策略配置已更新")
        else:
            print("\n❌ 失败: 部分敏感信息未检测到")
    else:
        print(f"\n❌ API调用失败: {response.status_code}")
        print(f"响应: {response.text}")

if __name__ == "__main__":
    test_detection()
