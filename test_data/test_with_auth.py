#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import requests
import json

BASE_URL = "http://localhost:8088/api"

# Step 1: Login to get JWT token
print("=" * 80)
print("Step 1: Login to get JWT token")
print("=" * 80)

login_response = requests.post(
    f"{BASE_URL}/role/login",
    json={
        "role": "elder",
        "user_id": "test_user_001",
        "password": "test123",
        "workspace_id": "default"
    }
)

if login_response.status_code != 200:
    print(f"[FAIL] Login failed: {login_response.status_code}")
    print(login_response.text)
    exit(1)

login_data = login_response.json()
print(f"Login response: {json.dumps(login_data, indent=2)}")

# Try different token paths
token = None
if 'token' in login_data:
    token = login_data['token']
elif 'data' in login_data and login_data['data'] and 'token' in login_data['data']:
    token = login_data['data']['token']

if not token:
    print(f"[FAIL] No token in response")
    exit(1)

print(f"[OK] Login successful, got token: {token[:20]}...")

# Step 2: Test sensitive info detection with auth
print("\n" + "=" * 80)
print("Step 2: Test Sensitive Info Detection")
print("=" * 80)

headers = {
    "Authorization": f"Bearer {token}",
    "Content-Type": "application/json"
}

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

for test in test_cases:
    print(f"\n[Test] {test['name']}")
    print(f"Text: {test['text']}")

    response = requests.post(
        f"{BASE_URL}/security/scan",
        json={"content": test['text']},
        headers=headers
    )

    if response.status_code != 200:
        print(f"[ERROR] API returned {response.status_code}: {response.text}")
        continue

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
