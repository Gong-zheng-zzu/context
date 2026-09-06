#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
检索融合实验脚本
验证三路检索融合（向量+知识图谱+时序）的优势

功能：
1. 完整模式：50对查询-答案对 × 4配置
2. 快速模式：10对查询 × 2配置对比
3. 交互模式：自定义查询，展示检索源
"""

import json
import requests
import argparse
import time
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Tuple
from dataclasses import dataclass, asdict
import sys

@dataclass
class RetrievalResult:
    """单次检索结果"""
    query_id: str
    query_text: str
    query_type: str
    config_name: str
    retrieved_docs: List[str]
    response_text: str
    latency_ms: float
    timestamp: str

    # 评估指标
    found_ground_truth: bool = False
    reciprocal_rank: float = 0.0  # 1/rank，用于计算MRR
    precision_at_5: float = 0.0
    recall_at_5: float = 0.0

@dataclass
class ConfigMetrics:
    """单个配置的聚合指标"""
    config_name: str
    total_queries: int
    avg_latency_ms: float

    # 核心指标
    mrr: float  # Mean Reciprocal Rank
    precision_at_5: float
    recall_at_5: float

    # 按查询类型细分
    temporal_mrr: float = 0.0
    causal_mrr: float = 0.0
    general_mrr: float = 0.0

class RetrievalEvaluator:
    """检索融合评估器"""

    def __init__(self, base_url: str = "http://localhost:8088"):
        self.base_url = base_url
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets"
        self.configs_dir = self.project_root / "configs"
        self.results_dir = self.project_root / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)

    def load_ground_truth(self) -> List[Dict]:
        """加载检索ground truth数据集（带降级方案）"""
        gt_file = self.datasets_dir / "retrieval_groundtruth" / "query_answer_pairs.json"

        if not gt_file.exists():
            print(f"\n[WARN] Ground truth文件不存在")
            print(f"   路径: {gt_file}")
            print(f"   降级: 生成10对基础查询\n")
            return self._generate_sample_queries()

        try:
            with open(gt_file, 'r', encoding='utf-8') as f:
                data = json.load(f)
                queries = data.get("queries", data) if isinstance(data, dict) else data

                if not queries:
                    print(f"[WARN] Ground truth为空，生成基础查询")
                    return self._generate_sample_queries()

                print(f"[SUCCESS] 加载了 {len(queries)} 对查询-答案")
                return queries

        except json.JSONDecodeError as e:
            print(f"[ERROR] JSON解析失败 - {e}")
            print(f"   降级: 生成基础查询")
            return self._generate_sample_queries()
        except Exception as e:
            print(f"[ERROR] 加载失败 - {e}")
            return self._generate_sample_queries()

    def _generate_sample_queries(self) -> List[Dict]:
        """生成示例查询（降级方案）"""
        self._using_sample_data = True  # 标记为离线样例模式
        return [
            {
                "query_id": "temporal_001",
                "query_type": "temporal",
                "question": "elder_025最近7天的血压变化趋势？",
                "ground_truth_docs": ["elder_025_*"],
                "expected_keywords": ["血压", "趋势", "7天"]
            },
            {
                "query_id": "temporal_002",
                "query_type": "temporal",
                "question": "elder_080跌倒事件的时间分布？",
                "ground_truth_docs": ["elder_080_*"],
                "expected_keywords": ["跌倒", "时间"]
            },
            {
                "query_id": "temporal_003",
                "query_type": "temporal",
                "question": "elder_025夜间谵妄发作记录汇总？",
                "ground_truth_docs": ["elder_025_*"],
                "expected_keywords": ["谵妄", "夜间"]
            },
            {
                "query_id": "causal_001",
                "query_type": "causal",
                "question": "elder_025血压升高与用药的因果关系？",
                "ground_truth_docs": ["elder_025_*"],
                "expected_keywords": ["血压", "用药", "因果"]
            },
            {
                "query_id": "causal_002",
                "query_type": "causal",
                "question": "elder_080睡眠质量对认知功能的影响？",
                "ground_truth_docs": ["elder_080_*"],
                "expected_keywords": ["睡眠", "认知"]
            },
            {
                "query_id": "causal_003",
                "query_type": "causal",
                "question": "elder_084情绪波动与环境温度的关联？",
                "ground_truth_docs": ["elder_084_*"],
                "expected_keywords": ["情绪", "温度"]
            },
            {
                "query_id": "general_001",
                "query_type": "general",
                "question": "elder_025的基本信息和诊断？",
                "ground_truth_docs": ["elder_025_*"],
                "expected_keywords": ["基本信息", "诊断"]
            },
            {
                "query_id": "general_002",
                "query_type": "general",
                "question": "所有患者的用药统计？",
                "ground_truth_docs": ["elder_*"],
                "expected_keywords": ["用药", "统计"]
            },
            {
                "query_id": "general_003",
                "query_type": "general",
                "question": "elder_020最近一次护理记录？",
                "ground_truth_docs": ["elder_020_*"],
                "expected_keywords": ["护理", "记录"]
            },
            {
                "query_id": "general_004",
                "query_type": "general",
                "question": "年龄>80岁的患者筛选？",
                "ground_truth_docs": ["elder_080_*", "elder_084_*"],
                "expected_keywords": ["年龄", "80岁"]
            }
        ]

    def _generate_basic_queries(self) -> List[Dict]:
        """从护理数据自动生成基础查询（已废弃，使用_generate_sample_queries）"""
        return self._generate_sample_queries()

    def load_config(self, config_name: str) -> Dict:
        """加载配置文件"""
        config_file = self.configs_dir / f"{config_name}.yaml"

        if not config_file.exists():
            print(f"[WARN] 配置文件不存在: {config_file}，使用默认配置")
            return {"name": config_name}

        import yaml
        with open(config_file, 'r', encoding='utf-8') as f:
            return yaml.safe_load(f)

    def query_system(self, query_text: str, session_id: str = "eval_session", config_name: str = None) -> Tuple[str, List[str], float]:
        """
        查询系统并返回结果（使用标准REST API）

        Args:
            query_text: 查询文本
            session_id: 会话ID
            config_name: 配置名称（用于动态切换）

        Returns:
            (response_text, retrieved_docs, latency_ms)
        """
        # 使用后端实际存在的MCP检索API
        url = f"{self.base_url}/api/mcp/tools/retrieve_context"

        payload = {
            "query": query_text,
            "session_id": session_id,
            "top_k": 5,
            "include_sources": True  # 确保返回检索源
        }

        # 支持配置切换（通过HTTP header）
        headers = {"Content-Type": "application/json"}
        if config_name:
            headers["X-Config-Name"] = config_name

        start_time = time.time()

        try:
            response = requests.post(url, json=payload, headers=headers, timeout=30)
            latency_ms = (time.time() - start_time) * 1000

            if response.status_code != 200:
                print(f"[ERROR] API返回错误: {response.status_code}")
                return "", [], latency_ms

            result = response.json()

            # 提取检索到的文档（适配多种响应格式）
            retrieved_docs = []
            if "retrieved_contexts" in result:
                retrieved_docs = [ctx.get("doc_id", ctx.get("id", ""))
                                for ctx in result["retrieved_contexts"]]
            elif "contexts" in result:
                retrieved_docs = [ctx.get("id", "") for ctx in result["contexts"]]

            response_text = result.get("answer", result.get("response", result.get("context", "")))

            return response_text, retrieved_docs, latency_ms

        except Exception as e:
            latency_ms = (time.time() - start_time) * 1000
            print(f"[ERROR] 查询失败: {e}")
            return "", [], latency_ms

    def calculate_metrics(self, retrieved_docs: List[str], ground_truth_docs: List[str]) -> Tuple[bool, float, float, float]:
        """
        计算检索指标

        Returns:
            (found, reciprocal_rank, precision_at_5, recall_at_5)
        """
        if not retrieved_docs or not ground_truth_docs:
            return False, 0.0, 0.0, 0.0

        # 取前5个结果
        top_5 = retrieved_docs[:5]

        # 查找第一个匹配的位置
        found = False
        reciprocal_rank = 0.0

        for rank, doc_id in enumerate(retrieved_docs, 1):
            if doc_id in ground_truth_docs:
                found = True
                reciprocal_rank = 1.0 / rank
                break

        # Precision@5: 前5个结果中正确的比例
        correct_in_top5 = sum(1 for doc in top_5 if doc in ground_truth_docs)
        precision_at_5 = correct_in_top5 / len(top_5) if top_5 else 0.0

        # Recall@5: 前5个结果召回了多少ground truth
        recall_at_5 = correct_in_top5 / len(ground_truth_docs) if ground_truth_docs else 0.0

        return found, reciprocal_rank, precision_at_5, recall_at_5

    def run_experiment(self, config_names: List[str], queries: List[Dict], sample_mode: str = "all") -> List[RetrievalResult]:
        """运行完整实验"""
        results = []

        # 如果是代表性采样，选择每种类型的前N个
        if sample_mode == "representative":
            queries = self._sample_representative(queries, per_type=3)

        total = len(config_names) * len(queries)
        current = 0

        for config_name in config_names:
            print(f"\n[CONFIG] 测试配置: {config_name}")
            config = self.load_config(config_name)

            for query_data in queries:
                current += 1
                query_id = query_data["query_id"]
                query_text = query_data["question"]
                query_type = query_data["query_type"]
                ground_truth = query_data.get("ground_truth_docs", [])

                print(f"[{current}/{total}] 查询: {query_text[:30]}...")

                # 执行检索（传递config_name）
                response_text, retrieved_docs, latency_ms = self.query_system(query_text, config_name=config_name)

                # 计算指标
                found, rr, p5, r5 = self.calculate_metrics(retrieved_docs, ground_truth)

                result = RetrievalResult(
                    query_id=query_id,
                    query_text=query_text,
                    query_type=query_type,
                    config_name=config_name,
                    retrieved_docs=retrieved_docs,
                    response_text=response_text,
                    latency_ms=latency_ms,
                    timestamp=datetime.now().isoformat(),
                    found_ground_truth=found,
                    reciprocal_rank=rr,
                    precision_at_5=p5,
                    recall_at_5=r5
                )

                results.append(result)

        return results

    def _sample_representative(self, queries: List[Dict], per_type: int = 3) -> List[Dict]:
        """选择代表性样本"""
        by_type = {}
        for q in queries:
            qtype = q["query_type"]
            if qtype not in by_type:
                by_type[qtype] = []
            by_type[qtype].append(q)

        sampled = []
        for qtype, qs in by_type.items():
            sampled.extend(qs[:per_type])

        return sampled

    def aggregate_metrics(self, results: List[RetrievalResult]) -> Dict[str, ConfigMetrics]:
        """聚合各配置的指标"""
        by_config = {}

        for result in results:
            config_name = result.config_name
            if config_name not in by_config:
                by_config[config_name] = []
            by_config[config_name].append(result)

        metrics = {}

        for config_name, config_results in by_config.items():
            # 计算平均值
            total = len(config_results)
            avg_latency = sum(r.latency_ms for r in config_results) / total
            mrr = sum(r.reciprocal_rank for r in config_results) / total
            precision = sum(r.precision_at_5 for r in config_results) / total
            recall = sum(r.recall_at_5 for r in config_results) / total

            # 按类型细分MRR
            by_type = {}
            for r in config_results:
                qtype = r.query_type
                if qtype not in by_type:
                    by_type[qtype] = []
                by_type[qtype].append(r.reciprocal_rank)

            temporal_mrr = sum(by_type.get("temporal", [0])) / len(by_type.get("temporal", [1]))
            causal_mrr = sum(by_type.get("causal", [0])) / len(by_type.get("causal", [1]))
            general_mrr = sum(by_type.get("general", [0])) / len(by_type.get("general", [1]))

            metrics[config_name] = ConfigMetrics(
                config_name=config_name,
                total_queries=total,
                avg_latency_ms=avg_latency,
                mrr=mrr,
                precision_at_5=precision,
                recall_at_5=recall,
                temporal_mrr=temporal_mrr,
                causal_mrr=causal_mrr,
                general_mrr=general_mrr
            )

        return metrics

    def save_results(self, results: List[RetrievalResult], metrics: Dict[str, ConfigMetrics]):
        """保存结果到JSON"""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = self.results_dir / f"retrieval_{timestamp}.json"

        # 检测execution_mode：是否使用了降级样例数据
        execution_mode = "live_api"  # 默认真实API
        if hasattr(self, '_using_sample_data') and self._using_sample_data:
            execution_mode = "offline_sample"

        output = {
            "execution_mode": execution_mode,
            "timestamp": timestamp,
            "total_queries": len(results),
            "configs": list(metrics.keys()),
            "detailed_results": [asdict(r) for r in results],
            "aggregated_metrics": {name: asdict(m) for name, m in metrics.items()}
        }

        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(output, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] 结果已保存: {output_file}")
        return output_file

    def print_summary(self, metrics: Dict[str, ConfigMetrics]):
        """打印结果摘要"""
        print("\n" + "="*60)
        print("检索融合实验结果摘要")
        print("="*60)

        for config_name, m in metrics.items():
            print(f"\n[{config_name}]")
            print(f"  查询数量: {m.total_queries}")
            print(f"  平均延迟: {m.avg_latency_ms:.1f}ms")
            print(f"  MRR: {m.mrr:.3f}")
            print(f"  Precision@5: {m.precision_at_5:.3f}")
            print(f"  Recall@5: {m.recall_at_5:.3f}")
            print(f"  时序MRR: {m.temporal_mrr:.3f}")
            print(f"  因果MRR: {m.causal_mrr:.3f}")
            print(f"  普通MRR: {m.general_mrr:.3f}")

    def interactive_mode(self):
        """交互模式：自定义查询"""
        print("\n" + "="*60)
        print("交互式检索演示")
        print("="*60)
        print("输入查询文本，展示检索过程和结果源")
        print("输入 'q' 退出")

        while True:
            query = input("\n查询> ").strip()

            if query.lower() == 'q':
                break

            if not query:
                continue

            print(f"\n[查询] {query}")
            print("-" * 60)

            # 交互模式默认使用full_system配置
            response, docs, latency = self.query_system(query, config_name="full_system")

            print(f"[延迟] {latency:.1f}ms")
            print(f"[检索到] {len(docs)} 个文档")

            if docs:
                print("\n[检索源]")
                for i, doc_id in enumerate(docs[:5], 1):
                    print(f"  {i}. {doc_id}")

            print(f"\n[响应]\n{response[:500]}")


def main():
    parser = argparse.ArgumentParser(description="检索融合实验")
    parser.add_argument("--queries", type=int, default=30, help="查询数量")
    parser.add_argument("--configs", default="all", help="配置列表，逗号分隔，或'all'")
    parser.add_argument("--sample-mode", default="all", choices=["all", "representative"],
                       help="采样模式")
    parser.add_argument("--query-types", default="all", help="查询类型过滤")
    parser.add_argument("--interactive", action="store_true", help="交互模式")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API基础URL")

    args = parser.parse_args()

    evaluator = RetrievalEvaluator(base_url=args.base_url)

    # 交互模式
    if args.interactive:
        evaluator.interactive_mode()
        return

    # 加载数据
    print("[LOAD] 加载检索ground truth...")
    queries = evaluator.load_ground_truth()

    if not queries:
        print("[ERROR] 无可用查询数据")
        sys.exit(1)

    # 限制查询数量
    if args.queries < len(queries):
        queries = queries[:args.queries]

    # 过滤查询类型
    if args.query_types != "all":
        types = args.query_types.split(",")
        queries = [q for q in queries if q["query_type"] in types]

    # 选择配置
    if args.configs == "all":
        config_names = ["baseline_vanilla_llm", "baseline_naive_rag",
                       "baseline_rag_with_filter", "full_system"]
    else:
        config_names = args.configs.split(",")

    print(f"[INFO] 将测试 {len(config_names)} 个配置 × {len(queries)} 条查询")

    # 运行实验
    results = evaluator.run_experiment(config_names, queries, args.sample_mode)

    # 聚合指标
    metrics = evaluator.aggregate_metrics(results)

    # 保存结果
    evaluator.save_results(results, metrics)

    # 打印摘要
    evaluator.print_summary(metrics)


if __name__ == "__main__":
    main()
