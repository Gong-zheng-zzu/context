#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
10轮测试脚本 - 调试版本
"""

import json
import time
import requests
from datetime import datetime
from typing import Dict, List, Tuple

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

    def detect_sensitive_info(self, content: str, sample_id: int = 0, debug: bool = False) -> Tuple[bool, float, dict]:
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
                    "session_id": f"test_session_{sample_id}"
                },
                headers=headers,
                timeout=5
            )

            response_time = (time.time() - start_time) * 1000

            if response.status_code == 200:
                result = response.json()
                detected = len(result.get("sensitive_infos", [])) > 0
                return detected, response_time, result
            else:
                return False, response_time, {"error": f"HTTP {response.status_code}"}
        except Exception as e:
            return False, 0, {"error": str(e)}

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
        print(f"\n=== 第 {round_num}/10 轮测试 ===")

        # 每轮测试前重新登录
        if round_num > 1:
            if not self.login():
                print(f"第{round_num}轮登录失败")
                return {"round": round_num, "tp": 0, "tn": 0, "fp": 0, "fn": 0}

        stats = {"TP": 0, "TN": 0, "FP": 0, "FN": 0}
        sample_counter = 0

        # 只测试前5个身份证样本（调试用）
        samples = test_data['id_card'][:5]

        for i, sample in enumerate(samples):
            value = sample["value"]
            should_detect = sample["should_detect"]

            detected, resp_time, result = self.detect_sensitive_info(value, sample_counter, debug=True)
            sample_counter += 1

            # 计算TP/TN/FP/FN
            if should_detect and detected:
                stats["TP"] += 1
                status = "TP"
            elif not should_detect and not detected:
                stats["TN"] += 1
                status = "TN"
            elif not should_detect and detected:
                stats["FP"] += 1
                status = "FP"
            elif should_detect and not detected:
                stats["FN"] += 1
                status = "FN"

            # 第2轮时打印详细信息
            if round_num == 2:
                print(f"  样本{i+1}: 值={value} 应检测={should_detect} 实际={detected} 状态={status}")
                print(f"    API响应: {result}")

            time.sleep(0.01)

        tp, tn, fp, fn = stats["TP"], stats["TN"], stats["FP"], stats["FN"]
        total = tp + tn + fp + fn
        accuracy = (tp + tn) / total * 100 if total > 0 else 0
        recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0

        print(f"  结果: TP={tp}, TN={tn}, FP={fp}, FN={fn}, 准确率={accuracy:.1f}%, 召回率={recall:.1f}%")

        return {"round": round_num, "tp": tp, "tn": tn, "fp": fp, "fn": fn}

    def run(self):
        """运行10轮测试"""
        print("=== Context-Keeper 10轮测试（调试版本）===\n")

        # 登录
        print("登录...")
        if not self.login():
            print("登录失败")
            return False

        # 加载测试数据
        print("加载测试数据...")
        test_data = self.load_test_data()
        if not test_data:
            return False

        # 运行2轮测试（调试用）
        for round_num in range(1, 3):
            result = self.run_single_round(round_num, test_data)
            self.all_rounds_results.append(result)
            time.sleep(0.5)

        print("\n=== 测试完成 ===")
        return True

if __name__ == "__main__":
    tester = MultiRoundTester()
    success = tester.run()
    exit(0 if success else 1)
