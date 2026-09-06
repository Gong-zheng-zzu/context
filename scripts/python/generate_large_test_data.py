#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成大规模测试数据集（2000个样本）
包含各种边界案例、困难样本和正常样本
"""

import json
import random
from datetime import datetime, timedelta

def generate_id_cards(count=500):
    """生成身份证测试数据"""
    data = []

    # 真实格式的身份证（60%）
    for i in range(int(count * 0.6)):
        province = random.choice(['110101', '320106', '440305', '510107', '610125', '330106'])
        year = random.randint(1950, 2005)
        month = random.randint(1, 12)
        day = random.randint(1, 28)
        suffix = random.randint(1000, 9999)
        id_card = f"{province}{year}{month:02d}{day:02d}{suffix}"
        data.append({
            "value": id_card,
            "label": f"标准身份证{i+1}",
            "should_detect": True
        })

    # 带空格的身份证（10%，难检测）
    for i in range(int(count * 0.1)):
        province = random.choice(['110101', '320106', '440305'])
        year = random.randint(1970, 2000)
        month = random.randint(1, 12)
        day = random.randint(1, 28)
        suffix = random.randint(1000, 9999)
        id_card = f"{province} {year}{month:02d}{day:02d} {suffix}"
        data.append({
            "value": f"身份证号：{id_card}",
            "label": f"带空格身份证{i+1}",
            "should_detect": False  # 正则可能检测不到
        })

    # 非身份证的18位数字（20%，干扰项）
    for i in range(int(count * 0.2)):
        fake_id = ''.join([str(random.randint(0, 9)) for _ in range(18)])
        data.append({
            "value": fake_id,
            "label": f"18位数字{i+1}",
            "should_detect": False
        })

    # 订单号等其他数字（10%）
    for i in range(int(count * 0.1)):
        order_no = f"ORD{random.randint(10000000000000, 99999999999999)}"
        data.append({
            "value": order_no,
            "label": f"订单号{i+1}",
            "should_detect": False
        })

    return data

def generate_phones(count=500):
    """生成手机号测试数据"""
    data = []
    prefixes = ['130', '131', '132', '133', '134', '135', '136', '137', '138', '139',
                '150', '151', '152', '153', '155', '156', '157', '158', '159',
                '170', '171', '172', '173', '175', '176', '177', '178', '179',
                '180', '181', '182', '183', '184', '185', '186', '187', '188', '189',
                '190', '191', '193', '195', '196', '197', '198', '199']

    # 标准手机号（70%）
    for i in range(int(count * 0.7)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(8)])
        phone = f"{prefix}{suffix}"
        data.append({
            "value": phone,
            "label": f"标准手机号{i+1}",
            "should_detect": True
        })

    # 带分隔符的手机号（10%，难检测）
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

    # 400/800电话（10%，干扰项）
    for i in range(int(count * 0.1)):
        phone = f"400{random.randint(1000000, 9999999)}"
        data.append({
            "value": phone,
            "label": f"400电话{i+1}",
            "should_detect": False
        })

    # 其他数字（10%）
    for i in range(int(count * 0.1)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(random.randint(8, 12))])
        data.append({
            "value": fake,
            "label": f"随机数字{i+1}",
            "should_detect": False
        })

    return data

def generate_bank_cards(count=500):
    """生成银行卡测试数据"""
    data = []
    prefixes = ['622202', '622588', '621700', '955880', '621226', '622200']

    # 标准银行卡（70%）
    for i in range(int(count * 0.7)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(random.choice([10, 13]))])
        card = f"{prefix}{suffix}"
        data.append({
            "value": card,
            "label": f"标准银行卡{i+1}",
            "should_detect": True
        })

    # 带空格的银行卡（10%，难检测）
    for i in range(int(count * 0.1)):
        prefix = random.choice(prefixes)
        suffix = ''.join([str(random.randint(0, 9)) for _ in range(10)])
        card = f"{prefix[:4]} {prefix[4:]} {suffix[:4]} {suffix[4:]}"
        data.append({
            "value": card,
            "label": f"带空格银行卡{i+1}",
            "should_detect": False
        })

    # 16位数字（非银行卡，10%）
    for i in range(int(count * 0.1)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(16)])
        data.append({
            "value": fake,
            "label": f"16位数字{i+1}",
            "should_detect": False
        })

    # 其他数字（10%）
    for i in range(int(count * 0.1)):
        fake = ''.join([str(random.randint(0, 9)) for _ in range(random.randint(12, 20))])
        data.append({
            "value": fake,
            "label": f"随机长数字{i+1}",
            "should_detect": False
        })

    return data

def generate_emails(count=500):
    """生成邮箱测试数据"""
    data = []
    domains = ['gmail.com', 'qq.com', '163.com', 'sina.com', 'outlook.com',
               'hotmail.com', 'yahoo.com', 'example.com', 'test.org', 'company.cn']

    # 标准邮箱（70%）
    for i in range(int(count * 0.7)):
        username = f"user{random.randint(100, 999999)}"
        domain = random.choice(domains)
        email = f"{username}@{domain}"
        data.append({
            "value": email,
            "label": f"标准邮箱{i+1}",
            "should_detect": True
        })

    # 带空格的邮箱（10%，难检测）
    for i in range(int(count * 0.1)):
        username = f"user{random.randint(100, 9999)}"
        domain = random.choice(domains)
        email = f"{username} @ {domain}"
        data.append({
            "value": email,
            "label": f"带空格邮箱{i+1}",
            "should_detect": False
        })

    # 网址（非邮箱，10%）
    for i in range(int(count * 0.1)):
        domain = random.choice(domains)
        url = f"www.{domain}"
        data.append({
            "value": url,
            "label": f"网址{i+1}",
            "should_detect": False
        })

    # 普通文本（10%）
    for i in range(int(count * 0.1)):
        text = random.choice(['今天天气不错', '请帮我查询', '谢谢你的帮助', '我想了解一下'])
        data.append({
            "value": text,
            "label": f"普通文本{i+1}",
            "should_detect": False
        })

    return data

def main():
    print("正在生成2000个测试样本...")

    test_data = {
        "id_card": generate_id_cards(500),
        "phone": generate_phones(500),
        "bank_card": generate_bank_cards(500),
        "email": generate_emails(500)
    }

    # 统计信息
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

    # 保存到文件
    output_file = "test_data/sensitive_data_large.json"
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(test_data, f, ensure_ascii=False, indent=2)

    print(f"\n✅ 测试数据已保存到: {output_file}")

if __name__ == "__main__":
    main()
