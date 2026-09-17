package causal_reasoning

import (
	"math"
)

// PCCMFusionEngine PCCM置信度融合引擎（因果侧 PCCM-C）
//
// 与安全侧 PCCM 的关系（同名不同域）：安全侧的 ProgressiveConfidenceModel
// （internal/security/pccm_model.go）融合多层敏感信息检测（正则/词典/上下文/LLM）
// 的置信度；本引擎融合因果抽取的三路证据置信度（rule 规则匹配 / PMI 共现统计 /
// LLM 推理）。两者共享"归一化加权 + 有效源归一"的思想，但权重、层数与校准
// 状态相互独立，不能混用。
//
// 当前实现（保持既有行为）：
//
//	C_final = (Σ w_s·C_s / Σ w_s)，仅统计置信度 > 0 的证据源
type PCCMFusionEngine struct {
	weights PCCMWeights
}

// NewPCCMFusionEngine 创建PCCM融合引擎实例
func NewPCCMFusionEngine(weights PCCMWeights) *PCCMFusionEngine {
	// 验证权重和为1
	total := weights.RuleWeight + weights.PMIWeight + weights.LLMWeight
	if math.Abs(total-1.0) > 0.01 {
		// 归一化权重
		weights.RuleWeight /= total
		weights.PMIWeight /= total
		weights.LLMWeight /= total
	}

	return &PCCMFusionEngine{
		weights: weights,
	}
}

// NewDefaultPCCMFusionEngine 创建默认权重的PCCM融合引擎
func NewDefaultPCCMFusionEngine() *PCCMFusionEngine {
	return NewPCCMFusionEngine(DefaultPCCMWeights)
}

// FuseConfidence 融合三路置信度
// ruleConf: 规则匹配置信度
// pmiConf: PMI统计置信度
// llmConf: LLM推理置信度
//
// evidenceCount 为原始证据条数，仅为保持 API 兼容而保留，不参与计算：
// 重复片段（同一段文本被多次引用）不是独立证据，因此不计入独立证据数，
// 也不会据此放大置信度。若未来引入"独立证据增强"（类似安全侧 PCCM-S 的
// 多层协同增强项），必须先定义"独立证据"的判定标准（如来自不同源、不同
// 文本片段的证据），并用同配置消融实验验证后再启用。
func (pe *PCCMFusionEngine) FuseConfidence(ruleConf, pmiConf, llmConf float64, evidenceCount int) float64 {
	// 基础融合: C_base = w1·C_rule + w2·C_pmi + w3·C_llm
	// A missing rule/PMI signal is not negative evidence. Normalize only across
	// sources that supplied a score so a standalone LLM extraction is not
	// mechanically reduced to its fixed 0.05 weight contribution.
	weightedSum := 0.0
	activeWeight := 0.0
	for _, source := range []struct {
		confidence float64
		weight     float64
	}{
		{confidence: ruleConf, weight: pe.weights.RuleWeight},
		{confidence: pmiConf, weight: pe.weights.PMIWeight},
		{confidence: llmConf, weight: pe.weights.LLMWeight},
	} {
		if source.confidence <= 0 || source.weight <= 0 {
			continue
		}
		weightedSum += source.confidence * source.weight
		activeWeight += source.weight
	}
	if activeWeight == 0 {
		return 0
	}
	baseConfidence := weightedSum / activeWeight

	// evidenceCount 有意不参与计算：重复片段不是独立证据，不放大置信度。
	_ = evidenceCount

	// 确保置信度在[0, 1]区间
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}
	if baseConfidence < 0.0 {
		baseConfidence = 0.0
	}

	return baseConfidence
}

// FuseCausalRelation 为因果关系融合置信度
func (pe *PCCMFusionEngine) FuseCausalRelation(relation *CausalRelation) {
	evidenceCount := len(relation.Evidence)
	if evidenceCount == 0 {
		evidenceCount = 1
	}

	relation.Confidence = pe.FuseConfidence(
		relation.RuleConfidence,
		relation.PMIConfidence,
		relation.LLMConfidence,
		evidenceCount,
	)
}

// CalculatePathConfidence 计算因果路径置信度
// 路径置信度 = 所有边置信度的乘积
func (pe *PCCMFusionEngine) CalculatePathConfidence(edgeConfidences []float64) float64 {
	if len(edgeConfidences) == 0 {
		return 0.0
	}

	confidence := 1.0
	for _, edgeConf := range edgeConfidences {
		confidence *= edgeConf
	}

	return confidence
}

// UpdateWeights 更新融合权重
func (pe *PCCMFusionEngine) UpdateWeights(weights PCCMWeights) {
	// 归一化权重
	total := weights.RuleWeight + weights.PMIWeight + weights.LLMWeight
	if total > 0 {
		weights.RuleWeight /= total
		weights.PMIWeight /= total
		weights.LLMWeight /= total
	}

	pe.weights = weights
}

// GetWeights 获取当前权重配置
func (pe *PCCMFusionEngine) GetWeights() PCCMWeights {
	return pe.weights
}

// EvaluateConfidenceLevel 评估置信度等级
func (pe *PCCMFusionEngine) EvaluateConfidenceLevel(confidence float64) string {
	switch {
	case confidence >= 0.9:
		return "非常高"
	case confidence >= 0.75:
		return "高"
	case confidence >= 0.6:
		return "中等"
	case confidence >= 0.4:
		return "较低"
	default:
		return "低"
	}
}

// CalculateWeightedAverage 计算加权平均置信度（备用融合方法）
func (pe *PCCMFusionEngine) CalculateWeightedAverage(confidences []float64, weights []float64) float64 {
	if len(confidences) != len(weights) || len(confidences) == 0 {
		return 0.0
	}

	var sum, weightSum float64
	for i, conf := range confidences {
		sum += conf * weights[i]
		weightSum += weights[i]
	}

	if weightSum == 0 {
		return 0.0
	}

	return sum / weightSum
}

// ApplyConfidenceDecay 应用时间衰减到置信度
// 用于历史因果关系的置信度调整
func (pe *PCCMFusionEngine) ApplyConfidenceDecay(confidence float64, daysSinceCreation int) float64 {
	if daysSinceCreation <= 0 {
		return confidence
	}

	// 每90天衰减5%
	decayRate := 0.05
	decayPeriod := 90.0
	decayFactor := math.Exp(-decayRate * float64(daysSinceCreation) / decayPeriod)

	return confidence * decayFactor
}

// NormalizeConfidence 归一化置信度到[0,1]区间
func (pe *PCCMFusionEngine) NormalizeConfidence(confidence float64) float64 {
	if confidence < 0.0 {
		return 0.0
	}
	if confidence > 1.0 {
		return 1.0
	}
	return confidence
}
