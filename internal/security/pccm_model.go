package security

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// ProgressiveConfidenceModel 渐进式置信度累积模型（PCCM，安全侧 PCCM-S）。
//
// 本类型是安全侧多层检测置信度融合的唯一真源：multi_layer_detector 的
// calculateConfidenceWithPCCM 只做委托，不再持有第二套公式实现。
//
// 数学模型（与生产行为一致）：
//
//	C_final = (Σ w_i·C_i / Σ w_i) × (1 + e·(n-1))
//
// 其中：
//   - C_i: 实际执行层的置信度 ∈ [0,1]，由调用方按实际执行的层填充
//   - w_i: 各层权重（LayerWeights），加权平均在"有检测结果的层"上归一化
//   - e:   EnhancementPerExtraLayer，多层协同增强系数（默认 0.05）
//   - n:   置信度超过 LayerActivationThreshold（默认 0.3）的层数
//   - n = 1 时增强因子为 1.0（单层不增强）
//
// 权重与阈值全部来自 PCCMSecurityConfig 配置契约，可通过环境变量覆盖。
// CalibrationSource 固定为 "fixed_unvalidated"：默认权重是固定工程配置，
// 尚未经过训练或冻结集校准，不得对外表述为学习或验证得到的参数。
type ProgressiveConfidenceModel struct {
	mu     sync.RWMutex
	config PCCMSecurityConfig
}

// NewProgressiveConfidenceModel 创建使用默认配置的渐进式置信度模型。
func NewProgressiveConfidenceModel() *ProgressiveConfidenceModel {
	return NewProgressiveConfidenceModelWithConfig(DefaultPCCMSecurityConfig())
}

// NewProgressiveConfidenceModelWithConfig 使用指定配置创建模型。
// 零值配置字段会被填充为与默认生产行为一致的值。
func NewProgressiveConfidenceModelWithConfig(config PCCMSecurityConfig) *ProgressiveConfidenceModel {
	return &ProgressiveConfidenceModel{
		config: normalizePCCMConfig(config),
	}
}

// Config 返回当前配置的快照（深拷贝，调用方修改不会影响模型）。
func (m *ProgressiveConfidenceModel) Config() PCCMSecurityConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return clonePCCMSecurityConfig(m.config)
}

// SetLayerWeight 更新单层权重，作为 MultiLayerDetector.SetLayerWeight 的
// 唯一真源。此前的实现只改动 MultiLayerDetector 遗留的 weights 后备映射，
// 而生产默认走 PCCM 模型，导致调用方设置的权重静默失效。
func (m *ProgressiveConfidenceModel) SetLayerWeight(layerID int, weight float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config.LayerWeights == nil {
		m.config.LayerWeights = map[int]float64{}
	}
	m.config.LayerWeights[layerID] = weight
}

// GetLayerWeight 返回单层权重（单一真源）。
func (m *ProgressiveConfidenceModel) GetLayerWeight(layerID int) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.LayerWeights[layerID]
}

// CalculateFinalConfidence 计算最终置信度。
//
// 参数 layerConfidences 的 key 为层 ID（1=正则, 2=词典, 4=上下文规则, 5=LLM），
// 调用方只填充本次实际执行的层。语义：
//  1. 归一化加权平均：仅统计置信度 > 0 的层（缺失信号不是负证据），
//     权重来自 LayerWeights；权重之和不需要为 1，内部做归一化。
//  2. 多层协同增强：统计置信度超过 LayerActivationThreshold 的层数 n，
//     n > 1 时乘以 (1 + EnhancementPerExtraLayer·(n-1))；单层结果不做增强。
//  3. 结果截断到 [0, 1]。
func (m *ProgressiveConfidenceModel) CalculateFinalConfidence(layerConfidences map[int]float64) float64 {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	weights := config.LayerWeights
	enhancement := config.EnhancementPerExtraLayer
	activationThreshold := config.LayerActivationThreshold

	// 归一化加权平均：只对有检测结果的层（置信度 > 0）加权。
	activeWeightSum := 0.0
	activeConfSum := 0.0
	for layerID, confidence := range layerConfidences {
		if confidence <= 0 {
			continue
		}
		weight := weights[layerID]
		activeWeightSum += weight
		activeConfSum += weight * confidence
	}

	var finalConfidence float64
	if activeWeightSum > 0 {
		finalConfidence = activeConfSum / activeWeightSum
	}

	// 多层协同增强：n = 超过激活阈值的层数；单层（n=1）不增强。
	layerCount := 0
	for _, confidence := range layerConfidences {
		if confidence > activationThreshold {
			layerCount++
		}
	}
	if layerCount > 1 {
		finalConfidence *= 1.0 + float64(layerCount-1)*enhancement
	}

	if finalConfidence > 1.0 {
		finalConfidence = 1.0
	}
	if finalConfidence < 0.0 {
		finalConfidence = 0.0
	}

	return finalConfidence
}

// normalizePCCMConfig 填充零值配置字段，保证默认行为与既有生产实现一致。
// 注意：EnhancementPerExtraLayer 等字段无法区分"显式配置为 0"与"未配置"，
// 因此显式配置 0 增强系数暂不支持（会被填充为默认值 0.05）。
func normalizePCCMConfig(config PCCMSecurityConfig) PCCMSecurityConfig {
	normalized := config
	if normalized.Version == "" {
		normalized.Version = PCCMSecurityConfigVersion
	}
	if normalized.CalibrationSource == "" {
		normalized.CalibrationSource = "fixed_unvalidated"
	}
	if len(normalized.LayerWeights) == 0 {
		normalized.LayerWeights = map[int]float64{1: 0.70, 2: 0.10, 4: 0.15, 5: 0.05}
	} else {
		normalized.LayerWeights = clonePCCMSecurityConfig(normalized).LayerWeights
	}
	if normalized.EnhancementPerExtraLayer == 0 {
		normalized.EnhancementPerExtraLayer = 0.05
	}
	if normalized.DecisionThreshold == 0 {
		normalized.DecisionThreshold = 0.45
	}
	if normalized.LayerActivationThreshold == 0 {
		normalized.LayerActivationThreshold = 0.3
	}
	return normalized
}

// PCCMSecurityConfigFromEnv 从环境变量构造 PCCM-S 配置，未设置的环境变量
// 回退到 DefaultPCCMSecurityConfig。支持的变量：
//
//	PCCM_LAYER_WEIGHT_<N>              各层权重，如 PCCM_LAYER_WEIGHT_1=0.7
//	PCCM_ENHANCEMENT_PER_EXTRA_LAYER   多层协同增强系数
//	PCCM_DECISION_THRESHOLD            决策阈值
//	PCCM_LAYER_ACTIVATION_THRESHOLD    层激活阈值（计数进入增强因子的下限）
func PCCMSecurityConfigFromEnv() PCCMSecurityConfig {
	config := DefaultPCCMSecurityConfig()

	layerWeights := make(map[int]float64, len(config.LayerWeights))
	for layerID := range config.LayerWeights {
		envName := "PCCM_LAYER_WEIGHT_" + strconv.Itoa(layerID)
		if raw, ok := os.LookupEnv(envName); ok {
			if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value >= 0 {
				layerWeights[layerID] = value
			}
		}
	}
	if len(layerWeights) > 0 {
		// 未通过环境变量覆盖的层保留默认权重。
		for layerID, weight := range config.LayerWeights {
			if _, overridden := layerWeights[layerID]; !overridden {
				layerWeights[layerID] = weight
			}
		}
		config.LayerWeights = layerWeights
	}

	if raw, ok := os.LookupEnv("PCCM_ENHANCEMENT_PER_EXTRA_LAYER"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value >= 0 {
			config.EnhancementPerExtraLayer = value
		}
	}
	if raw, ok := os.LookupEnv("PCCM_DECISION_THRESHOLD"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 {
			config.DecisionThreshold = value
		}
	}
	if raw, ok := os.LookupEnv("PCCM_LAYER_ACTIVATION_THRESHOLD"); ok {
		if value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil && value > 0 {
			config.LayerActivationThreshold = value
		}
	}

	return config
}

// GetModelDescription 获取模型描述（用于文档与答辩）。
func (m *ProgressiveConfidenceModel) GetModelDescription() string {
	config := m.Config()
	return `
渐进式置信度累积模型（PCCM - Progressive Confidence Cumulation Model，安全侧 PCCM-S）

核心思想：
多层检测置信度通过归一化加权平均与多层协同增强，形成最终置信度判定。

数学模型：
C_final = (Σ w_i·C_i / Σ w_i) × (1 + e·(n-1))

其中：
- C_i: 实际执行层的检测置信度 ∈ [0,1]
- w_i: 各层权重（LayerWeights），加权平均在有效层上归一化
- e:   多层协同增强系数 EnhancementPerExtraLayer（默认 0.05）
- n:   置信度超过层激活阈值（默认 0.3）的层数；单层不增强

工程配置（当前版本 ` + config.Version + `）：
- 默认权重 {1:0.70, 2:0.10, 4:0.15, 5:0.05}、增强系数 0.05、决策阈值 ` +
		strconv.FormatFloat(config.DecisionThreshold, 'f', -1, 64) + `
- 权重与阈值可通过 PCCM_* 环境变量覆盖
- CalibrationSource = ` + config.CalibrationSource + `：默认权重是固定工程配置，
  不代表已通过训练或冻结集校准
- 准确率和性能提升必须由同配置、同数据集的消融实验验证
- 每层置信度可追溯，便于审计和复测
`
}
