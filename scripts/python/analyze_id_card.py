#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import re

# 当前系统使用的身份证正则（从 sensitive_detector.go 第113行）
id_card_pattern = r'\b(?:1[1-5]|2[1-3]|3[1-7]|4[1-6]|5[0-4]|6[1-5])\d{4}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b'

test_id = '761024530060355252'

print('=' * 80)
print('身份证号格式分析')
print('=' * 80)
print(f'\n测试号码: {test_id}')
print(f'长度: {len(test_id)} 位')

# 分解号码
province = test_id[:2]
area = test_id[2:6]
year = test_id[6:10]
month = test_id[10:12]
day = test_id[12:14]
sequence = test_id[14:17]
checksum = test_id[17]

print(f'\n号码结构:')
print(f'  省份代码: {province}')
print(f'  地区代码: {area}')
print(f'  出生年份: {year}')
print(f'  出生月份: {month}')
print(f'  出生日期: {day}')
print(f'  顺序码: {sequence}')
print(f'  校验码: {checksum}')

print(f'\n格式验证:')
# 检查省份代码
valid_provinces = list(range(11, 16)) + list(range(21, 24)) + list(range(31, 38)) + list(range(41, 47)) + list(range(51, 55)) + list(range(61, 66))
province_valid = int(province) in valid_provinces
print(f'  省份代码 {province}: {"[OK] 有效" if province_valid else "[X] 无效（标准范围: 11-15, 21-23, 31-37, 41-46, 51-54, 61-65）"}')

# 检查年份
year_valid = year.startswith('19') or year.startswith('20')
print(f'  年份 {year}: {"[OK] 有效" if year_valid else "[X] 无效（标准范围: 1900-2099）"}')

# 检查月份
month_valid = 1 <= int(month) <= 12
print(f'  月份 {month}: {"[OK] 有效" if month_valid else "[X] 无效"}')

# 检查日期
day_valid = 1 <= int(day) <= 31
print(f'  日期 {day}: {"[OK] 有效" if day_valid else "[X] 无效"}')

# 正则匹配测试
match = re.search(id_card_pattern, test_id)
print(f'\n正则匹配结果: {"[OK] 匹配" if match else "[X] 不匹配"}')

if not match:
    print(f'\n结论: 此身份证号不符合中国身份证号标准格式，因此未被检测到')
    print(f'主要问题: 省份代码 {province} 和年份 {year} 都不在有效范围内')

print('\n' + '=' * 80)
print('测试有效的身份证号')
print('=' * 80)

valid_test_id = '110101199001011234'
print(f'\n测试号码: {valid_test_id}')
match_valid = re.search(id_card_pattern, valid_test_id)
print(f'正则匹配结果: {"[OK] 匹配" if match_valid else "[X] 不匹配"}')

if match_valid:
    print(f'匹配内容: {match_valid.group()}')
    province_v = valid_test_id[:2]
    year_v = valid_test_id[6:10]
    print(f'省份代码: {province_v} (北京)')
    print(f'出生年份: {year_v}')
