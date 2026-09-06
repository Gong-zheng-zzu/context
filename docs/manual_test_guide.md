# Context-Keeper 手动测试指南

## 一、测试前准备

### 1.1 确认服务运行
```bash
# 检查服务状态
docker-compose ps

# 查看服务日志
docker-compose logs -f context-keeper

# 检查健康状态
curl http://localhost:8088/health
```

### 1.2 获取JWT Token
```bash
# 登录获取Token
curl -X POST http://localhost:8088/api/role/login \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "password": "health_assistant_2024",
    "role": "caregiver",
    "workspace_id": "default"
  }'

# 响应示例
# {"success":true,"data":{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."}}

# 保存Token到环境变量（方便后续使用）
export TOKEN="你的JWT_TOKEN"
```

---

## 二、基础功能测试

### 2.1 测试聊天功能

**单次请求**:
```bash
curl -X POST http://localhost:8088/api/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "user_id": "test_user",
    "session_id": "manual_test_001",
    "message": "你好，请介绍一下自己"
  }'
```

**预期响应**:
```json
{
  "success": true,
  "data": {
    "response": "你好！我是Context-Keeper健康助手...",
    "session_id": "manual_test_001"
  }
}
```

### 2.2 测试文件上传

**准备测试文件**:
```bash
# 创建一个测试文件
echo "这是一个测试文件，包含身份证号：110101199001010011" > test_file.txt
```

**上传文件**:
```bash
curl -X POST http://localhost:8088/api/files/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@test_file.txt" \
  -F "user_id=test_user" \
  -F "session_id=manual_test_001"
```

**预期响应**:
```json
{
  "success": true,
  "data": {
    "file_id": "abc123...",
    "file_name": "test_file.txt",
    "sensitive_detected": true,
    "sensitive_types": ["id_card"]
  }
}
```

### 2.3 测试敏感信息检测

```bash
curl -X POST http://localhost:8088/api/security/detect \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "content": "我的手机号是13800138000，身份证是110101199001010011",
    "user_id": "test_user",
    "session_id": "manual_test_001"
  }'
```

**预期响应**:
```json
{
  "sensitive_infos": [
    {
      "type": "phone",
      "value": "13800138000",
      "confidence": 0.95,
      "position": {"start": 6, "end": 17}
    },
    {
      "type": "id_card",
      "value": "110101199001010011",
      "confidence": 0.98,
      "position": {"start": 23, "end": 41}
    }
  ],
  "has_sensitive": true,
  "avg_confidence": 0.965
}
```

---

## 三、速率限制测试

### 3.1 使用curl快速测试（简单）

**测试方法**: 在1分钟内快速发送多个请求

```bash
# 发送10个请求，观察响应
for i in {1..10}; do
  echo "请求 $i:"
  curl -X POST http://localhost:8088/api/chat \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{\"user_id\":\"test_user\",\"session_id\":\"rate_test_$i\",\"message\":\"测试消息$i\"}" \
    -w "\nHTTP状态码: %{http_code}\n" \
    -s | head -n 5
  echo "---"
done
```

**预期结果**:
- 前几个请求: HTTP 200
- 如果超过限制: HTTP 429

### 3.2 使用Python脚本测试（推荐）

创建测试脚本 `test_rate_limit.py`:

```python
#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
速率限制手动测试脚本
"""

import requests
import time
from datetime import datetime

API_BASE_URL = "http://localhost:8088"

def login():
    """登录获取Token"""
    response = requests.post(
        f"{API_BASE_URL}/api/role/login",
        json={
            "user_id": "test_user",
            "password": "health_assistant_2024",
            "role": "caregiver",
            "workspace_id": "default"
        }
    )
    if response.status_code == 200:
        data = response.json()
        return data["data"]["token"]
    return None

def test_rate_limit(token, num_requests=50, interval=0.1):
    """
    测试速率限制
    
    Args:
        token: JWT Token
        num_requests: 发送请求数量
        interval: 请求间隔（秒）
    """
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {token}"
    }
    
    success_count = 0
    rate_limited_count = 0
    error_count = 0
    
    print(f"开始测试: 发送{num_requests}个请求，间隔{interval}秒")
    print(f"预期速率: {60/interval:.0f} req/min")
    print(f"当前限制: 300 req/min")
    print("="*80)
    
    start_time = time.time()
    
    for i in range(1, num_requests + 1):
        try:
            response = requests.post(
                f"{API_BASE_URL}/api/chat",
                json={
                    "user_id": "test_user",
                    "session_id": f"rate_test_{i}",
                    "message": f"测试消息{i}"
                },
                headers=headers,
                timeout=5
            )
            
            # 检查响应头中的速率限制信息
            rate_limit = response.headers.get("X-RateLimit-Limit", "N/A")
            rate_remaining = response.headers.get("X-RateLimit-Remaining", "N/A")
            rate_reset = response.headers.get("X-RateLimit-Reset", "N/A")
            
            if response.status_code == 200:
                success_count += 1
                print(f"✓ 请求{i:3d}: 成功 | 剩余: {rate_remaining}/{rate_limit}")
            elif response.status_code == 429:
                rate_limited_count += 1
                print(f"✗ 请求{i:3d}: 速率限制 (429) | 重置时间: {rate_reset}")
            else:
                error_count += 1
                print(f"✗ 请求{i:3d}: 错误 ({response.status_code})")
                
        except Exception as e:
            error_count += 1
            print(f"✗ 请求{i:3d}: 异常 - {e}")
        
        time.sleep(interval)
    
    end_time = time.time()
    duration = end_time - start_time
    actual_rate = num_requests / (duration / 60)
    
    print("="*80)
    print(f"测试完成:")
    print(f"  总请求数: {num_requests}")
    print(f"  成功: {success_count} ({success_count/num_requests*100:.1f}%)")
    print(f"  被限流: {rate_limited_count} ({rate_limited_count/num_requests*100:.1f}%)")
    print(f"  错误: {error_count} ({error_count/num_requests*100:.1f}%)")
    print(f"  实际速率: {actual_rate:.0f} req/min")
    print(f"  测试耗时: {duration:.1f}秒")

if __name__ == "__main__":
    print("Context-Keeper 速率限制测试")
    print("="*80)
    
    # 登录
    print("正在登录...")
    token = login()
    if not token:
        print("登录失败")
        exit(1)
    print(f"登录成功，Token: {token[:20]}...")
    print()
    
    # 测试场景1: 正常使用（不会触发限制）
    print("\n【场景1】正常使用 - 10 req/min")
    test_rate_limit(token, num_requests=10, interval=6.0)
    
    # 等待一段时间
    print("\n等待60秒，让速率限制重置...")
    time.sleep(60)
    
    # 测试场景2: 高频使用（可能触发限制）
    print("\n【场景2】高频使用 - 600 req/min")
    test_rate_limit(token, num_requests=50, interval=0.1)
    
    # 等待一段时间
    print("\n等待60秒，让速率限制重置...")
    time.sleep(60)
    
    # 测试场景3: 极限测试（必定触发限制）
    print("\n【场景3】极限测试 - 3000 req/min")
    test_rate_limit(token, num_requests=100, interval=0.02)
```

**运行测试**:
```bash
python test_rate_limit.py
```

### 3.3 使用浏览器测试（可视化）

创建HTML测试页面 `test_rate_limit.html`:

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>速率限制测试</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
        }
        .test-section {
            margin: 20px 0;
            padding: 20px;
            border: 1px solid #ddd;
            border-radius: 5px;
        }
        button {
            padding: 10px 20px;
            font-size: 16px;
            cursor: pointer;
            margin: 5px;
        }
        .log {
            background: #f5f5f5;
            padding: 10px;
            height: 300px;
            overflow-y: auto;
            font-family: monospace;
            font-size: 12px;
        }
        .success { color: green; }
        .error { color: red; }
        .warning { color: orange; }
    </style>
</head>
<body>
    <h1>Context-Keeper 速率限制测试</h1>
    
    <div class="test-section">
        <h2>1. 登录</h2>
        <button onclick="login()">登录获取Token</button>
        <div id="token-display"></div>
    </div>
    
    <div class="test-section">
        <h2>2. 测试场景</h2>
        <button onclick="testNormal()">正常使用 (10次，间隔1秒)</button>
        <button onclick="testHigh()">高频使用 (50次，间隔0.1秒)</button>
        <button onclick="testExtreme()">极限测试 (100次，间隔0.02秒)</button>
        <button onclick="clearLog()">清空日志</button>
    </div>
    
    <div class="test-section">
        <h2>3. 测试日志</h2>
        <div id="log" class="log"></div>
    </div>
    
    <div class="test-section">
        <h2>4. 统计信息</h2>
        <div id="stats"></div>
    </div>

    <script>
        const API_BASE_URL = 'http://localhost:8088';
        let JWT_TOKEN = null;
        let stats = { success: 0, rateLimited: 0, error: 0 };

        function log(message, type = 'info') {
            const logDiv = document.getElementById('log');
            const time = new Date().toLocaleTimeString();
            const className = type === 'success' ? 'success' : type === 'error' ? 'error' : type === 'warning' ? 'warning' : '';
            logDiv.innerHTML += `<div class="${className}">[${time}] ${message}</div>`;
            logDiv.scrollTop = logDiv.scrollHeight;
        }

        function updateStats() {
            const total = stats.success + stats.rateLimited + stats.error;
            document.getElementById('stats').innerHTML = `
                <p>总请求: ${total}</p>
                <p>成功: ${stats.success} (${total > 0 ? (stats.success/total*100).toFixed(1) : 0}%)</p>
                <p>被限流: ${stats.rateLimited} (${total > 0 ? (stats.rateLimited/total*100).toFixed(1) : 0}%)</p>
                <p>错误: ${stats.error} (${total > 0 ? (stats.error/total*100).toFixed(1) : 0}%)</p>
            `;
        }

        function clearLog() {
            document.getElementById('log').innerHTML = '';
            stats = { success: 0, rateLimited: 0, error: 0 };
            updateStats();
        }

        async function login() {
            log('正在登录...', 'info');
            try {
                const response = await fetch(`${API_BASE_URL}/api/role/login`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        user_id: 'test_user',
                        password: 'health_assistant_2024',
                        role: 'caregiver',
                        workspace_id: 'default'
                    })
                });
                
                const data = await response.json();
                if (data.success && data.data.token) {
                    JWT_TOKEN = data.data.token;
                    log('登录成功！', 'success');
                    document.getElementById('token-display').innerHTML = 
                        `<p style="color: green;">Token: ${JWT_TOKEN.substring(0, 30)}...</p>`;
                } else {
                    log('登录失败', 'error');
                }
            } catch (error) {
                log(`登录异常: ${error.message}`, 'error');
            }
        }

        async function sendRequest(index) {
            if (!JWT_TOKEN) {
                log('请先登录', 'error');
                return;
            }

            try {
                const response = await fetch(`${API_BASE_URL}/api/chat`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': `Bearer ${JWT_TOKEN}`
                    },
                    body: JSON.stringify({
                        user_id: 'test_user',
                        session_id: `browser_test_${index}`,
                        message: `测试消息${index}`
                    })
                });

                const rateLimit = response.headers.get('X-RateLimit-Limit');
                const rateRemaining = response.headers.get('X-RateLimit-Remaining');

                if (response.status === 200) {
                    stats.success++;
                    log(`请求${index}: 成功 | 剩余: ${rateRemaining}/${rateLimit}`, 'success');
                } else if (response.status === 429) {
                    stats.rateLimited++;
                    log(`请求${index}: 速率限制 (429)`, 'warning');
                } else {
                    stats.error++;
                    log(`请求${index}: 错误 (${response.status})`, 'error');
                }
            } catch (error) {
                stats.error++;
                log(`请求${index}: 异常 - ${error.message}`, 'error');
            }

            updateStats();
        }

        async function runTest(numRequests, interval) {
            log(`开始测试: ${numRequests}个请求，间隔${interval}秒`, 'info');
            for (let i = 1; i <= numRequests; i++) {
                await sendRequest(i);
                await new Promise(resolve => setTimeout(resolve, interval * 1000));
            }
            log('测试完成', 'success');
        }

        function testNormal() {
            clearLog();
            runTest(10, 1.0);
        }

        function testHigh() {
            clearLog();
            runTest(50, 0.1);
        }

        function testExtreme() {
            clearLog();
            runTest(100, 0.02);
        }
    </script>
</body>
</html>
```

**使用方法**:
1. 用浏览器打开 `test_rate_limit.html`
2. 点击"登录获取Token"
3. 选择测试场景按钮
4. 观察日志和统计信息

---

## 四、验证速率限制配置

### 4.1 检查当前配置

```bash
# 查看环境变量配置
cat config/.env | grep RATE_LIMIT

# 预期输出
# RATE_LIMIT_PER_MIN=300
# RATE_LIMIT_BURST=500
```

### 4.2 修改配置测试

**测试不同的限制值**:

```bash
# 1. 修改为严格限制
echo "RATE_LIMIT_PER_MIN=10" >> config/.env
echo "RATE_LIMIT_BURST=20" >> config/.env

# 2. 重启服务
docker-compose restart context-keeper

# 3. 运行测试（应该很快触发429）
python test_rate_limit.py

# 4. 恢复原配置
# 编辑 config/.env，改回 300/500
docker-compose restart context-keeper
```

---

## 五、测试检查清单

### 5.1 功能测试
- [ ] 登录成功，获取JWT Token
- [ ] 聊天功能正常工作
- [ ] 文件上传功能正常工作
- [ ] 敏感信息检测功能正常工作

### 5.2 速率限制测试
- [ ] 正常使用不会触发限制（10-20 req/min）
- [ ] 高频使用会触发限制（600+ req/min）
- [ ] 触发限制后返回HTTP 429
- [ ] 响应头包含速率限制信息（X-RateLimit-*）
- [ ] 等待一段时间后限制自动重置

### 5.3 边界测试
- [ ] 刚好达到限制（299 req/min）不会被限流
- [ ] 超过限制（301 req/min）会被限流
- [ ] Burst机制正常工作（短时间突发请求）

### 5.4 安全测试
- [ ] 无Token请求被拒绝（401）
- [ ] 过期Token被拒绝（401）
- [ ] 错误Token被拒绝（401）

---

## 六、常见问题排查

### 问题1: 所有请求都返回401
**原因**: Token无效或过期  
**解决**: 重新登录获取新Token

### 问题2: 速率限制不生效
**原因**: 配置未生效或服务未重启  
**解决**: 
```bash
docker-compose restart context-keeper
docker-compose logs -f context-keeper | grep "rate limit"
```

### 问题3: 测试脚本无法连接
**原因**: 服务未启动或端口错误  
**解决**:
```bash
docker-compose ps
curl http://localhost:8088/health
```

### 问题4: 浏览器测试CORS错误
**原因**: 跨域配置问题  
**解决**: 检查 `config/.env` 中的 `ALLOWED_ORIGINS` 配置

---

## 七、测试报告模板

测试完成后，记录以下信息：

```
测试日期: 2026-05-21
测试人员: [你的名字]
测试环境: 本地开发环境

配置信息:
- RATE_LIMIT_PER_MIN: 300
- RATE_LIMIT_BURST: 500

测试结果:
1. 正常使用测试 (10 req/min)
   - 总请求: 10
   - 成功: 10 (100%)
   - 被限流: 0 (0%)
   
2. 高频使用测试 (600 req/min)
   - 总请求: 50
   - 成功: 25 (50%)
   - 被限流: 25 (50%)
   
3. 极限测试 (3000 req/min)
   - 总请求: 100
   - 成功: 10 (10%)
   - 被限流: 90 (90%)

结论:
✓ 速率限制功能正常工作
✓ 正常用户不受影响
✓ 恶意高频请求被有效拦截
```

---

## 八、自动化测试脚本

如果需要定期测试，可以创建自动化脚本 `auto_test.sh`:

```bash
#!/bin/bash

echo "Context-Keeper 自动化测试"
echo "=========================="

# 1. 检查服务
echo "1. 检查服务状态..."
curl -s http://localhost:8088/health > /dev/null
if [ $? -eq 0 ]; then
    echo "✓ 服务正常运行"
else
    echo "✗ 服务未运行"
    exit 1
fi

# 2. 运行速率限制测试
echo "2. 运行速率限制测试..."
python test_rate_limit.py > test_result.log 2>&1

# 3. 检查测试结果
if grep -q "测试完成" test_result.log; then
    echo "✓ 测试完成"
    grep "成功:" test_result.log
    grep "被限流:" test_result.log
else
    echo "✗ 测试失败"
    cat test_result.log
    exit 1
fi

echo "=========================="
echo "自动化测试完成"
```

**运行自动化测试**:
```bash
chmod +x auto_test.sh
./auto_test.sh
```
