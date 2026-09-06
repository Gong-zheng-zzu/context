#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Context-Keeper 大规模安全功能测试脚本（2000个样本）
"""

import json
import time
import requests
from datetime import datetime
from typing import Dict, List, Tuple
from collections import defaultdict
import os

# 彩色终端输出
class Colors:
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'

def print_header(text: str):
    print(f"\n{Colors.HEADER}{Colors.BOLD}{'='*80}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{text.center(80)}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{'='*80}{Colors.ENDC}\n")

def print_success(text: str):
    print(f"{Colors.OKGREEN}[OK] {text}{Colors.ENDC}")

def print_error(text: str):
    print(f"{Colors.FAIL}[ERROR] {text}{Colors.ENDC}")

def print_warning(text: str):
    print(f"{Colors.WARNING}[WARN] {text}{Colors.ENDC}")

def print_info(text: str):
    print(f"{Colors.OKCYAN}[INFO] {text}{Colors.ENDC}")

# 配置
API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_large.json"
RESULTS_DIR = "test_results"

class SecurityTester:
    def __init__(self):
        self.results = {}
        self.response_times = []
        self.jwt_token = None

    def check_server_health(self) -> bool:
        """检查服务器健康状态"""
        print_info("检查服务器健康状态...")
        try:
            response = requests.get(f"{API_BASE_URL}/health", timeout=5)
            if response.status_code == 200:
                print_success(f"服务器运行正常: {API_BASE_URL}")
                return True
            else:
                print_error(f"服务器响应异常: {response.status_code}")
                return False
        except Exception as e:
            print_error(f"无法连接到服务器: {e}")
            return False

    def login(self) -> bool:
        """登录获取JWT Token"""
        print_info("登录获取JWT Token...")
        try:
            response = requests.post(
                f"{API_BASE_URL}/api/role/login",
                json={
                    "user_id": "test_user",
                    "password": "health_assistant_2024",
                    "role": "caregiver",
                    "workspace_id": "default"
                },
                timeout=5
            )
            if response.status_code == 200:
                data = response.json()
                if data.get("success") and "data" in data and "token" in data["data"]:
                    self.jwt_token = data["data"]["token"]
                    print_success("成功获取JWT Token")
                    return True
            print_error(f"登录失败: {response.status_code}")
            return False
        except Exception as e:
            print_error(f"登录请求失败: {e}")
            return False

    def detect_sensitive_info(self, content: str) -> Tuple[Dict, float]:
        """调用安全检测API"""
        start_time = time.time()
        try:
            headers = {
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.jwt_token}"
            }

            response = requests.post(
                f"{API_BASE_URL}/api/security/detect",
                json={
                    "content": content,
                    "user_id": "test_user",
                    "session_id": "test_session"
                },
                headers=headers,
                timeout=5
            )

            response_time = (time.time() - start_time) * 1000
            self.response_times.append(response_time)

            if response.status_code == 200:
                return response.json(), response_time
            else:
                return {"error": f"HTTP {response.status_code}"}, response_time
        except Exception as e:
            response_time = (time.time() - start_time) * 1000
            return {"error": str(e)}, response_time

    def load_test_data(self) -> Dict:
        """加载测试数据"""
        print_info(f"加载测试数据: {TEST_DATA_FILE}")
        try:
            with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
                data = json.load(f)
            print_success("成功加载测试数据")
            return data
        except Exception as e:
            print_error(f"加载测试数据失败: {e}")
            return {}

    def test_sensitive_info_detection(self, test_data: Dict):
        """测试敏感信息检测功能"""
        print_header("大规模敏感信息检测测试 (2000个样本)")

        stats = defaultdict(lambda: {"TP": 0, "TN": 0, "FP": 0, "FN": 0, "total": 0})

        # 测试各类敏感信息
        for info_type in ["id_card", "phone", "bank_card", "email"]:
            if info_type not in test_data:
                continue

            print_info(f"\n测试 {info_type} 检测...")
            samples = test_data[info_type]

            # 显示进度
            total_samples = len(samples)
            print(f"  总样本数: {total_samples}")

            for i, sample in enumerate(samples, 1):
                value = sample["value"]
                label = sample["label"]
                should_detect = sample["should_detect"]

                # 每100个样本显示一次进度
                if i % 100 == 0 or i == total_samples:
                    print(f"  进度: {i}/{total_samples} ({i/total_samples*100:.1f}%)", end='\r')

                response, resp_time = self.detect_sensitive_info(value)
                stats[info_type]["total"] += 1

                # 检查是否检测到敏感信息
                detected = False
                if "error" not in response:
                    sensitive_infos = response.get("sensitive_infos", [])
                    if sensitive_infos:
                        detected = True

                # 计算TP/TN/FP/FN
                if should_detect and detected:
                    stats[info_type]["TP"] += 1
                elif not should_detect and not detected:
                    stats[info_type]["TN"] += 1
                elif not should_detect and detected:
                    stats[info_type]["FP"] += 1
                elif should_detect and not detected:
                    stats[info_type]["FN"] += 1

                time.sleep(0.01)  # 避免请求过快

            print()  # 换行

        # 计算总体指标
        self.print_detection_summary(stats)
        return stats

    def print_detection_summary(self, stats: Dict):
        """打印检测统计摘要"""
        print(f"\n{Colors.BOLD}敏感信息检测统计:{Colors.ENDC}\n")

        total_tp = total_tn = total_fp = total_fn = 0

        for info_type, metrics in stats.items():
            tp = metrics["TP"]
            tn = metrics["TN"]
            fp = metrics["FP"]
            fn = metrics["FN"]
            total = metrics["total"]

            total_tp += tp
            total_tn += tn
            total_fp += fp
            total_fn += fn

            accuracy = (tp + tn) / total * 100 if total > 0 else 0
            recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
            precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0
            f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0

            print(f"  {info_type}:")
            print(f"    样本数: {total}")
            print(f"    准确率: {accuracy:.1f}%")
            print(f"    召回率: {recall:.1f}%")
            print(f"    精确率: {precision:.1f}%")
            print(f"    F1分数: {f1:.1f}%")
            print(f"    混淆矩阵: TP={tp}, TN={tn}, FP={fp}, FN={fn}")
            print()

        # 总体统计
        total = total_tp + total_tn + total_fp + total_fn
        accuracy = (total_tp + total_tn) / total * 100 if total > 0 else 0
        recall = total_tp / (total_tp + total_fn) * 100 if (total_tp + total_fn) > 0 else 0
        precision = total_tp / (total_tp + total_fp) * 100 if (total_tp + total_fp) > 0 else 0
        f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0

        print(f"  {Colors.BOLD}总体统计:{Colors.ENDC}")
        print(f"    总测试数: {total}")

        acc_color = Colors.OKGREEN if accuracy >= 95 else Colors.WARNING
        print(f"    准确率: {acc_color}{accuracy:.1f}%{Colors.ENDC}")

        rec_color = Colors.OKGREEN if recall >= 90 else Colors.WARNING
        print(f"    召回率: {rec_color}{recall:.1f}%{Colors.ENDC}")

        prec_color = Colors.OKGREEN if precision >= 85 else Colors.WARNING
        print(f"    精确率: {prec_color}{precision:.1f}%{Colors.ENDC}")

        f1_color = Colors.OKGREEN if f1 >= 90 else Colors.WARNING
        print(f"    F1分数: {f1_color}{f1:.1f}%{Colors.ENDC}")

        print(f"    混淆矩阵: TP={total_tp}, TN={total_tn}, FP={total_fp}, FN={total_fn}")

    def print_performance_summary(self):
        """打印性能统计"""
        print_header("性能指标统计")

        if not self.response_times:
            print_warning("没有性能数据")
            return

        avg_time = sum(self.response_times) / len(self.response_times)
        sorted_times = sorted(self.response_times)
        p50 = sorted_times[len(sorted_times) // 2]
        p95 = sorted_times[int(len(sorted_times) * 0.95)]
        p99 = sorted_times[int(len(sorted_times) * 0.99)]

        print(f"  总请求数: {len(self.response_times)}")

        avg_color = Colors.OKGREEN if avg_time < 500 else Colors.WARNING
        print(f"  平均响应时间: {avg_color}{avg_time:.0f}ms{Colors.ENDC}")

        print(f"  P50响应时间: {p50:.0f}ms")

        p95_color = Colors.OKGREEN if p95 < 1000 else Colors.WARNING
        print(f"  P95响应时间: {p95_color}{p95:.0f}ms{Colors.ENDC}")

        print(f"  P99响应时间: {p99:.0f}ms")
        print(f"  最大响应时间: {max(self.response_times):.0f}ms")
        print(f"  最小响应时间: {min(self.response_times):.0f}ms")

    def run(self):
        """运行完整测试"""
        print_header("Context-Keeper 大规模安全检测测试")
        print_info(f"测试时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

        # 1. 检查服务器
        if not self.check_server_health():
            return False

        # 2. 登录
        if not self.login():
            return False

        # 3. 加载测试数据
        test_data = self.load_test_data()
        if not test_data:
            return False

        # 4. 运行测试
        stats = self.test_sensitive_info_detection(test_data)

        # 5. 性能统计
        self.print_performance_summary()

        print_header("测试总结")
        print_success("测试完成！")

        return True

if __name__ == "__main__":
    # 创建结果目录
    os.makedirs(RESULTS_DIR, exist_ok=True)

    tester = SecurityTester()
    success = tester.run()

    exit(0 if success else 1)
