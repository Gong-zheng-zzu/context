package security

// Action 检测到敏感信息后的动作
type Action string

const (
	ActionLog          Action = "log"           // 仅记录日志
	ActionAllow        Action = "allow"         // 允许通过
	ActionRedact       Action = "redact"        // 脱敏处理
	ActionBlock        Action = "block"         // 阻止操作
	ActionAlert        Action = "alert"         // 发送告警
	ActionEncrypt      Action = "encrypt"       // 加密存储
	ActionManualReview Action = "manual_review" // 人工审核
)

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// 为了兼容性，添加别名
const (
	RiskLevelLow      = RiskLow
	RiskLevelMedium   = RiskMedium
	RiskLevelHigh     = RiskHigh
	RiskLevelCritical = RiskCritical
)

// SecurityLevel 安全级别
type SecurityLevel string

const (
	SecurityLevelLow    SecurityLevel = "low"
	SecurityLevelMedium SecurityLevel = "medium"
	SecurityLevelHigh   SecurityLevel = "high"
)

// DetectionResult 检测结果（用于审计日志）
type DetectionResult struct {
	Type       SensitiveType `json:"type"`
	Value      string        `json:"value"`
	Start      int           `json:"start"`
	End        int           `json:"end"`
	Confidence float64       `json:"confidence"`
	Context    string        `json:"context,omitempty"`
}

// AuditLevel 审计级别
type AuditLevel string

const (
	AuditLevelInfo     AuditLevel = "info"
	AuditLevelWarning  AuditLevel = "warning"
	AuditLevelCritical AuditLevel = "critical"
)

// EventType 事件类型
type EventType string

const (
	EventTypeSensitiveDetected EventType = "sensitive_detected"
	EventTypeAlert             EventType = "alert"
	EventTypeDecision          EventType = "decision"
	EventTypeCompliance        EventType = "compliance"
)
