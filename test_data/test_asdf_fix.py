#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试ASDF框架修复后的效果
验证AI回复中的混淆格式敏感信息是否能被正确检测和脱敏
"""

import sys
import io
import requests
import json

# 设置Windows控制台编码为UTF-8
if sys.platform == 'win32':
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')

API_BASE = "http://localhost:8088"

def login(user_id="test_user"):
    """登录获取JWT Token"""
    response = requests.post(
        f"{API_BASE}/api/role/login",
        json={
            "user_id": user_id,
            "password": "health_assistant_2024",
            "workspace_id": "default",
            "role": "caregiver"
        }
    )
    if response.status_code == 200:
        data = response.json()
        if data.get("success"):
            return data["data"]["token"]
    print(f"❌ 登录失败: {response.text}")
    return None

def send_message(token, message, session_id="test_asdf_fix"):
    """发送消息"""
    response = requests.post(
        f"{API_BASE}/api/chat",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json"
        },
        json={
            "user_id": "test_user",
            "session_id": session_id,
            "message": message,
            "history": []
        }
    )
    return response

print("=" * 80)
print("ASDF框架修复验证测试")
print("=" * 80)

# 1. 登录
print("\n[步骤1] 登录获取Token...")
token = login()
if not token:
    print("❌ 测试终止：无法获取Token")
    exit(1)
print(f"✅ Token获取成功: {token[:20]}...")

# 2. 测试用例1：空格分隔的电话号码
print("\n" + "=" * 80)
print("[测试1] 空格分隔的电话号码")
print("=" * 80)
test_message_1 = "记录 小明 电话182 9134 2499"
print(f"📤 发送消息: {test_message_1}")

response = send_message(token, test_message_1)
if response.status_code == 200:
    data = response.json()
    if data.get("success"):
        ai_reply = data["data"]["message"]
        warnings = data["data"].get("security_warnings", [])

        print(f"\n🤖 AI回复:\n{ai_reply}")
        print(f"\n⚠️ 安全警告: {warnings}")

        # 检查AI回复中是否包含完整电话号码
        if "182-9134-2499" in ai_reply or "18291342499" in ai_reply or "182 9134 2499" in ai_reply:
            print("\n❌ 测试失败：AI回复中包含完整电话号码（未脱敏）")
        elif "182****2499" in ai_reply or "138****" in ai_reply or "****" in ai_reply:
            print("\n✅ 测试通过：AI回复中电话号码已脱敏")
        else:
            print(f"\n⚠️ 无法判断：AI回复中未找到电话号码相关内容")
    else:
        print(f"❌ API返回失败: {data.get('error')}")
else:
    print(f"❌ 请求失败: {response.status_code} - {response.text}")

# 3. 测试用例2：连字符分隔的电话号码
print("\n" + "=" * 80)
print("[测试2] 连字符分隔的电话号码")
print("=" * 80)
test_message_2 = "记录 小红 电话138-1234-5678"
print(f"📤 发送消息: {test_message_2}")

response = send_message(token, test_message_2)
if response.status_code == 200:
    data = response.json()
    if data.get("success"):
        ai_reply = data["data"]["message"]
        warnings = data["data"].get("security_warnings", [])

        print(f"\n🤖 AI回复:\n{ai_reply}")
        print(f"\n⚠️ 安全警告: {warnings}")

        # 检查AI回复中是否包含完整电话号码
        if "138-1234-5678" in ai_reply or "13812345678" in ai_reply:
            print("\n❌ 测试失败：AI回复中包含完整电话号码（未脱敏）")
        elif "138****5678" in ai_reply or "****" in ai_reply:
            print("\n✅ 测试通过：AI回复中电话号码已脱敏")
        else:
            print(f"\n⚠️ 无法判断：AI回复中未找到电话号码相关内容")
    else:
        print(f"❌ API返回失败: {data.get('error')}")
else:
    print(f"❌ 请求失败: {response.status_code} - {response.text}")

# 4. 测试用例3：中文数字混合的身份证号
print("\n" + "=" * 80)
print("[测试3] 中文数字混合的身份证号")
print("=" * 80)
test_message_3 = "记录 小李 身份证6101二52零0503153524"
print(f"📤 发送消息: {test_message_3}")

response = send_message(token, test_message_3)
if response.status_code == 200:
    data = response.json()
    if data.get("success"):
        ai_reply = data["data"]["message"]
        warnings = data["data"].get("security_warnings", [])

        print(f"\n🤖 AI回复:\n{ai_reply}")
        print(f"\n⚠️ 安全警告: {warnings}")

        # 检查AI回复中是否包含完整身份证号
        if "610125200503153524" in ai_reply or "6101二52零0503153524" in ai_reply:
            print("\n❌ 测试失败：AI回复中包含完整身份证号（未脱敏）")
        elif "610125" in ai_reply and "****" in ai_reply:
            print("\n✅ 测试通过：AI回复中身份证号已脱敏")
        else:
            print(f"\n⚠️ 无法判断：AI回复中未找到身份证号相关内容")
    else:
        print(f"❌ API返回失败: {data.get('error')}")
else:
    print(f"❌ 请求失败: {response.status_code} - {response.text}")

print("\n" + "=" * 80)
print("测试完成")
print("=" * 80)
