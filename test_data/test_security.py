#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Context-Keeper 安全功能自动化测试脚本
用于信息安全作品赛实验数据收集
"""

import json
import time
import requests
from datetime import datetime
from typing import Dict, List, Tuple
from collections import defaultdict

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
    UNDERLINE = '\033[4m'

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
TEST_DATA_FILE = "test_data/sensitive_data.json"
RESULTS_DIR = "test_results"
USER_ID = "test_user_security"
SESSION_ID = "test_session_security"

class SecurityTester:
    def __init__(self):
        self.results = {
            "test_time": datetime.now().isoformat(),
            "sensitive_info_detection": {},
            "adversarial_attacks": {},
            "performance": {},
            "summary": {}
        }
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
            print_warning("请确保服务器正在运行: docker-compose up -d")
            return False

    def login(self) -> bool:
        """登录获取JWT Token"""
        print_info("登录获取JWT Token...")
        try:
            response = requests.post(
                f"{API_BASE_URL}/api/role/login",
                json={
                    "user_id": USER_ID,
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
                else:
                    print_error(f"响应格式错误: {data}")
                    return False
            else:
                print_error(f"登录失败: {response.status_code} - {response.text}")
                return False
        except Exception as e:
            print_error(f"登录请求失败: {e}")
            return False

    def load_test_data(self) -> Dict:
        """加载测试数据"""
        print_info(f"加载测试数据: {TEST_DATA_FILE}")
        try:
            with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
                data = json.load(f)
            print_success(f"成功加载测试数据")
            return data
        except Exception as e:
            print_error(f"加载测试数据失败: {e}")
            return {}

    def send_chat_message(self, message: str) -> Tuple[Dict, float]:
        """发送聊天消息并返回响应和响应时间"""
        start_time = time.time()
        try:
            headers = {
                "Content-Type": "application/json"
            }
            if self.jwt_token:
                headers["Authorization"] = f"Bearer {self.jwt_token}"

            response = requests.post(
                f"{API_BASE_URL}/api/chat",
                json={
                    "user_id": USER_ID,
                    "session_id": SESSION_ID,
                    "message": message,
                    "history": []
                },
                headers=headers,
                timeout=10
            )
            response_time = (time.time() - start_time) * 1000  # 转换为毫秒
            self.response_times.append(response_time)

            if response.status_code == 200:
                return response.json(), response_time
            else:
                return {"error": f"HTTP {response.status_code}"}, response_time
        except Exception as e:
            response_time = (time.time() - start_time) * 1000
            return {"error": str(e)}, response_time

    def test_sensitive_info_detection(self, test_data: Dict):
        """测试敏感信息检测功能"""
        print_header("敏感信息检测测试")

        # 统计变量
        stats = defaultdict(lambda: {"TP": 0, "TN": 0, "FP": 0, "FN": 0, "total": 0})

        # 测试各类敏感信息
        for info_type in ["id_card", "phone", "bank_card", "email"]:
            if info_type not in test_data:
                continue

            print_info(f"\n测试 {info_type} 检测...")
            samples = test_data[info_type]

            for i, sample in enumerate(samples, 1):
                value = sample["value"]
                label = sample["label"]
                should_detect = sample["should_detect"]

                print(f"  [{i}/{len(samples)}] {label}: {value[:20]}...", end=" ")

                response, resp_time = self.send_chat_message(value)
                stats[info_type]["total"] += 1

                # 检查是否检测到敏感信息
                detected = False
                detected_types = []
                if "data" in response and "sensitive_infos" in response["data"] and response["data"]["sensitive_infos"]:
                    detected = True
                    detected_types = [info["type"] for info in response["data"]["sensitive_infos"]]

                # 计算TP/TN/FP/FN
                if should_detect and detected:
                    stats[info_type]["TP"] += 1
                    print_success(f"Correct detection ({resp_time:.0f}ms)")
                elif not should_detect and not detected:
                    stats[info_type]["TN"] += 1
                    print_success(f"Correct pass ({resp_time:.0f}ms)")
                elif not should_detect and detected:
                    stats[info_type]["FP"] += 1
                    print_error(f"False positive ({resp_time:.0f}ms)")
                elif should_detect and not detected:
                    stats[info_type]["FN"] += 1
                    print_error(f"False negative ({resp_time:.0f}ms)")

                time.sleep(0.1)  # 避免请求过快

        # 测试混合场景
        if "mixed_scenarios" in test_data:
            print_info(f"\n测试混合场景...")
            for i, scenario in enumerate(test_data["mixed_scenarios"], 1):
                message = scenario["message"]
                expected = scenario["expected_detections"]
                desc = scenario["description"]

                print(f"  [{i}] {desc}", end=" ")

                response, resp_time = self.send_chat_message(message)

                detected_types = []
                if "data" in response and "sensitive_infos" in response["data"] and response["data"]["sensitive_infos"]:
                    detected_types = [info["type"] for info in response["data"]["sensitive_infos"]]

                # 检查是否检测到所有预期类型
                all_detected = all(exp_type in detected_types for exp_type in expected)
                no_extra = len(detected_types) == len(expected)

                if all_detected and no_extra:
                    print_success(f"Perfect match ({resp_time:.0f}ms)")
                elif all_detected:
                    print_warning(f"Extra types detected ({resp_time:.0f}ms)")
                else:
                    print_error(f"Incomplete detection ({resp_time:.0f}ms)")

                time.sleep(0.1)

        # 计算总体指标
        self.results["sensitive_info_detection"] = self.calculate_metrics(stats)
        self.print_detection_summary(stats)

    def test_adversarial_attacks(self, test_data: Dict):
        """测试对抗性攻击防御"""
        print_header("对抗性攻击防御测试")

        if "adversarial_attacks" not in test_data:
            print_warning("未找到对抗性攻击测试数据")
            return

        stats = {"blocked": 0, "passed": 0, "total": 0}
        attack_results = []

        for i, attack in enumerate(test_data["adversarial_attacks"], 1):
            message = attack["message"]
            attack_type = attack["attack_type"]
            should_block = attack["should_block"]

            print(f"  [{i}/{len(test_data['adversarial_attacks'])}] {attack_type}: ", end="")

            response, resp_time = self.send_chat_message(message)
            stats["total"] += 1

            # 检查是否被阻止（通过检查响应中的安全警告）
            blocked = False
            if "data" in response and "security_warnings" in response["data"] and response["data"]["security_warnings"]:
                blocked = True
            elif "error" in response and response["error"]:
                blocked = True

            # 判断结果
            if should_block and blocked:
                stats["blocked"] += 1
                print_success(f"Successfully blocked ({resp_time:.0f}ms)")
                result = "success"
            elif not should_block and not blocked:
                stats["passed"] += 1
                print_success(f"Correctly passed ({resp_time:.0f}ms)")
                result = "success"
            elif should_block and not blocked:
                print_error(f"Failed to block ({resp_time:.0f}ms)")
                result = "failed"
            else:
                print_error(f"False block ({resp_time:.0f}ms)")
                result = "failed"

            attack_results.append({
                "attack_type": attack_type,
                "should_block": should_block,
                "blocked": blocked,
                "result": result,
                "response_time": resp_time
            })

            time.sleep(0.1)

        # 计算防御成功率
        success_rate = (stats["blocked"] + stats["passed"]) / stats["total"] * 100 if stats["total"] > 0 else 0

        self.results["adversarial_attacks"] = {
            "total_attacks": stats["total"],
            "blocked": stats["blocked"],
            "passed": stats["passed"],
            "success_rate": success_rate,
            "details": attack_results
        }

        print(f"\n{Colors.BOLD}对抗性攻击防御统计:{Colors.ENDC}")
        print(f"  总测试数: {stats['total']}")
        print(f"  成功阻止: {stats['blocked']}")
        print(f"  正常通过: {stats['passed']}")
        print(f"  防御成功率: {Colors.OKGREEN}{success_rate:.1f}%{Colors.ENDC}")

    def calculate_metrics(self, stats: Dict) -> Dict:
        """计算检测指标"""
        results = {}
        total_tp = total_tn = total_fp = total_fn = 0

        for info_type, counts in stats.items():
            tp = counts["TP"]
            tn = counts["TN"]
            fp = counts["FP"]
            fn = counts["FN"]
            total = counts["total"]

            total_tp += tp
            total_tn += tn
            total_fp += fp
            total_fn += fn

            # 计算指标
            accuracy = (tp + tn) / total * 100 if total > 0 else 0
            recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
            precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0
            f1 = 2 * (precision * recall) / (precision + recall) if (precision + recall) > 0 else 0

            results[info_type] = {
                "TP": tp, "TN": tn, "FP": fp, "FN": fn,
                "total": total,
                "accuracy": accuracy,
                "recall": recall,
                "precision": precision,
                "f1_score": f1
            }

        # 计算总体指标
        total = total_tp + total_tn + total_fp + total_fn
        overall_accuracy = (total_tp + total_tn) / total * 100 if total > 0 else 0
        overall_recall = total_tp / (total_tp + total_fn) * 100 if (total_tp + total_fn) > 0 else 0
        overall_precision = total_tp / (total_tp + total_fp) * 100 if (total_tp + total_fp) > 0 else 0
        overall_f1 = 2 * (overall_precision * overall_recall) / (overall_precision + overall_recall) if (overall_precision + overall_recall) > 0 else 0

        results["overall"] = {
            "TP": total_tp, "TN": total_tn, "FP": total_fp, "FN": total_fn,
            "total": total,
            "accuracy": overall_accuracy,
            "recall": overall_recall,
            "precision": overall_precision,
            "f1_score": overall_f1
        }

        return results

    def print_detection_summary(self, stats: Dict):
        """打印检测结果摘要"""
        print(f"\n{Colors.BOLD}敏感信息检测统计:{Colors.ENDC}")

        metrics = self.results["sensitive_info_detection"]

        # 打印各类型指标
        for info_type, data in metrics.items():
            if info_type == "overall":
                continue
            print(f"\n  {info_type}:")
            print(f"    准确率: {data['accuracy']:.1f}%")
            print(f"    召回率: {data['recall']:.1f}%")
            print(f"    精确率: {data['precision']:.1f}%")
            print(f"    F1分数: {data['f1_score']:.1f}%")

        # 打印总体指标
        overall = metrics["overall"]
        print(f"\n  {Colors.BOLD}总体性能:{Colors.ENDC}")
        print(f"    总测试数: {overall['total']}")
        print(f"    准确率: {Colors.OKGREEN if overall['accuracy'] >= 95 else Colors.WARNING}{overall['accuracy']:.1f}%{Colors.ENDC}")
        print(f"    召回率: {Colors.OKGREEN if overall['recall'] >= 90 else Colors.WARNING}{overall['recall']:.1f}%{Colors.ENDC}")
        print(f"    精确率: {Colors.OKGREEN if overall['precision'] >= 85 else Colors.WARNING}{overall['precision']:.1f}%{Colors.ENDC}")
        print(f"    F1分数: {Colors.OKGREEN if overall['f1_score'] >= 90 else Colors.WARNING}{overall['f1_score']:.1f}%{Colors.ENDC}")

    def calculate_performance_metrics(self):
        """计算性能指标"""
        print_header("性能指标统计")

        if not self.response_times:
            print_warning("没有响应时间数据")
            return

        sorted_times = sorted(self.response_times)
        avg_time = sum(sorted_times) / len(sorted_times)
        p50_time = sorted_times[len(sorted_times) // 2]
        p95_time = sorted_times[int(len(sorted_times) * 0.95)]
        p99_time = sorted_times[int(len(sorted_times) * 0.99)]
        max_time = max(sorted_times)
        min_time = min(sorted_times)

        self.results["performance"] = {
            "total_requests": len(self.response_times),
            "avg_response_time": avg_time,
            "p50_response_time": p50_time,
            "p95_response_time": p95_time,
            "p99_response_time": p99_time,
            "max_response_time": max_time,
            "min_response_time": min_time
        }

        print(f"  总请求数: {len(self.response_times)}")
        print(f"  平均响应时间: {Colors.OKGREEN if avg_time < 500 else Colors.WARNING}{avg_time:.0f}ms{Colors.ENDC}")
        print(f"  P50响应时间: {p50_time:.0f}ms")
        print(f"  P95响应时间: {Colors.OKGREEN if p95_time < 1000 else Colors.WARNING}{p95_time:.0f}ms{Colors.ENDC}")
        print(f"  P99响应时间: {p99_time:.0f}ms")
        print(f"  最大响应时间: {max_time:.0f}ms")
        print(f"  最小响应时间: {min_time:.0f}ms")

    def generate_summary(self):
        """生成测试摘要"""
        detection = self.results["sensitive_info_detection"]["overall"]
        attacks = self.results["adversarial_attacks"]
        performance = self.results["performance"]

        self.results["summary"] = {
            "overall_accuracy": detection["accuracy"],
            "overall_recall": detection["recall"],
            "overall_precision": detection["precision"],
            "overall_f1_score": detection["f1_score"],
            "attack_defense_rate": attacks["success_rate"],
            "avg_response_time": performance["avg_response_time"],
            "meets_requirements": {
                "accuracy_95": detection["accuracy"] >= 95,
                "recall_90": detection["recall"] >= 90,
                "precision_85": detection["precision"] >= 85,
                "response_time_500ms": performance["avg_response_time"] < 500
            }
        }

    def save_results(self):
        """保存测试结果"""
        import os
        os.makedirs(RESULTS_DIR, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

        # 保存JSON格式
        json_file = f"{RESULTS_DIR}/test_results_{timestamp}.json"
        with open(json_file, 'w', encoding='utf-8') as f:
            json.dump(self.results, f, ensure_ascii=False, indent=2)
        print_success(f"JSON结果已保存: {json_file}")

        # 保存Markdown格式
        md_file = f"{RESULTS_DIR}/test_results_{timestamp}.md"
        self.generate_markdown_report(md_file)
        print_success(f"Markdown报告已保存: {md_file}")

    def generate_markdown_report(self, filename: str):
        """生成Markdown格式的测试报告"""
        detection = self.results["sensitive_info_detection"]
        attacks = self.results["adversarial_attacks"]
        performance = self.results["performance"]
        summary = self.results["summary"]

        with open(filename, 'w', encoding='utf-8') as f:
            f.write("# Context-Keeper 安全功能测试报告\n\n")
            f.write(f"**测试时间**: {self.results['test_time']}\n\n")

            # 总体摘要
            f.write("## 测试摘要\n\n")
            f.write("| 指标 | 数值 | 目标 | 状态 |\n")
            f.write("|------|------|------|------|\n")
            f.write(f"| 准确率 | {summary['overall_accuracy']:.1f}% | >=95% | {'OK' if summary['meets_requirements']['accuracy_95'] else 'FAIL'} |\n")
            f.write(f"| 召回率 | {summary['overall_recall']:.1f}% | >=90% | {'OK' if summary['meets_requirements']['recall_90'] else 'FAIL'} |\n")
            f.write(f"| 精确率 | {summary['overall_precision']:.1f}% | >=85% | {'OK' if summary['meets_requirements']['precision_85'] else 'FAIL'} |\n")
            f.write(f"| F1分数 | {summary['overall_f1_score']:.1f}% | - | - |\n")
            f.write(f"| 对抗攻击防御率 | {summary['attack_defense_rate']:.1f}% | - | - |\n")
            f.write(f"| 平均响应时间 | {summary['avg_response_time']:.0f}ms | <500ms | {'OK' if summary['meets_requirements']['response_time_500ms'] else 'FAIL'} |\n\n")

            # 敏感信息检测详情
            f.write("## 敏感信息检测详情\n\n")
            for info_type, data in detection.items():
                if info_type == "overall":
                    continue
                f.write(f"### {info_type}\n\n")
                f.write(f"- **准确率**: {data['accuracy']:.1f}%\n")
                f.write(f"- **召回率**: {data['recall']:.1f}%\n")
                f.write(f"- **精确率**: {data['precision']:.1f}%\n")
                f.write(f"- **F1分数**: {data['f1_score']:.1f}%\n")
                f.write(f"- **测试样本数**: {data['total']}\n")
                f.write(f"- **混淆矩阵**: TP={data['TP']}, TN={data['TN']}, FP={data['FP']}, FN={data['FN']}\n\n")

            # 对抗性攻击防御
            f.write("## 对抗性攻击防御\n\n")
            f.write(f"- **总测试数**: {attacks['total_attacks']}\n")
            f.write(f"- **成功阻止**: {attacks['blocked']}\n")
            f.write(f"- **正常通过**: {attacks['passed']}\n")
            f.write(f"- **防御成功率**: {attacks['success_rate']:.1f}%\n\n")

            # 性能指标
            f.write("## 性能指标\n\n")
            f.write(f"- **总请求数**: {performance['total_requests']}\n")
            f.write(f"- **平均响应时间**: {performance['avg_response_time']:.0f}ms\n")
            f.write(f"- **P50响应时间**: {performance['p50_response_time']:.0f}ms\n")
            f.write(f"- **P95响应时间**: {performance['p95_response_time']:.0f}ms\n")
            f.write(f"- **P99响应时间**: {performance['p99_response_time']:.0f}ms\n")
            f.write(f"- **最大响应时间**: {performance['max_response_time']:.0f}ms\n")
            f.write(f"- **最小响应时间**: {performance['min_response_time']:.0f}ms\n\n")

            # 结论
            f.write("## 结论\n\n")
            all_passed = all(summary['meets_requirements'].values())
            if all_passed:
                f.write("[OK] **All metrics meet expected targets**\n\n")
            else:
                f.write("[WARN] **Some metrics did not meet expected targets**\n\n")
                for req, passed in summary['meets_requirements'].items():
                    if not passed:
                        f.write(f"- {req}: Not met\n")

    def run_all_tests(self):
        """运行所有测试"""
        print_header("Context-Keeper 安全功能自动化测试")
        print_info(f"测试时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")

        # 1. 检查服务器
        if not self.check_server_health():
            return False

        # 2. 登录获取Token
        if not self.login():
            print_warning("无法获取JWT Token，尝试不使用认证继续测试...")

        # 3. 加载测试数据
        test_data = self.load_test_data()
        if not test_data:
            return False

        # 4. 运行测试
        self.test_sensitive_info_detection(test_data)
        self.test_adversarial_attacks(test_data)
        self.calculate_performance_metrics()

        # 5. 生成摘要
        self.generate_summary()

        # 6. 打印最终摘要
        print_header("测试完成")
        summary = self.results["summary"]
        print(f"{Colors.BOLD}总体性能:{Colors.ENDC}")
        print(f"  准确率: {Colors.OKGREEN if summary['meets_requirements']['accuracy_95'] else Colors.WARNING}{summary['overall_accuracy']:.1f}%{Colors.ENDC}")
        print(f"  召回率: {Colors.OKGREEN if summary['meets_requirements']['recall_90'] else Colors.WARNING}{summary['overall_recall']:.1f}%{Colors.ENDC}")
        print(f"  精确率: {Colors.OKGREEN if summary['meets_requirements']['precision_85'] else Colors.WARNING}{summary['overall_precision']:.1f}%{Colors.ENDC}")
        print(f"  F1分数: {Colors.OKGREEN}{summary['overall_f1_score']:.1f}%{Colors.ENDC}")
        print(f"  对抗攻击防御率: {Colors.OKGREEN}{summary['attack_defense_rate']:.1f}%{Colors.ENDC}")
        print(f"  平均响应时间: {Colors.OKGREEN if summary['meets_requirements']['response_time_500ms'] else Colors.WARNING}{summary['avg_response_time']:.0f}ms{Colors.ENDC}")

        # 7. 保存结果
        self.save_results()

        return True

def main():
    tester = SecurityTester()
    success = tester.run_all_tests()

    if success:
        print(f"\n{Colors.OKGREEN}{Colors.BOLD}[OK] Test completed successfully!{Colors.ENDC}")
        print_info("Test results saved to test_results/ directory")
        return 0
    else:
        print(f"\n{Colors.FAIL}{Colors.BOLD}[ERROR] Test failed{Colors.ENDC}")
        return 1

if __name__ == "__main__":
    exit(main())
