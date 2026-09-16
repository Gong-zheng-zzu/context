package security

import (
	"math"
)

// ProgressiveConfidenceModel 渐进式置信度累积模型（PCCM）
// Progressive Confidence Cumulation Model
type ProgressiveConfidenceModel struct {
	// 权重配置
	RegexWeight float64 // 正则检测权重
	TrieWeight  float64 // 字典树检测权重
	LLMWeight   float64 // LLM语义检测权重

	// 阈值配置
	HighConfidenceThreshold   float64 // 高置信度阈值（≥0.9）
	MediumConfidenceThreshold float64 // 中置信度阈值（≥0.7）
}

// PCCMDetectionResult 检测结果（PCCM专用）
type PCCMDetectionResult struct {
	IsSensitive     bool     // 是否为敏感信息
	FinalConfidence float64  // 最终置信度
	RegexConfidence float64  // 正则检测置信度
	TrieConfidence  float64  // 字典树检测置信度
	LLMConfidence   float64  // LLM语义检测置信度
	ConfidenceLevel string   // 置信度等级：high/medium/low
	DetectionLayers []string // 触发的检测层
	RecommendAction string   // 推荐操作：auto_redact/manual_review/pass
}

// NewProgressiveConfidenceModel 创建渐进式置信度模型
func NewProgressiveConfidenceModel() *ProgressiveConfidenceModel {
	configuration := DefaultPCCMSecurityConfig()
	// This legacy three-input API projects the single PCCM-S source of truth
	// onto regex/dictionary/LLM and renormalizes after excluding context.
	projectedTotal := configuration.LayerWeights[1] + configuration.LayerWeights[2] + configuration.LayerWeights[5]
	return &ProgressiveConfidenceModel{
		RegexWeight: configuration.LayerWeights[1] / projectedTotal,
		TrieWeight:  configuration.LayerWeights[2] / projectedTotal,
		LLMWeight:   configuration.LayerWeights[5] / projectedTotal,

		// 默认阈值配置
		HighConfidenceThreshold:   0.9,
		MediumConfidenceThreshold: 0.7,
	}
}

// CalculateFinalConfidence 计算最终置信度
// 核心算法：加权累积 + 非线性增强
func (m *ProgressiveConfidenceModel) CalculateFinalConfidence(
	regexConf, trieConf, llmConf float64,
) float64 {
	// 基础加权累积
	baseConfidence := m.RegexWeight*regexConf +
		m.TrieWeight*trieConf +
		m.LLMWeight*llmConf

	// 非线性增强：如果多层都检测到，置信度额外提升
	layerCount := 0
	if regexConf > 0.5 {
		layerCount++
	}
	if trieConf > 0.5 {
		layerCount++
	}
	if llmConf > 0.5 {
		layerCount++
	}

	// 多层协同增强因子（1层=1.0, 2层=1.1, 3层=1.2）
	enhancementFactor := 1.0 + float64(layerCount-1)*0.1

	// 最终置信度 = 基础置信度 × 增强因子
	finalConfidence := baseConfidence * enhancementFactor

	// 限制在[0, 1]区间
	if finalConfidence > 1.0 {
		finalConfidence = 1.0
	}

	return finalConfidence
}

// Detect 执行渐进式检测
func (m *ProgressiveConfidenceModel) Detect(
	text string,
	regexDetector func(string) (bool, float64),
	trieDetector func(string) (bool, float64),
	llmDetector func(string) (bool, float64),
) PCCMDetectionResult {
	result := PCCMDetectionResult{
		DetectionLayers: []string{},
	}

	// 第一层：正则检测
	regexDetected, regexConf := regexDetector(text)
	result.RegexConfidence = regexConf
	if regexDetected {
		result.DetectionLayers = append(result.DetectionLayers, "regex")
	}

	// 第二层：字典树检测（仅在第一层未高置信度检出时执行）
	if regexConf < m.HighConfidenceThreshold {
		trieDetected, trieConf := trieDetector(text)
		result.TrieConfidence = trieConf
		if trieDetected {
			result.DetectionLayers = append(result.DetectionLayers, "trie")
		}
	}

	// 第三层：LLM语义检测（仅在前两层都未高置信度检出时执行）
	if regexConf < m.HighConfidenceThreshold && result.TrieConfidence < m.HighConfidenceThreshold {
		llmDetected, llmConf := llmDetector(text)
		result.LLMConfidence = llmConf
		if llmDetected {
			result.DetectionLayers = append(result.DetectionLayers, "llm")
		}
	}

	// 计算最终置信度
	result.FinalConfidence = m.CalculateFinalConfidence(
		result.RegexConfidence,
		result.TrieConfidence,
		result.LLMConfidence,
	)

	// 判定置信度等级
	if result.FinalConfidence >= m.HighConfidenceThreshold {
		result.ConfidenceLevel = "high"
		result.IsSensitive = true
		result.RecommendAction = "auto_redact" // 自动脱敏
	} else if result.FinalConfidence >= m.MediumConfidenceThreshold {
		result.ConfidenceLevel = "medium"
		result.IsSensitive = true
		result.RecommendAction = "manual_review" // 人工审核
	} else {
		result.ConfidenceLevel = "low"
		result.IsSensitive = false
		result.RecommendAction = "pass" // 放行
	}

	return result
}

// OptimizeWeights 根据历史数据优化权重（自适应学习）
// 使用梯度下降法优化权重，最小化误报率和漏报率
func (m *ProgressiveConfidenceModel) OptimizeWeights(
	trainingData []struct {
		RegexConf   float64
		TrieConf    float64
		LLMConf     float64
		GroundTruth bool // 真实标签
	},
	learningRate float64,
	iterations int,
) {
	for iter := 0; iter < iterations; iter++ {
		// 计算梯度
		gradRegex := 0.0
		gradTrie := 0.0
		gradLLM := 0.0

		for _, data := range trainingData {
			predicted := m.CalculateFinalConfidence(data.RegexConf, data.TrieConf, data.LLMConf)
			actual := 0.0
			if data.GroundTruth {
				actual = 1.0
			}

			// 计算误差
			error := predicted - actual

			// 累积梯度
			gradRegex += error * data.RegexConf
			gradTrie += error * data.TrieConf
			gradLLM += error * data.LLMConf
		}

		// 归一化梯度
		n := float64(len(trainingData))
		gradRegex /= n
		gradTrie /= n
		gradLLM /= n

		// 更新权重
		m.RegexWeight -= learningRate * gradRegex
		m.TrieWeight -= learningRate * gradTrie
		m.LLMWeight -= learningRate * gradLLM

		// 归一化权重（确保和为1）
		totalWeight := m.RegexWeight + m.TrieWeight + m.LLMWeight
		m.RegexWeight /= totalWeight
		m.TrieWeight /= totalWeight
		m.LLMWeight /= totalWeight

		// 确保权重非负
		m.RegexWeight = math.Max(0, m.RegexWeight)
		m.TrieWeight = math.Max(0, m.TrieWeight)
		m.LLMWeight = math.Max(0, m.LLMWeight)
	}
}

// GetModelDescription 获取模型描述（用于论文和答辩）
func (m *ProgressiveConfidenceModel) GetModelDescription() string {
	return `
渐进式置信度累积模型（PCCM - Progressive Confidence Cumulation Model）

核心思想：
多层检测结果通过加权累积和非线性增强，形成最终置信度判定。

数学模型：
C_final = (w1·C_regex + w2·C_trie + w3·C_llm) × (1 + 0.1·(n-1))

其中：
- C_regex, C_trie, C_llm: 各层检测置信度 ∈ [0,1]
- w1, w2, w3: 权重系数，满足 w1+w2+w3=1
- n: 触发的检测层数 ∈ {1,2,3}
- 增强因子: 1 + 0.1·(n-1)，体现多层协同效应

创新点：
1. 渐进式检测：高置信度层可提前终止，降低计算开销
2. 置信度累积：多层结果融合，提升准确率
3. 非线性增强：多层协同时置信度额外提升
4. 自适应优化：权重可根据历史数据自动调优

评测边界：
- 默认权重是固定工程配置，不代表已通过训练或冻结集校准
- 准确率和性能提升必须由同配置、同数据集的消融实验验证
- 每层置信度可追溯，便于审计和复测
`
}
