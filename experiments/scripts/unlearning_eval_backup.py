#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
机器遗忘实验脚本
包装unlearning_benchmark，测试梯度正交投影算法

功能：
1. 完整模式：5个场景测试
2. 快速模式：1个场景演示
3. 交互模式：自定义user_id遗忘测试
"""

import json
import requests
import argparse
import time
from pathlib import Path
from datetime import datetime
from typing import List, Dict
from dataclasses import dataclass, asdict
import sys

@dataclass
class UnlearningResult:
    """遗忘结果"""
    scenario_name: str
    target_user_id: str
    config_name: str

    # 遗忘前
    before_hit_count: int
    before_hit_rate: float

    # 遗忘后
    after_hit_count: int
    after_hit_rate: float

    # 效果指标
    unlearning_rate: float  # 遗忘率
    side_effect_rate: float  # 副作用率（其他用户受影响程度）

    # 性能指标
    unlearning_time_ms: float
    convergence_iterations: int

    timestamp: str

@dataclass
class ScenarioMetrics:
    """场景指标"""
    scenario_name: str
    config_name: str

    avg_unlearning_rate: float
    avg_side_effect_rate: float
    avg_unlearning_time_ms: float
    avg_convergence_iterations: float

class UnlearningEvaluator:
    """机器遗忘评估器"""

    def __init__(self, base_url: str = "http://localhost:8088"):
        self.base_url = base_url
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets" / "nursing_data"
        self.configs_dir = self.project_root / "configs"
        self.results_dir = self.project_root / "results" / "raw"
        self.results_dir.mkdir(parents=True, exist_ok=True)

        # 5个测试场景
        self.scenarios = [
            {
                "name": "user_data_deletion",
                "description": "用户数据完全删除（GDPR）",
                "test_users": ["elder_001", "elder_002"]
            },
            {
                "name": "time_range_forgetting",
                "description": "特定时间段遗忘",
                "test_users": ["elder_003"],
                "time_range": "2026-06-01 to 2026-06-07"
            },
            {
                "name": "event_type_forgetting",
                "description": "特定事件类型遗忘",
                "test_users": ["elder_004"],
                "event_type": "fall"
            },
            {
                "name": "cascading_deletion",
                "description": "关联数据遗忘",
                "test_users": ["elder_005"]
            },
            {
                "name": "sensitive_info_forgetting",
                "description": "敏感信息遗忘",
                "test_users": ["elder_006"]
            }
        ]

    def query_user_data(self, user_id: str) -> int:
        """查询用户数据命中次数"""
        url = f"{self.base_url}/api/v1/retrieve"

        try:
            response = requests.post(
                url,
                json={
                    "query": f"查询{user_id}的所有记录",
                    "user_id": user_id
                },
                timeout=10
            )

            if response.status_code == 200:
                data = response.json()
                # 统计命中数量
                contexts = data.get("contexts", [])
                hit_count = len([c for c in contexts if user_id in str(c)])
                return hit_count
            else:
                return 0

        except Exception as e:
            print(f"[ERROR] 查询失败: {e}")
            return 0

    def trigger_unlearning(self, user_id: str, scenario: Dict) -> Dict:
        """触发遗忘操作"""
        url = f"{self.base_url}/api/v1/unlearning/forget"

        payload = {
            "user_id": user_id,
            "scenario": scenario["name"]
        }

        # 添加场景特定参数
        if "time_range" in scenario:
            payload["time_range"] = scenario["time_range"]
        if "event_type" in scenario:
            payload["event_type"] = scenario["event_type"]

        start_time = time.time()

        try:
            response = requests.post(
                url,
                json=payload,
                timeout=120  # 遗忘操作可能较慢
            )

            unlearning_time_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return {
                    "success": True,
                    "unlearning_time_ms": unlearning_time_ms,
                    "convergence_iterations": data.get("iterations", 0),
                    "message": data.get("message", "")
                }
            else:
                return {
                    "success": False,
                    "unlearning_time_ms": unlearning_time_ms,
                    "convergence_iterations": 0,
                    "message": f"Error: {response.status_code}"
                }

        except Exception as e:
            unlearning_time_ms = (time.time() - start_time) * 1000
            print(f"[ERROR] 遗忘失败: {e}")
            return {
                "success": False,
                "unlearning_time_ms": unlearning_time_ms,
                "convergence_iterations": 0,
                "message": str(e)
            }

    def measure_side_effects(self, excluded_user_ids: List[str], before_counts: Dict[str, int] = None) -> Dict:
        """
        增强副作用测量：遗忘前后对照测量

        Args:
            excluded_user_ids: 被遗忘的用户ID列表
            before_counts: 遗忘前的对照组数据（如果为None则不计算变化率）

        Returns:
            {
                "before_avg_count": float,
                "after_avg_count": float,
                "side_effect_rate": float,
                "affected_users": List[str],
                "data_loss_rate": float
            }
        """
        # 随机采样对照用户（不包含目标用户）
        control_users = ["elder_010", "elder_011", "elder_012", "elder_020", "elder_033"]
        control_users = [u for u in control_users if u not in excluded_user_ids]

        if not control_users:
            return {
                "before_avg_count": 0,
                "after_avg_count": 0,
                "side_effect_rate": 0.0,
                "affected_users": [],
                "data_loss_rate": 0.0
            }

        # 测量遗忘后的数据量
        after_counts = {}
        for user in control_users:
            count = self.query_user_data(user)
            after_counts[user] = count

        after_avg = sum(after_counts.values()) / len(after_counts) if after_counts else 0

        # 如果有遗忘前的数据，计算变化
        if before_counts:
            before_avg = sum(before_counts.values()) / len(before_counts) if before_counts else 0

            # 识别受影响的用户
            affected_users = []
            for user in control_users:
                before = before_counts.get(user, 0)
                after = after_counts.get(user, 0)
                if before > 0 and after < before:
                    affected_users.append(user)

            # 副作用率 = 受影响用户数量 / 总对照用户数量
            side_effect_rate = len(affected_users) / len(control_users) if control_users else 0.0

            # 数据量变化率
            data_loss_rate = (before_avg - after_avg) / before_avg if before_avg > 0 else 0.0

            # 取更严格的指标
            side_effect_rate = max(side_effect_rate, data_loss_rate)

            return {
                "before_avg_count": before_avg,
                "after_avg_count": after_avg,
                "side_effect_rate": side_effect_rate,
                "affected_users": affected_users,
                "data_loss_rate": data_loss_rate
            }
        else:
            # 没有前置数据，仅返回当前状态
            return {
                "before_avg_count": 0,
                "after_avg_count": after_avg,
                "side_effect_rate": 0.0,
                "affected_users": [],
                "data_loss_rate": 0.0
            }

    def test_scenario(self, scenario: Dict, config_name: str) -> UnlearningResult:
        """测试单个场景"""
        scenario_name = scenario["name"]
        test_users = scenario["test_users"]

        # 选择第一个测试用户
        target_user_id = test_users[0]

        print(f"\n[场景] {scenario['description']}")
        print(f"[目标] {target_user_id}")

        # 1. 遗忘前查询（目标用户+对照组）
        print("  [1/4] 遗忘前查询...")
        before_hit_count = self.query_user_data(target_user_id)
        before_hit_rate = 1.0 if before_hit_count > 0 else 0.0

        # 测量对照组遗忘前状态
        control_users = ["elder_010", "elder_011", "elder_012", "elder_020", "elder_033"]
        control_users = [u for u in control_users if u not in test_users]
        before_control_counts = {}
        for user in control_users:
            before_control_counts[user] = self.query_user_data(user)

        # 2. 执行遗忘
        print("  [2/4] 执行遗忘...")
        unlearning_result = self.trigger_unlearning(target_user_id, scenario)

        if not unlearning_result["success"]:
            print(f"  [WARN] 遗忘失败: {unlearning_result['message']}")

        # 3. 遗忘后查询
        print("  [3/4] 遗忘后查询...")
        time.sleep(2)  # 等待索引更新
        after_hit_count = self.query_user_data(target_user_id)
        after_hit_rate = 1.0 if after_hit_count > 0 else 0.0

        # 4. 测量副作用（对比遗忘前后）
        print("  [4/4] 测量副作用...")
        side_effect_metrics = self.measure_side_effects(test_users, before_control_counts)

        # 计算遗忘率
        unlearning_rate = 1.0 - after_hit_rate if before_hit_rate > 0 else 0.0

        result = UnlearningResult(
            scenario_name=scenario_name,
            target_user_id=target_user_id,
            config_name=config_name,
            before_hit_count=before_hit_count,
            before_hit_rate=before_hit_rate,
            after_hit_count=after_hit_count,
            after_hit_rate=after_hit_rate,
            unlearning_rate=unlearning_rate,
            side_effect_rate=side_effect_metrics["side_effect_rate"],
            unlearning_time_ms=unlearning_result["unlearning_time_ms"],
            convergence_iterations=unlearning_result["convergence_iterations"],
            timestamp=datetime.now().isoformat()
        )

        print(f"  遗忘率: {unlearning_rate*100:.1f}%")
        print(f"  副作用率: {side_effect_metrics['side_effect_rate']*100:.1f}%")
        if side_effect_metrics["affected_users"]:
            print(f"  受影响用户: {', '.join(side_effect_metrics['affected_users'])}")
        print(f"  耗时: {result.unlearning_time_ms:.0f}ms")

        return result

    def run_experiment(self, config_name: str, selected_scenarios: List[str] = None) -> List[UnlearningResult]:
        """运行实验"""
        results = []

        # 筛选场景
        scenarios = self.scenarios
        if selected_scenarios:
            scenarios = [s for s in scenarios if s["name"] in selected_scenarios]

        print(f"\n[CONFIG] 测试配置: {config_name}")
        print(f"[INFO] 将测试 {len(scenarios)} 个场景")

        for scenario in scenarios:
            result = self.test_scenario(scenario, config_name)
            results.append(result)

        return results

    def calculate_metrics(self, results: List[UnlearningResult]) -> ScenarioMetrics:
        """计算指标"""
        if not results:
            return None

        config_name = results[0].config_name
        scenario_name = "all_scenarios"

        avg_unlearning_rate = sum(r.unlearning_rate for r in results) / len(results)
        avg_side_effect_rate = sum(r.side_effect_rate for r in results) / len(results)
        avg_time = sum(r.unlearning_time_ms for r in results) / len(results)
        avg_iterations = sum(r.convergence_iterations for r in results) / len(results)

        return ScenarioMetrics(
            scenario_name=scenario_name,
            config_name=config_name,
            avg_unlearning_rate=avg_unlearning_rate,
            avg_side_effect_rate=avg_side_effect_rate,
            avg_unlearning_time_ms=avg_time,
            avg_convergence_iterations=avg_iterations
        )

    def save_results(self, results: List[UnlearningResult], metrics: ScenarioMetrics):
        """保存结果"""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = self.results_dir / f"unlearning_{timestamp}.json"

        # 检测execution_mode（unlearning默认为live_api，因为需要真实服务）
        execution_mode = "live_api"

        output = {
            "execution_mode": execution_mode,
            "timestamp": timestamp,
            "total_tests": len(results),
            "config": results[0].config_name if results else "unknown",
            "detailed_results": [asdict(r) for r in results],
            "aggregated_metrics": asdict(metrics) if metrics else {}
        }

        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(output, f, ensure_ascii=False, indent=2)

        print(f"\n[SAVE] 结果已保存: {output_file}")
        return output_file

    def print_summary(self, metrics: ScenarioMetrics):
        """打印摘要"""
        print("\n" + "="*60)
        print("机器遗忘实验结果摘要")
        print("="*60)

        if metrics:
            print(f"\n[{metrics.config_name}]")
            print(f"  平均遗忘率: {metrics.avg_unlearning_rate*100:.1f}%")
            print(f"  平均副作用率: {metrics.avg_side_effect_rate*100:.1f}%")
            print(f"  平均耗时: {metrics.avg_unlearning_time_ms:.1f}ms")
            print(f"  平均收敛迭代: {metrics.avg_convergence_iterations:.1f}")

    def interactive_mode(self):
        """交互模式"""
        print("\n" + "="*60)
        print("交互式遗忘测试")
        print("="*60)
        print("输入user_id进行遗忘测试")
        print("输入 'q' 退出")

        while True:
            user_id = input("\nUser ID> ").strip()

            if user_id.lower() == 'q':
                break

            if not user_id:
                continue

            # 遗忘前查询
            before = self.query_user_data(user_id)
            print(f"遗忘前命中数: {before}")

            # 执行遗忘
            scenario = {"name": "user_data_deletion", "test_users": [user_id]}
            result = self.trigger_unlearning(user_id, scenario)

            print(f"遗忘耗时: {result['unlearning_time_ms']:.0f}ms")
            print(f"收敛迭代: {result['convergence_iterations']}")

            # 遗忘后查询
            time.sleep(2)
            after = self.query_user_data(user_id)
            print(f"遗忘后命中数: {after}")

            unlearning_rate = (before - after) / before * 100 if before > 0 else 0
            print(f"遗忘率: {unlearning_rate:.1f}%")


def main():
    parser = argparse.ArgumentParser(description="机器遗忘实验")
    parser.add_argument("--scenarios", type=int, default=5, help="场景数量")
    parser.add_argument("--scenario-name", help="指定场景名称")
    parser.add_argument("--config", default="full_system", help="配置名称")
    parser.add_argument("--interactive", action="store_true", help="交互模式")
    parser.add_argument("--user-id", help="指定user_id测试")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API基础URL")

    args = parser.parse_args()

    evaluator = UnlearningEvaluator(base_url=args.base_url)

    # 交互模式
    if args.interactive:
        evaluator.interactive_mode()
        return

    # 单用户测试
    if args.user_id:
        scenario = {"name": "user_data_deletion", "test_users": [args.user_id]}
        result = evaluator.test_scenario(scenario, args.config)
        print(f"\n遗忘率: {result.unlearning_rate*100:.1f}%")
        return

    # 选择场景
    selected_scenarios = None
    if args.scenario_name:
        selected_scenarios = [args.scenario_name]
    elif args.scenarios < 5:
        # 选择前N个场景
        selected_scenarios = [s["name"] for s in evaluator.scenarios[:args.scenarios]]

    # 运行实验
    results = evaluator.run_experiment(args.config, selected_scenarios)

    # 计算指标
    metrics = evaluator.calculate_metrics(results)

    # 保存结果
    evaluator.save_results(results, metrics)

    # 打印摘要
    evaluator.print_summary(metrics)


if __name__ == "__main__":
    main()
