#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
因果推理准确率测试脚本
测试Context-Keeper因果推理API的四元组抽取准确性
"""

import json
import time
import requests
import random
from pathlib import Path
from typing import Dict, List, Tuple
from dataclasses import dataclass, asdict
from datetime import datetime
import sys

@dataclass
class AccuracyTestResult:
    """准确率测试结果"""
    record_id: str
    text: str
    has_ground_truth: bool
    extraction_success: bool
    api_response_time_ms: float
    relation_count: int
    avg_confidence: float
    error_message: str = ""

    # 四元组字段检查
    has_object: bool = False
    has_mediator: bool = False
    has_property: bool = False
    has_result: bool = False

    # 如果有ground_truth，计算匹配度
    match_score: float = 0.0

@dataclass
class AccuracyMetrics:
    """准确率指标"""
    total_samples: int
    successful_extractions: int
    failed_extractions: int
    success_rate: float
    avg_confidence: float
    avg_response_time_ms: float
    avg_relations_per_record: float

    # 字段完整性
    object_coverage: float  # 有object字段的比例
    mediator_coverage: float
    property_coverage: float
    result_coverage: float

    # 有ground_truth的样本评估
    matched_samples: int
    avg_match_score: float

class CausalReasoningAccuracyTest:
    """因果推理准确率测试"""

    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.api_endpoint = f"{base_url}/api/v1/causal/extract"
        self.test_results: List[AccuracyTestResult] = []

        # 数据路径
        self.datasets_dir = Path(__file__).parent.parent / "datasets"
        self.nursing_data_path = self.datasets_dir / "nursing_data" / "nursing_records.json"
        self.annotated_data_path = self.datasets_dir / "causal_reasoning" / "annotated_nursing_records.json"
        self.output_dir = Path(__file__).parent / "results"
        self.output_dir.mkdir(parents=True, exist_ok=True)

    def load_test_samples(self, sample_size: int = 100) -> List[Dict]:
        """加载测试样本"""
        samples = []

        # 优先加载标注数据
        if self.annotated_data_path.exists():
            print(f"[LOAD] Loading annotated data from {self.annotated_data_path}")
            with open(self.annotated_data_path, 'r', encoding='utf-8') as f:
                annotated_samples = json.load(f)
                samples.extend(annotated_samples)
                print(f"[LOAD] Loaded {len(annotated_samples)} annotated samples")

        # 如果标注数据不足，从护理记录中随机抽取
        if len(samples) < sample_size:
            if self.nursing_data_path.exists():
                print(f"[LOAD] Loading nursing records from {self.nursing_data_path}")
                with open(self.nursing_data_path, 'r', encoding='utf-8') as f:
                    all_records = json.load(f)

                needed = sample_size - len(samples)
                random_samples = random.sample(all_records, min(needed, len(all_records)))

                # 转换为统一格式
                for record in random_samples:
                    samples.append({
                        "record_id": record.get("elder_id", "") + "_" + record.get("timestamp", ""),
                        "text": record.get("content", ""),
                        "has_ground_truth": False
                    })

                print(f"[LOAD] Added {len(random_samples)} random samples from nursing records")
            else:
                print(f"[ERROR] Nursing data file not found: {self.nursing_data_path}")
                return []

        print(f"[LOAD] Total test samples: {len(samples)}")
        return samples[:sample_size]

    def call_extract_api(self, text: str, min_confidence: float = 0.5) -> Tuple[bool, Dict, float]:
        """调用因果关系抽取API"""
        start_time = time.time()

        try:
            response = requests.post(
                self.api_endpoint,
                json={
                    "text": text,
                    "use_llm": True,
                    "use_rules": True,
                    "use_pmi": True,
                    "min_confidence": min_confidence
                },
                timeout=30
            )

            elapsed_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                return True, response.json(), elapsed_ms
            else:
                return False, {"error": f"HTTP {response.status_code}: {response.text}"}, elapsed_ms

        except requests.exceptions.Timeout:
            elapsed_ms = (time.time() - start_time) * 1000
            return False, {"error": "Request timeout (30s)"}, elapsed_ms
        except Exception as e:
            elapsed_ms = (time.time() - start_time) * 1000
            return False, {"error": str(e)}, elapsed_ms

    def evaluate_extraction(self, sample: Dict) -> AccuracyTestResult:
        """评估单个样本的抽取结果"""
        record_id = sample.get("record_id", "unknown")
        text = sample.get("text", "")
        has_ground_truth = sample.get("has_ground_truth", False)
        ground_truth = sample.get("ground_truth", {})

        # 调用API
        success, response_data, elapsed_ms = self.call_extract_api(text)

        result = AccuracyTestResult(
            record_id=record_id,
            text=text[:100] + "..." if len(text) > 100 else text,
            has_ground_truth=has_ground_truth,
            extraction_success=success,
            api_response_time_ms=elapsed_ms,
            relation_count=0,
            avg_confidence=0.0
        )

        if not success:
            result.error_message = response_data.get("error", "Unknown error")
            return result

        # 解析返回的relations
        relations = response_data.get("relations", [])
        result.relation_count = len(relations)

        if len(relations) > 0:
            # 计算平均置信度
            confidences = [r.get("confidence", 0.0) for r in relations]
            result.avg_confidence = sum(confidences) / len(confidences)

            # 检查第一个relation的字段完整性
            first_relation = relations[0]
            result.has_object = bool(first_relation.get("object", ""))
            result.has_mediator = bool(first_relation.get("mediator", ""))
            result.has_property = bool(first_relation.get("property", ""))
            result.has_result = bool(first_relation.get("result", ""))

            # 如果有ground_truth，计算匹配分数
            if has_ground_truth and ground_truth:
                match_count = 0
                total_fields = 4

                if first_relation.get("object", "") == ground_truth.get("object", ""):
                    match_count += 1
                if first_relation.get("mediator", "") == ground_truth.get("mediator", ""):
                    match_count += 1
                if first_relation.get("property", "") == ground_truth.get("property", ""):
                    match_count += 1
                if first_relation.get("result", "") == ground_truth.get("result", ""):
                    match_count += 1

                result.match_score = match_count / total_fields

        return result

    def run_test(self, sample_size: int = 100) -> AccuracyMetrics:
        """运行准确率测试"""
        print(f"\n{'='*60}")
        print(f"因果推理准确率测试")
        print(f"{'='*60}")
        print(f"API Endpoint: {self.api_endpoint}")
        print(f"Sample Size: {sample_size}")
        print(f"{'='*60}\n")

        # 加载测试样本
        samples = self.load_test_samples(sample_size)
        if not samples:
            print("[ERROR] No test samples available")
            sys.exit(1)

        # 测试每个样本
        print(f"\n[TEST] Testing {len(samples)} samples...\n")

        for i, sample in enumerate(samples, 1):
            print(f"[{i}/{len(samples)}] Testing record: {sample.get('record_id', 'unknown')}...", end=" ")

            result = self.evaluate_extraction(sample)
            self.test_results.append(result)

            if result.extraction_success:
                print(f"[OK] Success ({result.relation_count} relations, {result.avg_confidence:.2f} confidence, {result.api_response_time_ms:.0f}ms)")
            else:
                print(f"[X] Failed: {result.error_message}")

            # 避免API过载
            time.sleep(0.1)

        # 计算指标
        metrics = self.calculate_metrics()
        return metrics

    def calculate_metrics(self) -> AccuracyMetrics:
        """计算准确率指标"""
        total = len(self.test_results)
        successful = sum(1 for r in self.test_results if r.extraction_success)
        failed = total - successful

        if total == 0:
            return AccuracyMetrics(
                total_samples=0,
                successful_extractions=0,
                failed_extractions=0,
                success_rate=0.0,
                avg_confidence=0.0,
                avg_response_time_ms=0.0,
                avg_relations_per_record=0.0,
                object_coverage=0.0,
                mediator_coverage=0.0,
                property_coverage=0.0,
                result_coverage=0.0,
                matched_samples=0,
                avg_match_score=0.0
            )

        # 只统计成功的样本
        successful_results = [r for r in self.test_results if r.extraction_success]

        if successful_results:
            avg_confidence = sum(r.avg_confidence for r in successful_results) / len(successful_results)
            avg_response_time = sum(r.api_response_time_ms for r in successful_results) / len(successful_results)
            avg_relations = sum(r.relation_count for r in successful_results) / len(successful_results)

            object_cov = sum(1 for r in successful_results if r.has_object) / len(successful_results)
            mediator_cov = sum(1 for r in successful_results if r.has_mediator) / len(successful_results)
            property_cov = sum(1 for r in successful_results if r.has_property) / len(successful_results)
            result_cov = sum(1 for r in successful_results if r.has_result) / len(successful_results)
        else:
            avg_confidence = 0.0
            avg_response_time = 0.0
            avg_relations = 0.0
            object_cov = mediator_cov = property_cov = result_cov = 0.0

        # 计算匹配分数（仅针对有ground_truth的样本）
        matched_results = [r for r in self.test_results if r.has_ground_truth and r.extraction_success]
        matched_samples = len(matched_results)
        avg_match_score = sum(r.match_score for r in matched_results) / matched_samples if matched_samples > 0 else 0.0

        return AccuracyMetrics(
            total_samples=total,
            successful_extractions=successful,
            failed_extractions=failed,
            success_rate=successful / total,
            avg_confidence=avg_confidence,
            avg_response_time_ms=avg_response_time,
            avg_relations_per_record=avg_relations,
            object_coverage=object_cov,
            mediator_coverage=mediator_cov,
            property_coverage=property_cov,
            result_coverage=result_cov,
            matched_samples=matched_samples,
            avg_match_score=avg_match_score
        )

    def generate_report(self, metrics: AccuracyMetrics) -> str:
        """生成Markdown格式报告"""
        report = f"""# 因果推理准确率测试报告

## 测试信息

- **测试时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
- **API端点**: {self.api_endpoint}
- **样本数量**: {metrics.total_samples}

## 整体指标

| 指标 | 数值 | 目标值 | 达标情况 |
|------|------|--------|----------|
| **成功率** | {metrics.success_rate*100:.1f}% ({metrics.successful_extractions}/{metrics.total_samples}) | ≥80% | {'[OK] 达标' if metrics.success_rate >= 0.8 else '[X] 未达标'} |
| **平均置信度** | {metrics.avg_confidence:.3f} | ≥0.7 | {'[OK] 达标' if metrics.avg_confidence >= 0.7 else '[X] 未达标'} |
| **平均响应时间** | {metrics.avg_response_time_ms:.0f} ms | <2000ms | {'[OK] 达标' if metrics.avg_response_time_ms < 2000 else '[X] 未达标'} |
| **平均关系数/记录** | {metrics.avg_relations_per_record:.2f} | ≥1 | {'[OK] 达标' if metrics.avg_relations_per_record >= 1 else '[X] 未达标'} |

## 四元组字段完整性

| 字段 | 覆盖率 |
|------|--------|
| Object (对象) | {metrics.object_coverage*100:.1f}% |
| Mediator (中介) | {metrics.mediator_coverage*100:.1f}% |
| Property (属性) | {metrics.property_coverage*100:.1f}% |
| Result (结果) | {metrics.result_coverage*100:.1f}% |

## Ground Truth匹配评估

"""

        if metrics.matched_samples > 0:
            report += f"""- **标注样本数**: {metrics.matched_samples}
- **平均匹配分数**: {metrics.avg_match_score*100:.1f}%

> 匹配分数计算方法：逐字段对比抽取结果与标注数据，完全匹配得1分，完全不匹配得0分。
"""
        else:
            report += "- **标注样本数**: 0 (无ground truth数据)\n\n"

        report += f"""
## 失败案例分析

- **失败数量**: {metrics.failed_extractions}
- **主要错误类型**:
"""

        # 统计错误类型
        error_counts = {}
        for result in self.test_results:
            if not result.extraction_success:
                error_msg = result.error_message
                error_counts[error_msg] = error_counts.get(error_msg, 0) + 1

        if error_counts:
            for error, count in sorted(error_counts.items(), key=lambda x: -x[1]):
                report += f"  - `{error}`: {count} 次\n"
        else:
            report += "  - 无失败案例\n"

        report += """
## 典型案例展示

### 成功案例 (Top 3 by confidence)

"""

        # 选择置信度最高的3个成功案例
        successful_cases = [r for r in self.test_results if r.extraction_success and r.relation_count > 0]
        successful_cases.sort(key=lambda x: x.avg_confidence, reverse=True)

        for i, case in enumerate(successful_cases[:3], 1):
            report += f"""**案例 {i}**

- **记录ID**: {case.record_id}
- **文本**: {case.text}
- **关系数**: {case.relation_count}
- **置信度**: {case.avg_confidence:.3f}
- **响应时间**: {case.api_response_time_ms:.0f}ms
- **字段完整性**: Object={'[OK]' if case.has_object else '[X]'} | Mediator={'[OK]' if case.has_mediator else '[X]'} | Property={'[OK]' if case.has_property else '[X]'} | Result={'[OK]' if case.has_result else '[X]'}

"""

        report += """
## 结论与建议

"""

        if metrics.success_rate >= 0.8:
            report += "[OK] **整体表现良好**：抽取成功率达到预期目标\n\n"
        else:
            report += "[X] **需要改进**：抽取成功率未达到80%目标\n\n"

        report += "**改进建议**:\n\n"

        if metrics.avg_confidence < 0.7:
            report += "1. 提升置信度计算方法，当前平均置信度偏低\n"

        if metrics.mediator_coverage < 0.5:
            report += "2. 中介(Mediator)字段覆盖率较低，需要优化O→C抽取逻辑\n"

        if metrics.avg_response_time_ms > 2000:
            report += "3. 响应时间超过2秒目标，需要性能优化\n"

        if metrics.failed_extractions > metrics.total_samples * 0.2:
            report += "4. 失败率较高，需要增强错误处理和API稳定性\n"

        report += "\n---\n\n*本报告由 causal_reasoning_accuracy_test.py 自动生成*\n"

        return report

    def save_results(self, metrics: AccuracyMetrics):
        """保存测试结果"""
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')

        # 保存详细结果(JSON)
        results_file = self.output_dir / f"accuracy_test_results_{timestamp}.json"
        with open(results_file, 'w', encoding='utf-8') as f:
            json.dump({
                "metrics": asdict(metrics),
                "test_results": [asdict(r) for r in self.test_results]
            }, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] Detailed results saved to: {results_file}")

        # 保存报告(Markdown)
        report = self.generate_report(metrics)
        report_file = self.output_dir / f"accuracy_test_report_{timestamp}.md"
        with open(report_file, 'w', encoding='utf-8') as f:
            f.write(report)

        print(f"[SAVE] Report saved to: {report_file}")

        return report_file

def main():
    """主函数"""
    import argparse

    parser = argparse.ArgumentParser(description='因果推理准确率测试')
    parser.add_argument('--url', type=str, default='http://localhost:8080', help='API base URL')
    parser.add_argument('--samples', type=int, default=100, help='测试样本数量')

    args = parser.parse_args()

    # 创建测试实例
    tester = CausalReasoningAccuracyTest(base_url=args.url)

    # 运行测试
    try:
        metrics = tester.run_test(sample_size=args.samples)

        # 打印结果摘要
        print(f"\n{'='*60}")
        print("测试结果摘要")
        print(f"{'='*60}")
        print(f"总样本数: {metrics.total_samples}")
        print(f"成功率: {metrics.success_rate*100:.1f}%")
        print(f"平均置信度: {metrics.avg_confidence:.3f}")
        print(f"平均响应时间: {metrics.avg_response_time_ms:.0f}ms")
        print(f"{'='*60}\n")

        # 保存结果
        report_file = tester.save_results(metrics)

        print(f"\n[OK] 测试完成！查看报告: {report_file}\n")

    except Exception as e:
        print(f"\n[ERROR] Test failed: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == '__main__':
    main()
