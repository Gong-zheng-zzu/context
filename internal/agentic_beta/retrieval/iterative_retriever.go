package retrieval

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// IterativeRetriever 迭代检索器 - 实现多轮检索和质量评估
type IterativeRetriever struct {
	config           *IterativeRetrieverConfig
	queryRewriter    *QueryRewriter
	qualityEvaluator QualityEvaluator
	enabled          bool
	stats            *IterativeStats
}

// IterativeRetrieverConfig 迭代检索器配置
type IterativeRetrieverConfig struct {
	MaxIterations        int     `json:"max_iterations"`        // 最大迭代次数
	QualityThreshold     float64 `json:"quality_threshold"`     // 质量阈值
	ImprovementThreshold float64 `json:"improvement_threshold"` // 改进阈值
	TimeoutMs            int     `json:"timeout_ms"`            // 超时时间(毫秒)

	// 检索策略
	RetrievalStrategy  string `json:"retrieval_strategy"`   // "adaptive", "conservative", "aggressive"
	MinResultsRequired int    `json:"min_results_required"` // 最少结果数量

	// 质量评估配置
	QualityMetrics []string `json:"quality_metrics"` // ["relevance", "diversity", "completeness"]
	FeedbackLoop   bool     `json:"feedback_loop"`   // 是否启用反馈循环
}

// QualityEvaluator 质量评估器接口
type QualityEvaluator interface {
	EvaluateResults(ctx context.Context, query string, results []RetrievalResult) *QualityAssessment
	SuggestImprovements(assessment *QualityAssessment) []ImprovementSuggestion
}

// RetrievalResult 检索结果
type RetrievalResult struct {
	Content       string                 `json:"content"`
	Score         float64                `json:"score"`
	Source        string                 `json:"source"`
	Metadata      map[string]interface{} `json:"metadata"`
	RetrievalTime time.Time              `json:"retrieval_time"`
}

// QualityAssessment 质量评估结果
type QualityAssessment struct {
	OverallScore      float64                `json:"overall_score"`
	RelevanceScore    float64                `json:"relevance_score"`
	DiversityScore    float64                `json:"diversity_score"`
	CompletenessScore float64                `json:"completeness_score"`
	Confidence        float64                `json:"confidence"`
	Issues            []QualityIssue         `json:"issues"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// QualityIssue 质量问题
type QualityIssue struct {
	Type        string  `json:"type"`     // "low_relevance", "redundancy", "incompleteness"
	Severity    string  `json:"severity"` // "low", "medium", "high"
	Description string  `json:"description"`
	Score       float64 `json:"score"`
}

// ImprovementSuggestion 改进建议
type ImprovementSuggestion struct {
	Type         string                 `json:"type"`     // "query_rewrite", "expand_search", "filter_results"
	Priority     int                    `json:"priority"` // 1-10
	Description  string                 `json:"description"`
	Parameters   map[string]interface{} `json:"parameters"`
	ExpectedGain float64                `json:"expected_gain"` // 预期改进幅度
}

// IterativeSearchResult 迭代检索最终结果
type IterativeSearchResult struct {
	FinalResults      []RetrievalResult       `json:"final_results"`
	IterationCount    int                     `json:"iteration_count"`
	TotalTime         time.Duration           `json:"total_time"`
	QualityHistory    []QualityAssessment     `json:"quality_history"`
	QueryHistory      []string                `json:"query_history"`
	ImprovementLog    []ImprovementSuggestion `json:"improvement_log"`
	FinalQuality      *QualityAssessment      `json:"final_quality"`
	Success           bool                    `json:"success"`
	TerminationReason string                  `json:"termination_reason"`
}

// IterativeStats 迭代统计信息
type IterativeStats struct {
	TotalSearches       int64         `json:"total_searches"`
	SuccessfulSearches  int64         `json:"successful_searches"`
	AverageIterations   float64       `json:"average_iterations"`
	AverageQuality      float64       `json:"average_quality"`
	AverageTime         time.Duration `json:"average_time"`
	QualityImprovements int64         `json:"quality_improvements"`
}

// NewIterativeRetriever 创建迭代检索器
func NewIterativeRetriever(config *IterativeRetrieverConfig, queryRewriter *QueryRewriter) *IterativeRetriever {
	return &IterativeRetriever{
		config:           config,
		queryRewriter:    queryRewriter,
		qualityEvaluator: &DefaultQualityEvaluator{},
		enabled:          true,
		stats:            &IterativeStats{},
	}
}

// Search 执行迭代检索 - 核心入口方法
func (ir *IterativeRetriever) Search(ctx context.Context, originalQuery string, retriever func(string) ([]RetrievalResult, error)) (*IterativeSearchResult, error) {
	if !ir.enabled {
		// 如果禁用，执行简单检索
		results, err := retriever(originalQuery)
		if err != nil {
			return nil, err
		}
		return &IterativeSearchResult{
			FinalResults:      results,
			IterationCount:    1,
			Success:           true,
			TerminationReason: "disabled",
		}, nil
	}

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(ir.config.TimeoutMs)*time.Millisecond)
	defer cancel()

	result := &IterativeSearchResult{
		QueryHistory:   make([]string, 0),
		QualityHistory: make([]QualityAssessment, 0),
		ImprovementLog: make([]ImprovementSuggestion, 0),
	}

	currentQuery := originalQuery
	var bestResults []RetrievalResult
	var bestQuality *QualityAssessment
	bestQualityScore := 0.0

	// 🔥 迭代检索循环
	for iteration := 0; iteration < ir.config.MaxIterations; iteration++ {
		select {
		case <-ctx.Done():
			result.TerminationReason = "timeout"
			break
		default:
		}

		log.Printf("🔄 迭代检索 #%d: 查询='%s'", iteration+1, currentQuery)

		// 执行当前查询
		currentResults, err := retriever(currentQuery)
		if err != nil {
			log.Printf("❌ 检索失败: %v", err)
			continue
		}

		// 🔥 质量评估
		assessment := ir.qualityEvaluator.EvaluateResults(ctx, currentQuery, currentResults)
		result.QualityHistory = append(result.QualityHistory, *assessment)
		result.QueryHistory = append(result.QueryHistory, currentQuery)

		log.Printf("📊 质量评分: %.2f (相关性:%.2f, 多样性:%.2f, 完整性:%.2f)",
			assessment.OverallScore, assessment.RelevanceScore,
			assessment.DiversityScore, assessment.CompletenessScore)

		// 🔥 更新最佳结果
		if assessment.OverallScore > bestQualityScore {
			bestResults = currentResults
			bestQuality = assessment
			bestQualityScore = assessment.OverallScore
			log.Printf("✅ 发现更好结果，质量提升: %.2f -> %.2f", bestQualityScore, assessment.OverallScore)
		}

		// 🔥 检查是否达到质量阈值
		if assessment.OverallScore >= ir.config.QualityThreshold {
			result.TerminationReason = "quality_threshold_reached"
			log.Printf("🎯 达到质量阈值: %.2f >= %.2f", assessment.OverallScore, ir.config.QualityThreshold)
			break
		}

		// 🔥 检查是否需要继续改进
		if !ir.shouldContinueImproving(assessment, iteration) {
			result.TerminationReason = "no_improvement_possible"
			log.Printf("🛑 无进一步改进空间，停止迭代")
			break
		}

		// 🔥 生成改进建议
		suggestions := ir.qualityEvaluator.SuggestImprovements(assessment)
		if len(suggestions) == 0 {
			result.TerminationReason = "no_suggestions"
			log.Printf("🛑 无改进建议，停止迭代")
			break
		}

		// 🔥 应用最佳改进建议
		bestSuggestion := ir.selectBestSuggestion(suggestions)
		result.ImprovementLog = append(result.ImprovementLog, bestSuggestion)

		log.Printf("🔧 应用改进建议: %s (优先级:%d, 预期收益:%.2f)",
			bestSuggestion.Description, bestSuggestion.Priority, bestSuggestion.ExpectedGain)

		// 🔥 根据建议改写查询
		nextQuery, err := ir.applyImprovement(ctx, currentQuery, bestSuggestion)
		if err != nil {
			log.Printf("❌ 应用改进失败: %v", err)
			continue
		}

		// 检查查询是否有实质变化
		if nextQuery == currentQuery {
			result.TerminationReason = "query_unchanged"
			log.Printf("🛑 查询无变化，停止迭代")
			break
		}

		currentQuery = nextQuery
	}

	// 🔥 设置最终结果
	result.FinalResults = bestResults
	result.FinalQuality = bestQuality
	result.IterationCount = len(result.QueryHistory)
	result.TotalTime = time.Since(startTime)
	result.Success = bestQuality != nil && bestQuality.OverallScore >= ir.config.QualityThreshold

	if result.TerminationReason == "" {
		result.TerminationReason = "max_iterations_reached"
	}

	// 🔥 更新统计信息
	ir.updateStats(result)

	log.Printf("🏁 迭代检索完成: %d轮迭代, 最终质量:%.2f, 耗时:%v, 原因:%s",
		result.IterationCount, bestQualityScore, result.TotalTime, result.TerminationReason)

	return result, nil
}

// 🔥 默认质量评估器实现
type DefaultQualityEvaluator struct{}

func (dqe *DefaultQualityEvaluator) EvaluateResults(ctx context.Context, query string, results []RetrievalResult) *QualityAssessment {
	if len(results) == 0 {
		return &QualityAssessment{
			OverallScore:      0.0,
			RelevanceScore:    0.0,
			DiversityScore:    0.0,
			CompletenessScore: 0.0,
			Confidence:        1.0,
			Issues: []QualityIssue{{
				Type:        "no_results",
				Severity:    "high",
				Description: "未找到任何检索结果",
				Score:       0.0,
			}},
		}
	}

	// 🔥 相关性评估
	relevanceScore := dqe.calculateRelevance(query, results)

	// 🔥 多样性评估
	diversityScore := dqe.calculateDiversity(results)

	// 🔥 完整性评估
	completenessScore := dqe.calculateCompleteness(query, results)

	// 🔥 综合评分 (加权平均)
	overallScore := (relevanceScore*0.5 + diversityScore*0.2 + completenessScore*0.3)

	// 🔥 识别质量问题
	issues := dqe.identifyIssues(relevanceScore, diversityScore, completenessScore, results)

	return &QualityAssessment{
		OverallScore:      overallScore,
		RelevanceScore:    relevanceScore,
		DiversityScore:    diversityScore,
		CompletenessScore: completenessScore,
		Confidence:        dqe.calculateConfidence(results),
		Issues:            issues,
		Metadata: map[string]interface{}{
			"result_count": len(results),
			"avg_score":    dqe.calculateAverageScore(results),
			"query_length": len(query),
		},
	}
}

func (dqe *DefaultQualityEvaluator) SuggestImprovements(assessment *QualityAssessment) []ImprovementSuggestion {
	suggestions := make([]ImprovementSuggestion, 0)

	// 🔥 基于问题类型生成建议
	for _, issue := range assessment.Issues {
		switch issue.Type {
		case "low_relevance":
			suggestions = append(suggestions, ImprovementSuggestion{
				Type:         "query_rewrite",
				Priority:     8,
				Description:  "重写查询以提高相关性",
				ExpectedGain: 0.3,
				Parameters: map[string]interface{}{
					"focus":  "relevance",
					"method": "keyword_enhancement",
				},
			})

		case "low_diversity":
			suggestions = append(suggestions, ImprovementSuggestion{
				Type:         "expand_search",
				Priority:     6,
				Description:  "扩展搜索范围以增加多样性",
				ExpectedGain: 0.2,
				Parameters: map[string]interface{}{
					"strategy": "semantic_expansion",
					"scope":    "broader",
				},
			})

		case "incompleteness":
			suggestions = append(suggestions, ImprovementSuggestion{
				Type:         "query_decomposition",
				Priority:     7,
				Description:  "分解查询以获得更完整结果",
				ExpectedGain: 0.25,
				Parameters: map[string]interface{}{
					"method": "sub_queries",
					"depth":  2,
				},
			})
		}
	}

	// 🔥 基于整体质量分数添加通用建议
	if assessment.OverallScore < 0.3 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Type:         "query_rewrite",
			Priority:     9,
			Description:  "重新制定查询策略",
			ExpectedGain: 0.4,
			Parameters: map[string]interface{}{
				"strategy": "complete_rewrite",
			},
		})
	} else if assessment.OverallScore < 0.6 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Type:         "refine_query",
			Priority:     5,
			Description:  "细化查询表达",
			ExpectedGain: 0.15,
			Parameters: map[string]interface{}{
				"method": "term_weighting",
			},
		})
	}

	return suggestions
}

// 🔥 辅助方法实现

func (dqe *DefaultQualityEvaluator) calculateRelevance(query string, results []RetrievalResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	totalRelevance := 0.0
	for _, result := range results {
		// 简化实现：基于分数和内容匹配
		scoreRelevance := result.Score
		contentRelevance := dqe.calculateContentMatch(query, result.Content)
		relevance := (scoreRelevance + contentRelevance) / 2.0
		totalRelevance += relevance
	}

	return totalRelevance / float64(len(results))
}

func (dqe *DefaultQualityEvaluator) calculateDiversity(results []RetrievalResult) float64 {
	if len(results) <= 1 {
		return 1.0
	}

	// 简化实现：基于内容相似度计算多样性
	similarities := make([]float64, 0)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			similarity := dqe.calculateContentSimilarity(results[i].Content, results[j].Content)
			similarities = append(similarities, similarity)
		}
	}

	if len(similarities) == 0 {
		return 1.0
	}

	avgSimilarity := 0.0
	for _, sim := range similarities {
		avgSimilarity += sim
	}
	avgSimilarity /= float64(len(similarities))

	// 多样性 = 1 - 平均相似度
	return 1.0 - avgSimilarity
}

func (dqe *DefaultQualityEvaluator) calculateCompleteness(query string, results []RetrievalResult) float64 {
	// 简化实现：基于结果数量和查询复杂度
	queryComplexity := dqe.estimateQueryComplexity(query)
	resultCoverage := float64(len(results)) / (queryComplexity * 3.0) // 假设每个复杂度单位需要3个结果

	if resultCoverage > 1.0 {
		resultCoverage = 1.0
	}

	return resultCoverage
}

func (dqe *DefaultQualityEvaluator) calculateContentMatch(query, content string) float64 {
	// 简化实现：基于关键词匹配
	queryWords := extractWords(query)
	contentWords := extractWords(content)

	matches := 0
	for _, qWord := range queryWords {
		for _, cWord := range contentWords {
			if qWord == cWord {
				matches++
				break
			}
		}
	}

	if len(queryWords) == 0 {
		return 0.0
	}

	return float64(matches) / float64(len(queryWords))
}

func (dqe *DefaultQualityEvaluator) calculateContentSimilarity(content1, content2 string) float64 {
	// 简化实现：基于共同词汇比例
	words1 := extractWords(content1)
	words2 := extractWords(content2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	intersection := 0
	for _, w1 := range words1 {
		for _, w2 := range words2 {
			if w1 == w2 {
				intersection++
				break
			}
		}
	}

	union := len(words1) + len(words2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func (dqe *DefaultQualityEvaluator) estimateQueryComplexity(query string) float64 {
	words := extractWords(query)
	// 复杂度基于词汇数量和特殊词汇
	complexity := float64(len(words))

	// 检查复杂词汇
	for _, word := range words {
		if len(word) > 6 { // 长词增加复杂度
			complexity += 0.5
		}
	}

	return complexity / 10.0 // 标准化到合理范围
}

func (dqe *DefaultQualityEvaluator) calculateConfidence(results []RetrievalResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	// 基于分数分布计算置信度
	totalScore := 0.0
	maxScore := 0.0
	for _, result := range results {
		totalScore += result.Score
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}

	avgScore := totalScore / float64(len(results))

	// 置信度基于平均分和最高分
	confidence := (avgScore + maxScore) / 2.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

func (dqe *DefaultQualityEvaluator) calculateAverageScore(results []RetrievalResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	total := 0.0
	for _, result := range results {
		total += result.Score
	}

	return total / float64(len(results))
}

func (dqe *DefaultQualityEvaluator) identifyIssues(relevance, diversity, completeness float64, results []RetrievalResult) []QualityIssue {
	issues := make([]QualityIssue, 0)

	if relevance < 0.4 {
		issues = append(issues, QualityIssue{
			Type:        "low_relevance",
			Severity:    "high",
			Description: fmt.Sprintf("相关性过低: %.2f", relevance),
			Score:       relevance,
		})
	}

	if diversity < 0.3 {
		issues = append(issues, QualityIssue{
			Type:        "low_diversity",
			Severity:    "medium",
			Description: fmt.Sprintf("结果多样性不足: %.2f", diversity),
			Score:       diversity,
		})
	}

	if completeness < 0.5 {
		issues = append(issues, QualityIssue{
			Type:        "incompleteness",
			Severity:    "medium",
			Description: fmt.Sprintf("结果不够完整: %.2f", completeness),
			Score:       completeness,
		})
	}

	if len(results) < 3 {
		issues = append(issues, QualityIssue{
			Type:        "insufficient_results",
			Severity:    "high",
			Description: fmt.Sprintf("结果数量不足: %d", len(results)),
			Score:       float64(len(results)) / 10.0,
		})
	}

	return issues
}

// shouldContinueImproving 判断是否应该继续改进
func (ir *IterativeRetriever) shouldContinueImproving(assessment *QualityAssessment, iteration int) bool {
	// 如果质量过低，继续尝试
	if assessment.OverallScore < 0.3 {
		return true
	}

	// 如果还有明显的改进空间
	if len(assessment.Issues) > 0 {
		for _, issue := range assessment.Issues {
			if issue.Severity == "high" {
				return true
			}
		}
	}

	// 如果是早期迭代且质量中等，继续尝试
	if iteration < ir.config.MaxIterations/2 && assessment.OverallScore < 0.7 {
		return true
	}

	return false
}

// selectBestSuggestion 选择最佳改进建议
func (ir *IterativeRetriever) selectBestSuggestion(suggestions []ImprovementSuggestion) ImprovementSuggestion {
	if len(suggestions) == 0 {
		return ImprovementSuggestion{
			Type:        "no_suggestion",
			Priority:    0,
			Description: "无改进建议",
		}
	}

	// 选择优先级最高且预期收益最大的建议
	bestSuggestion := suggestions[0]
	bestScore := float64(bestSuggestion.Priority) * bestSuggestion.ExpectedGain

	for _, suggestion := range suggestions[1:] {
		score := float64(suggestion.Priority) * suggestion.ExpectedGain
		if score > bestScore {
			bestSuggestion = suggestion
			bestScore = score
		}
	}

	return bestSuggestion
}

// applyImprovement 应用改进建议
func (ir *IterativeRetriever) applyImprovement(ctx context.Context, currentQuery string, suggestion ImprovementSuggestion) (string, error) {
	switch suggestion.Type {
	case "query_rewrite":
		if ir.queryRewriter != nil {
			result, err := ir.queryRewriter.ProcessQuery(ctx, currentQuery)
			if err != nil {
				return currentQuery, err
			}
			return result.RewrittenQuery, nil
		}
		return currentQuery, nil

	case "expand_search":
		// 简化实现：添加相关术语
		expansion := " 相关 相似 关联"
		return currentQuery + expansion, nil

	case "refine_query":
		// 简化实现：优化查询表达
		refined := currentQuery + " 详细 具体"
		return refined, nil

	case "query_decomposition":
		// 简化实现：添加分解关键词
		decomposed := currentQuery + " 原理 方法 步骤"
		return decomposed, nil

	default:
		return currentQuery, nil
	}
}

// updateStats 更新统计信息
func (ir *IterativeRetriever) updateStats(result *IterativeSearchResult) {
	ir.stats.TotalSearches++

	if result.Success {
		ir.stats.SuccessfulSearches++
	}

	// 更新平均迭代次数
	ir.stats.AverageIterations = (ir.stats.AverageIterations*float64(ir.stats.TotalSearches-1) +
		float64(result.IterationCount)) / float64(ir.stats.TotalSearches)

	// 更新平均质量
	if result.FinalQuality != nil {
		ir.stats.AverageQuality = (ir.stats.AverageQuality*float64(ir.stats.TotalSearches-1) +
			result.FinalQuality.OverallScore) / float64(ir.stats.TotalSearches)
	}

	// 更新平均时间
	totalTime := int64(ir.stats.AverageTime)*(ir.stats.TotalSearches-1) + int64(result.TotalTime)
	ir.stats.AverageTime = time.Duration(totalTime / ir.stats.TotalSearches)

	// 检查质量改进
	if len(result.QualityHistory) > 1 {
		firstQuality := result.QualityHistory[0].OverallScore
		lastQuality := result.QualityHistory[len(result.QualityHistory)-1].OverallScore
		if lastQuality > firstQuality {
			ir.stats.QualityImprovements++
		}
	}
}

// 公共方法

func (ir *IterativeRetriever) SetEnabled(enabled bool) {
	ir.enabled = enabled
}

func (ir *IterativeRetriever) IsEnabled() bool {
	return ir.enabled
}

func (ir *IterativeRetriever) GetStats() *IterativeStats {
	return ir.stats
}

func (ir *IterativeRetriever) GetConfig() *IterativeRetrieverConfig {
	return ir.config
}

// 辅助函数

func extractWords(text string) []string {
	// 简化实现：基于空格分词
	words := make([]string, 0)
	for _, word := range splitWords(text) {
		if len(word) > 2 { // 过滤短词
			words = append(words, toLowerCase(word))
		}
	}
	return words
}

func splitWords(text string) []string {
	// 简化的分词实现
	result := make([]string, 0)
	current := ""

	for _, char := range text {
		if char == ' ' || char == '\t' || char == '\n' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

func toLowerCase(text string) string {
	// 简化的小写转换
	result := ""
	for _, char := range text {
		if char >= 'A' && char <= 'Z' {
			result += string(char - 'A' + 'a')
		} else {
			result += string(char)
		}
	}
	return result
}

type IterativeRetrievalLog struct {
	OriginalQuery       string                  `json:"original_query"`
	FinalQuery          string                  `json:"final_query"`
	IterationSteps      []IterationStepLog      `json:"iteration_steps"`
	TotalIterations     int                     `json:"total_iterations"`
	FinalQualityScore   QualityScore            `json:"final_quality_score"`
	QualityImprovement  float64                 `json:"quality_improvement"`
	TotalProcessingTime time.Duration           `json:"total_processing_time"`
	Timestamp           time.Time               `json:"timestamp"`
	RetrievalStats      IterativeRetrievalStats `json:"retrieval_stats"`
}

type IterationStepLog struct {
	Iteration              int                     `json:"iteration"`
	Query                  string                  `json:"query"`
	RetrievedDocuments     []DocumentResult        `json:"retrieved_documents"`
	QualityScore           QualityScore            `json:"quality_score"`
	ImprovementSuggestions []ImprovementSuggestion `json:"improvement_suggestions"`
	ProcessingTime         time.Duration           `json:"processing_time"`
	Metadata               map[string]interface{}  `json:"metadata"`
}

type DocumentResult struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Content      string  `json:"content"`
	Score        float64 `json:"score"`
	Source       string  `json:"source"`
	RelevanceTag string  `json:"relevance_tag"` // "高相关", "中等相关", "低相关"
}

type QualityScore struct {
	Relevance    float64 `json:"relevance"`
	Diversity    float64 `json:"diversity"`
	Completeness float64 `json:"completeness"`
	Overall      float64 `json:"overall"`
}

type IterativeRetrievalStats struct {
	TotalDocuments       int64   `json:"total_documents"`
	UniqueDocuments      int64   `json:"unique_documents"`
	HighRelevanceCount   int64   `json:"high_relevance_count"`
	MediumRelevanceCount int64   `json:"medium_relevance_count"`
	LowRelevanceCount    int64   `json:"low_relevance_count"`
	AverageScore         float64 `json:"average_score"`
	SuccessRate          float64 `json:"success_rate"`
}

// 添加详细日志输出的检索方法
func (ir *IterativeRetriever) SearchWithDetailedLogging(ctx context.Context, originalQuery string, retriever func(string) ([]RetrievalResult, error)) (*IterativeSearchResult, error) {
	startTime := time.Now()

	// 调用原有的Search方法
	result, err := ir.Search(ctx, originalQuery, retriever)
	if err != nil {
		return result, err
	}

	// 创建详细日志
	log := &IterativeRetrievalLog{
		OriginalQuery:       originalQuery,
		FinalQuery:          ir.extractFinalQuery(result),
		TotalIterations:     result.IterationCount,
		TotalProcessingTime: time.Since(startTime),
		Timestamp:           startTime,
		IterationSteps:      make([]IterationStepLog, 0),
	}

	// 转换迭代步骤日志
	for i, query := range result.QueryHistory {
		stepLog := IterationStepLog{
			Iteration:              i + 1,
			Query:                  query,
			RetrievedDocuments:     []DocumentResult{},        // 简化版本
			QualityScore:           QualityScore{},            // 简化版本
			ImprovementSuggestions: []ImprovementSuggestion{}, // 简化版本
			ProcessingTime:         time.Duration(0),          // 简化版本
			Metadata:               make(map[string]interface{}),
		}

		log.IterationSteps = append(log.IterationSteps, stepLog)
	}

	// 计算质量改进
	if len(log.IterationSteps) > 0 {
		log.FinalQualityScore = log.IterationSteps[len(log.IterationSteps)-1].QualityScore
		initialQuality := log.IterationSteps[0].QualityScore.Overall
		log.QualityImprovement = log.FinalQualityScore.Overall - initialQuality
	}

	// 生成统计信息
	log.RetrievalStats = ir.generateRetrievalStats(log)

	// 输出详细对比日志
	ir.printIterativeRetrievalComparison(log)

	return result, nil
}

func (ir *IterativeRetriever) printIterativeRetrievalComparison(log *IterativeRetrievalLog) {
	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("🔄 ITERATIVE RETRIEVAL ANALYSIS - 迭代检索优化分析")
	fmt.Println(strings.Repeat("=", 100))

	// 1. 原始查询和检索设置
	fmt.Println("\n📝 1. RETRIEVAL SETUP - 检索设置")
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("原始查询: %s\n", log.OriginalQuery)
	fmt.Printf("最终查询: %s\n", log.FinalQuery)
	fmt.Printf("开始时间: %v\n", log.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("总处理时间: %v\n", log.TotalProcessingTime)
	fmt.Printf("总迭代次数: %d\n", log.TotalIterations)

	// 查询演变分析
	if log.OriginalQuery != log.FinalQuery {
		fmt.Printf("查询演变: %s → %s\n", log.OriginalQuery, log.FinalQuery)
		querySimilarity := ir.calculateQuerySimilarity(log.OriginalQuery, log.FinalQuery)
		fmt.Printf("查询相似度: %.3f", querySimilarity)
		if querySimilarity > 0.8 {
			fmt.Println(" ✅ (保持原意)")
		} else if querySimilarity > 0.6 {
			fmt.Println(" ⚪ (适度优化)")
		} else {
			fmt.Println(" ⚠️ (显著变化)")
		}
	} else {
		fmt.Println("查询保持不变")
	}

	// 2. 迭代过程详情
	fmt.Println("\n🔄 2. ITERATION DETAILS - 迭代过程详情")
	fmt.Println(strings.Repeat("-", 80))

	for i, step := range log.IterationSteps {
		fmt.Printf("\n📍 迭代 %d (耗时: %v)\n", i+1, step.ProcessingTime)
		fmt.Printf("  查询: %s\n", step.Query)
		fmt.Printf("  检索文档数: %d\n", len(step.RetrievedDocuments))

		// 质量分数详情
		fmt.Printf("  质量评分:\n")
		fmt.Printf("    - 相关性: %.3f\n", step.QualityScore.Relevance)
		fmt.Printf("    - 多样性: %.3f\n", step.QualityScore.Diversity)
		fmt.Printf("    - 完整性: %.3f\n", step.QualityScore.Completeness)
		fmt.Printf("    - 总分: %.3f\n", step.QualityScore.Overall)

		// 文档预览（前3个）
		if len(step.RetrievedDocuments) > 0 {
			fmt.Println("  📄 检索文档预览:")
			previewCount := min(3, len(step.RetrievedDocuments))
			for j := 0; j < previewCount; j++ {
				doc := step.RetrievedDocuments[j]
				fmt.Printf("    %d. [%s] %s (评分: %.3f)\n",
					j+1, doc.RelevanceTag, truncateString(doc.Title, 50), doc.Score)
			}
			if len(step.RetrievedDocuments) > 3 {
				fmt.Printf("    ... 还有 %d 个文档\n", len(step.RetrievedDocuments)-3)
			}
		}

		// 改进建议
		if len(step.ImprovementSuggestions) > 0 {
			fmt.Println("  💡 改进建议:")
			for _, suggestion := range step.ImprovementSuggestions {
				fmt.Printf("    • %s (预期收益: %.2f) - %s\n",
					suggestion.Type, suggestion.ExpectedGain, suggestion.Description)
			}
		}

		// 终止原因
		if reason, exists := step.Metadata["termination_reason"].(string); exists {
			fmt.Printf("  🏁 终止原因: %s\n", ir.translateTerminationReason(reason))
		}

		// 查询改进状态
		if improved, exists := step.Metadata["query_improved"].(bool); exists {
			if improved {
				if newQuery, exists := step.Metadata["new_query"].(string); exists {
					fmt.Printf("  ✅ 查询已优化 → %s\n", newQuery)
				}
			} else {
				fmt.Println("  ⚪ 查询未改进")
			}
		}
	}

	// 3. 质量改进分析
	fmt.Println("\n📊 3. QUALITY IMPROVEMENT - 质量改进分析")
	fmt.Println(strings.Repeat("-", 80))

	if len(log.IterationSteps) > 1 {
		firstQuality := log.IterationSteps[0].QualityScore
		finalQuality := log.FinalQualityScore

		fmt.Printf("初始质量: %.3f\n", firstQuality.Overall)
		fmt.Printf("最终质量: %.3f\n", finalQuality.Overall)
		fmt.Printf("质量提升: %+.3f", log.QualityImprovement)

		if log.QualityImprovement > 0.1 {
			fmt.Println(" ✅ (显著改进)")
		} else if log.QualityImprovement > 0 {
			fmt.Println(" ⚪ (轻微改进)")
		} else {
			fmt.Println(" ❌ (无改进或退化)")
		}

		// 各维度改进
		fmt.Println("\n分维度改进:")
		fmt.Printf("  相关性: %.3f → %.3f (%+.3f)\n",
			firstQuality.Relevance, finalQuality.Relevance,
			finalQuality.Relevance-firstQuality.Relevance)
		fmt.Printf("  多样性: %.3f → %.3f (%+.3f)\n",
			firstQuality.Diversity, finalQuality.Diversity,
			finalQuality.Diversity-firstQuality.Diversity)
		fmt.Printf("  完整性: %.3f → %.3f (%+.3f)\n",
			firstQuality.Completeness, finalQuality.Completeness,
			finalQuality.Completeness-firstQuality.Completeness)
	}

	// 4. 检索统计
	fmt.Println("\n📈 4. RETRIEVAL STATISTICS - 检索统计")
	fmt.Println(strings.Repeat("-", 80))

	stats := log.RetrievalStats
	fmt.Printf("总检索文档数: %d\n", stats.TotalDocuments)
	fmt.Printf("去重后文档数: %d\n", stats.UniqueDocuments)
	fmt.Printf("高相关文档数: %d\n", stats.HighRelevanceCount)
	fmt.Printf("中等相关文档数: %d\n", stats.MediumRelevanceCount)
	fmt.Printf("低相关文档数: %d\n", stats.LowRelevanceCount)
	fmt.Printf("平均文档评分: %.3f\n", stats.AverageScore)
	fmt.Printf("迭代成功率: %.1f%%\n", stats.SuccessRate*100)

	// 5. 性能分析
	fmt.Println("\n⚡ 5. PERFORMANCE ANALYSIS - 性能分析")
	fmt.Println(strings.Repeat("-", 80))

	fmt.Printf("平均每次迭代耗时: %v\n", log.TotalProcessingTime/time.Duration(log.TotalIterations))
	fmt.Printf("检索效率: %.2f 文档/秒\n", float64(stats.TotalDocuments)/log.TotalProcessingTime.Seconds())

	// 检索质量趋势
	if len(log.IterationSteps) > 1 {
		fmt.Println("\n质量趋势:")
		for i, step := range log.IterationSteps {
			bar := ir.generateQualityBar(step.QualityScore.Overall)
			fmt.Printf("  迭代%d: %s %.3f\n", i+1, bar, step.QualityScore.Overall)
		}
	}

	// 6. 优化效果总结
	fmt.Println("\n🎯 6. OPTIMIZATION SUMMARY - 优化效果总结")
	fmt.Println(strings.Repeat("-", 80))

	effectiveness := ir.evaluateRetrievalEffectiveness(log)
	fmt.Printf("检索效果: %s\n", effectiveness.Overall)
	fmt.Printf("查询优化: %s\n", effectiveness.QueryOptimization)
	fmt.Printf("文档质量: %s\n", effectiveness.DocumentQuality)
	fmt.Printf("迭代效率: %s\n", effectiveness.IterationEfficiency)

	if len(effectiveness.Recommendations) > 0 {
		fmt.Println("\n💡 优化建议:")
		for _, rec := range effectiveness.Recommendations {
			fmt.Printf("  • %s\n", rec)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 100))
}

// convertToDocumentResults 函数已删除，因为未被使用

func (ir *IterativeRetriever) calculateQuerySimilarity(query1, query2 string) float64 {
	// 简单的查询相似度计算
	words1 := strings.Fields(strings.ToLower(query1))
	words2 := strings.Fields(strings.ToLower(query2))

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}

	set1 := make(map[string]bool)
	for _, word := range words1 {
		set1[word] = true
	}

	common := 0
	for _, word := range words2 {
		if set1[word] {
			common++
		}
	}

	return float64(common*2) / float64(len(words1)+len(words2))
}

func (ir *IterativeRetriever) translateTerminationReason(reason string) string {
	translations := map[string]string{
		"quality_threshold_reached":  "达到质量阈值",
		"no_more_improvements":       "无更多改进",
		"no_improvement_suggestions": "无改进建议",
		"max_iterations_reached":     "达到最大迭代次数",
		"timeout":                    "处理超时",
	}

	if translated, exists := translations[reason]; exists {
		return translated
	}
	return reason
}

func (ir *IterativeRetriever) generateQualityBar(score float64) string {
	// 生成质量分数的可视化条形图
	barLength := 20
	filledLength := int(score * float64(barLength))

	bar := strings.Repeat("█", filledLength) + strings.Repeat("░", barLength-filledLength)
	return fmt.Sprintf("[%s]", bar)
}

type RetrievalEffectiveness struct {
	Overall             string   `json:"overall"`
	QueryOptimization   string   `json:"query_optimization"`
	DocumentQuality     string   `json:"document_quality"`
	IterationEfficiency string   `json:"iteration_efficiency"`
	Recommendations     []string `json:"recommendations"`
}

func (ir *IterativeRetriever) evaluateRetrievalEffectiveness(log *IterativeRetrievalLog) RetrievalEffectiveness {
	effectiveness := RetrievalEffectiveness{
		Recommendations: make([]string, 0),
	}

	// 整体效果评价
	if log.QualityImprovement > 0.15 {
		effectiveness.Overall = "优秀 ✅"
	} else if log.QualityImprovement > 0.05 {
		effectiveness.Overall = "良好 ⚪"
	} else {
		effectiveness.Overall = "需改进 ⚠️"
		effectiveness.Recommendations = append(effectiveness.Recommendations, "考虑调整检索策略或质量评估标准")
	}

	// 查询优化评价
	querySimilarity := ir.calculateQuerySimilarity(log.OriginalQuery, log.FinalQuery)
	if querySimilarity > 0.8 && log.FinalQuery != log.OriginalQuery {
		effectiveness.QueryOptimization = "优秀 ✅"
	} else if log.FinalQuery != log.OriginalQuery {
		effectiveness.QueryOptimization = "良好 ⚪"
	} else {
		effectiveness.QueryOptimization = "无优化 ⚪"
		effectiveness.Recommendations = append(effectiveness.Recommendations, "查询改写组件可能需要调优")
	}

	// 文档质量评价
	if log.FinalQualityScore.Overall > 0.8 {
		effectiveness.DocumentQuality = "优秀 ✅"
	} else if log.FinalQualityScore.Overall > 0.6 {
		effectiveness.DocumentQuality = "良好 ⚪"
	} else {
		effectiveness.DocumentQuality = "需改进 ⚠️"
		effectiveness.Recommendations = append(effectiveness.Recommendations, "文档库质量或检索策略需要优化")
	}

	// 迭代效率评价
	avgIterationTime := log.TotalProcessingTime / time.Duration(log.TotalIterations)
	if avgIterationTime < 500*time.Millisecond && log.TotalIterations <= 3 {
		effectiveness.IterationEfficiency = "高效 ✅"
	} else if avgIterationTime < 1*time.Second && log.TotalIterations <= 5 {
		effectiveness.IterationEfficiency = "适中 ⚪"
	} else {
		effectiveness.IterationEfficiency = "较慢 ⚠️"
		effectiveness.Recommendations = append(effectiveness.Recommendations, "考虑优化检索性能或减少迭代次数")
	}

	return effectiveness
}

func (ir *IterativeRetriever) generateRetrievalStats(log *IterativeRetrievalLog) IterativeRetrievalStats {
	stats := IterativeRetrievalStats{}

	// 统计文档信息
	documentMap := make(map[string]bool)
	var totalScore float64
	var docCount int

	for _, step := range log.IterationSteps {
		for _, doc := range step.RetrievedDocuments {
			// 统计唯一文档
			if !documentMap[doc.ID] {
				documentMap[doc.ID] = true
				stats.UniqueDocuments++
			}

			// 统计总文档数和评分
			stats.TotalDocuments++
			totalScore += doc.Score
			docCount++

			// 按相关性分类
			switch doc.RelevanceTag {
			case "高相关":
				stats.HighRelevanceCount++
			case "中等相关":
				stats.MediumRelevanceCount++
			case "低相关":
				stats.LowRelevanceCount++
			}
		}
	}

	// 计算平均分数
	if docCount > 0 {
		stats.AverageScore = totalScore / float64(docCount)
	}

	// 计算成功率（有改进的迭代数 / 总迭代数）
	successfulIterations := 0
	for i := 1; i < len(log.IterationSteps); i++ {
		if log.IterationSteps[i].QualityScore.Overall > log.IterationSteps[i-1].QualityScore.Overall {
			successfulIterations++
		}
	}

	if len(log.IterationSteps) > 1 {
		stats.SuccessRate = float64(successfulIterations) / float64(len(log.IterationSteps)-1)
	} else {
		stats.SuccessRate = 1.0
	}

	return stats
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractFinalQuery 从迭代结果中提取最终查询
func (ir *IterativeRetriever) extractFinalQuery(result *IterativeSearchResult) string {
	if len(result.QueryHistory) > 0 {
		return result.QueryHistory[len(result.QueryHistory)-1]
	}
	return ""
}

// convertToRetrievalResults 转换检索结果
func (ir *IterativeRetriever) convertToRetrievalResults(results []RetrievalResult) []RetrievalResult {
	return results
}

// convertQualityAssessment 转换质量评估结果
func (ir *IterativeRetriever) convertQualityAssessment(assessment *QualityAssessment) float64 {
	if assessment != nil {
		return assessment.OverallScore
	}
	return 0.0
}

func (ir *IterativeRetriever) calculateInitialQuality(query string) float64 {
	// 简单的初始质量估算
	score := 0.3 // 基础分数

	words := strings.Fields(query)
	if len(words) > 3 {
		score += 0.2
	}

	// 可以添加更多质量评估逻辑
	return score
}
