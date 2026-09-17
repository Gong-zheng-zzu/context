package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/llm"
	"github.com/contextkeeper/service/internal/models"
)

// RerankerService 检索结果重排序服务
type RerankerService struct {
	llmClient llm.LLMClient
	// llmClientFactory 惰性LLM客户端工厂，仅在 UseLLMRerank=true 首次触发时调用，
	// 避免构造期写入LLM全局配置（未启用时行为与改动前一致）
	llmClientFactory func() llm.LLMClient
	llmClientOnce    sync.Once
}

// NewRerankerService 创建重排序服务（直接注入已有客户端）
func NewRerankerService(llmClient llm.LLMClient) *RerankerService {
	return &RerankerService{
		llmClient: llmClient,
	}
}

// NewRerankerServiceWithFactory 使用惰性工厂创建重排序服务。
// 工厂仅在 UseLLMRerank=true 首次调用 Rerank 时才被触发，
// 因此未启用LLM重排序时不会创建客户端、也不会写入LLM全局状态。
func NewRerankerServiceWithFactory(factory func() llm.LLMClient) *RerankerService {
	return &RerankerService{
		llmClientFactory: factory,
	}
}

// resolveLLMClient 返回LLM客户端，必要时通过惰性工厂创建（并发安全）。
// 工厂为nil或返回nil时返回nil，调用方据此降级为纯规则打分。
func (r *RerankerService) resolveLLMClient() llm.LLMClient {
	if r.llmClientFactory == nil {
		return r.llmClient
	}
	r.llmClientOnce.Do(func() {
		if r.llmClient == nil {
			r.llmClient = r.llmClientFactory()
		}
	})
	return r.llmClient
}

// 重排序可配置常量（默认值，保持改动前的行为不变）
const (
	// defaultLLMFusionWeight LLM分数与规则分数的默认融合权重
	defaultLLMFusionWeight = 0.6
	// defaultAnswerBoostMultiplier 命中答案时的默认分数提升倍数（提升50%）
	defaultAnswerBoostMultiplier = 1.5
	// defaultEntityBoostMultiplier 个人信息查询命中实体时的默认分数提升倍数（提升30%）
	defaultEntityBoostMultiplier = 1.3
	// defaultLLMMaxDocChars 送入LLM打分的单个候选正文默认最大字符数
	defaultLLMMaxDocChars = 500
)

// RerankConfig 重排序配置
type RerankConfig struct {
	SemanticWeight float64 // 语义相似度权重
	KeywordWeight  float64 // 关键词匹配权重
	RecencyWeight  float64 // 时间新鲜度权重
	EntityWeight   float64 // 实体匹配权重
	UseEntityBoost bool    // 是否启用实体提升
	UseLLMRerank   bool    // 是否使用LLM重排序

	// LLMFusionWeight LLM相关性分数与规则分数的融合权重（0, 1]：
	// 最终分数 = (1-LLMFusionWeight)*规则分数 + LLMFusionWeight*LLM分数。
	// <=0 或 >1 时回退到默认值 0.6。仅在 UseLLMRerank 为 true 时生效。
	LLMFusionWeight float64

	// AnswerBoostMultiplier 命中答案（非问题本身）时的分数提升倍数，<=0 时回退到默认值 1.5
	AnswerBoostMultiplier float64
	// EntityBoostMultiplier 个人信息查询命中实体（EntityScore>0.5）时的分数提升倍数，<=0 时回退到默认值 1.3
	EntityBoostMultiplier float64

	// LLMMaxDocChars 送入LLM打分的单个候选正文最大字符数，<=0 时回退到默认值 500（用于成本调优）
	LLMMaxDocChars int
}

// DefaultRerankConfig 默认配置
var DefaultRerankConfig = RerankConfig{
	SemanticWeight: 0.5,
	KeywordWeight:  0.3,
	RecencyWeight:  0.1,
	EntityWeight:   0.1,
	UseEntityBoost: true,
	UseLLMRerank:   false, // 默认关闭LLM重排序（性能考虑）

	LLMFusionWeight:       defaultLLMFusionWeight,
	AnswerBoostMultiplier: defaultAnswerBoostMultiplier,
	EntityBoostMultiplier: defaultEntityBoostMultiplier,
	LLMMaxDocChars:        defaultLLMMaxDocChars,
}

// effectiveLLMFusionWeight 返回可用的LLM融合权重，非法配置回退到默认值
func effectiveLLMFusionWeight(configured float64) float64 {
	if configured <= 0 || configured > 1 {
		return defaultLLMFusionWeight
	}
	return configured
}

// effectiveBoostMultiplier 返回可用的提升倍数，非法配置回退到默认值
func effectiveBoostMultiplier(configured, fallback float64) float64 {
	if configured <= 0 {
		return fallback
	}
	return configured
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

	// 3.5 LLM重排序（可选）：用LLM相关性分数与规则分数融合；
	// LLM客户端缺失或调用/解析失败时降级为纯规则打分，并记录降级原因。
	if config.UseLLMRerank {
		r.applyLLMRerank(ctx, query, scoredResults, config)
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

	// 6. 答案提升（如果包含答案，提升分数，倍数可配置，默认1.5倍）
	if scored.HasAnswer && !scored.IsQuestionOnly {
		scored.FinalScore *= effectiveBoostMultiplier(config.AnswerBoostMultiplier, defaultAnswerBoostMultiplier)
	}

	// 7. 实体提升（如果是个人信息查询且包含实体，倍数可配置，默认1.3倍）
	if config.UseEntityBoost && intent.Type == "personal_info" && scored.EntityScore > 0.5 {
		scored.FinalScore *= effectiveBoostMultiplier(config.EntityBoostMultiplier, defaultEntityBoostMultiplier)
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
		`[一-龥]{2,4}`,   // 中文姓名（2-4个字）
		`1[3-9]\d{9}`,  // 手机号
		`\w+@\w+\.\w+`, // 邮箱
		`\d{15,18}`,    // 身份证号
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

// sortByFinalScore 按最终分数降序排序。
// 使用稳定排序，分数相同时保持候选的原始相对顺序，保证结果确定性。
func (r *RerankerService) sortByFinalScore(results []ScoredResult) {
	sort.SliceStable(results, func(left, right int) bool {
		return results[left].FinalScore > results[right].FinalScore
	})
}

// applyLLMRerank 使用LLM对候选结果做相关性打分，并与规则分数加权融合。
// 融合前先将规则分 min-max 归一化到[0,1]，与LLM分统一量纲，避免规则分超过1时稀释LLM权重。
// 任何LLM调用或解析失败都会降级为纯规则打分并记录降级原因，不影响主流程。
func (r *RerankerService) applyLLMRerank(ctx context.Context, query string, scoredResults []ScoredResult, config RerankConfig) {
	client := r.resolveLLMClient()
	if client == nil {
		log.Printf("[重排序] LLM重排序已启用但LLM客户端不可用，降级为纯规则打分")
		return
	}

	maxDocChars := config.LLMMaxDocChars
	if maxDocChars <= 0 {
		maxDocChars = defaultLLMMaxDocChars
	}

	llmScores, err := r.scoreRelevanceWithLLM(ctx, client, query, scoredResults, maxDocChars)
	if err != nil {
		log.Printf("[重排序] LLM重排序降级为纯规则打分，原因: %v", err)
		return
	}
	if len(llmScores) != len(scoredResults) {
		log.Printf("[重排序] LLM重排序降级为纯规则打分，原因: LLM返回分数数量(%d)与候选数(%d)不一致",
			len(llmScores), len(scoredResults))
		return
	}

	weight := effectiveLLMFusionWeight(config.LLMFusionWeight)
	normalizedRules := normalizeRuleScores(scoredResults)
	for index := range scoredResults {
		llmScore := clampUnitInterval(llmScores[index])
		scoredResults[index].FinalScore = (1-weight)*normalizedRules[index] + weight*llmScore
	}
	log.Printf("[重排序] LLM重排序完成，融合权重: %.2f，候选数: %d", weight, len(scoredResults))
}

// normalizeRuleScores 将本轮候选的规则分数按 min-max 归一化到[0,1]，
// 使规则分与[0,1]区间的LLM分处于同一量纲后再加权融合。
// 当所有规则分相同（无区分度）时，统一映射为1.0。
func normalizeRuleScores(results []ScoredResult) []float64 {
	normalized := make([]float64, len(results))
	if len(results) == 0 {
		return normalized
	}

	minScore, maxScore := results[0].FinalScore, results[0].FinalScore
	for _, result := range results {
		if result.FinalScore < minScore {
			minScore = result.FinalScore
		}
		if result.FinalScore > maxScore {
			maxScore = result.FinalScore
		}
	}
	if maxScore == minScore {
		for index := range normalized {
			normalized[index] = 1.0
		}
		return normalized
	}

	span := maxScore - minScore
	for index, result := range results {
		normalized[index] = (result.FinalScore - minScore) / span
	}
	return normalized
}

// scoreRelevanceWithLLM 调用LLM为每个候选输出[0,1]相关性分数，返回顺序与候选一致。
func (r *RerankerService) scoreRelevanceWithLLM(ctx context.Context, client llm.LLMClient, query string, scoredResults []ScoredResult, maxDocChars int) ([]float64, error) {
	var builder strings.Builder
	builder.WriteString("请评估下列文档与用户问题的相关性，输出0到1之间的小数（数值越大越相关）。\n")
	builder.WriteString("用户问题: ")
	builder.WriteString(query)
	builder.WriteString("\n\n待评估文档:\n")
	for index, scored := range scoredResults {
		content, _ := scored.Result.Fields["content"].(string)
		if maxDocChars > 0 && len(content) > maxDocChars {
			content = content[:maxDocChars]
		}
		fmt.Fprintf(&builder, "[%d] %s\n", index, content)
	}
	builder.WriteString("\n仅返回JSON，格式为 {\"scores\":[...]}，数组长度必须等于文档数量，顺序与文档编号一致。")

	response, err := client.Complete(ctx, &llm.LLMRequest{
		Prompt:       builder.String(),
		SystemPrompt: "你是一个检索结果相关性打分器，只输出JSON。",
		Temperature:  0,
		MaxTokens:    512,
		Format:       "json",
	})
	if err != nil {
		return nil, fmt.Errorf("LLM调用失败: %w", err)
	}
	if response == nil {
		return nil, errors.New("LLM返回空响应")
	}

	return parseLLMRelevanceScores(response.Content, len(scoredResults))
}

// parseLLMRelevanceScores 解析LLM返回的相关性分数，
// 兼容 {"scores":[...]} 与纯数组 [...] 两种结构，并校验数量与候选数一致。
func parseLLMRelevanceScores(content string, expected int) ([]float64, error) {
	cleaned := cleanCodeFence(content)

	var wrapped struct {
		Scores []float64 `json:"scores"`
	}
	if err := json.Unmarshal([]byte(cleaned), &wrapped); err == nil && len(wrapped.Scores) > 0 {
		if len(wrapped.Scores) != expected {
			return nil, fmt.Errorf("LLM相关性分数数量(%d)与候选数(%d)不一致", len(wrapped.Scores), expected)
		}
		return wrapped.Scores, nil
	}

	var raw []float64
	if err := json.Unmarshal([]byte(cleaned), &raw); err == nil && len(raw) > 0 {
		if len(raw) != expected {
			return nil, fmt.Errorf("LLM相关性分数数量(%d)与候选数(%d)不一致", len(raw), expected)
		}
		return raw, nil
	}

	return nil, errors.New("无法解析LLM相关性分数")
}

// cleanCodeFence 去除LLM响应可能包裹的markdown代码块标记
func cleanCodeFence(content string) string {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	return strings.TrimSpace(cleaned)
}

// clampUnitInterval 将分数限制在[0,1]区间
func clampUnitInterval(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
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
