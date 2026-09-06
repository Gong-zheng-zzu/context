#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import requests
import json

BASE_URL = "http://localhost:8088/api"

# Login
login_response = requests.post(
    f"{BASE_URL}/role/login",
    json={
        "role": "elder",
        "user_id": "test_user_001",
        "password": "test123",
        "workspace_id": "default"
    }
)

token = login_response.json()['data']['token']
headers = {
    "Authorization": f"Bearer {token}",
    "Content-Type": "application/json"
}

print("=" * 80)
print("Final Verification: Sensitive Info Detection")
print("=" * 80)

# Test with real sensitive data
test_text = "Zhang Nainai's phone is 13812345678, ID card is 110101199001011234"
print(f"\nTest Text: {test_text}")

response = requests.post(
    f"{BASE_URL}/security/scan",
    json={"content": test_text},
    headers=headers
)

result = response.json()
print(f"\nFull API Response:")
print(json.dumps(result, indent=2, ensure_ascii=False))

sensitive_infos = result.get('sensitive_infos', [])
if sensitive_infos:
    print(f"\n[SUCCESS] Detected {len(sensitive_infos)} sensitive items:")
    for info in sensitive_infos:
        print(f"  - Type: {info['type']}, Value: {info['value']}, Confidence: {info['confidence']}")
else:
    print("\n[FAIL] No sensitive info detected")

print("\n" + "=" * 80)
print("Conclusion:")
print("=" * 80)
if sensitive_infos and any(info['type'] == 'phone' for info in sensitive_infos) and any(info['type'] == 'id_card' for info in sensitive_infos):
    print("[OK] Both phone and id_card detected successfully!")
    print("[OK] The fix is working!")
else:
    print("[FAIL] Detection incomplete")
