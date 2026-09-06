#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import re
import sys

# 从Go代码中提取的正则表达式
phone_pattern = r'\b1(?:3\d|4[5-9]|5[0-35-9]|6[2567]|7[0-8]|8\d|9[1389])\d{8}\b'
id_card_pattern = r'\b(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])\d{4}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b'

# 测试数据
test_phone = "23425925899"
test_id_card = "610123453324334213"

print("=" * 80)
print("Regex Test")
print("=" * 80)

# 测试电话号码
print(f"\nTest Phone: {test_phone}")
phone_match = re.search(phone_pattern, test_phone)
if phone_match:
    print(f"[OK] Match: {phone_match.group()}")
else:
    print("[FAIL] No match")
    # 分析原因
    if not test_phone.startswith('1'):
        print("   Reason: Does not start with 1")
    elif len(test_phone) != 11:
        print(f"   Reason: Length is not 11 (actual: {len(test_phone)})")
    else:
        print(f"   Reason: Invalid prefix (first 3 digits: {test_phone[:3]})")

# 测试身份证号
print(f"\nTest ID Card: {test_id_card}")
id_card_match = re.search(id_card_pattern, test_id_card)
if id_card_match:
    print(f"[OK] Match: {id_card_match.group()}")
else:
    print("[FAIL] No match")
    # 分析原因
    province_code = test_id_card[:2]
    year = test_id_card[6:10]
    month = test_id_card[10:12]
    day = test_id_card[12:14]

    print(f"   Province: {province_code}")
    print(f"   Year: {year}")
    print(f"   Month: {month}")
    print(f"   Day: {day}")
    print(f"   Length: {len(test_id_card)}")

    if len(test_id_card) != 18:
        print(f"   [X] Invalid length (should be 18)")
    if not re.match(r'(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])', province_code):
        print(f"   [X] Invalid province code")
    if not re.match(r'(?:19|20)\d{2}', year):
        print(f"   [X] Invalid year format")
    if not re.match(r'(?:0[1-9]|1[0-2])', month):
        print(f"   [X] Invalid month format")
    if not re.match(r'(?:0[1-9]|[12]\d|3[01])', day):
        print(f"   [X] Invalid day format")

print("\n" + "=" * 80)
