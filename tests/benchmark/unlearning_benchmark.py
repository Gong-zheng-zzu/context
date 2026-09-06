#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
机器遗忘验证实验
测试梯度正交投影算法的遗忘效果
"""

import json
import time
import requests
from pathlib import Path
from typing import Dict, List, Tuple
from dataclasses import dataclass, asdict
from datetime import datetime

@dataclass
class UnlearningTestResult:
    """遗忘测试结果"""
    user_id: str
    test_phase: str  # "before_unlearning" or "after_unlearning"

    # 检索测试
    direct_query_hits: int  # 直接查询命中数
    fuzzy_query_hits: int  # 模糊查询命中数
    related_query_hits: int  # 关联查询命中数

    # 遗忘统计
    vectors_before: int
    vectors_after: int
    removal_rate: float  # 移除率

    # 非目标用户影响
    other_users_affected: int
    other_users_total: int
    side_effect_rate: float

    # 性能指标
    unlearning_duration_sec: float
    iterations_used: int
    convergence_achieved: bool

    timestamp: str

class UnlearningBenchmark:
    """机器遗忘基准测试"""

    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.output_dir = Path(__file__).parent / "results"
        self.output_dir.mkdir(parents=True, exist_ok=True)

    def create_test_user_data(self, user_id: str, num_records: int = 50) -> List[Dict]:
        """为测试用户创建向量数据"""
        print(f"[CREATE] Creating {num_records} records for user {user_id}...")

        # 模拟护理记录
        records = []
        for i in range(num_records):
            record = {
                "user_id": user_id,
                "content": f"测试用户{user_id}的护理记录{i+1}：血压正常，情绪稳定，饮食良好。",
                "timestamp": datetime.now().isoformat(),
                "metadata": {
                    "record_id": f"{user_id}_record_{i+1:03d}",
                    "type": "nursing_log"
                }
            }
            records.append(record)

            # 发送到系统创建向量
            try:
                response = requests.post(
                    f"{self.base_url}/api/v1/memory/store",
                    json=record,
                    timeout=10
                )
                if response.status_code == 200:
                    if (i + 1) % 10 == 0:
                        print(f"[PROGRESS] Created {i+1}/{num_records} records")
                else:
                    print(f"[ERROR] Failed to create record {i+1}: {response.status_code}")
            except Exception as e:
                print(f"[ERROR] Exception creating record {i+1}: {e}")

        print(f"[OK] Created {num_records} records for user {user_id}")
        return records

    def test_retrieval(self, query: str, user_id: str = None) -> int:
        """测试检索，返回命中数"""
        try:
            payload = {
                "query": query,
                "top_k": 50
            }
            if user_id:
                payload["filter"] = {"user_id": user_id}

            response = requests.post(
                f"{self.base_url}/api/v1/search",
                json=payload,
                timeout=10
            )

            if response.status_code == 200:
                data = response.json()
                results = data.get("results", [])
                return len(results)
            else:
                return 0
        except Exception as e:
            print(f"[ERROR] Retrieval test failed: {e}")
            return 0

    def execute_unlearning(self, user_id: str, epsilon: float = 1.0) -> Dict:
        """执行机器遗忘"""
        print(f"\n[UNLEARNING] Executing unlearning for user {user_id}...")

        start_time = time.time()

        try:
            response = requests.delete(
                f"{self.base_url}/api/v1/users/{user_id}/forget",
                json={
                    "epsilon": epsilon,
                    "max_iterations": 50,
                    "learning_rate": 0.001
                },
                timeout=300  # 5分钟超时
            )

            duration = time.time() - start_time

            if response.status_code == 200:
                data = response.json()
                result = data.get("result", {})

                print(f"[OK] Unlearning completed in {duration:.2f}s")
                print(f"  - Iterations: {result.get('iterations_used', 0)}")
                print(f"  - Convergence: {result.get('convergence_achieved', False)}")
                print(f"  - Vectors removed: {result.get('removed_vector_count', 0)}")

                return {
                    "success": True,
                    "duration": duration,
                    "result": result
                }
            else:
                print(f"[ERROR] Unlearning failed: {response.status_code}")
                return {
                    "success": False,
                    "duration": duration,
                    "error": f"HTTP {response.status_code}"
                }

        except Exception as e:
            duration = time.time() - start_time
            print(f"[ERROR] Unlearning exception: {e}")
            return {
                "success": False,
                "duration": duration,
                "error": str(e)
            }

    def verify_unlearning(self, user_id: str) -> Dict:
        """验证遗忘效果"""
        print(f"\n[VERIFY] Verifying unlearning for user {user_id}...")

        try:
            response = requests.get(
                f"{self.base_url}/api/v1/unlearning/verify/{user_id}",
                timeout=30
            )

            if response.status_code == 200:
                data = response.json()
                print(f"[OK] Verification completed")
                print(f"  - Residual vectors: {data.get('residual_count', 'N/A')}")
                print(f"  - Similarity: {data.get('avg_similarity', 'N/A')}")
                return data
            else:
                print(f"[ERROR] Verification failed: {response.status_code}")
                return {}

        except Exception as e:
            print(f"[ERROR] Verification exception: {e}")
            return {}

    def run_full_test(self, target_user_id: str = "user_unlearn_test",
                      other_user_ids: List[str] = None) -> UnlearningTestResult:
        """运行完整遗忘测试"""

        if other_user_ids is None:
            other_user_ids = ["user_control_1", "user_control_2", "user_control_3"]

        print(f"\n{'='*60}")
        print(f"[TEST] Machine Unlearning Benchmark")
        print(f"{'='*60}")

        # 1. 准备测试数据
        print(f"\n[PHASE 1] Preparing test data...")
        self.create_test_user_data(target_user_id, num_records=50)

        for user_id in other_user_ids:
            self.create_test_user_data(user_id, num_records=30)

        time.sleep(2)  # 等待数据入库

        # 2. 遗忘前检索测试
        print(f"\n[PHASE 2] Testing retrieval BEFORE unlearning...")

        direct_hits_before = self.test_retrieval(f"用户{target_user_id}", target_user_id)
        fuzzy_hits_before = self.test_retrieval("护理记录", target_user_id)
        related_hits_before = self.test_retrieval("血压正常", target_user_id)

        print(f"  Direct query hits: {direct_hits_before}")
        print(f"  Fuzzy query hits: {fuzzy_hits_before}")
        print(f"  Related query hits: {related_hits_before}")

        # 记录其他用户的检索基线
        other_users_baseline = {}
        for user_id in other_user_ids:
            hits = self.test_retrieval("护理记录", user_id)
            other_users_baseline[user_id] = hits
            print(f"  Control user {user_id}: {hits} hits")

        # 3. 执行遗忘
        print(f"\n[PHASE 3] Executing machine unlearning...")
        unlearning_result = self.execute_unlearning(target_user_id, epsilon=1.0)

        if not unlearning_result["success"]:
            print(f"[ERROR] Unlearning failed, aborting test")
            return None

        time.sleep(2)  # 等待索引更新

        # 4. 遗忘后检索测试
        print(f"\n[PHASE 4] Testing retrieval AFTER unlearning...")

        direct_hits_after = self.test_retrieval(f"用户{target_user_id}", target_user_id)
        fuzzy_hits_after = self.test_retrieval("护理记录", target_user_id)
        related_hits_after = self.test_retrieval("血压正常", target_user_id)

        print(f"  Direct query hits: {direct_hits_after} (before: {direct_hits_before})")
        print(f"  Fuzzy query hits: {fuzzy_hits_after} (before: {fuzzy_hits_before})")
        print(f"  Related query hits: {related_hits_after} (before: {related_hits_before})")

        # 5. 检查对其他用户的影响
        print(f"\n[PHASE 5] Checking side effects on other users...")
        other_users_affected = 0
        other_users_total = len(other_user_ids)

        for user_id in other_user_ids:
            hits_after = self.test_retrieval("护理记录", user_id)
            hits_before = other_users_baseline[user_id]

            # 允许±5%的波动
            if abs(hits_after - hits_before) > hits_before * 0.05:
                other_users_affected += 1
                print(f"  [WARNING] User {user_id}: {hits_before} -> {hits_after} (affected)")
            else:
                print(f"  [OK] User {user_id}: {hits_before} -> {hits_after} (stable)")

        # 6. 验证遗忘效果
        verification = self.verify_unlearning(target_user_id)

        # 7. 计算指标
        result = UnlearningTestResult(
            user_id=target_user_id,
            test_phase="complete",
            direct_query_hits=direct_hits_after,
            fuzzy_query_hits=fuzzy_hits_after,
            related_query_hits=related_hits_after,
            vectors_before=direct_hits_before,
            vectors_after=direct_hits_after,
            removal_rate=1.0 - (direct_hits_after / direct_hits_before) if direct_hits_before > 0 else 1.0,
            other_users_affected=other_users_affected,
            other_users_total=other_users_total,
            side_effect_rate=other_users_affected / other_users_total if other_users_total > 0 else 0.0,
            unlearning_duration_sec=unlearning_result["duration"],
            iterations_used=unlearning_result["result"].get("iterations_used", 0),
            convergence_achieved=unlearning_result["result"].get("convergence_achieved", False),
            timestamp=datetime.now().isoformat()
        )

        # 8. 保存结果
        self.save_result(result)
        self.print_result(result)

        return result

    def save_result(self, result: UnlearningTestResult):
        """保存测试结果"""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        result_file = self.output_dir / f"unlearning_test_{timestamp}.json"

        with open(result_file, 'w', encoding='utf-8') as f:
            json.dump(asdict(result), f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] Result saved to {result_file}")

    def print_result(self, result: UnlearningTestResult):
        """打印测试结果"""
        print(f"\n{'='*60}")
        print(f"[RESULT] Machine Unlearning Test")
        print(f"{'='*60}")
        print(f"User ID: {result.user_id}")
        print(f"\n[Retrieval After Unlearning]")
        print(f"  Direct query hits:   {result.direct_query_hits}")
        print(f"  Fuzzy query hits:    {result.fuzzy_query_hits}")
        print(f"  Related query hits:  {result.related_query_hits}")
        print(f"\n[Unlearning Effectiveness]")
        print(f"  Vectors before:      {result.vectors_before}")
        print(f"  Vectors after:       {result.vectors_after}")
        print(f"  Removal rate:        {result.removal_rate*100:.2f}%")
        print(f"  Residual rate:       {(1-result.removal_rate)*100:.2f}%")
        print(f"\n[Side Effects]")
        print(f"  Other users total:   {result.other_users_total}")
        print(f"  Other users affected: {result.other_users_affected}")
        print(f"  Side effect rate:    {result.side_effect_rate*100:.2f}%")
        print(f"\n[Performance]")
        print(f"  Duration:            {result.unlearning_duration_sec:.2f}s")
        print(f"  Iterations:          {result.iterations_used}")
        print(f"  Convergence:         {'Yes' if result.convergence_achieved else 'No'}")
        print(f"{'='*60}\n")

        # 评估
        if result.removal_rate >= 0.95:
            print("[PASS] Unlearning is highly effective (>95% removal)")
        elif result.removal_rate >= 0.90:
            print("[PASS] Unlearning is effective (>90% removal)")
        else:
            print("[FAIL] Unlearning is insufficient (<90% removal)")

        if result.side_effect_rate <= 0.05:
            print("[PASS] Side effects are minimal (<5%)")
        else:
            print("[WARNING] Side effects are significant (>5%)")

def main():
    """主函数"""
    benchmark = UnlearningBenchmark()

    print("[INFO] Starting machine unlearning benchmark...")
    print("[INFO] This test will:")
    print("  1. Create test user data (50 records)")
    print("  2. Test retrieval before unlearning")
    print("  3. Execute machine unlearning")
    print("  4. Test retrieval after unlearning")
    print("  5. Verify side effects on other users")

    result = benchmark.run_full_test(
        target_user_id="user_unlearn_test_001",
        other_user_ids=["user_control_001", "user_control_002", "user_control_003"]
    )

    if result:
        print("\n[DONE] Machine unlearning benchmark completed!")
    else:
        print("\n[ERROR] Machine unlearning benchmark failed!")

if __name__ == "__main__":
    main()
