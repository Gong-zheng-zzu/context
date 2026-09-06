package security

import (
	"fmt"
	"time"
)

// 类型定义已移至 types.go

// DecisionContext 决策上下文
type DecisionContext struct {
	UserID          string                 `json:"user_id"`
	SessionID       string                 `json:"session_id"`
	MessageContent  string                 `json:"message_content"`
	SensitiveInfos  []SensitiveInfo        `json:"sensitive_infos"`
	ComplianceRules []string               `json:"compliance_rules"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// Decision 智能决策结果
type Decision struct {
	RiskLevel       RiskLevel              `json:"risk_level"`
	RiskScore       float64                `json:"risk_score"`        // 0-100
	Actions         []Action               `json:"actions"`
	Reasoning       string                 `json:"reasoning"`
	ShouldBlock     bool                   `json:"should_block"`
	ShouldEncrypt   bool                   `json:"should_encrypt"`
	ShouldAlert     bool                   `json:"should_alert"`
	RedactedContent string                 `json:"redacted_content"`
	Recommendations []string               `json:"recommendations"`
	Timestamp       time.Time              `json:"timestamp"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// SmartDecisionEngine 智能决策引擎
type SmartDecisionEngine struct {
	detector         *Detector
	policyManager    *PolicyManager
	complianceEngine *ComplianceEngine
	auditLogger      *AuditLogger
}

// NewSmartDecisionEngine 创建智能决策引擎
func NewSmartDecisionEngine(detector *Detector, policyManager *PolicyManager,
	complianceEngine *ComplianceEngine, auditLogger *AuditLogger) *SmartDecisionEngine {
	return &SmartDecisionEngine{
		detector:         detector,
		policyManager:    policyManager,
		complianceEngine: complianceEngine,
		auditLogger:      auditLogger,
	}
}

// MakeDecision 做出智能决策
func (e *SmartDecisionEngine) MakeDecision(ctx DecisionContext) (*Decision, error) {
	decision := &Decision{
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}

	// 1. 检测敏感信息（已包含置信度）
	sensitiveInfos := e.detector.Detect(ctx.MessageContent)
	ctx.SensitiveInfos = sensitiveInfos

	if len(sensitiveInfos) == 0 {
		// 无敏感信息，低风险
		decision.RiskLevel = RiskLevelLow
		decision.RiskScore = 0
		decision.Actions = []Action{ActionAllow}
		decision.Reasoning = "未检测到敏感信息"
		decision.ShouldBlock = false
		decision.ShouldEncrypt = false
		decision.ShouldAlert = false
		return decision, nil
	}

	// 2. 基于置信度计算风险分数
	riskScore := e.calculateRiskScore(sensitiveInfos)
	decision.RiskScore = riskScore

	// 3. 确定风险等级
	decision.RiskLevel = e.determineRiskLevel(riskScore)

	// 4. 应用安全策略
	actions := e.applySecurityPolicies(ctx, decision.RiskLevel, sensitiveInfos)
	decision.Actions = actions

	// 5. 生成决策建议
	decision.ShouldBlock = e.shouldBlock(actions, decision.RiskLevel)
	decision.ShouldEncrypt = e.shouldEncrypt(actions, sensitiveInfos)
	decision.ShouldAlert = e.shouldAlert(decision.RiskLevel, sensitiveInfos)

	// 6. 生成脱敏内容
	if contains(actions, ActionRedact) {
		decision.RedactedContent = e.generateRedactedContent(ctx.MessageContent, sensitiveInfos)
	}

	// 7. 生成推理说明
	decision.Reasoning = e.generateReasoning(sensitiveInfos, decision.RiskLevel, riskScore)

	// 8. 生成建议
	decision.Recommendations = e.generateRecommendations(ctx, decision)

	// 9. 记录审计日志
	e.logDecision(ctx, decision)

	return decision, nil
}

// calculateRiskScore 计算风险分数（0-100）
func (e *SmartDecisionEngine) calculateRiskScore(infos []SensitiveInfo) float64 {
	if len(infos) == 0 {
		return 0
	}

	// 基于敏感信息类型的权重
	typeWeights := map[SensitiveType]float64{
		SensitiveTypePrivateKey:  100.0, // 私钥最高风险
		SensitiveTypeAWSKey:      95.0,
		SensitiveTypePassword:    90.0,
		SensitiveTypeAPIKey:      85.0,
		SensitiveTypeSecretKey:   85.0,
		SensitiveTypeDatabase:    80.0,
		SensitiveTypeBearerToken: 75.0,
		SensitiveTypeToken:       70.0,
		SensitiveTypeCreditCard:  65.0,
		SensitiveTypeSSN:         60.0,
		SensitiveTypePhone:       30.0,
		SensitiveTypeEmail:       25.0,
		SensitiveTypeIPAddress:   20.0,
	}

	totalScore := 0.0
	maxScore := 0.0

	for _, info := range infos {
		weight := typeWeights[info.Type]
		if weight == 0 {
			weight = 50.0 // 默认权重
		}

		// 风险分数 = 类型权重 × 置信度
		score := weight * info.Confidence
		totalScore += score

		if score > maxScore {
			maxScore = score
		}
	}

	// 综合评分：最高分占70%，平均分占30%
	avgScore := totalScore / float64(len(infos))
	finalScore := maxScore*0.7 + avgScore*0.3

	// 多个敏感信息的累积效应
	if len(infos) > 1 {
		multiplier := 1.0 + float64(len(infos)-1)*0.1
		if multiplier > 1.5 {
			multiplier = 1.5 // 最多增加50%
		}
		finalScore *= multiplier
	}

	// 确保在0-100范围内
	if finalScore > 100 {
		finalScore = 100
	}

	return finalScore
}

// determineRiskLevel 确定风险等级
func (e *SmartDecisionEngine) determineRiskLevel(riskScore float64) RiskLevel {
	switch {
	case riskScore >= 80:
		return RiskLevelCritical
	case riskScore >= 60:
		return RiskLevelHigh
	case riskScore >= 30:
		return RiskLevelMedium
	default:
		return RiskLevelLow
	}
}

// applySecurityPolicies 应用安全策略
func (e *SmartDecisionEngine) applySecurityPolicies(ctx DecisionContext,
	riskLevel RiskLevel, infos []SensitiveInfo) []Action {

	actions := []Action{}

	// 基于风险等级的默认策略
	switch riskLevel {
	case RiskLevelCritical:
		actions = append(actions, ActionBlock, ActionAlert, ActionEncrypt, ActionManualReview)
	case RiskLevelHigh:
		actions = append(actions, ActionRedact, ActionAlert, ActionEncrypt)
	case RiskLevelMedium:
		actions = append(actions, ActionRedact, ActionEncrypt)
	case RiskLevelLow:
		actions = append(actions, ActionAllow)
	}

	// 基于敏感信息类型的特殊策略
	for _, info := range infos {
		switch info.Type {
		case SensitiveTypePrivateKey, SensitiveTypeAWSKey:
			// 私钥和AWS密钥必须阻止
			if !contains(actions, ActionBlock) {
				actions = append(actions, ActionBlock)
			}
			if !contains(actions, ActionAlert) {
				actions = append(actions, ActionAlert)
			}
		case SensitiveTypePassword, SensitiveTypeAPIKey:
			// 密码和API密钥必须加密
			if !contains(actions, ActionEncrypt) {
				actions = append(actions, ActionEncrypt)
			}
		}

		// 低置信度的敏感信息需要人工审核
		if info.Confidence < 0.7 && !contains(actions, ActionManualReview) {
			actions = append(actions, ActionManualReview)
		}
	}

	// 应用合规规则
	if e.complianceEngine != nil {
		for _, rule := range ctx.ComplianceRules {
			switch rule {
			case "GDPR", "HIPAA":
				// GDPR和HIPAA要求加密存储
				if !contains(actions, ActionEncrypt) {
					actions = append(actions, ActionEncrypt)
				}
			case "PCI-DSS":
				// PCI-DSS要求信用卡信息必须脱敏
				for _, info := range infos {
					if info.Type == SensitiveTypeCreditCard {
						if !contains(actions, ActionRedact) {
							actions = append(actions, ActionRedact)
						}
					}
				}
			}
		}
	}

	return actions
}

// shouldBlock 是否应该阻止
func (e *SmartDecisionEngine) shouldBlock(actions []Action, riskLevel RiskLevel) bool {
	return contains(actions, ActionBlock) || riskLevel == RiskLevelCritical
}

// shouldEncrypt 是否应该加密
func (e *SmartDecisionEngine) shouldEncrypt(actions []Action, infos []SensitiveInfo) bool {
	if contains(actions, ActionEncrypt) {
		return true
	}

	// 高风险敏感信息自动加密
	for _, info := range infos {
		if info.Type == SensitiveTypePrivateKey ||
		   info.Type == SensitiveTypePassword ||
		   info.Type == SensitiveTypeAPIKey {
			return true
		}
	}

	return false
}

// shouldAlert 是否应该告警
func (e *SmartDecisionEngine) shouldAlert(riskLevel RiskLevel, infos []SensitiveInfo) bool {
	if riskLevel == RiskLevelCritical || riskLevel == RiskLevelHigh {
		return true
	}

	// 特定类型的敏感信息总是告警
	for _, info := range infos {
		if info.Type == SensitiveTypePrivateKey ||
		   info.Type == SensitiveTypeAWSKey {
			return true
		}
	}

	return false
}

// generateRedactedContent 生成脱敏内容
func (e *SmartDecisionEngine) generateRedactedContent(content string, infos []SensitiveInfo) string {
	redacted := content

	// 从后往前替换，避免位置偏移
	for i := len(infos) - 1; i >= 0; i-- {
		info := infos[i]

		// 根据置信度决定脱敏方式
		var replacement string
		if info.Confidence >= 0.9 {
			// 高置信度：完全脱敏
			replacement = fmt.Sprintf("[%s已脱敏]", info.Label)
		} else if info.Confidence >= 0.7 {
			// 中等置信度：部分脱敏
			if len(info.Value) > 4 {
				replacement = info.Value[:2] + "***" + info.Value[len(info.Value)-2:]
			} else {
				replacement = "***"
			}
		} else {
			// 低置信度：标记但保留
			replacement = fmt.Sprintf("[可能的%s: %s]", info.Label, info.Value)
		}

		redacted = redacted[:info.Start] + replacement + redacted[info.End:]
	}

	return redacted
}

// generateReasoning 生成推理说明
func (e *SmartDecisionEngine) generateReasoning(infos []SensitiveInfo,
	riskLevel RiskLevel, riskScore float64) string {

	if len(infos) == 0 {
		return "未检测到敏感信息"
	}

	reasoning := fmt.Sprintf("检测到 %d 个敏感信息，风险评分 %.1f/100，风险等级：%s。",
		len(infos), riskScore, riskLevel)

	// 列出检测到的敏感信息类型
	typeCount := make(map[SensitiveType]int)
	highConfCount := 0
	for _, info := range infos {
		typeCount[info.Type]++
		if info.Confidence >= 0.9 {
			highConfCount++
		}
	}

	reasoning += " 包含："
	for t, count := range typeCount {
		label := e.detector.labels[t]
		reasoning += fmt.Sprintf(" %s(%d个)", label, count)
	}

	if highConfCount > 0 {
		reasoning += fmt.Sprintf("。其中 %d 个为高置信度检测。", highConfCount)
	}

	return reasoning
}

// generateRecommendations 生成建议
func (e *SmartDecisionEngine) generateRecommendations(ctx DecisionContext,
	decision *Decision) []string {

	recommendations := []string{}

	if decision.ShouldBlock {
		recommendations = append(recommendations,
			"建议阻止此消息的存储和传输，并通知用户移除敏感信息")
	}

	if decision.ShouldEncrypt {
		recommendations = append(recommendations,
			"建议使用AES-256-GCM加密存储敏感内容")
	}

	if decision.ShouldAlert {
		recommendations = append(recommendations,
			"建议立即通知安全团队进行审查")
	}

	if contains(decision.Actions, ActionManualReview) {
		recommendations = append(recommendations,
			"建议人工审核以确认检测结果的准确性")
	}

	// 基于敏感信息类型的建议
	for _, info := range ctx.SensitiveInfos {
		switch info.Type {
		case SensitiveTypePrivateKey:
			recommendations = append(recommendations,
				"检测到私钥，建议立即轮换密钥并检查是否已泄露")
		case SensitiveTypeAWSKey:
			recommendations = append(recommendations,
				"检测到AWS凭证，建议立即在AWS控制台禁用该密钥")
		case SensitiveTypePassword:
			recommendations = append(recommendations,
				"检测到密码，建议提醒用户不要在对话中分享密码")
		}
	}

	return recommendations
}

// logDecision 记录决策日志
func (e *SmartDecisionEngine) logDecision(ctx DecisionContext, decision *Decision) {
	if e.auditLogger == nil {
		return
	}

	level := AuditLevelInfo
	if decision.ShouldBlock {
		level = AuditLevelCritical
	} else if decision.ShouldAlert {
		level = AuditLevelWarning
	}

	// 转换 SensitiveInfo 为 DetectionResult
	detections := make([]DetectionResult, len(ctx.SensitiveInfos))
	for i, info := range ctx.SensitiveInfos {
		detections[i] = DetectionResult{
			Type:       info.Type,
			Value:      info.Value,
			Start:      info.Start,
			End:        info.End,
			Confidence: info.Confidence,
		}
	}

	e.auditLogger.Log(AuditEvent{
		ID:            fmt.Sprintf("decision-%d", time.Now().UnixNano()),
		Timestamp:     time.Now(),
		Level:         level,
		EventType:     "smart_decision",
		UserID:        ctx.UserID,
		SessionID:     ctx.SessionID,
		Action:        decision.Actions,
		RiskLevel:     decision.RiskLevel,
		Detections:    detections,
		Message:       decision.Reasoning,
		Metadata: map[string]interface{}{
			"risk_score":     decision.RiskScore,
			"should_block":   decision.ShouldBlock,
			"should_encrypt": decision.ShouldEncrypt,
			"should_alert":   decision.ShouldAlert,
		},
	})
}

// contains 检查切片是否包含元素
func contains(slice []Action, item Action) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
