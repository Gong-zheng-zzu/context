package security

import (
	"regexp"
	"strings"
)

// ConfidenceScorer AI响应置信度评分器
type ConfidenceScorer struct {
	detector *Detector
	uncertaintyWords []string
	sourceIndicators []string
}

// NewConfidenceScorer 创建置信度评分器
func NewConfidenceScorer() *ConfidenceScorer {
	return &ConfidenceScorer{
		detector: NewDetector(),
		uncertaintyWords: []string{
			// 英文不确定性词汇
			"maybe", "perhaps", "possibly", "probably", "might", "may",
			"could", "would", "should", "seem", "appear", "likely",
			"uncertain", "unclear", "unsure", "guess", "assume",
			"I think", "I believe", "I suppose", "not sure", "don't know",

			// 中文不确定性词汇
			"可能", "也许", "大概", "或许", "似乎", "好像",
			"应该", "估计", "猜测", "不确定", "不清楚",
			"我认为", "我觉得", "我想", "不太确定", "不知道",

			// 日文不确定性词汇
			"たぶん", "おそらく", "かもしれない", "だろう",
			"と思います", "ようです", "みたいです",
		},
		sourceIndicators: []string{
			// 来源引用指示词
			"according to", "based on", "reference", "source",
			"study shows", "research indicates", "data from",
			"根据", "基于", "来源", "参考", "引用",
			"研究表明", "数据显示", "报告指出",
			"によると", "によれば", "参照",
		},
	}
}

// ScoreResponse 评估AI响应的置信度
// 返回：置信度分数（0.0-1.0）
func (cs *ConfidenceScorer) ScoreResponse(response string, hasKnowledgeData bool) float64 {
	baseScore := 0.7 // 基础分数70分

	// 1. 检测不确定性词汇（每个-10分）
	uncertaintyCount := cs.countUncertaintyWords(response)
	uncertaintyPenalty := float64(uncertaintyCount) * 0.10
	if uncertaintyPenalty > 0.3 { // 最多扣30分
		uncertaintyPenalty = 0.3
	}

	// 2. 检测来源引用（每个+20分）
	sourceCount := cs.countSourceIndicators(response)
	sourceBonus := float64(sourceCount) * 0.20
	if sourceBonus > 0.4 { // 最多加40分
		sourceBonus = 0.4
	}

	// 3. 响应长度评估
	lengthScore := cs.evaluateLength(response)

	// 4. 知识库数据存在性（+15分）
	knowledgeBonus := 0.0
	if hasKnowledgeData {
		knowledgeBonus = 0.15
	}

	// 5. 结构化程度评估（+10分）
	structureBonus := cs.evaluateStructure(response)

	// 6. 具体性评估（+10分）
	specificityBonus := cs.evaluateSpecificity(response)

	// 7. 专业术语使用（+5分）
	terminologyBonus := cs.evaluateTerminology(response)

	// 8. 检测免责声明（-5分）
	disclaimerPenalty := cs.detectDisclaimer(response)

	// 9. 检测矛盾内容（-15分）
	contradictionPenalty := cs.detectContradiction(response)

	// 10. 检测敏感信息泄露（-20分）
	leakagePenalty := cs.detectSensitiveLeakage(response)

	// 计算最终分数
	finalScore := baseScore -
		uncertaintyPenalty +
		sourceBonus +
		lengthScore +
		knowledgeBonus +
		structureBonus +
		specificityBonus +
		terminologyBonus -
		disclaimerPenalty -
		contradictionPenalty -
		leakagePenalty

	// 确保分数在0-1范围内
	if finalScore > 1.0 {
		finalScore = 1.0
	} else if finalScore < 0.0 {
		finalScore = 0.0
	}

	return finalScore
}

// countUncertaintyWords 统计不确定性词汇
func (cs *ConfidenceScorer) countUncertaintyWords(response string) int {
	lowerResponse := strings.ToLower(response)
	count := 0

	for _, word := range cs.uncertaintyWords {
		count += strings.Count(lowerResponse, strings.ToLower(word))
	}

	return count
}

// countSourceIndicators 统计来源引用指示词
func (cs *ConfidenceScorer) countSourceIndicators(response string) int {
	lowerResponse := strings.ToLower(response)
	count := 0

	for _, indicator := range cs.sourceIndicators {
		if strings.Contains(lowerResponse, strings.ToLower(indicator)) {
			count++
		}
	}

	// 检测引用格式 [1], [2], (Smith, 2020) 等
	citationPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\[\d+\]`),                    // [1], [2]
		regexp.MustCompile(`\([A-Z][a-z]+,?\s+\d{4}\)`), // (Smith, 2020)
		regexp.MustCompile(`\d{4}年.*?研究`),               // 2020年研究
	}

	for _, pattern := range citationPatterns {
		matches := pattern.FindAllString(response, -1)
		count += len(matches)
	}

	return count
}

// evaluateLength 评估响应长度
func (cs *ConfidenceScorer) evaluateLength(response string) float64 {
	length := len([]rune(response))

	// 过短的响应（<50字符）：-15分
	if length < 50 {
		return -0.15
	}

	// 合理长度（50-500字符）：0分
	if length <= 500 {
		return 0.0
	}

	// 详细响应（500-2000字符）：+5分
	if length <= 2000 {
		return 0.05
	}

	// 过长响应（>2000字符）：0分（可能是冗余）
	return 0.0
}

// evaluateStructure 评估结构化程度
func (cs *ConfidenceScorer) evaluateStructure(response string) float64 {
	score := 0.0

	// 检测列表结构
	if strings.Contains(response, "1.") || strings.Contains(response, "1、") ||
		strings.Contains(response, "- ") || strings.Contains(response, "• ") {
		score += 0.05
	}

	// 检测段落分隔
	paragraphs := strings.Split(response, "\n\n")
	if len(paragraphs) >= 2 {
		score += 0.03
	}

	// 检测标题或分节
	if strings.Contains(response, "##") || strings.Contains(response, "**") ||
		strings.Contains(response, "【") {
		score += 0.02
	}

	return score
}

// evaluateSpecificity 评估具体性
func (cs *ConfidenceScorer) evaluateSpecificity(response string) float64 {
	score := 0.0

	// 检测数字和数据
	numberPattern := regexp.MustCompile(`\d+\.?\d*\s*(mg|ml|kg|cm|mmHg|mmol/L|%|次|天|小时)`)
	matches := numberPattern.FindAllString(response, -1)
	if len(matches) > 0 {
		score += 0.05
	}

	// 检测具体时间
	timePattern := regexp.MustCompile(`\d{4}年|\d{1,2}月|\d{1,2}日|\d{1,2}:\d{2}`)
	if timePattern.MatchString(response) {
		score += 0.03
	}

	// 检测具体名称（药物、疾病等）
	if strings.Contains(response, "（") && strings.Contains(response, "）") {
		score += 0.02
	}

	return score
}

// evaluateTerminology 评估专业术语使用
func (cs *ConfidenceScorer) evaluateTerminology(response string) float64 {
	// 医疗健康领域专业术语
	medicalTerms := []string{
		"血压", "血糖", "心率", "体温", "血脂", "胆固醇",
		"高血压", "糖尿病", "冠心病", "心律失常",
		"收缩压", "舒张压", "空腹血糖", "餐后血糖",
		"mmHg", "mmol/L", "mg/dl", "bpm",
		"hypertension", "diabetes", "glucose", "cholesterol",
	}

	count := 0
	lowerResponse := strings.ToLower(response)
	for _, term := range medicalTerms {
		if strings.Contains(lowerResponse, strings.ToLower(term)) {
			count++
		}
	}

	// 每个专业术语+1分，最多+5分
	score := float64(count) * 0.01
	if score > 0.05 {
		score = 0.05
	}

	return score
}

// detectDisclaimer 检测免责声明
func (cs *ConfidenceScorer) detectDisclaimer(response string) float64 {
	disclaimers := []string{
		"仅供参考", "建议咨询医生", "请就医", "不能替代",
		"for reference only", "consult a doctor", "seek medical",
		"参考のみ", "医師に相談",
	}

	lowerResponse := strings.ToLower(response)
	for _, disclaimer := range disclaimers {
		if strings.Contains(lowerResponse, strings.ToLower(disclaimer)) {
			return 0.05 // 有免责声明，轻微降低置信度
		}
	}

	return 0.0
}

// detectContradiction 检测矛盾内容
func (cs *ConfidenceScorer) detectContradiction(response string) float64 {
	// 检测矛盾词对
	contradictions := [][]string{
		{"是", "不是"},
		{"可以", "不可以"},
		{"应该", "不应该"},
		{"yes", "no"},
		{"true", "false"},
		{"increase", "decrease"},
		{"升高", "降低"},
		{"增加", "减少"},
	}

	lowerResponse := strings.ToLower(response)
	contradictionCount := 0

	for _, pair := range contradictions {
		if strings.Contains(lowerResponse, strings.ToLower(pair[0])) &&
			strings.Contains(lowerResponse, strings.ToLower(pair[1])) {
			// 检查是否在相近位置（可能是真正的矛盾）
			idx1 := strings.Index(lowerResponse, strings.ToLower(pair[0]))
			idx2 := strings.Index(lowerResponse, strings.ToLower(pair[1]))
			if abs(idx1-idx2) < 200 { // 在200字符内出现矛盾词
				contradictionCount++
			}
		}
	}

	// 每个矛盾-5分，最多-15分
	penalty := float64(contradictionCount) * 0.05
	if penalty > 0.15 {
		penalty = 0.15
	}

	return penalty
}

// detectSensitiveLeakage 检测敏感信息泄露
func (cs *ConfidenceScorer) detectSensitiveLeakage(response string) float64 {
	// 检测是否泄露了不应该出现的敏感信息
	sensitiveInfos := cs.detector.Detect(response)

	// 某些类型的敏感信息不应该出现在AI响应中
	criticalTypes := []SensitiveType{
		SensitiveTypePassword,
		SensitiveTypeAPIKey,
		SensitiveTypePrivateKey,
		SensitiveTypeToken,
		SensitiveTypeDatabase,
	}

	leakageCount := 0
	for _, info := range sensitiveInfos {
		for _, criticalType := range criticalTypes {
			if info.Type == criticalType {
				leakageCount++
				break
			}
		}
	}

	// 每个泄露-10分，最多-20分
	penalty := float64(leakageCount) * 0.10
	if penalty > 0.20 {
		penalty = 0.20
	}

	return penalty
}

// ScoreWithDetails 评估置信度并返回详细信息
func (cs *ConfidenceScorer) ScoreWithDetails(response string, hasKnowledgeData bool) (score float64, details map[string]interface{}) {
	details = make(map[string]interface{})

	// 计算各项指标
	uncertaintyCount := cs.countUncertaintyWords(response)
	sourceCount := cs.countSourceIndicators(response)
	length := len([]rune(response))

	details["uncertainty_words"] = uncertaintyCount
	details["source_indicators"] = sourceCount
	details["length"] = length
	details["has_knowledge_data"] = hasKnowledgeData

	// 计算最终分数
	score = cs.ScoreResponse(response, hasKnowledgeData)
	details["confidence_score"] = score

	// 添加置信度等级
	if score >= 0.8 {
		details["confidence_level"] = "高"
	} else if score >= 0.6 {
		details["confidence_level"] = "中"
	} else {
		details["confidence_level"] = "低"
	}

	return score, details
}

// GetConfidenceLevel 获取置信度等级描述
func (cs *ConfidenceScorer) GetConfidenceLevel(score float64) string {
	if score >= 0.9 {
		return "非常高"
	} else if score >= 0.8 {
		return "高"
	} else if score >= 0.7 {
		return "较高"
	} else if score >= 0.6 {
		return "中等"
	} else if score >= 0.5 {
		return "较低"
	} else {
		return "低"
	}
}

// abs 计算绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
