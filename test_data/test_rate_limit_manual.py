#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
速率限制手动测试脚本
"""

import requests
import time
from datetime import datetime

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

def test_rate_limit(token, num_requests=50, interval=0.1):
    """
    测试速率限制

    Args:
        token: JWT Token
        num_requests: 发送请求数量
        interval: 请求间隔（秒）
    """
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}"
    }

    success_count = 0
    rate_limited_count = 0
    error_count = 0

    print(f"开始测试: 发送{num_requests}个请求，间隔{interval}秒")
    print(f"预期速率: {60/interval:.0f} req/min")
    print(f"当前限制: 300 req/min")
    print("="*80)

    start_time = time.time()

    for i in range(1, num_requests + 1):
        try:
            response = requests.post(
                f"{API_BASE_URL}/api/chat",
                json={
                    "user_id": "test_user",
                    "session_id": f"rate_test_{i}",
                    "message": f"测试消息{i}"
                },
                headers=headers,
                timeout=5
            )

            # 检查响应头中的速率限制信息
            rate_limit = response.headers.get("X-RateLimit-Limit", "N/A")
            rate_remaining = response.headers.get("X-RateLimit-Remaining", "N/A")
            rate_reset = response.headers.get("X-RateLimit-Reset", "N/A")

            if response.status_code == 200:
                success_count += 1
                print(f"[PASS] 请求{i:3d}: 成功 | 剩余: {rate_remaining}/{rate_limit}")
            elif response.status_code == 429:
                rate_limited_count += 1
                print(f"[LIMIT] 请求{i:3d}: 速率限制 (429) | 重置时间: {rate_reset}")
            else:
                error_count += 1
                print(f"[ERROR] 请求{i:3d}: 错误 ({response.status_code})")

        except Exception as e:
            error_count += 1
            print(f"[ERROR] 请求{i:3d}: 异常 - {e}")

        time.sleep(interval)

    end_time = time.time()
    duration = end_time - start_time
    actual_rate = num_requests / (duration / 60)

    print("="*80)
    print(f"测试完成:")
    print(f"  总请求数: {num_requests}")
    print(f"  成功: {success_count} ({success_count/num_requests*100:.1f}%)")
    print(f"  被限流: {rate_limited_count} ({rate_limited_count/num_requests*100:.1f}%)")
    print(f"  错误: {error_count} ({error_count/num_requests*100:.1f}%)")
    print(f"  实际速率: {actual_rate:.0f} req/min")
    print(f"  测试耗时: {duration:.1f}秒")

if __name__ == "__main__":
    print("Context-Keeper 速率限制测试")
    print("="*80)

    # 登录
    print("正在登录...")
    token = login()
    if not token:
        print("登录失败")
        exit(1)
    print(f"登录成功，Token: {token[:20]}...")
    print()

    # 测试场景1: 正常使用（不会触发限制）
    print("\n【场景1】正常使用 - 10 req/min")
    test_rate_limit(token, num_requests=10, interval=6.0)

    # 等待一段时间
    print("\n等待60秒，让速率限制重置...")
    time.sleep(60)

    # 测试场景2: 高频使用（可能触发限制）
    print("\n【场景2】高频使用 - 600 req/min")
    test_rate_limit(token, num_requests=50, interval=0.1)

    # 等待一段时间
    print("\n等待60秒，让速率限制重置...")
    time.sleep(60)

    # 测试场景3: 极限测试（必定触发限制）
    print("\n【场景3】极限测试 - 3000 req/min")
    test_rate_limit(token, num_requests=100, interval=0.02)
