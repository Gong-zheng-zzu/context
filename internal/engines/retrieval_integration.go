package engines

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval"
)

// RetrievalIntegrationEngine 检索集成引擎
// 负责协调现有检索系统和新的多维度检索系统
type RetrievalIntegrationEngine struct {
	// 现有检索组件（不修改）
	semanticEngine *SemanticAnalysisEngine
	// TODO: 添加其他现有检索组件的引用

	// 新的多维度检索引擎
	multiDimensionalEngine *multi_dimensional_retrieval.MultiDimensionalRetrievalEngine

	// 配置
	config *RetrievalIntegrationConfig
}

// RetrievalIntegrationConfig 检索集成配置
type RetrievalIntegrationConfig struct {
	// 总开关
	EnableMultiDimensional bool `yaml:"enable_multi_dimensional" json:"enable_multi_dimensional"`

	// 回退策略
	FallbackToLegacy bool `yaml:"fallback_to_legacy" json:"fallback_to_legacy"`

	// 结果合并策略
	MergeStrategy string `yaml:"merge_strategy" json:"merge_strategy"` // "replace", "merge", "hybrid"

	// 性能配置
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// NewRetrievalIntegrationEngine 创建检索集成引擎
func NewRetrievalIntegrationEngine(
	semanticEngine *SemanticAnalysisEngine,
	config *RetrievalIntegrationConfig,
) (*RetrievalIntegrationEngine, error) {

	if config == nil {
		config = &RetrievalIntegrationConfig{
			EnableMultiDimensional: false, // 默认关闭
			FallbackToLegacy:       true,
			MergeStrategy:          "replace",
			Timeout:                30 * time.Second,
		}
	}

	engine := &RetrievalIntegrationEngine{
		semanticEngine: semanticEngine,
		config:         config,
	}

	// 如果启用多维度检索，初始化多维度引擎
	if config.EnableMultiDimensional {
		if err := engine.initMultiDimensionalEngine(); err != nil {
			log.Printf("⚠️ 多维度检索引擎初始化失败: %v", err)
			if !config.FallbackToLegacy {
				return nil, fmt.Errorf("多维度检索引擎初始化失败且未启用回退: %w", err)
			}
		}
	}

	log.Printf("✅ 检索集成引擎初始化完成 - 多维度检索: %v, 回退策略: %v",
		config.EnableMultiDimensional, config.FallbackToLegacy)

	return engine, nil
}

// NewRetrievalIntegrationEngineWithMultiDimensional 创建检索集成引擎（注入多维度引擎）
func NewRetrievalIntegrationEngineWithMultiDimensional(
	semanticEngine *SemanticAnalysisEngine,
	multiDimensionalEngine *multi_dimensional_retrieval.MultiDimensionalRetrievalEngine,
	config *RetrievalIntegrationConfig,
) (*RetrievalIntegrationEngine, error) {

	if config == nil {
		config = &RetrievalIntegrationConfig{
			EnableMultiDimensional: true,
			FallbackToLegacy:       true,
			MergeStrategy:          "replace",
			Timeout:                30 * time.Second,
		}
	}

	engine := &RetrievalIntegrationEngine{
		semanticEngine:         semanticEngine,
		multiDimensionalEngine: multiDimensionalEngine, // 🔥 注入真实的多维度引擎
		config:                 config,
	}

	log.Printf("✅ 检索集成引擎初始化完成（多维度引擎注入） - 多维度检索: %v, 回退策略: %v",
		config.EnableMultiDimensional, config.FallbackToLegacy)

	return engine, nil
}

// initMultiDimensionalEngine 初始化多维度检索引擎
func (engine *RetrievalIntegrationEngine) initMultiDimensionalEngine() error {
	// 加载多维度检索配置
	multiConfig, err := multi_dimensional_retrieval.LoadConfig("config/multi_dimensional_retrieval.yaml")
	if err != nil {
		log.Printf("⚠️ 加载多维度检索配置失败，使用默认配置: %v", err)
		multiConfig = multi_dimensional_retrieval.DefaultConfig()
	}

	// 创建多维度检索引擎
	multiEngine, err := multi_dimensional_retrieval.NewMultiDimensionalRetrievalEngine(multiConfig)
	if err != nil {
		return fmt.Errorf("创建多维度检索引擎失败: %w", err)
	}

	engine.multiDimensionalEngine = multiEngine
	return nil
}

// IntegratedRetrievalRequest 集成检索请求
type IntegratedRetrievalRequest struct {
	// 基础信息
	UserID      string `json:"user_id"`
	SessionID   string `json:"session_id"`
	WorkspaceID string `json:"workspace_id"`
	Query       string `json:"query"`

	// 上下文信息
	ContextInfo *ContextInfo `json:"context_info"`

	// 检索参数
	MaxResults   int     `json:"max_results"`
	MinRelevance float64 `json:"min_relevance"`

	// 策略选择
	Strategy string `json:"strategy"` // "auto", "legacy", "multi_dimensional"

	// 请求ID
	RequestID string `json:"request_id"`
}

// IntegratedRetrievalResponse 集成检索响应
type IntegratedRetrievalResponse struct {
	// 基础信息
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`

	// 语义分析结果
	SemanticAnalysis *SemanticAnalysisResult `json:"semantic_analysis"`

	// 检索结果
	Results []IntegratedResult `json:"results"`
	Total   int                `json:"total"`

	// 执行信息
	Strategy    string        `json:"strategy"` // 实际使用的策略
	Duration    time.Duration `json:"duration"`
	EnginesUsed []string      `json:"engines_used"`

	// 性能指标
	SemanticAnalysisDuration time.Duration `json:"semantic_analysis_duration"`
	RetrievalDuration        time.Duration `json:"retrieval_duration"`
}

// IntegratedResult 集成检索结果项
type IntegratedResult struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"` // "legacy", "timeline", "knowledge", "vector"
	Content   string                 `json:"content"`
	Title     string                 `json:"title"`
	Score     float64                `json:"score"`
	Relevance float64                `json:"relevance"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Retrieve 执行集成检索
func (engine *RetrievalIntegrationEngine) Retrieve(ctx context.Context, request *IntegratedRetrievalRequest) (*IntegratedRetrievalResponse, error) {
	startTime := time.Now()

	// 第一步：语义分析（使用现有逻辑）
	log.Printf("🔍 开始语义分析 - 请求ID: %s, 查询: %s", request.RequestID, request.Query)

	semanticStartTime := time.Now()
	semanticResult, err := engine.semanticEngine.AnalyzeQuery(ctx, request.Query, request.SessionID)
	if err != nil {
		return nil, fmt.Errorf("语义分析失败: %w", err)
	}
	semanticDuration := time.Since(semanticStartTime)

	log.Printf("✅ 语义分析完成 - 意图: %s, 置信度: %.2f, 耗时: %v",
		semanticResult.Intent, semanticResult.Confidence, semanticDuration)

	// 第二步：选择检索策略
	strategy := engine.selectRetrievalStrategy(request, semanticResult)
	log.Printf("📋 选择检索策略: %s", strategy)

	// 第三步：执行检索
	retrievalStartTime := time.Now()
	results, enginesUsed, err := engine.executeRetrieval(ctx, request, semanticResult, strategy)
	if err != nil {
		return nil, fmt.Errorf("检索执行失败: %w", err)
	}
	retrievalDuration := time.Since(retrievalStartTime)

	// 构建响应
	response := &IntegratedRetrievalResponse{
		RequestID:                request.RequestID,
		Timestamp:                time.Now(),
		SemanticAnalysis:         semanticResult,
		Results:                  results,
		Total:                    len(results),
		Strategy:                 strategy,
		Duration:                 time.Since(startTime),
		EnginesUsed:              enginesUsed,
		SemanticAnalysisDuration: semanticDuration,
		RetrievalDuration:        retrievalDuration,
	}

	log.Printf("✅ 集成检索完成 - 请求ID: %s, 策略: %s, 结果数: %d, 总耗时: %v",
		request.RequestID, strategy, len(results), response.Duration)

	return response, nil
}

// selectRetrievalStrategy 选择检索策略
func (engine *RetrievalIntegrationEngine) selectRetrievalStrategy(request *IntegratedRetrievalRequest, semanticResult *SemanticAnalysisResult) string {
	// 如果明确指定策略
	if request.Strategy != "" && request.Strategy != "auto" {
		return request.Strategy
	}

	// 如果多维度检索未启用或不可用
	if !engine.config.EnableMultiDimensional || engine.multiDimensionalEngine == nil || !engine.multiDimensionalEngine.IsEnabled() {
		return "legacy"
	}

	// 自动选择策略（可以基于语义分析结果）
	// 例如：复杂查询使用多维度检索，简单查询使用传统检索
	if semanticResult.Confidence > 0.8 && len(semanticResult.Keywords) > 3 {
		return "multi_dimensional"
	}

	return "legacy"
}

// executeRetrieval 执行检索
func (engine *RetrievalIntegrationEngine) executeRetrieval(
	ctx context.Context,
	request *IntegratedRetrievalRequest,
	semanticResult *SemanticAnalysisResult,
	strategy string,
) ([]IntegratedResult, []string, error) {

	switch strategy {
	case "multi_dimensional":
		return engine.executeMultiDimensionalRetrieval(ctx, request, semanticResult)

	case "legacy":
		return engine.executeLegacyRetrieval(ctx, request, semanticResult)

	default:
		return nil, nil, fmt.Errorf("未知的检索策略: %s", strategy)
	}
}

// executeMultiDimensionalRetrieval 执行多维度检索
func (engine *RetrievalIntegrationEngine) executeMultiDimensionalRetrieval(
	ctx context.Context,
	request *IntegratedRetrievalRequest,
	semanticResult *SemanticAnalysisResult,
) ([]IntegratedResult, []string, error) {

	if engine.multiDimensionalEngine == nil {
		return nil, nil, fmt.Errorf("多维度检索引擎未初始化")
	}

	// 🔥 检查多维度引擎是否真正启用
	if !engine.multiDimensionalEngine.IsEnabled() {
		log.Printf("🔄 多维度检索引擎未启用，回退到传统检索")
		if engine.config.FallbackToLegacy {
			return engine.executeLegacyRetrieval(ctx, request, semanticResult)
		}
		return nil, nil, fmt.Errorf("多维度检索引擎未启用")
	}

	log.Printf("🚀 执行真正的多维度检索")

	// 构建多维度检索查询
	multiQuery := &multi_dimensional_retrieval.MultiDimensionalRetrievalQuery{
		UserID:           request.UserID,
		SessionID:        request.SessionID,
		WorkspaceID:      request.WorkspaceID,
		OriginalQuery:    request.Query,
		SemanticAnalysis: convertSemanticResult(semanticResult),
		MaxResults:       request.MaxResults,
		MinRelevance:     request.MinRelevance,
		RequestID:        request.RequestID,
	}

	// 执行多维度检索
	multiResult, err := engine.multiDimensionalEngine.Retrieve(ctx, multiQuery)
	if err != nil {
		// 如果启用回退策略
		if engine.config.FallbackToLegacy {
			log.Printf("⚠️ 多维度检索失败，回退到传统检索: %v", err)
			return engine.executeLegacyRetrieval(ctx, request, semanticResult)
		}
		return nil, nil, fmt.Errorf("多维度检索失败: %w", err)
	}

	log.Printf("✅ 多维度检索成功 - 结果数: %d, 使用引擎: %v",
		len(multiResult.Results), multiResult.EnginesUsed)

	// 转换结果格式
	results := make([]IntegratedResult, len(multiResult.Results))
	for i, result := range multiResult.Results {
		results[i] = IntegratedResult{
			ID:        result.ID,
			Source:    result.Source,
			Content:   result.Content,
			Title:     result.Title,
			Score:     result.Score,
			Relevance: result.Relevance,
			Timestamp: result.Timestamp,
			Metadata:  result.Metadata,
		}
	}

	return results, multiResult.EnginesUsed, nil
}

// executeLegacyRetrieval 执行传统检索
func (engine *RetrievalIntegrationEngine) executeLegacyRetrieval(
	ctx context.Context,
	request *IntegratedRetrievalRequest,
	semanticResult *SemanticAnalysisResult,
) ([]IntegratedResult, []string, error) {

	log.Printf("🔄 执行传统检索逻辑")

	// TODO: 调用现有的检索逻辑
	// 这里不修改现有代码，只是包装现有结果

	// 模拟现有检索结果
	results := []IntegratedResult{
		{
			ID:        "legacy_result_1",
			Source:    "legacy",
			Content:   "传统检索结果示例",
			Title:     "Legacy Result",
			Score:     0.8,
			Relevance: 0.8,
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"source": "legacy_system"},
		},
	}

	return results, []string{"legacy"}, nil
}

// IsMultiDimensionalEnabled 检查多维度检索是否启用
func (engine *RetrievalIntegrationEngine) IsMultiDimensionalEnabled() bool {
	return engine.config.EnableMultiDimensional &&
		engine.multiDimensionalEngine != nil &&
		engine.multiDimensionalEngine.IsEnabled()
}

// EnableMultiDimensional 启用多维度检索
func (engine *RetrievalIntegrationEngine) EnableMultiDimensional() error {
	if engine.multiDimensionalEngine == nil {
		if err := engine.initMultiDimensionalEngine(); err != nil {
			return fmt.Errorf("初始化多维度检索引擎失败: %w", err)
		}
	}

	engine.multiDimensionalEngine.Enable()
	engine.config.EnableMultiDimensional = true

	log.Printf("✅ 多维度检索已启用")
	return nil
}

// DisableMultiDimensional 禁用多维度检索
func (engine *RetrievalIntegrationEngine) DisableMultiDimensional() {
	if engine.multiDimensionalEngine != nil {
		engine.multiDimensionalEngine.Disable()
	}

	engine.config.EnableMultiDimensional = false
	log.Printf("⏸️ 多维度检索已禁用")
}

// GetMetrics 获取性能指标
func (engine *RetrievalIntegrationEngine) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	// 添加多维度检索指标
	if engine.multiDimensionalEngine != nil {
		metrics["multi_dimensional"] = engine.multiDimensionalEngine.GetMetrics()
	}

	// TODO: 添加其他指标

	return metrics
}

// Close 关闭引擎
func (engine *RetrievalIntegrationEngine) Close() error {
	log.Printf("🔄 关闭检索集成引擎...")

	if engine.multiDimensionalEngine != nil {
		if err := engine.multiDimensionalEngine.Close(); err != nil {
			log.Printf("⚠️ 关闭多维度检索引擎失败: %v", err)
		}
	}

	return nil
}

// convertSemanticResult 转换语义分析结果类型
func convertSemanticResult(result *SemanticAnalysisResult) *multi_dimensional_retrieval.SemanticAnalysisResult {
	if result == nil {
		return nil
	}

	// 转换实体
	entities := make([]multi_dimensional_retrieval.Entity, len(result.Entities))
	for i, entity := range result.Entities {
		entities[i] = multi_dimensional_retrieval.Entity{
			Text:       entity.Text,
			Type:       entity.Type,
			Confidence: entity.Confidence,
		}
	}

	// 转换查询
	var queries *multi_dimensional_retrieval.MultiDimensionalQuery
	if result.Queries != nil {
		queries = &multi_dimensional_retrieval.MultiDimensionalQuery{
			ContextQueries:   result.Queries.ContextQueries,
			TimelineQueries:  result.Queries.TimelineQueries,
			KnowledgeQueries: result.Queries.KnowledgeQueries,
			VectorQueries:    result.Queries.VectorQueries,
		}
	}

	return &multi_dimensional_retrieval.SemanticAnalysisResult{
		Intent:     string(result.Intent),
		Confidence: result.Confidence,
		Categories: result.Categories,
		Keywords:   result.Keywords,
		Entities:   entities,
		Queries:    queries,
		TokenUsage: result.TokenUsage,
		Metadata:   result.Metadata,
	}
}
