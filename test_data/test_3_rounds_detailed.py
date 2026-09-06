#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
3轮详细测试 - 完全模拟test_10_rounds.py的流程，但添加详细日志
"""

import json
import time
import requests
from typing import Dict, Tuple

API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

class DetailedTester:
    def __init__(self):
        self.jwt_token = None

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
                    print(f"  [登录成功] Token前20字符: {self.jwt_token[:20]}...")
                    return True
            print(f"  [登录失败] HTTP {response.status_code}")
            return False
        except Exception as e:
            print(f"  [登录异常] {e}")
            return False

    def detect_sensitive_info(self, content: str, sample_id: int, round_num: int) -> Tuple[bool, dict]:
        """调用安全检测API"""
        try:
            headers = {
                "Content-Type": "application/json",
                "Authorization": f"Bearer {self.jwt_token}"
            }

            session_id = f"test_session_{sample_id}"

            response = requests.post(
                f"{API_BASE_URL}/api/security/detect",
                json={
                    "content": content,
                    "user_id": "test_user",
                    "session_id": session_id
                },
                headers=headers,
                timeout=5
            )

            if response.status_code == 200:
                result = response.json()
                detected = len(result.get("sensitive_infos", [])) > 0
                return detected, result
            else:
                return False, {"error": f"HTTP {response.status_code}", "body": response.text}
        except Exception as e:
            return False, {"error": f"Exception: {str(e)}"}

    def run_single_round(self, round_num: int, test_data: Dict) -> Dict:
        """运行单轮测试"""
        print(f"\n{'='*80}")
        print(f"第 {round_num}/3 轮测试")
        print(f"{'='*80}")

        # 每轮测试前重新登录（除了第1轮）
        if round_num > 1:
            print(f"\n[第{round_num}轮] 重新登录...")
            if not self.login():
                print(f"第{round_num}轮登录失败")
                return {"round": round_num, "tp": 0, "tn": 0, "fp": 0, "fn": 0}

        stats = {"TP": 0, "TN": 0, "FP": 0, "FN": 0, "total": 0}

        # 全局样本计数器
        sample_counter = (round_num - 1) * 1000
        print(f"\n[第{round_num}轮] 样本计数器起始值: {sample_counter}")

        # 测试所有类型（每种类型只测试前5个样本）
        for info_type in ["id_card", "phone", "bank_card", "email"]:
            if info_type not in test_data:
                continue

            samples = test_data[info_type]  # 测试所有样本
            print(f"\n[{info_type}] 测试 {len(samples)} 个样本")

            for idx, sample in enumerate(samples):
                value = sample["value"]
                should_detect = sample["should_detect"]
                label = sample.get("label", "")

                detected, result = self.detect_sensitive_info(value, sample_counter, round_num)
                sample_counter += 1

                stats["total"] += 1

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
                    print(f"    [{status}] {label}: {value}")
                    print(f"         Response: {result}")

                time.sleep(0.1)  # 降低请求速率，避免触发速率限制

        # 计算指标
        tp, tn, fp, fn = stats["TP"], stats["TN"], stats["FP"], stats["FN"]
        total = stats["total"]

        accuracy = (tp + tn) / total * 100 if total > 0 else 0
        recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
        precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0

        print(f"\n[第{round_num}轮结果]")
        print(f"  TP={tp}, TN={tn}, FP={fp}, FN={fn}")
        print(f"  准确率: {accuracy:.1f}%  召回率: {recall:.1f}%  精确率: {precision:.1f}%")

        return {"round": round_num, "tp": tp, "tn": tn, "fp": fp, "fn": fn, "accuracy": accuracy, "recall": recall}

    def run(self):
        """运行3轮测试"""
        print("="*80)
        print("3轮详细测试（每轮200个样本）")
        print("="*80)

        # 第1轮登录
        print("\n[初始登录]")
        if not self.login():
            print("初始登录失败")
            return False

        # 加载测试数据
        with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
            test_data = json.load(f)

        # 运行3轮测试
        results = []
        for round_num in range(1, 4):
            result = self.run_single_round(round_num, test_data)
            results.append(result)
            time.sleep(0.5)

        # 总结
        print("\n" + "="*80)
        print("3轮测试总结")
        print("="*80)
        for r in results:
            print(f"第{r['round']}轮: 准确率={r['accuracy']:.1f}%, 召回率={r['recall']:.1f}%, TP={r['tp']}, FN={r['fn']}")

        return True

if __name__ == "__main__":
    tester = DetailedTester()
    tester.run()
