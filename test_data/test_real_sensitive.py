#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import requests
import json

API_URL = "http://localhost:8088/api/security/scan"

# 使用真实格式的测试数据
test_cases = [
    {
        "name": "Valid Phone",
        "text": "My phone is 13812345678",
        "expected": "phone"
    },
    {
        "name": "Valid ID Card",
        "text": "ID: 110101199001011234",
        "expected": "id_card"
    },
    {
        "name": "Invalid Phone (from screenshot)",
        "text": "Phone: 23425925899",
        "expected": None
    },
    {
        "name": "Invalid ID Card (from screenshot)",
        "text": "ID: 610123453324334213",
        "expected": None
    }
]

print("=" * 80)
print("Testing Sensitive Info Detection")
print("=" * 80)

for test in test_cases:
    print(f"\n[Test] {test['name']}")
    print(f"Text: {test['text']}")

    response = requests.post(
        API_URL,
        json={"content": test['text']},
        headers={"Content-Type": "application/json"}
    )

    result = response.json()
    detected = result.get('count', 0) > 0

    if test['expected'] is None:
        # Should NOT detect
        if not detected:
            print(f"[PASS] Correctly rejected invalid data")
        else:
            print(f"[FAIL] False positive - detected: {result}")
    else:
        # Should detect
        if detected:
            types = [info['type'] for info in result.get('sensitive_infos', [])]
            if test['expected'] in types:
                print(f"[PASS] Correctly detected: {test['expected']}")
            else:
                print(f"[FAIL] Wrong type - expected {test['expected']}, got {types}")
        else:
            print(f"[FAIL] Not detected")

    print(f"API Response: count={result.get('count')}, types={[info['type'] for info in result.get('sensitive_infos', [])] if result.get('sensitive_infos') else []}")

print("\n" + "=" * 80)
