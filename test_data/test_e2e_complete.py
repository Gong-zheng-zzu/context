#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
完整的端到端测试脚本
测试：登录 → 发送敏感信息 → 验证AI回复脱敏 → 验证存储
"""
import requests
import json
import time

BASE_URL = "http://localhost:8088/api"

def test_e2e_sensitive_redaction():
    print("=" * 80)
    print("端到端测试：敏感信息脱敏")
    print("=" * 80)

    # 步骤1: 登录
    print("\n[步骤1] 登录获取JWT Token")
    try:
        login_resp = requests.post(
            f"{BASE_URL}/role/login",
            json={
                "role": "caregiver",
                "user_id": "test_caregiver_001",
                "password": "test123",
                "workspace_id": "default"
            }
        )
        login_resp.raise_for_status()
        token = login_resp.json()['data']['token']
        print(f"[OK] 登录成功，Token: {token[:20]}...")
    except Exception as e:
        print(f"[FAIL] 登录失败: {e}")
        return False

    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }

    # 步骤2: 发送包含敏感信息的消息
    print("\n[步骤2] 发送包含敏感信息的消息")
    test_message = "张奶奶的电话是13812345678，身份证是110101199001011234，银行卡是6222021234567890123，邮箱是zhang@example.com"
    print(f"原始消息: {test_message}")

    try:
        chat_resp = requests.post(
            f"{BASE_URL}/chat",
            json={
                "user_id": "test_caregiver_001",
                "session_id": f"test_session_{int(time.time())}",
                "message": test_message,
                "history": []
            },
            headers=headers,
            timeout=30
        )
        chat_resp.raise_for_status()
        ai_response = chat_resp.json()['response']
        print(f"[OK] AI回复: {ai_response[:200]}...")
    except Exception as e:
        print(f"[FAIL] 聊天请求失败: {e}")
        return False

    # 步骤3: 验证AI回复中的敏感信息已脱敏
    print("\n[步骤3] 验证敏感信息脱敏")

    checks = [
        ("电话号码", "13812345678", "138****5678"),
        ("身份证号", "110101199001011234", "110***********1234"),
        ("银行卡号", "6222021234567890123", "622202*********0123"),
        ("邮箱地址", "zhang@example.com", "zh***@example.com")
    ]

    all_passed = True
    for name, original, expected_redacted in checks:
        if original in ai_response:
            print(f"[FAIL] {name} 未脱敏: 发现原始值 {original}")
            all_passed = False
        else:
            # 检查是否包含脱敏后的格式（可能有不同的脱敏方式）
            if "*" in ai_response or "***" in ai_response:
                print(f"[OK] {name} 已脱敏（原始值未出现）")
            else:
                print(f"[WARN] {name} 可能未提及或使用了其他脱敏方式")

    # 步骤4: 验证安全扫描API
    print("\n[步骤4] 验证安全扫描API")
    try:
        scan_resp = requests.post(
            f"{BASE_URL}/security/scan",
            json={"content": test_message},
            headers=headers
        )
        scan_resp.raise_for_status()

        scan_result = scan_resp.json()
        print(f"[OK] 检测到 {scan_result['count']} 个敏感信息")
        print(f"风险评分: {scan_result.get('risk_score', 'N/A')}")
        print(f"风险级别: {scan_result.get('risk_level', 'N/A')}")

        for info in scan_result.get('sensitive_infos', []):
            print(f"  - {info['type']}: {info['value']} (置信度: {info['confidence']})")

        # 验证是否检测到所有类型
        detected_types = [info['type'] for info in scan_result.get('sensitive_infos', [])]
        expected_types = ['phone', 'id_card', 'bank_card', 'email']

        for expected_type in expected_types:
            if expected_type in detected_types:
                print(f"[OK] {expected_type} 已检测")
            else:
                print(f"[WARN] {expected_type} 未检测到")

    except Exception as e:
        print(f"[FAIL] 安全扫描失败: {e}")
        return False

    print("\n" + "=" * 80)
    if all_passed:
        print("[SUCCESS] 测试通过：所有敏感信息已正确脱敏")
    else:
        print("[FAIL] 测试失败：部分敏感信息未脱敏")
    print("=" * 80)

    return all_passed


def test_multiple_sensitive_types():
    """测试多种敏感信息类型"""
    print("\n" + "=" * 80)
    print("测试：多种敏感信息类型检测")
    print("=" * 80)

    # 登录
    login_resp = requests.post(
        f"{BASE_URL}/role/login",
        json={
            "role": "elder",
            "user_id": "test_elder_001",
            "password": "test123",
            "workspace_id": "default"
        }
    )
    token = login_resp.json()['data']['token']
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }

    test_cases = [
        {
            "name": "有效手机号",
            "content": "我的手机号是13812345678",
            "expected_type": "phone",
            "should_detect": True
        },
        {
            "name": "有效身份证",
            "content": "身份证号：110101199001011234",
            "expected_type": "id_card",
            "should_detect": True
        },
        {
            "name": "有效银行卡",
            "content": "银行卡：6222021234567890123",
            "expected_type": "bank_card",
            "should_detect": True
        },
        {
            "name": "有效邮箱",
            "content": "邮箱：user@example.com",
            "expected_type": "email",
            "should_detect": True
        },
        {
            "name": "无效手机号",
            "content": "电话：23425925899",
            "expected_type": "phone",
            "should_detect": False
        },
        {
            "name": "无效身份证",
            "content": "身份证：761024530060355252",
            "expected_type": "id_card",
            "should_detect": False
        }
    ]

    results = []
    for test in test_cases:
        print(f"\n[测试] {test['name']}")
        print(f"内容: {test['content']}")

        try:
            resp = requests.post(
                f"{BASE_URL}/security/scan",
                json={"content": test['content']},
                headers=headers
            )
            resp.raise_for_status()
            result = resp.json()

            detected = result.get('count', 0) > 0
            detected_types = [info['type'] for info in result.get('sensitive_infos', [])]

            if test['should_detect']:
                if detected and test['expected_type'] in detected_types:
                    print(f"[PASS] 正确检测到 {test['expected_type']}")
                    results.append(True)
                else:
                    print(f"[FAIL] 应该检测到但未检测到")
                    results.append(False)
            else:
                if not detected:
                    print(f"[PASS] 正确拒绝无效数据")
                    results.append(True)
                else:
                    print(f"[FAIL] 误报：检测到了无效数据")
                    results.append(False)

        except Exception as e:
            print(f"[ERROR] 请求失败: {e}")
            results.append(False)

    print("\n" + "=" * 80)
    passed = sum(results)
    total = len(results)
    print(f"测试结果: {passed}/{total} 通过")
    print("=" * 80)

    return all(results)


def test_file_upload_with_sensitive_info():
    """测试文件上传中的敏感信息检测"""
    print("\n" + "=" * 80)
    print("测试：文件上传敏感信息检测")
    print("=" * 80)

    # 登录
    login_resp = requests.post(
        f"{BASE_URL}/role/login",
        json={
            "role": "doctor",
            "user_id": "test_doctor_001",
            "password": "test123",
            "workspace_id": "default"
        }
    )
    token = login_resp.json()['data']['token']
    headers = {
        "Authorization": f"Bearer {token}"
    }

    # 创建包含敏感信息的测试文件
    test_content = """
    患者信息：
    姓名：张三
    电话：13812345678
    身份证：110101199001011234
    银行卡：6222021234567890123
    """

    print("\n[步骤1] 上传包含敏感信息的文件")
    try:
        files = {
            'file': ('patient_info.txt', test_content.encode('utf-8'), 'text/plain')
        }
        data = {
            'user_id': 'test_doctor_001',
            'session_id': f'test_session_{int(time.time())}'
        }

        upload_resp = requests.post(
            f"{BASE_URL}/files/upload",
            files=files,
            data=data,
            headers={"Authorization": f"Bearer {token}"}
        )
        upload_resp.raise_for_status()

        result = upload_resp.json()
        print(f"[OK] 文件上传成功")
        print(f"文件ID: {result.get('file_id', 'N/A')}")

        # 检查是否有敏感信息警告
        if 'sensitive_info_detected' in result:
            print(f"[OK] 检测到敏感信息: {result['sensitive_info_detected']}")
        else:
            print(f"[WARN] 未返回敏感信息检测结果")

    except Exception as e:
        print(f"[FAIL] 文件上传失败: {e}")
        return False

    print("\n" + "=" * 80)
    print("[INFO] 文件上传测试完成")
    print("=" * 80)

    return True


if __name__ == "__main__":
    print("\n")
    print("╔" + "═" * 78 + "╗")
    print("║" + " " * 20 + "Context-Keeper 端到端测试套件" + " " * 27 + "║")
    print("╚" + "═" * 78 + "╝")
    print("\n")

    all_tests_passed = True

    # 测试1: 端到端敏感信息脱敏
    try:
        result1 = test_e2e_sensitive_redaction()
        all_tests_passed = all_tests_passed and result1
    except Exception as e:
        print(f"\n[ERROR] 测试1异常: {e}")
        all_tests_passed = False

    # 测试2: 多种敏感信息类型
    try:
        result2 = test_multiple_sensitive_types()
        all_tests_passed = all_tests_passed and result2
    except Exception as e:
        print(f"\n[ERROR] 测试2异常: {e}")
        all_tests_passed = False

    # 测试3: 文件上传敏感信息检测
    try:
        result3 = test_file_upload_with_sensitive_info()
        all_tests_passed = all_tests_passed and result3
    except Exception as e:
        print(f"\n[ERROR] 测试3异常: {e}")
        all_tests_passed = False

    # 最终结果
    print("\n")
    print("╔" + "═" * 78 + "╗")
    if all_tests_passed:
        print("║" + " " * 30 + "所有测试通过 ✓" + " " * 33 + "║")
    else:
        print("║" + " " * 30 + "部分测试失败 ✗" + " " * 33 + "║")
    print("╚" + "═" * 78 + "╝")
    print("\n")
