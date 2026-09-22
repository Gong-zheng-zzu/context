package security

import (
	"math"
	"testing"
)

const pccmFloatTolerance = 1e-9

func assertFloatEqual(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > pccmFloatTolerance {
		t.Fatalf("%s = %v, want %v (diff %v)", name, got, want, math.Abs(got-want))
	}
}

// TestCalculateFinalConfidenceFormula 验证公式：
// C_final = (Σ w_i·C_i / Σ w_i) × (1 + e·(n-1))。
// 默认权重 {1:0.70, 2:0.10, 4:0.15, 5:0.05}、增强系数 0.05：
// 输入 {1:0.8, 2:0.6, 4:0.4, 5:0.9} → 加权平均 = 0.725，n=4 → ×1.15 = 0.83375。
func TestCalculateFinalConfidenceFormula(t *testing.T) {
	model := NewProgressiveConfidenceModel()

	got := model.CalculateFinalConfidence(map[int]float64{1: 0.8, 2: 0.6, 4: 0.4, 5: 0.9})
	assertFloatEqual(t, "final confidence", got, 0.83375)
}

// TestSingleLayerNotEnhanced 验证单层（n=1）增强因子为 1.0。
func TestSingleLayerNotEnhanced(t *testing.T) {
	model := NewProgressiveConfidenceModel()

	assertFloatEqual(t, "single high layer", model.CalculateFinalConfidence(map[int]float64{1: 0.9}), 0.9)
	// 置信度低于激活阈值 0.3 的单层同样不增强，且按加权平均返回原值。
	assertFloatEqual(t, "single low layer", model.CalculateFinalConfidence(map[int]float64{5: 0.2}), 0.2)
}

// TestWeightNormalization 验证权重无需预先归一化：加权平均在有效层上归一化。
// 权重 {1:2.0, 5:2.0}、输入 {1:0.5, 5:1.0} → 平均 = (1.0+2.0)/4 = 0.75，n=2 → ×1.05 = 0.7875。
func TestWeightNormalization(t *testing.T) {
	config := DefaultPCCMSecurityConfig()
	config.LayerWeights = map[int]float64{1: 2.0, 5: 2.0}
	model := NewProgressiveConfidenceModelWithConfig(config)

	got := model.CalculateFinalConfidence(map[int]float64{1: 0.5, 5: 1.0})
	assertFloatEqual(t, "normalized weighted average", got, 0.7875)
}

// TestZeroConfidenceLayersExcluded 验证置信度为 0 的层不计入加权平均
// （缺失信号不是负证据），但仍与既有生产行为一致地参与层数判定边界。
func TestZeroConfidenceLayersExcluded(t *testing.T) {
	model := NewProgressiveConfidenceModel()

	// 仅 Layer 5 有效（0.9 > 0），Layer 1 置信度 0 不参与加权平均。
	got := model.CalculateFinalConfidence(map[int]float64{1: 0, 5: 0.9})
	assertFloatEqual(t, "single effective layer", got, 0.9)

	// 一个超过激活阈值的层 + 一个零置信度层 → n=1，不增强。
	got = model.CalculateFinalConfidence(map[int]float64{1: 0.5, 2: 0, 4: 0, 5: 0})
	assertFloatEqual(t, "zero layers ignored", got, 0.5)
}

// TestEmptyInputBoundary 验证空输入边界：返回 0，不 panic。
func TestEmptyInputBoundary(t *testing.T) {
	model := NewProgressiveConfidenceModel()

	assertFloatEqual(t, "nil map", model.CalculateFinalConfidence(nil), 0)
	assertFloatEqual(t, "empty map", model.CalculateFinalConfidence(map[int]float64{}), 0)
	// 无对应权重的层（如未实现的 NER 层）贡献为零。
	assertFloatEqual(t, "unknown layer", model.CalculateFinalConfidence(map[int]float64{3: 0.9}), 0)
}

// TestConfigurationSnapshotConsistency 验证配置契约一致性：
// 模型配置、默认配置与多层检测器导出的 PCCM-S 快照三者一致。
func TestConfigurationSnapshotConsistency(t *testing.T) {
	model := NewProgressiveConfidenceModel()
	modelConfig := model.Config()

	defaultConfig := DefaultPCCMSecurityConfig()
	if modelConfig.Version != defaultConfig.Version ||
		modelConfig.CalibrationSource != defaultConfig.CalibrationSource ||
		modelConfig.EnhancementPerExtraLayer != defaultConfig.EnhancementPerExtraLayer ||
		modelConfig.DecisionThreshold != defaultConfig.DecisionThreshold ||
		modelConfig.LayerActivationThreshold != defaultConfig.LayerActivationThreshold {
		t.Fatalf("model config = %+v, want default %+v", modelConfig, defaultConfig)
	}
	for layerID, weight := range defaultConfig.LayerWeights {
		if modelConfig.LayerWeights[layerID] != weight {
			t.Fatalf("model weight for layer %d = %v, want %v", layerID, modelConfig.LayerWeights[layerID], weight)
		}
	}

	// 修改快照不得影响模型内部状态。
	modelConfig.LayerWeights[1] = 0
	if model.Config().LayerWeights[1] != defaultConfig.LayerWeights[1] {
		t.Fatal("config snapshot mutation leaked into model state")
	}

	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")
	detectorConfig := detector.GetConfiguration().PCCM
	if detectorConfig.Version != modelConfig.Version ||
		detectorConfig.CalibrationSource != modelConfig.CalibrationSource ||
		detectorConfig.EnhancementPerExtraLayer != modelConfig.EnhancementPerExtraLayer ||
		detectorConfig.DecisionThreshold != modelConfig.DecisionThreshold ||
		detectorConfig.LayerActivationThreshold != modelConfig.LayerActivationThreshold {
		t.Fatalf("detector PCCM-S config = %+v, want model config %+v", detectorConfig, modelConfig)
	}
	// 必须用未被污染的 defaultConfig 比对：上方为验证深拷贝已把
	// modelConfig.LayerWeights[1] 改为 0，直接复用会得到错误的期望值。
	for layerID, weight := range defaultConfig.LayerWeights {
		if detectorConfig.LayerWeights[layerID] != weight {
			t.Fatalf("detector weight for layer %d = %v, want %v", layerID, detectorConfig.LayerWeights[layerID], weight)
		}
	}
}

// TestMultiLayerDetectorDelegatesToPCCMModel 验证检测器委托调用模型，
// 两套实现不再各自为政：同一层置信度输入下结果必须完全一致。
func TestMultiLayerDetectorDelegatesToPCCMModel(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")
	model := detector.GetPCCMModel()

	cases := []map[int]float64{
		{1: 0.8, 2: 0.6, 4: 0.4, 5: 0.9},
		{1: 0.5, 2: 0.0, 4: 0.0, 5: 0.0},
		{1: 0.2, 2: 0.25, 4: 0.1, 5: 0.0},
		{1: 0.0, 2: 0.0, 4: 0.0, 5: 0.0},
	}

	for i, layerConfs := range cases {
		results := make([]LayerResult, 0, len(layerConfs))
		for layerID, confidence := range layerConfs {
			results = append(results, LayerResult{LayerID: layerID, Confidence: confidence})
		}
		detectorResult := detector.calculateConfidence(results)
		modelResult := model.CalculateFinalConfidence(layerConfs)
		if math.Abs(detectorResult-modelResult) > pccmFloatTolerance {
			t.Fatalf("case %d: detector = %v, model = %v", i, detectorResult, modelResult)
		}
	}
}

// TestPCCMSecurityConfigFromEnv 验证环境变量覆盖与回退行为。
func TestPCCMSecurityConfigFromEnv(t *testing.T) {
	defaultConfig := DefaultPCCMSecurityConfig()

	// 未设置任何环境变量时完全回退到默认配置。
	envConfig := PCCMSecurityConfigFromEnv()
	if envConfig.EnhancementPerExtraLayer != defaultConfig.EnhancementPerExtraLayer ||
		envConfig.LayerActivationThreshold != defaultConfig.LayerActivationThreshold {
		t.Fatalf("env config without overrides = %+v, want defaults %+v", envConfig, defaultConfig)
	}

	t.Setenv("PCCM_LAYER_WEIGHT_1", "0.5")
	t.Setenv("PCCM_ENHANCEMENT_PER_EXTRA_LAYER", "0.02")
	t.Setenv("PCCM_LAYER_ACTIVATION_THRESHOLD", "0.4")

	envConfig = PCCMSecurityConfigFromEnv()
	if envConfig.LayerWeights[1] != 0.5 {
		t.Fatalf("layer 1 weight = %v, want 0.5", envConfig.LayerWeights[1])
	}
	// 未覆盖的层保留默认权重。
	if envConfig.LayerWeights[2] != defaultConfig.LayerWeights[2] {
		t.Fatalf("layer 2 weight = %v, want default %v", envConfig.LayerWeights[2], defaultConfig.LayerWeights[2])
	}
	if envConfig.EnhancementPerExtraLayer != 0.02 {
		t.Fatalf("enhancement = %v, want 0.02", envConfig.EnhancementPerExtraLayer)
	}
	if envConfig.LayerActivationThreshold != 0.4 {
		t.Fatalf("activation threshold = %v, want 0.4", envConfig.LayerActivationThreshold)
	}

	// 非法数值必须被忽略，回退到默认值。
	t.Setenv("PCCM_ENHANCEMENT_PER_EXTRA_LAYER", "not-a-number")
	envConfig = PCCMSecurityConfigFromEnv()
	if envConfig.EnhancementPerExtraLayer != defaultConfig.EnhancementPerExtraLayer {
		t.Fatalf("invalid env value should fall back, got %v", envConfig.EnhancementPerExtraLayer)
	}
}

// TestMultiLayerDetectorSetLayerWeightUsesPCCMAsSingleSource 固化修复：
// SetLayerWeight 必须作用到 PCCM 模型（生产默认路径），而非只改遗留的
// weights 后备映射。此前调用 SetLayerWeight 在生产路径上静默失效。
func TestMultiLayerDetectorSetLayerWeightUsesPCCMAsSingleSource(t *testing.T) {
	detector := NewMultiLayerDetector("http://127.0.0.1:1", "unused")

	if got := detector.GetLayerWeight(1); got != 0.70 {
		t.Fatalf("default layer 1 weight = %v, want 0.70", got)
	}

	detector.SetLayerWeight(1, 0.33)
	if got := detector.GetLayerWeight(1); got != 0.33 {
		t.Fatalf("after SetLayerWeight(1, 0.33), GetLayerWeight(1) = %v, want 0.33", got)
	}

	// 模型（单一真源）必须同步更新。
	model := detector.GetPCCMModel()
	if got := model.GetLayerWeight(1); got != 0.33 {
		t.Fatalf("model weight for layer 1 = %v, want 0.33 (single source not updated)", got)
	}

	// 置信度计算必须反映新权重：只激活 Layer 1（置信度 0.9）时，
	// 单层不增强，最终置信度应等于该层置信度本身（与权重无关），
	// 因此改用两层验证权重确实进入归一化加权。
	detector.SetLayerWeight(2, 0.0)
	conf := detector.calculateConfidence([]LayerResult{
		{LayerID: 1, Confidence: 0.9},
		{LayerID: 2, Confidence: 0.3},
	})
	// Layer 1 权重 0.33、Layer 2 权重 0 → 加权平均 = 0.9（Layer 2 权重为 0 不贡献）。
	assertFloatEqual(t, "weighted confidence with layer2 weight 0", conf, 0.9)
}
