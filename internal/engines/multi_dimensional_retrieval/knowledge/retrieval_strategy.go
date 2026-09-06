package knowledge

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RetrievalStrategy 检索策略类型
type RetrievalStrategy string

const (
	StrategyTimeRecall        RetrievalStrategy = "time_recall"         // 时间回溯
	StrategyGraphPriority     RetrievalStrategy = "graph_priority"      // 图谱优先
	StrategyTimeContentHybrid RetrievalStrategy = "time_content_hybrid" // 时间+内容混合
	StrategyVectorOnly        RetrievalStrategy = "vector_only"         // 纯向量检索
)

// RetrievalRequest 检索请求
type RetrievalRequest struct {
	Query     string            `json:"query"`
	SessionID string            `json:"session_id"`
	UserID    string            `json:"user_id"`
	Workspace string            `json:"workspace"`
	Strategy  RetrievalStrategy `json:"strategy"`
	TopK      int               `json:"top_k"`
	TimeRange *TimeRange        `json:"time_range,omitempty"`
	EntityIDs []string          `json:"entity_ids,omitempty"`
	MinScore  float64           `json:"min_score"`
}

// TimeRange 时间范围
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	MemoryID  string  `json:"memory_id"`
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"` // vector, graph, time
	Timestamp int64   `json:"timestamp"`
	SessionID string  `json:"session_id"`
}

// QueryAnalysis 查询分析结果
type QueryAnalysis struct {
	HasTimeReference bool     `json:"has_time_reference"`
	HasEntityMention bool     `json:"has_entity_mention"`
	TimeKeywords     []string `json:"time_keywords"`
	EntityKeywords   []string `json:"entity_keywords"`
	QueryComplexity  string   `json:"query_complexity"` // simple, medium, complex
}

// SelectRetrievalStrategy 选择最佳检索策略
func SelectRetrievalStrategy(query string, analysis *QueryAnalysis) RetrievalStrategy {
	if analysis == nil {
		analysis = AnalyzeQuery(query)
	}

	// 策略选择逻辑
	// 1. 明确的时间引用 -> time_recall
	if analysis.HasTimeReference && len(analysis.TimeKeywords) > 0 {
		log.Printf("🎯 选择策略: time_recall (检测到时间引用: %v)", analysis.TimeKeywords)
		return StrategyTimeRecall
	}

	// 2. 实体提及 -> graph_priority
	if analysis.HasEntityMention && len(analysis.EntityKeywords) > 0 {
		log.Printf("🎯 选择策略: graph_priority (检测到实体: %v)", analysis.EntityKeywords)
		return StrategyGraphPriority
	}

	// 3. 复杂查询 -> time_content_hybrid
	if analysis.QueryComplexity == "complex" {
		log.Printf("🎯 选择策略: time_content_hybrid (复杂查询)")
		return StrategyTimeContentHybrid
	}

	// 4. 默认 -> vector_only
	log.Printf("🎯 选择策略: vector_only (默认策略)")
	return StrategyVectorOnly
}

// AnalyzeQuery 分析查询内容
func AnalyzeQuery(query string) *QueryAnalysis {
	analysis := &QueryAnalysis{
		TimeKeywords:   []string{},
		EntityKeywords: []string{},
	}

	queryLower := strings.ToLower(query)

	// 时间关键词检测
	timePatterns := []string{
		"昨天", "今天", "明天", "上周", "本周", "下周",
		"上个月", "这个月", "下个月", "去年", "今年", "明年",
		"之前", "以前", "刚才", "最近", "earlier", "yesterday",
		"today", "last week", "recently", "before", "ago",
		"\\d+天前", "\\d+小时前", "\\d+分钟前",
	}

	for _, pattern := range timePatterns {
		if matched, _ := regexp.MatchString(pattern, queryLower); matched {
			analysis.HasTimeReference = true
			analysis.TimeKeywords = append(analysis.TimeKeywords, pattern)
		}
	}

	// 实体关键词检测 (简单启发式)
	entityIndicators := []string{
		"项目", "系统", "服务", "模块", "组件", "接口", "API",
		"数据库", "缓存", "队列", "配置", "架构", "设计",
		"团队", "负责人", "开发", "测试", "部署",
		"project", "system", "service", "module", "component",
		"database", "cache", "queue", "config", "architecture",
	}

	for _, indicator := range entityIndicators {
		if strings.Contains(queryLower, strings.ToLower(indicator)) {
			analysis.HasEntityMention = true
			analysis.EntityKeywords = append(analysis.EntityKeywords, indicator)
		}
	}

	// 查询复杂度评估
	wordCount := len(strings.Fields(query))
	if wordCount > 20 || (analysis.HasTimeReference && analysis.HasEntityMention) {
		analysis.QueryComplexity = "complex"
	} else if wordCount > 10 || analysis.HasTimeReference || analysis.HasEntityMention {
		analysis.QueryComplexity = "medium"
	} else {
		analysis.QueryComplexity = "simple"
	}

	return analysis
}

// RRFConfig RRF融合配置
type RRFConfig struct {
	K            float64            `json:"k"`             // RRF参数k，默认60
	SourceWeight map[string]float64 `json:"source_weight"` // 来源权重
}

// DefaultRRFConfig 默认RRF配置
func DefaultRRFConfig() *RRFConfig {
	return &RRFConfig{
		K: 60.0,
		SourceWeight: map[string]float64{
			"vector": 1.0,
			"graph":  1.2,
			"time":   0.8,
		},
	}
}

// FuseResultsWithRRF 使用RRF算法融合多路检索结果
// RRF(d) = Σ(1 / (k + rank(d)))
func FuseResultsWithRRF(resultSets [][]RetrievalResult, config *RRFConfig) []RetrievalResult {
	if config == nil {
		config = DefaultRRFConfig()
	}

	// 收集所有MemoryID的RRF分数
	rrfScores := make(map[string]float64)
	resultMap := make(map[string]*RetrievalResult)

	for _, results := range resultSets {
		for rank, result := range results {
			// 计算RRF分数
			sourceWeight := config.SourceWeight[result.Source]
			if sourceWeight == 0 {
				sourceWeight = 1.0
			}
			rrfScore := sourceWeight / (config.K + float64(rank+1))
			rrfScores[result.MemoryID] += rrfScore

			// 保存结果详情（保留最高分的版本）
			if existing, ok := resultMap[result.MemoryID]; !ok || result.Score > existing.Score {
				resultCopy := result
				resultMap[result.MemoryID] = &resultCopy
			}
		}
	}

	// 构建最终结果并按RRF分数排序
	var fusedResults []RetrievalResult
	for memoryID, rrfScore := range rrfScores {
		if result, ok := resultMap[memoryID]; ok {
			result.Score = rrfScore // 使用RRF分数作为最终分数
			fusedResults = append(fusedResults, *result)
		}
	}

	// 按分数降序排序
	sort.Slice(fusedResults, func(i, j int) bool {
		return fusedResults[i].Score > fusedResults[j].Score
	})

	log.Printf("📊 RRF融合完成: 输入%d路结果，输出%d个结果", len(resultSets), len(fusedResults))
	return fusedResults
}

// MultiDimensionalRetriever 多维检索器
type MultiDimensionalRetriever struct {
	neo4jEngine *Neo4jEngine
	// vectorStore 接口待注入
}

// NewMultiDimensionalRetriever 创建多维检索器
func NewMultiDimensionalRetriever(neo4jEngine *Neo4jEngine) *MultiDimensionalRetriever {
	return &MultiDimensionalRetriever{
		neo4jEngine: neo4jEngine,
	}
}

// Retrieve 执行多维检索
func (r *MultiDimensionalRetriever) Retrieve(ctx context.Context, req *RetrievalRequest) ([]RetrievalResult, error) {
	// 如果未指定策略，自动选择
	if req.Strategy == "" {
		analysis := AnalyzeQuery(req.Query)
		req.Strategy = SelectRetrievalStrategy(req.Query, analysis)
	}

	var resultSets [][]RetrievalResult

	switch req.Strategy {
	case StrategyTimeRecall:
		// 时间回溯：主要依赖时间维度
		timeResults := r.retrieveByTime(ctx, req)
		vectorResults := r.retrieveByVector(ctx, req)
		resultSets = append(resultSets, timeResults, vectorResults)

	case StrategyGraphPriority:
		// 图谱优先：主要依赖知识图谱
		graphResults := r.retrieveByGraph(ctx, req)
		vectorResults := r.retrieveByVector(ctx, req)
		resultSets = append(resultSets, graphResults, vectorResults)

	case StrategyTimeContentHybrid:
		// 混合检索：三路融合
		timeResults := r.retrieveByTime(ctx, req)
		graphResults := r.retrieveByGraph(ctx, req)
		vectorResults := r.retrieveByVector(ctx, req)
		resultSets = append(resultSets, timeResults, graphResults, vectorResults)

	default: // StrategyVectorOnly
		// 纯向量检索
		vectorResults := r.retrieveByVector(ctx, req)
		resultSets = append(resultSets, vectorResults)
	}

	// RRF融合
	fusedResults := FuseResultsWithRRF(resultSets, nil)

	// 限制返回数量
	if req.TopK > 0 && len(fusedResults) > req.TopK {
		fusedResults = fusedResults[:req.TopK]
	}

	return fusedResults, nil
}

// retrieveByVector 向量检索（占位实现，需要注入VectorStore）
func (r *MultiDimensionalRetriever) retrieveByVector(ctx context.Context, req *RetrievalRequest) []RetrievalResult {
	// TODO: 调用VectorStore进行向量检索
	log.Printf("📡 执行向量检索: query=%s", req.Query)
	return []RetrievalResult{}
}

// retrieveByGraph 图谱检索
func (r *MultiDimensionalRetriever) retrieveByGraph(ctx context.Context, req *RetrievalRequest) []RetrievalResult {
	if r.neo4jEngine == nil {
		log.Printf("⚠️ Neo4j引擎未初始化，跳过图谱检索")
		return []RetrievalResult{}
	}

	var results []RetrievalResult

	// 如果提供了EntityIDs，直接获取关联的MemoryIDs
	if len(req.EntityIDs) > 0 {
		memoryIDs, err := r.neo4jEngine.GetMemoryIDsByEntityIDs(ctx, req.EntityIDs)
		if err != nil {
			log.Printf("⚠️ 图谱检索失败: %v", err)
			return results
		}

		for i, memoryID := range memoryIDs {
			results = append(results, RetrievalResult{
				MemoryID: memoryID,
				Score:    1.0 - float64(i)*0.01, // 简单的衰减分数
				Source:   "graph",
			})
		}
	}

	// 如果查询中包含实体关键词，尝试搜索
	analysis := AnalyzeQuery(req.Query)
	if len(analysis.EntityKeywords) > 0 {
		for _, keyword := range analysis.EntityKeywords {
			entities, err := r.neo4jEngine.SearchEntitiesByName(ctx, keyword, req.Workspace, 10)
			if err != nil {
				continue
			}

			for _, entity := range entities {
				for i, memoryID := range entity.MemoryIDs {
					results = append(results, RetrievalResult{
						MemoryID: memoryID,
						Score:    0.9 - float64(i)*0.02,
						Source:   "graph",
					})
				}
			}
		}
	}

	log.Printf("📊 图谱检索完成: 返回%d个结果", len(results))
	return results
}

// retrieveByTime 时间检索（占位实现）
func (r *MultiDimensionalRetriever) retrieveByTime(ctx context.Context, req *RetrievalRequest) []RetrievalResult {
	// TODO: 调用时间线检索
	log.Printf("⏱️ 执行时间检索: query=%s", req.Query)
	return []RetrievalResult{}
}
