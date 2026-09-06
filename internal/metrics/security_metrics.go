package metrics

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// SecurityMetrics 安全监控指标
type SecurityMetrics struct {
	// 敏感信息检测指标
	SensitiveDetectedCount   int64            // 检测到的敏感信息总数
	SensitiveTypeCount       map[string]int64 // 各类型敏感信息数量
	MultiLayerDetectionCount int64            // 多层检测次数

	// JWT认证指标
	JWTAuthSuccessCount int64 // JWT认证成功次数
	JWTAuthFailedCount  int64 // JWT认证失败次数
	JWTForgeryAttempts  int64 // JWT伪造尝试次数

	// 速率限制指标
	RateLimitHitCount     int64            // 触发速率限制次数
	RateLimitByEndpoint   map[string]int64 // 各端点触发速率限制次数
	RateLimitBlockedIPs   map[string]int64 // 被阻止的IP及次数

	// 加密操作指标
	EncryptionOperations int64 // 加密操作次数
	DecryptionOperations int64 // 解密操作次数
	EncryptionErrors     int64 // 加密错误次数

	// AI安全指标
	AIRequestCount          int64 // AI请求总数
	AIOutputFilteredCount   int64 // AI输出被过滤次数
	AIDoSProtectionTriggered int64 // DoS防护触发次数
	AIAdversarialDetected   int64 // 对抗样本检测次数
	AIModelTheftDetected    int64 // 模型窃取检测次数

	// 审计日志指标
	AuditLogCount      int64            // 审计日志总数
	AuditLogByLevel    map[string]int64 // 各级别审计日志数量
	AuditLogByEventType map[string]int64 // 各事件类型审计日志数量

	// 时间统计
	StartTime      time.Time // 服务启动时间
	LastUpdateTime time.Time // 最后更新时间

	mutex sync.RWMutex // 读写锁
}

// NewSecurityMetrics 创建安全监控服务
func NewSecurityMetrics() *SecurityMetrics {
	return &SecurityMetrics{
		SensitiveTypeCount:    make(map[string]int64),
		RateLimitByEndpoint:   make(map[string]int64),
		RateLimitBlockedIPs:   make(map[string]int64),
		AuditLogByLevel:       make(map[string]int64),
		AuditLogByEventType:   make(map[string]int64),
		StartTime:             time.Now(),
		LastUpdateTime:        time.Now(),
	}
}

// RecordSensitiveDetection 记录敏感信息检测
func (m *SecurityMetrics) RecordSensitiveDetection(sensitiveType string, count int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.SensitiveDetectedCount += int64(count)
	m.SensitiveTypeCount[sensitiveType] += int64(count)
	m.LastUpdateTime = time.Now()
}

// RecordMultiLayerDetection 记录多层检测
func (m *SecurityMetrics) RecordMultiLayerDetection() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.MultiLayerDetectionCount++
	m.LastUpdateTime = time.Now()
}

// RecordJWTAuthSuccess 记录JWT认证成功
func (m *SecurityMetrics) RecordJWTAuthSuccess() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.JWTAuthSuccessCount++
	m.LastUpdateTime = time.Now()
}

// RecordJWTAuthFailed 记录JWT认证失败
func (m *SecurityMetrics) RecordJWTAuthFailed() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.JWTAuthFailedCount++
	m.LastUpdateTime = time.Now()
}

// RecordJWTForgeryAttempt 记录JWT伪造尝试
func (m *SecurityMetrics) RecordJWTForgeryAttempt() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.JWTForgeryAttempts++
	m.LastUpdateTime = time.Now()
}

// RecordRateLimitHit 记录速率限制触发
func (m *SecurityMetrics) RecordRateLimitHit(endpoint string, ip string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.RateLimitHitCount++
	m.RateLimitByEndpoint[endpoint]++
	m.RateLimitBlockedIPs[ip]++
	m.LastUpdateTime = time.Now()
}

// RecordEncryption 记录加密操作
func (m *SecurityMetrics) RecordEncryption(success bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.EncryptionOperations++
	if !success {
		m.EncryptionErrors++
	}
	m.LastUpdateTime = time.Now()
}

// RecordDecryption 记录解密操作
func (m *SecurityMetrics) RecordDecryption() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.DecryptionOperations++
	m.LastUpdateTime = time.Now()
}

// RecordAIRequest 记录AI请求
func (m *SecurityMetrics) RecordAIRequest() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AIRequestCount++
	m.LastUpdateTime = time.Now()
}

// RecordAIOutputFiltered 记录AI输出被过滤
func (m *SecurityMetrics) RecordAIOutputFiltered() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AIOutputFilteredCount++
	m.LastUpdateTime = time.Now()
}

// RecordAIDoSProtection 记录DoS防护触发
func (m *SecurityMetrics) RecordAIDoSProtection() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AIDoSProtectionTriggered++
	m.LastUpdateTime = time.Now()
}

// RecordAIAdversarialDetected 记录对抗样本检测
func (m *SecurityMetrics) RecordAIAdversarialDetected() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AIAdversarialDetected++
	m.LastUpdateTime = time.Now()
}

// RecordAIModelTheft 记录模型窃取检测
func (m *SecurityMetrics) RecordAIModelTheft() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AIModelTheftDetected++
	m.LastUpdateTime = time.Now()
}

// RecordAuditLog 记录审计日志
func (m *SecurityMetrics) RecordAuditLog(level string, eventType string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.AuditLogCount++
	m.AuditLogByLevel[level]++
	m.AuditLogByEventType[eventType]++
	m.LastUpdateTime = time.Now()
}

// GetSnapshot 获取指标快照
func (m *SecurityMetrics) GetSnapshot() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	uptime := time.Since(m.StartTime)

	return map[string]interface{}{
		"uptime_seconds": uptime.Seconds(),
		"last_update":    m.LastUpdateTime.Format("2006-01-02 15:04:05"),

		// 敏感信息检测
		"sensitive_detection": map[string]interface{}{
			"total_detected":       m.SensitiveDetectedCount,
			"by_type":              m.SensitiveTypeCount,
			"multi_layer_count":    m.MultiLayerDetectionCount,
		},

		// JWT认证
		"jwt_auth": map[string]interface{}{
			"success_count":     m.JWTAuthSuccessCount,
			"failed_count":      m.JWTAuthFailedCount,
			"forgery_attempts":  m.JWTForgeryAttempts,
			"success_rate":      m.calculateSuccessRate(m.JWTAuthSuccessCount, m.JWTAuthFailedCount),
		},

		// 速率限制
		"rate_limit": map[string]interface{}{
			"total_hits":     m.RateLimitHitCount,
			"by_endpoint":    m.RateLimitByEndpoint,
			"blocked_ips":    m.RateLimitBlockedIPs,
			"unique_blocked": len(m.RateLimitBlockedIPs),
		},

		// 加密操作
		"encryption": map[string]interface{}{
			"encrypt_operations": m.EncryptionOperations,
			"decrypt_operations": m.DecryptionOperations,
			"errors":             m.EncryptionErrors,
			"error_rate":         m.calculateErrorRate(m.EncryptionErrors, m.EncryptionOperations),
		},

		// AI安全
		"ai_security": map[string]interface{}{
			"total_requests":       m.AIRequestCount,
			"output_filtered":      m.AIOutputFilteredCount,
			"dos_triggered":        m.AIDoSProtectionTriggered,
			"adversarial_detected": m.AIAdversarialDetected,
			"theft_detected":       m.AIModelTheftDetected,
			"filter_rate":          m.calculateFilterRate(m.AIOutputFilteredCount, m.AIRequestCount),
		},

		// 审计日志
		"audit": map[string]interface{}{
			"total_logs":   m.AuditLogCount,
			"by_level":     m.AuditLogByLevel,
			"by_event_type": m.AuditLogByEventType,
		},
	}
}

// GetSummary 获取安全摘要
func (m *SecurityMetrics) GetSummary() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	uptime := time.Since(m.StartTime)

	summary := fmt.Sprintf(`
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🔒 安全监控摘要
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
运行时间: %.2f 小时
最后更新: %s

📊 敏感信息检测
  - 检测总数: %d
  - 多层检测: %d
  - 检测类型: %d 种

🔐 JWT认证
  - 成功: %d
  - 失败: %d
  - 伪造尝试: %d
  - 成功率: %.2f%%

⏱️ 速率限制
  - 触发次数: %d
  - 被阻止IP: %d 个

🔒 加密操作
  - 加密: %d
  - 解密: %d
  - 错误: %d

🤖 AI安全
  - AI请求: %d
  - 输出过滤: %d
  - DoS防护: %d
  - 对抗检测: %d
  - 窃取检测: %d

📝 审计日志
  - 总日志数: %d
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`,
		uptime.Hours(),
		m.LastUpdateTime.Format("2006-01-02 15:04:05"),
		m.SensitiveDetectedCount,
		m.MultiLayerDetectionCount,
		len(m.SensitiveTypeCount),
		m.JWTAuthSuccessCount,
		m.JWTAuthFailedCount,
		m.JWTForgeryAttempts,
		m.calculateSuccessRate(m.JWTAuthSuccessCount, m.JWTAuthFailedCount),
		m.RateLimitHitCount,
		len(m.RateLimitBlockedIPs),
		m.EncryptionOperations,
		m.DecryptionOperations,
		m.EncryptionErrors,
		m.AIRequestCount,
		m.AIOutputFilteredCount,
		m.AIDoSProtectionTriggered,
		m.AIAdversarialDetected,
		m.AIModelTheftDetected,
		m.AuditLogCount,
	)

	return summary
}

// PrintSummary 打印安全摘要
func (m *SecurityMetrics) PrintSummary() {
	log.Println(m.GetSummary())
}

// calculateSuccessRate 计算成功率
func (m *SecurityMetrics) calculateSuccessRate(success, failed int64) float64 {
	total := success + failed
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

// calculateErrorRate 计算错误率
func (m *SecurityMetrics) calculateErrorRate(errors, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total) * 100
}

// calculateFilterRate 计算过滤率
func (m *SecurityMetrics) calculateFilterRate(filtered, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(filtered) / float64(total) * 100
}

// Reset 重置所有指标（用于测试）
func (m *SecurityMetrics) Reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.SensitiveDetectedCount = 0
	m.SensitiveTypeCount = make(map[string]int64)
	m.MultiLayerDetectionCount = 0
	m.JWTAuthSuccessCount = 0
	m.JWTAuthFailedCount = 0
	m.JWTForgeryAttempts = 0
	m.RateLimitHitCount = 0
	m.RateLimitByEndpoint = make(map[string]int64)
	m.RateLimitBlockedIPs = make(map[string]int64)
	m.EncryptionOperations = 0
	m.DecryptionOperations = 0
	m.EncryptionErrors = 0
	m.AIRequestCount = 0
	m.AIOutputFilteredCount = 0
	m.AIDoSProtectionTriggered = 0
	m.AIAdversarialDetected = 0
	m.AIModelTheftDetected = 0
	m.AuditLogCount = 0
	m.AuditLogByLevel = make(map[string]int64)
	m.AuditLogByEventType = make(map[string]int64)
	m.StartTime = time.Now()
	m.LastUpdateTime = time.Now()

	log.Println("🔒 [安全监控] 所有指标已重置")
}

// 全局安全监控实例
var GlobalSecurityMetrics = NewSecurityMetrics()
