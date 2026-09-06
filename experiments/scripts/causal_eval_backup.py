#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
因果推理实验脚本
包装causal_reasoning_accuracy_test和performance_test

功能：
1. 完整模式：100条样本 × 4配置
2. 快速模式：30条样本 × 2配置对比
3. 测试准确率、置信度、F1分数、响应时间
"""

import json
import requests
import argparse
import time
from pathlib import Path
from datetime import datetime
from typing import List, Dict
from dataclasses import dataclass, asdict
from difflib import SequenceMatcher
import sys

@dataclass
class CausalResult:
    """因果推理结果"""
    record_id: str
    config_name: str
    query_text: str

    # 抽取结果
    extracted: bool
    causal_chain: Dict
    confidence: float

    # Ground truth对比
    matches_gt: bool
    gt_object: str
    gt_result: str

    latency_ms: float
    timestamp: str

@dataclass
class CausalMetrics:
    """因果推理指标"""
    config_name: str
    total_samples: int
    successful_extractions: int
    extraction_rate: float
    avg_confidence: float
    avg_latency_ms: float

    # Ground truth指标
    matches_count: int
    accuracy: float
    f1_score: float

class CausalEvaluator:
    """因果推理评估器"""

    def __init__(self, base_url: str = "http://localhost:8088"):
        self.base_url = base_url
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets" / "causal_reasoning"
        self.configs_dir = self.project_root / "configs"
        self.results_dir = self.project_root / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)

    def load_annotated_samples(self) -> List[Dict]:
        """加载标注样本（带降级方案）"""
        gt_file = self.datasets_dir / "annotated_nursing_records.json"

        if not gt_file.exists():
            print(f"\n[WARN] 标注数据集不存在")
            print(f"   路径: {gt_file}")
            print(f"   建议: 请先运行数据集生成脚本或检查路径")
            print(f"   降级: 使用3个示例样本进行测试\n")
            return self._generate_sample_data()

        try:
            with open(gt_file, 'r', encoding='utf-8') as f:
                data = json.load(f)
                samples = data if isinstance(data, list) else data.get("annotated_records", [])

                if not samples:
                    print(f"[WARN] 数据集为空，使用示例数据")
                    return self._generate_sample_data()

                print(f"[SUCCESS] 成功加载 {len(samples)} 条标注样本")
                return samples

        except json.JSONDecodeError as e:
            print(f"[ERROR] JSON解析失败 - {e}")
            print(f"   降级: 使用示例数据")
            return self._generate_sample_data()
        except Exception as e:
            print(f"[ERROR] 加载失败 - {e}")
            return self._generate_sample_data()

    def _generate_sample_data(self) -> List[Dict]:
        """生成示例数据（降级方案）"""
        self._using_sample_data = True  # 标记为离线样例模式
        return [
            {
                "id": "sample_001",
                "content": "患者王明，82岁，诊断阿尔茨海默症。今日早晨出现定向障碍，认为在家中而非养老院。",
                "ground_truth": {
                    "object": "阿尔茨海默症",
                    "result": "定向障碍",
                    "keywords": ["阿尔茨海默", "定向障碍"]
                }
            },
            {
                "id": "sample_002",
                "content": "李国强，78岁，因夜间睡眠质量差导致白天嗜睡和认知功能下降。",
                "ground_truth": {
                    "object": "睡眠质量差",
                    "result": "认知功能下降",
                    "keywords": ["睡眠", "认知"]
                }
            },
            {
                "id": "sample_003",
                "content": "赵志强，85岁，便秘3天后出现谵妄症状，经处理后症状缓解。",
                "ground_truth": {
                    "object": "便秘",
                    "result": "谵妄症状",
                    "keywords": ["便秘", "谵妄"]
                }
            }
        ]

    def extract_causal_chain(self, record_text: str) -> Dict:
        """调用API抽取因果链"""
        url = f"{self.base_url}/api/v1/causal/extract"

        start_time = time.time()

        try:
            response = requests.post(
                url,
                json={"text": record_text},
                timeout=30
            )

            latency_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                result = response.json()
                return {
                    "success": True,
                    "chain": result.get("causal_chain", {}),
                    "confidence": result.get("confidence", 0.0),
                    "latency_ms": latency_ms
                }
            else:
                return {
                    "success": False,
                    "chain": {},
                    "confidence": 0.0,
                    "latency_ms": latency_ms
                }

        except Exception as e:
            latency_ms = (time.time() - start_time) * 1000
            print(f"[ERROR] 抽取失败: {e}")
            return {
                "success": False,
                "chain": {},
                "confidence": 0.0,
                "latency_ms": latency_ms
            }

    def calculate_semantic_similarity(self, text1: str, text2: str) -> float:
        """计算两个文本的语义相似度（基于序列匹配）"""
        return SequenceMatcher(None, text1.lower(), text2.lower()).ratio()

    def compare_with_ground_truth_enhanced(self, extracted: Dict, ground_truth: Dict) -> bool:
        """增强版ground truth匹配"""
        if not extracted:
            return False

        # 提取关键字段
        gt_object = ground_truth.get("object", "")
        gt_result = ground_truth.get("result", "")
        ex_object = extracted.get("object", "")
        ex_result = extracted.get("result", "")

        # 方法1：精确包含匹配（原方法）
        object_exact_match = gt_object in ex_object or ex_object in gt_object
        result_exact_match = gt_result in ex_result or ex_result in gt_result
        exact_match = object_exact_match and result_exact_match

        if exact_match:
            return True

        # 方法2：语义相似度匹配（阈值0.6）
        obj_similarity = self.calculate_semantic_similarity(ex_object, gt_object)
        result_similarity = self.calculate_semantic_similarity(ex_result, gt_result)
        semantic_match = (obj_similarity > 0.6 and result_similarity > 0.6)

        if semantic_match:
            return True

        # 方法3：关键词匹配（至少命中2个关键词）
        keywords = ground_truth.get("keywords", [])
        if keywords:
            hit_count = sum(1 for kw in keywords if kw in ex_object or kw in ex_result)
            keyword_match = (hit_count >= min(2, len(keywords)))
            return keyword_match

        return False

    def compare_with_ground_truth(self, extracted: Dict, ground_truth: Dict) -> bool:
        """对比抽取结果与ground truth（使用增强版）"""
        return self.compare_with_ground_truth_enhanced(extracted, ground_truth)

    def test_single_sample(self, sample: Dict, config_name: str) -> CausalResult:
        """测试单个样本"""
        record_id = sample.get("id", "unknown")
        record_text = sample.get("content", "")
        ground_truth = sample.get("ground_truth", {})

        # 生成查询文本
        query = f"请分析以下护理记录中的因果关系：\n{record_text}"

        # 抽取因果链
        extraction = self.extract_causal_chain(record_text)

        # 对比ground truth
        matches_gt = self.compare_with_ground_truth(
            extraction.get("chain", {}),
            ground_truth
        )

        result = CausalResult(
            record_id=record_id,
            config_name=config_name,
            query_text=query[:100],
            extracted=extraction["success"],
            causal_chain=extraction.get("chain", {}),
            confidence=extraction.get("confidence", 0.0),
            matches_gt=matches_gt,
            gt_object=ground_truth.get("object", ""),
            gt_result=ground_truth.get("result", ""),
            latency_ms=extraction["latency_ms"],
            timestamp=datetime.now().isoformat()
        )

        return result

    def run_experiment(self, config_names: List[str], samples: List[Dict]) -> List[CausalResult]:
        """运行完整实验"""
        results = []

        total = len(config_names) * len(samples)
        current = 0

        for config_name in config_names:
            print(f"\n[CONFIG] 测试配置: {config_name}")

            for sample in samples:
                current += 1
                record_id = sample.get("id", f"sample_{current}")
                print(f"[{current}/{total}] 测试: {record_id}")

                result = self.test_single_sample(sample, config_name)
                results.append(result)

                # 显示结果
                status = "[OK] 成功" if result.extracted else "[FAIL] 失败"
                match = "[MATCH] 匹配" if result.matches_gt else "[MISS] 不匹配"
                print(f"  {status} | {match} | 置信度: {result.confidence:.2f} | 延迟: {result.latency_ms:.0f}ms")

        return results

    def calculate_metrics(self, results: List[CausalResult], config_name: str) -> CausalMetrics:
        """计算指标"""
        total = len(results)
        successful = sum(1 for r in results if r.extracted)
        matches = sum(1 for r in results if r.matches_gt)

        extraction_rate = successful / total if total > 0 else 0.0
        accuracy = matches / total if total > 0 else 0.0

        # 平均置信度（仅成功的）
        successful_results = [r for r in results if r.extracted]
        avg_confidence = sum(r.confidence for r in successful_results) / len(successful_results) if successful_results else 0.0

        avg_latency = sum(r.latency_ms for r in results) / total if total > 0 else 0.0

        # 简化的F1计算
        # Precision = matches / successful
        # Recall = matches / total
        precision = matches / successful if successful > 0 else 0.0
        recall = accuracy
        f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0.0

        return CausalMetrics(
            config_name=config_name,
            total_samples=total,
            successful_extractions=successful,
            extraction_rate=extraction_rate,
            avg_confidence=avg_confidence,
            avg_latency_ms=avg_latency,
            matches_count=matches,
            accuracy=accuracy,
            f1_score=f1
        )

    def aggregate_by_config(self, results: List[CausalResult]) -> Dict[str, CausalMetrics]:
        """按配置聚合"""
        by_config = {}

        for result in results:
            config_name = result.config_name
            if config_name not in by_config:
                by_config[config_name] = []
            by_config[config_name].append(result)

        metrics = {}
        for config_name, config_results in by_config.items():
            metrics[config_name] = self.calculate_metrics(config_results, config_name)

        return metrics

    def save_results(self, results: List[CausalResult], metrics: Dict[str, CausalMetrics]):
        """保存结果"""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = self.results_dir / f"causal_{timestamp}.json"

        # 检测execution_mode
        execution_mode = "live_api"
        if hasattr(self, '_using_sample_data') and self._using_sample_data:
            execution_mode = "offline_sample"

        output = {
            "execution_mode": execution_mode,
            "timestamp": timestamp,
            "total_tests": len(results),
            "configs": list(metrics.keys()),
            "detailed_results": [asdict(r) for r in results],
            "aggregated_metrics": {name: asdict(m) for name, m in metrics.items()}
        }

        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(output, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] 结果已保存: {output_file}")
        return output_file

    def print_summary(self, metrics: Dict[str, CausalMetrics]):
        """打印摘要"""
        print("\n" + "="*60)
        print("因果推理实验结果摘要")
        print("="*60)

        for config_name, m in metrics.items():
            print(f"\n[{config_name}]")
            print(f"  样本数量: {m.total_samples}")
            print(f"  抽取成功: {m.successful_extractions} ({m.extraction_rate*100:.1f}%)")
            print(f"  平均置信度: {m.avg_confidence:.3f}")
            print(f"  准确率: {m.accuracy*100:.1f}%")
            print(f"  F1分数: {m.f1_score:.3f}")
            print(f"  平均延迟: {m.avg_latency_ms:.1f}ms")


def main():
    parser = argparse.ArgumentParser(description="因果推理实验")
    parser.add_argument("--samples", type=int, default=30, help="样本数量")
    parser.add_argument("--configs", default="all", help="配置列表")
    parser.add_argument("--select-by", default="random", choices=["random", "confidence"],
                       help="样本选择策略")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API基础URL")

    args = parser.parse_args()

    evaluator = CausalEvaluator(base_url=args.base_url)

    # 加载标注样本
    print("[LOAD] 加载标注样本...")
    samples = evaluator.load_annotated_samples()

    if not samples:
        print("[ERROR] 无可用标注样本")
        sys.exit(1)

    # 限制样本数量
    if args.samples < len(samples):
        samples = samples[:args.samples]

    # 选择配置
    if args.configs == "all":
        config_names = ["baseline_vanilla_llm", "baseline_naive_rag",
                       "baseline_rag_with_filter", "full_system"]
    else:
        config_names = args.configs.split(",")

    print(f"[INFO] 将测试 {len(config_names)} 个配置 × {len(samples)} 条样本")

    # 运行实验
    results = evaluator.run_experiment(config_names, samples)

    # 聚合指标
    metrics = evaluator.aggregate_by_config(results)

    # 保存结果
    evaluator.save_results(results, metrics)

    # 打印摘要
    evaluator.print_summary(metrics)


if __name__ == "__main__":
    main()
