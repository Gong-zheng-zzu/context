# 安全模块集成指南

## 📋 概述

本文档说明如何将敏感信息检测、审计日志、策略管理和合规检查等安全功能集成到 Context-Keeper 的对话存储流程中。

## 🏗️ 架构设计

```
用户对话
    ↓
MCP 接口层 (store_conversation)
    ↓
安全服务层 (SecurityService)
    ├─ 敏感信息检测 (SensitiveDetector)
    ├─ 策略评估 (PolicyManager)
    ├─ 合规检查 (ComplianceEngine)
    ├─ 内容脱敏/加密 (Encryptor)
    └─ 审计日志 (AuditLogger)
    ↓
存储层 (SessionManager)
    ├─ 短期记忆 (JSON 文件)
    ├─ 长期记忆 (向量数据库)
    └─ 审计日志 (TimescaleDB)
```

## 📦 已实现的模块

### 1. 敏感信息检测器 (`sensitive_detector.go`)

**功能：**
- 检测 15+ 种敏感信息类型（API密钥、密码、邮箱、电话、身份证等）
- 计算检测置信度（0.0-1.0）
- 支持自定义正则表达式规则

**使用示例：**
```go
detector := security.NewSensitiveDetector()
results := detector.Detect("我的邮箱是 user@example.com")

for _, info := range results {
    fmt.Printf("类型: %s, 内容: %s, 置信度: %.2f\n", 
        info.Type, info.Value, info.Confidence)
}
```

### 2. 加密器 (`encryptor.go`)

**功能：**
- AES-256-GCM 加密敏感数据
- 支持加密/解密操作
- 密钥管理

**使用示例：**
```go
encryptor := security.NewEncryptor("your-secret-key")
encrypted, _ := encryptor.Encrypt("sensitive-data")
decrypted, _ := encryptor.Decrypt(encrypted)
```

### 3. 审计日志系统 (`audit_logger.go`, `audit_types.go`)

**功能：**
- 记录所有安全事件
- 支持按时间、用户、事件类型查询
- 导出审计报告

**事件类型：**
- `sensitive_detected` - 检测到敏感信息
- `content_redacted` - 内容已脱敏
- `access_blocked` - 访问被阻止
- `policy_violation` - 策略违规
- `compliance_check` - 合规检查

### 4. 策略管理器 (`policy_manager.go`)

**功能：**
- 定义安全策略规则
- 评估内容是否违反策略
- 支持策略的启用/禁用

**策略示例：**
```go
policy := &SecurityPolicy{
    ID:          "policy-001",
    Name:        "禁止存储API密钥",
    Description: "检测到API密钥时阻止存储",
    Rules: []PolicyRule{
        {
            Type:       "api_key",
            Threshold:  0.8,
            Action:     ActionBlock,
        },
    },
}
```

### 5. 合规检查引擎 (`compliance_engine.go`)

**功能：**
- 支持 GDPR、HIPAA、PCI-DSS 等合规标准
- 生成合规报告
- 识别违规项并提供建议

### 6. 安全服务集成层 (`security_service.go`)

**功能：**
- 整合所有安全模块
- 提供统一的安全检查接口
- 在消息存储前自动执行安全检查

## 🔧 集成步骤

### 步骤 1：初始化安全服务

在 `cmd/server/main.go` 中初始化安全服务：

```go
package main

import (
    "context-keeper/internal/security"
    "context-keeper/internal/storage"
)

func main() {
    // 初始化存储
    sessionManager := storage.NewSessionManager("./data/sessions")
    
    // 初始化安全组件
    detector := security.NewSensitiveDetector()
    encryptor := security.NewEncryptor(os.Getenv("ENCRYPTION_KEY"))
    auditLogger := security.NewFileAuditLogger("./data/audit")
    policyManager := security.NewPolicyManager()
    complianceEngine := security.NewComplianceEngine()
    
    // 创建安全服务
    securityService := security.NewSecurityService(
        detector,
        encryptor,
        auditLogger,
        policyManager,
        complianceEngine,
    )
    
    // 配置默认策略
    setupDefaultPolicies(policyManager)
    
    // 启动服务器
    server := NewServer(sessionManager, securityService)
    server.Start()
}

func setupDefaultPolicies(pm *security.PolicyManager) {
    // 策略1：高风险敏感信息阻止存储
    pm.AddPolicy(&security.SecurityPolicy{
        ID:   "block-high-risk",
        Name: "阻止高风险敏感信息",
        Rules: []security.PolicyRule{
            {Type: "private_key", Threshold: 0.8, Action: security.ActionBlock},
            {Type: "password", Threshold: 0.8, Action: security.ActionBlock},
            {Type: "api_key", Threshold: 0.9, Action: security.ActionRedact},
        },
        Enabled: true,
    })
    
    // 策略2：个人信息脱敏
    pm.AddPolicy(&security.SecurityPolicy{
        ID:   "redact-pii",
        Name: "个人信息脱敏",
        Rules: []security.PolicyRule{
            {Type: "email", Threshold: 0.7, Action: security.ActionRedact},
            {Type: "phone", Threshold: 0.7, Action: security.ActionRedact},
            {Type: "id_card", Threshold: 0.8, Action: security.ActionRedact},
        },
        Enabled: true,
    })
}
```

### 步骤 2：修改对话存储流程

在 `internal/mcp/tools.go` 中集成安全检查：

```go
func (s *Server) handleStoreConversation(params map[string]interface{}) (interface{}, error) {
    sessionID := params["session_id"].(string)
    messages := params["messages"].([]interface{})
    
    // 获取会话
    session, err := s.sessionManager.GetSession(sessionID)
    if err != nil {
        return nil, err
    }
    
    // 对每条消息进行安全检查
    processedMessages := make([]Message, 0)
    for _, msg := range messages {
        message := msg.(map[string]interface{})
        content := message["content"].(string)
        
        // 🔒 安全检查和处理
        result, err := s.securityService.ProcessMessage(sessionID, content)
        if err != nil {
            return nil, err
        }
        
        // 根据安全检查结果决定是否存储
        if result.Action == security.ActionBlock {
            return nil, fmt.Errorf("消息包含违规内容，已阻止存储")
        }
        
        // 使用处理后的内容（可能已脱敏）
        processedMessages = append(processedMessages, Message{
            Role:      message["role"].(string),
            Content:   result.ProcessedContent,
            Timestamp: time.Now(),
        })
    }
    
    // 存储处理后的消息
    session.Messages = append(session.Messages, processedMessages...)
    err = s.sessionManager.SaveSession(session)
    
    return map[string]interface{}{
        "success": true,
        "stored":  len(processedMessages),
        "security_report": result.Report,
    }, err
}
```

### 步骤 3：添加安全查询接口

添加新的 MCP 工具用于查询安全信息：

```go
// 查询审计日志
func (s *Server) handleQueryAuditLog(params map[string]interface{}) (interface{}, error) {
    filters := make(map[string]interface{})
    
    if sessionID, ok := params["session_id"].(string); ok {
        filters["session_id"] = sessionID
    }
    if eventType, ok := params["event_type"].(string); ok {
        filters["event_type"] = eventType
    }
    
    events, err := s.securityService.QueryAuditLog(filters)
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "events": events,
        "count":  len(events),
    }, nil
}

// 生成合规报告
func (s *Server) handleGenerateComplianceReport(params map[string]interface{}) (interface{}, error) {
    sessionID := params["session_id"].(string)
    
    session, err := s.sessionManager.GetSession(sessionID)
    if err != nil {
        return nil, err
    }
    
    // 对所有消息进行合规检查
    report := s.securityService.GenerateComplianceReport(session)
    
    return report, nil
}
```

### 步骤 4：配置环境变量

在 `.env` 文件中添加安全配置：

```bash
# 加密密钥（32字节）
ENCRYPTION_KEY=your-32-byte-encryption-key-here

# 审计日志路径
AUDIT_LOG_PATH=./data/audit

# 安全策略配置
SECURITY_POLICY_STRICT=true
ENABLE_AUTO_REDACTION=true
ENABLE_COMPLIANCE_CHECK=true

# 合规标准
COMPLIANCE_STANDARDS=GDPR,PCI-DSS,SOC2
```

## 📊 可视化监控

已创建安全监控中心界面：`data/temp/security-dashboard.html`

**功能：**
- 实时统计（扫描次数、检测数量、合规率）
- 敏感信息检测测试
- 内容脱敏演示
- 合规检查报告
- 实时审计日志

**使用方法：**
1. 在浏览器中打开 `file:///d:/context/context-keeper-main/data/temp/security-dashboard.html`
2. 输入测试文本
3. 点击相应按钮进行测试

## 🧪 测试流程

### 1. 单元测试

创建 `internal/security/security_test.go`：

```go
package security

import "testing"

func TestSensitiveDetector(t *testing.T) {
    detector := NewSensitiveDetector()
    
    tests := []struct {
        input    string
        expected int
    }{
        {"我的邮箱是 user@example.com", 1},
        {"API密钥：sk-1234567890abcdef", 1},
        {"电话：13800138000，邮箱：test@test.com", 2},
    }
    
    for _, tt := range tests {
        results := detector.Detect(tt.input)
        if len(results) != tt.expected {
            t.Errorf("期望检测到 %d 项，实际 %d 项", tt.expected, len(results))
        }
    }
}
```

### 2. 集成测试

使用 `security-dashboard.html` 进行手动测试：

1. **敏感信息检测测试**
   - 输入包含邮箱、电话、API密钥的文本
   - 验证是否正确识别

2. **脱敏功能测试**
   - 输入敏感信息
   - 验证脱敏后的内容是否正确

3. **合规检查测试**
   - 输入不同类型的敏感信息
   - 验证合规报告是否准确

### 3. API 测试

使用 curl 测试 MCP 接口：

```bash
# 存储包含敏感信息的对话
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "store_conversation",
      "arguments": {
        "session_id": "test-security",
        "messages": [
          {
            "role": "user",
            "content": "我的API密钥是 sk-1234567890abcdef"
          }
        ]
      }
    },
    "id": 1
  }'

# 查询审计日志
curl -X POST http://localhost:8088/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "params": {
      "name": "query_audit_log",
      "arguments": {
        "session_id": "test-security"
      }
    },
    "id": 2
  }'
```

## 📈 性能优化建议

1. **缓存检测结果**
   - 对相同内容的检测结果进行缓存
   - 使用 LRU 缓存策略

2. **异步审计日志**
   - 审计日志写入使用异步队列
   - 避免阻塞主流程

3. **批量处理**
   - 对多条消息进行批量安全检查
   - 减少重复初始化开销

4. **正则表达式优化**
   - 预编译所有正则表达式
   - 使用更高效的匹配算法

## 🔐 安全最佳实践

1. **密钥管理**
   - 使用环境变量存储加密密钥
   - 定期轮换密钥
   - 使用密钥管理服务（如 AWS KMS）

2. **审计日志**
   - 记录所有安全事件
   - 定期备份审计日志
   - 设置日志保留策略

3. **访问控制**
   - 限制审计日志的访问权限
   - 实施最小权限原则

4. **合规性**
   - 定期进行合规审计
   - 更新合规规则以符合最新标准

## 📚 相关文档

- [敏感信息检测规则](./docs/sensitive-patterns.md)
- [安全策略配置](./docs/security-policies.md)
- [合规标准说明](./docs/compliance-standards.md)
- [审计日志格式](./docs/audit-log-format.md)

## 🎯 下一步计划

- [ ] 实现实时告警系统（Webhook、邮件、Slack）
- [ ] 添加机器学习模型提升检测准确率
- [ ] 支持自定义敏感信息规则
- [ ] 集成第三方安全扫描服务
- [ ] 实现细粒度的访问控制（RBAC）
- [ ] 添加数据脱敏的可逆性（授权解密）
- [ ] 支持多租户隔离
- [ ] 实现安全事件的可视化分析

## 💡 常见问题

**Q: 如何添加自定义敏感信息类型？**

A: 在 `sensitive_detector.go` 中添加新的正则表达式规则：

```go
detector.AddPattern("custom_type", regexp.MustCompile(`your-pattern`))
```

**Q: 如何调整检测的严格程度？**

A: 修改策略中的 `Threshold` 值，范围 0.0-1.0，值越高越严格。

**Q: 审计日志会占用多少存储空间？**

A: 取决于对话量，建议设置日志轮转策略，定期归档旧日志。

**Q: 如何导出合规报告？**

A: 调用 `GenerateComplianceReport` API，支持导出为 JSON、PDF 格式。

---

**文档版本：** 1.0  
**最后更新：** 2024-01-XX  
**维护者：** Context-Keeper Team
