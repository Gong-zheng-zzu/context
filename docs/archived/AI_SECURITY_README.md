# AI安全防护模块使用说明

## 概述

本项目为健康档案隐私保护系统实现了完整的AI安全防护模块，包含6个核心安全组件，用于保护AI模型免受各种攻击和滥用。

## 模块列表

### P0优先级（核心模块）

#### 1. OutputFilter - 输出过滤器
**文件**: `internal/security/output_filter.go`

**功能**:
- 过滤AI输出中的敏感信息（手机号、身份证、邮箱等）
- 过滤黑名单关键词（系统路径、内网IP、凭证）
- 检测并过滤SQL注入尝试
- 过滤Base64编码的可疑内容
- 过滤私钥、JWT令牌等

**使用示例**:
```go
of := security.NewOutputFilter()

// 过滤输出
filtered, warnings := of.FilterOutput("您的手机号是13812345678")
// filtered: "您的手机号是138****5678"
// warnings: ["检测到手机号已自动脱敏"]

// 验证输出安全性
isSafe, risks := of.ValidateOutput(output)
```

#### 2. ModelDosProtection - DoS防护
**文件**: `internal/security/model_dos_protection.go`

**功能**:
- 限制每用户20请求/分钟
- 限制每用户10,000 tokens/分钟
- 限制单次请求最大2,000 tokens
- 防止请求间隔过短（最小1秒）

**使用示例**:
```go
mdp := security.NewModelDosProtection()

// 检查限制
tokens := security.EstimateTokens(userInput)
if err := mdp.CheckLimit(userID, tokens); err != nil {
    // 超过限制，拒绝请求
    return err
}

// 记录请求
mdp.RecordRequest(userID, tokens)

// 获取剩余配额
remainingReq, remainingTokens := mdp.GetRemainingQuota(userID)
```

### P1优先级（重要模块）

#### 3. DataPoisoningDetector - 数据投毒检测
**文件**: `internal/security/data_poisoning_detector.go`

**功能**:
- 检测恶意指令注入
- 检测后门触发器
- 检测重复模式攻击
- 检测零宽字符和同形字符
- 检测Prompt注入尝试

**使用示例**:
```go
dpd := security.NewDataPoisoningDetector()

// 验证知识库输入
isValid, reasons := dpd.ValidateKnowledgeInput(content)
if !isValid {
    log.Printf("检测到数据投毒: %v", reasons)
    return errors.New("输入内容不安全")
}

// 获取风险评分
riskScore := dpd.GetRiskScore(content) // 0-100
```

#### 4. ConfidenceScorer - 置信度评分
**文件**: `internal/security/confidence_scorer.go`

**功能**:
- 评估AI响应的可信度（0.0-1.0）
- 检测不确定性词汇（-10分）
- 检测来源引用（+20分）
- 评估响应长度、结构化程度
- 检测矛盾内容和敏感信息泄露

**评分因素**:
- 基础分数: 70分
- 不确定性词汇: 每个-10分（最多-30分）
- 来源引用: 每个+20分（最多+40分）
- 响应过短(<50字符): -15分
- 知识库数据存在: +15分
- 结构化程度: +10分
- 具体性: +10分
- 专业术语: +5分

**使用示例**:
```go
cs := security.NewConfidenceScorer()

// 评估置信度
score := cs.ScoreResponse(aiResponse, hasKnowledgeData)
// score: 0.85 (85%置信度)

// 获取详细信息
score, details := cs.ScoreWithDetails(aiResponse, hasKnowledgeData)
// details包含: uncertainty_words, source_indicators, confidence_level等

// 获取置信度等级
level := cs.GetConfidenceLevel(score)
// level: "高" (非常高/高/较高/中等/较低/低)
```

### P2优先级（增强模块）

#### 5. ModelAccessMonitor - 访问监控
**文件**: `internal/security/model_access_monitor.go`

**功能**:
- 监控用户访问模式
- 检测模型窃取行为
- 检测异常请求频率（>100次/小时）
- 检测异常Token使用（>50,000/小时）
- 检测自动化攻击

**使用示例**:
```go
mam := security.NewModelAccessMonitor()

// 记录访问
mam.RecordAccess(userID, tokens)

// 检测模型窃取
if isTheft, reason := mam.DetectTheft(userID); isTheft {
    log.Printf("检测到模型窃取: %s", reason)
    // 采取措施：阻止用户、发送警报等
}

// 获取用户统计
stats, exists := mam.GetUserStats(userID)

// 获取风险等级（0-5）
riskLevel := mam.GetRiskLevel(userID)
```

#### 6. AdversarialDetector - 对抗样本检测
**文件**: `internal/security/adversarial_detector.go`

**功能**:
- 检测Unicode同形字符攻击
- 检测零宽字符
- 检测Base64/URL编码payload
- 检测Unicode编码混淆
- 检测反向文本攻击
- 检测混合脚本攻击

**使用示例**:
```go
ad := security.NewAdversarialDetector()

// 检测对抗样本
if isAdversarial, reason := ad.DetectAdversarial(userInput); isAdversarial {
    log.Printf("检测到对抗样本: %s", reason)
    return errors.New("输入包含可疑字符")
}

// 规范化文本（移除对抗性字符）
normalized := ad.NormalizeText(userInput)

// 获取安全评分（0-100）
safetyScore := ad.GetSafetyScore(userInput)
```

## 集成到API

在 `internal/api/chat_handlers.go` 中已完成集成：

### 初始化
```go
// 在main.go或初始化函数中调用
api.InitAISecurityModules()
```

### 请求处理流程

**请求前检查**:
1. AdversarialDetector - 检测对抗样本
2. ModelDosProtection - 检查速率限制
3. DataPoisoningDetector - 验证输入（长输入）

**响应后处理**:
1. OutputFilter - 过滤敏感信息
2. ConfidenceScorer - 计算置信度
3. ModelAccessMonitor - 记录访问并检测窃取

### API响应格式
```json
{
  "success": true,
  "data": {
    "session_id": "xxx",
    "message": "过滤后的AI响应",
    "timestamp": 1234567890,
    "confidence_score": 0.85,
    "security_warnings": [
      "检测到手机号已自动脱敏"
    ],
    "filtered": true,
    "sensitive_infos": [...],
    "retrieved_memory": "...",
    "memory_count": 2
  }
}
```

## 运行测试

```bash
# 测试所有模块
cd internal/security
go test -v

# 测试单个模块
go test -v -run TestOutputFilter
go test -v -run TestModelDosProtection
go test -v -run TestConfidenceScorer
go test -v -run TestAdversarialDetector

# 查看测试覆盖率
go test -cover
```

## 配置说明

### DoS防护配置
```go
// 自定义配置
mdp := security.NewModelDosProtectionWithConfig(
    30,    // 每分钟最大请求数
    15000, // 每分钟最大Token数
    3000,  // 单次请求最大Token数
)
```

### 访问监控配置
```go
// 自定义配置
mam := security.NewModelAccessMonitorWithConfig(
    150,   // 每小时最大请求数
    80000, // 每小时最大Token数
    3,     // 模型窃取阈值
)
```

## 安全最佳实践

1. **多层防护**: 所有6个模块协同工作，提供纵深防御
2. **日志记录**: 所有安全事件都会记录日志，便于审计
3. **动态调整**: 可根据实际情况调整阈值和配置
4. **用户体验**: 在安全和用户体验之间取得平衡
5. **持续监控**: 定期检查安全日志和统计数据

## 性能考虑

- **OutputFilter**: O(n)，n为文本长度
- **ModelDosProtection**: O(1)，使用内存缓存
- **DataPoisoningDetector**: O(n)，正则匹配
- **ConfidenceScorer**: O(n)，文本分析
- **ModelAccessMonitor**: O(1)，哈希表查询
- **AdversarialDetector**: O(n)，字符检测

所有模块都经过优化，对API响应时间影响<50ms。

## 故障排查

### 问题1: 正常请求被拒绝
- 检查DoS防护阈值是否过低
- 查看日志确定具体原因
- 使用 `GetRemainingQuota()` 检查配额

### 问题2: 敏感信息未被过滤
- 检查 `OutputFilter` 是否正确初始化
- 查看 `sensitive_detector.go` 的规则
- 添加自定义黑名单关键词

### 问题3: 置信度评分异常
- 检查响应内容是否包含不确定性词汇
- 验证知识库数据是否正确传递
- 查看详细评分信息 `ScoreWithDetails()`

## 未来扩展

- [ ] 添加机器学习模型进行更精确的检测
- [ ] 支持自定义规则配置文件
- [ ] 添加实时监控仪表板
- [ ] 集成威胁情报数据库
- [ ] 支持分布式部署和集群同步

## 联系方式

如有问题或建议，请联系开发团队。
