package models

import (
	"time"
)

// ============================================================================
// LLM驱动服务相关数据模型
// ============================================================================

// LLMDrivenRequest LLM驱动请求基础结构
type LLMDrivenRequest struct {
	RequestID string                 `json:"request_id"`
	SessionID string                 `json:"session_id"`
	UserID    string                 `json:"user_id,omitempty"`
	Query     string                 `json:"query"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// LLMDrivenResponse LLM驱动响应基础结构
type LLMDrivenResponse struct {
	RequestID      string                 `json:"request_id"`
	Success        bool                   `json:"success"`
	Data           interface{}            `json:"data,omitempty"`
	Error          string                 `json:"error,omitempty"`
	ProcessingTime time.Duration          `json:"processing_time"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
}

// ============================================================================
// 语料分析相关模型
// ============================================================================

// SemanticAnalysisRequest 语料分析请求
type SemanticAnalysisRequest struct {
	Query     string `json:"query"`
	SessionID string `json:"session_id"`
	Context   string `json:"context,omitempty"`
}

// SemanticAnalysisResponse 语料分析响应
type SemanticAnalysisResponse struct {
	Intent     string                 `json:"intent"`
	Confidence float64                `json:"confidence"`
	Categories []string               `json:"categories"`
	Keywords   []string               `json:"keywords"`
	Entities   []Entity               `json:"entities"`
	Queries    *MultiDimensionalQuery `json:"queries"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// 注意：Entity现在使用unified_models.go中的统一定义
// 这里不再重复定义，请导入并使用统一模型

// IntentType 意图类型枚举
type IntentType string

const (
	// === 信息获取类 ===
	IntentQuery       IntentType = "query"       // 查询询问（如何做、是什么）
	IntentExplanation IntentType = "explanation" // 解释说明（为什么、原理）
	IntentComparison  IntentType = "comparison"  // 对比分析（A和B区别、哪个更好）
	IntentLearning    IntentType = "learning"    // 学习理解（教我、讲解）

	// === 执行操作类 ===
	IntentCommand      IntentType = "command"      // 执行指令（帮我做、请执行）
	IntentCreation     IntentType = "creation"     // 创建新内容（写代码、生成配置）
	IntentModification IntentType = "modification" // 修改优化（改进代码、调整配置）
	IntentReview       IntentType = "review"       // 审查评估（代码review、方案评审）

	// === 问题解决类 ===
	IntentTroubleshooting IntentType = "troubleshooting" // 问题求助（为什么错、怎么解决）
	IntentDebug           IntentType = "debug"           // 调试排查（定位问题、分析日志）

	// === 规划设计类 ===
	IntentPlanning IntentType = "planning" // 规划设计（系统设计、技术选型）
	IntentAnalysis IntentType = "analysis" // 分析评估（性能分析、代码分析）

	// === 交流沟通类 ===
	IntentConversation IntentType = "conversation" // 对话交流（讨论、闲聊）
	IntentConfirmation IntentType = "confirmation" // 确认反馈（确认、反馈）

	// === 其他 ===
	IntentOther IntentType = "other" // 其他类型
)

// ============================================================================
// 多维度检索相关模型
// ============================================================================

// MultiDimensionalQuery 多维度查询
type MultiDimensionalQuery struct {
	ContextQueries   []string `json:"context_queries"`
	TimelineQueries  []string `json:"timeline_queries"`
	KnowledgeQueries []string `json:"knowledge_queries"`
	VectorQueries    []string `json:"vector_queries"`

	// 🔥 新增：用户和工作空间信息
	UserID      string `json:"user_id,omitempty"`      // 用户ID
	SessionID   string `json:"session_id,omitempty"`   // 会话ID（用于时间线检索隔离）
	WorkspaceID string `json:"workspace_id,omitempty"` // 工作空间ID

	// 🔥 新增：LLM分析结果，用于传递关键概念
	KeyConcepts []string `json:"key_concepts,omitempty"` // LLM分析的关键概念
}

// MultiDimensionalRetrievalRequest 多维度检索请求
type MultiDimensionalRetrievalRequest struct {
	Queries   *MultiDimensionalQuery `json:"queries"`
	SessionID string                 `json:"session_id"`
	Limit     int                    `json:"limit"`
	Strategy  string                 `json:"strategy"`
}

// MultiDimensionalRetrievalResponse 多维度检索响应
type MultiDimensionalRetrievalResponse struct {
	ContextResults   []RetrievalResult `json:"context_results"`
	TimelineResults  []RetrievalResult `json:"timeline_results"`
	KnowledgeResults []RetrievalResult `json:"knowledge_results"`
	VectorResults    []RetrievalResult `json:"vector_results"`
	TotalResults     int               `json:"total_results"`
	ProcessingTime   time.Duration     `json:"processing_time"`
	Sources          []string          `json:"sources"`
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Score     float64                `json:"score"`
	Source    string                 `json:"source"`
	Type      string                 `json:"type"`
	Metadata  map[string]interface{} `json:"metadata"`
	Timestamp time.Time              `json:"timestamp"`
}

// RetrievalStrategy 检索策略
type RetrievalStrategy struct {
	Name       string                 `json:"name"`
	Priorities map[string]float64     `json:"priorities"`
	Parallel   bool                   `json:"parallel"`
	MaxResults int                    `json:"max_results"`
	Timeout    time.Duration          `json:"timeout"`
	Parameters map[string]interface{} `json:"parameters"`
}

// ============================================================================
// 内容合成相关模型
// ============================================================================

// ContentSynthesisRequest 内容合成请求
type ContentSynthesisRequest struct {
	OriginalQuery    string                             `json:"original_query"`
	AnalysisResult   *SemanticAnalysisResponse          `json:"analysis_result"`
	RetrievalResults *MultiDimensionalRetrievalResponse `json:"retrieval_results"`
	SessionID        string                             `json:"session_id"`
	SynthesisType    string                             `json:"synthesis_type"`
}

// ContentSynthesisResponse 内容合成响应
type ContentSynthesisResponse struct {
	SynthesizedContent string                 `json:"synthesized_content"`
	ContextUpdates     *ContextUpdates        `json:"context_updates"`
	Confidence         float64                `json:"confidence"`
	Sources            []string               `json:"sources"`
	Reasoning          string                 `json:"reasoning"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// ContextUpdates 上下文更新信息
type ContextUpdates struct {
	UpdateType     string                 `json:"update_type"`
	Updates        map[string]interface{} `json:"updates"`
	Reason         string                 `json:"reason"`
	Confidence     float64                `json:"confidence"`
	AffectedLayers []string               `json:"affected_layers"`
}

// SynthesisType 合成类型
type SynthesisType string

const (
	SynthesisAnswer         SynthesisType = "answer"         // 直接回答
	SynthesisSummary        SynthesisType = "summary"        // 总结
	SynthesisAnalysis       SynthesisType = "analysis"       // 分析
	SynthesisRecommendation SynthesisType = "recommendation" // 推荐
	SynthesisExplanation    SynthesisType = "explanation"    // 解释
)

// ============================================================================
// 上下文模型相关
// ============================================================================

// LLMDrivenContextModel LLM驱动的上下文模型
type LLMDrivenContextModel struct {
	// 元数据
	ContextID    string    `json:"context_id"`
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastAccessed time.Time `json:"last_accessed"`

	// 核心语境信息
	Core *CoreContext `json:"core"`

	// 业务维度上下文（引用模式）
	Dimensions *ContextDimensions `json:"dimensions"`

	// 变更追踪
	ChangeTracking *ContextChangeTracking `json:"change_tracking"`

	// 元数据
	Metadata map[string]interface{} `json:"metadata"`
}

// CoreContext 核心上下文
type CoreContext struct {
	ConversationThread string     `json:"conversation_thread"`
	CurrentFocus       string     `json:"current_focus"`
	IntentCategory     IntentType `json:"intent_category"`
	Complexity         string     `json:"complexity"`
	Priority           string     `json:"priority"`
}

// ContextDimensions 上下文维度（引用模式）
type ContextDimensions struct {
	TechnicalRef  string `json:"technical_ref"`  // 技术上下文引用ID
	ProblemRef    string `json:"problem_ref"`    // 问题上下文引用ID
	HistoricalRef string `json:"historical_ref"` // 历史上下文引用ID
	UserRef       string `json:"user_ref"`       // 用户上下文引用ID
	ProjectRef    string `json:"project_ref"`    // 项目上下文引用ID
}

// ContextChangeTracking 上下文变更追踪
type ContextChangeTracking struct {
	LastChangeTimestamp time.Time       `json:"last_change_timestamp"`
	ChangedDimensions   []string        `json:"changed_dimensions"`
	ChangeReasons       []string        `json:"change_reasons"`
	UpdateStrategy      string          `json:"update_strategy"`
	ChangeHistory       []ContextChange `json:"change_history"`
}

// ContextChange 上下文变更记录
type ContextChange struct {
	ChangeID   string                 `json:"change_id"`
	Timestamp  time.Time              `json:"timestamp"`
	ChangeType string                 `json:"change_type"`
	Dimension  string                 `json:"dimension"`
	OldValue   interface{}            `json:"old_value"`
	NewValue   interface{}            `json:"new_value"`
	Reason     string                 `json:"reason"`
	Confidence float64                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// ============================================================================
// 监控和指标相关模型
// ============================================================================

// LLMDrivenMetrics LLM驱动服务指标
type LLMDrivenMetrics struct {
	// 基础指标
	TotalRequests      int64 `json:"total_requests"`
	LLMDrivenRequests  int64 `json:"llm_driven_requests"`
	FallbackRequests   int64 `json:"fallback_requests"`
	SuccessfulRequests int64 `json:"successful_requests"`
	FailedRequests     int64 `json:"failed_requests"`

	// 性能指标
	AverageLatency      time.Duration `json:"average_latency"`
	P95Latency          time.Duration `json:"p95_latency"`
	P99Latency          time.Duration `json:"p99_latency"`
	ThroughputPerSecond float64       `json:"throughput_per_second"`

	// 质量指标
	SuccessRate      float64 `json:"success_rate"`
	FallbackRate     float64 `json:"fallback_rate"`
	UserSatisfaction float64 `json:"user_satisfaction"`
	AccuracyScore    float64 `json:"accuracy_score"`

	// 资源指标
	TokenUsage     int64   `json:"token_usage"`
	CostPerRequest float64 `json:"cost_per_request"`
	CacheHitRate   float64 `json:"cache_hit_rate"`

	// 时间信息
	LastUpdated     time.Time `json:"last_updated"`
	ReportingPeriod string    `json:"reporting_period"`
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	Timestamp time.Time              `json:"timestamp"`
	Metrics   *LLMDrivenMetrics      `json:"metrics"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ============================================================================
// 错误和状态相关模型
// ============================================================================

// LLMDrivenError LLM驱动服务错误
type LLMDrivenError struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`
	Component string    `json:"component"`
	Severity  string    `json:"severity"`
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	ServiceName   string                 `json:"service_name"`
	Status        string                 `json:"status"` // healthy, degraded, unhealthy
	Version       string                 `json:"version"`
	Uptime        time.Duration          `json:"uptime"`
	LastCheck     time.Time              `json:"last_check"`
	Components    map[string]string      `json:"components"`
	Configuration map[string]interface{} `json:"configuration"`
	Metrics       *LLMDrivenMetrics      `json:"metrics"`
}

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	Component string                 `json:"component"`
	Status    string                 `json:"status"`
	Message   string                 `json:"message"`
	Latency   time.Duration          `json:"latency"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
}
