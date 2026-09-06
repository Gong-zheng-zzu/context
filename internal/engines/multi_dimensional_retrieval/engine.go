package multi_dimensional_retrieval

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/utils"
)

// MultiDimensionalRetrievalEngine 多维度检索引擎
type MultiDimensionalRetrievalEngine struct {
	config *MultiDimensionalRetrievalConfig

	// 存储引擎（按需初始化）
	timelineEngine  TimelineEngine
	knowledgeEngine KnowledgeEngine
	vectorEngine    VectorEngine

	// 缓存和性能监控
	cache       Cache
	metrics     Metrics
	rateLimiter RateLimiter

	// 状态管理
	mu      sync.RWMutex
	enabled bool
}

// NewMultiDimensionalRetrievalEngine 创建多维度检索引擎
func NewMultiDimensionalRetrievalEngine(config *MultiDimensionalRetrievalConfig) (*MultiDimensionalRetrievalEngine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	engine := &MultiDimensionalRetrievalEngine{
		config:  config,
		enabled: config.IsEnabled(),
		metrics: NewMetrics(),
	}

	// 初始化缓存
	if config.Performance.EnableCache {
		engine.cache = NewCache(config.Performance.CacheSize, config.Performance.CacheTTL)
	}

	// 初始化限流器
	engine.rateLimiter = NewRateLimiter(config.Performance.RateLimit)

	// 按需初始化存储引擎
	if err := engine.initializeStorageEngines(); err != nil {
		return nil, fmt.Errorf("初始化存储引擎失败: %w", err)
	}

	log.Printf("✅ 多维度检索引擎初始化完成 - 启用状态: %v, 启用引擎: %v",
		engine.enabled, config.GetEnabledEngines())

	return engine, nil
}

// NewMultiDimensionalRetrievalEngineWithEngines 创建多维度检索引擎（注入具体存储引擎）
func NewMultiDimensionalRetrievalEngineWithEngines(
	config *MultiDimensionalRetrievalConfig,
	timelineEngine TimelineEngine,
	knowledgeEngine KnowledgeEngine,
	vectorEngine VectorEngine,
) (*MultiDimensionalRetrievalEngine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	engine := &MultiDimensionalRetrievalEngine{
		config:          config,
		enabled:         config.IsEnabled(),
		metrics:         NewMetrics(),
		timelineEngine:  timelineEngine,
		knowledgeEngine: knowledgeEngine,
		vectorEngine:    vectorEngine,
	}

	// 初始化缓存
	if config.Performance.EnableCache {
		engine.cache = NewCache(config.Performance.CacheSize, config.Performance.CacheTTL)
	}

	// 初始化限流器
	engine.rateLimiter = NewRateLimiter(config.Performance.RateLimit)

	log.Printf("✅ 多维度检索引擎初始化完成（注入模式） - 启用状态: %v, 启用引擎: %v",
		engine.enabled, config.GetEnabledEngines())

	return engine, nil
}

// initializeStorageEngines 初始化存储引擎
func (engine *MultiDimensionalRetrievalEngine) initializeStorageEngines() error {
	enabledEngines := engine.config.GetEnabledEngines()

	for _, engineType := range enabledEngines {
		switch engineType {
		case "timeline":
			if err := engine.initTimelineEngine(); err != nil {
				log.Printf("⚠️ TimescaleDB引擎初始化失败: %v", err)
				// 不返回错误，允许其他引擎继续工作
			}

		case "knowledge":
			if err := engine.initKnowledgeEngine(); err != nil {
				log.Printf("⚠️ Neo4j引擎初始化失败: %v", err)
				// 不返回错误，允许其他引擎继续工作
			}

		case "vector":
			if err := engine.initVectorEngine(); err != nil {
				log.Printf("⚠️ 向量引擎初始化失败: %v", err)
				// 不返回错误，允许其他引擎继续工作
			}
		}
	}

	return nil
}

// initTimelineEngine 初始化时间线引擎
func (engine *MultiDimensionalRetrievalEngine) initTimelineEngine() error {
	// TODO: 实现TimescaleDB引擎初始化
	log.Printf("📅 TimescaleDB时间线引擎初始化（待实现）")
	return nil
}

// initKnowledgeEngine 初始化知识图谱引擎
func (engine *MultiDimensionalRetrievalEngine) initKnowledgeEngine() error {
	// TODO: 实现Neo4j引擎初始化
	log.Printf("🧠 Neo4j知识图谱引擎初始化（待实现）")
	return nil
}

// initVectorEngine 初始化向量引擎
func (engine *MultiDimensionalRetrievalEngine) initVectorEngine() error {
	// TODO: 复用现有向量引擎，不修改现有逻辑
	log.Printf("🔍 向量引擎初始化（复用现有逻辑）")
	return nil
}

// MultiDimensionalRetrievalQuery 多维度检索查询
type MultiDimensionalRetrievalQuery struct {
	// 用户上下文
	UserID        string `json:"user_id"`
	SessionID     string `json:"session_id"`
	WorkspaceID   string `json:"workspace_id"`
	OriginalQuery string `json:"original_query"`

	// LLM分析结果
	SemanticAnalysis *SemanticAnalysisResult `json:"semantic_analysis"`

	// 检索参数
	MaxResults   int     `json:"max_results"`
	MinRelevance float64 `json:"min_relevance"`

	// 请求ID（用于追踪）
	RequestID string `json:"request_id"`
}

// MultiDimensionalResult 多维度检索结果
type MultiDimensionalResult struct {
	// 基础信息
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`

	// 检索结果
	Results []RetrievalResult `json:"results"`
	Total   int               `json:"total"`

	// 各维度结果统计
	TimelineResults  int `json:"timeline_results"`
	KnowledgeResults int `json:"knowledge_results"`
	VectorResults    int `json:"vector_results"`

	// 性能指标
	Duration    time.Duration `json:"duration"`
	EnginesUsed []string      `json:"engines_used"`
	CacheHit    bool          `json:"cache_hit"`
}

// RetrievalResult 检索结果项
type RetrievalResult struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"` // "timeline", "knowledge", "vector"
	Content   string                 `json:"content"`
	Title     string                 `json:"title"`
	Score     float64                `json:"score"`
	Relevance float64                `json:"relevance"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Retrieve 多维度检索主方法
func (engine *MultiDimensionalRetrievalEngine) Retrieve(ctx context.Context, query *MultiDimensionalRetrievalQuery) (*MultiDimensionalResult, error) {
	// 检查是否启用
	if !engine.IsEnabled() {
		return engine.fallbackToLegacyRetrieval(ctx, query)
	}

	// 限流检查
	if err := engine.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("限流检查失败: %w", err)
	}

	// 缓存检查
	if engine.cache != nil {
		if cached := engine.cache.Get(query.RequestID); cached != nil {
			log.Printf("🎯 缓存命中: %s", query.RequestID)
			result := cached.(*MultiDimensionalResult)
			result.CacheHit = true
			return result, nil
		}
	}

	// 执行多维度检索
	startTime := time.Now()
	result, err := engine.executeMultiDimensionalRetrieval(ctx, query)
	if err != nil {
		return nil, err
	}

	// 设置性能指标
	result.Duration = time.Since(startTime)
	result.RequestID = query.RequestID
	result.Timestamp = time.Now()

	// 缓存结果
	if engine.cache != nil {
		engine.cache.Set(query.RequestID, result)
	}

	// 记录指标
	engine.metrics.RecordQuery(result.Duration, len(result.Results), result.EnginesUsed)

	log.Printf("✅ 多维度检索完成 - 请求ID: %s, 结果数: %d, 耗时: %v, 引擎: %v",
		query.RequestID, result.Total, result.Duration, result.EnginesUsed)

	return result, nil
}

// executeMultiDimensionalRetrieval 执行多维度检索
func (engine *MultiDimensionalRetrievalEngine) executeMultiDimensionalRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) (*MultiDimensionalResult, error) {
	// 如果启用并行检索
	if engine.config.Strategy.EnableParallel {
		return engine.executeParallelRetrieval(ctx, query)
	} else {
		return engine.executeSequentialRetrieval(ctx, query)
	}
}

// executeParallelRetrieval 并行检索
func (engine *MultiDimensionalRetrievalEngine) executeParallelRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) (*MultiDimensionalResult, error) {
	log.Printf("🚀 执行真正的并行多维度检索")

	// 使用channel收集并行结果
	type retrievalResult struct {
		results []RetrievalResult
		engine  string
		err     error
	}

	resultChan := make(chan retrievalResult, 3)

	// 1. 并行执行时间线检索
	if engine.config.TimelineEnabled && engine.timelineEngine != nil {
		go func() {
			log.Printf("📅 并行执行时间线检索...")
			results, err := engine.executeTimelineRetrieval(ctx, query)
			resultChan <- retrievalResult{results: results, engine: "timeline", err: err}
		}()
	}

	// 2. 并行执行知识图谱检索
	if engine.config.KnowledgeEnabled && engine.knowledgeEngine != nil {
		go func() {
			log.Printf("🧠 并行执行知识图谱检索...")
			results, err := engine.executeKnowledgeRetrieval(ctx, query)
			resultChan <- retrievalResult{results: results, engine: "knowledge", err: err}
		}()
	}

	// 3. 并行执行向量检索
	if engine.config.VectorEnabled && engine.vectorEngine != nil {
		go func() {
			log.Printf("🔍 并行执行向量检索...")
			results, err := engine.executeVectorRetrieval(ctx, query)
			resultChan <- retrievalResult{results: results, engine: "vector", err: err}
		}()
	}

	// 收集结果
	var allResults []RetrievalResult
	var usedEngines []string
	expectedResults := 0

	if engine.config.TimelineEnabled && engine.timelineEngine != nil {
		expectedResults++
	}
	if engine.config.KnowledgeEnabled && engine.knowledgeEngine != nil {
		expectedResults++
	}
	if engine.config.VectorEnabled && engine.vectorEngine != nil {
		expectedResults++
	}

	for i := 0; i < expectedResults; i++ {
		select {
		case result := <-resultChan:
			if result.err != nil {
				log.Printf("⚠️ %s检索失败: %v", result.engine, result.err)
			} else {
				allResults = append(allResults, result.results...)
				usedEngines = append(usedEngines, result.engine)
				log.Printf("✅ %s检索完成，获得 %d 个结果", result.engine, len(result.results))
			}
		case <-ctx.Done():
			log.Printf("⚠️ 并行检索超时")
			return &MultiDimensionalResult{
				Results:     engine.mergeAndRankResults(allResults, query),
				Total:       len(allResults),
				EnginesUsed: usedEngines,
			}, nil
		}
	}

	// 结果融合和排序
	finalResults := engine.mergeAndRankResults(allResults, query)

	log.Printf("🎉 并行多维度检索完成 - 总结果数: %d, 使用引擎: %v",
		len(finalResults), usedEngines)

	return &MultiDimensionalResult{
		Results:     finalResults,
		Total:       len(finalResults),
		EnginesUsed: usedEngines,
	}, nil
}

// executeSequentialRetrieval 串行检索
func (engine *MultiDimensionalRetrievalEngine) executeSequentialRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) (*MultiDimensionalResult, error) {
	log.Printf("🔄 执行串行多维度检索")

	var allResults []RetrievalResult
	var usedEngines []string

	// 1. 时间线检索
	if engine.config.TimelineEnabled && engine.timelineEngine != nil {
		log.Printf("📅 执行时间线检索...")
		timelineResults, err := engine.executeTimelineRetrieval(ctx, query)
		if err != nil {
			log.Printf("⚠️ 时间线检索失败: %v", err)
		} else {
			allResults = append(allResults, timelineResults...)
			usedEngines = append(usedEngines, "timeline")
			log.Printf("✅ 时间线检索完成，获得 %d 个结果", len(timelineResults))
		}
	}

	// 2. 知识图谱检索
	if engine.config.KnowledgeEnabled && engine.knowledgeEngine != nil {
		log.Printf("🧠 执行知识图谱检索...")
		knowledgeResults, err := engine.executeKnowledgeRetrieval(ctx, query)
		if err != nil {
			log.Printf("⚠️ 知识图谱检索失败: %v", err)
		} else {
			allResults = append(allResults, knowledgeResults...)
			usedEngines = append(usedEngines, "knowledge")
			log.Printf("✅ 知识图谱检索完成，获得 %d 个结果", len(knowledgeResults))
		}
	}

	// 3. 向量检索
	if engine.config.VectorEnabled && engine.vectorEngine != nil {
		log.Printf("🔍 执行向量检索...")
		vectorResults, err := engine.executeVectorRetrieval(ctx, query)
		if err != nil {
			log.Printf("⚠️ 向量检索失败: %v", err)
		} else {
			allResults = append(allResults, vectorResults...)
			usedEngines = append(usedEngines, "vector")
			log.Printf("✅ 向量检索完成，获得 %d 个结果", len(vectorResults))
		}
	}

	// 4. 结果融合和排序
	finalResults := engine.mergeAndRankResults(allResults, query)

	return &MultiDimensionalResult{
		Results:     finalResults,
		Total:       len(finalResults),
		EnginesUsed: usedEngines,
	}, nil
}

// fallbackToLegacyRetrieval 回退到现有检索逻辑
func (engine *MultiDimensionalRetrievalEngine) fallbackToLegacyRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) (*MultiDimensionalResult, error) {
	log.Printf("🔄 多维度检索未启用，回退到现有检索逻辑")

	// TODO: 调用现有的检索逻辑
	// 这里不修改现有代码，只是包装现有结果

	return &MultiDimensionalResult{
		RequestID:   query.RequestID,
		Timestamp:   time.Now(),
		Results:     []RetrievalResult{}, // 空结果，表示使用现有逻辑
		Total:       0,
		Duration:    0,
		EnginesUsed: []string{"legacy"},
		CacheHit:    false,
	}, nil
}

// IsEnabled 检查引擎是否启用
func (engine *MultiDimensionalRetrievalEngine) IsEnabled() bool {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.enabled
}

// Enable 启用引擎
func (engine *MultiDimensionalRetrievalEngine) Enable() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.enabled = true
	log.Printf("✅ 多维度检索引擎已启用")
}

// Disable 禁用引擎
func (engine *MultiDimensionalRetrievalEngine) Disable() {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.enabled = false
	log.Printf("⏸️ 多维度检索引擎已禁用")
}

// GetMetrics 获取性能指标
func (engine *MultiDimensionalRetrievalEngine) GetMetrics() Metrics {
	return engine.metrics
}

// Close 关闭引擎
func (engine *MultiDimensionalRetrievalEngine) Close() error {
	log.Printf("🔄 关闭多维度检索引擎...")

	// 关闭各存储引擎连接
	// TODO: 实现各引擎的关闭逻辑

	return nil
}

// executeTimelineRetrieval 执行时间线检索
func (engine *MultiDimensionalRetrievalEngine) executeTimelineRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) ([]RetrievalResult, error) {
	if engine.timelineEngine == nil {
		return nil, fmt.Errorf("时间线引擎未初始化")
	}

	// 🔥 真正调用时间线引擎
	timelineQuery := &TimelineQuery{
		UserID:      query.UserID,
		SessionID:   query.SessionID,
		WorkspaceID: utils.ExtractWorkspaceNameFromPath(query.WorkspaceID), // 🔥 修复：使用公共工具函数
		SearchText:  query.OriginalQuery,
		Keywords:    extractKeywords(query.SemanticAnalysis),
		EventTypes:  []string{"discussion", "problem_solve", "design", "code_edit"},
		Limit:       query.MaxResults,
		Offset:      0,
	}

	// 如果有时间线查询，添加时间范围
	if query.SemanticAnalysis != nil && query.SemanticAnalysis.Queries != nil {
		// 基于语义分析结果设置时间范围
		timelineQuery.TimeRanges = []TimeRange{
			{
				StartTime: time.Now().Add(-24 * time.Hour), // 最近24小时
				EndTime:   time.Now(),
				Label:     "recent",
			},
		}
	}

	result, err := engine.timelineEngine.RetrieveEvents(ctx, timelineQuery)
	if err != nil {
		return nil, fmt.Errorf("时间线检索失败: %w", err)
	}

	// 转换为统一的检索结果格式
	results := make([]RetrievalResult, len(result.Events))
	for i, event := range result.Events {
		results[i] = RetrievalResult{
			ID:        event.ID,
			Source:    "timeline",
			Content:   event.Content,
			Title:     event.Title,
			Score:     event.ImportanceScore, // 使用统一模型的ImportanceScore字段
			Relevance: event.RelevanceScore,  // 使用统一模型的RelevanceScore字段
			Timestamp: event.Timestamp,
			Metadata: map[string]interface{}{ // 构建元数据
				"event_type":    event.EventType,
				"source_doc_id": event.SourceDocID,
				"user_id":       event.UserID,
				"workspace_id":  event.WorkspaceID,
				"session_id":    event.SessionID,
			},
		}
	}

	return results, nil
}

// executeKnowledgeRetrieval 执行知识图谱检索
func (engine *MultiDimensionalRetrievalEngine) executeKnowledgeRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) ([]RetrievalResult, error) {
	if engine.knowledgeEngine == nil {
		return nil, fmt.Errorf("知识图谱引擎未初始化")
	}

	// 🔥 真正调用知识图谱引擎
	keywords := extractKeywords(query.SemanticAnalysis)
	if len(keywords) == 0 {
		return []RetrievalResult{}, nil
	}

	knowledgeQuery := &KnowledgeQuery{
		UserID:        query.UserID,
		SessionID:     query.SessionID,
		WorkspaceID:   query.WorkspaceID,
		StartNodes:    keywords[:min(len(keywords), 3)], // 最多使用前3个关键词作为起始节点
		MaxDepth:      2,
		MaxNodes:      query.MaxResults,
		MinWeight:     0.5,
		RelationTypes: []string{"RELATED_TO", "USED_WITH", "IMPLEMENTS", "SOLVES"},
		NodeTypes:     []string{"Concept", "Technology"},
	}

	result, err := engine.knowledgeEngine.ExpandGraph(ctx, knowledgeQuery)
	if err != nil {
		return nil, fmt.Errorf("知识图谱检索失败: %w", err)
	}

	// 转换为统一的检索结果格式
	results := make([]RetrievalResult, len(result.Nodes))
	for i, node := range result.Nodes {
		results[i] = RetrievalResult{
			ID:        node.ID,
			Source:    "knowledge",
			Content:   fmt.Sprintf("知识概念: %s", node.Name),
			Title:     node.Name,
			Score:     node.Score,
			Relevance: node.Score,
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"node_type":  node.Type,
				"doc_ids":    node.DocIDs,
				"properties": node.Properties,
				"depth":      node.Depth,
				"source":     "knowledge_engine",
			},
		}
	}

	return results, nil
}

// executeVectorRetrieval 执行向量检索
func (engine *MultiDimensionalRetrievalEngine) executeVectorRetrieval(ctx context.Context, query *MultiDimensionalRetrievalQuery) ([]RetrievalResult, error) {
	if engine.vectorEngine == nil {
		return nil, fmt.Errorf("向量引擎未初始化")
	}

	// 调用向量引擎的检索方法
	// TODO: 这里需要将MultiDimensionalRetrievalQuery转换为向量引擎的查询格式
	// 目前返回模拟结果

	results := []RetrievalResult{
		{
			ID:        "vector_result_1",
			Source:    "vector",
			Content:   "向量检索结果示例",
			Title:     "Vector Result",
			Score:     0.88,
			Relevance: 0.88,
			Timestamp: time.Now(),
			Metadata:  map[string]interface{}{"source": "vector_engine"},
		},
	}

	return results, nil
}

// mergeAndRankResults 合并和排序结果
func (engine *MultiDimensionalRetrievalEngine) mergeAndRankResults(results []RetrievalResult, query *MultiDimensionalRetrievalQuery) []RetrievalResult {
	if len(results) == 0 {
		return results
	}

	// 检查是否启用RRF融合
	if engine.config.Strategy.EnableRRF && engine.config.Strategy.FusionMethod == "rrf" {
		return engine.mergeWithRRF(results, query)
	}

	// 降级到原有的简单融合逻辑
	return engine.mergeWithSimpleFusion(results, query)
}

// mergeWithRRF 使用RRF算法融合结果
func (engine *MultiDimensionalRetrievalEngine) mergeWithRRF(results []RetrievalResult, query *MultiDimensionalRetrievalQuery) []RetrievalResult {
	// 按来源分组结果
	resultsBySource := make(map[string][]RetrievalResult)
	for _, result := range results {
		source := result.Source
		if source == "" {
			source = "unknown"
		}
		resultsBySource[source] = append(resultsBySource[source], result)
	}

	// 对每个来源的结果按Score排序
	for source := range resultsBySource {
		sourceResults := resultsBySource[source]
		for i := 0; i < len(sourceResults)-1; i++ {
			for j := i + 1; j < len(sourceResults); j++ {
				if sourceResults[i].Score < sourceResults[j].Score {
					sourceResults[i], sourceResults[j] = sourceResults[j], sourceResults[i]
				}
			}
		}
		resultsBySource[source] = sourceResults
	}

	// 计算RRF分数
	k := engine.config.Strategy.RRFParameter
	if k <= 0 {
		k = 60.0 // 默认值
	}

	// 📊 RRF融合输入日志
	log.Printf("🔀 [RRF融合输入] 总结果数=%d k=%.1f", len(results), k)
	vectorWeight := engine.config.Strategy.SourceWeights["vector"]
	knowledgeWeight := engine.config.Strategy.SourceWeights["knowledge"]
	timelineWeight := engine.config.Strategy.SourceWeights["timeline"]
	if vectorWeight == 0 {
		vectorWeight = 1.0
	}
	if knowledgeWeight == 0 {
		knowledgeWeight = 1.0
	}
	if timelineWeight == 0 {
		timelineWeight = 1.0
	}
	log.Printf("  权重配置: vector=%.2f graph=%.2f timeline=%.2f",
		vectorWeight, knowledgeWeight, timelineWeight)
	for source, sourceResults := range resultsBySource {
		log.Printf("  来源[%s]: %d条结果", source, len(sourceResults))
		for i, r := range sourceResults {
			if i >= 3 {
				break
			}
			log.Printf("    [%d] ID=%s Score=%.4f", i+1, r.ID, r.Score)
		}
	}

	rrfScores := make(map[string]float64)
	resultMap := make(map[string]RetrievalResult)

	for source, sourceResults := range resultsBySource {
		// 获取来源权重
		sourceWeight := engine.config.Strategy.SourceWeights[source]
		if sourceWeight == 0 {
			sourceWeight = 1.0
		}

		// 计算RRF分数: RRF(d) = Σ source_weight_m / (k + rank_m(d))
		for rank, result := range sourceResults {
			rrfScore := sourceWeight / (k + float64(rank+1))
			rrfScores[result.ID] += rrfScore

			// 保留结果详情（保留最高分的版本）
			if existing, ok := resultMap[result.ID]; !ok || result.Score > existing.Score {
				resultMap[result.ID] = result
			}
		}
	}

	// 构建最终结果并按RRF分数排序
	fusedResults := make([]RetrievalResult, 0, len(rrfScores))
	for id, rrfScore := range rrfScores {
		if result, ok := resultMap[id]; ok {
			result.Relevance = rrfScore // 使用RRF分数作为最终相关性分数
			fusedResults = append(fusedResults, result)
		}
	}

	// 按RRF分数降序排序
	for i := 0; i < len(fusedResults)-1; i++ {
		for j := i + 1; j < len(fusedResults); j++ {
			if fusedResults[i].Relevance < fusedResults[j].Relevance {
				fusedResults[i], fusedResults[j] = fusedResults[j], fusedResults[i]
			}
		}
	}

	// 限制结果数量
	if query.MaxResults > 0 && len(fusedResults) > query.MaxResults {
		fusedResults = fusedResults[:query.MaxResults]
	}

	// 📊 RRF融合输出日志
	log.Printf("✅ [RRF融合输出] Top-10结果:")
	for i, r := range fusedResults {
		if i >= 10 {
			break
		}
		log.Printf("  [%d] ID=%s RRFScore=%.4f OrigScore=%.4f Source=%s",
			i+1, r.ID, r.Relevance, r.Score, r.Source)
	}
	log.Printf("📊 [RRF融合输出] 融合完成: 输入%d个来源, 输出%d个结果, k=%.1f", len(resultsBySource), len(fusedResults), k)
	return fusedResults
}

// mergeWithSimpleFusion 使用简单融合逻辑（向后兼容）
func (engine *MultiDimensionalRetrievalEngine) mergeWithSimpleFusion(results []RetrievalResult, query *MultiDimensionalRetrievalQuery) []RetrievalResult {
	// 1. 去重（基于ID）
	uniqueResults := make(map[string]RetrievalResult)
	for _, result := range results {
		if existing, exists := uniqueResults[result.ID]; exists {
			// 如果已存在，保留得分更高的
			if result.Score > existing.Score {
				uniqueResults[result.ID] = result
			}
		} else {
			uniqueResults[result.ID] = result
		}
	}

	// 2. 转换为切片
	finalResults := make([]RetrievalResult, 0, len(uniqueResults))
	for _, result := range uniqueResults {
		finalResults = append(finalResults, result)
	}

	// 3. 按相关性排序
	for i := 0; i < len(finalResults)-1; i++ {
		for j := i + 1; j < len(finalResults); j++ {
			if finalResults[i].Relevance < finalResults[j].Relevance {
				finalResults[i], finalResults[j] = finalResults[j], finalResults[i]
			}
		}
	}

	// 4. 限制结果数量
	if query.MaxResults > 0 && len(finalResults) > query.MaxResults {
		finalResults = finalResults[:query.MaxResults]
	}

	return finalResults
}

// extractKeywords 从语义分析结果中提取关键词
// 🔥 优先使用intent_analysis.key_concepts，回退到传统Keywords
func extractKeywords(semanticAnalysis *SemanticAnalysisResult) []string {
	if semanticAnalysis == nil {
		return []string{}
	}

	// 🔥 优先使用LLM intent_analysis提取的关键概念
	if len(semanticAnalysis.KeyConcepts) > 0 {
		return semanticAnalysis.KeyConcepts
	}

	// 🔧 回退到传统Keywords（兼容旧版本）
	return semanticAnalysis.Keywords
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
