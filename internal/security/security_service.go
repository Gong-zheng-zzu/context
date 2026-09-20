package security

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/logger"
)

// SecurityService 安全服务
type SecurityService struct {
	detector           *Detector
	multiLayerDetector *MultiLayerDetector                // 🆕 多层检测器
	asdfFramework      *AdversarialSampleDefenseFramework // 🆕 ASDF对抗样本防御框架
	policyManager      *PolicyManager
	complianceEngine   *ComplianceEngine
	auditLogger        *AuditLogger
	encryptor          *Encryptor
	decisionEngine     *SmartDecisionEngine
	mu                 sync.RWMutex
	stats              *SecurityStats
	alertHandlers      []AlertHandler
	useMultiLayer      bool // 🆕 是否使用多层检测
}

// SecurityStats 安全统计
type SecurityStats struct {
	TotalScans          int64
	TotalDetections     int64
	TotalBlocked        int64
	TotalRedacted       int64
	DetectionsByType    map[SensitiveType]int64
	LastScanTime        time.Time
	AverageResponseTime time.Duration
}

// AlertHandler 告警处理器
type AlertHandler func(alert *SecurityAlert)

// SecurityAlert 安全告警
type SecurityAlert struct {
	ID              string
	Timestamp       time.Time
	SessionID       string
	UserID          string
	SensitiveType   SensitiveType
	SecurityLevel   SecurityLevel
	Message         string
	Content         string
	RedactedContent string
	Actions         []Action
	Metadata        map[string]interface{}
}

// ScanResult 扫描结果
type ScanResult struct {
	SessionID        string
	UserID           string
	OriginalContent  string
	RedactedContent  string
	SensitiveInfos   []SensitiveInfo
	MatchedPolicies  []*PolicyRule
	ComplianceReport *ComplianceReport
	Actions          []Action
	Blocked          bool
	ScanTime         time.Duration
	Timestamp        time.Time
	Metadata         map[string]interface{} // 🆕 额外的元数据（如多层检测信息）
}

// MultiLayerSecurityConfiguration is the service-level audit snapshot. It
// explicitly distinguishes an enabled detector from a request that actually
// reached the multi-layer path (which may fall back on an error).
type MultiLayerSecurityConfiguration struct {
	Enabled      bool               `json:"enabled"`
	PCCMEnabled  bool               `json:"pccm_enabled"`
	CASIAEnabled bool               `json:"casia_enabled"`
	EarlyStop    bool               `json:"early_stop"`
	PCCM         PCCMSecurityConfig `json:"pccm_s"`
	CASIA        CASIAConfig        `json:"casia"`
}

// CASIAEmbeddingSimilarityProvider 由 services 层在 init 阶段注册，用于在【显式启用】时
// 向 CASIA 注入一套基于 FastEmbed 的语义相似度实现。
//
// 设计约束：internal/security 不能依赖 internal/services（循环依赖），因此这里只暴露一个
// 注册钩子；具体实现（FastEmbed 客户端）由 services 层提供。provider 返回：
//   - similarity：语义相似度函数；返回 nil 表示"未启用"，此时默认行为完全不变；
//   - threshold ：相似度阈值（∈(0,1]，越界回退到 DefaultCASIASimilarityThreshold）；
//   - model     ：用于审计的模型/实现标识。
type CASIAEmbeddingSimilarityProvider func() (similarity ContextSimilarityFunc, threshold float64, model string)

var (
	casiaEmbeddingProviderMu         sync.RWMutex
	casiaEmbeddingSimilarityProvider CASIAEmbeddingSimilarityProvider
)

// RegisterCASIAEmbeddingSimilarityProvider 注册语义相似度实现提供方（由 services 层调用）。
// 重复注册以最后一次为准；传入 nil 表示注销。
func RegisterCASIAEmbeddingSimilarityProvider(provider CASIAEmbeddingSimilarityProvider) {
	casiaEmbeddingProviderMu.Lock()
	defer casiaEmbeddingProviderMu.Unlock()
	casiaEmbeddingSimilarityProvider = provider
}

// applyCASIAEmbeddingSimilarityProvider 在构造流程中尝试注入可选语义路径。
// 未注册、provider 返回 nil、或算法实例为空时保持关闭，行为与改动前完全一致。
func applyCASIAEmbeddingSimilarityProvider(algorithm *ContextAwareSensitiveInfoAlgorithm) {
	if algorithm == nil {
		return
	}
	casiaEmbeddingProviderMu.RLock()
	provider := casiaEmbeddingSimilarityProvider
	casiaEmbeddingProviderMu.RUnlock()
	if provider == nil {
		return
	}
	similarity, threshold, model := provider()
	if similarity == nil {
		return // 未启用：保持纯 strings.Contains 行为
	}
	algorithm.EnableEmbeddingSimilarity(similarity, threshold, model)
}

// NewSecurityService 创建安全服务
func NewSecurityService(configPath string, auditLogPath string) (*SecurityService, error) {
	detector := NewDetector()
	policyManager := NewPolicyManager(configPath)
	auditLogger, err := NewAuditLogger(auditLogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create audit logger: %w", err)
	}

	secret := strings.TrimSpace(os.Getenv("SECURITY_ENCRYPTION_SECRET"))
	if secret == "" {
		return nil, fmt.Errorf("SECURITY_ENCRYPTION_SECRET is not configured")
	}

	encryptor, err := NewEncryptor(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	complianceEngine := NewComplianceEngine(detector, policyManager, auditLogger)

	// 创建智能决策引擎
	decisionEngine := NewSmartDecisionEngine(detector, policyManager, complianceEngine, auditLogger)

	// 🆕 创建多层检测器
	// 优先从环境变量读取，支持 Docker 环境
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "qwen2.5:3b" // 默认值与 docker-compose.yml 一致
	}
	multiLayerDetector := NewMultiLayerDetector(ollamaHost, ollamaModel)
	multiLayerDetector.dictMatcher.LoadDefaultDictionary()
	configureMultiLayerAlgorithms(
		multiLayerDetector,
		getSecurityEnvAsBool("SECURITY_ENABLE_PCCM", true),
		getSecurityEnvAsBool("SECURITY_ENABLE_CASIA", true),
	)

	// 可选：若 services 层已注册提供方且环境变量显式启用，则注入 FastEmbed 语义
	// 相似度实现；否则保持默认（纯 strings.Contains）行为，与改动前完全一致。
	applyCASIAEmbeddingSimilarityProvider(multiLayerDetector.casiaAlgorithm)

	// 🆕 创建ASDF对抗样本防御框架
	asdfFramework := NewAdversarialSampleDefenseFramework()

	return &SecurityService{
		detector:           detector,
		multiLayerDetector: multiLayerDetector,
		asdfFramework:      asdfFramework,
		policyManager:      policyManager,
		complianceEngine:   complianceEngine,
		auditLogger:        auditLogger,
		encryptor:          encryptor,
		decisionEngine:     decisionEngine,
		stats: &SecurityStats{
			DetectionsByType: make(map[SensitiveType]int64),
		},
		alertHandlers: make([]AlertHandler, 0),
		// The protected input path must use the same configured detector stack as
		// the competition console. Operators can still explicitly disable it.
		useMultiLayer: getSecurityEnvAsBool("SECURITY_ENABLE_MULTI_LAYER", true),
	}, nil
}

func getSecurityEnvAsBool(key string, defaultValue bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func configureMultiLayerAlgorithms(detector *MultiLayerDetector, pccmEnabled, casiaEnabled bool) {
	if detector == nil {
		return
	}
	if pccmEnabled {
		detector.EnablePCCM()
	} else {
		detector.DisablePCCM()
	}
	if casiaEnabled {
		detector.EnableCASIA()
	} else {
		detector.DisableCASIA()
	}
}

// RegisterAlertHandler 注册告警处理器
func (s *SecurityService) RegisterAlertHandler(handler AlertHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertHandlers = append(s.alertHandlers, handler)
}

// DefendAndNormalize 使用ASDF框架进行对抗样本防御和归一化
func (s *SecurityService) DefendAndNormalize(text string) (
	normalizedText string,
	isAdversarial bool,
	attackTypes []string,
	confidence float64,
) {
	if s.asdfFramework == nil {
		// 如果ASDF框架未初始化，返回原文本
		return text, false, []string{}, 0.0
	}

	return s.asdfFramework.DefendAndNormalize(text)
}

// DefendAndNormalizeWithAudit exposes metadata-only ASDF evidence while the
// normalized plaintext remains available only to the server-side pipeline.
func (s *SecurityService) DefendAndNormalizeWithAudit(text string) ASDFAuditResult {
	if s == nil || s.asdfFramework == nil {
		return ASDFAuditResult{
			NormalizedText: text, PipelineVersion: ASDFPipelineVersion,
			OriginalSHA256: asdfTextHash(text), NormalizedSHA256: asdfTextHash(text),
			AttackTypes: []string{}, NormalizationSteps: []ASDFNormalizationStep{},
			ResidualAttackTypes: []string{}, RedetectionPerformed: false,
		}
	}
	return s.asdfFramework.DefendAndNormalizeWithAudit(text)
}

// ScanContent 扫描内容
func (s *SecurityService) ScanContent(ctx context.Context, sessionID, userID, content string) (*ScanResult, error) {
	startTime := time.Now()

	result := &ScanResult{
		SessionID:       sessionID,
		UserID:          userID,
		OriginalContent: content,
		Timestamp:       startTime,
		Actions:         make([]Action, 0),
	}

	// 1. 检测敏感信息
	// Always run the normalized detector. The multi-layer detector may not
	// recognize identifiers whose digits were separated by spaces or symbols.
	_, normalizedInfos := s.detector.DetectAndRedact(content)
	var sensitiveInfos []SensitiveInfo

	// 🆕 如果启用了多层检测，使用多层检测器
	if s.useMultiLayer && s.multiLayerDetector != nil {
		fusionResult, err := s.multiLayerDetector.Detect(ctx, content)
		if err != nil {
			// 如果多层检测失败，回退到单层检测
			logger.Infof("多层检测失败，回退到单层检测: %v", err)
			sensitiveInfos = s.detector.Detect(content)
			result.Metadata = map[string]interface{}{
				"multi_layer_used":     false,
				"multi_layer_fallback": "single_layer",
				"multi_layer_error":    err.Error(),
				"configuration":        s.GetMultiLayerConfiguration(),
			}
		} else {
			// 转换多层检测结果为SensitiveInfo格式
			sensitiveInfos = make([]SensitiveInfo, 0, len(fusionResult.FinalItems))
			for _, item := range fusionResult.FinalItems {
				sensitiveInfos = append(sensitiveInfos, SensitiveInfo{
					Type:       item.Type,
					Value:      item.Value,
					Start:      item.Start,
					End:        item.End,
					Label:      item.Label,
					Position:   item.Position,
					Length:     item.Length,
					Confidence: item.Confidence,
					Encrypted:  item.Encrypted,
					CASIA:      item.CASIA,
				})
			}
			// 记录多层检测的额外信息
			result.Metadata = map[string]interface{}{
				"multi_layer_used":   true,
				"layers_used":        fusionResult.LayersUsed,
				"final_confidence":   fusionResult.FinalConfidence,
				"conflicts_resolved": fusionResult.ConflictsResolved,
				"total_time":         fusionResult.TotalTime.String(),
				"configuration":      s.multiLayerDetector.GetConfiguration(),
			}
		}
	} else {
		// 使用单层检测
		sensitiveInfos = s.detector.Detect(content)
	}

	// Preserve normalized matches so obfuscated phone and ID-card numbers
	// cannot bypass policy matching when another detector misses them.
	for _, normalizedInfo := range normalizedInfos {
		found := false
		for _, info := range sensitiveInfos {
			if info.Type == normalizedInfo.Type && info.Value == normalizedInfo.Value {
				found = true
				break
			}
		}
		if !found {
			sensitiveInfos = append(sensitiveInfos, normalizedInfo)
		}
	}

	result.SensitiveInfos = sensitiveInfos

	// 更新统计
	s.updateStats(sensitiveInfos)

	// 如果没有检测到敏感信息，直接返回
	if len(sensitiveInfos) == 0 {
		result.RedactedContent = content
		result.ScanTime = time.Since(startTime)
		return result, nil
	}

	// 2. 匹配策略
	sensitiveTypes := make([]SensitiveType, 0, len(sensitiveInfos))
	for _, info := range sensitiveInfos {
		sensitiveTypes = append(sensitiveTypes, info.Type)
	}
	matchedPolicies := s.policyManager.MatchPolicies(sensitiveTypes)
	result.MatchedPolicies = matchedPolicies

	// 3. 收集所有需要执行的动作
	actionSet := make(map[Action]bool)
	highestLevel := SecurityLevelLow

	for _, policy := range matchedPolicies {
		for _, action := range policy.Actions {
			actionSet[action] = true
		}
		if policy.SecurityLevel == SecurityLevelHigh {
			highestLevel = SecurityLevelHigh
		} else if policy.SecurityLevel == SecurityLevelMedium && highestLevel != SecurityLevelHigh {
			highestLevel = SecurityLevelMedium
		}
	}

	for action := range actionSet {
		result.Actions = append(result.Actions, action)
	}

	// 4. 执行动作
	redactedContent := content

	// 执行脱敏
	if actionSet[ActionRedact] {
		redactedContent, _ = s.detector.DetectAndRedact(content)
		result.RedactedContent = redactedContent
		s.incrementRedactedCount()
	} else {
		result.RedactedContent = content
	}

	// 执行日志记录
	if actionSet[ActionLog] {
		s.logDetection(sessionID, userID, sensitiveInfos, matchedPolicies)
	}

	// 执行告警
	if actionSet[ActionAlert] {
		s.sendAlerts(sessionID, userID, sensitiveInfos, highestLevel, content, redactedContent)
	}

	// 执行阻止
	if actionSet[ActionBlock] {
		result.Blocked = true
		s.incrementBlockedCount()
	}

	// 5. 合规检查
	complianceCtx := &ComplianceContext{
		SessionID:       sessionID,
		UserID:          userID,
		Content:         content,
		SensitiveInfos:  sensitiveInfos,
		DetectedTypes:   sensitiveTypes,
		RedactedContent: redactedContent,
		Timestamp:       startTime,
		Metadata: map[string]interface{}{
			"audit_logged": actionSet[ActionLog],
		},
	}
	complianceReport := s.complianceEngine.CheckCompliance(complianceCtx)
	result.ComplianceReport = complianceReport

	result.ScanTime = time.Since(startTime)
	return result, nil
}

// GetMultiLayerConfiguration returns the effective service configuration for
// API audit output. It is safe to call while scans are running.
func (s *SecurityService) GetMultiLayerConfiguration() MultiLayerSecurityConfiguration {
	if s == nil {
		return MultiLayerSecurityConfiguration{}
	}
	s.mu.RLock()
	enabled := s.useMultiLayer
	s.mu.RUnlock()
	configuration := MultiLayerSecurityConfiguration{Enabled: enabled}
	if s.multiLayerDetector != nil {
		layerConfiguration := s.multiLayerDetector.GetConfiguration()
		configuration.PCCMEnabled = layerConfiguration.PCCMEnabled
		configuration.CASIAEnabled = layerConfiguration.CASIAEnabled
		configuration.EarlyStop = layerConfiguration.EarlyStop
		configuration.PCCM = layerConfiguration.PCCM
		configuration.CASIA = layerConfiguration.CASIA
	}
	return configuration
}

// updateStats 更新统计信息
func (s *SecurityService) updateStats(sensitiveInfos []SensitiveInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalScans++
	s.stats.LastScanTime = time.Now()

	if len(sensitiveInfos) > 0 {
		s.stats.TotalDetections++
		for _, info := range sensitiveInfos {
			s.stats.DetectionsByType[info.Type]++
		}
	}
}

// incrementBlockedCount 增加阻止计数
func (s *SecurityService) incrementBlockedCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.TotalBlocked++
}

// incrementRedactedCount 增加脱敏计数
func (s *SecurityService) incrementRedactedCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.TotalRedacted++
}

// logDetection 记录检测日志
func (s *SecurityService) logDetection(sessionID, userID string, sensitiveInfos []SensitiveInfo, policies []*PolicyRule) {
	for _, info := range sensitiveInfos {
		event := AuditEvent{
			ID:        fmt.Sprintf("detection-%d", time.Now().UnixNano()),
			EventType: string(EventTypeSensitiveDetected),
			SessionID: sessionID,
			UserID:    userID,
			Timestamp: time.Now(),
			Level:     AuditLevelInfo,
			Metadata: map[string]interface{}{
				"sensitive_type": info.Type,
				"position":       info.Position,
				"length":         info.Length,
				"confidence":     info.Confidence,
			},
		}
		s.auditLogger.Log(event)
	}
}

// sendAlerts 发送告警
func (s *SecurityService) sendAlerts(sessionID, userID string, sensitiveInfos []SensitiveInfo, level SecurityLevel, content, redactedContent string) {
	s.mu.RLock()
	handlers := make([]AlertHandler, len(s.alertHandlers))
	copy(handlers, s.alertHandlers)
	s.mu.RUnlock()

	for _, info := range sensitiveInfos {
		alert := &SecurityAlert{
			ID:              fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			Timestamp:       time.Now(),
			SessionID:       sessionID,
			UserID:          userID,
			SensitiveType:   info.Type,
			SecurityLevel:   level,
			Message:         fmt.Sprintf("检测到敏感信息: %s", info.Type),
			Content:         content,
			RedactedContent: redactedContent,
			Metadata: map[string]interface{}{
				"position":   info.Position,
				"length":     info.Length,
				"confidence": info.Confidence,
			},
		}

		// 记录告警日志
		event := AuditEvent{
			ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
			EventType: string(EventTypeAlert),
			SessionID: sessionID,
			UserID:    userID,
			Timestamp: time.Now(),
			Level:     AuditLevelWarning,
			Metadata: map[string]interface{}{
				"alert_id":       alert.ID,
				"sensitive_type": info.Type,
				"security_level": level,
			},
		}
		s.auditLogger.Log(event)

		// 调用告警处理器
		for _, handler := range handlers {
			go handler(alert)
		}
	}
}

// GetStats 获取统计信息
func (s *SecurityService) GetStats() *SecurityStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 创建副本
	stats := &SecurityStats{
		TotalScans:          s.stats.TotalScans,
		TotalDetections:     s.stats.TotalDetections,
		TotalBlocked:        s.stats.TotalBlocked,
		TotalRedacted:       s.stats.TotalRedacted,
		LastScanTime:        s.stats.LastScanTime,
		AverageResponseTime: s.stats.AverageResponseTime,
		DetectionsByType:    make(map[SensitiveType]int64),
	}

	for k, v := range s.stats.DetectionsByType {
		stats.DetectionsByType[k] = v
	}

	return stats
}

// GetPolicyManager 获取策略管理器
func (s *SecurityService) GetPolicyManager() *PolicyManager {
	return s.policyManager
}

// GetComplianceEngine 获取合规引擎
func (s *SecurityService) GetComplianceEngine() *ComplianceEngine {
	return s.complianceEngine
}

// GetAuditLogger 获取审计日志器
func (s *SecurityService) GetAuditLogger() *AuditLogger {
	return s.auditLogger
}

// Close 关闭服务
func (s *SecurityService) Close() error {
	return s.auditLogger.Close()
}

// ScanContentWithSmartDecision 使用智能决策引擎扫描内容
func (s *SecurityService) ScanContentWithSmartDecision(ctx context.Context, sessionID, userID, content string, complianceRules []string) (*Decision, error) {
	// 构建决策上下文
	decisionCtx := DecisionContext{
		UserID:          userID,
		SessionID:       sessionID,
		MessageContent:  content,
		ComplianceRules: complianceRules,
		Metadata:        make(map[string]interface{}),
	}

	// 使用智能决策引擎做出决策
	decision, err := s.decisionEngine.MakeDecision(decisionCtx)
	if err != nil {
		return nil, fmt.Errorf("decision engine failed: %w", err)
	}

	// 更新统计信息
	s.updateStatsFromDecision(decision)

	// 如果需要加密，执行加密
	if decision.ShouldEncrypt && len(decision.RedactedContent) > 0 {
		encrypted, err := s.encryptor.Encrypt(decision.RedactedContent)
		if err == nil {
			decision.Metadata["encrypted_content"] = encrypted
		}
	}

	return decision, nil
}

// updateStatsFromDecision 从决策结果更新统计信息
func (s *SecurityService) updateStatsFromDecision(decision *Decision) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalScans++
	s.stats.LastScanTime = time.Now()

	if len(decision.Metadata) > 0 {
		if sensitiveInfos, ok := decision.Metadata["sensitive_infos"].([]SensitiveInfo); ok {
			if len(sensitiveInfos) > 0 {
				s.stats.TotalDetections++
				for _, info := range sensitiveInfos {
					s.stats.DetectionsByType[info.Type]++
				}
			}
		}
	}

	if decision.ShouldBlock {
		s.stats.TotalBlocked++
	}

	if containsAction(decision.Actions, ActionRedact) {
		s.stats.TotalRedacted++
	}
}

// containsAction 检查动作切片是否包含指定动作
func containsAction(slice []Action, item Action) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetDecisionEngine 获取智能决策引擎
func (s *SecurityService) GetDecisionEngine() *SmartDecisionEngine {
	return s.decisionEngine
}

// 🆕 ScanContentWithMultiLayer 使用多层检测器扫描内容
func (s *SecurityService) ScanContentWithMultiLayer(ctx context.Context, sessionID, userID, content string) (*FusionResult, error) {
	if !s.useMultiLayer {
		// 如果未启用多层检测，使用原有方法
		return nil, fmt.Errorf("multi-layer detection is not enabled")
	}

	// 使用多层检测器
	result, err := s.multiLayerDetector.Detect(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("multi-layer detection failed: %w", err)
	}

	// 记录审计日志
	for _, item := range result.FinalItems {
		event := AuditEvent{
			ID:        fmt.Sprintf("multi-layer-detection-%d", time.Now().UnixNano()),
			EventType: string(EventTypeSensitiveDetected),
			SessionID: sessionID,
			UserID:    userID,
			Timestamp: time.Now(),
			Level:     AuditLevelInfo,
			Metadata: map[string]interface{}{
				"sensitive_type": item.Type,
				"position":       item.Position,
				"length":         item.Length,
				"confidence":     item.Confidence,
				"layers_used":    result.LayersUsed,
				"total_time":     result.TotalTime.String(),
			},
		}
		s.auditLogger.Log(event)
	}

	// 更新统计
	s.updateStatsFromMultiLayer(result)

	return result, nil
}

// 🆕 updateStatsFromMultiLayer 从多层检测结果更新统计
func (s *SecurityService) updateStatsFromMultiLayer(result *FusionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalScans++
	s.stats.LastScanTime = time.Now()

	if len(result.FinalItems) > 0 {
		s.stats.TotalDetections++
		for _, item := range result.FinalItems {
			s.stats.DetectionsByType[item.Type]++
		}
	}

	// 更新平均响应时间
	if s.stats.TotalScans == 1 {
		s.stats.AverageResponseTime = result.TotalTime
	} else {
		s.stats.AverageResponseTime = (s.stats.AverageResponseTime*time.Duration(s.stats.TotalScans-1) + result.TotalTime) / time.Duration(s.stats.TotalScans)
	}
}

// 🆕 EnableMultiLayerDetection 启用多层检测
func (s *SecurityService) EnableMultiLayerDetection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.useMultiLayer = true
}

// 🆕 DisableMultiLayerDetection 禁用多层检测
func (s *SecurityService) DisableMultiLayerDetection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.useMultiLayer = false
}

// 🆕 GetMultiLayerDetector 获取多层检测器
func (s *SecurityService) GetMultiLayerDetector() *MultiLayerDetector {
	return s.multiLayerDetector
}

// 🆕 GetMultiLayerStats 获取多层检测统计
func (s *SecurityService) GetMultiLayerStats() *DetectionStats {
	if s.multiLayerDetector == nil {
		return nil
	}
	return s.multiLayerDetector.GetStats()
}

// GetDetector 获取基础检测器（单例）
func (s *SecurityService) GetDetector() *Detector {
	return s.detector
}
