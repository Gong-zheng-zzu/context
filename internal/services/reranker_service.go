package services

import (
	"context"
	"log"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/models"
)

// RerankerService 检索结果重排序服务
type RerankerService struct {
	llmClient llm.LLMClient
}

// NewRerankerService 创建重排序服务
func NewRerankerService(llmClient llm.LLMClient) *RerankerService {
	return &RerankerService{
		llmClient: llmClient,
	}
}

// RerankConfig 重排序配置
type RerankConfig struct {
	SemanticWeight float64 // 语义相似度权重
	KeywordWeight  float64 // 关键词匹配权重
	RecencyWeight  float64 // 时间新鲜度权重
	EntityWeight   float64 // 实体匹配权重
	UseEntityBoost bool    // 是否启用实体提升
	UseLLMRerank   bool    // 是否使用LLM重排序
}

// DefaultRerankConfig 默认配置
var DefaultRerankConfig = RerankConfig{
	SemanticWeight: 0.5,
	KeywordWeight:  0.3,
	RecencyWeight:  0.1,
	EntityWeight:   0.1,
	UseEntityBoost: true,
	UseLLMRerank:   false, // 默认关闭LLM重排序（性能考虑）
}

// QueryIntent 查询意图类型
type QueryIntent struct {
	Type           string   // personal_info, technical, project, general
	Keywords       []string // 关键词
	Entities       []string // 实体（人名、地名等）
	IsQuestionOnly bool     // 是否只是问题本身（需要过滤）
}

// Rerank 重排序检索结果
func (r *RerankerService) Rerank(ctx context.Context, query string, results []models.SearchResult, config RerankConfig) ([]models.SearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	startTime := time.Now()
	log.Printf("[重排序] 开始重排序，原始结果数: %d", len(results))

	// 1. 分析查询意图
	intent := r.analyzeQueryIntent(query)
	log.Printf("[重排序] 查询意图: type=%s, keywords=%v, entities=%v", intent.Type, intent.Keywords, intent.Entities)

	// 2. 过滤掉"问题本身"类型的记忆
	filteredResults := r.filterQuestionOnlyResults(results, intent)
	log.Printf("[重排序] 过滤后结果数: %d", len(filteredResults))

	if len(filteredResults) == 0 {
		log.Printf("[重排序] 警告: 过滤后无结果，返回原始结果")
		return results, nil
	}

	// 3. 计算多维度分数
	scoredResults := make([]ScoredResult, 0, len(filteredResults))
	for _, result := range filteredResults {
		scored := r.calculateMultiDimensionalScore(query, result, intent, config)
		scoredResults = append(scoredResults, scored)
	}

	// 4. 按最终分数排序
	r.sortByFinalScore(scoredResults)

	// 5. 转换回原始格式
	rerankedResults := make([]models.SearchResult, 0, len(scoredResults))
	for _, scored := range scoredResults {
		// 更新分数为最终分数
		scored.Result.Score = scored.FinalScore
		rerankedResults = append(rerankedResults, scored.Result)
	}

	log.Printf("[重排序] 完成，耗时: %v", time.Since(startTime))
	r.logTopResults(rerankedResults, 5)

	return rerankedResults, nil
}

// ScoredResult 带多维度分数的结果
type ScoredResult struct {
	Result         models.SearchResult
	SemanticScore  float64 // 语义相似度分数
	KeywordScore   float64 // 关键词匹配分数
	RecencyScore   float64 // 时间新鲜度分数
	EntityScore    float64 // 实体匹配分数
	FinalScore     float64 // 最终加权分数
	HasAnswer      bool    // 是否包含答案（而非问题）
	IsQuestionOnly bool    // 是否只是问题
}

// analyzeQueryIntent 分析查询意图
func (r *RerankerService) analyzeQueryIntent(query string) QueryIntent {
	intent := QueryIntent{
		Type:     "general",
		Keywords: []string{},
		Entities: []string{},
	}

	queryLower := strings.ToLower(query)

	// 检测个人信息查询
	personalPatterns := []string{
		"我是谁", "我的名字", "我的信息", "我叫什么", "个人信息",
		"我的电话", "我的邮箱", "我的身份证", "我的地址",
	}
	for _, pattern := range personalPatterns {
		if strings.Contains(queryLower, pattern) {
			intent.Type = "personal_info"
			intent.Keywords = append(intent.Keywords, "姓名", "名字", "电话", "邮箱", "身份证", "地址")
			break
		}
	}

	// 检测技术查询
	technicalPatterns := []string{
		"技术栈", "用什么", "什么语言", "什么框架", "什么数据库",
		"怎么实现", "如何", "代码", "开发", "项目",
	}
	for _, pattern := range technicalPatterns {
		if strings.Contains(queryLower, pattern) {
			intent.Type = "technical"
			intent.Keywords = append(intent.Keywords, "Go", "Python", "Java", "数据库", "框架", "API")
			break
		}
	}

	// 检测是否只是问题（需要过滤）
	questionOnlyPatterns := []string{
		"^我是谁[？?]?$",
		"^我的.*是什么[？?]?$",
		"^.*吗[？?]?$",
	}
	for _, pattern := range questionOnlyPatterns {
		if matched, _ := regexp.MatchString(pattern, query); matched {
			intent.IsQuestionOnly = true
			break
		}
	}

	return intent
}

// filterQuestionOnlyResults 过滤掉"只是问题"的结果
func (r *RerankerService) filterQuestionOnlyResults(results []models.SearchResult, intent QueryIntent) []models.SearchResult {
	filtered := make([]models.SearchResult, 0, len(results))

	for _, result := range results {
		content, ok := result.Fields["content"].(string)
		if !ok {
			continue
		}

		// 检查是否是"问题本身"
		if r.isQuestionOnly(content) {
			log.Printf("[重排序] 过滤问题: %s", content)
			continue
		}

		filtered = append(filtered, result)
	}

	return filtered
}

// isQuestionOnly 判断内容是否只是问题
func (r *RerankerService) isQuestionOnly(content string) bool {
	// 问题特征
	questionPatterns := []string{
		`^我是谁[？?]?`,
		`^我的.*是什么[？?]?`,
		`^.*吗[？?]?$`,
		`^你知道.*吗[？?]?$`,
		`^刚才.*什么[？?]?$`,
		`^之前.*什么[？?]?$`,
	}

	for _, pattern := range questionPatterns {
		if matched, _ := regexp.MatchString(pattern, content); matched {
			// 检查是否包含答案（如果包含"是"、"为"等，可能是陈述句）
			if !strings.Contains(content, "是") && !strings.Contains(content, "为") {
				return true
			}
		}
	}

	return false
}

// calculateMultiDimensionalScore 计算多维度分数
func (r *RerankerService) calculateMultiDimensionalScore(query string, result models.SearchResult, intent QueryIntent, config RerankConfig) ScoredResult {
	scored := ScoredResult{
		Result:        result,
		SemanticScore: result.Score, // 原始语义相似度
	}

	content, ok := result.Fields["content"].(string)
	if !ok {
		scored.FinalScore = result.Score
		return scored
	}

	// 1. 关键词匹配分数
	scored.KeywordScore = r.calculateKeywordScore(query, content, intent)

	// 2. 实体匹配分数
	scored.EntityScore = r.calculateEntityScore(content, intent)

	// 3. 时间新鲜度分数
	scored.RecencyScore = r.calculateRecencyScore(result)

	// 4. 判断是否包含答案
	scored.HasAnswer = r.hasAnswer(content, intent)
	scored.IsQuestionOnly = r.isQuestionOnly(content)

	// 5. 计算最终加权分数
	scored.FinalScore = config.SemanticWeight*scored.SemanticScore +
		config.KeywordWeight*scored.KeywordScore +
		config.RecencyWeight*scored.RecencyScore +
		config.EntityWeight*scored.EntityScore

	// 6. 答案提升（如果包含答案，提升分数）
	if scored.HasAnswer && !scored.IsQuestionOnly {
		scored.FinalScore *= 1.5 // 提升50%
	}

	// 7. 实体提升（如果是个人信息查询且包含实体）
	if config.UseEntityBoost && intent.Type == "personal_info" && scored.EntityScore > 0.5 {
		scored.FinalScore *= 1.3 // 提升30%
	}

	return scored
}

// calculateKeywordScore 计算关键词匹配分数
func (r *RerankerService) calculateKeywordScore(query, content string, intent QueryIntent) float64 {
	if len(intent.Keywords) == 0 {
		return 0.0
	}

	contentLower := strings.ToLower(content)
	matchCount := 0

	for _, keyword := range intent.Keywords {
		if strings.Contains(contentLower, strings.ToLower(keyword)) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(intent.Keywords))
}

// calculateEntityScore 计算实体匹配分数
func (r *RerankerService) calculateEntityScore(content string, intent QueryIntent) float64 {
	// 检测常见实体模式
	entityPatterns := []string{
		`[一-龥]{2,4}`, // 中文姓名（2-4个字）
		`1[3-9]\d{9}`,          // 手机号
		`\w+@\w+\.\w+`,         // 邮箱
		`\d{15,18}`,            // 身份证号
	}

	score := 0.0
	for _, pattern := range entityPatterns {
		if matched, _ := regexp.MatchString(pattern, content); matched {
			score += 0.25
		}
	}

	return math.Min(score, 1.0)
}

// calculateRecencyScore 计算时间新鲜度分数
func (r *RerankerService) calculateRecencyScore(result models.SearchResult) float64 {
	timestamp, ok := result.Fields["timestamp"].(int64)
	if !ok {
		return 0.5 // 默认中等分数
	}

	// 计算时间衰减（越新分数越高）
	now := time.Now().Unix()
	ageSeconds := now - timestamp
	ageDays := float64(ageSeconds) / 86400.0

	// 使用指数衰减：score = e^(-age/30)
	// 30天内的记忆保持较高分数
	score := math.Exp(-ageDays / 30.0)
	return score
}

// hasAnswer 判断内容是否包含答案
func (r *RerankerService) hasAnswer(content string, intent QueryIntent) bool {
	// 如果是个人信息查询，检查是否包含陈述句
	if intent.Type == "personal_info" {
		// 包含"是"、"为"、"叫"等陈述词
		answerPatterns := []string{
			"我的.*是",
			"我的.*为",
			"我叫",
			"姓名是",
			"名字是",
		}
		for _, pattern := range answerPatterns {
			if matched, _ := regexp.MatchString(pattern, content); matched {
				return true
			}
		}
	}

	// 检查是否包含实体信息
	if r.calculateEntityScore(content, intent) > 0.5 {
		return true
	}

	return false
}

// sortByFinalScore 按最终分数排序
func (r *RerankerService) sortByFinalScore(results []ScoredResult) {
	// 使用冒泡排序（简单实现）
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if results[j].FinalScore < results[j+1].FinalScore {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}

// logTopResults 记录前N个结果
func (r *RerankerService) logTopResults(results []models.SearchResult, topN int) {
	if topN > len(results) {
		topN = len(results)
	}

	log.Printf("[重排序] Top %d 结果:", topN)
	for i := 0; i < topN; i++ {
		content, _ := results[i].Fields["content"].(string)
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		log.Printf("  %d. [分数:%.4f] %s", i+1, results[i].Score, content)
	}
}
