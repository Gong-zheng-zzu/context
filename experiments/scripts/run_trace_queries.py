#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
运行10条代表性查询的检索链路追踪
"""

import argparse
import json
import requests
import time
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from base_evaluator import BaseEvaluator

# 10条代表性查询（基于agent分析结果）
TRACE_QUERIES = [
    # Temporal查询（3条 - 全部MRR=0）
    {"query_id": "temporal_001", "question": "王明最近的健康记录和变化趋势是什么？"},
    {"query_id": "temporal_002", "question": "李国强最近的健康记录和变化趋势是什么？"},
    {"query_id": "temporal_003", "question": "赵志强最近的健康记录和变化趋势是什么？"},

    # Causal查询（3条 - 全部MRR=0）
    {"query_id": "causal_001", "question": "李国强骨折是因为什么原因引起的？"},
    {"query_id": "causal_002", "question": "为什么杨桂英会有睡眠问题？"},
    {"query_id": "causal_003", "question": "陈秀兰想开心起来的原因是什么？"},

    # General查询（4条 - 覆盖不同MRR）
    {"query_id": "general_001", "question": "王明的基本健康状况如何？"},
    {"query_id": "general_006", "question": "李华服用的药物有哪些？"},
    {"query_id": "general_012", "question": "当前所有老人用药清单及使用频率？"},
    {"query_id": "general_008", "question": "哪些老人出现过夜间咳嗽？"},  # 唯一成功案例
]

BASE_URL = "http://localhost:8088"
SESSION_ID = "eval_retrieval_test"
AUTH_HEADERS = {}

def test_single_query(query_data, mode):
    """测试单个查询并捕获日志"""
    print(f"\n{'='*80}")
    print(f"测试查询: {query_data['query_id']}")
    print(f"问题: {query_data['question']}")
    print('='*80)

    url = f"{BASE_URL}/api/mcp/tools/retrieve_context"
    payload = {
        "sessionId": SESSION_ID,
        "query": query_data['question'],
        "maxResults": 5,
    }
    if mode == "rrf":
        payload["evaluationRetrievalOnly"] = True
    else:
        payload["contextsOnly"] = True

    try:
        start_time = time.time()
        headers = {"Content-Type": "application/json"}
        headers.update(AUTH_HEADERS)
        response = requests.post(url, json=payload, headers=headers, timeout=60)
        latency_ms = (time.time() - start_time) * 1000

        if 200 <= response.status_code < 300:
            result = response.json()
            contexts = result.get("contexts")
            if contexts is None:
                contexts = []
            if not isinstance(contexts, list):
                raise ValueError("response contexts must be a list or null")

            print(f"✅ 查询成功 - 耗时: {latency_ms:.0f}ms")
            print(f"   返回结果数: {len(contexts)}")

            if len(contexts) > 0:
                print(f"   Top-5文档ID:")
                for i, ctx in enumerate(contexts[:5]):
                    doc_id = ctx.get("doc_id", ctx.get("id", "unknown"))
                    score = ctx.get("score", ctx.get("relevance", 0))
                    sources = ctx.get("metadata", {}).get("rrf_sources", [ctx.get("source", "unknown")])
                    print(f"     [{i+1}] {doc_id} (score={score:.4f}, sources={sources})")
            else:
                print(f"   ⚠️ 返回结果为空")

            doc_ids = [ctx.get("doc_id", ctx.get("id", "")) for ctx in contexts[:5]]
            valid_doc_ids = all(isinstance(doc_id, str) and doc_id.strip() for doc_id in doc_ids)
            unique_doc_ids = len(doc_ids) == len(set(doc_ids))
            evidence = {}
            if contexts:
                metadata = contexts[0].get("metadata", {})
                evidence = {
                    "active_sources": metadata.get("retrieval_active_sources", []),
                    "empty_sources": metadata.get("retrieval_empty_sources", []),
                    "fusion_mode": metadata.get("retrieval_fusion_mode", "not_reported"),
                }
            retrieval_metadata = result.get("retrieval_metadata", {})
            if not isinstance(retrieval_metadata, dict):
                retrieval_metadata = {}

            return {
                "query_id": query_data['query_id'],
                "question": query_data['question'],
                "success": True,
                "empty_result": not contexts,
                "result_count": len(contexts),
                "doc_ids": doc_ids,
                "valid_doc_ids": valid_doc_ids,
                "unique_doc_ids": unique_doc_ids,
                "sources": [ctx.get("metadata", {}).get("rrf_sources", [ctx.get("source", "unknown")]) for ctx in contexts[:5]],
                "evidence": evidence,
                "retrieval_metadata": retrieval_metadata,
                "latency_ms": latency_ms
            }
        else:
            print(f"❌ 查询失败 - HTTP {response.status_code}")
            return {
                "query_id": query_data['query_id'],
                "question": query_data['question'],
                "success": False,
                "error": f"HTTP {response.status_code}"
            }

    except Exception as e:
        print(f"❌ 查询异常: {e}")
        return {
            "query_id": query_data['query_id'],
            "question": query_data['question'],
            "success": False,
            "error": str(e)
        }

def main():
    global AUTH_HEADERS, BASE_URL, SESSION_ID
    parser = argparse.ArgumentParser(description="Run retrieval trace queries")
    parser.add_argument("--mode", choices=["vector", "rrf"], default="vector")
    parser.add_argument("--limit", type=int, default=len(TRACE_QUERIES), help="Number of representative queries to trace (1-10).")
    parser.add_argument("--base-url", default=BASE_URL)
    parser.add_argument("--session-id", default=SESSION_ID)
    parser.add_argument("--output", type=Path, help="Write the trace JSON to this path.")
    parser.add_argument(
        "--require-candidate-filter",
        action="store_true",
        help="Fail unless every trace request succeeds and includes retrieval_metadata.candidate_filter.",
    )
    args = parser.parse_args()

    if not 1 <= args.limit <= len(TRACE_QUERIES):
        parser.error(f"--limit must be between 1 and {len(TRACE_QUERIES)}")

    BASE_URL = args.base_url.rstrip("/")
    SESSION_ID = args.session_id

    print("🔍 开始10条代表性查询的检索链路追踪")
    print(f"服务地址: {BASE_URL}")
    print(f"Session ID: {SESSION_ID}")
    print(f"评测模式: {args.mode}")

    # 检查服务健康
    try:
        health_response = requests.get(f"{BASE_URL}/health", timeout=5)
        if health_response.status_code != 200:
            print(f"❌ 服务健康检查失败: HTTP {health_response.status_code}")
            print("请先启动服务: docker-compose up -d")
            sys.exit(1)
        print("✅ 服务健康检查通过\n")
    except Exception as e:
        print(f"❌ 无法连接到服务: {e}")
        print("请先启动服务: docker-compose up -d")
        sys.exit(1)

    # 运行10条查询
    evaluator = BaseEvaluator(BASE_URL)
    auth_response = evaluator.authenticate_from_env()
    if not auth_response.is_valid_business_response():
        print(f"[FAIL] Authentication failed: {auth_response.error_message}")
        sys.exit(1)
    AUTH_HEADERS = evaluator.get_auth_headers()
    print("[OK] Trace authentication succeeded\n")

    results = []
    selected_queries = TRACE_QUERIES[:args.limit]
    for query_data in selected_queries:
        result = test_single_query(query_data, args.mode)
        results.append(result)
        time.sleep(1)  # 避免请求过快

    # 保存结果
    output_dir = Path(__file__).parent.parent / "results" / "trace"
    output_dir.mkdir(parents=True, exist_ok=True)

    timestamp = time.strftime("%Y%m%d_%H%M%S")
    output_file = args.output or output_dir / f"trace_{args.mode}_queries_{timestamp}.json"
    output_file.parent.mkdir(parents=True, exist_ok=True)

    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump({
            "timestamp": timestamp,
            "mode": args.mode,
            "base_url": BASE_URL,
            "session_id": SESSION_ID,
            "total_queries": len(selected_queries),
            "results": results
        }, f, ensure_ascii=False, indent=2)

    print(f"\n{'='*80}")
    print(f"追踪完成！结果已保存到: {output_file}")
    print(f"\n📋 统计:")
    success_count = sum(1 for r in results if r.get("success"))
    print(f"   成功: {success_count}/{len(results)}")
    print(f"   失败: {len(results) - success_count}/{len(results)}")

    empty_count = sum(1 for r in results if r.get("success") and r.get("result_count", 0) == 0)
    print(f"   返回空结果: {empty_count}/{success_count}")

    print(f"\n💡 下一步:")
    print(f"   1. 提取Docker日志中的三路检索和RRF证据")
    print(f"   2. 验证每个返回doc_id有明确来源且不重复")
    if args.require_candidate_filter:
        failed_queries = [result["query_id"] for result in results if not result.get("success")]
        if failed_queries:
            print(f"[FAIL] trace requests did not succeed: {failed_queries}")
            return 1

        missing_candidate_filter = []
        for result in results:
            metadata = result.get("retrieval_metadata")
            if not isinstance(metadata, dict) or "candidate_filter" not in metadata:
                missing_candidate_filter.append(result["query_id"])
        if missing_candidate_filter:
            print(f"[FAIL] missing retrieval_metadata.candidate_filter: {missing_candidate_filter}")
            return 1

    print('='*80)
    return 0

if __name__ == "__main__":
    sys.exit(main())
