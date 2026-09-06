"""共享工具库：Tailwind CSS 色阶 + 绘图辅助函数"""
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.font_manager as fm

# 中文字体配置
_cn_font = None
for _fname in ['Microsoft YaHei', 'Noto Sans SC', 'SimHei', 'WenQuanYi Micro Hei']:
    try:
        fm.findfont(_fname, fallback_to_default=False)
        _cn_font = _fname
        break
    except Exception:
        continue
if _cn_font:
    plt.rcParams['font.sans-serif'] = [_cn_font] + plt.rcParams.get('font.sans-serif', [])
plt.rcParams['axes.unicode_minus'] = False
import matplotlib.patches as mpatches
from matplotlib.patches import FancyBboxPatch, FancyArrowPatch
import numpy as np

COLORS = {
    'bg':         '#F8FAFC',
    'layer1':     '#DBEAFE', 'layer1_bdr': '#3B82F6', 'accent1': '#2563EB',
    'layer2':     '#FEF3C7', 'layer2_bdr': '#F59E0B', 'accent2': '#D97706',
    'layer3':     '#D1FAE5', 'layer3_bdr': '#10B981', 'accent3': '#059669',
    'layer4':     '#EDE9FE', 'layer4_bdr': '#8B5CF6', 'accent4': '#7C3AED',
    'layer5':     '#F1F5F9', 'layer5_bdr': '#64748B', 'accent5': '#475569',
    'text_dark':  '#1E293B', 'text_mid': '#64748B', 'arrow': '#94A3B8',
    'white':      '#FFFFFF',
}

def new_fig(w, h):
    fig, ax = plt.subplots(figsize=(w, h))
    fig.patch.set_facecolor(COLORS['bg'])
    ax.set_xlim(0, 1); ax.set_ylim(0, 1)
    ax.axis('off'); ax.set_aspect('equal')
    return fig, ax

def draw_rounded_box(ax, xy, w, h, facecolor, edgecolor, text='',
                     fontsize=10, fontweight='bold', fontcolor='#1E293B',
                     sub_text=None, sub_fontsize=8, lw=1.5, radius=0.012):
    box = FancyBboxPatch(xy, w, h, boxstyle=f"round,pad=0.003,rounding_size={radius}",
                         facecolor=facecolor, edgecolor=edgecolor, linewidth=lw, zorder=2)
    ax.add_patch(box)
    cx, cy = xy[0] + w/2, xy[1] + h/2
    if sub_text:
        cy += h * 0.1
    if text:
        ax.text(cx, cy, text, ha='center', va='center', fontsize=fontsize,
                fontweight=fontweight, color=fontcolor, zorder=3)
    if sub_text:
        ax.text(cx, xy[1] + h * 0.28, sub_text, ha='center', va='center',
                fontsize=sub_fontsize, color=COLORS['text_mid'], zorder=3, linespacing=1.3)

def draw_arrow(ax, start, end, color='#94A3B8', lw=1.5):
    arrow = FancyArrowPatch(start, end, arrowstyle='->', color=color,
                            linewidth=lw, mutation_scale=12, zorder=4)
    ax.add_patch(arrow)

def draw_dashed_arrow(ax, start, end, color='#94A3B8', lw=1):
    arrow = FancyArrowPatch(start, end, arrowstyle='->', color=color,
                            linewidth=lw, mutation_scale=10,
                            linestyle='dashed', zorder=4)
    ax.add_patch(arrow)

def draw_layer_bg(ax, bottom, height, facecolor, edgecolor, label='', label_color='#1E293B'):
    w, h = 0.98, height
    x = (1 - w) / 2
    box = FancyBboxPatch((x, bottom), w, h, boxstyle="round,pad=0.005,rounding_size=0.01",
                         facecolor=facecolor, edgecolor=edgecolor, linewidth=1.5,
                         alpha=0.5, zorder=0)
    ax.add_patch(box)
    if label:
        ax.text(x + 0.01, bottom + h - 0.015, label, fontsize=9, fontweight='bold',
                color=label_color, va='top', zorder=1)
