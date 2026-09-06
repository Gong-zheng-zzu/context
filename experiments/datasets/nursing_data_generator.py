#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
护理记录测试数据生成器
生成100个老人档案 + 1000条护理日志，用于因果推理和检索测试
"""

import json
import random
from datetime import datetime, timedelta
from pathlib import Path
from typing import List, Dict

class NursingDataGenerator:
    """护理数据生成器"""

    def __init__(self):
        self.output_dir = Path(__file__).parent / "nursing_data"
        self.output_dir.mkdir(parents=True, exist_ok=True)

        # 基础数据
        self.surnames = ["李", "张", "王", "刘", "陈", "杨", "赵", "黄", "周", "吴"]
        self.male_names = ["建国", "国强", "志强", "明", "军", "勇", "伟", "磊", "涛", "鹏"]
        self.female_names = ["芳", "秀英", "桂英", "丽", "静", "敏", "华", "红", "玲", "娟"]

        # 医学数据
        self.chronic_diseases = [
            {"name": "高血压", "medication": "硝苯地平缓释片", "dosage": "30mg", "frequency": "每日一次"},
            {"name": "糖尿病", "medication": "二甲双胍", "dosage": "500mg", "frequency": "每日两次"},
            {"name": "冠心病", "medication": "阿司匹林", "dosage": "100mg", "frequency": "每日一次"},
            {"name": "阿尔茨海默病", "medication": "多奈哌齐", "dosage": "10mg", "frequency": "每日一次"},
            {"name": "骨质疏松", "medication": "阿仑膦酸钠", "dosage": "70mg", "frequency": "每周一次"},
        ]

        self.allergies = ["青霉素", "头孢", "磺胺类", "阿司匹林", "碘造影剂", "无"]

        # 护理事件模板
        self.event_templates = [
            self._generate_medication_event,
            self._generate_vital_signs_event,
            self._generate_fall_event,
            self._generate_sleep_event,
            self._generate_meal_event,
            self._generate_mood_event,
            self._generate_confusion_event,
            self._generate_pain_event,
        ]

    def generate_elder_profile(self, elder_id: int) -> Dict:
        """生成老人档案"""
        gender = random.choice(["男", "女"])
        surname = random.choice(self.surnames)

        if gender == "男":
            given_name = random.choice(self.male_names)
        else:
            given_name = random.choice(self.female_names)

        name = f"{surname}{given_name}"

        # 年龄：70-95岁
        age = random.randint(70, 95)
        birth_year = 2026 - age

        # 慢性病（1-3种）
        num_diseases = random.randint(1, 3)
        diseases = random.sample(self.chronic_diseases, num_diseases)

        # 过敏史
        allergy = random.choice(self.allergies)

        # 床位号
        floor = random.randint(2, 5)
        room = random.randint(1, 20)
        bed = random.choice(["A", "B"])
        bed_number = f"{floor}{room:02d}{bed}"

        profile = {
            "elder_id": f"elder_{elder_id:03d}",
            "name": name,
            "gender": gender,
            "age": age,
            "birth_year": birth_year,
            "bed_number": bed_number,
            "admission_date": (datetime.now() - timedelta(days=random.randint(30, 365))).strftime("%Y-%m-%d"),
            "chronic_diseases": [d["name"] for d in diseases],
            "medications": diseases,
            "allergies": allergy,
            "emergency_contact": {
                "name": f"{surname}{'明' if gender == '男' else '华'}",
                "relationship": random.choice(["儿子", "女儿", "侄子", "侄女"]),
                "phone": f"139{random.randint(10000000, 99999999)}"
            },
            "care_level": random.choice(["一级护理", "二级护理", "三级护理"]),
            "mobility": random.choice(["行动自如", "需要辅助器具", "轮椅", "卧床"]),
            "cognitive_status": random.choice(["正常", "轻度认知障碍", "中度认知障碍", "重度认知障碍"])
        }

        return profile

    def _generate_medication_event(self, elder: Dict, date: datetime) -> Dict:
        """生成服药记录"""
        if not elder["medications"]:
            return None

        med = random.choice(elder["medications"])
        hour = random.choice([8, 12, 18, 21])  # 服药时间

        templates = [
            f"{elder['name']}，{elder['age']}岁，{hour}:00服用{med['medication']}（{med['dosage']}），{med['frequency']}。患者服药后无不适反应，生命体征平稳。",
            f"护理记录：{elder['name']}于{hour}:00按医嘱服用{med['medication']} {med['dosage']}。观察30分钟，患者未诉不适，血压心率正常。",
            f"{elder['name']}早{hour}点服用常规药物{med['medication']}。药物从药盒中取出，患者在护士监督下完成服药。",
        ]

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=hour, minute=random.randint(0, 59)).isoformat(),
            "event_type": "medication",
            "severity": "normal",
            "content": random.choice(templates),
            "tags": ["服药", med["medication"], "常规护理"]
        }

    def _generate_vital_signs_event(self, elder: Dict, date: datetime) -> Dict:
        """生成生命体征监测记录"""
        has_hypertension = "高血压" in elder["chronic_diseases"]
        has_diabetes = "糖尿病" in elder["chronic_diseases"]

        # 血压
        if has_hypertension:
            systolic = random.randint(130, 160)
            diastolic = random.randint(80, 100)
        else:
            systolic = random.randint(110, 140)
            diastolic = random.randint(70, 90)

        # 血糖
        if has_diabetes:
            blood_glucose = round(random.uniform(6.0, 12.0), 1)
        else:
            blood_glucose = round(random.uniform(4.5, 6.5), 1)

        heart_rate = random.randint(60, 90)
        temperature = round(random.uniform(36.2, 36.8), 1)

        hour = random.choice([7, 14, 19])

        content = f"{elder['name']}，{elder['age']}岁，{hour}:00常规生命体征监测：血压{systolic}/{diastolic}mmHg，心率{heart_rate}次/分，体温{temperature}℃"

        if has_diabetes:
            content += f"，空腹血糖{blood_glucose}mmol/L"

        content += "。生命体征平稳，无异常。"

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=hour, minute=random.randint(0, 59)).isoformat(),
            "event_type": "vital_signs",
            "severity": "normal",
            "content": content,
            "tags": ["生命体征", "血压", "心率"],
            "structured_data": {
                "blood_pressure": f"{systolic}/{diastolic}",
                "heart_rate": heart_rate,
                "temperature": temperature,
                "blood_glucose": blood_glucose if has_diabetes else None
            }
        }

    def _generate_fall_event(self, elder: Dict, date: datetime) -> Dict:
        """生成摔倒事件（因果推理重点）"""
        has_hypertension = "高血压" in elder["chronic_diseases"]

        locations = ["洗手间", "走廊", "房间", "楼梯口", "餐厅"]
        location = random.choice(locations)

        hour = random.randint(6, 22)
        minute = random.randint(0, 59)

        templates = []

        # 低血压导致摔倒
        if has_hypertension:
            templates.append(
                f"{elder['name']}，{elder['age']}岁，长期服用降压药物。"
                f"今日上午{hour}:{minute:02d}，护工发现{elder['name']}在{location}摔倒，意识清醒，诉头晕。"
                f"测量血压{random.randint(80, 95)}/{random.randint(50, 65)}mmHg，较平时偏低。"
                f"询问得知患者早餐后立即服药，随后起身时突然眩晕失去平衡。"
                f"初步判断：体位性低血压导致跌倒风险。"
                f"处理措施：协助患者卧床休息，抬高下肢，监测血压变化，通知家属及主治医生。"
            )

        # 夜间如厕摔倒
        templates.append(
            f"{elder['name']}，{elder['age']}岁，夜间{random.randint(2, 4)}:{random.randint(0, 59):02d}起床如厕，"
            f"在{location}滑倒。护士听到响声立即赶到，发现患者坐在地上，诉右髋部疼痛。"
            f"查体：右髋关节活动受限，局部压痛明显。"
            f"处理：协助患者躺下，冰敷患处，通知医生行X线检查排除骨折。"
        )

        # 环境因素摔倒
        templates.append(
            f"{elder['name']}在{location}被地面水渍滑倒，幸好被护工及时扶住，未完全跌倒。"
            f"检查后无明显外伤，生命体征平稳。已清理地面并加设防滑垫。"
        )

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=hour, minute=minute).isoformat(),
            "event_type": "fall",
            "severity": "high",
            "content": random.choice(templates),
            "tags": ["摔倒", "安全事件", location, "紧急处理"],
            "causal_factors": ["低血压", "服药", "体位变化"] if has_hypertension else ["环境因素", "夜间"]
        }

    def _generate_sleep_event(self, elder: Dict, date: datetime) -> Dict:
        """生成睡眠记录"""
        sleep_quality = random.choice(["良好", "一般", "较差", "失眠"])

        templates = {
            "良好": f"{elder['name']}夜间睡眠良好，22:30入睡，次日6:30自然醒，期间未起夜，精神状态佳。",
            "一般": f"{elder['name']}夜间睡眠一般，23:00入睡，凌晨2:00起夜一次如厕，6:00醒来，诉睡眠浅。",
            "较差": f"{elder['name']}夜间睡眠较差，辗转反侧至凌晨1:00方入睡，5:00即醒，诉多梦，白天精神欠佳。",
            "失眠": f"{elder['name']}夜间失眠，整夜未眠，在床上翻来覆去，诉心烦焦虑。护士多次安抚无效。"
        }

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=6, minute=random.randint(0, 59)).isoformat(),
            "event_type": "sleep",
            "severity": "low" if sleep_quality in ["良好", "一般"] else "medium",
            "content": templates[sleep_quality],
            "tags": ["睡眠", sleep_quality],
            "structured_data": {"sleep_quality": sleep_quality}
        }

    def _generate_meal_event(self, elder: Dict, date: datetime) -> Dict:
        """生成饮食记录"""
        meal_type = random.choice(["早餐", "午餐", "晚餐"])
        appetite = random.choice(["良好", "一般", "较差"])

        hour_map = {"早餐": 7, "午餐": 12, "晚餐": 18}
        hour = hour_map[meal_type]

        templates = {
            "良好": f"{elder['name']}{meal_type}食欲良好，主食、副食均完食，饮水充足。",
            "一般": f"{elder['name']}{meal_type}食欲一般，仅吃半碗粥和少量菜肴，劝导后增加少许进食。",
            "较差": f"{elder['name']}{meal_type}食欲较差，几乎未进食，仅喝少量汤水。询问原因，患者诉无食欲，未诉不适。"
        }

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=hour, minute=random.randint(0, 59)).isoformat(),
            "event_type": "meal",
            "severity": "normal" if appetite != "较差" else "medium",
            "content": templates[appetite],
            "tags": ["饮食", meal_type, appetite]
        }

    def _generate_mood_event(self, elder: Dict, date: datetime) -> Dict:
        """生成情绪记录"""
        mood = random.choice(["平和", "焦虑", "抑郁", "烦躁"])

        templates = {
            "平和": f"{elder['name']}今日情绪平和，与护工交流顺畅，愿意参与康复活动。",
            "焦虑": f"{elder['name']}今日显得焦虑不安，反复询问家属何时来访，安抚后情绪略有缓解。",
            "抑郁": f"{elder['name']}今日情绪低落，不愿与人交流，独自坐在窗边发呆。护士多次关心，患者诉想家。",
            "烦躁": f"{elder['name']}今日情绪烦躁，对护工态度不佳，拒绝配合护理。耐心沟通后情绪稍平复。"
        }

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=random.randint(9, 17), minute=random.randint(0, 59)).isoformat(),
            "event_type": "mood",
            "severity": "low" if mood == "平和" else "medium",
            "content": templates[mood],
            "tags": ["情绪", mood, "心理护理"]
        }

    def _generate_confusion_event(self, elder: Dict, date: datetime) -> Dict:
        """生成谵妄/认知混乱事件（阿尔茨海默病患者）"""
        if "阿尔茨海默病" not in elder["chronic_diseases"]:
            return None

        hour = random.choice([2, 3, 14, 20])
        minute = random.randint(0, 59)

        template1 = (
            f"{elder['name']}，{elder['age']}岁，阿尔茨海默病中期，服用多奈哌齐10mg/日。"
            f"夜班护士记录：凌晨{hour}:{minute:02d}，患者独自下床，试图走出病房，称'要回家做饭'。"
            f"护士劝阻时患者情绪激动，推搡护士。判断为夜间谵妄发作。"
            f"处理：使用安抚技术，陪伴患者回房间，播放其喜爱的老歌，逐渐平复情绪。"
            f"未使用约束带，遵循非药物干预原则。家属已告知，建议调整夜间照护策略。"
        )

        template2 = (
            f"{elder['name']}下午出现定向力障碍，不认识护工，坚称自己在家中。"
            f"护士耐心引导，使用怀旧疗法，向患者展示熟悉的物品，情绪逐渐稳定。"
        )

        templates = [template1, template2]

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=hour, minute=random.randint(0, 59)).isoformat(),
            "event_type": "confusion",
            "severity": "high",
            "content": random.choice(templates),
            "tags": ["认知障碍", "谵妄", "阿尔茨海默病", "紧急处理"]
        }

    def _generate_pain_event(self, elder: Dict, date: datetime) -> Dict:
        """生成疼痛记录"""
        pain_location = random.choice(["腰部", "膝关节", "右肩", "头部", "腹部"])
        pain_level = random.randint(2, 8)

        content = f"{elder['name']}诉{pain_location}疼痛，疼痛评分{pain_level}/10分。"

        if pain_level >= 7:
            content += "疼痛剧烈，影响活动。已通知医生，遵医嘱给予止痛药物，局部冷敷处理。"
        elif pain_level >= 4:
            content += "疼痛明显。协助患者调整体位，进行物理治疗，疼痛有所缓解。"
        else:
            content += "轻微疼痛，患者可耐受，暂未特殊处理，继续观察。"

        return {
            "elder_id": elder["elder_id"],
            "elder_name": elder["name"],
            "timestamp": date.replace(hour=random.randint(8, 20), minute=random.randint(0, 59)).isoformat(),
            "event_type": "pain",
            "severity": "high" if pain_level >= 7 else "medium",
            "content": content,
            "tags": ["疼痛", pain_location, f"评分{pain_level}"],
            "structured_data": {"pain_location": pain_location, "pain_level": pain_level}
        }

    def generate_nursing_records(self, elders: List[Dict], days: int = 30) -> List[Dict]:
        """为所有老人生成护理记录"""
        records = []
        start_date = datetime.now() - timedelta(days=days)

        for elder in elders:
            # 每个老人每天生成2-5条记录
            for day in range(days):
                current_date = start_date + timedelta(days=day)
                num_events = random.randint(2, 5)

                for _ in range(num_events):
                    event_generator = random.choice(self.event_templates)
                    event = event_generator(elder, current_date)

                    if event:
                        records.append(event)

        # 按时间排序
        records.sort(key=lambda x: x["timestamp"])

        return records

    def generate_all(self, num_elders: int = 100):
        """生成所有数据"""
        print(f"[GENERATE] Generating {num_elders} elder profiles...")

        # 生成老人档案
        elders = [self.generate_elder_profile(i+1) for i in range(num_elders)]

        # 保存档案
        profiles_file = self.output_dir / "elder_profiles.json"
        with open(profiles_file, 'w', encoding='utf-8') as f:
            json.dump(elders, f, ensure_ascii=False, indent=2)
        print(f"[OK] Saved {len(elders)} profiles -> {profiles_file}")

        # 生成护理记录
        print(f"[GENERATE] Generating nursing records (30 days)...")
        records = self.generate_nursing_records(elders, days=30)

        # 保存记录
        records_file = self.output_dir / "nursing_records.json"
        with open(records_file, 'w', encoding='utf-8') as f:
            json.dump(records, f, ensure_ascii=False, indent=2)
        print(f"[OK] Saved {len(records)} nursing records -> {records_file}")

        # 统计信息
        print(f"\n[STATS] Dataset statistics:")
        print(f"  - Elder profiles: {len(elders)}")
        print(f"  - Nursing records: {len(records)}")
        print(f"  - Time range: {records[0]['timestamp'][:10]} to {records[-1]['timestamp'][:10]}")

        event_types = {}
        for record in records:
            event_type = record["event_type"]
            event_types[event_type] = event_types.get(event_type, 0) + 1

        print(f"  - Event types:")
        for event_type, count in sorted(event_types.items()):
            print(f"    * {event_type}: {count}")

        return elders, records

if __name__ == "__main__":
    generator = NursingDataGenerator()
    generator.generate_all(num_elders=100)
