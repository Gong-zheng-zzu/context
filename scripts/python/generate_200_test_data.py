#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成200个样本的测试数据集
"""

import json
import random

def generate_id_cards(count=50):
    """生成身份证测试数据"""
    data = []

    # 标准身份证（70%）
    for i in range(int(count * 0.7)):
        province = random.choice(['110101', '320106', '440305', '510107', '610125'])
        year = random.randint(1960, 2005)
        month = random.randint(1, 12)
        day = random.randint(1, 28)
        suffix = random.randint(1000, 9999)
        id_card = f"{province}{year}{month:02d}{day:02d}{suffix}"
        data.append({
            "value": id_card,
            "label": f"标准身份证{i+1}",
            "should_detect": True
        })

    # 带空格的身份证（10%）
    for i in range(int(count * 0.1)):
        province = random.choice(['110101', '320106'])
        year = random.randint(1970, 2000)
        month = random.randint(1, 12)
        day = random.randint(1, 28)
        suffix = random.randint(1000, 9999)
        id_card = f"{province} {year}{month:02d}{day:02d} {suffix}"
        data.append({
            "value": id_card,
            "label": f"带空格身份证{i+1}",
            "should_detect": False
        })

    # 非身份证（20%）
    for i in range(int(count * 0.2)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(18)])
        data.append({
            "value": fake,
            "label": f"18位数字{i+1}",
            "should_detect": False
        })

    return data

def generate_phones(count=50):
    """生成手机号测试数据"""
    data = []
    prefixes = ['130', '131', '138', '150', '156', '170', '177', '180', '186', '189']

    # 标准手机号（80%）
    for i in range(int(count * 0.8)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(8)])
        phone = f"{prefix}{suffix}"
        data.append({
            "value": phone,
            "label": f"标准手机号{i+1}",
            "should_detect": True
        })

    # 带分隔符（10%）
    for i in range(int(count * 0.1)):
        prefix = random.choice(prefixes)
        mid = ''.join([str(random.randint(0, 9)) for _ in range(4)])
        end = ''.join([str(random.randint(0, 9)) for _ in range(4)])
        phone = f"{prefix}-{mid}-{end}"
        data.append({
            "value": phone,
            "label": f"带分隔符手机号{i+1}",
            "should_detect": False
        })

    # 非手机号（10%）
    for i in range(int(count * 0.1)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(random.randint(8, 12))])
        data.append({
            "value": fake,
            "label": f"随机数字{i+1}",
            "should_detect": False
        })

    return data

def generate_bank_cards(count=50):
    """生成银行卡测试数据"""
    data = []
    prefixes = ['622202', '622588', '621700', '955880']

    # 标准银行卡（80%）
    for i in range(int(count * 0.8)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(random.choice([10, 13]))])
        card = f"{prefix}{suffix}"
        data.append({
            "value": card,
            "label": f"标准银行卡{i+1}",
            "should_detect": True
        })

    # 带空格（10%）
    for i in range(int(count * 0.1)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(10)])
        card = f"{prefix[:4]} {prefix[4:]} {suffix[:4]} {suffix[4:]}"
        data.append({
            "value": card,
            "label": f"带空格银行卡{i+1}",
            "should_detect": False
        })

    # 非银行卡（10%）
    for i in range(int(count * 0.1)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(16)])
        data.append({
            "value": fake,
            "label": f"16位数字{i+1}",
            "should_detect": False
        })

    return data

def generate_emails(count=50):
    """生成邮箱测试数据"""
    data = []
    domains = ['gmail.com', 'qq.com', '163.com', 'outlook.com', 'example.com']

    # 标准邮箱（80%）
    for i in range(int(count * 0.8)):
        username = f"user{random.randint(100, 9999)}"
        domain = random.choice(domains)
        email = f"{username}@{domain}"
        data.append({
            "value": email,
            "label": f"标准邮箱{i+1}",
            "should_detect": True
        })

    # 带空格（10%）
    for i in range(int(count * 0.1)):
        username = f"user{random.randint(100, 999)}"
        domain = random.choice(domains)
        email = f"{username} @ {domain}"
        data.append({
            "value": email,
            "label": f"带空格邮箱{i+1}",
            "should_detect": False
        })

    # 非邮箱（10%）
    for i in range(int(count * 0.1)):
        text = random.choice(['今天天气不错', '请帮我查询', '谢谢'])
        data.append({
            "value": text,
            "label": f"普通文本{i+1}",
            "should_detect": False
        })

    return data

def main():
    print("生成200个测试样本...")

    test_data = {
        "id_card": generate_id_cards(50),
        "phone": generate_phones(50),
        "bank_card": generate_bank_cards(50),
        "email": generate_emails(50)
    }

    # 统计
    total = 0
    should_detect_count = 0
    for category, items in test_data.items():
        count = len(items)
        detect_count = sum(1 for item in items if item['should_detect'])
        total += count
        should_detect_count += detect_count
        print(f"  {category}: {count}个样本 (应检测: {detect_count}, 不应检测: {count - detect_count})")

    print(f"\n总计: {total}个样本")
    print(f"  应检测: {should_detect_count} ({should_detect_count/total*100:.1f}%)")
    print(f"  不应检测: {total - should_detect_count} ({(total - should_detect_count)/total*100:.1f}%)")

    # 保存
    output_file = "test_data/sensitive_data_200.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(test_data, f, ensure_ascii=False, indent=2)

    print(f"\n测试数据已保存到: {output_file}")

if __name__ == "__main__":
    main()
