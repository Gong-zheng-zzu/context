# P0第二轮修复：API对接与完整验证

**修复时间**: 2026-07-13 下午  
**修复范围**: 安全API认证、遗忘完整流程、烟雾测试失败策略

---

## 修复内容

### 1. 安全API：修复路由 + 实现JWT认证

**问题诊断**：
- ❌ 原代码调用不存在的 `/api/v1/chat`
- ❌ 缺少JWT认证流程

**实际后端路由**（从 `main_http.go` 确认）：
- 登录端点：`POST /api/auth/login`（公开，无需认证）
- 聊天端点：`POST /api/chat`（受保护，需要JWT）

**登录请求字段**（从 `auth_handlers.go` 确认）：
```json
{
  "user_id": "test_user",
  "password": "test_pass",
  "workspace_id": "default"
}
```

**修复实施**：

修改文件：
- `base_evaluator.py` - 新增 `login()` 和 `get_auth_headers()`
- `smoke_test.py` - 改为两步骤（登录→聊天）
- `security_eval.py` - 发送攻击前自动登录

---

### 2. 遗忘API：改为完整前后验证

**问题诊断**：
- ❌ 原烟雾测试只调用 `GET /api/v1/unlearning/config`
- ❌ 未验证 `DELETE` 端点和遗忘效果

**修复实施**：四步骤验证流程
1. 验证配置API可达
2. 遗忘前查询目标用户数据
3. 执行DELETE操作
4. 遗忘后查询验证

通过条件：配置API返回200 + DELETE返回2xx + 遗忘前后查询都返回200

---

### 3. 烟雾测试：失败时不降级

**修复**：
- 失败时输出清晰的修复指引
- 不使用降级数据，明确要求修复API

---

## 修改文件清单

1. `experiments/scripts/base_evaluator.py` - 新增认证方法
2. `experiments/scripts/smoke_test.py` - 修改安全和遗忘测试
3. `experiments/scripts/security_eval.py` - 使用实际路由+JWT

---

## 验证方法

```bash
cd experiments/scripts
python smoke_test.py
```

预期：安全API从404改为PASS，遗忘API从单一GET改为完整流程验证

---

## 剩余问题

- P1：Ollama模型加载（因果API 500）
- P1：检索底层实现（SearchByQuery未实现）
- P2：配置切换机制（X-Config-Name被忽略）

---

## 核心原则

1. 不硬编码生产配置（JWT密钥由环境变量控制）
2. 不降级掩盖问题（烟雾测试失败即失败）
3. 对齐后端契约（使用实际endpoint和字段名）
4. 完整流程验证（测试完整调用链，不只测单点）
