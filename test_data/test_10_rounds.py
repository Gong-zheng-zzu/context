#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
10轮测试脚本 - 使用200个样本测试准确率稳定性
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
    print(f"{Colors.OKGREEN}[PASS] {text}{Colors.ENDC}")

def print_info(text: str):
    print(f"{Colors.OKCYAN}[INFO] {text}{Colors.ENDC}")

# 配置
API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

class MultiRoundTester:
    def __init__(self):
        self.jwt_token = None
        self.all_rounds_results = []

    def login(self) -> bool:
        """登录获取JWT Token"""
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
                    return True
            return False
        except Exception as e:
            print(f"登录失败: {e}")
            return False

    def detect_sensitive_info(self, content: str, sample_id: int = 0) -> Tuple[bool, float]:
        """调用安全检测API"""
        start_time = time.time()
        try:
            headers = {
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.jwt_token}"
            }

            # 🔥 使用唯一的session_id，避免缓存干扰
            response = requests.post(
                f"{API_BASE_URL}/api/security/detect",
                json={
                    "content": content,
                    "user_id": "test_user",
                    "session_id": f"test_session_{sample_id}"
                },
                headers=headers,
                timeout=5
            )

            response_time = (time.time() - start_time) * 1000

            if response.status_code == 200:
                result = response.json()
                detected = len(result.get("sensitive_infos", [])) > 0
                return detected, response_time
            else:
                return False, response_time
        except Exception as e:
            return False, 0

    def load_test_data(self) -> Dict:
        """加载测试数据"""
        try:
            with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception as e:
            print(f"加载测试数据失败: {e}")
            return {}

    def run_single_round(self, round_num: int, test_data: Dict) -> Dict:
        """运行单轮测试"""
        print(f"\n{Colors.BOLD}第 {round_num}/10 轮测试{Colors.ENDC}")

        # 🔥 每轮测试前重新登录，避免Token过期或缓存问题
        if round_num > 1:
            if not self.login():
                print(f"第{round_num}轮登录失败")
                return {"round": round_num, "accuracy": 0, "recall": 0, "precision": 0, "f1": 0, "avg_time": 0, "tp": 0, "tn": 0, "fp": 0, "fn": 0}

        stats = {"TP": 0, "TN": 0, "FP": 0, "FN": 0, "total": 0}
        response_times = []

        # 🔥 全局样本计数器，确保每个样本有唯一ID（包含轮次信息）
        sample_counter = (round_num - 1) * 1000

        # 测试所有类型
        for info_type in ["id_card", "phone", "bank_card", "email"]:
            if info_type not in test_data:
                continue

            samples = test_data[info_type]
            for sample in samples:
                value = sample["value"]
                should_detect = sample["should_detect"]

                # 🔥 传入唯一的sample_id
                detected, resp_time = self.detect_sensitive_info(value, sample_counter)
                sample_counter += 1

                response_times.append(resp_time)
                stats["total"] += 1

                # 计算TP/TN/FP/FN
                if should_detect and detected:
                    stats["TP"] += 1
                elif not should_detect and not detected:
                    stats["TN"] += 1
                elif not should_detect and detected:
                    stats["FP"] += 1
                elif should_detect and not detected:
                    stats["FN"] += 1

                time.sleep(0.2)  # 降低请求速率至5 req/sec，避免触发速率限制（300 req/min）

        # 计算指标
        total = stats["total"]
        tp, tn, fp, fn = stats["TP"], stats["TN"], stats["FP"], stats["FN"]

        accuracy = (tp + tn) / total * 100 if total > 0 else 0
        recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
        precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0
        f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0
        avg_time = sum(response_times) / len(response_times) if response_times else 0

        result = {
            "round": round_num,
            "accuracy": accuracy,
            "recall": recall,
            "precision": precision,
            "f1": f1,
            "avg_time": avg_time,
            "tp": tp,
            "tn": tn,
            "fp": fp,
            "fn": fn
        }

        # 显示结果
        print(f"  准确率: {accuracy:.1f}%  召回率: {recall:.1f}%  精确率: {precision:.1f}%  F1: {f1:.1f}%  平均响应: {avg_time:.0f}ms")

        return result

    def print_summary(self):
        """打印10轮测试总结"""
        print_header("10轮测试总结")

        # 计算平均值和标准差
        accuracies = [r["accuracy"] for r in self.all_rounds_results]
        recalls = [r["recall"] for r in self.all_rounds_results]
        precisions = [r["precision"] for r in self.all_rounds_results]
        f1s = [r["f1"] for r in self.all_rounds_results]
        times = [r["avg_time"] for r in self.all_rounds_results]

        def calc_stats(values):
            avg = sum(values) / len(values)
            variance = sum((x - avg) ** 2 for x in values) / len(values)
            std = variance ** 0.5
            return avg, std, min(values), max(values)

        acc_avg, acc_std, acc_min, acc_max = calc_stats(accuracies)
        rec_avg, rec_std, rec_min, rec_max = calc_stats(recalls)
        prec_avg, prec_std, prec_min, prec_max = calc_stats(precisions)
        f1_avg, f1_std, f1_min, f1_max = calc_stats(f1s)
        time_avg, time_std, time_min, time_max = calc_stats(times)

        print(f"{Colors.BOLD}准确率 (Accuracy):{Colors.ENDC}")
        print(f"  平均值: {acc_avg:.2f}%")
        print(f"  标准差: {acc_std:.2f}%")
        print(f"  范围: {acc_min:.1f}% - {acc_max:.1f}%")

        print(f"\n{Colors.BOLD}召回率 (Recall):{Colors.ENDC}")
        print(f"  平均值: {rec_avg:.2f}%")
        print(f"  标准差: {rec_std:.2f}%")
        print(f"  范围: {rec_min:.1f}% - {rec_max:.1f}%")

        print(f"\n{Colors.BOLD}精确率 (Precision):{Colors.ENDC}")
        print(f"  平均值: {prec_avg:.2f}%")
        print(f"  标准差: {prec_std:.2f}%")
        print(f"  范围: {prec_min:.1f}% - {prec_max:.1f}%")

        print(f"\n{Colors.BOLD}F1分数:{Colors.ENDC}")
        print(f"  平均值: {f1_avg:.2f}%")
        print(f"  标准差: {f1_std:.2f}%")
        print(f"  范围: {f1_min:.1f}% - {f1_max:.1f}%")

        print(f"\n{Colors.BOLD}响应时间:{Colors.ENDC}")
        print(f"  平均值: {time_avg:.0f}ms")
        print(f"  标准差: {time_std:.0f}ms")
        print(f"  范围: {time_min:.0f}ms - {time_max:.0f}ms")

        # 评估稳定性
        print(f"\n{Colors.BOLD}稳定性评估:{Colors.ENDC}")
        if acc_std < 2:
            print_success(f"准确率非常稳定 (标准差 {acc_std:.2f}%)")
        elif acc_std < 5:
            print_success(f"准确率较稳定 (标准差 {acc_std:.2f}%)")
        else:
            print(f"  准确率波动较大 (标准差 {acc_std:.2f}%)")

    def run(self):
        """运行10轮测试"""
        print_header("Context-Keeper 10轮稳定性测试 (阈值=0.45, 200样本)")
        print_info(f"测试时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

        # 登录
        print_info("登录获取JWT Token...")
        if not self.login():
            print("登录失败")
            return False

        # 加载测试数据
        print_info("加载测试数据...")
        test_data = self.load_test_data()
        if not test_data:
            return False

        # 运行10轮测试
        for round_num in range(1, 11):
            result = self.run_single_round(round_num, test_data)
            self.all_rounds_results.append(result)
            time.sleep(0.5)  # 轮次间隔

        # 打印总结
        self.print_summary()

        print_header("测试完成")
        return True

if __name__ == "__main__":
    tester = MultiRoundTester()
    success = tester.run()
    exit(0 if success else 1)
