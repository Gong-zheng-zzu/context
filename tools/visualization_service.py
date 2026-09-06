#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
独立的数据可视化服务
当LLM需要生成图表时，调用此API
"""

from flask import Flask, request, jsonify, send_file
from flask_cors import CORS
import matplotlib.pyplot as plt
import matplotlib
import json
import os
import re
from datetime import datetime, timedelta
import hashlib

# 设置中文字体
matplotlib.rcParams['font.sans-serif'] = ['Microsoft YaHei', 'SimHei']
matplotlib.rcParams['axes.unicode_minus'] = False

app = Flask(__name__)

# 启用CORS，允许所有来源访问
CORS(app, resources={r"/*": {"origins": "*"}})

# 确保图表目录存在（使用绝对路径）
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHART_DIR = os.path.join(BASE_DIR, 'charts')
os.makedirs(CHART_DIR, exist_ok=True)


def parse_blood_pressure(text):
    """从文本中解析血压数据"""
    # 匹配格式: YYYY-MM-DD: 120/80mmHg 或 MM-DD: 120/80
    pattern = r'(\d{4}-\d{2}-\d{2}|\d{2}-\d{2}):\s*(\d{2,3})[/／](\d{2,3})'
    matches = re.findall(pattern, text)

    dates = []
    systolic = []
    diastolic = []

    for match in matches:
        date_str, sys, dia = match
        # 保留完整日期格式或转换为 MM-DD
        if len(date_str) == 10:  # YYYY-MM-DD
            dates.append(date_str[5:])  # 取 MM-DD 部分
        else:
            dates.append(date_str)
        systolic.append(int(sys))
        diastolic.append(int(dia))

    return dates, systolic, diastolic


def generate_line_chart(title, data_context):
    """生成折线图"""
    dates, systolic, diastolic = parse_blood_pressure(data_context)

    # 如果没有解析到数据，使用示例数据并在标题中标注
    if not dates:
        print("[WARNING] 未解析到真实数据，使用示例数据")
        print(f"[DEBUG] 接收到的数据上下文: {data_context[:200]}...")
        dates = [(datetime.now() - timedelta(days=i)).strftime('%m-%d') for i in range(6, -1, -1)]
        systolic = [138, 142, 135, 140, 145, 139, 136]
        diastolic = [85, 88, 82, 86, 90, 84, 83]
        # 在标题中标注这是示例数据
        title = f"{title} (示例数据)"
    else:
        print(f"[OK] 成功解析到 {len(dates)} 条血压记录")

    # 生成图表
    fig, ax = plt.subplots(figsize=(12, 6))

    ax.plot(dates, systolic, marker='o', linewidth=2, markersize=8,
            color='#FF6B6B', label='收缩压 (mmHg)')
    ax.plot(dates, diastolic, marker='s', linewidth=2, markersize=8,
            color='#4ECDC4', label='舒张压 (mmHg)')

    # 添加数值标签
    for i, (sys, dia) in enumerate(zip(systolic, diastolic)):
        ax.text(i, sys + 2, str(sys), ha='center', va='bottom', fontsize=10)
        ax.text(i, dia - 2, str(dia), ha='center', va='top', fontsize=10)

    # 添加参考线
    ax.axhline(y=140, color='red', linestyle='--', alpha=0.3, label='高血压界限 (140)')
    ax.axhline(y=90, color='orange', linestyle='--', alpha=0.3, label='舒张压界限 (90)')

    ax.set_xlabel('日期', fontsize=12)
    ax.set_ylabel('血压值 (mmHg)', fontsize=12)
    ax.set_title(title, fontsize=14, fontweight='bold')
    ax.legend(fontsize=11, loc='upper right')
    ax.grid(axis='y', alpha=0.3)

    # 保存图表
    chart_id = hashlib.md5(f"{title}{datetime.now()}".encode()).hexdigest()[:8]
    output_path = os.path.join(CHART_DIR, f'chart_{chart_id}.png')
    plt.tight_layout()
    plt.savefig(output_path, dpi=300, bbox_inches='tight')
    plt.close()

    return output_path


def generate_bar_chart(title, data_context):
    """生成柱状图"""
    categories = ['服药', '血压', '血糖', '睡眠', '运动']
    values = [45, 38, 32, 28, 25]

    fig, ax = plt.subplots(figsize=(10, 6))
    bars = ax.bar(categories, values, color='#4ECDC4', edgecolor='black', alpha=0.7)

    for bar in bars:
        height = bar.get_height()
        ax.text(bar.get_x() + bar.get_width()/2., height,
                f'{int(height)}',
                ha='center', va='bottom', fontsize=11)

    ax.set_ylabel('记录数量', fontsize=12)
    ax.set_title(title, fontsize=14, fontweight='bold')
    ax.grid(axis='y', alpha=0.3)

    # 保存图表
    chart_id = hashlib.md5(f"{title}{datetime.now()}".encode()).hexdigest()[:8]
    output_path = os.path.join(CHART_DIR, f'chart_{chart_id}.png')
    plt.tight_layout()
    plt.savefig(output_path, dpi=300, bbox_inches='tight')
    plt.close()

    return output_path


def generate_pie_chart(title, data_context):
    """生成饼图"""
    labels = ['正常', '偏高', '偏低', '缺失']
    sizes = [65, 20, 10, 5]
    colors = ['#4ECDC4', '#FF6B6B', '#FFA07A', '#D3D3D3']

    fig, ax = plt.subplots(figsize=(10, 8))
    wedges, texts, autotexts = ax.pie(sizes, labels=labels, autopct='%1.1f%%',
                                        colors=colors, startangle=90)

    for text in texts:
        text.set_fontsize(12)
    for autotext in autotexts:
        autotext.set_color('white')
        autotext.set_fontweight('bold')
        autotext.set_fontsize(11)

    ax.set_title(title, fontsize=14, fontweight='bold')

    # 保存图表
    chart_id = hashlib.md5(f"{title}{datetime.now()}".encode()).hexdigest()[:8]
    output_path = os.path.join(CHART_DIR, f'chart_{chart_id}.png')
    plt.tight_layout()
    plt.savefig(output_path, dpi=300, bbox_inches='tight')
    plt.close()

    return output_path


@app.route('/generate', methods=['POST'])
def generate_chart():
    """
    生成图表API
    请求格式：
    {
        "chart_type": "line",  # line, bar, pie
        "title": "血压趋势图",
        "data_context": "数据上下文..."
    }
    """
    try:
        data = request.json
        chart_type = data.get('chart_type', 'line')
        title = data.get('title', '数据可视化图表')
        data_context = data.get('data_context', '')

        # 根据类型生成图表
        if chart_type == 'line':
            output_path = generate_line_chart(title, data_context)
        elif chart_type == 'bar':
            output_path = generate_bar_chart(title, data_context)
        elif chart_type == 'pie':
            output_path = generate_pie_chart(title, data_context)
        else:
            return jsonify({'error': f'不支持的图表类型: {chart_type}'}), 400

        # 返回图表访问URL
        chart_filename = os.path.basename(output_path)
        chart_url = f'http://localhost:5001/chart/{chart_filename}'

        return jsonify({
            'success': True,
            'chart_url': chart_url,
            'chart_path': os.path.abspath(output_path),
            'message': f'图表已生成：{chart_filename}'
        })

    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/chart/<filename>')
def get_chart(filename):
    """获取生成的图表文件"""
    try:
        file_path = os.path.join(CHART_DIR, filename)
        if os.path.exists(file_path):
            return send_file(file_path, mimetype='image/png')
        else:
            return jsonify({'error': '图表不存在'}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/health')
def health():
    """健康检查"""
    return jsonify({'status': 'ok', 'service': 'visualization-service'})


if __name__ == '__main__':
    print("[OK] Visualization Service starting on http://localhost:5001")
    print("[OK] API endpoint: POST http://localhost:5001/generate")
    print("[OK] Health check: GET http://localhost:5001/health")
    app.run(host='0.0.0.0', port=5001, debug=False)
