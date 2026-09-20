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
from typing import List, Dict, Tuple, Any
from dataclasses import dataclass, asdict, field

# 添加当前目录到Python路径
sys.path.insert(0, str(Path(__file__).parent))

# 导入基础评估器
from base_evaluator import BaseEvaluator, APIResponse

# 统计原语：bootstrap 区间与 Wilson 区间。放在本目录，避免评测链路依赖训练管线。
import stat_tools

DEFAULT_SESSION_ID = "eval_retrieval_test"

# 重采样次数与种子固定，使同一批结果每次重算都得到相同区间。
STAT_BOOTSTRAP_RESAMPLES = 10000
STAT_BOOTSTRAP_SEED = 20260920

# 真值标注状态。只有经人工单评审人批准的查询集才可声明为可报告真值；由构造自动
# 推导的标注必须走独立门禁，并在结果中明确记录「未经人工复核」，两者不可混用。
REVIEWED_ANNOTATION_STATUS = "approved_single_reviewer"
AUTO_DERIVED_ANNOTATION_STATUS = "auto_derived_unreviewed"

# 自动标注数据集必须自述其相关性来源：本项目的构造式真值来自「查询由目标文档生成」，
# 即相关性是定义性的，而不是人工相关性判定。门禁据此确认数据集没有冒充人工标注。
AUTO_DERIVED_DERIVATION_METHOD = "query_constructed_from_source_document"


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

    # 融合审计证据（用于归因 RRF 与其他多源结果）。
    # 响应未携带对应字段时保持 "not_captured"，绝不臆造，也不影响既有字段。
    retrieval_metadata: Dict[str, Any] = field(default_factory=dict)
    fusion_mode: str = "not_captured"
    source_statuses: Dict[str, str] = field(default_factory=dict)
    source_candidate_counts: Dict[str, int] = field(default_factory=dict)
    # rrf_sources_seen：本次响应中每个 doc 命中的来源集合（去重后按首次出现顺序）
    rrf_sources_seen: List[str] = field(default_factory=list)
    # doc_source_map：doc_id -> 该 doc 的来源集合（来自行级 rrf_sources）
    doc_source_map: Dict[str, List[str]] = field(default_factory=dict)


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

    # 融合审计聚合：用于解释「融合到底做了什么」。
    # 响应未携带证据时保持空字典/not_captured，既有指标不受影响。
    fusion_mode_counts: Dict[str, int] = field(default_factory=dict)
    source_status_counts: Dict[str, Dict[str, int]] = field(default_factory=dict)
    source_candidate_total: Dict[str, int] = field(default_factory=dict)
    source_hit_query_count: Dict[str, int] = field(default_factory=dict)
    captured_query_count: int = 0


class RetrievalEvaluator(BaseEvaluator):
    """检索融合评估器"""

    def __init__(
        self,
        base_url: str = "http://localhost:8088",
        retrieval_mode: str = "vector",
        ground_truth_file: Path = None,
        corpus_file: Path = None,
        session_id: str = DEFAULT_SESSION_ID,
        bootstrap_resamples: int = STAT_BOOTSTRAP_RESAMPLES,
        bootstrap_seed: int = STAT_BOOTSTRAP_SEED,
    ):
        super().__init__(base_url)
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets"
        self.results_dir = self.project_root / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)
        self.ground_truth_file = ground_truth_file or self.datasets_dir / "retrieval_groundtruth" / "query_answer_pairs.json"
        self.corpus_file = corpus_file
        # 会话可配置：换语料时必须先清空同一会话。旧语料若残留在会话中，检回的文档会
        # 与当前语料的真值不相交，指标随之失真。
        self.session_id = session_id
        # 区间参数进入结果文件，使区间本身可复算、可审计。
        self.bootstrap_resamples = bootstrap_resamples
        self.bootstrap_seed = bootstrap_seed
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
        session_id: str = None,
        config_name: str = None
    ) -> APIResponse:
        """
        调用检索API - 使用后端实际endpoint

        根据后端代码，使用 /api/mcp/tools/retrieve_context
        请求字段使用 sessionId（驼峰命名）
        """
        payload = {
            "sessionId": session_id or self.session_id,  # 使用驼峰命名，对齐Go后端
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
            sources_seen: List[str] = []
            doc_source_map: Dict[str, List[str]] = {}
            for context in contexts:
                doc_id = context.get("doc_id") if isinstance(context, dict) else None
                if not isinstance(doc_id, str) or not doc_id.strip():
                    invalid_doc_ids += 1
                    continue
                retrieved_docs.append(doc_id.strip())

                # 行级融合证据：每个 doc 命中的来源集合（rrf_sources）。
                # 该字段由 Go 侧 ApplyFusionAuditMetadata 注入；缺失时静默跳过。
                metadata = context.get("metadata") if isinstance(context, dict) else None
                if isinstance(metadata, dict):
                    doc_sources = metadata.get("rrf_sources")
                    if isinstance(doc_sources, list):
                        normalized_sources = [s for s in doc_sources if isinstance(s, str) and s]
                        if normalized_sources:
                            doc_source_map[doc_id.strip()] = normalized_sources
                            for source in normalized_sources:
                                if source not in sources_seen:
                                    sources_seen.append(source)

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
            result.rrf_sources_seen = sources_seen
            result.doc_source_map = doc_source_map

            # 响应级融合证据：融合模式、各路状态与各路候选数。
            # 这些字段是判断「RRF 是否真的融合了某些来源」的唯一依据，
            # 缺失时保持 not_captured，绝不臆造。
            retrieval_metadata = data.get("retrieval_metadata")
            if isinstance(retrieval_metadata, dict):
                result.retrieval_metadata = retrieval_metadata

                fusion_mode = retrieval_metadata.get("retrieval_fusion_mode")
                if isinstance(fusion_mode, str) and fusion_mode:
                    result.fusion_mode = fusion_mode

                statuses = retrieval_metadata.get("source_statuses")
                if isinstance(statuses, dict):
                    result.source_statuses = {str(k): str(v) for k, v in statuses.items()}

                counts = retrieval_metadata.get("source_candidate_counts")
                if isinstance(counts, dict):
                    result.source_candidate_counts = {
                        str(k): v for k, v in counts.items() if isinstance(v, int) and not isinstance(v, bool)
                    }

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

            # 融合审计聚合：把「融合模式 / 各路状态 / 各路候选数 / 各路命中查询数」
            # 汇总起来，使 RRF 的收益或负收益可以被归因到具体来源。
            fusion_mode_counts: Dict[str, int] = {}
            source_status_counts: Dict[str, Dict[str, int]] = {}
            source_candidate_total: Dict[str, int] = {}
            source_hit_query_count: Dict[str, int] = {}
            captured_query_count = 0

            for r in api_success:
                if not isinstance(r.retrieval_metadata, dict) or not r.retrieval_metadata:
                    continue
                captured_query_count += 1

                if r.fusion_mode and r.fusion_mode != "not_captured":
                    fusion_mode_counts[r.fusion_mode] = fusion_mode_counts.get(r.fusion_mode, 0) + 1

                for source, status in r.source_statuses.items():
                    bucket = source_status_counts.setdefault(source, {})
                    bucket[status] = bucket.get(status, 0) + 1

                for source, count in r.source_candidate_counts.items():
                    source_candidate_total[source] = source_candidate_total.get(source, 0) + count

                for source in set(r.rrf_sources_seen):
                    source_hit_query_count[source] = source_hit_query_count.get(source, 0) + 1

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
                general_mrr=general_mrr,
                fusion_mode_counts=fusion_mode_counts,
                source_status_counts=source_status_counts,
                source_candidate_total=source_candidate_total,
                source_hit_query_count=source_hit_query_count,
                captured_query_count=captured_query_count
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

    def load_ground_truth_metadata(self) -> Dict[str, Any]:
        """读取真值文件的 metadata 块，用于记录标注状态与来源指纹。

        文件缺失或结构不符时返回空字典：门禁据此拒绝运行，而不是静默放行。
        """
        if not self.ground_truth_file.exists():
            return {}
        try:
            payload = json.loads(self.ground_truth_file.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return {}
        if not isinstance(payload, dict):
            return {}
        metadata = payload.get("metadata")
        return metadata if isinstance(metadata, dict) else {}

    def build_statistical_analysis(
        self,
        results: List[RetrievalResult],
        resamples: int = None,
        seed: int = None,
    ) -> Dict[str, Any]:
        """为排名指标计算 bootstrap 区间，并为命中率附 Wilson 区间。

        只用可评分响应（``is_scorable``）：结构不完整的响应不进入分母，避免把响应
        格式问题混进检索质量。区间与点估计必须同口径，否则区间无法解释。

        MRR / P@5 / R@5 的逐样本取值不是 0/1 比例量（MRR 可取 1/2、1/3…），比例检验
        不适用，因此用 bootstrap。命中率（hit@5）本身是比例量，额外给出 Wilson 区间，
        以便与同一批数据上的其它比例指标直接比较。
        """
        resamples = self.bootstrap_resamples if resamples is None else resamples
        seed = self.bootstrap_seed if seed is None else seed

        scorable = [r for r in results if r.error_type == "None" and r.is_scorable]
        analysis: Dict[str, Any] = {
            "method": "bootstrap_percentile",
            "resamples": resamples,
            "seed": seed,
            "confidence_level": 0.95,
            "sample_size": len(scorable),
            "metrics": {},
        }
        if not scorable:
            return analysis

        metric_intervals = analysis["metrics"]
        metric_intervals["mrr"] = stat_tools.mean_bootstrap_ci(
            [r.reciprocal_rank for r in scorable], resamples=resamples, seed=seed
        )
        metric_intervals["precision_at_5"] = stat_tools.mean_bootstrap_ci(
            [r.precision_at_5 for r in scorable], resamples=resamples, seed=seed
        )
        metric_intervals["recall_at_5"] = stat_tools.mean_bootstrap_ci(
            [r.recall_at_5 for r in scorable], resamples=resamples, seed=seed
        )

        hit_count = sum(1 for r in scorable if r.found_ground_truth)
        hit_interval = stat_tools.mean_bootstrap_ci(
            [1.0 if r.found_ground_truth else 0.0 for r in scorable],
            resamples=resamples,
            seed=seed,
        )
        hit_interval["hit_count"] = hit_count
        hit_interval["wilson_95_ci"] = stat_tools.wilson_ci(hit_count, len(scorable))
        metric_intervals["hit_at_5"] = hit_interval
        return analysis

    def save_results(
        self,
        results: List[RetrievalResult],
        metrics: Dict[str, ConfigMetrics],
        output_file: Path = None,
        statistical_analysis: Dict[str, Any] = None,
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
        ground_truth_metadata = self.load_ground_truth_metadata()
        runtime_config = {
            "config_name": os.getenv("EVAL_RUNTIME_CONFIG_NAME", "not_captured"),
            "config_file": os.getenv("EVAL_RUNTIME_CONFIG_FILE", "not_captured"),
            "config_hash": os.getenv("EVAL_RUNTIME_CONFIG_HASH", "not_captured"),
            "service_start_time": os.getenv("EVAL_SERVICE_START_TIME", "not_captured"),
        }

        # 真值的标注状态与来源指纹必须随结果落盘：机器推导的标注与人工复核的标注在
        # 结论强度上并不相同，仅凭文件路径无法区分，事后也无法审计。
        dataset_block: Dict[str, Any] = {
            "ground_truth_file": str(ground_truth_file),
            "ground_truth_sha256": dataset_hash,
            "corpus_file": str(self.corpus_file) if self.corpus_file else "not_captured",
            "corpus_sha256": self._calculate_file_hash(self.corpus_file) if self.corpus_file else "not_captured",
            "session_id": self.session_id,
            "annotation_status": ground_truth_metadata.get("annotation_status", "not_captured"),
            "report_eligible": ground_truth_metadata.get("report_eligible", "not_captured"),
            "derivation_method": ground_truth_metadata.get("derivation_method", "not_captured"),
            "source_corpus_sha256": ground_truth_metadata.get("source_corpus_sha256", "not_captured"),
            "annotation_caveat": ground_truth_metadata.get("annotation_caveat", ""),
        }

        output = {
            "execution_mode": execution_mode,
            "evaluation_mode": self.evaluation_mode,
            "timestamp": timestamp,
            "total_queries": len(results),
            "configs": list(metrics.keys()),
            "config_hashes": config_hashes,  # 🔥 新增：配置文件hash
            "runtime_config": runtime_config,
            "dataset": dataset_block,
            # 新键追加，既有聚合键保持不变，避免影响下游消费者。
            "statistical_analysis": statistical_analysis or self.build_statistical_analysis(results),
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

    def print_summary(
        self,
        metrics: Dict[str, ConfigMetrics],
        statistical_analysis: Dict[str, Any] = None,
    ):
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

            # 融合审计摘要：解释「融合到底做了什么」，用于归因收益或负收益。
            if m.captured_query_count > 0:
                print(f"  融合证据: {m.captured_query_count}/{m.api_success_count} 条响应携带检索元数据")
                if m.fusion_mode_counts:
                    modes = ", ".join(f"{name}={count}" for name, count in sorted(m.fusion_mode_counts.items()))
                    print(f"    融合模式: {modes}")
                if m.source_status_counts:
                    for source in sorted(m.source_status_counts):
                        statuses = ", ".join(
                            f"{name}={count}" for name, count in sorted(m.source_status_counts[source].items())
                        )
                        candidates = m.source_candidate_total.get(source, 0)
                        hits = m.source_hit_query_count.get(source, 0)
                        print(f"    {source}: 状态[{statuses}] 候选总数={candidates} 命中查询数={hits}")
            else:
                print("  融合证据: not_captured（响应未携带 retrieval_metadata）")

            # 统计区间：没有区间时，点估计之间的小差距无法判断是真实差异还是样本波动。
            if statistical_analysis:
                print(
                    f"\n  统计区间（{statistical_analysis.get('method')}, "
                    f"重采样={statistical_analysis.get('resamples')}, "
                    f"seed={statistical_analysis.get('seed')}, "
                    f"n={statistical_analysis.get('sample_size')}）"
                )
                for metric_name, interval in (statistical_analysis.get("metrics") or {}).items():
                    if not isinstance(interval, dict):
                        continue
                    point = interval.get("point")
                    low = interval.get("ci_low")
                    high = interval.get("ci_high")
                    if point is None or low is None or high is None:
                        continue
                    print(f"    {metric_name}: {point:.4f} [{low:.4f}, {high:.4f}]")
                    wilson = interval.get("wilson_95_ci")
                    if wilson:
                        print(
                            f"      Wilson 95%: [{wilson[0]:.4f}, {wilson[1]:.4f}]"
                            f" (hit={interval.get('hit_count')})"
                        )


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
    parser.add_argument("--allow-auto-derived-ground-truth", action="store_true",
                       help="Permit query metadata that declares machine-derived, unreviewed labels; "
                            "the caveat is recorded with the result.")
    parser.add_argument("--session-id", default=DEFAULT_SESSION_ID,
                       help="Session whose seeded corpus this run measures; recorded with the result.")
    parser.add_argument("--bootstrap-resamples", type=int, default=STAT_BOOTSTRAP_RESAMPLES,
                       help="Bootstrap resample count for the reported confidence intervals.")
    parser.add_argument("--bootstrap-seed", type=int, default=STAT_BOOTSTRAP_SEED,
                       help="Bootstrap seed; fixed so the intervals are reproducible.")

    args = parser.parse_args()

    evaluator = RetrievalEvaluator(
        base_url=args.base_url,
        retrieval_mode=args.retrieval_mode,
        ground_truth_file=args.ground_truth_file,
        corpus_file=args.corpus_file,
        session_id=args.session_id,
        bootstrap_resamples=args.bootstrap_resamples,
        bootstrap_seed=args.bootstrap_seed,
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

    if args.require_reviewed_ground_truth and args.allow_auto_derived_ground_truth:
        print("[ERROR] --require-reviewed-ground-truth and --allow-auto-derived-ground-truth "
              "are mutually exclusive.")
        sys.exit(2)

    metadata = evaluator.load_ground_truth_metadata()

    if args.require_reviewed_ground_truth:
        if metadata.get("annotation_status") != REVIEWED_ANNOTATION_STATUS or metadata.get("report_eligible") is not True:
            print("[ERROR] Ground truth is not approved by the declared single reviewer; formal metrics are blocked.")
            sys.exit(2)
    elif args.allow_auto_derived_ground_truth:
        # 自动标注分支不放松判据，只是承认一种**弱于**人工复核的明确标注状态，并要求
        # 数据集自述为不可报告。任何含糊或「看起来像已复核」的声明都会被拒绝，避免机器
        # 推导的标签被当成人工标注来用。
        status = metadata.get("annotation_status")
        if status != AUTO_DERIVED_ANNOTATION_STATUS:
            print(f"[ERROR] --allow-auto-derived-ground-truth expects annotation_status="
                  f"{AUTO_DERIVED_ANNOTATION_STATUS}, got {status!r}.")
            sys.exit(2)
        if metadata.get("report_eligible") is not False:
            print("[ERROR] Auto-derived ground truth must declare report_eligible=false.")
            sys.exit(2)
        if metadata.get("derivation_method") != AUTO_DERIVED_DERIVATION_METHOD:
            print("[ERROR] Auto-derived ground truth must declare "
                  f"derivation_method={AUTO_DERIVED_DERIVATION_METHOD}.")
            sys.exit(2)
        print(f"[WARN] Ground truth is machine-derived and unreviewed ({status}). "
              "Metrics are recorded with this caveat and must not be described as human-verified.\n")
    elif metadata and metadata.get("report_eligible") is not True:
        print(f"[INFO] Ground truth is not report-eligible "
              f"(annotation_status={metadata.get('annotation_status')!r}); the provenance will be "
              "recorded with the result. Use --allow-auto-derived-ground-truth to acknowledge "
              "auto-derived labels explicitly, or --require-reviewed-ground-truth to demand "
              "reviewed ones.\n")

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

    # 统计区间（bootstrap 与 Wilson）。只算一次并同时交给落盘与打印，
    # 避免同一批结果被重复重采样而浪费算力。
    statistical_analysis = evaluator.build_statistical_analysis(results)

    # 保存结果
    evaluator.save_results(results, metrics, args.output, statistical_analysis)

    # 打印摘要
    evaluator.print_summary(metrics, statistical_analysis)


if __name__ == "__main__":
    main()
