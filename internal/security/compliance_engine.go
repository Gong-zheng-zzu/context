package security

import (
	"fmt"
	"sync"
	"time"
)

// ComplianceStandard 合规标准
type ComplianceStandard string

const (
	StandardGDPR     ComplianceStandard = "GDPR"      // 欧盟通用数据保护条例
	StandardHIPAA    ComplianceStandard = "HIPAA"     // 美国健康保险流通与责任法案
	StandardPCIDSS   ComplianceStandard = "PCI-DSS"   // 支付卡行业数据安全标准
	StandardSOC2     ComplianceStandard = "SOC2"      // 服务组织控制2
	StandardISO27001 ComplianceStandard = "ISO27001"  // 信息安全管理体系
	StandardCCPA     ComplianceStandard = "CCPA"      // 加州消费者隐私法案
	StandardCustom   ComplianceStandard = "CUSTOM"    // 自定义标准
)

// ComplianceRule 合规规则
type ComplianceRule struct {
	ID          string             `json:"id"`
	Standard    ComplianceStandard `json:"standard"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Requirement string             `json:"requirement"`
	CheckFunc   func(ctx *ComplianceContext) (bool, string)
	Severity    string `json:"severity"` // critical, high, medium, low
	Enabled     bool   `json:"enabled"`
}

// ComplianceContext 合规检查上下文
type ComplianceContext struct {
	SessionID       string
	UserID          string
	Content         string
	SensitiveInfos  []SensitiveInfo
	Metadata        map[string]interface{}
	Timestamp       time.Time
	DetectedTypes   []SensitiveType
	RedactedContent string
}

// ComplianceResult 合规检查结果
type ComplianceResult struct {
	RuleID      string             `json:"rule_id"`
	Standard    ComplianceStandard `json:"standard"`
	RuleName    string             `json:"rule_name"`
	Passed      bool               `json:"passed"`
	Message     string             `json:"message"`
	Severity    string             `json:"severity"`
	Timestamp   time.Time          `json:"timestamp"`
	Remediation string             `json:"remediation,omitempty"`
}

// ComplianceReport 合规报告
type ComplianceReport struct {
	SessionID      string              `json:"session_id"`
	UserID         string              `json:"user_id"`
	Timestamp      time.Time           `json:"timestamp"`
	Results        []ComplianceResult  `json:"results"`
	OverallStatus  string              `json:"overall_status"` // compliant, non_compliant, warning
	PassedCount    int                 `json:"passed_count"`
	FailedCount    int                 `json:"failed_count"`
	TotalChecks    int                 `json:"total_checks"`
	Standards      []ComplianceStandard `json:"standards"`
	Recommendations []string           `json:"recommendations,omitempty"`
}

// ComplianceEngine 合规引擎
type ComplianceEngine struct {
	mu              sync.RWMutex
	rules           map[string]*ComplianceRule
	detector        *Detector
	policyManager   *PolicyManager
	auditLogger     *AuditLogger
	enabledStandards map[ComplianceStandard]bool
}

// NewComplianceEngine 创建合规引擎
func NewComplianceEngine(detector *Detector, policyManager *PolicyManager, auditLogger *AuditLogger) *ComplianceEngine {
	ce := &ComplianceEngine{
		rules:            make(map[string]*ComplianceRule),
		detector:         detector,
		policyManager:    policyManager,
		auditLogger:      auditLogger,
		enabledStandards: make(map[ComplianceStandard]bool),
	}
	ce.initDefaultRules()
	return ce
}

// initDefaultRules 初始化默认合规规则
func (ce *ComplianceEngine) initDefaultRules() {
	// GDPR 规则
	ce.AddRule(&ComplianceRule{
		ID:          "gdpr-001",
		Standard:    StandardGDPR,
		Name:        "个人数据保护",
		Description: "确保个人数据得到适当保护",
		Requirement: "GDPR Article 32 - Security of processing",
		Severity:    "critical",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			hasPersonalData := false
			for _, info := range ctx.SensitiveInfos {
				if info.Type == SensitiveTypeEmail || info.Type == SensitiveTypePhone || info.Type == SensitiveTypeSSN {
					hasPersonalData = true
					break
				}
			}
			if hasPersonalData && ctx.RedactedContent == "" {
				return false, "检测到个人数据但未进行脱敏处理"
			}
			return true, "个人数据已得到适当保护"
		},
	})

	// PCI-DSS 规则
	ce.AddRule(&ComplianceRule{
		ID:          "pci-001",
		Standard:    StandardPCIDSS,
		Name:        "支付卡数据保护",
		Description: "确保信用卡信息不被明文存储或传输",
		Requirement: "PCI-DSS Requirement 3.4 - Render PAN unreadable",
		Severity:    "critical",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			for _, info := range ctx.SensitiveInfos {
				if info.Type == SensitiveTypeCreditCard {
					return false, "检测到未加密的信用卡号"
				}
			}
			return true, "未检测到明文信用卡信息"
		},
	})

	// HIPAA 规则
	ce.AddRule(&ComplianceRule{
		ID:          "hipaa-001",
		Standard:    StandardHIPAA,
		Name:        "健康信息保护",
		Description: "确保受保护的健康信息(PHI)得到适当处理",
		Requirement: "HIPAA Security Rule - Access Control",
		Severity:    "high",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			// 检查是否包含健康相关的敏感信息
			hasHealthData := false
			for _, info := range ctx.SensitiveInfos {
				if info.Type == SensitiveTypeSSN {
					hasHealthData = true
					break
				}
			}
			if hasHealthData {
				// 检查是否有适当的访问控制
				if ctx.Metadata["access_controlled"] != true {
					return false, "健康信息未实施访问控制"
				}
			}
			return true, "健康信息访问控制符合要求"
		},
	})

	// SOC2 规则
	ce.AddRule(&ComplianceRule{
		ID:          "soc2-001",
		Standard:    StandardSOC2,
		Name:        "凭证安全管理",
		Description: "确保凭证和密钥得到安全管理",
		Requirement: "SOC2 CC6.1 - Logical and Physical Access Controls",
		Severity:    "high",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			hasCredentials := false
			for _, info := range ctx.SensitiveInfos {
				if info.Type == SensitiveTypeAPIKey || info.Type == SensitiveTypePassword ||
					info.Type == SensitiveTypePrivateKey || info.Type == SensitiveTypeAWSKey {
					hasCredentials = true
					break
				}
			}
			if hasCredentials && ctx.RedactedContent == "" {
				return false, "检测到凭证信息但未进行脱敏"
			}
			return true, "凭证信息已得到适当保护"
		},
	})

	// ISO 27001 规则
	ce.AddRule(&ComplianceRule{
		ID:          "iso27001-001",
		Standard:    StandardISO27001,
		Name:        "信息分类与处理",
		Description: "确保敏感信息按照分类要求进行处理",
		Requirement: "ISO 27001 A.8.2 - Information classification",
		Severity:    "medium",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			if len(ctx.SensitiveInfos) > 0 {
				// 检查是否标记了信息分类
				if ctx.Metadata["classification"] == nil {
					return false, "敏感信息未进行分类标记"
				}
			}
			return true, "信息分类符合要求"
		},
	})

	// 通用安全规则
	ce.AddRule(&ComplianceRule{
		ID:          "general-001",
		Standard:    StandardCustom,
		Name:        "敏感信息审计",
		Description: "确保所有敏感信息访问都被记录",
		Requirement: "Security Best Practice - Audit Logging",
		Severity:    "medium",
		Enabled:     true,
		CheckFunc: func(ctx *ComplianceContext) (bool, string) {
			if len(ctx.SensitiveInfos) > 0 {
				// 检查是否记录了审计日志
				if ctx.Metadata["audit_logged"] != true {
					return false, "敏感信息访问未记录审计日志"
				}
			}
			return true, "审计日志记录完整"
		},
	})
}

// AddRule 添加合规规则
func (ce *ComplianceEngine) AddRule(rule *ComplianceRule) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.rules[rule.ID] = rule
}

// EnableStandard 启用合规标准
func (ce *ComplianceEngine) EnableStandard(standard ComplianceStandard) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.enabledStandards[standard] = true
}

// DisableStandard 禁用合规标准
func (ce *ComplianceEngine) DisableStandard(standard ComplianceStandard) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	delete(ce.enabledStandards, standard)
}

// CheckCompliance 执行合规检查
func (ce *ComplianceEngine) CheckCompliance(ctx *ComplianceContext) *ComplianceReport {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	report := &ComplianceReport{
		SessionID: ctx.SessionID,
		UserID:    ctx.UserID,
		Timestamp: time.Now(),
		Results:   make([]ComplianceResult, 0),
		Standards: make([]ComplianceStandard, 0),
	}

	standardSet := make(map[ComplianceStandard]bool)

	// 执行所有启用的规则
	for _, rule := range ce.rules {
		if !rule.Enabled {
			continue
		}

		// 如果指定了启用的标准，只检查这些标准
		if len(ce.enabledStandards) > 0 && !ce.enabledStandards[rule.Standard] {
			continue
		}

		passed, message := rule.CheckFunc(ctx)
		result := ComplianceResult{
			RuleID:    rule.ID,
			Standard:  rule.Standard,
			RuleName:  rule.Name,
			Passed:    passed,
			Message:   message,
			Severity:  rule.Severity,
			Timestamp: time.Now(),
		}

		if !passed {
			result.Remediation = ce.getRemediation(rule)
		}

		report.Results = append(report.Results, result)
		standardSet[rule.Standard] = true

		if passed {
			report.PassedCount++
		} else {
			report.FailedCount++
		}
	}

	report.TotalChecks = len(report.Results)

	// 收集涉及的标准
	for standard := range standardSet {
		report.Standards = append(report.Standards, standard)
	}

	// 确定整体状态
	if report.FailedCount == 0 {
		report.OverallStatus = "compliant"
	} else {
		hasCritical := false
		for _, result := range report.Results {
			if !result.Passed && result.Severity == "critical" {
				hasCritical = true
				break
			}
		}
		if hasCritical {
			report.OverallStatus = "non_compliant"
		} else {
			report.OverallStatus = "warning"
		}
	}

	// 生成建议
	report.Recommendations = ce.generateRecommendations(report)

	return report
}

// getRemediation 获取修复建议
func (ce *ComplianceEngine) getRemediation(rule *ComplianceRule) string {
	remediations := map[string]string{
		"gdpr-001":      "对个人数据进行脱敏处理，确保符合 GDPR 数据保护要求",
		"pci-001":       "立即删除或加密信用卡信息，使用令牌化技术替代明文存储",
		"hipaa-001":     "实施基于角色的访问控制(RBAC)，确保只有授权人员可以访问健康信息",
		"soc2-001":      "对所有凭证信息进行脱敏，使用密钥管理系统(KMS)存储敏感凭证",
		"iso27001-001":  "为敏感信息添加分类标签(如:机密、内部、公开)，并按分类要求处理",
		"general-001":   "启用审计日志记录，确保所有敏感信息访问都有完整的审计追踪",
	}

	if remediation, exists := remediations[rule.ID]; exists {
		return remediation
	}
	return "请参考相关合规标准要求进行整改"
}

// generateRecommendations 生成整体建议
func (ce *ComplianceEngine) generateRecommendations(report *ComplianceReport) []string {
	var recommendations []string

	if report.FailedCount == 0 {
		recommendations = append(recommendations, "所有合规检查均已通过，请继续保持")
		return recommendations
	}

	// 按严重程度分类失败项
	criticalCount := 0
	highCount := 0
	for _, result := range report.Results {
		if !result.Passed {
			if result.Severity == "critical" {
				criticalCount++
			} else if result.Severity == "high" {
				highCount++
			}
		}
	}

	if criticalCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("发现 %d 个严重合规问题，建议立即处理", criticalCount))
	}

	if highCount > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("发现 %d 个高风险合规问题，建议优先处理", highCount))
	}

	// 针对特定标准的建议
	standardIssues := make(map[ComplianceStandard]int)
	for _, result := range report.Results {
		if !result.Passed {
			standardIssues[result.Standard]++
		}
	}

	for standard, count := range standardIssues {
		recommendations = append(recommendations,
			fmt.Sprintf("%s 标准存在 %d 个不合规项，请参考修复建议进行整改", standard, count))
	}

	return recommendations
}

// GetRules 获取所有规则
func (ce *ComplianceEngine) GetRules() []*ComplianceRule {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	rules := make([]*ComplianceRule, 0, len(ce.rules))
	for _, rule := range ce.rules {
		rules = append(rules, rule)
	}
	return rules
}

// GetRulesByStandard 按标准获取规则
func (ce *ComplianceEngine) GetRulesByStandard(standard ComplianceStandard) []*ComplianceRule {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var rules []*ComplianceRule
	for _, rule := range ce.rules {
		if rule.Standard == standard {
			rules = append(rules, rule)
		}
	}
	return rules
}
