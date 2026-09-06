#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
向量库数据准备脚本 - 为检索实验写入ground truth对应的测试数据
"""

import json
import requests
import argparse
import os
from pathlib import Path
from datetime import datetime

class VectorDataSeeder:
    """向量库数据种子脚本"""

    def __init__(self, base_url: str = "http://localhost:8088"):
        self.base_url = base_url
        self.project_root = Path(__file__).parent.parent
        self.datasets_dir = self.project_root / "datasets"
        self.auth_token = None

    def authenticate(self, user_id: str, password: str):
        """获取JWT token"""
        url = f"{self.base_url}/api/auth/login"
        payload = {
            "user_id": user_id,
            "password": password
        }

        try:
            response = requests.post(url, json=payload, timeout=10)

            if response.status_code == 200:
                result = response.json()
                # 处理嵌套的响应结构 {"success": true, "data": {"token": "..."}}
                if result.get("success") and result.get("data"):
                    self.auth_token = result["data"].get("token")
                else:
                    # 降级：直接从顶层获取token（兼容旧版API）
                    self.auth_token = result.get("token")

                if self.auth_token:
                    print(f"[AUTH] 成功获取token (user_id={user_id})")
                    return True
                else:
                    print(f"[AUTH] 响应中未找到token: {result}")
                    return False
            else:
                print(f"[AUTH] 登录失败: HTTP {response.status_code}, {response.text[:100]}")
                return False

        except Exception as e:
            print(f"[AUTH] 登录异常: {e}")
            return False

    def get_auth_headers(self):
        """获取认证header"""
        if self.auth_token:
            return {"Authorization": f"Bearer {self.auth_token}"}
        return {}

    def delete_session(self, session_id: str):
        """删除会话（清空所有数据）"""
        url = f"{self.base_url}/api/sessions/{session_id}"
        headers = self.get_auth_headers()

        try:
            response = requests.delete(url, headers=headers, timeout=10)
            if response.status_code in [200, 204, 404]:
                print(f"[DELETE] 会话已删除: {session_id}")
                return True
            else:
                print(f"[DELETE] 删除失败: HTTP {response.status_code}, {response.text[:100]}")
                return False
        except Exception as e:
            print(f"[DELETE] 删除异常: {e}")
            return False

    def load_ground_truth_docs(self):
        """加载ground truth中引用的文档ID"""
        gt_file = self.datasets_dir / "retrieval_groundtruth" / "query_answer_pairs.json"

        with open(gt_file, 'r', encoding='utf-8') as f:
            data = json.load(f)
            queries = data.get("queries", data)

        # 提取所有唯一的doc_id
        doc_ids = set()
        for query in queries:
            doc_ids.update(query.get("ground_truth_docs", []))

        return sorted(doc_ids)

    def generate_test_documents(self, doc_ids):
        """为每个doc_id生成测试文档内容"""
        documents = []

        for doc_id in doc_ids:
            # 解析doc_id格式: elder_080_2026-06-06T02:10:43.194500
            parts = doc_id.split('_')
            if len(parts) >= 3:
                elder_id = f"{parts[0]}_{parts[1]}"  # elder_080
                timestamp = '_'.join(parts[2:])       # 2026-06-06T02:10:43.194500
            else:
                elder_id = doc_id
                timestamp = datetime.now().isoformat()

            # 生成文档内容（包含患者信息和健康事件）
            content = f"""患者ID: {elder_id}
记录时间: {timestamp}
健康状态: 认知障碍评估
症状描述: 患者出现定向力障碍，夜间谵妄发作，情绪激动。
护理记录: 凌晨观察到患者行为异常，进行安抚处理。
用药情况: 多奈哌齐 10mg/日，阿尔茨海默病治疗。
诊断: 阿尔茨海默病中期，认知功能下降趋势明显。
"""

            documents.append({
                "doc_id": doc_id,
                "content": content,
                "metadata": {
                    "patient_id": elder_id,
                    "timestamp": timestamp,
                    "type": "health_record",
                    "tags": ["认知障碍", "谵妄", "阿尔茨海默病"]
                }
            })

        return documents

    def load_corpus_documents(self, corpus_file: Path):
        """Load real nursing records selected for the retrieval evaluation."""
        with open(corpus_file, 'r', encoding='utf-8') as f:
            data = json.load(f)

        records = data.get("documents", data) if isinstance(data, dict) else data
        if not isinstance(records, list) or not records:
            raise ValueError(f"语料文件没有可用文档: {corpus_file}")

        documents = []
        for record in records:
            doc_id = record.get("doc_id", "")
            content = record.get("content", "")
            metadata = record.get("metadata", {})
            if not doc_id or not content or not isinstance(metadata, dict):
                raise ValueError(f"语料文档字段不完整: {record}")
            documents.append({"doc_id": doc_id, "content": content, "metadata": metadata})
        return documents

    def store_document(self, session_id: str, doc_id: str, content: str, metadata: dict):
        """将文档存储到向量库"""
        normalized_metadata = dict(metadata)
        normalized_metadata.update({
            "doc_id": doc_id,
            "id": doc_id,
            "timestamp": metadata.get("timestamp", datetime.now().isoformat()),
            "patient_id": metadata.get("patient_id", ""),
            "type": metadata.get("type", "health_record"),
            "tags": metadata.get("tags", [])
        })

        payload = {
            "sessionId": session_id,  # 必须使用驼峰命名
            "content": content,
            "metadata": normalized_metadata
        }

        # 使用create_context API存储文档（对应后端的HandleStoreContext）
        url = f"{self.base_url}/mcp/tools/create_context"

        # 添加认证header
        headers = self.get_auth_headers()
        headers["Content-Type"] = "application/json"

        try:
            # Multi-dimensional ingestion performs local LLM analysis before
            # the vector, timeline, and graph writes. Keep the client timeout
            # above the normal single-record processing time.
            response = requests.post(url, json=payload, headers=headers, timeout=90)

            if response.status_code == 200:
                return True, None
            else:
                return False, f"HTTP {response.status_code}: {response.text[:100]}"

        except Exception as e:
            return False, str(e)

    def seed_all_documents(self, documents, session_id: str = "seed_session"):
        """批量写入所有文档"""
        print(f"[INFO] 准备写入 {len(documents)} 个文档到向量库")
        print(f"[INFO] Session ID: {session_id}\n")

        success_count = 0
        failed_count = 0

        for i, doc in enumerate(documents, 1):
            doc_id = doc["doc_id"]
            content = doc["content"]
            metadata = doc["metadata"]

            print(f"[{i}/{len(documents)}] 写入文档: {doc_id}")

            success, error = self.store_document(session_id, doc_id, content, metadata)

            if success:
                success_count += 1
                print(f"  [OK] 成功")
            else:
                failed_count += 1
                print(f"  [FAIL] 失败: {error}")

        print(f"\n[SUMMARY]")
        print(f"  成功: {success_count}")
        print(f"  失败: {failed_count}")
        print(f"  总计: {len(documents)}")

        return success_count, failed_count

    def verify_storage(self, doc_ids, session_id: str = "seed_session"):
        """验证文档是否成功存储"""
        print(f"\n[VERIFY] 验证文档是否可检索\n")

        # 随机抽取3个文档进行验证
        import random
        sample_ids = random.sample(doc_ids, min(3, len(doc_ids)))

        for doc_id in sample_ids:
            # 构造查询（使用doc_id的一部分）
            query = doc_id.split('_')[1] if '_' in doc_id else doc_id

            payload = {
                "sessionId": session_id,
                "query": query,
                "maxResults": 5,
                "contextsOnly": True,
            }

            url = f"{self.base_url}/api/mcp/tools/retrieve_context"

            # 添加认证header
            headers = self.get_auth_headers()
            headers["Content-Type"] = "application/json"

            try:
                response = requests.post(url, json=payload, headers=headers, timeout=30)

                if response.status_code == 200:
                    data = response.json()

                    # 检查是否有contexts字段（新版结构化响应）
                    contexts = data.get("contexts", [])
                    if contexts and len(contexts) > 0:
                        print(f"[OK] {doc_id} - 可检索 (返回{len(contexts)}条结构化结果)")
                        # 🔍 DEBUG: 打印第一条结果的字段
                        if len(contexts) > 0:
                            first_context = contexts[0]
                            print(f"    字段: {list(first_context.keys())}")
                            if 'doc_id' in first_context:
                                print(f"    ✅ 包含doc_id: {first_context['doc_id']}")
                            elif 'id' in first_context:
                                print(f"    ✅ 包含id: {first_context['id']}")
                            else:
                                print(f"    ❌ 不包含doc_id字段")
                    else:
                        # 降级检查relevant_knowledge字段
                        relevant_knowledge = data.get("relevant_knowledge", "")
                        if relevant_knowledge and len(relevant_knowledge) > 0:
                            print(f"[OK] {doc_id} - 可检索 (legacy格式)")
                        else:
                            print(f"[WARN] {doc_id} - 无结果")
                else:
                    print(f"[FAIL] {doc_id} - HTTP {response.status_code}: {response.text[:100]}")

            except Exception as e:
                print(f"[ERROR] {doc_id} - {e}")


def main():
    parser = argparse.ArgumentParser(description="向量库数据准备脚本")
    parser.add_argument("--base-url", default="http://localhost:8088", help="API基础URL")
    parser.add_argument("--session-id", default="eval_groundtruth", help="Session ID")
    parser.add_argument("--verify", action="store_true", help="验证存储结果")
    parser.add_argument("--force-delete", action="store_true", help="强制删除现有会话数据")
    parser.add_argument(
        "--corpus-file",
        type=Path,
        default=Path(__file__).resolve().parent.parent / "datasets" / "retrieval_corpus" / "retrieval_corpus_30.json",
        help="Fixed retrieval corpus (defaults to the validated 30-document corpus).",
    )
    parser.add_argument(
        "--doc-id",
        action="append",
        dest="doc_ids",
        help="Seed only an exact corpus document ID. Repeat for a bounded repair run.",
    )
    parser.add_argument("--skip-session-init", action="store_true", help="跳过聊天初始化，避免评测语料污染")

    args = parser.parse_args()

    seeder = VectorDataSeeder(base_url=args.base_url)

    # 认证：从环境变量读取凭据
    user_id = os.environ.get("EVAL_USER_ID", "").strip()
    password = os.environ.get("EVAL_PASSWORD", "")
    if not user_id or not password:
        print("[ERROR] 请设置 EVAL_USER_ID 和 EVAL_PASSWORD 后再运行")
        return 1

    print(f"[AUTH] 使用用户: {user_id}")
    if not seeder.authenticate(user_id, password):
        print("[ERROR] 认证失败，无法继续")
        return 1

    # 删除现有会话（如果指定了--force-delete）
    if args.force_delete:
        print(f"\n[DELETE] 强制删除模式：删除会话 {args.session_id}")
        seeder.delete_session(args.session_id)
        import time
        time.sleep(2)  # 等待删除完成

    # 初始化会话：通过chat接口创建包含userId的会话
    if not args.skip_session_init:
        print(f"\n[SESSION] 初始化会话: {args.session_id}")
        init_url = f"{seeder.base_url}/api/chat"
        init_payload = {
            "user_id": user_id,
            "session_id": args.session_id,
            "message": "初始化检索实验会话"
        }
        headers = seeder.get_auth_headers()
        headers["Content-Type"] = "application/json"

        try:
            response = requests.post(init_url, json=init_payload, headers=headers, timeout=60)
            if response.status_code == 200:
                print(f"[SESSION] 会话初始化成功")
            else:
                print(f"[WARN] 会话初始化失败: HTTP {response.status_code}, 尝试继续...")
        except Exception as e:
            print(f"[WARN] 会话初始化异常: {e}, 尝试继续...")

    print(f"\n[LOAD] 加载固定检索语料: {args.corpus_file}")
    documents = seeder.load_corpus_documents(args.corpus_file)
    if args.doc_ids:
        requested_ids = set(args.doc_ids)
        documents = [document for document in documents if document["doc_id"] in requested_ids]
        found_ids = {document["doc_id"] for document in documents}
        missing_ids = requested_ids - found_ids
        if missing_ids:
            print(f"[ERROR] corpus does not contain requested doc_id values: {sorted(missing_ids)}")
            return 1
    doc_ids = [document["doc_id"] for document in documents]
    if not args.doc_ids and len(documents) != len(set(doc_ids)):
        print("[ERROR] 检索语料必须包含唯一 doc_id")
        return 1
    print(f"[INFO] 加载 {len(documents)} 条固定文档\n")

    # 写入向量库
    success, failed = seeder.seed_all_documents(documents, session_id=args.session_id)

    # 验证存储
    if args.verify and success > 0:
        seeder.verify_storage(doc_ids, session_id=args.session_id)

    # 输出测试元数据（不包含token或密码）
    print(f"\n[METADATA]")
    print(f"  test_user_id: {user_id}")
    print(f"  session_id: {args.session_id}")
    print(f"  doc_ids: {doc_ids[:3]}... (共{len(doc_ids)}个)")

    print(f"\n[DONE] 数据准备完成")
    return 0

if __name__ == "__main__":
    import sys
    sys.exit(main())
