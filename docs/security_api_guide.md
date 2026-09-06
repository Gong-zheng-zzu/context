# 安全检测 API 使用指南

## 📋 概述

Context-Keeper 现已集成完整的安全检测 API，用于检测和脱敏对话中的敏感信息，保护企业数据安全。

---

## 🚀 快速开始

### 1. 启动服务器

```bash
cd context-keeper-main
go run -tags http cmd/server/main.go cmd/server/main_http.go cmd/server/shared.go
```

服务器将在 `http://localhost:8088` 启动。

### 2. 打开测试页面

在浏览器中打开：
```
http://localhost:8088/test-smart-decision.html
```

或者直接打开本地文件：
```
context-keeper-main/test-smart-decision.html
```

---

## 🔌 API 接口

### 1. 完整安全扫描

**接口**: `POST /api/security/scan`

**功能**: 检测敏感信息、计算风险、自动脱敏、决策安全动作

**请求示例**:
```json
{
  "content": "我的电话号码：13812345678，密码是admin123",
  "user_id": "test_user",
  "session_id": "test_session_123"
}
```

**响应示例**:
```json
{
  "original_content": "我的电话号码：13812345678，密码是admin123",
  "redacted_content": "我的电话号码：[已脱敏]，密码是[暗文]",
  "sensitive_infos": [
    {
      "type": "phone",
      "value": "13812345678",
      "start": 8,
      "end": 19,
      "label": "手机号",
      "confidence": 0.85
    },
    {
      "type": "password",
      "value": "admin123",
      "start": 23,
      "end": 31,
      "label": "密码",
      "confidence": 0.9
    }
  ],
  "avg_confidence": 0.875,
  "risk_score": 67.5,
  "risk_level": "HIGH",
  "actions": ["ALERT", "REDACT"],
  "should_block": false,
  "should_alert": true,
  "should_redact": true
}
```

---

### 2. 仅检测敏感信息

**接口**: `POST /api/security/detect`

**功能**: 仅检测敏感信息，不进行脱敏处理

**请求示例**:
```json
{
  "content": "联系邮箱：user@example.com"
}
```

**响应示例**:
```json
{
  "sensitive_infos": [
    {
      "type": "email",
      "value": "user@example.com",
      "start": 5,
      "end": 21,
      "label": "邮箱",
      "confidence": 0.9
    }
  ],
  "count": 1,
  "avg_confidence": 0.9
}
```

---

### 3. 脱敏处理

**接口**: `POST /api/security/redact`

**功能**: 对内容进行脱敏处理

**请求示例**:
```json
{
  "content": "API密钥：sk-1234567890abcdef"
}
```

**响应示例**:
```json
{
  "original_content": "API密钥：sk-1234567890abcdef",
  "redacted_content": "API密钥：[令牌]",
  "sensitive_infos": [
    {
      "type": "api_key",
      "value": "sk-1234567890abcdef",
      "start": 5,
      "end": 24,
      "label": "API 密钥",
      "confidence": 0.95
    }
  ],
  "count": 1
}
```

---

### 4. 安全统计信息

**接口**: `GET /api/security/stats`

**功能**: 获取安全检测的统计数据

**响应示例**:
```json
{
  "total_scans": 156,
  "total_detections": 89,
  "total_blocked": 12,
  "total_redacted": 77,
  "detections_by_type": {
    "phone": 23,
    "email": 18,
    "api_key": 15,
    "password": 12,
    "credit_card": 8
  },
  "last_scan_time": "2026-05-08T10:30:45Z"
}
```

---

## 🔍 支持的敏感信息类型

| 类型 | 标识 | 示例 | 脱敏标记 |
|------|------|------|---------|
| API 密钥 | `api_key` | sk-1234567890abcdef | `[令牌]` |
| 密码 | `password` | MyP@ssw0rd123 | `[暗文]` |
| 认证令牌 | `token` | eyJhbGciOiJIUzI1NiIs... | `[令牌]` |
| Bearer 令牌 | `bearer_token` | Bearer abc123... | `[令牌]` |
| AWS 密钥 | `aws_key` | AKIAIOSFODNN7EXAMPLE | `[令牌]` |
| 私钥 | `private_key` | -----BEGIN PRIVATE KEY----- | `[暗文]` |
| 数据库凭证 | `database_credential` | mongodb://user:pass@host | `[已脱敏]` |
| 信用卡号 | `credit_card` | 4111 1111 1111 1111 | `[信用卡]` |
| 社保号 | `ssn` | 123-45-6789 | `[社保号]` |
| 手机号 | `phone` | 13812345678 | `[已脱敏]` |
| 邮箱 | `email` | user@example.com | `[已脱敏]` |
| IP 地址 | `ip_address` | 192.168.1.100 | `[已脱敏]` |
| 密钥 | `secret_key` | client_secret=abc123 | `[令牌]` |

---

## 🎯 风险等级

| 等级 | 分数范围 | 说明 | 安全动作 |
|------|---------|------|---------|
| **LOW** | 0-39 | 低风险 | ALLOW（允许） |
| **MEDIUM** | 40-59 | 中等风险 | REDACT（脱敏） |
| **HIGH** | 60-79 | 高风险 | ALERT + REDACT（告警+脱敏） |
| **CRITICAL** | 80-100 | 严重风险 | BLOCK + ALERT + REDACT（拦截+告警+脱敏） |

---

## 🧪 测试场景

### 场景 1: 高置信度场景
```
我的OpenAI API密钥是sk-1234567890abcdefghijklmnopqrstuvwxyz
数据库密码是MySecureP@ssw0rd123
AWS访问密钥：AKIAIOSFODNN7EXAMPLE
```

**预期结果**: 
- 检测到 3 个敏感信息
- 风险等级: CRITICAL
- 动作: BLOCK + ALERT + REDACT

---

### 场景 2: 中等置信度场景
```
联系电话是13812345678
邮箱地址：user@example.com
服务器IP：192.168.1.100
```

**预期结果**:
- 检测到 3 个敏感信息
- 风险等级: MEDIUM
- 动作: REDACT

---

### 场景 3: 低置信度场景
```
这个数字123456可能是什么？
我的幸运数字是888888
房间号码是404
```

**预期结果**:
- 检测到 0 个敏感信息
- 风险等级: LOW
- 动作: ALLOW

---

## 🔧 配置说明

### 环境变量

```bash
# 安全配置文件路径（可选）
export SECURITY_CONFIG_PATH="./config/security_policy.yaml"

# 审计日志路径（可选）
export SECURITY_AUDIT_LOG_PATH="./data/security_audit.log"
```

### 默认配置

如果不设置环境变量，系统将使用以下默认值：
- 配置文件: `./config/security_policy.yaml`
- 审计日志: `./data/security_audit.log`

---

## 📊 使用示例（curl）

### 扫描内容
```bash
curl -X POST http://localhost:8088/api/security/scan \
  -H "Content-Type: application/json" \
  -d '{
    "content": "我的电话号码：13812345678",
    "user_id": "test_user",
    "session_id": "test_123"
  }'
```

### 获取统计信息
```bash
curl http://localhost:8088/api/security/stats
```

---

## 🛡️ 企业应用场景

### 1. AI 对话记录保护
- 自动检测用户在与 AI 助手对话中泄露的敏感信息
- 存储前自动脱敏，保护用户隐私

### 2. 代码审查
- 检测代码中硬编码的密钥、密码
- 防止敏感凭证泄露到版本控制系统

### 3. 合规审计
- 记录所有敏感信息访问
- 生成审计报告，满足 GDPR、PCI-DSS 等法规要求

### 4. 实时告警
- 高风险内容实时告警
- 通知安全团队及时处理

---

## 🔮 未来增强（可选）

### LLM 增强检测
集成大模型可以实现：
- 更智能的上下文理解
- 减少误报率
- 检测变形的敏感信息
- 动态置信度计算

实现方式：
1. 在 `security_service.go` 中集成 LLM 客户端
2. 使用 LLM 分析上下文语义
3. 结合正则表达式和 LLM 判断，提高准确率

---

## 📝 注意事项

1. **性能考虑**: 安全扫描会增加响应时间，建议异步处理
2. **误报处理**: 可以通过配置文件调整检测规则和阈值
3. **审计日志**: 定期清理审计日志，避免磁盘空间不足
4. **加密存储**: 敏感信息检测结果建议加密存储

---

## 🤝 贡献

如需添加新的敏感信息类型或改进检测规则，请修改：
- `internal/security/sensitive_detector.go` - 检测规则
- `internal/api/security_handlers.go` - API 处理逻辑

---

## 📞 支持

如有问题，请提交 Issue 或联系开发团队。
