#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
诊断AI回复脱敏问题
"""

import requests
import json

API_BASE_URL = "http://localhost:8088"

def login():
    """登录获取Token"""
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

def test_redaction_issue():
    """复现截图中的问题"""
    token = login()
    if not token:
        print("❌ 登录失败")
        return

    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}"
    }

    print("="*80)
    print("诊断AI回复脱敏问题")
    print("="*80)

    # 复现截图中的场景
    message = "张奶奶的身高是159，体重是50，电话为23425925899，身份证号为610123453324334213帮我存起来"

    print(f"\n用户输入: {message}")
    print("\n发送请求...")

    response = requests.post(
        f"{API_BASE_URL}/api/chat",
        json={
            "user_id": "test_user",
            "session_id": "diagnosis_test",
            "message": message
        },
        headers=headers,
        timeout=30
    )

    print(f"HTTP状态码: {response.status_code}")

    if response.status_code == 200:
        data = response.json()

        print("\n" + "="*80)
        print("响应数据分析")
        print("="*80)

        if data.get("success"):
            chat_data = data["data"]

            # 1. AI回复内容
            ai_message = chat_data.get("message", "")
            print(f"\n【AI回复】\n{ai_message}")

            # 2. 检测到的敏感信息
            sensitive_infos = chat_data.get("sensitive_infos", [])
            print(f"\n【检测到的敏感信息】({len(sensitive_infos)}个)")
            for info in sensitive_infos:
                print(f"  - 类型: {info['type']}, 值: {info['value']}, 置信度: {info['confidence']}")

            # 3. 是否过滤
            is_filtered = chat_data.get("filtered", False)
            print(f"\n【是否过滤】{is_filtered}")

            # 4. 安全警告
            security_warnings = chat_data.get("security_warnings", [])
            print(f"\n【安全警告】({len(security_warnings)}个)")
            for warning in security_warnings:
                print(f"  - {warning}")

            # 5. 检查AI回复中是否包含原始敏感信息
            print("\n" + "="*80)
            print("敏感信息泄露检查")
            print("="*80)

            contains_phone = "23425925899" in ai_message
            contains_id = "610123453324334213" in ai_message
            contains_masked = "***" in ai_message or "****" in ai_message

            print(f"\n包含原始电话号码: {contains_phone}")
            print(f"包含原始身份证号: {contains_id}")
            print(f"包含脱敏标记(***): {contains_masked}")

            if contains_phone or contains_id:
                print("\n❌ 问题确认: AI回复包含原始敏感信息，脱敏失败！")
                print("\n可能原因:")
                print("1. SecurityService未正确初始化")
                print("2. AI回复脱敏代码未执行")
                print("3. 脱敏逻辑有bug")

                # 建议
                print("\n建议检查:")
                print("1. 查看服务器日志: docker logs context-keeper-context-keeper-1 | grep 'AI安全'")
                print("2. 检查filtered字段是否为true")
                print("3. 检查security_warnings是否有警告")
            elif contains_masked:
                print("\n✅ 正常: AI回复已脱敏")
            else:
                print("\n⚠️  AI回复不包含敏感信息（可能拒绝回答或换了表述）")

        else:
            print(f"\n❌ API返回失败: {data.get('error')}")
    else:
        print(f"\n❌ HTTP错误")
        print(f"响应: {response.text}")

    # 额外测试：直接测试敏感信息检测API
    print("\n" + "="*80)
    print("测试敏感信息检测API")
    print("="*80)

    test_content = "电话号码：23425925899，身份证号：610123453324334213"
    print(f"\n测试内容: {test_content}")

    response = requests.post(
        f"{API_BASE_URL}/api/security/detect",
        json={
            "content": test_content,
            "user_id": "test_user",
            "session_id": "diagnosis_test"
        },
        headers=headers,
        timeout=5
    )

    if response.status_code == 200:
        result = response.json()
        sensitive_infos = result.get("sensitive_infos", [])
        print(f"\n检测结果: {len(sensitive_infos)}个敏感信息")
        for info in sensitive_infos:
            print(f"  - {info['type']}: {info['value']}")

        if len(sensitive_infos) > 0:
            print("\n✅ 敏感信息检测功能正常")
        else:
            print("\n❌ 敏感信息检测功能异常")
    else:
        print(f"\n❌ 检测API调用失败: {response.status_code}")

if __name__ == "__main__":
    test_redaction_issue()
