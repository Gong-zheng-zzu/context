package api

import (
	"log"
	"net/http"

	"github.com/contextkeeper/service/internal/security"
	"github.com/gin-gonic/gin"
)

// SecurityScanRequest 安全扫描请求
type SecurityScanRequest struct {
	Content   string `json:"content" binding:"required"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
}

// SecurityScanResponse 安全扫描响应
type SecurityScanResponse struct {
	OriginalContent   string                  `json:"original_content"`
	RedactedContent   string                  `json:"redacted_content"`
	SensitiveInfos    []security.SensitiveInfo `json:"sensitive_infos"`
	AvgConfidence     float64                 `json:"avg_confidence"`
	RiskScore         float64                 `json:"risk_score"`
	RiskLevel         string                  `json:"risk_level"`
	Actions           []string                `json:"actions"`
	ShouldBlock       bool                    `json:"should_block"`
	ShouldAlert       bool                    `json:"should_alert"`
	ShouldRedact      bool                    `json:"should_redact"`
}

// handleSecurityScan 处理安全扫描请求
func (h *Handler) HandleSecurityScan(c *gin.Context) {
	var req SecurityScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数",
			"details": err.Error(),
		})
		return
	}

	// 检查安全服务是否已初始化
	if h.securityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "安全服务未初始化",
		})
		return
	}

	// 调用安全服务进行扫描
	result, err := h.securityService.ScanContent(
		c.Request.Context(),
		req.SessionID,
		req.UserID,
		req.Content,
	)

	if err != nil {
		log.Printf("安全扫描失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "安全扫描失败",
			"details": err.Error(),
		})
		return
	}

	// 计算平均置信度
	avgConfidence := 0.0
	if len(result.SensitiveInfos) > 0 {
		totalConfidence := 0.0
		for _, info := range result.SensitiveInfos {
			totalConfidence += info.Confidence
		}
		avgConfidence = totalConfidence / float64(len(result.SensitiveInfos))
	}

	// 计算风险分数和等级
	riskScore := calculateRiskScore(result.SensitiveInfos, avgConfidence)
	riskLevel := getRiskLevel(riskScore)

	// 确定安全动作
	actions := determineActions(avgConfidence, riskLevel, result.SensitiveInfos)

	// 构建响应
	response := SecurityScanResponse{
		OriginalContent: result.OriginalContent,
		RedactedContent: result.RedactedContent,
		SensitiveInfos:  result.SensitiveInfos,
		AvgConfidence:   avgConfidence,
		RiskScore:       riskScore,
		RiskLevel:       riskLevel,
		Actions:         actions,
		ShouldBlock:     result.Blocked,
		ShouldAlert:     containsAction(actions, "ALERT"),
		ShouldRedact:    containsAction(actions, "REDACT"),
	}

	c.JSON(http.StatusOK, response)
}

// handleSecurityDetect 仅检测敏感信息，不脱敏
func (h *Handler) HandleSecurityDetect(c *gin.Context) {
	var req SecurityScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数",
		})
		return
	}

	if h.securityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "安全服务未初始化",
		})
		return
	}

	// 使用安全服务的单例检测器（避免每次创建新Detector导致goroutine泄漏）
	detector := h.securityService.GetDetector()
	infos := detector.Detect(req.Content)

	// 计算平均置信度
	avgConfidence := 0.0
	if len(infos) > 0 {
		totalConfidence := 0.0
		for _, info := range infos {
			totalConfidence += info.Confidence
		}
		avgConfidence = totalConfidence / float64(len(infos))
	}

	c.JSON(http.StatusOK, gin.H{
		"sensitive_infos": infos,
		"count":           len(infos),
		"avg_confidence":  avgConfidence,
	})
}

// handleSecurityRedact 脱敏处理
func (h *Handler) HandleSecurityRedact(c *gin.Context) {
	var req SecurityScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数",
		})
		return
	}

	if h.securityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "安全服务未初始化",
		})
		return
	}

	// 使用检测器进行脱敏
	detector := security.NewDetector()
	redacted, infos := detector.DetectAndRedact(req.Content)

	c.JSON(http.StatusOK, gin.H{
		"original_content": req.Content,
		"redacted_content": redacted,
		"sensitive_infos":  infos,
		"count":            len(infos),
	})
}

// handleSecurityStats 获取安全统计信息
func (h *Handler) HandleSecurityStats(c *gin.Context) {
	if h.securityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "安全服务未初始化",
		})
		return
	}

	stats := h.securityService.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"total_scans":       stats.TotalScans,
		"total_detections":  stats.TotalDetections,
		"total_blocked":     stats.TotalBlocked,
		"total_redacted":    stats.TotalRedacted,
		"detections_by_type": stats.DetectionsByType,
		"last_scan_time":    stats.LastScanTime,
	})
}

// calculateRiskScore 计算风险分数
func calculateRiskScore(sensitiveInfos []security.SensitiveInfo, avgConfidence float64) float64 {
	if len(sensitiveInfos) == 0 {
		return 0
	}

	score := avgConfidence * 100

	// 类型权重
	typeWeights := map[security.SensitiveType]float64{
		security.SensitiveTypeAPIKey:      30,
		security.SensitiveTypePassword:    25,
		security.SensitiveTypeCreditCard:  20,
		security.SensitiveTypePhone:       10,
		security.SensitiveTypeEmail:       10,
		security.SensitiveTypeIPAddress:   5,
		security.SensitiveTypeSecretKey:   30,
		security.SensitiveTypeAWSKey:      30,
		security.SensitiveTypePrivateKey:  35,
		security.SensitiveTypeBearerToken: 25,
		security.SensitiveTypeToken:       20,
		security.SensitiveTypeDatabase:    25,
		security.SensitiveTypeSSN:         20,
	}

	for _, info := range sensitiveInfos {
		if weight, ok := typeWeights[info.Type]; ok {
			score += weight
		} else {
			score += 10
		}
	}

	// 数量因素
	if len(sensitiveInfos) > 3 {
		score += 20
	}

	if score > 100 {
		score = 100
	}

	return score
}

// getRiskLevel 获取风险等级
func getRiskLevel(score float64) string {
	if score >= 80 {
		return "CRITICAL"
	}
	if score >= 60 {
		return "HIGH"
	}
	if score >= 40 {
		return "MEDIUM"
	}
	return "LOW"
}

// determineActions 确定安全动作
func determineActions(confidence float64, riskLevel string, sensitiveInfos []security.SensitiveInfo) []string {
	actions := []string{}

	if riskLevel == "CRITICAL" || (riskLevel == "HIGH" && confidence > 0.7) {
		actions = append(actions, "BLOCK", "ALERT", "REDACT")
	} else if riskLevel == "HIGH" || (riskLevel == "MEDIUM" && confidence > 0.6) {
		actions = append(actions, "ALERT", "REDACT")
	} else if riskLevel == "MEDIUM" || confidence > 0.5 {
		actions = append(actions, "REDACT")
	} else {
		actions = append(actions, "ALLOW")
	}

	return actions
}

// containsAction 检查动作列表是否包含指定动作
func containsAction(actions []string, action string) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}
