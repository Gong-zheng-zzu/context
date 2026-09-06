#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
分类型调试脚本 - 分别测试phone、bank_card、email类型
找出导致第2轮开始0%召回率的根本原因
"""

import json
import time
import requests
from typing import Dict, List, Tuple

# 配置
API_BASE_URL = "http://localhost:8088"
TEST_DATA_FILE = "test_data/sensitive_data_200.json"

class TypeDebugger:
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
                    return True
            return False
        except Exception as e:
            print(f"登录失败: {e}")
            return False

    def detect_sensitive_info(self, content: str, sample_id: int = 0) -> Tuple[bool, List[Dict], float]:
        """调用安全检测API，返回详细检测结果"""
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
                    "session_id": f"debug_session_{sample_id}"
                },
                headers=headers,
                timeout=5
            )

            response_time = (time.time() - start_time) * 1000

            if response.status_code == 200:
                result = response.json()
                sensitive_infos = result.get("sensitive_infos", [])
                if sensitive_infos is None:
                    sensitive_infos = []
                detected = len(sensitive_infos) > 0
                return detected, sensitive_infos, response_time
            else:
                return False, [], response_time
        except Exception as e:
            print(f"检测异常: {e}")
            return False, [], 0

    def load_test_data(self) -> Dict:
        """加载测试数据"""
        try:
            with open(TEST_DATA_FILE, 'r', encoding='utf-8') as f:
                return json.load(f)
        except Exception as e:
            print(f"加载测试数据失败: {e}")
            return {}

    def test_single_type(self, info_type: str, samples: List[Dict], round_num: int):
        """测试单个类型"""
        print(f"\n{'='*80}")
        print(f"第 {round_num} 轮 - 测试类型: {info_type.upper()}")
        print(f"{'='*80}")

        tp, tn, fp, fn = 0, 0, 0, 0
        false_negatives = []  # 记录漏检样本
        false_positives = []  # 记录误检样本

        for idx, sample in enumerate(samples):
            value = sample["value"]
            should_detect = sample["should_detect"]
            label = sample["label"]

            detected, infos, resp_time = self.detect_sensitive_info(value, idx + round_num * 1000)

            # 统计
            if should_detect and detected:
                tp += 1
            elif not should_detect and not detected:
                tn += 1
            elif not should_detect and detected:
                fp += 1
                false_positives.append({
                    "label": label,
                    "value": value,
                    "detected_infos": infos
                })
            elif should_detect and not detected:
                fn += 1
                false_negatives.append({
                    "label": label,
                    "value": value
                })

            time.sleep(0.01)

        # 计算指标
        total = tp + tn + fp + fn
        accuracy = (tp + tn) / total * 100 if total > 0 else 0
        recall = tp / (tp + fn) * 100 if (tp + fn) > 0 else 0
        precision = tp / (tp + fp) * 100 if (tp + fp) > 0 else 0

        print(f"\n结果统计:")
        print(f"  TP={tp}, TN={tn}, FP={fp}, FN={fn}")
        print(f"  准确率: {accuracy:.1f}%")
        print(f"  召回率: {recall:.1f}%")
        print(f"  精确率: {precision:.1f}%")

        # 显示漏检样本（最多显示5个）
        if false_negatives:
            print(f"\n[WARNING] 漏检样本 (FN={fn}):")
            for item in false_negatives[:5]:
                print(f"    - {item['label']}: {item['value']}")
            if len(false_negatives) > 5:
                print(f"    ... 还有 {len(false_negatives) - 5} 个漏检样本")

        # 显示误检样本（最多显示5个）
        if false_positives:
            print(f"\n[WARNING] 误检样本 (FP={fp}):")
            for item in false_positives[:5]:
                print(f"    - {item['label']}: {item['value']}")
                print(f"      检测到: {item['detected_infos']}")
            if len(false_positives) > 5:
                print(f"    ... 还有 {len(false_positives) - 5} 个误检样本")

        return {
            "type": info_type,
            "round": round_num,
            "accuracy": accuracy,
            "recall": recall,
            "precision": precision,
            "tp": tp,
            "tn": tn,
            "fp": fp,
            "fn": fn
        }

    def run(self):
        """运行调试测试"""
        print("="*80)
        print("Context-Keeper 分类型调试测试")
        print("="*80)

        # 登录
        print("\n登录获取JWT Token...")
        if not self.login():
            print("登录失败")
            return False

        # 加载测试数据
        print("加载测试数据...")
        test_data = self.load_test_data()
        if not test_data:
            return False

        # 测试3轮，每轮测试phone、bank_card、email
        all_results = []
        for round_num in range(1, 4):
            print(f"\n\n{'#'*80}")
            print(f"# 第 {round_num} 轮测试")
            print(f"{'#'*80}")

            # 重新登录
            if round_num > 1:
                if not self.login():
                    print(f"第{round_num}轮登录失败")
                    continue

            for info_type in ["phone", "bank_card", "email"]:
                if info_type not in test_data:
                    continue

                samples = test_data[info_type]
                result = self.test_single_type(info_type, samples, round_num)
                all_results.append(result)
                time.sleep(0.5)

        # 打印总结
        print(f"\n\n{'='*80}")
        print("总结：各类型各轮次召回率")
        print(f"{'='*80}")

        for info_type in ["phone", "bank_card", "email"]:
            print(f"\n{info_type.upper()}:")
            type_results = [r for r in all_results if r["type"] == info_type]
            for r in type_results:
                print(f"  第{r['round']}轮: 召回率={r['recall']:.1f}%, 准确率={r['accuracy']:.1f}%")

        return True

if __name__ == "__main__":
    debugger = TypeDebugger()
    success = debugger.run()
    exit(0 if success else 1)
