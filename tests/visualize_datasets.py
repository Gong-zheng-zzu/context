#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
可视化测试数据集统计信息
"""

import json
import matplotlib.pyplot as plt
import matplotlib
from collections import Counter

# 设置中文字体
matplotlib.rcParams['font.sans-serif'] = ['Microsoft YaHei', 'SimHei']
matplotlib.rcParams['axes.unicode_minus'] = False

def visualize_attack_samples():
    """可视化攻击样本分布"""
    with open('tests/datasets/attack_samples/all_attack_samples.json', 'r', encoding='utf-8') as f:
        samples = json.load(f)

    # 统计攻击类型分布
    type_counts = Counter(s['type'] for s in samples)

    # 创建柱状图
    fig, ax = plt.subplots(figsize=(12, 6))
    types = list(type_counts.keys())
    counts = list(type_counts.values())

    bars = ax.bar(types, counts, color=['#FF6B6B', '#4ECDC4', '#45B7D1', '#FFA07A', '#98D8C8', '#F7DC6F'])
    ax.set_xlabel('攻击类型', fontsize=12)
    ax.set_ylabel('样本数量', fontsize=12)
    ax.set_title('攻击样本数据集分布（真实生成数据）', fontsize=14, fontweight='bold')
    ax.set_ylim(0, 120)

    # 在柱状图上标注数值
    for bar in bars:
        height = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2., height,
                f'{int(height)}',
                ha='center', va='bottom', fontsize=10)

    plt.xticks(rotation=45, ha='right')
    plt.tight_layout()
    plt.savefig('tests/attack_samples_distribution.png', dpi=300, bbox_inches='tight')
    print("[OK] Attack samples chart saved: tests/attack_samples_distribution.png")
    plt.close()

def visualize_nursing_events():
    """可视化护理事件分布"""
    with open('tests/datasets/nursing_data/nursing_records.json', 'r', encoding='utf-8') as f:
        records = json.load(f)

    # 统计事件类型分布
    event_counts = Counter(r['event_type'] for r in records)

    # 创建饼图
    fig, ax = plt.subplots(figsize=(10, 8))
    colors = ['#FF6B6B', '#4ECDC4', '#45B7D1', '#FFA07A', '#98D8C8', '#F7DC6F', '#DDA15E', '#BC6C25']

    wedges, texts, autotexts = ax.pie(
        event_counts.values(),
        labels=event_counts.keys(),
        autopct='%1.1f%%',
        colors=colors,
        startangle=90
    )

    # 美化文本
    for text in texts:
        text.set_fontsize(11)
    for autotext in autotexts:
        autotext.set_color('white')
        autotext.set_fontweight('bold')
        autotext.set_fontsize(10)

    ax.set_title('护理事件类型分布（9,809条真实记录）', fontsize=14, fontweight='bold')
    plt.tight_layout()
    plt.savefig('tests/nursing_events_distribution.png', dpi=300, bbox_inches='tight')
    print("[OK] Nursing events chart saved: tests/nursing_events_distribution.png")
    plt.close()

def visualize_age_distribution():
    """可视化老人年龄分布"""
    with open('tests/datasets/nursing_data/elder_profiles.json', 'r', encoding='utf-8') as f:
        profiles = json.load(f)

    ages = [p['age'] for p in profiles]

    # 创建直方图
    fig, ax = plt.subplots(figsize=(10, 6))
    n, bins, patches = ax.hist(ages, bins=10, color='#4ECDC4', edgecolor='black', alpha=0.7)

    ax.set_xlabel('年龄', fontsize=12)
    ax.set_ylabel('人数', fontsize=12)
    ax.set_title('养老院老人年龄分布（100人档案）', fontsize=14, fontweight='bold')
    ax.grid(axis='y', alpha=0.3)

    plt.tight_layout()
    plt.savefig('tests/elder_age_distribution.png', dpi=300, bbox_inches='tight')
    print("[OK] Age distribution chart saved: tests/elder_age_distribution.png")
    plt.close()

def create_mock_comparison_chart():
    """创建预期测试结果对比图（标注为预期值）"""
    systems = ['Vanilla\nLLM', 'Naive\nRAG', 'RAG+\nFilter', 'Full\nSystem']
    asr_values = [70, 60, 30, 8]  # 攻击成功率（预期值）
    defense_rates = [30, 40, 70, 92]  # 防御成功率（预期值）

    fig, ax = plt.subplots(figsize=(12, 6))

    x = range(len(systems))
    width = 0.35

    bars1 = ax.bar([i - width/2 for i in x], asr_values, width, label='Attack Success Rate ASR (%)', color='#FF6B6B')
    bars2 = ax.bar([i + width/2 for i in x], defense_rates, width, label='Defense Rate (%)', color='#4ECDC4')

    ax.set_xlabel('System Configuration', fontsize=12)
    ax.set_ylabel('Percentage (%)', fontsize=12)
    ax.set_title('Expected Test Results (WARNING: Expected, Not Real)', fontsize=14, fontweight='bold', color='red')
    ax.set_xticks(x)
    ax.set_xticklabels(systems)
    ax.legend(fontsize=11)
    ax.set_ylim(0, 100)
    ax.grid(axis='y', alpha=0.3)

    # 添加数值标签
    for bars in [bars1, bars2]:
        for bar in bars:
            height = bar.get_height()
            ax.text(bar.get_x() + bar.get_width()/2., height,
                    f'{int(height)}%',
                    ha='center', va='bottom', fontsize=9)

    # 添加警告文本
    ax.text(0.5, 0.95, 'WARNING: Expected values from proposal, not actual test results',
            transform=ax.transAxes, fontsize=10, color='red',
            ha='center', va='top', bbox=dict(boxstyle='round', facecolor='yellow', alpha=0.3))

    plt.tight_layout()
    plt.savefig('tests/expected_comparison.png', dpi=300, bbox_inches='tight')
    print("[WARNING] Expected results chart saved: tests/expected_comparison.png (marked as expected)")
    plt.close()

if __name__ == '__main__':
    print("Generating dataset visualization charts...\n")

    try:
        visualize_attack_samples()
    except Exception as e:
        print(f"[ERROR] Attack samples chart failed: {e}")

    try:
        visualize_nursing_events()
    except Exception as e:
        print(f"[ERROR] Nursing events chart failed: {e}")

    try:
        visualize_age_distribution()
    except Exception as e:
        print(f"[ERROR] Age distribution chart failed: {e}")

    try:
        create_mock_comparison_chart()
    except Exception as e:
        print(f"[ERROR] Comparison chart failed: {e}")

    print("\n[DONE] All charts generated!")
    print("Location: d:/context/context-keeper-main/tests/")
