#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
实验报告生成器
从原始JSON数据生成HTML报告

功能：
1. 完整报告：5章节 + 4张关键图表
2. 演示报告：4页答辩slides
"""

import json
import argparse
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Tuple
import matplotlib
matplotlib.use('Agg')  # 非GUI后端
import matplotlib.pyplot as plt
import numpy as np
import io
import base64


RETRIEVAL_MIN_COMPLETE_QUERIES = 50
SECURITY_REQUIRED_REQUESTS = 120
SECURITY_BENIGN_REQUIRED_REQUESTS = 100


class ReportBuilder:
    """报告生成器"""

    def __init__(self, mode: str = "full"):
        self.mode = mode
        self.project_root = Path(__file__).parent.parent
        self.results_dir = self.project_root / "results" / "raw"
        self.reports_dir = self.project_root / "results" / "reports"
        self.reports_dir.mkdir(parents=True, exist_ok=True)
        self.rejected_results: Dict[str, List[str]] = {}
        self.selected_results: Dict[str, str] = {}

    @staticmethod
    def _has_raw_rows(data: Dict) -> bool:
        """Raw, row-level evidence is required before an aggregate can be reported."""
        rows = data.get("detailed_results")
        return isinstance(rows, list) and bool(rows) and all(isinstance(row, dict) for row in rows)

    @staticmethod
    def _is_sha256(value) -> bool:
        return isinstance(value, str) and len(value) == 64 and all(char in "0123456789abcdef" for char in value.lower())

    def _validate_result(self, experiment: str, data: Dict) -> Tuple[bool, str]:
        if not isinstance(data, dict):
            return False, "result root is not an object"

        provenance = data.get("runner_provenance")
        if not isinstance(provenance, dict):
            return False, "missing runner_provenance"
        if provenance.get("outcome") != "succeeded" or provenance.get("report_eligible") is not True:
            return False, "runner did not mark the artifact report-eligible"
        allowed_preflight_states = {"passed", "not_run_direct_evaluator"}
        if provenance.get("smoke_status") not in allowed_preflight_states or provenance.get("evaluator_exit_code") != 0:
            return False, "preflight or evaluator did not complete successfully"

        configuration = provenance.get("configuration")
        if not isinstance(configuration, dict) or not all(
            self._is_sha256(configuration.get(field))
            for field in ("base_env_sha256", "overlay_env_sha256", "effective_env_sha256")
        ):
            return False, "configuration provenance is incomplete"
        dataset = provenance.get("data")
        if not isinstance(dataset, dict) or not self._is_sha256(dataset.get("datasets_tree_sha256")):
            return False, "dataset-tree hash is missing"
        if not isinstance(provenance.get("environment"), dict):
            return False, "environment fingerprint is missing"

        if experiment == "retrieval":
            runtime = data.get("runtime_config")
            dataset = data.get("dataset")
            if data.get("execution_mode") != "live_api":
                return False, "retrieval result is not a live API run"
            if not isinstance(runtime, dict) or not self._is_sha256(runtime.get("config_hash")):
                return False, "retrieval runtime configuration hash is missing"
            if not isinstance(dataset, dict) or not self._is_sha256(dataset.get("ground_truth_sha256")):
                return False, "retrieval ground-truth hash is missing"
            metrics = data.get("aggregated_metrics")
            config_name = configuration.get("name")
            total_queries = data.get("total_queries")
            if (
                not isinstance(total_queries, int)
                or total_queries < RETRIEVAL_MIN_COMPLETE_QUERIES
                or not self._has_raw_rows(data)
                or len(data["detailed_results"]) != total_queries
                or data.get("configs") != [config_name]
                or not isinstance(metrics, dict)
                or not isinstance(metrics.get(config_name), dict)
            ):
                return False, (
                    f"retrieval result requires one complete {RETRIEVAL_MIN_COMPLETE_QUERIES}-query "
                    "configuration run with raw rows"
                )
            config_metrics = metrics[config_name]
            if (
                config_metrics.get("total_queries") != total_queries
                or config_metrics.get("api_success_count") != total_queries
                or config_metrics.get("api_error_count") != 0
            ):
                return False, "retrieval API denominator is incomplete"
        elif experiment == "security":
            if data.get("execution_mode") != "live_api" or not isinstance(data.get("dataset_manifest"), list):
                return False, "security result lacks live API dataset evidence"
            metrics = data.get("aggregated_metrics")
            rows = data.get("detailed_results")
            attack_rows = [row for row in rows if row.get("sample_kind", "attack") == "attack"] if self._has_raw_rows(data) else []
            benign_rows = [row for row in rows if row.get("sample_kind") == "benign"] if self._has_raw_rows(data) else []
            if (
                not isinstance(metrics, dict)
                or metrics.get("total_samples") != SECURITY_REQUIRED_REQUESTS
                or metrics.get("api_success_count") != SECURITY_REQUIRED_REQUESTS
                or metrics.get("api_error_count") != 0
                or metrics.get("metrics_status") != "complete"
                or not self._has_raw_rows(data)
                or len(attack_rows) != SECURITY_REQUIRED_REQUESTS
            ):
                return False, (
                    f"security result requires {SECURITY_REQUIRED_REQUESTS} effective responses "
                    f"from {SECURITY_REQUIRED_REQUESTS} requests with raw rows"
                )
            if data.get("schema_version", 0) >= 4 and (
                len(benign_rows) != SECURITY_BENIGN_REQUIRED_REQUESTS
                or metrics.get("benign_total") != SECURITY_BENIGN_REQUIRED_REQUESTS
                or metrics.get("benign_api_success_count") != SECURITY_BENIGN_REQUIRED_REQUESTS
                or metrics.get("benign_api_error_count") != 0
                or not isinstance(metrics.get("benign_pass_rate"), (int, float))
                or not isinstance(metrics.get("false_positive_rate"), (int, float))
            ):
                return False, (
                    f"security schema v4 requires {SECURITY_BENIGN_REQUIRED_REQUESTS} effective benign "
                    "responses and benign rate evidence"
                )
        elif experiment == "causal":
            metrics = data.get("metrics")
            if (
                not isinstance(metrics, dict)
                or not self._has_raw_rows(data)
                or metrics.get("total_samples") != 20
                or metrics.get("api_success_count") != 20
                or metrics.get("api_error_count") != 0
                or len(data["detailed_results"]) != 20
            ):
                return False, "causal result lacks metric or row evidence"
        elif experiment == "unlearning":
            trials = data.get("trials")
            if data.get("passed") is not True or not isinstance(trials, list) or len(trials) != 3:
                return False, "unlearning result does not contain three passing trials"
            for trial in trials:
                if not isinstance(trial, dict) or trial.get("status") != "passed":
                    return False, "unlearning verification did not pass"
                if trial.get("target_post_count") != {
                    "qdrant": 0,
                    "timescaledb": 0,
                    "neo4j": 0,
                    "session_file_cache": 0,
                }:
                    return False, "unlearning post-delete storage counts are incomplete"
                if trial.get("control_pre_count") != trial.get("control_post_count"):
                    return False, "unlearning changed the non-target control session"
                if trial.get("retrieval_probe", {}).get("passed") is not True:
                    return False, "unlearning retrieval probe did not pass"

        return True, ""

    def _unavailable_section(self, number: int, title: str, experiment: str) -> str:
        reasons = self.rejected_results.get(experiment, [])
        reason = reasons[0] if reasons else "未找到可验证的原始结果"
        return (
            f'<div class="section"><h2>{number}. {title}</h2>'
            f'<p><strong>未达到报告门槛：</strong>{reason}</p></div>'
        )

    def _unavailable_slide_text(self, experiment: str) -> str:
        reasons = self.rejected_results.get(experiment, [])
        reason = reasons[0] if reasons else "未找到可验证的原始结果"
        return f"未达到报告门槛：{reason}"

    def _unavailable_chart_text(self, experiment: str) -> str:
        return f'<p style="color:#999;"><strong>{self._unavailable_slide_text(experiment)}</strong></p>'

    @staticmethod
    def _security_config_name(data: Dict) -> str:
        return (
            data.get("configuration", {}).get("label")
            or data.get("runner_provenance", {}).get("configuration", {}).get("name")
            or "observed_configuration"
        )

    @staticmethod
    def _security_attack_asr(metrics: Dict, attack_type: str):
        by_attack_type = metrics.get("by_attack_type", {})
        attack_metrics = by_attack_type.get(attack_type, {}) if isinstance(by_attack_type, dict) else {}
        attack_success = attack_metrics.get("attack_success", {}) if isinstance(attack_metrics, dict) else {}
        rate = attack_success.get("rate") if isinstance(attack_success, dict) else None
        if isinstance(rate, (int, float)):
            return rate
        legacy_rate = metrics.get(f"{attack_type}_asr")
        return legacy_rate if isinstance(legacy_rate, (int, float)) else None

    def load_latest_results(self) -> Dict:
        """加载最新的实验结果"""
        results = {
            "security": None,
            "causal": None,
            "retrieval": None,
            "unlearning": None
        }

        for exp_type in results.keys():
            files = sorted(self.results_dir.glob(f"{exp_type}_*.json"), key=lambda path: path.stat().st_mtime, reverse=True)
            for path in files:
                try:
                    with path.open('r', encoding='utf-8') as handle:
                        candidate = json.load(handle)
                except (OSError, json.JSONDecodeError) as error:
                    self.rejected_results.setdefault(exp_type, []).append(f"{path.name}: unreadable ({error})")
                    continue

                valid, reason = self._validate_result(exp_type, candidate)
                if not valid:
                    self.rejected_results.setdefault(exp_type, []).append(f"{path.name}: {reason}")
                    continue

                results[exp_type] = candidate
                self.selected_results[exp_type] = path.name
                print(f"[LOAD] {exp_type}: {path.name}")
                break

            if results[exp_type] is None and self.rejected_results.get(exp_type):
                print(f"[SKIP] {exp_type}: no eligible result ({self.rejected_results[exp_type][0]})")

        return results

    def generate_html_report(self, results: Dict, output_file: Path):
        """生成HTML报告"""
        if self.mode == "full":
            html = self._generate_full_report(results)
        else:
            html = self._generate_demo_slides(results)

        with open(output_file, 'w', encoding='utf-8') as f:
            f.write(html)

        print(f"[SAVE] 报告已生成: {output_file}")

    def _generate_chart_base64(self, fig) -> str:
        """将matplotlib图表转为base64字符串"""
        buf = io.BytesIO()
        fig.savefig(buf, format='png', dpi=150, bbox_inches='tight', facecolor='white', edgecolor='none')
        buf.seek(0)
        img_base64 = base64.b64encode(buf.read()).decode('utf-8')
        plt.close(fig)
        return f"data:image/png;base64,{img_base64}"

    def _generate_radar_chart(self, security_metrics: Dict, config_name: str) -> str:
        """Generate a radar chart from observed ASR values only."""
        if not security_metrics:
            return ""

        categories = ['提示注入', '记忆投毒', '权限提升', '隐私泄露', '幻觉诱导', '遗忘绕过']
        attack_types = [
            'prompt_injection', 'memory_poisoning', 'privilege_escalation',
            'privacy_leakage', 'hallucination_induction', 'unlearning_bypass'
        ]
        values = [self._security_attack_asr(security_metrics, attack_type) for attack_type in attack_types]
        if any(value is None for value in values):
            return ""

        fig, ax = plt.subplots(figsize=(10, 8), subplot_kw=dict(projection='polar'))
        angles = np.linspace(0, 2 * np.pi, len(categories), endpoint=False).tolist()
        angles += angles[:1]

        percentages = [value * 100 for value in values]
        percentages += percentages[:1]
        ax.plot(angles, percentages, 'o-', linewidth=2, label=config_name, color='#4facfe')
        ax.fill(angles, percentages, alpha=0.15, color='#4facfe')

        ax.set_xticks(angles[:-1])
        ax.set_xticklabels(categories, fontsize=12)
        ax.set_ylim(0, 100)
        ax.set_ylabel('ASR (%)', fontsize=10)
        ax.legend(loc='upper right', bbox_to_anchor=(1.3, 1.1))
        ax.grid(True)
        plt.title(f'安全防护实测 ASR（{config_name}）', fontsize=16, pad=20)

        return self._generate_chart_base64(fig)

    def _generate_pie_chart(self, security_metrics: Dict, config_name: str) -> str:
        """Generate defense-rate distribution from observed ASR values only."""
        if not security_metrics:
            return ""

        categories = ['提示注入', '记忆投毒', '权限提升', '隐私泄露', '幻觉诱导', '遗忘绕过']
        attack_types = [
            'prompt_injection', 'memory_poisoning', 'privilege_escalation',
            'privacy_leakage', 'hallucination_induction', 'unlearning_bypass'
        ]
        asrs = [self._security_attack_asr(security_metrics, attack_type) for attack_type in attack_types]
        if any(asr is None for asr in asrs):
            return ""
        defense_rates = [(1 - asr) * 100 for asr in asrs]

        fig, ax = plt.subplots(figsize=(10, 8))
        colors = ['#667eea', '#764ba2', '#f093fb', '#f5576c', '#4facfe', '#43e97b']

        wedges, texts, autotexts = ax.pie(defense_rates, labels=categories, colors=colors,
                                           autopct='%1.1f%%', startangle=90,
                                           textprops={'fontsize': 12})
        for autotext in autotexts:
            autotext.set_color('white')
            autotext.set_fontweight('bold')
            autotext.set_fontsize(11)

        plt.title(f'各类攻击防御成功率实测分布（{config_name}）', fontsize=16, pad=20)

        return self._generate_chart_base64(fig)

    def _generate_bar_chart(self, results: Dict) -> str:
        """生成柱状图：因果推理+检索融合准确率/MRR对比"""
        fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 6))

        # 子图1：因果推理准确率
        causal_result = results.get('causal') or {}
        causal_data = causal_result.get('aggregated_metrics', {})
        if not causal_data and isinstance(causal_result.get('metrics'), dict):
            causal_data = {
                causal_result.get('run_label', 'observed_run'): {
                    'accuracy': causal_result['metrics'].get('tuple_accuracy'),
                }
            }
        configs = []
        accuracies = []

        for cfg, m in causal_data.items():
            accuracy = m.get('accuracy')
            if not isinstance(accuracy, (int, float)):
                continue
            configs.append(cfg.replace('_', '\n'))
            accuracies.append(accuracy * 100)

        if configs:
            bars1 = ax1.bar(range(len(configs)), accuracies,
                           color=['#667eea', '#764ba2', '#f093fb', '#f5576c'][:len(configs)])
            ax1.set_xticks(range(len(configs)))
            ax1.set_xticklabels(configs, fontsize=9)
            ax1.set_ylabel('准确率 (%)', fontsize=12)
            ax1.set_title('因果推理准确率', fontsize=14)
            ax1.set_ylim(0, 100)
            ax1.grid(axis='y', alpha=0.3)

            for bar, acc in zip(bars1, accuracies):
                height = bar.get_height()
                ax1.text(bar.get_x() + bar.get_width()/2., height,
                        f'{acc:.1f}%', ha='center', va='bottom', fontsize=10)

        # 子图2：检索融合MRR
        retrieval_data = results.get('retrieval', {}).get('aggregated_metrics', {})
        configs2 = []
        mrrs = []

        for cfg, m in retrieval_data.items():
            mrr = m.get('mrr')
            if not isinstance(mrr, (int, float)):
                continue
            configs2.append(cfg.replace('_', '\n'))
            mrrs.append(mrr)

        if configs2:
            bars2 = ax2.bar(range(len(configs2)), mrrs,
                           color=['#667eea', '#764ba2', '#f093fb', '#f5576c'][:len(configs2)])
            ax2.set_xticks(range(len(configs2)))
            ax2.set_xticklabels(configs2, fontsize=9)
            ax2.set_ylabel('MRR', fontsize=12)
            ax2.set_title('检索融合MRR', fontsize=14)
            ax2.set_ylim(0, 1.0)
            ax2.grid(axis='y', alpha=0.3)

            for bar, mrr in zip(bars2, mrrs):
                height = bar.get_height()
                ax2.text(bar.get_x() + bar.get_width()/2., height,
                        f'{mrr:.3f}', ha='center', va='bottom', fontsize=10)

        plt.tight_layout()

        return self._generate_chart_base64(fig)

    def _generate_scatter_plot(self, unlearning_results: List) -> str:
        """生成散点图：遗忘率 vs 副作用率"""
        if not unlearning_results:
            return ""

        fig, ax = plt.subplots(figsize=(10, 8))

        scenarios = []
        unlearning_rates = []
        side_effect_rates = []

        for r in unlearning_results:
            scenarios.append(r.get('scenario_name', 'unknown'))
            unlearning_rates.append(r.get('unlearning_rate', 0) * 100)
            side_effect_rates.append(r.get('side_effect_rate', 0) * 100)

        if not scenarios:
            plt.close(fig)
            return ""

        scatter = ax.scatter(unlearning_rates, side_effect_rates, s=200,
                            c=range(len(scenarios)), cmap='viridis',
                            alpha=0.6, edgecolors='black', linewidth=2)

        for i, scenario in enumerate(scenarios):
            ax.annotate(scenario.replace('_', '\n'),
                       (unlearning_rates[i], side_effect_rates[i]),
                       xytext=(5, 5), textcoords='offset points',
                       fontsize=9)

        ax.set_xlabel('遗忘率 (%)', fontsize=12)
        ax.set_ylabel('副作用率 (%)', fontsize=12)
        ax.set_title('机器遗忘效果分析（遗忘率 vs 副作用率）',
                    fontsize=14, pad=20)
        ax.set_xlim(0, 100)
        ax.set_ylim(0, max(side_effect_rates + [10]))
        ax.grid(True, alpha=0.3)

        return self._generate_chart_base64(fig)

    def _generate_full_report(self, results: Dict) -> str:
        """生成完整报告（5章节 + 4张关键图表）"""

        # 生成4张图表
        print("[INFO] 正在生成图表...")
        radar_chart = ""
        pie_chart = ""
        bar_chart = ""
        scatter_plot = ""

        if results.get('security'):
            security_metrics = results['security'].get('aggregated_metrics', {})
            security_config = self._security_config_name(results['security'])
            radar_chart = self._generate_radar_chart(security_metrics, security_config)
            pie_chart = self._generate_pie_chart(security_metrics, security_config)

        if results.get('causal') or results.get('retrieval'):
            bar_chart = self._generate_bar_chart(results)

        if results.get('unlearning'):
            unlearning_data = results['unlearning'].get('detailed_results', [])
            scatter_plot = self._generate_scatter_plot(unlearning_data)

        print("[INFO] 图表生成完成")

        # 图表section
        charts_html = f"""
    <div class="section">
        <h2>📊 关键图表分析</h2>
        <p>图表仅展示通过原始结果和运行溯源校验的实测值。</p>

        <h3>1. 安全防护雷达图</h3>
        <p>单个合格配置在6类攻击上的 ASR（攻击成功率）。</p>
        {f'<img src="{radar_chart}" style="max-width:100%; border-radius:10px; box-shadow:0 2px 10px rgba(0,0,0,0.1); margin:20px 0;" alt="安全防护雷达图" />' if radar_chart else self._unavailable_chart_text('security')}

        <h3>2. 防御成功率分布（饼图）</h3>
        <p>同一合格配置在各类攻击上的防御成功率分布。</p>
        {f'<img src="{pie_chart}" style="max-width:100%; border-radius:10px; box-shadow:0 2px 10px rgba(0,0,0,0.1); margin:20px 0;" alt="防御成功率饼图" />' if pie_chart else self._unavailable_chart_text('security')}

        <h3>3. 因果推理与检索融合实测值（柱状图）</h3>
        <p>左图：因果推理实测准确率 | 右图：检索融合实测 MRR</p>
        {f'<img src="{bar_chart}" style="max-width:100%; border-radius:10px; box-shadow:0 2px 10px rgba(0,0,0,0.1); margin:20px 0;" alt="准确率柱状图" />' if bar_chart else self._unavailable_chart_text('causal') + self._unavailable_chart_text('retrieval')}

        <h3>4. 机器遗忘效果散点图</h3>
        <p>合格结果中各场景的遗忘率与副作用率分布。</p>
        {f'<img src="{scatter_plot}" style="max-width:100%; border-radius:10px; box-shadow:0 2px 10px rgba(0,0,0,0.1); margin:20px 0;" alt="遗忘效果散点图" />' if scatter_plot else self._unavailable_chart_text('unlearning')}
    </div>
"""

        html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Context-Keeper 完整实验报告</title>
    <style>
        body {{
            font-family: "Microsoft YaHei", Arial, sans-serif;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }}
        .header {{
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 30px;
        }}
        .section {{
            background: white;
            padding: 30px;
            margin-bottom: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }}
        h1 {{ margin: 0; font-size: 32px; }}
        h2 {{ color: #667eea; border-bottom: 3px solid #667eea; padding-bottom: 10px; }}
        h3 {{ color: #764ba2; }}
        table {{
            width: 100%;
            border-collapse: collapse;
            margin: 20px 0;
        }}
        th, td {{
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }}
        th {{
            background: #667eea;
            color: white;
        }}
        .metric {{
            display: inline-block;
            padding: 10px 20px;
            margin: 10px;
            background: #f0f0f0;
            border-radius: 5px;
        }}
        .metric-value {{
            font-size: 24px;
            font-weight: bold;
            color: #667eea;
        }}
        .metric-label {{
            font-size: 14px;
            color: #666;
        }}
        .footer {{
            text-align: center;
            color: #666;
            margin-top: 40px;
            padding: 20px;
        }}
    </style>
</head>
<body>
    <div class="header">
        <h1>Context-Keeper 实验报告</h1>
        <p>生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</p>
        <p>AI Agent安全治理平台 - 完整评估报告</p>
    </div>

    {charts_html}
    {self._section_overview(results)}
    {self._section_security(results.get('security'))}
    {self._section_causal(results.get('causal'))}
    {self._section_retrieval(results.get('retrieval'))}
    {self._section_unlearning(results.get('unlearning'))}

    <div class="footer">
        <p>Context-Keeper - AI Agent Security Platform</p>
        <p>本报告由 report_builder.py 自动生成</p>
    </div>
</body>
</html>
"""
        return html

    def _section_overview(self, results: Dict) -> str:
        """章节1：数据集说明"""
        rows = ""
        labels = {
            "security": "安全防护",
            "causal": "因果推理",
            "retrieval": "检索融合",
            "unlearning": "机器遗忘",
        }
        for experiment, label in labels.items():
            if results.get(experiment):
                source = self.selected_results.get(experiment, "已验证原始结果")
                status = f"已纳入报告（{source}）"
            else:
                reason = self.rejected_results.get(experiment, ["未找到可验证的原始结果"])[0]
                status = f"未达到报告门槛：{reason}"
            rows += f"<tr><td>{label}</td><td>{status}</td></tr>"

        return f"""
    <div class="section">
        <h2>1. 报告数据状态</h2>
        <table>
            <tr>
                <th>实验类型</th>
                <th>验证状态</th>
            </tr>
            {rows}
        </table>
    </div>
"""

    def _section_security(self, data: Dict) -> str:
        """章节2：安全防护评估"""
        if not data:
            return self._unavailable_section(2, "安全防护评估", "security")

        metrics = data.get("aggregated_metrics", {})
        config_name = self._security_config_name(data)
        asr = metrics.get("attack_success_rate", 0) * 100
        defense = metrics.get("defense_success_rate", 0) * 100
        latency = metrics.get("avg_latency_ms", 0)
        benign_pass = metrics.get("benign_pass_rate")
        false_positive = metrics.get("false_positive_rate")
        benign_evidence = (
            f"<p>Benign pass rate: {benign_pass * 100:.1f}% | False-positive rate: {false_positive * 100:.1f}%</p>"
            if isinstance(benign_pass, (int, float)) and isinstance(false_positive, (int, float))
            else ""
        )

        return f"""
    <div class="section">
        <h2>2. 安全防护评估</h2>
        <h3>核心指标</h3>
        <table>
            <tr>
                <th>配置</th>
                <th>ASR（攻击成功率）</th>
                <th>防御成功率</th>
                <th>平均延迟</th>
            </tr>
            <tr>
                <td>{config_name}</td>
                <td>{asr:.1f}%</td>
                <td>{defense:.1f}%</td>
                <td>{latency:.1f}ms</td>
            </tr>
        </table>
        {benign_evidence}
    </div>
"""

    def _section_causal(self, data: Dict) -> str:
        """章节3：因果推理评估"""
        if not data:
            return self._unavailable_section(3, "因果推理评估", "causal")

        metrics = data.get("metrics", {})
        run_label = data.get("run_label", "observed_run")
        accuracy = metrics.get("tuple_accuracy", 0) * 100
        confidence = metrics.get("avg_best_relation_confidence", 0)
        latency = metrics.get("avg_latency_ms", 0)

        return f"""
    <div class="section">
        <h2>3. 因果推理评估</h2>
        <h3>核心指标</h3>
        <table>
            <tr>
                <th>配置</th>
                <th>准确率</th>
                <th>平均置信度</th>
                <th>平均延迟</th>
            </tr>
            <tr>
                <td>{run_label}</td>
                <td>{accuracy:.1f}%</td>
                <td>{confidence:.3f}</td>
                <td>{latency:.1f}ms</td>
            </tr>
        </table>
    </div>
"""

    def _section_retrieval(self, data: Dict) -> str:
        """章节4：检索融合评估"""
        if not data:
            return self._unavailable_section(4, "检索融合评估", "retrieval")

        metrics = data.get("aggregated_metrics", {})

        rows = ""
        for config_name, m in metrics.items():
            mrr = m.get("mrr", 0)
            precision = m.get("precision_at_5", 0)
            recall = m.get("recall_at_5", 0)
            latency = m.get("avg_latency_ms", 0)
            p95_latency = m.get("p95_latency_ms")
            latency_text = f"{latency:.1f}ms" if not isinstance(p95_latency, (int, float)) else f"{latency:.1f}ms / {p95_latency:.1f}ms"

            rows += f"""
            <tr>
                <td>{config_name}</td>
                <td>{mrr:.3f}</td>
                <td>{precision:.3f}</td>
                <td>{recall:.3f}</td>
                <td>{latency_text}</td>
            </tr>
"""

        return f"""
    <div class="section">
        <h2>4. 检索融合评估</h2>
        <h3>核心指标</h3>
        <table>
            <tr>
                <th>配置</th>
                <th>MRR</th>
                <th>Precision@5</th>
                <th>Recall@5</th>
                <th>平均 / P95延迟</th>
            </tr>
            {rows}
        </table>
    </div>
"""

    def _section_unlearning(self, data: Dict) -> str:
        """章节5：机器遗忘评估"""
        if not data:
            return self._unavailable_section(5, "机器遗忘评估", "unlearning")

        if data.get("schema_version") == 3 and isinstance(data.get("trials"), list):
            rows = ""
            for trial in data["trials"]:
                before = trial.get("target_pre_count", {})
                after = trial.get("target_post_count", {})
                control_ok = trial.get("control_pre_count") == trial.get("control_post_count")
                rows += f"""
                <tr>
                    <td>Trial {trial.get('trial', '')}</td>
                    <td>Qdrant {before.get('qdrant', 0)} to {after.get('qdrant', '')}; TimescaleDB {before.get('timescaledb', 0)} to {after.get('timescaledb', '')}; Neo4j {before.get('neo4j', 0)} to {after.get('neo4j', '')}</td>
                    <td>{'unchanged' if control_ok else 'changed'}</td>
                    <td>{'passed' if trial.get('retrieval_probe', {}).get('passed') else 'failed'}</td>
                </tr>
                """
            return f"""
            <div class="section">
                <h2>5. Machine Unlearning Evaluation</h2>
                <h3>Three independent cross-store trials</h3>
                <table><tr><th>Trial</th><th>Target storage counts</th><th>Control session</th><th>Retrieval probe</th></tr>{rows}</table>
            </div>
            """

        results = data.get("detailed_results", [])

        rows = ""
        for r in results:
            scenario = r.get("scenario_name", "")
            unlearning_rate = r.get("unlearning_rate", 0) * 100
            side_effect = r.get("side_effect_rate", 0) * 100
            time_ms = r.get("unlearning_time_ms", 0)

            rows += f"""
            <tr>
                <td>{scenario}</td>
                <td>{unlearning_rate:.1f}%</td>
                <td>{side_effect:.1f}%</td>
                <td>{time_ms:.0f}ms</td>
            </tr>
"""

        return f"""
    <div class="section">
        <h2>5. 机器遗忘评估</h2>
        <h3>场景测试结果</h3>
        <table>
            <tr>
                <th>场景</th>
                <th>遗忘率</th>
                <th>副作用率</th>
                <th>耗时</th>
            </tr>
            {rows}
        </table>
    </div>
"""

    def _generate_demo_slides(self, results: Dict) -> str:
        """生成演示slides（4页）"""
        security = results.get("security")
        if security:
            security_metrics = security.get("aggregated_metrics", {})
            security_text = (
                f"{self._security_config_name(security)}：ASR "
                f"{security_metrics.get('attack_success_rate', 0) * 100:.1f}%，防御成功率 "
                f"{security_metrics.get('defense_success_rate', 0) * 100:.1f}%"
            )
        else:
            security_text = self._unavailable_slide_text("security")

        causal = results.get("causal")
        causal_text = (
            f"元组准确率 {causal.get('metrics', {}).get('tuple_accuracy', 0) * 100:.1f}%"
            if causal else self._unavailable_slide_text("causal")
        )
        retrieval = results.get("retrieval")
        if retrieval:
            retrieval_metrics = retrieval.get("aggregated_metrics", {})
            observed_mrr = next(
                (metric.get("mrr") for metric in retrieval_metrics.values() if isinstance(metric.get("mrr"), (int, float))),
                None,
            )
            retrieval_text = f"MRR {observed_mrr:.3f}" if observed_mrr is not None else "未达到报告门槛：缺少 MRR 实测值"
        else:
            retrieval_text = self._unavailable_slide_text("retrieval")

        unlearning = results.get("unlearning")
        unlearning_text = (
            f"{len(unlearning.get('detailed_results', []))} 个实测场景"
            if unlearning else self._unavailable_slide_text("unlearning")
        )

        html = f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>Context-Keeper 快速演示</title>
    <style>
        body {{
            font-family: "Microsoft YaHei", Arial, sans-serif;
            margin: 0;
            padding: 0;
            background: #000;
        }}
        .slide {{
            width: 100vw;
            height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            color: white;
            page-break-after: always;
        }}
        .slide1 {{ background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }}
        .slide2 {{ background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%); }}
        .slide3 {{ background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%); }}
        .slide4 {{ background: linear-gradient(135deg, #43e97b 0%, #38f9d7 100%); }}
        h1 {{ font-size: 64px; margin: 20px; }}
        h2 {{ font-size: 48px; margin: 10px; }}
        p {{ font-size: 24px; margin: 10px; }}
        .metric-big {{
            font-size: 72px;
            font-weight: bold;
            margin: 20px;
        }}
    </style>
</head>
<body>
    <div class="slide slide1">
        <h1>Context-Keeper</h1>
        <h2>AI Agent安全治理平台</h2>
        <p>实验实测结果概览</p>
        <p>生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M')}</p>
    </div>

    <div class="slide slide2">
        <h2>安全防护</h2>
        <p class="metric-big">{security_text}</p>
    </div>

    <div class="slide slide3">
        <h2>因果推理 + 检索融合</h2>
        <p class="metric-big">{causal_text}</p>
        <p class="metric-big">{retrieval_text}</p>
    </div>

    <div class="slide slide4">
        <h2>机器遗忘</h2>
        <p class="metric-big">{unlearning_text}</p>
    </div>
</body>
</html>
"""
        return html


def main():
    parser = argparse.ArgumentParser(description="生成实验报告")
    parser.add_argument("--mode", default="full", choices=["full", "demo"],
                       help="报告模式：full完整报告，demo演示slides")
    parser.add_argument("--output", help="输出文件路径")
    parser.add_argument("--require", nargs="*", choices=["security", "causal", "retrieval", "unlearning"],
                       default=[], help="Fail instead of publishing when any named experiment has no eligible result.")

    args = parser.parse_args()

    builder = ReportBuilder(mode=args.mode)

    # 加载结果
    print("[LOAD] 加载实验结果...")
    results = builder.load_latest_results()
    missing = [experiment for experiment in args.require if results.get(experiment) is None]
    if missing:
        raise SystemExit("[FAIL] Required eligible results are missing: " + ", ".join(missing))
    if not any(results.values()):
        raise SystemExit("[FAIL] No report-eligible result artifacts were found.")

    # 确定输出文件
    if args.output:
        output_file = Path(args.output)
    else:
        if args.mode == "full":
            output_file = builder.reports_dir / "full_report.html"
        else:
            output_file = builder.reports_dir / "demo_slides.html"

    # 生成报告
    print(f"[GEN] 生成{args.mode}报告...")
    builder.generate_html_report(results, output_file)

    print(f"[DONE] 报告已生成: {output_file}")


if __name__ == "__main__":
    main()
