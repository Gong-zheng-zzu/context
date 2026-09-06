"""安全验证信息图：纯数据，无装饰"""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from chart_utils import *

fig, ax = new_fig(16, 10)
ax.set_ylim(-0.02, 1.05)

# ── 标题 ──
ax.text(0.50, 1.02, '忆安智护 · 安全验证报告', ha='center', fontsize=22,
        fontweight='bold', color=COLORS['text_dark'])
ax.text(0.50, 0.985, 'OWASP ZAP 2.17.0  |  2026-05-30  |  Scan Duration: 56s',
        ha='center', fontsize=10, color=COLORS['text_mid'])

# ═══════════════ 左上：ZAP 扫描结果 ═══════════════
ax.text(0.25, 0.94, 'OWASP ZAP 全量扫描结果', fontsize=13, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

stats = [
    ('扫描 URL',       '10'),
    ('攻击插件',       '78'),
    ('规则通过',       '136'),
    ('False Positives','0'),
]
for i, (label, val) in enumerate(stats):
    sx = 0.04 + i * 0.12
    ax.text(sx, 0.90, val, fontsize=20, fontweight='bold', color=COLORS['accent1'], ha='center')
    ax.text(sx, 0.875, label, fontsize=8, color=COLORS['text_mid'], ha='center')

# 风险分布 — 纯文本
ax.text(0.25, 0.84, 'Risk Distribution', fontsize=10, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

risk_data = [
    ('High',   '0'),
    ('Medium', '0'),
    ('Low',    '5'),
    ('Info',   '1'),
]
risk_line = '    '.join([f'{level}: {count}' for level, count in risk_data])
ax.text(0.25, 0.805, risk_line, fontsize=11, color=COLORS['text_dark'], ha='center',
        fontfamily='monospace')

ax.text(0.25, 0.76, '136', fontsize=36, fontweight='bold', color=COLORS['accent3'],
        ha='center')
ax.text(0.25, 0.735, '项安全规则通过', fontsize=10, color=COLORS['text_mid'], ha='center')

# ═══════════════ 左下：手动渗透 ═══════════════
ax.text(0.25, 0.69, '手动 API 渗透测试', fontsize=13, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

manual_tests = [
    ('JWT 伪造 Token',      '拒绝'),
    ('SQL 注入登录绕过',    '防御成功'),
    ('XSS 反射型/存储型',   '防御成功'),
    ('越权访问他人数据',    '防御成功'),
]
for i, (test, result) in enumerate(manual_tests):
    ty = 0.66 - i * 0.035
    row_bg = FancyBboxPatch((0.04, ty - 0.013), 0.42, 0.03,
                            boxstyle="round,pad=0.001",
                            facecolor='#F9FAFB' if i % 2 == 0 else 'white',
                            edgecolor='#E5E7EB', linewidth=0.5, zorder=2)
    ax.add_patch(row_bg)
    ax.text(0.05, ty + 0.002, test, fontsize=9, color=COLORS['text_dark'], zorder=4)
    ax.text(0.44, ty + 0.002, result, fontsize=9, fontweight='bold', color=COLORS['accent3'],
            ha='right', zorder=4)

# ═══════════════ 右半区：脱敏测试 ═══════════════
ax.text(0.75, 0.94, '敏感信息脱敏测试', fontsize=13, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

hdr_y = 0.90
cols = [('类型', 0.54), ('原始值', 0.67), ('脱敏效果', 0.82), ('置信度', 0.93)]
for label, cx in cols:
    ax.text(cx, hdr_y, label, fontsize=9, fontweight='bold', color=COLORS['text_mid'],
            ha='center', zorder=4)
ax.plot([0.52, 0.97], [hdr_y - 0.01, hdr_y - 0.01], color=COLORS['text_dark'],
        linewidth=1.5, zorder=3)

redact_data = [
    ('身份证号', '110101199001011234', '110***********1234', '52%'),
    ('手机号',   '13812345678',        '138****5678',        '98%'),
    ('银行卡号', '6222021234567890123','6222 **** **** 0123','53%'),
    ('API 密钥', 'AKIA1234567890ABCDEF','[AWS密钥]1234...BCDEF','98%'),
]
for i, (typ, orig, masked, conf) in enumerate(redact_data):
    ry = 0.86 - i * 0.04
    row_bg = FancyBboxPatch((0.52, ry - 0.015), 0.45, 0.035,
                            boxstyle="round,pad=0.001",
                            facecolor='#F9FAFB' if i % 2 == 0 else 'white',
                            edgecolor='#E5E7EB', linewidth=0.5, zorder=2)
    ax.add_patch(row_bg)
    ax.text(0.54, ry + 0.002, typ, fontsize=9, fontweight='bold', color=COLORS['text_dark'],
            zorder=4)
    ax.text(0.67, ry + 0.002, orig, fontsize=8, color=COLORS['text_mid'], zorder=4)
    ax.text(0.82, ry + 0.002, masked, fontsize=8, fontweight='bold',
            color=COLORS['accent3'], zorder=4)
    ax.text(0.93, ry + 0.002, conf, fontsize=9, fontweight='bold', color=COLORS['accent1'],
            zorder=4)
ax.plot([0.52, 0.97], [0.86 - 4*0.04, 0.86 - 4*0.04], color=COLORS['text_dark'],
        linewidth=1.5, zorder=3)

# ═══════════════ 右下：风险评估 ═══════════════
ax.text(0.75, 0.64, '风险评估结果', fontsize=13, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

ax.text(0.75, 0.605, '输入：身份证 + 手机号 + 银行卡', fontsize=9,
        color=COLORS['text_mid'], ha='center')

ax.text(0.75, 0.56, '风险评分：95 / 100    等级：CRITICAL', fontsize=12,
        fontweight='bold', color=COLORS['text_dark'], ha='center')

ax.text(0.75, 0.52, '响应动作：BLOCK 拦截  |  ALERT 告警  |  REDACT 脱敏', fontsize=10,
        color=COLORS['text_mid'], ha='center')

ax.text(0.75, 0.48, '触发条件：多种敏感信息混合输入', fontsize=9,
        color=COLORS['text_mid'], ha='center')

# ═══════════════ 底部：攻击向量 ═══════════════
ax.plot([0.03, 0.97], [0.42, 0.42], color='#E5E7EB', linewidth=1, zorder=2)
ax.text(0.50, 0.40, '主动扫描覆盖的 78 种攻击向量', fontsize=10, fontweight='bold',
        color=COLORS['text_dark'], ha='center')

plugins = [
    ('SQL Injection',       'MySQL/PostgreSQL/Oracle/MsSQL/Hypersonic'),
    ('Cross-Site Scripting','Reflected / Persistent / DOM Based'),
    ('Remote Code Exec',    'ShellShock / CVE-2012-1823 / Spring4Shell'),
    ('Server-Side',         'SSRF / XXE / Template Injection / SSI'),
    ('Cryptographic',       'Heartbleed / CRLF Injection'),
    ('Infrastructure',      'Log4Shell / Text4shell / Cloud Metadata'),
]
for i, (cat, detail) in enumerate(plugins):
    px = 0.05 + (i % 3) * 0.32
    py = 0.37 - (i // 3) * 0.04
    ax.text(px, py, cat, fontsize=9, fontweight='bold', color=COLORS['accent1'], zorder=4)
    ax.text(px + 0.13, py, detail, fontsize=8, color=COLORS['text_mid'], zorder=4)

# ═══════════════ 底部来源标注 ═══════════════
ax.plot([0.03, 0.97], [0.26, 0.26], color='#E5E7EB', linewidth=1, zorder=2)
ax.text(0.50, 0.24, '数据来源：OWASP ZAP 2.17.0 全量扫描 + 手动 API 渗透 + 内置安全引擎实测',
        ha='center', fontsize=9, color=COLORS['text_mid'])

plt.tight_layout()
plt.savefig('fig6_security_infographic.png', dpi=200, bbox_inches='tight', facecolor=COLORS['bg'])
plt.close()
print("fig6_security_infographic.png done")
