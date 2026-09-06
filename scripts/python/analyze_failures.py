#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
分析检测失败的样本
"""

import json
import requests
import time

API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

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
    try:
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
            return detected, confidence, result
        else:
            return False, 0, {"error": f"HTTP {response.status_code}"}
    except Exception as e:
        return False, 0, {"error": str(e)}

def main():
    print("="*80)
    print("分析检测失败的样本")
    print("="*80)

    jwt_token = login()
    if not jwt_token:
        print("登录失败")
        return

    with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
        test_data = json.load(f)

    failures = {"FN": [], "FP": []}
    sample_id = 0

    for info_type in ["id_card", "phone", "bank_card", "email"]:
        samples = test_data.get(info_type, [])

        for sample in samples:
            value = sample["value"]
            should_detect = sample["should_detect"]
            label = sample.get("label", "")

            detected, confidence, result = detect(value, jwt_token, f"analyze_{sample_id}")
            sample_id += 1

            # 记录失败样本
            if should_detect and not detected:
                failures["FN"].append({
                    "type": info_type,
                    "label": label,
                    "value": value,
                    "confidence": confidence,
                    "result": result
                })
            elif not should_detect and detected:
                failures["FP"].append({
                    "type": info_type,
                    "label": label,
                    "value": value,
                    "confidence": confidence,
                    "result": result
                })

            time.sleep(0.2)

    # 打印分析结果
    print(f"\n假阴性 (FN): {len(failures['FN'])} 个")
    print("="*80)
    for i, f in enumerate(failures["FN"], 1):
        print(f"{i}. [{f['type']}] {f['label']}: {f['value']}")
        if 'sensitive_infos' in f['result']:
            print(f"   检测结果: {f['result']['sensitive_infos']}")
        print()

    print(f"\n假阳性 (FP): {len(failures['FP'])} 个")
    print("="*80)
    for i, f in enumerate(failures["FP"], 1):
        print(f"{i}. [{f['type']}] {f['label']}: {f['value']}")
        if 'sensitive_infos' in f['result']:
            print(f"   检测结果: {f['result']['sensitive_infos']}")
        print()

    # 统计
    total_positive = sum(len([s for s in test_data[t] if s["should_detect"]]) for t in ["id_card", "phone", "bank_card", "email"])
    total_negative = sum(len([s for s in test_data[t] if not s["should_detect"]]) for t in ["id_card", "phone", "bank_card", "email"])

    print("\n" + "="*80)
    print("统计")
    print("="*80)
    print(f"总正样本: {total_positive}, 漏检: {len(failures['FN'])}, 召回率: {(total_positive - len(failures['FN'])) / total_positive * 100:.1f}%")
    print(f"总负样本: {total_negative}, 误检: {len(failures['FP'])}, 特异性: {(total_negative - len(failures['FP'])) / total_negative * 100:.1f}%")
    print(f"总准确率: {(total_positive + total_negative - len(failures['FN']) - len(failures['FP'])) / (total_positive + total_negative) * 100:.1f}%")

if __name__ == "__main__":
    main()
