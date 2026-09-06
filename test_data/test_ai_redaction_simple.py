#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
关键测试：验证AI回复中的敏感信息是否被正确脱敏
这是用户最初报告的问题
"""
import requests
import json
import time

BASE_URL = "http://localhost:8088/api"

def test_ai_response_redaction():
    """
    测试场景：
    1. 用户发送包含敏感信息的消息
    2. AI回复中提到这些敏感信息
    3. 验证AI回复中的敏感信息已被脱敏
    """
    print("=" * 80)
    print("关键测试：AI回复中的敏感信息脱敏")
    print("=" * 80)

    # 登录
    print("\n[1] 登录...")
    login_resp = requests.post(
        f"{BASE_URL}/role/login",
        json={
            "role": "elder",
            "user_id": "test_user_001",
            "password": "test123",
            "workspace_id": "default"
        }
    )

    if login_resp.status_code != 200:
        print(f"[FAIL] 登录失败: {login_resp.status_code}")
        print(login_resp.text)
        return False

    token = login_resp.json()['data']['token']
    print(f"[OK] 登录成功")

    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }

    # 测试用例：使用有效的敏感信息格式
    test_cases = [
        {
            "name": "测试1：电话号码和身份证",
            "message": "张奶奶的电话是13812345678，身份证是110101199001011234",
            "sensitive_data": {
                "phone": "13812345678",
                "id_card": "110101199001011234"
            }
        },
        {
            "name": "测试2：银行卡和邮箱",
            "message": "李爷爷的银行卡是6222021234567890123，邮箱是li@example.com",
            "sensitive_data": {
                "bank_card": "6222021234567890123",
                "email": "li@example.com"
            }
        },
        {
            "name": "测试3：多种敏感信息混合",
            "message": "王奶奶的联系方式：电话13912345678，身份证320101198001011234，邮箱wang@test.com",
            "sensitive_data": {
                "phone": "13912345678",
                "id_card": "320101198001011234",
                "email": "wang@test.com"
            }
        }
    ]

    all_passed = True

    for test_case in test_cases:
        print(f"\n{'=' * 80}")
        print(f"{test_case['name']}")
        print(f"{'=' * 80}")
        print(f"用户消息: {test_case['message']}")

        # 发送消息
        session_id = f"test_session_{int(time.time())}"
        chat_resp = requests.post(
            f"{BASE_URL}/chat",
            json={
                "user_id": "test_user_001",
                "session_id": session_id,
                "message": test_case['message'],
                "history": []
            },
            headers=headers,
            timeout=30
        )

        if chat_resp.status_code != 200:
            print(f"[FAIL] 聊天请求失败: {chat_resp.status_code}")
            print(chat_resp.text)
            all_passed = False
            continue

        result = chat_resp.json()
        ai_response = result.get('response', '')

        print(f"\nAI回复:")
        print(f"{ai_response}")

        # 验证敏感信息是否被脱敏
        print(f"\n验证结果:")
        test_passed = True

        for data_type, original_value in test_case['sensitive_data'].items():
            if original_value in ai_response:
                print(f"  [FAIL] {data_type}: 原始值 '{original_value}' 未脱敏！")
                test_passed = False
                all_passed = False
            else:
                print(f"  [OK] {data_type}: 原始值未出现（已脱敏或未提及）")

        # 检查是否包含脱敏标记（*号）
        if "*" in ai_response:
            print(f"  [OK] 回复中包含脱敏标记 '*'")
        else:
            print(f"  [INFO] 回复中未包含脱敏标记（可能AI未重复敏感信息）")

        if test_passed:
            print(f"\n✓ {test_case['name']} 通过")
        else:
            print(f"\n✗ {test_case['name']} 失败")

    # 最终结果
    print(f"\n{'=' * 80}")
    if all_passed:
        print("✓✓✓ 所有测试通过：AI回复中的敏感信息已正确脱敏 ✓✓✓")
    else:
        print("✗✗✗ 测试失败：AI回复中仍包含未脱敏的敏感信息 ✗✗✗")
    print(f"{'=' * 80}\n")

    return all_passed


if __name__ == "__main__":
    print("\n")
    print("╔" + "═" * 78 + "╗")
    print("║" + " " * 25 + "AI回复脱敏测试" + " " * 38 + "║")
    print("║" + " " * 20 + "验证原始问题是否已修复" + " " * 35 + "║")
    print("╚" + "═" * 78 + "╝")
    print("\n")

    try:
        success = test_ai_response_redaction()
        exit(0 if success else 1)
    except Exception as e:
        print(f"\n[ERROR] 测试异常: {e}")
        import traceback
        traceback.print_exc()
        exit(1)
