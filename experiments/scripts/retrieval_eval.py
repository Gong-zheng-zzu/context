#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
检索融合实验脚本 - P0修复版
严格区分API错误和业务失败，只有HTTP 2xx且有效响应才计入指标
"""

import json
import argparse
import hashlib
import math
import os
import sys
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Tuple
from dataclasses import dataclass, asdict

# 添加当前目录到Python路径
sys.path.insert(0, str(Path(__file__).parent))

# 导入基础评估器
from base_evaluator import BaseEvaluator, APIResponse


@dataclass
class RetrievalResult:
    """单次检索结果"""
    query_id: str
    query_text: str
    query_type: str
    config_name: str

    # API响应信息
    http_status: int
    error_type: str  # None表示成功
    error_message: str
    latency_ms: float

    # 业务数据（仅在API成功时有效）
    retrieved_docs: List[str]
    response_text: str
    is_scorable: bool = True
    unscorable_reason: str = ""

    # 评估指标（仅在API成功时计算）
    found_ground_truth: bool = False
    reciprocal_rank: float = 0.0
    precision_at_5: float = 0.0
    recall_at_5: float = 0.0

    timestamp: str = ""


@dataclass
class ConfigMetrics:
    """单个配置的聚合指标"""
    config_name: str
    total_queries: int
    api_success_count: int  # HTTP 2xx且有效响应
    api_error_count: int    # 4xx/5xx/timeout/parse error
    scorable_count: int     # 返回结构完整、可用于排名指标的响应数
    unscorable_count: int   # HTTP成功但缺失或重复doc_id，不能用于排名指标

    # 核心指标（仅基于成功的API调用计算）
    mrr: float
    precision_at_5: float
    recall_at_5: float
    avg_latency_ms: float
    p50_latency_ms: float = 0.0
    p95_latency_ms: float = 0.0

    # 按查询类型细分
    temporal_mrr: float = 0.0
    causal_mrr: float = 0.0
    general_mrr: float = 0.0


class RetrievalEvaluator(BaseEvaluator):
    """检索融合评估器"""

    def __init__(
        self,
        base_url: str = "http://localhost:8088",
        retrieval_mode: str = "vector",
        ground_truth_file: Path = None,
        corpus_file: Path = None,
    ):
        super().__init__(base_url)
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets"
        self.results_dir = self.project_root / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)
        self.ground_truth_file = ground_truth_file or self.datasets_dir / "retrieval_groundtruth" / "query_answer_pairs.json"
        self.corpus_file = corpus_file
        self._using_sample_data = False
        if retrieval_mode not in {"vector", "rrf"}:
            raise ValueError(f"Unsupported retrieval mode: {retrieval_mode}")
        self.retrieval_mode = retrieval_mode
        self.evaluation_mode = "vector_contexts_only" if retrieval_mode == "vector" else "multi_source_rrf_evaluation_only"

    def load_ground_truth(self) -> List[Dict]:
        """加载检索ground truth数据集"""
        gt_file = self.ground_truth_file

        if not gt_file.exists():
            print(f"\n[WARN] Ground truth文件不存在: {gt_file}")
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

        except Exception as e:
            print(f"[ERROR] 加载失败 - {e}")
            return self._generate_sample_queries()

    def _generate_sample_queries(self) -> List[Dict]:
        """生成示例查询"""
        self._using_sample_data = True
        return [
            {"query_id": "temporal_001", "query_type": "temporal",
             "question": "elder_025最近7天的血压变化趋势？",
             "ground_truth_docs": ["elder_025_*"]},
            {"query_id": "causal_001", "query_type": "causal",
             "question": "elder_025血压升高与用药的因果关系？",
             "ground_truth_docs": ["elder_025_*"]},
            {"query_id": "general_001", "query_type": "general",
             "question": "elder_025的基本信息和诊断？",
             "ground_truth_docs": ["elder_025_*"]},
        ]

    def query_retrieval_api(
        self,
        query_text: str,
        session_id: str = "eval_retrieval_test",
        config_name: str = None
    ) -> APIResponse:
        """
        调用检索API - 使用后端实际endpoint

        根据后端代码，使用 /api/mcp/tools/retrieve_context
        请求字段使用 sessionId（驼峰命名）
        """
        payload = {
            "sessionId": session_id,  # 使用驼峰命名，对齐Go后端
            "query": query_text,
            "maxResults": 5,
        }
        if self.retrieval_mode == "rrf":
            payload["evaluationRetrievalOnly"] = True
        else:
            payload["contextsOnly"] = True

        headers = {"Content-Type": "application/json"}
        headers.update(self.get_auth_headers())
        # Config switching now handled by service restart, not HTTP header
        # X-Config-Name header removed - backend doesn't process it

        return self.call_api("POST", "/api/mcp/tools/retrieve_context", payload, headers)

    def calculate_metrics(
        self,
        retrieved_docs: List[str],
        ground_truth_docs: List[str]
    ) -> Tuple[bool, float, float, float]:
        """计算检索指标"""
        if not retrieved_docs or not ground_truth_docs:
            return False, 0.0, 0.0, 0.0

        # MRR: 第一个匹配的倒数排名
        found = False
        reciprocal_rank = 0.0
        for rank, doc_id in enumerate(retrieved_docs, 1):
            if any(gt in doc_id or doc_id in gt for gt in ground_truth_docs):
                found = True
                reciprocal_rank = 1.0 / rank
                break

        # Precision@5
        top_5 = retrieved_docs[:5]
        correct = sum(1 for doc in top_5
                     if any(gt in doc or doc in gt for gt in ground_truth_docs))
        precision_at_5 = correct / len(top_5) if top_5 else 0.0

        # Recall@5
        recall_at_5 = correct / len(ground_truth_docs) if ground_truth_docs else 0.0

        return found, reciprocal_rank, precision_at_5, recall_at_5

    def test_single_query(
        self,
        query_data: Dict,
        config_name: str
    ) -> RetrievalResult:
        """测试单个查询"""
        query_id = query_data["query_id"]
        query_text = query_data["question"]
        query_type = query_data["query_type"]
        ground_truth = query_data.get("ground_truth_docs", [])

        # 调用API
        response = self.query_retrieval_api(query_text, config_name=config_name)

        # 初始化结果
        result = RetrievalResult(
            query_id=query_id,
            query_text=query_text,
            query_type=query_type,
            config_name=config_name,
            http_status=response.http_status,
            error_type=response.error_type or "None",
            error_message=response.error_message or "",
            latency_ms=response.latency_ms,
            retrieved_docs=[],
            response_text="",
            timestamp=datetime.now().isoformat()
        )

        # 只有在API成功时才提取业务数据和计算指标
        if response.is_valid_business_response():
            # 提取检索到的文档
            data = response.data
            retrieved_docs = []

            contexts = data.get("contexts", data.get("retrieved_contexts", []))
            if not isinstance(contexts, list):
                result.is_scorable = False
                result.unscorable_reason = "contexts is not a list"
                return result

            invalid_doc_ids = 0
            for context in contexts:
                doc_id = context.get("doc_id") if isinstance(context, dict) else None
                if not isinstance(doc_id, str) or not doc_id.strip():
                    invalid_doc_ids += 1
                    continue
                retrieved_docs.append(doc_id.strip())

            if invalid_doc_ids:
                result.is_scorable = False
                result.unscorable_reason = f"{invalid_doc_ids} context item(s) missing doc_id"
                return result

            if len(retrieved_docs) != len(set(retrieved_docs)):
                result.is_scorable = False
                result.unscorable_reason = "response contains duplicate doc_id values"
                return result

            # 严格截断到top-5，确保MRR计算口径准确
            if len(retrieved_docs) > 5:
                retrieved_docs = retrieved_docs[:5]

            response_text = data.get("context", data.get("answer", ""))

            result.retrieved_docs = retrieved_docs
            result.response_text = response_text

            # 计算指标
            if retrieved_docs:
                found, rr, p5, r5 = self.calculate_metrics(retrieved_docs, ground_truth)
                result.found_ground_truth = found
                result.reciprocal_rank = rr
                result.precision_at_5 = p5
                result.recall_at_5 = r5

        return result

    def run_experiment(
        self,
        config_names: List[str],
        queries: List[Dict],
        sample_mode: str = "all"
    ) -> List[RetrievalResult]:
        """运行完整实验"""
        results = []

        if sample_mode == "representative":
            queries = self._sample_representative(queries, per_type=3)

        total = len(config_names) * len(queries)
        current = 0

        for config_name in config_names:
            print(f"\n[CONFIG] 测试配置: {config_name}")

            for query_data in queries:
                current += 1
                query_text = query_data["question"]
                print(f"[{current}/{total}] 查询: {query_text[:40]}...")

                result = self.test_single_query(query_data, config_name)
                results.append(result)

                # 显示结果
                if result.error_type == "None" and result.is_scorable:
                    print(f"  [OK] HTTP {result.http_status} | "
                          f"检索到{len(result.retrieved_docs)}文档 | "
                          f"MRR={result.reciprocal_rank:.3f} | "
                          f"{result.latency_ms:.0f}ms")
                elif result.error_type == "None":
                    print(f"  [UNSCORABLE] {result.unscorable_reason}")
                else:
                    print(f"  [API_ERROR] {result.error_type}: {result.error_message[:50]}")

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
        """聚合指标 - 只统计API成功的请求"""
        by_config = {}

        for result in results:
            config_name = result.config_name
            if config_name not in by_config:
                by_config[config_name] = []
            by_config[config_name].append(result)

        metrics = {}

        for config_name, config_results in by_config.items():
            total = len(config_results)

            # 区分API成功和失败
            api_success = [r for r in config_results if r.error_type == "None"]
            api_error = [r for r in config_results if r.error_type != "None"]
            scorable = [r for r in api_success if r.is_scorable]

            api_success_count = len(api_success)
            api_error_count = len(api_error)

            # Ranking metrics only use structurally valid, uniquely identified documents.
            if scorable:
                mrr = sum(r.reciprocal_rank for r in scorable) / len(scorable)
                precision = sum(r.precision_at_5 for r in scorable) / len(scorable)
                recall = sum(r.recall_at_5 for r in scorable) / len(scorable)
                latencies = sorted(r.latency_ms for r in api_success)
                avg_latency = sum(latencies) / api_success_count
                p50_latency = self._percentile(latencies, 50)
                p95_latency = self._percentile(latencies, 95)

                # 按类型细分
                by_type = {}
                for r in scorable:
                    qtype = r.query_type
                    if qtype not in by_type:
                        by_type[qtype] = []
                    by_type[qtype].append(r.reciprocal_rank)

                temporal_mrr = (sum(by_type.get("temporal", [0])) /
                               len(by_type.get("temporal", [1])))
                causal_mrr = (sum(by_type.get("causal", [0])) /
                             len(by_type.get("causal", [1])))
                general_mrr = (sum(by_type.get("general", [0])) /
                              len(by_type.get("general", [1])))
            else:
                mrr = precision = recall = avg_latency = p50_latency = p95_latency = 0.0
                temporal_mrr = causal_mrr = general_mrr = 0.0

            metrics[config_name] = ConfigMetrics(
                config_name=config_name,
                total_queries=total,
                api_success_count=api_success_count,
                api_error_count=api_error_count,
                scorable_count=len(scorable),
                unscorable_count=api_success_count - len(scorable),
                mrr=mrr,
                precision_at_5=precision,
                recall_at_5=recall,
                avg_latency_ms=avg_latency,
                p50_latency_ms=p50_latency,
                p95_latency_ms=p95_latency,
                temporal_mrr=temporal_mrr,
                causal_mrr=causal_mrr,
                general_mrr=general_mrr
            )

        return metrics

    @staticmethod
    def _percentile(values: List[float], percentile: float) -> float:
        if not values:
            return 0.0
        index = (len(values) - 1) * percentile / 100
        lower = math.floor(index)
        upper = math.ceil(index)
        if lower == upper:
            return values[lower]
        return values[lower] + (values[upper] - values[lower]) * (index - lower)

    def save_results(
        self,
        results: List[RetrievalResult],
        metrics: Dict[str, ConfigMetrics],
        output_file: Path = None,
    ):
        """保存结果"""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        if output_file is None:
            output_file = self.results_dir / f"retrieval_{timestamp}.json"
        else:
            output_file.parent.mkdir(parents=True, exist_ok=True)

        execution_mode = "offline_sample" if self._using_sample_data else "live_api"

        # 计算配置文件hash
        config_hashes = {}
        for config_name in metrics.keys():
            config_path = self.get_config_path(config_name)
            if config_path:
                config_hashes[config_name] = self.calculate_config_hash(config_path)
            else:
                config_hashes[config_name] = "config_file_not_found"

        ground_truth_file = self.ground_truth_file
        dataset_hash = self._calculate_file_hash(ground_truth_file)
        runtime_config = {
            "config_name": os.getenv("EVAL_RUNTIME_CONFIG_NAME", "not_captured"),
            "config_file": os.getenv("EVAL_RUNTIME_CONFIG_FILE", "not_captured"),
            "config_hash": os.getenv("EVAL_RUNTIME_CONFIG_HASH", "not_captured"),
            "service_start_time": os.getenv("EVAL_SERVICE_START_TIME", "not_captured"),
        }

        output = {
            "execution_mode": execution_mode,
            "evaluation_mode": self.evaluation_mode,
            "timestamp": timestamp,
            "total_queries": len(results),
            "configs": list(metrics.keys()),
            "config_hashes": config_hashes,  # 🔥 新增：配置文件hash
            "runtime_config": runtime_config,
            "dataset": {
                "ground_truth_file": str(ground_truth_file),
                "ground_truth_sha256": dataset_hash,
                "corpus_file": str(self.corpus_file) if self.corpus_file else "not_captured",
                "corpus_sha256": self._calculate_file_hash(self.corpus_file) if self.corpus_file else "not_captured",
            },
            "detailed_results": [asdict(r) for r in results],
            "aggregated_metrics": {name: asdict(m) for name, m in metrics.items()}
        }

        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(output, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] 结果已保存: {output_file}")

        # 显示配置hash
        print("\n[CONFIG HASHES]")
        for config_name, hash_value in config_hashes.items():
            print(f"  {config_name}: {hash_value[:16]}...")

        return output_file

    @staticmethod
    def _calculate_file_hash(path: Path) -> str:
        if not path.exists():
            return "file_not_found"
        return hashlib.sha256(path.read_bytes()).hexdigest()

    def print_summary(self, metrics: Dict[str, ConfigMetrics]):
        """打印摘要"""
        print("\n" + "=" * 60)
        print("检索融合实验结果摘要")
        print("=" * 60)

        for config_name, m in metrics.items():
            print(f"\n[{config_name}]")
            print(f"  总查询数: {m.total_queries}")
            print(f"  API成功: {m.api_success_count} ({m.api_success_count/m.total_queries*100:.1f}%)")
            print(f"  API错误: {m.api_error_count}")
            print(f"  可评分响应: {m.scorable_count}")
            print(f"  不可评分响应: {m.unscorable_count}")

            if m.scorable_count > 0:
                print(f"  MRR: {m.mrr:.3f}")
                print(f"  Precision@5: {m.precision_at_5:.3f}")
                print(f"  Recall@5: {m.recall_at_5:.3f}")
                print(f"  平均延迟: {m.avg_latency_ms:.1f}ms")
                print(f"  P50/P95延迟: {m.p50_latency_ms:.1f}ms / {m.p95_latency_ms:.1f}ms")
            else:
                print(f"  [警告] 无可评分响应，无法计算排名指标")


def main():
    parser = argparse.ArgumentParser(description="检索融合实验")
    parser.add_argument("--queries", type=int, default=30, help="查询数量")
    parser.add_argument("--configs", default="all", help="配置列表")
    parser.add_argument("--sample-mode", default="all",
                       choices=["all", "representative"], help="采样模式")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API基础URL")
    parser.add_argument("--retrieval-mode", choices=["vector", "rrf"], default="vector",
                       help="vector: contextsOnly基线; rrf: 三路检索RRF评测通道")
    parser.add_argument("--output", type=Path, default=None,
                       help="Write this run's raw result to an explicit JSON path.")
    parser.add_argument("--ground-truth-file", type=Path, default=None,
                       help="Ground-truth JSON path; defaults to the 30/50 regression set.")
    parser.add_argument("--corpus-file", type=Path, default=None,
                       help="Corpus JSON used to seed this run; recorded with the result hash.")
    parser.add_argument("--require-reviewed-ground-truth", action="store_true",
                       help="Refuse to run unless query metadata is approved for single-reviewer reporting.")

    args = parser.parse_args()

    evaluator = RetrievalEvaluator(
        base_url=args.base_url,
        retrieval_mode=args.retrieval_mode,
        ground_truth_file=args.ground_truth_file,
        corpus_file=args.corpus_file,
    )

    # 检查服务健康
    healthy, message = evaluator.check_service_health()
    if not healthy:
        print(f"[FAIL] {message}")
        sys.exit(1)
    print(f"[OK] {message}\n")

    auth_response = evaluator.authenticate_from_env()
    if not auth_response.is_valid_business_response():
        print(f"[FAIL] Retrieval evaluation authentication failed: {auth_response.error_message}")
        sys.exit(1)
    print("[OK] Retrieval evaluation authentication succeeded\n")

    # 加载数据
    print("[LOAD] 加载检索ground truth...")
    queries = evaluator.load_ground_truth()

    if args.require_reviewed_ground_truth:
        raw_ground_truth = json.loads(evaluator.ground_truth_file.read_text(encoding="utf-8"))
        metadata = raw_ground_truth.get("metadata", {}) if isinstance(raw_ground_truth, dict) else {}
        if metadata.get("annotation_status") != "approved_single_reviewer" or metadata.get("report_eligible") is not True:
            print("[ERROR] Ground truth is not approved by the declared single reviewer; formal metrics are blocked.")
            sys.exit(2)

    if not queries:
        print("[ERROR] 无可用查询数据")
        sys.exit(1)

    if args.queries < len(queries):
        queries = queries[:args.queries]

    # 选择配置
    if args.configs == "all":
        config_names = ["baseline_vanilla_llm", "baseline_naive_rag",
                       "baseline_rag_with_filter", "full_system"]
    else:
        config_names = args.configs.split(",")

    if len(config_names) != 1:
        print("[ERROR] One evaluation result can record only one runtime configuration.")
        sys.exit(2)

    print(f"[INFO] 评测模式: {evaluator.evaluation_mode}")
    print(f"[INFO] 将测试 {len(config_names)} 个配置 × {len(queries)} 条查询")

    # 运行实验
    results = evaluator.run_experiment(config_names, queries, args.sample_mode)

    # 聚合指标
    metrics = evaluator.aggregate_metrics(results)

    # 保存结果
    evaluator.save_results(results, metrics, args.output)

    # 打印摘要
    evaluator.print_summary(metrics)


if __name__ == "__main__":
    main()
