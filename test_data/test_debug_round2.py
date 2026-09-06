#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
调试脚本：查看第2轮测试为什么失败
"""

import json
import requests

API_BASE_URL = "http://localhost:8088"

# 登录
login_resp = requests.post(
    f"{API_BASE_URL}/api/role/login",
    json={
        "user_id": "test_user",
        "password": "health_assistant_2024",
        "role": "caregiver",
        "workspace_id": "default"
    }
)
token = login_resp.json()["data"]["token"]

# 加载测试数据
with open('test_data/sensitive_data_200.json', 'r', encoding='utf-8') as f:
    test_data = json.load(f)

# 测试前10个身份证样本
print("=== 测试前10个身份证样本 ===\n")
for i, sample in enumerate(test_data['id_card'][:10]):
    value = sample["value"]
    should_detect = sample["should_detect"]

    resp = requests.post(
        f"{API_BASE_URL}/api/security/detect",
        headers={"Authorization": f"Bearer {token}"},
        json={
            "content": value,
            "user_id": "test_user",
            "session_id": f"debug_{i}"
        }
    )

    result = resp.json()
    detected = len(result.get("sensitive_infos", [])) > 0
    confidence = result.get("avg_confidence", 0)

    status = "OK" if (detected == should_detect) else "FAIL"
    print(f"{status} 样本{i+1}: 值={value} 应检测={should_detect} 实际={detected} 置信度={confidence:.2f}")

    if detected != should_detect:
        print(f"   详情: {result}")
