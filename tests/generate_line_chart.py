#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
生成血压趋势折线图示例
"""

import matplotlib.pyplot as plt
import matplotlib
import numpy as np
from datetime import datetime, timedelta

# 设置中文字体
matplotlib.rcParams['font.sans-serif'] = ['Microsoft YaHei', 'SimHei']
matplotlib.rcParams['axes.unicode_minus'] = False

# 生成示例数据：某位老人7天的血压记录
dates = [datetime.now() - timedelta(days=i) for i in range(6, -1, -1)]
date_labels = [d.strftime('%m-%d') for d in dates]

# 收缩压和舒张压数据
systolic = [138, 142, 135, 140, 145, 139, 136]  # 收缩压
diastolic = [85, 88, 82, 86, 90, 84, 83]  # 舒张压

# 创建折线图
fig, ax = plt.subplots(figsize=(12, 6))

# 绘制两条折线
line1 = ax.plot(date_labels, systolic, marker='o', linewidth=2,
                markersize=8, color='#FF6B6B', label='收缩压 (mmHg)')
line2 = ax.plot(date_labels, diastolic, marker='s', linewidth=2,
                markersize=8, color='#4ECDC4', label='舒张压 (mmHg)')

# 添加数值标签
for i, (sys, dia) in enumerate(zip(systolic, diastolic)):
    ax.text(i, sys + 2, str(sys), ha='center', va='bottom', fontsize=10)
    ax.text(i, dia - 2, str(dia), ha='center', va='top', fontsize=10)

# 添加正常范围参考线
ax.axhline(y=140, color='red', linestyle='--', alpha=0.3, label='高血压界限 (140)')
ax.axhline(y=90, color='orange', linestyle='--', alpha=0.3, label='舒张压界限 (90)')

ax.set_xlabel('日期', fontsize=12)
ax.set_ylabel('血压值 (mmHg)', fontsize=12)
ax.set_title('老人血压7天趋势折线图（示例数据）', fontsize=14, fontweight='bold')
ax.legend(fontsize=11, loc='upper right')
ax.grid(axis='y', alpha=0.3)
ax.set_ylim(75, 150)

plt.tight_layout()
plt.savefig('tests/blood_pressure_trend.png', dpi=300, bbox_inches='tight')
print("[OK] Line chart saved: tests/blood_pressure_trend.png")
plt.close()
