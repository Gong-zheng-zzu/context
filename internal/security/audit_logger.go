package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEvent 审计事件
type AuditEvent struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Level        AuditLevel             `json:"level"`
	SessionID    string                 `json:"session_id"`
	UserID       string                 `json:"user_id"`
	EventType    string                 `json:"event_type"`
	RiskLevel    RiskLevel              `json:"risk_level"`
	Detections   []DetectionResult      `json:"detections"`
	OriginalText string                 `json:"original_text,omitempty"`
	MaskedText   string                 `json:"masked_text,omitempty"`
	Action       []Action               `json:"action"`            // 处理动作列表
	Message      string                 `json:"message,omitempty"` // 日志消息
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// RetrievalAuditEvent 检索决策审计事件（新增）
type RetrievalAuditEvent struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id"`
	UserID      string    `json:"user_id"`
	Query       string    `json:"query"`
	QueryIntent string    `json:"query_intent"` // 查询意图

	// 引擎调用情况
	EnginesInvoked []string `json:"engines_invoked"` // 调用的引擎列表：["timeline", "knowledge", "vector"]
	TimelineHits   int      `json:"timeline_hits"`   // 时间线命中数
	KnowledgeHits  int      `json:"knowledge_hits"`  // 知识图谱命中数
	VectorHits     int      `json:"vector_hits"`     // 向量检索命中数

	// RRF融合参数
	FusionMethod  string             `json:"fusion_method"`  // 融合方法："rrf", "weighted", "simple"
	RRFParameter  float64            `json:"rrf_parameter"`  // RRF参数k
	SourceWeights map[string]float64 `json:"source_weights"` // 来源权重

	// 性能指标
	LatencyMetrics LatencyMetrics `json:"latency_metrics"` // 延迟指标
	TotalResults   int            `json:"total_results"`   // 总结果数
	FinalResults   int            `json:"final_results"`   // 最终返回结果数

	// 结果质量
	AvgRelevance float64 `json:"avg_relevance"` // 平均相关性
	MaxRelevance float64 `json:"max_relevance"` // 最大相关性

	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LatencyMetrics 延迟指标
type LatencyMetrics struct {
	TotalLatency     int64 `json:"total_latency_ms"`     // 总延迟（毫秒）
	TimelineLatency  int64 `json:"timeline_latency_ms"`  // 时间线查询延迟
	KnowledgeLatency int64 `json:"knowledge_latency_ms"` // 知识图谱查询延迟
	VectorLatency    int64 `json:"vector_latency_ms"`    // 向量查询延迟
	FusionLatency    int64 `json:"fusion_latency_ms"`    // 融合处理延迟
}

// AuditLogger 审计日志记录器
type AuditLogger struct {
	logDir           string
	mu               sync.Mutex
	currentFile      *os.File
	currentDate      string
	alertChan        chan AuditEvent
	enabled          bool
	retrievalLogFile *os.File // 检索审计日志文件
}

// NewAuditLogger 创建审计日志记录器
func NewAuditLogger(logDir string) (*AuditLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger := &AuditLogger{
		logDir:    logDir,
		alertChan: make(chan AuditEvent, 100),
		enabled:   true,
	}

	// 启动告警处理协程
	go logger.processAlerts()

	return logger, nil
}

// Log 记录审计事件
func (al *AuditLogger) Log(event AuditEvent) error {
	if !al.enabled {
		return nil
	}

	al.mu.Lock()
	defer al.mu.Unlock()
	if err := attachAuditSignature(&event); err != nil {
		return fmt.Errorf("audit signature: %w", err)
	}

	// 检查是否需要切换日志文件
	today := time.Now().Format("2006-01-02")
	if al.currentDate != today || al.currentFile == nil {
		if err := al.rotateLogFile(today); err != nil {
			return err
		}
	}

	// 序列化事件
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// 写入日志文件
	if _, err := al.currentFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	// 如果是高危事件，发送告警
	if event.RiskLevel == RiskCritical || event.RiskLevel == RiskHigh {
		select {
		case al.alertChan <- event:
		default:
			// 告警通道满了，记录但不阻塞
			fmt.Fprintf(os.Stderr, "Alert channel full, dropping alert for event %s\n", event.ID)
		}
	}

	return nil
}

// rotateLogFile 切换日志文件
func (al *AuditLogger) rotateLogFile(date string) error {
	// 关闭旧文件
	if al.currentFile != nil {
		al.currentFile.Close()
	}

	// 创建新文件
	filename := filepath.Join(al.logDir, fmt.Sprintf("audit_%s.jsonl", date))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	al.currentFile = file
	al.currentDate = date
	return nil
}

// processAlerts 处理告警
func (al *AuditLogger) processAlerts() {
	for event := range al.alertChan {
		// 这里可以集成告警系统，如发送邮件、钉钉、企业微信等
		al.handleAlert(event)
	}
}

// handleAlert 处理单个告警
func (al *AuditLogger) handleAlert(event AuditEvent) {
	// 输出到标准错误
	fmt.Fprintf(os.Stderr, "[SECURITY ALERT] %s - Session: %s, Risk: %s, Detections: %d\n",
		event.Timestamp.Format(time.RFC3339),
		event.SessionID,
		event.RiskLevel,
		len(event.Detections))

	// 这里可以添加更多告警渠道
	// 例如：发送到监控系统、发送邮件、调用webhook等
}

// QueryLogs 查询审计日志
func (al *AuditLogger) QueryLogs(startDate, endDate time.Time, filters map[string]interface{}) ([]AuditEvent, error) {
	events := make([]AuditEvent, 0)

	// 遍历日期范围内的所有日志文件
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		filename := filepath.Join(al.logDir, fmt.Sprintf("audit_%s.jsonl", d.Format("2006-01-02")))

		fileEvents, err := al.readLogFile(filename, filters)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 文件不存在，跳过
			}
			return nil, err
		}

		events = append(events, fileEvents...)
	}

	return events, nil
}

// readLogFile 读取日志文件
func (al *AuditLogger) readLogFile(filename string, filters map[string]interface{}) ([]AuditEvent, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	events := make([]AuditEvent, 0)
	lines := splitLines(data)

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event AuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // 跳过无效行
		}

		// 应用过滤器
		if matchFilters(event, filters) {
			events = append(events, event)
		}
	}

	return events, nil
}

// matchFilters 检查事件是否匹配过滤条件
func matchFilters(event AuditEvent, filters map[string]interface{}) bool {
	if len(filters) == 0 {
		return true
	}

	if sessionID, ok := filters["session_id"].(string); ok {
		if event.SessionID != sessionID {
			return false
		}
	}

	if userID, ok := filters["user_id"].(string); ok {
		if event.UserID != userID {
			return false
		}
	}

	if riskLevel, ok := filters["risk_level"].(RiskLevel); ok {
		if event.RiskLevel != riskLevel {
			return false
		}
	}

	return true
}

// splitLines 分割字节数组为行
func splitLines(data []byte) [][]byte {
	lines := make([][]byte, 0)
	start := 0

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}

	if start < len(data) {
		lines = append(lines, data[start:])
	}

	return lines
}

// GetStatistics 获取统计信息
func (al *AuditLogger) GetStatistics(startDate, endDate time.Time) (map[string]interface{}, error) {
	events, err := al.QueryLogs(startDate, endDate, nil)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_events":  len(events),
		"by_risk_level": make(map[RiskLevel]int),
		"by_type":       make(map[SensitiveType]int),
		"by_action":     make(map[string]int),
	}

	riskStats := stats["by_risk_level"].(map[RiskLevel]int)
	typeStats := stats["by_type"].(map[SensitiveType]int)
	actionStats := stats["by_action"].(map[string]int)

	for _, event := range events {
		riskStats[event.RiskLevel]++
		// 统计每个动作
		for _, action := range event.Action {
			actionStats[string(action)]++
		}

		for _, detection := range event.Detections {
			typeStats[detection.Type]++
		}
	}

	return stats, nil
}

// Close 关闭审计日志记录器
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	close(al.alertChan)

	if al.currentFile != nil {
		al.currentFile.Close()
	}

	if al.retrievalLogFile != nil {
		al.retrievalLogFile.Close()
	}

	return nil
}

// Enable 启用审计日志
func (al *AuditLogger) Enable() {
	al.enabled = true
}

// Disable 禁用审计日志
func (al *AuditLogger) Disable() {
	al.enabled = false
}

// ============================================
// 检索审计日志方法（新增）
// ============================================

// LogRetrieval 记录检索决策审计事件
func (al *AuditLogger) LogRetrieval(event RetrievalAuditEvent) error {
	if !al.enabled {
		return nil
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	// 检查是否需要切换日志文件
	today := time.Now().Format("2006-01-02")
	if al.retrievalLogFile == nil {
		filename := filepath.Join(al.logDir, fmt.Sprintf("retrieval_audit_%s.jsonl", today))
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open retrieval log file: %w", err)
		}
		al.retrievalLogFile = file
	}

	// 序列化事件
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal retrieval event: %w", err)
	}

	// 写入日志文件
	if _, err := al.retrievalLogFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write retrieval log: %w", err)
	}

	return nil
}

// QueryRetrievalLogs 查询检索审计日志
func (al *AuditLogger) QueryRetrievalLogs(startDate, endDate time.Time, filters map[string]interface{}) ([]RetrievalAuditEvent, error) {
	events := make([]RetrievalAuditEvent, 0)

	// 遍历日期范围内的所有日志文件
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		filename := filepath.Join(al.logDir, fmt.Sprintf("retrieval_audit_%s.jsonl", d.Format("2006-01-02")))

		fileEvents, err := al.readRetrievalLogFile(filename, filters)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 文件不存在，跳过
			}
			return nil, err
		}

		events = append(events, fileEvents...)
	}

	return events, nil
}

// readRetrievalLogFile 读取检索审计日志文件
func (al *AuditLogger) readRetrievalLogFile(filename string, filters map[string]interface{}) ([]RetrievalAuditEvent, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	events := make([]RetrievalAuditEvent, 0)
	lines := splitLines(data)

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var event RetrievalAuditEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // 跳过无效行
		}

		// 应用过滤器
		if matchRetrievalFilters(event, filters) {
			events = append(events, event)
		}
	}

	return events, nil
}

// matchRetrievalFilters 检查检索事件是否匹配过滤条件
func matchRetrievalFilters(event RetrievalAuditEvent, filters map[string]interface{}) bool {
	if len(filters) == 0 {
		return true
	}

	if sessionID, ok := filters["session_id"].(string); ok {
		if event.SessionID != sessionID {
			return false
		}
	}

	if userID, ok := filters["user_id"].(string); ok {
		if event.UserID != userID {
			return false
		}
	}

	if fusionMethod, ok := filters["fusion_method"].(string); ok {
		if event.FusionMethod != fusionMethod {
			return false
		}
	}

	return true
}

// GetRetrievalStatistics 获取检索审计统计信息
func (al *AuditLogger) GetRetrievalStatistics(startDate, endDate time.Time) (map[string]interface{}, error) {
	events, err := al.QueryRetrievalLogs(startDate, endDate, nil)
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"total_queries":         len(events),
		"by_fusion_method":      make(map[string]int),
		"by_engines_invoked":    make(map[string]int),
		"avg_total_latency":     float64(0),
		"avg_timeline_latency":  float64(0),
		"avg_knowledge_latency": float64(0),
		"avg_vector_latency":    float64(0),
		"avg_fusion_latency":    float64(0),
		"avg_relevance":         float64(0),
		"total_timeline_hits":   0,
		"total_knowledge_hits":  0,
		"total_vector_hits":     0,
	}

	fusionStats := stats["by_fusion_method"].(map[string]int)
	enginesStats := stats["by_engines_invoked"].(map[string]int)

	var totalLatency, timelineLatency, knowledgeLatency, vectorLatency, fusionLatency int64
	var totalRelevance float64

	for _, event := range events {
		fusionStats[event.FusionMethod]++

		// 统计调用的引擎
		for _, engine := range event.EnginesInvoked {
			enginesStats[engine]++
		}

		// 累计延迟
		totalLatency += event.LatencyMetrics.TotalLatency
		timelineLatency += event.LatencyMetrics.TimelineLatency
		knowledgeLatency += event.LatencyMetrics.KnowledgeLatency
		vectorLatency += event.LatencyMetrics.VectorLatency
		fusionLatency += event.LatencyMetrics.FusionLatency

		// 累计相关性
		totalRelevance += event.AvgRelevance

		// 累计命中数
		stats["total_timeline_hits"] = stats["total_timeline_hits"].(int) + event.TimelineHits
		stats["total_knowledge_hits"] = stats["total_knowledge_hits"].(int) + event.KnowledgeHits
		stats["total_vector_hits"] = stats["total_vector_hits"].(int) + event.VectorHits
	}

	// 计算平均值
	if len(events) > 0 {
		stats["avg_total_latency"] = float64(totalLatency) / float64(len(events))
		stats["avg_timeline_latency"] = float64(timelineLatency) / float64(len(events))
		stats["avg_knowledge_latency"] = float64(knowledgeLatency) / float64(len(events))
		stats["avg_vector_latency"] = float64(vectorLatency) / float64(len(events))
		stats["avg_fusion_latency"] = float64(fusionLatency) / float64(len(events))
		stats["avg_relevance"] = totalRelevance / float64(len(events))
	}

	return stats, nil
}
