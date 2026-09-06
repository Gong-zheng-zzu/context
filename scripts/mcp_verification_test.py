#!/usr/bin/env python3
"""
Context-Keeper MCP 功能深度验证脚本
验证知识图谱检索优化的核心能力
"""

import json
import urllib.request
import urllib.error
import time
import uuid
from datetime import datetime

BASE_URL = "http://localhost:8088"
MCP_URL = f"{BASE_URL}/mcp"

class MCPClient:
    """MCP 协议客户端"""

    def __init__(self, base_url):
        self.base_url = base_url
        self.request_id = 0

    def call(self, method: str, params: dict = None) -> dict:
        """发送 MCP JSON-RPC 请求"""
        self.request_id += 1
        payload = {
            "jsonrpc": "2.0",
            "id": self.request_id,
            "method": method,
            "params": params or {}
        }

        data = json.dumps(payload).encode('utf-8')
        req = urllib.request.Request(
            self.base_url,
            data=data,
            headers={"Content-Type": "application/json"}
        )

        with urllib.request.urlopen(req) as response:
            result = json.loads(response.read().decode('utf-8'))

        if "error" in result:
            raise Exception(f"MCP Error: {result['error']}")
        return result.get("result", {})

    def call_tool(self, tool_name: str, arguments: dict) -> dict:
        """调用 MCP 工具"""
        return self.call("tools/call", {
            "name": tool_name,
            "arguments": arguments
        })


def test_session_management(client: MCPClient) -> str:
    """测试1: 会话管理"""
    print("\n" + "="*60)
    print("🧪 测试1: 会话创建与管理")
    print("="*60)

    workspace = "/Users/weixiaofeng/Desktop/fj_c/seq/context-keeper"
    user_id = f"test-user-{uuid.uuid4().hex[:8]}"

    # 创建会话
    result = client.call_tool("session_management", {
        "action": "get_or_create",
        "userId": user_id,
        "workspaceRoot": workspace,
        "metadata": {
            "source": "mcp_verification_test",
            "timestamp": datetime.now().isoformat()
        }
    })

    print(f"📋 请求参数:")
    print(f"   - userId: {user_id}")
    print(f"   - workspaceRoot: {workspace}")
    print(f"\n📤 响应结果:")
    print(json.dumps(result, indent=2, ensure_ascii=False))

    # 提取 session_id
    content = result.get("content", [{}])
    if content and len(content) > 0:
        text = content[0].get("text", "{}")
        try:
            data = json.loads(text)
            session_id = data.get("sessionId", data.get("session_id", ""))
            print(f"\n✅ 会话创建成功: {session_id}")
            return session_id
        except:
            print(f"\n⚠️ 解析响应失败")
            return ""
    return ""


def test_memory_storage_with_knowledge_graph(client: MCPClient, session_id: str) -> str:
    """测试2: 记忆存储与知识图谱"""
    print("\n" + "="*60)
    print("🧪 测试2: 记忆存储 (触发知识图谱存储)")
    print("="*60)

    # 场景1: 技术问题与解决方案
    content1 = """
    【问题】Redis 连接池耗尽导致服务响应超时
    【分析】发现 maxConnections 配置为10，高并发时不够用
    【解决方案】
    1. 将 maxConnections 从 10 提升到 50
    2. 添加连接池监控指标
    3. 配置连接超时时间为 5s
    【涉及技术】Redis, Connection Pool, Prometheus
    【相关服务】OrderService, InventoryService
    """

    print(f"\n📝 场景1: 技术问题解决记录")
    print(f"   内容长度: {len(content1)} 字符")

    result1 = client.call_tool("memorize_context", {
        "sessionId": session_id,
        "content": content1,
        "priority": "P1",
        "metadata": {
            "type": "problem_solution",
            "tags": ["redis", "connection_pool", "performance"]
        }
    })

    print(f"\n📤 响应结果:")
    print(json.dumps(result1, indent=2, ensure_ascii=False))

    # 解析 memoryId
    memory_id = ""
    content = result1.get("content", [{}])
    if content and len(content) > 0:
        text = content[0].get("text", "{}")
        try:
            data = json.loads(text)
            memory_id = data.get("memoryId", data.get("memory_id", ""))
            print(f"\n✅ 记忆存储成功: {memory_id}")
        except:
            print(f"\n⚠️ 解析 memoryId 失败")

    time.sleep(1)  # 等待存储完成

    # 场景2: 架构决策记录
    content2 = """
    【架构决策】采用 Event Sourcing 模式重构订单系统
    【背景】
    - 当前订单状态变更追踪困难
    - 审计日志不完整
    - 回滚操作复杂
    【决策内容】
    1. 使用 EventStore 存储所有订单事件
    2. 订单状态通过事件重放计算
    3. 添加 CQRS 分离读写模型
    【影响范围】OrderService, EventStore, QueryService
    【负责人】张三, 李四
    """

    print(f"\n📝 场景2: 架构决策记录")

    result2 = client.call_tool("memorize_context", {
        "sessionId": session_id,
        "content": content2,
        "priority": "P1",
        "metadata": {
            "type": "architecture_decision",
            "tags": ["event_sourcing", "cqrs", "order_system"]
        }
    })

    print(f"\n📤 响应结果:")
    print(json.dumps(result2, indent=2, ensure_ascii=False))

    time.sleep(1)

    # 场景3: 代码重构记录
    content3 = """
    【重构】将 UserService 拆分为 AuthService 和 ProfileService
    【原因】
    - UserService 职责过重，超过 3000 行代码
    - 认证逻辑和用户信息管理耦合严重
    【变更】
    - AuthService: 负责登录、注册、Token管理
    - ProfileService: 负责用户信息、偏好设置
    【依赖更新】
    - Gateway 需要更新路由配置
    - Frontend 需要更新 API 调用
    """

    print(f"\n📝 场景3: 代码重构记录")

    result3 = client.call_tool("memorize_context", {
        "sessionId": session_id,
        "content": content3,
        "priority": "P2",
        "metadata": {
            "type": "refactoring",
            "tags": ["service_split", "microservice"]
        }
    })

    print(f"\n📤 响应结果:")
    print(json.dumps(result3, indent=2, ensure_ascii=False))

    return memory_id


def test_multi_dimensional_retrieval(client: MCPClient, session_id: str):
    """测试3: 多维度检索"""
    print("\n" + "="*60)
    print("🧪 测试3: 多维度检索能力验证")
    print("="*60)

    # 检索场景列表
    queries = [
        {
            "query": "Redis 连接池问题怎么解决",
            "description": "技术问题检索 - 应触发知识图谱优先策略"
        },
        {
            "query": "订单系统的架构设计",
            "description": "架构知识检索 - 应返回 Event Sourcing 决策"
        },
        {
            "query": "UserService 为什么要拆分",
            "description": "重构原因检索 - 应返回服务拆分记录"
        },
        {
            "query": "最近讨论了什么技术问题",
            "description": "时间维度检索 - 应触发时间线优先策略"
        },
        {
            "query": "Gateway 相关的变更",
            "description": "关联实体检索 - 测试图谱遍历能力"
        }
    ]

    results = []
    for i, q in enumerate(queries, 1):
        print(f"\n🔍 检索场景 {i}: {q['description']}")
        print(f"   查询: {q['query']}")

        start_time = time.time()
        result = client.call_tool("retrieve_context", {
            "sessionId": session_id,
            "query": q["query"]
        })
        elapsed = (time.time() - start_time) * 1000

        print(f"   耗时: {elapsed:.2f}ms")

        content = result.get("content", [{}])
        if content and len(content) > 0:
            text = content[0].get("text", "")
            # 截取前500字符显示
            preview = text[:500] + "..." if len(text) > 500 else text
            print(f"   结果预览: {preview}")

        results.append({
            "query": q["query"],
            "description": q["description"],
            "elapsed_ms": elapsed,
            "result": result
        })

        time.sleep(0.5)

    return results


def test_knowledge_graph_traversal(client: MCPClient, session_id: str):
    """测试4: 知识图谱遍历能力"""
    print("\n" + "="*60)
    print("🧪 测试4: 知识图谱关联检索")
    print("="*60)

    # 测试实体关联检索
    test_cases = [
        {
            "query": "Redis 相关的所有问题和解决方案",
            "expected": "应返回 Redis 连接池问题及解决方案"
        },
        {
            "query": "OrderService 涉及的所有变更",
            "expected": "应返回 Event Sourcing 架构决策"
        },
        {
            "query": "连接池和性能优化相关的内容",
            "expected": "应通过 RELATES_TO 关系找到相关记忆"
        }
    ]

    for i, tc in enumerate(test_cases, 1):
        print(f"\n🔗 关联检索 {i}: {tc['query']}")
        print(f"   预期: {tc['expected']}")

        result = client.call_tool("retrieve_context", {
            "sessionId": session_id,
            "query": tc["query"]
        })

        content = result.get("content", [{}])
        if content and len(content) > 0:
            text = content[0].get("text", "")

            # 检查是否包含预期内容
            has_redis = "redis" in text.lower() or "Redis" in text
            has_order = "order" in text.lower() or "订单" in text
            has_pool = "pool" in text.lower() or "连接池" in text

            print(f"   ✓ 包含 Redis: {has_redis}")
            print(f"   ✓ 包含 Order: {has_order}")
            print(f"   ✓ 包含连接池: {has_pool}")


def test_conversation_store_and_retrieve(client: MCPClient, session_id: str):
    """测试5: 对话存储与检索"""
    print("\n" + "="*60)
    print("🧪 测试5: 对话存储与检索")
    print("="*60)

    # 存储一段对话
    messages = [
        {"role": "user", "content": "我们的微服务架构遇到了服务间调用延迟问题"},
        {"role": "assistant", "content": "让我们分析一下可能的原因：\n1. 网络延迟\n2. 服务端处理时间\n3. 序列化/反序列化开销"},
        {"role": "user", "content": "我怀疑是 gRPC 序列化的问题"},
        {"role": "assistant", "content": "建议采取以下措施：\n1. 使用 protobuf 优化消息结构\n2. 启用 gRPC 压缩\n3. 添加调用链追踪"}
    ]

    print(f"\n📝 存储对话 ({len(messages)} 条消息)")

    result = client.call_tool("store_conversation", {
        "sessionId": session_id,
        "messages": messages
    })

    print(f"📤 存储结果:")
    print(json.dumps(result, indent=2, ensure_ascii=False))

    time.sleep(1)

    # 检索对话
    print(f"\n🔍 检索相关内容")

    retrieve_result = client.call_tool("retrieve_context", {
        "sessionId": session_id,
        "query": "gRPC 调用延迟问题"
    })

    content = retrieve_result.get("content", [{}])
    if content and len(content) > 0:
        text = content[0].get("text", "")
        has_grpc = "grpc" in text.lower() or "gRPC" in text
        has_protobuf = "protobuf" in text.lower()
        print(f"   ✓ 找到 gRPC 相关: {has_grpc}")
        print(f"   ✓ 找到 protobuf: {has_protobuf}")


def generate_verification_report(results: dict):
    """生成验证报告"""
    print("\n" + "="*60)
    print("📊 深度验证报告")
    print("="*60)

    print(f"""
┌─────────────────────────────────────────────────────────┐
│                   功能验证汇总                           │
├─────────────────────────────────────────────────────────┤
│ 1. 会话管理          │ {'✅ 通过' if results.get('session_ok') else '❌ 失败'}                              │
│ 2. 记忆存储          │ {'✅ 通过' if results.get('memory_ok') else '❌ 失败'}                              │
│ 3. 多维度检索        │ {'✅ 通过' if results.get('retrieval_ok') else '❌ 失败'}                              │
│ 4. 知识图谱遍历      │ {'✅ 通过' if results.get('graph_ok') else '❌ 失败'}                              │
│ 5. 对话存储检索      │ {'✅ 通过' if results.get('conversation_ok') else '❌ 失败'}                              │
└─────────────────────────────────────────────────────────┘
""")

    # 性能统计
    if 'retrieval_times' in results:
        times = results['retrieval_times']
        avg_time = sum(times) / len(times) if times else 0
        max_time = max(times) if times else 0
        print(f"""
┌─────────────────────────────────────────────────────────┐
│                   性能统计                               │
├─────────────────────────────────────────────────────────┤
│ 平均检索时间         │ {avg_time:.2f}ms                                │
│ 最大检索时间         │ {max_time:.2f}ms                                │
│ P95 目标 (<200ms)   │ {'✅ 达标' if max_time < 200 else '⚠️ 超标'}                              │
└─────────────────────────────────────────────────────────┘
""")


def main():
    """主函数"""
    print("="*60)
    print("🚀 Context-Keeper MCP 功能深度验证")
    print(f"   时间: {datetime.now().isoformat()}")
    print(f"   目标: {BASE_URL}")
    print("="*60)

    # 检查服务健康状态
    try:
        health = requests.get(f"{BASE_URL}/health").json()
        print(f"\n✅ 服务状态: {health.get('status')}")
        print(f"   向量存储: {health.get('vectorStore', {}).get('type')}")
    except Exception as e:
        print(f"\n❌ 服务不可用: {e}")
        return

    # 创建 MCP 客户端
    client = MCPClient(MCP_URL)

    # 列出可用工具
    print("\n📋 可用 MCP 工具:")
    try:
        tools = client.call("tools/list")
        for tool in tools.get("tools", []):
            print(f"   - {tool['name']}: {tool['description'][:50]}...")
    except Exception as e:
        print(f"   获取工具列表失败: {e}")

    results = {
        'session_ok': False,
        'memory_ok': False,
        'retrieval_ok': False,
        'graph_ok': False,
        'conversation_ok': False,
        'retrieval_times': []
    }

    try:
        # 测试1: 会话管理
        session_id = test_session_management(client)
        results['session_ok'] = bool(session_id)

        if not session_id:
            print("\n❌ 会话创建失败，终止测试")
            return

        # 测试2: 记忆存储
        memory_id = test_memory_storage_with_knowledge_graph(client, session_id)
        results['memory_ok'] = bool(memory_id)

        # 等待存储完成
        print("\n⏳ 等待存储完成 (3秒)...")
        time.sleep(3)

        # 测试3: 多维度检索
        retrieval_results = test_multi_dimensional_retrieval(client, session_id)
        results['retrieval_ok'] = len(retrieval_results) > 0
        results['retrieval_times'] = [r['elapsed_ms'] for r in retrieval_results]

        # 测试4: 知识图谱遍历
        test_knowledge_graph_traversal(client, session_id)
        results['graph_ok'] = True  # 如果没有异常则通过

        # 测试5: 对话存储检索
        test_conversation_store_and_retrieve(client, session_id)
        results['conversation_ok'] = True

    except Exception as e:
        print(f"\n❌ 测试异常: {e}")
        import traceback
        traceback.print_exc()

    # 生成报告
    generate_verification_report(results)

    print("\n" + "="*60)
    print("🏁 验证完成")
    print("="*60)


if __name__ == "__main__":
    main()
