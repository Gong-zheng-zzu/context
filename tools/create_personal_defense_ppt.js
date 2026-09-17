const pptxgen = require('pptxgenjs');

const pptx = new pptxgen();
pptx.layout = 'LAYOUT_WIDE';
pptx.author = '宫正行';
pptx.subject = '忆安智护个人负责工作答辩';
pptx.title = '忆安智护：个人负责工作答辩';
pptx.company = '郑州大学网络空间安全学院';
pptx.lang = 'zh-CN';
pptx.theme = {
  headFontFace: 'Microsoft YaHei',
  bodyFontFace: 'Microsoft YaHei',
  lang: 'zh-CN',
};

const W = 13.333;
const H = 7.5;
const C = {
  navy: '061D4E',
  blue: '0B4E9A',
  panel: '0B3674',
  cyan: '32D9F2',
  teal: '24C7B4',
  amber: 'F4C64E',
  white: 'FFFFFF',
  muted: 'B9D6EE',
  line: '3BA9DB',
  red: 'E75C5C',
};
const chartDir = 'D:/context/context-keeper-main/experiments/results/reports/charts';

function background(slide, section, index) {
  slide.background = { color: C.navy };
  slide.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: 0.12, fill: { color: C.cyan }, line: { color: C.cyan } });
  slide.addShape(pptx.ShapeType.rect, { x: 0, y: 6.99, w: W, h: 0.51, fill: { color: '041538' }, line: { color: '041538' } });
  slide.addText(section.toUpperCase(), { x: 0.58, y: 0.31, w: 3.2, h: 0.28, fontFace: 'Arial', fontSize: 8.5, bold: true, color: C.cyan, margin: 0 });
  slide.addText('忆安智护 | 个人负责工作答辩', { x: 0.58, y: 7.14, w: 4.0, h: 0.18, fontSize: 8, color: C.muted, margin: 0 });
  slide.addText(String(index).padStart(2, '0'), { x: 12.03, y: 7.09, w: 0.7, h: 0.23, fontFace: 'Arial', fontSize: 12, bold: true, color: C.cyan, align: 'right', margin: 0 });
}

function title(slide, value, subtitle) {
  slide.addText(value, { x: 0.58, y: 0.72, w: 11.9, h: 0.55, fontSize: 27, bold: true, color: C.white, margin: 0, breakLine: false });
  if (subtitle) slide.addText(subtitle, { x: 0.6, y: 1.32, w: 11.8, h: 0.28, fontSize: 11.5, color: C.muted, margin: 0 });
}

function panel(slide, x, y, w, h, options = {}) {
  slide.addShape(pptx.ShapeType.roundRect, {
    x, y, w, h,
    rectRadius: 0.08,
    fill: { color: options.fill || C.panel, transparency: options.transparency ?? 8 },
    line: { color: options.line || C.line, width: options.lineWidth || 1 },
  });
}

function metric(slide, x, y, w, label, value, note, accent) {
  panel(slide, x, y, w, 1.52, { line: accent, fill: '0A3979' });
  slide.addText(label, { x: x + 0.14, y: y + 0.18, w: w - 0.28, h: 0.26, fontSize: 11.5, bold: true, color: C.white, align: 'center', margin: 0 });
  slide.addText(value, { x: x + 0.12, y: y + 0.53, w: w - 0.24, h: 0.43, fontFace: 'Arial', fontSize: 24, bold: true, color: accent, align: 'center', margin: 0 });
  slide.addText(note, { x: x + 0.12, y: y + 1.13, w: w - 0.24, h: 0.18, fontSize: 8.5, color: C.muted, align: 'center', margin: 0 });
}

function workCard(slide, x, y, n, heading, body, accent) {
  panel(slide, x, y, 3.85, 3.82, { line: accent, fill: '092C62' });
  slide.addShape(pptx.ShapeType.ellipse, { x: x + 0.28, y: y + 0.32, w: 0.6, h: 0.6, fill: { color: accent }, line: { color: accent } });
  slide.addText(String(n), { x: x + 0.28, y: y + 0.43, w: 0.6, h: 0.18, fontFace: 'Arial', fontSize: 13, bold: true, color: C.navy, align: 'center', margin: 0 });
  slide.addText(heading, { x: x + 1.04, y: y + 0.35, w: 2.45, h: 0.28, fontSize: 17, bold: true, color: C.white, margin: 0 });
  slide.addText(body, { x: x + 0.35, y: y + 1.18, w: 3.15, h: 2.25, fontSize: 12, color: C.muted, breakLine: false, margin: 0.02, breakLine: false, valign: 'top', fit: 'shrink' });
}

// Slide 1
{
  const s = pptx.addSlide();
  s.background = { color: C.navy };
  s.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: H, fill: { color: C.navy }, line: { color: C.navy } });
  s.addShape(pptx.ShapeType.arc, { x: 8.7, y: -1.6, w: 5.6, h: 5.6, adjustPoint: 0.25, rotate: 30, line: { color: C.cyan, transparency: 65, width: 2 }, fill: { color: C.navy, transparency: 100 } });
  s.addShape(pptx.ShapeType.ellipse, { x: 9.9, y: 1.12, w: 1.68, h: 1.68, fill: { color: C.panel }, line: { color: C.cyan, width: 2 } });
  s.addText('忆安智护', { x: 0.72, y: 1.38, w: 5.8, h: 0.7, fontSize: 35, bold: true, color: C.white, margin: 0 });
  s.addText('个人负责工作答辩', { x: 0.74, y: 2.18, w: 4.6, h: 0.4, fontSize: 19, color: C.cyan, bold: true, margin: 0 });
  s.addText('代码编写与修改 · 实验运行与审核 · PPT与策划书制作', { x: 0.75, y: 3.0, w: 7.4, h: 0.36, fontSize: 15, color: C.muted, margin: 0 });
  s.addShape(pptx.ShapeType.line, { x: 0.76, y: 3.67, w: 5.58, h: 0, line: { color: C.cyan, width: 1.5 } });
  s.addText('郑州大学网络空间安全学院\n答辩人：宫正行', { x: 0.75, y: 4.22, w: 4.8, h: 0.72, fontSize: 14, color: C.white, breakLine: false, margin: 0 });
  s.addText('CODE\nEXPERIMENT\nMATERIAL', { x: 9.4, y: 3.42, w: 2.7, h: 1.2, fontFace: 'Arial', fontSize: 15, bold: true, color: C.amber, align: 'center', margin: 0 });
  s.addText('01', { x: 0.75, y: 6.7, w: 0.6, h: 0.24, fontFace: 'Arial', fontSize: 13, color: C.cyan, bold: true, margin: 0 });
}

// Slide 2
{
  const s = pptx.addSlide();
  background(s, 'PERSONAL SCOPE', 2);
  title(s, '我主要负责的三项工作', '以工程实现、可信实验和答辩材料为主线推进项目落地');
  workCard(s, 0.62, 1.92, 1, '代码编写与修复', '围绕认证、检索、因果、安全与遗忘链路进行定位和修改。\n\n重点处理：JWT 接入、配置切换、三路 RRF、跨存储删除、性能与错误分母。', C.cyan);
  workCard(s, 4.74, 1.92, 2, '实验运行与审核', '搭建固定数据集、评测脚本、运行清单和原始结果审核流程。\n\n确保 API 错误不进入业务指标，结果可回溯至数据、配置与日志。', C.teal);
  workCard(s, 8.86, 1.92, 3, 'PPT与策划书制作', '将技术逻辑、场景价值和实测结果转化为答辩材料。\n\n制作项目叙事、实验页、图表、报告与可现场演示的说明材料。', C.amber);
}

// Slide 3
{
  const s = pptx.addSlide();
  background(s, 'ENGINEERING', 3);
  title(s, '代码工作：把“能运行”变成“可验证”', '我负责的修改重点，是让实验接口、数据范围和结果口径真正对齐');
  const items = [
    ['认证与路由', '统一 JWT 登录和受保护 API；避免 404、认证失败被误算为业务成功。'],
    ['检索链路', '补齐 doc_id 溯源、会话隔离和直接三路 RRF 评测路径。'],
    ['安全与因果', '补充输入策略、修复 PCCM 置信度融合与中文 O-M-P-R 解析。'],
    ['遗忘验证', '实现 Qdrant、TimescaleDB、Neo4j、缓存的跨存储计数与删除校验。'],
  ];
  items.forEach((item, i) => {
    const x = i % 2 === 0 ? 0.78 : 6.87;
    const y = i < 2 ? 1.9 : 4.08;
    panel(s, x, y, 5.63, 1.63, { line: i % 2 ? C.teal : C.cyan, fill: '092C62' });
    s.addText(`0${i + 1}`, { x: x + 0.28, y: y + 0.32, w: 0.62, h: 0.26, fontFace: 'Arial', fontSize: 15, bold: true, color: i % 2 ? C.teal : C.cyan, margin: 0 });
    s.addText(item[0], { x: x + 1.1, y: y + 0.27, w: 3.9, h: 0.25, fontSize: 15, bold: true, color: C.white, margin: 0 });
    s.addText(item[1], { x: x + 1.1, y: y + 0.74, w: 4.15, h: 0.56, fontSize: 10.8, color: C.muted, margin: 0, fit: 'shrink' });
  });
}

// Slide 4
{
  const s = pptx.addSlide();
  background(s, 'EXPERIMENT', 4);
  title(s, '实验工作：固定数据、独立配置、原始证据', '把结果从一次性演示，变成可复跑、可检查、可解释的实验过程');
  const rows = [
    ['检索', '30条语料 + 50条标注查询', '3配置分别运行，保存 MRR、P@5、R@5 与来源 trace'],
    ['因果', '20条人工 O-M-P-R 标注', '保存有效响应、严格四元组匹配、置信度与延迟'],
    ['安全', '6类攻击，每类20条', 'API 错误单列；ASR 仅按有效响应计算'],
    ['遗忘', '目标会话 + 对照会话', '3次独立试验，跨四类存储计数与检索探针验证'],
  ];
  s.addTable([['实验', '固定数据', '我负责的验证方式'], ...rows], {
    x: 0.75, y: 1.85, w: 11.82, h: 3.55,
    border: { type: 'solid', color: '79CFF2', pt: 1 },
    fill: C.panel,
    color: C.white,
    fontFace: 'Microsoft YaHei',
    fontSize: 12,
    rowH: 0.68,
    colW: [1.25, 3.15, 7.42],
    margin: 0.08,
    autoFit: false,
    bold: false,
    valign: 'mid',
    fill: C.panel,
    autoFit: false,
  });
  s.addShape(pptx.ShapeType.roundRect, { x: 0.75, y: 5.73, w: 11.82, h: 0.62, rectRadius: 0.06, fill: { color: '08336B' }, line: { color: C.cyan, width: 1 } });
  s.addText('关键原则：原始 JSON + 配置哈希 + 数据集哈希 + 运行日志；认证、超时和服务错误不进入业务指标分母。', { x: 1.0, y: 5.92, w: 11.3, h: 0.2, fontSize: 11, bold: true, color: C.amber, align: 'center', margin: 0 });
}

// Slide 5
{
  const s = pptx.addSlide();
  background(s, 'MEASURED RESULTS', 5);
  title(s, '实测结果：我的工作如何形成可答辩证据', '以下均来自通过有效性门槛的原始 JSON 与运行清单');
  metric(s, 0.78, 1.78, 2.75, '三路检索 MRR', '0.491', '50条查询 | 103.5 ms', C.cyan);
  metric(s, 3.78, 1.78, 2.75, '因果严格 O-M-P-R', '85%', '20条标注案例', C.teal);
  metric(s, 6.78, 1.78, 2.75, '安全防御成功率', '93.3%', '120条攻击样本', C.amber);
  metric(s, 9.78, 1.78, 2.75, '机器遗忘', '3 / 3', '跨存储试验通过', C.cyan);
  panel(s, 0.78, 3.72, 5.74, 2.22, { line: C.line, fill: '092C62' });
  s.addText('检索优化结果', { x: 1.06, y: 4.02, w: 2.1, h: 0.28, fontSize: 16, bold: true, color: C.white, margin: 0 });
  s.addText('通过跳过评测场景下的 LLM 语义分析与回答生成，将三路 RRF 检索从约 16.4 秒降至 103.5 毫秒，同时保留 vector、timeline、knowledge 的来源审计。', { x: 1.06, y: 4.52, w: 4.98, h: 0.72, fontSize: 11.3, color: C.muted, margin: 0, fit: 'shrink' });
  s.addImage({ path: `${chartDir}/retrieval_quality.png`, x: 6.85, y: 3.72, w: 5.7, h: 2.22, transparency: 4 });
}

// Slide 6
{
  const s = pptx.addSlide();
  background(s, 'MATERIAL & CLOSING', 6);
  title(s, '材料制作与答辩交付', '把工程过程转换为评委能够快速理解和复核的表达');
  const deliverables = [
    ['PPT叙事', '将项目背景、技术架构、实验结果和未来方向串成答辩主线。'],
    ['策划书', '梳理需求、系统设计、实现、测试与实习工作内容。'],
    ['实验报告', '生成 HTML 报告、CSV、PNG 图表和原始 JSON 证据。'],
  ];
  deliverables.forEach((item, i) => {
    const y = 1.82 + i * 1.24;
    s.addShape(pptx.ShapeType.roundRect, { x: 0.82, y, w: 7.06, h: 0.94, rectRadius: 0.05, fill: { color: '092C62' }, line: { color: i === 2 ? C.amber : C.cyan, width: 1 } });
    s.addText(`0${i + 1}`, { x: 1.08, y: y + 0.31, w: 0.44, h: 0.18, fontFace: 'Arial', fontSize: 13, bold: true, color: i === 2 ? C.amber : C.cyan, margin: 0 });
    s.addText(item[0], { x: 1.82, y: y + 0.22, w: 1.35, h: 0.22, fontSize: 14, bold: true, color: C.white, margin: 0 });
    s.addText(item[1], { x: 3.2, y: y + 0.22, w: 4.25, h: 0.4, fontSize: 10.5, color: C.muted, margin: 0, fit: 'shrink' });
  });
  panel(s, 8.28, 1.82, 4.22, 3.42, { line: C.teal, fill: '0A3979' });
  s.addText('个人工作总结', { x: 8.62, y: 2.15, w: 3.45, h: 0.26, fontSize: 17, bold: true, color: C.white, align: 'center', margin: 0 });
  s.addText('我承担的核心不是单纯展示功能，而是把代码修复、实验验证和答辩材料连接起来：\n\n让系统可运行，让指标可解释，让结果可复核。', { x: 8.72, y: 2.78, w: 3.22, h: 1.42, fontSize: 13, color: C.muted, align: 'center', valign: 'mid', margin: 0.02, fit: 'shrink' });
  s.addText('谢谢各位老师', { x: 8.77, y: 4.55, w: 3.1, h: 0.25, fontSize: 15, bold: true, color: C.amber, align: 'center', margin: 0 });
}

pptx.writeFile({ fileName: 'D:/context/忆安智护_个人工作答辩版.pptx' });
