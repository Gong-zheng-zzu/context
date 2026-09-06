#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import re

# 系统使用的正则表达式
phone_pattern = r'\b1(?:3\d|4[5-9]|5[0-35-9]|6[2567]|7[0-8]|8\d|9[1389])\d{8}\b'
id_card_pattern = r'\b(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])\d{4}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b'

print("=" * 80)
print("测试用户输入的敏感信息格式")
print("=" * 80)

# 测试电话号码
test_phones = [
    "182-9134 24 99",      # 用户输入（带分隔符和空格）
    "182913424 99",        # 去掉连字符
    "18291342499",         # 标准格式
]

print("\n[电话号码测试]")
for phone in test_phones:
    match = re.search(phone_pattern, phone)
    print(f"  {phone:20s} -> {'[OK] 匹配' if match else '[X] 不匹配'}")

# 测试身份证号
test_ids = [
    "6101二52零0503153524",    # 用户输入（包含中文）
    "610152零0503153524",      # AI回复中的格式
    "610152050503153524",      # 标准格式（假设"二"=2，"零"=0）
]

print("\n[身份证号测试]")
for id_card in test_ids:
    match = re.search(id_card_pattern, id_card)
    print(f"  {id_card:25s} -> {'[OK] 匹配' if match else '[X] 不匹配'}")

print("\n" + "=" * 80)
print("结论：")
print("=" * 80)
print("1. 电话号码 '182-9134 24 99' 包含分隔符和空格，不匹配正则")
print("2. 身份证号 '6101二52零0503153524' 包含中文字符，不匹配正则")
print("3. 系统只检测标准格式的敏感信息（连续的数字）")
print("4. 这是一个已知的绕过方式！")
print("=" * 80)
