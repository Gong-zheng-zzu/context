#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
因果推理性能测试脚本
测试Context-Keeper因果推理API的响应时间、并发能力、吞吐量
"""

import json
import time
import requests
import random
import threading
import statistics
from pathlib import Path
from typing import Dict, List, Tuple
from dataclasses import dataclass, asdict
from datetime import datetime
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed

@dataclass
class PerformanceTestResult:
    """性能测试结果"""
    test_type: str  # single, batch, concurrent, inference
    total_requests: int
    successful_requests: int
    failed_requests: int
    avg_response_time_ms: float
    min_response_time_ms: float
    max_response_time_ms: float
    median_response_time_ms: float
    p95_response_time_ms: float
    p99_response_time_ms: float
    throughput_qps: float
    total_duration_seconds: float
    error_rate: float

class CausalReasoningPerformanceTest:
    """因果推理性能测试"""

    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.extract_endpoint = f"{base_url}/api/v1/causal/extract"
        self.infer_endpoint = f"{base_url}/api/v1/causal/infer"
        self.stats_endpoint = f"{base_url}/api/v1/causal/stats"

        # 数据路径
        self.datasets_dir = Path(__file__).parent.parent / "datasets"
        self.nursing_data_path = self.datasets_dir / "nursing_data" / "nursing_records.json"
        self.output_dir = Path(__file__).parent / "results"
        self.output_dir.mkdir(parents=True, exist_ok=True)

        # 测试数据缓存
        self.test_samples: List[str] = []

    def load_test_samples(self, count: int = 1000) -> List[str]:
        """加载测试样本"""
        if self.test_samples:
            return self.test_samples

        print(f"[LOAD] Loading {count} test samples...")

        if not self.nursing_data_path.exists():
            print(f"[ERROR] Nursing data file not found: {self.nursing_data_path}")
            sys.exit(1)

        with open(self.nursing_data_path, 'r', encoding='utf-8') as f:
            all_records = json.load(f)

        # 随机抽取样本
        samples = random.sample(all_records, min(count, len(all_records)))
        self.test_samples = [record.get("content", "") for record in samples if record.get("content")]

        print(f"[LOAD] Loaded {len(self.test_samples)} samples")
        return self.test_samples

    def test_single_request_latency(self, sample_count: int = 100) -> PerformanceTestResult:
        """测试单条记录抽取响应时间（目标<2秒）"""
        print(f"\n{'='*60}")
        print("测试1: 单条记录抽取响应时间")
        print(f"{'='*60}")
        print(f"目标: 平均响应时间 < 2000ms")
        print(f"样本数: {sample_count}\n")

        samples = self.load_test_samples(sample_count)
        response_times = []
        successful = 0
        failed = 0

        start_time = time.time()

        for i, text in enumerate(samples[:sample_count], 1):
            print(f"\r[{i}/{sample_count}] Testing...", end="", flush=True)

            req_start = time.time()
            try:
                response = requests.post(
                    self.extract_endpoint,
                    json={"text": text, "min_confidence": 0.5},
                    timeout=30
                )
                elapsed_ms = (time.time() - req_start) * 1000

                if response.status_code == 200:
                    successful += 1
                    response_times.append(elapsed_ms)
                else:
                    failed += 1

            except Exception as e:
                failed += 1

            # 避免过载
            time.sleep(0.05)

        total_duration = time.time() - start_time
        print()  # 换行

        if not response_times:
            print("[ERROR] All requests failed")
            return self._create_empty_result("single_request")

        return PerformanceTestResult(
            test_type="single_request",
            total_requests=sample_count,
            successful_requests=successful,
            failed_requests=failed,
            avg_response_time_ms=statistics.mean(response_times),
            min_response_time_ms=min(response_times),
            max_response_time_ms=max(response_times),
            median_response_time_ms=statistics.median(response_times),
            p95_response_time_ms=statistics.quantiles(response_times, n=20)[18] if len(response_times) >= 20 else max(response_times),
            p99_response_time_ms=statistics.quantiles(response_times, n=100)[98] if len(response_times) >= 100 else max(response_times),
            throughput_qps=successful / total_duration,
            total_duration_seconds=total_duration,
            error_rate=failed / sample_count
        )

    def test_concurrent_requests(self, concurrency_levels: List[int] = [10, 50, 100]) -> Dict[int, PerformanceTestResult]:
        """测试并发能力（目标支持100并发）"""
        print(f"\n{'='*60}")
        print("测试2: 并发处理能力")
        print(f"{'='*60}")
        print(f"目标: 支持100并发，平均响应时间 < 3000ms")
        print(f"并发级别: {concurrency_levels}\n")

        results = {}
        samples = self.load_test_samples(max(concurrency_levels) * 2)

        for concurrency in concurrency_levels:
            print(f"\n[TEST] Testing {concurrency} concurrent requests...")

            response_times = []
            successful = 0
            failed = 0

            def make_request(text: str) -> Tuple[bool, float]:
                req_start = time.time()
                try:
                    response = requests.post(
                        self.extract_endpoint,
                        json={"text": text, "min_confidence": 0.5},
                        timeout=30
                    )
                    elapsed_ms = (time.time() - req_start) * 1000
                    return (response.status_code == 200, elapsed_ms)
                except:
                    elapsed_ms = (time.time() - req_start) * 1000
                    return (False, elapsed_ms)

            start_time = time.time()

            with ThreadPoolExecutor(max_workers=concurrency) as executor:
                futures = [executor.submit(make_request, text) for text in samples[:concurrency]]

                for future in as_completed(futures):
                    success, elapsed_ms = future.result()
                    if success:
                        successful += 1
                        response_times.append(elapsed_ms)
                    else:
                        failed += 1

            total_duration = time.time() - start_time

            if response_times:
                result = PerformanceTestResult(
                    test_type=f"concurrent_{concurrency}",
                    total_requests=concurrency,
                    successful_requests=successful,
                    failed_requests=failed,
                    avg_response_time_ms=statistics.mean(response_times),
                    min_response_time_ms=min(response_times),
                    max_response_time_ms=max(response_times),
                    median_response_time_ms=statistics.median(response_times),
                    p95_response_time_ms=statistics.quantiles(response_times, n=20)[18] if len(response_times) >= 20 else max(response_times),
                    p99_response_time_ms=max(response_times),
                    throughput_qps=successful / total_duration,
                    total_duration_seconds=total_duration,
                    error_rate=failed / concurrency
                )
                results[concurrency] = result

                print(f"  - 成功: {successful}/{concurrency}")
                print(f"  - 平均响应时间: {result.avg_response_time_ms:.0f}ms")
                print(f"  - 吞吐量: {result.throughput_qps:.2f} QPS")

        return results

    def test_batch_throughput(self, batch_size: int = 1000) -> PerformanceTestResult:
        """测试批量处理吞吐量（目标1000条<30分钟）"""
        print(f"\n{'='*60}")
        print("测试3: 批量处理吞吐量")
        print(f"{'='*60}")
        print(f"目标: 1000条记录 < 30分钟 (>0.56 QPS)")
        print(f"批量大小: {batch_size}\n")

        samples = self.load_test_samples(batch_size)
        response_times = []
        successful = 0
        failed = 0

        start_time = time.time()

        for i, text in enumerate(samples[:batch_size], 1):
            if i % 50 == 0:
                elapsed = time.time() - start_time
                qps = i / elapsed
                eta_seconds = (batch_size - i) / qps if qps > 0 else 0
                print(f"\r[{i}/{batch_size}] 进度: {i/batch_size*100:.1f}% | QPS: {qps:.2f} | ETA: {eta_seconds/60:.1f}分钟", end="", flush=True)

            req_start = time.time()
            try:
                response = requests.post(
                    self.extract_endpoint,
                    json={"text": text, "min_confidence": 0.5},
                    timeout=30
                )
                elapsed_ms = (time.time() - req_start) * 1000

                if response.status_code == 200:
                    successful += 1
                    response_times.append(elapsed_ms)
                else:
                    failed += 1

            except:
                failed += 1

            # 小延迟避免过载
            time.sleep(0.02)

        total_duration = time.time() - start_time
        print()  # 换行

        if not response_times:
            print("[ERROR] All requests failed")
            return self._create_empty_result("batch_throughput")

        return PerformanceTestResult(
            test_type="batch_throughput",
            total_requests=batch_size,
            successful_requests=successful,
            failed_requests=failed,
            avg_response_time_ms=statistics.mean(response_times),
            min_response_time_ms=min(response_times),
            max_response_time_ms=max(response_times),
            median_response_time_ms=statistics.median(response_times),
            p95_response_time_ms=statistics.quantiles(response_times, n=20)[18] if len(response_times) >= 20 else max(response_times),
            p99_response_time_ms=statistics.quantiles(response_times, n=100)[98] if len(response_times) >= 100 else max(response_times),
            throughput_qps=successful / total_duration,
            total_duration_seconds=total_duration,
            error_rate=failed / batch_size
        )

    def test_inference_query(self, query_count: int = 100) -> PerformanceTestResult:
        """测试推理查询响应时间（目标<1秒）"""
        print(f"\n{'='*60}")
        print("测试4: 推理查询响应时间")
        print(f"{'='*60}")
        print(f"目标: 平均响应时间 < 1000ms")
        print(f"查询数: {query_count}\n")

        # 测试查询
        test_queries = [
            {"entity": "李奶奶", "relation_type": "causes"},
            {"entity": "血压升高", "relation_type": "effects"},
            {"entity": "跌倒", "relation_type": "causes"},
            {"entity": "降压药", "relation_type": "effects"},
            {"entity": "头晕", "relation_type": "both"}
        ]

        response_times = []
        successful = 0
        failed = 0

        start_time = time.time()

        for i in range(query_count):
            query = random.choice(test_queries)
            print(f"\r[{i+1}/{query_count}] Testing inference query...", end="", flush=True)

            req_start = time.time()
            try:
                response = requests.post(
                    self.infer_endpoint,
                    json=query,
                    timeout=10
                )
                elapsed_ms = (time.time() - req_start) * 1000

                if response.status_code == 200:
                    successful += 1
                    response_times.append(elapsed_ms)
                else:
                    failed += 1

            except:
                failed += 1

            time.sleep(0.05)

        total_duration = time.time() - start_time
        print()  # 换行

        if not response_times:
            print("[ERROR] All requests failed")
            return self._create_empty_result("inference_query")

        return PerformanceTestResult(
            test_type="inference_query",
            total_requests=query_count,
            successful_requests=successful,
            failed_requests=failed,
            avg_response_time_ms=statistics.mean(response_times),
            min_response_time_ms=min(response_times),
            max_response_time_ms=max(response_times),
            median_response_time_ms=statistics.median(response_times),
            p95_response_time_ms=statistics.quantiles(response_times, n=20)[18] if len(response_times) >= 20 else max(response_times),
            p99_response_time_ms=max(response_times),
            throughput_qps=successful / total_duration,
            total_duration_seconds=total_duration,
            error_rate=failed / query_count
        )

    def _create_empty_result(self, test_type: str) -> PerformanceTestResult:
        """创建空结果（用于失败情况）"""
        return PerformanceTestResult(
            test_type=test_type,
            total_requests=0,
            successful_requests=0,
            failed_requests=0,
            avg_response_time_ms=0.0,
            min_response_time_ms=0.0,
            max_response_time_ms=0.0,
            median_response_time_ms=0.0,
            p95_response_time_ms=0.0,
            p99_response_time_ms=0.0,
            throughput_qps=0.0,
            total_duration_seconds=0.0,
            error_rate=1.0
        )

    def generate_report(self,
                       single_result: PerformanceTestResult,
                       concurrent_results: Dict[int, PerformanceTestResult],
                       batch_result: PerformanceTestResult,
                       inference_result: PerformanceTestResult) -> str:
        """生成Markdown格式报告"""

        report = f"""# 因果推理性能测试报告

## 测试信息

- **测试时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
- **API端点**: {self.base_url}
- **测试类型**: 单条请求、并发处理、批量吞吐、推理查询

## 测试1: 单条记录抽取响应时间

**目标**: 平均响应时间 < 2000ms

| 指标 | 数值 | 目标值 | 达标情况 |
|------|------|--------|----------|
| 总请求数 | {single_result.total_requests} | - | - |
| 成功率 | {(1-single_result.error_rate)*100:.1f}% | ≥95% | {'[OK]' if single_result.error_rate <= 0.05 else '[X]'} |
| **平均响应时间** | {single_result.avg_response_time_ms:.0f} ms | <2000ms | {'[OK] 达标' if single_result.avg_response_time_ms < 2000 else '[X] 未达标'} |
| 中位数响应时间 | {single_result.median_response_time_ms:.0f} ms | - | - |
| P95响应时间 | {single_result.p95_response_time_ms:.0f} ms | - | - |
| P99响应时间 | {single_result.p99_response_time_ms:.0f} ms | - | - |
| 最快/最慢 | {single_result.min_response_time_ms:.0f} / {single_result.max_response_time_ms:.0f} ms | - | - |
| 吞吐量 | {single_result.throughput_qps:.2f} QPS | - | - |

## 测试2: 并发处理能力

**目标**: 支持100并发，平均响应时间 < 3000ms

"""

        for concurrency, result in sorted(concurrent_results.items()):
            target_met = "[OK] 达标" if result.avg_response_time_ms < 3000 else "[X] 未达标"
            report += f"""
### {concurrency}并发

| 指标 | 数值 |
|------|------|
| 成功率 | {(1-result.error_rate)*100:.1f}% ({result.successful_requests}/{result.total_requests}) |
| 平均响应时间 | {result.avg_response_time_ms:.0f} ms {target_met} |
| P95响应时间 | {result.p95_response_time_ms:.0f} ms |
| 吞吐量 | {result.throughput_qps:.2f} QPS |
| 总耗时 | {result.total_duration_seconds:.1f}秒 |

"""

        batch_minutes = batch_result.total_duration_seconds / 60
        batch_target = 30  # 30分钟
        batch_target_met = "[OK] 达标" if batch_minutes < batch_target else "[X] 未达标"

        report += f"""
## 测试3: 批量处理吞吐量

**目标**: 1000条记录 < 30分钟

| 指标 | 数值 | 目标值 | 达标情况 |
|------|------|--------|----------|
| 总处理数 | {batch_result.total_requests} | 1000 | - |
| 成功数 | {batch_result.successful_requests} | - | - |
| **总耗时** | {batch_minutes:.1f} 分钟 | <30分钟 | {batch_target_met} |
| 平均响应时间 | {batch_result.avg_response_time_ms:.0f} ms | - | - |
| 吞吐量 | {batch_result.throughput_qps:.2f} QPS | ≥0.56 QPS | {'[OK]' if batch_result.throughput_qps >= 0.56 else '[X]'} |

## 测试4: 推理查询响应时间

**目标**: 平均响应时间 < 1000ms

| 指标 | 数值 | 目标值 | 达标情况 |
|------|------|--------|----------|
| 总查询数 | {inference_result.total_requests} | - | - |
| 成功率 | {(1-inference_result.error_rate)*100:.1f}% | ≥95% | {'[OK]' if inference_result.error_rate <= 0.05 else '[X]'} |
| **平均响应时间** | {inference_result.avg_response_time_ms:.0f} ms | <1000ms | {'[OK] 达标' if inference_result.avg_response_time_ms < 1000 else '[X] 未达标'} |
| 中位数响应时间 | {inference_result.median_response_time_ms:.0f} ms | - | - |
| P95响应时间 | {inference_result.p95_response_time_ms:.0f} ms | - | - |
| 吞吐量 | {inference_result.throughput_qps:.2f} QPS | - | - |

## 结论与建议

"""

        issues = []

        if single_result.avg_response_time_ms >= 2000:
            issues.append("- ⚠️ 单条抽取响应时间超过2秒目标，需要优化抽取算法或LLM调用")

        if any(r.avg_response_time_ms >= 3000 for r in concurrent_results.values()):
            issues.append("- ⚠️ 高并发下响应时间过长，需要优化并发处理能力")

        if batch_minutes >= batch_target:
            issues.append("- ⚠️ 批量处理耗时过长，建议增加批处理优化或异步处理")

        if inference_result.avg_response_time_ms >= 1000:
            issues.append("- ⚠️ 推理查询响应时间超标，需要优化Neo4j查询或添加缓存")

        if issues:
            report += "**发现的问题**:\n\n" + "\n".join(issues) + "\n\n"
        else:
            report += "[OK] **所有性能指标均达标！**\n\n"

        report += """**优化建议**:

1. **响应时间优化**: 考虑添加缓存层，减少重复计算
2. **并发优化**: 使用连接池管理Neo4j和向量数据库连接
3. **批处理优化**: 实现批量API，一次调用处理多条记录
4. **查询优化**: 为Neo4j高频查询添加索引
5. **异步处理**: 对于大批量任务，使用消息队列异步处理

---

*本报告由 causal_reasoning_performance_test.py 自动生成*
"""

        return report

    def save_results(self,
                    single_result: PerformanceTestResult,
                    concurrent_results: Dict[int, PerformanceTestResult],
                    batch_result: PerformanceTestResult,
                    inference_result: PerformanceTestResult):
        """保存测试结果"""
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')

        # 保存详细结果(JSON)
        results_file = self.output_dir / f"performance_test_results_{timestamp}.json"
        with open(results_file, 'w', encoding='utf-8') as f:
            json.dump({
                "single_request": asdict(single_result),
                "concurrent": {k: asdict(v) for k, v in concurrent_results.items()},
                "batch_throughput": asdict(batch_result),
                "inference_query": asdict(inference_result)
            }, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] Detailed results saved to: {results_file}")

        # 保存报告(Markdown)
        report = self.generate_report(single_result, concurrent_results, batch_result, inference_result)
        report_file = self.output_dir / f"performance_test_report_{timestamp}.md"
        with open(report_file, 'w', encoding='utf-8') as f:
            f.write(report)

        print(f"[SAVE] Report saved to: {report_file}")

        return report_file

def main():
    """主函数"""
    import argparse

    parser = argparse.ArgumentParser(description='因果推理性能测试')
    parser.add_argument('--url', type=str, default='http://localhost:8080', help='API base URL')
    parser.add_argument('--quick', action='store_true', help='快速测试模式（小样本）')

    args = parser.parse_args()

    # 创建测试实例
    tester = CausalReasoningPerformanceTest(base_url=args.url)

    try:
        if args.quick:
            print("\n[MODE] 快速测试模式\n")
            single_result = tester.test_single_request_latency(sample_count=20)
            concurrent_results = tester.test_concurrent_requests(concurrency_levels=[10, 20])
            batch_result = tester.test_batch_throughput(batch_size=50)
            inference_result = tester.test_inference_query(query_count=20)
        else:
            print("\n[MODE] 完整测试模式\n")
            single_result = tester.test_single_request_latency(sample_count=100)
            concurrent_results = tester.test_concurrent_requests(concurrency_levels=[10, 50, 100])
            batch_result = tester.test_batch_throughput(batch_size=1000)
            inference_result = tester.test_inference_query(query_count=100)

        # 保存结果
        report_file = tester.save_results(single_result, concurrent_results, batch_result, inference_result)

        print(f"\n[OK] 测试完成！查看报告: {report_file}\n")

    except KeyboardInterrupt:
        print("\n\n[STOP] 测试被用户中断")
        sys.exit(0)
    except Exception as e:
        print(f"\n[ERROR] Test failed: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == '__main__':
    main()
