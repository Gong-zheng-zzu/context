package calibration

// 本文件提供**合成**标注开发集，仅用于校准流水线的自测与演示。
//
// 重要声明：
//   - 数据是人工构造的合成样本（Source = DevSetSourceSynthetic），规模很小（11 条），
//     不代表真实业务分布，也绝不能作为任何准确率/提升幅度的对外结论；
//   - 真实指标必须在真实服务环境、用真实带标注数据集、以相同配置跑出；
//   - 样本只包含分层置信度与二分类标注，用于验证「离线开发集权重校准」流程本身。
const (
	// SyntheticDevSetName 是内置合成开发集的名称。
	SyntheticDevSetName = "pccm_calibration_synthetic_devset_v1"
	// SyntheticDecisionThreshold 是合成集在其设计意图下使用的决策阈值。
	SyntheticDecisionThreshold = 0.8
)

// SyntheticDevSet 返回内置的合成标注开发集。
//
// 设计意图：Layer 1（正则）容易产生误报，Layer 5（高精度确认层）在真阳性上
// 与 Layer 1 同样活跃、在误报上则明显偏低。这使层权重成为可被离线搜索优化的
// 有效变量，从而能自测坐标下降确实收敛到更好的目标值。
func SyntheticDevSet() DevSet {
	return DevSet{
		Name:   SyntheticDevSetName,
		Source: DevSetSourceSynthetic,
		Notes:  "合成标注集，仅用于离线校准流水线自测；规模 11 条，不代表真实评测集。",
		Samples: []LabeledSample{
			{ID: "syn-pos-01", ExpectedSensitive: true, SensitiveType: "id_card", LayerConfidences: map[int]float64{1: 0.95, 5: 0.90}},
			{ID: "syn-pos-02", ExpectedSensitive: true, SensitiveType: "phone", LayerConfidences: map[int]float64{1: 0.92, 5: 0.88}},
			{ID: "syn-pos-03", ExpectedSensitive: true, SensitiveType: "phone", LayerConfidences: map[int]float64{1: 0.85}},
			{ID: "syn-pos-04", ExpectedSensitive: true, SensitiveType: "id_card", LayerConfidences: map[int]float64{1: 0.90, 2: 0.60, 5: 0.85}},
			{ID: "syn-pos-05", ExpectedSensitive: true, SensitiveType: "medical_record", LayerConfidences: map[int]float64{1: 0.88, 4: 0.75, 5: 0.80}},
			{ID: "syn-pos-06", ExpectedSensitive: true, SensitiveType: "blood_pressure", LayerConfidences: map[int]float64{4: 0.90, 5: 0.85}},
			{ID: "syn-neg-01", ExpectedSensitive: false, SensitiveType: "id_card", LayerConfidences: map[int]float64{1: 0.95, 5: 0.10}},
			{ID: "syn-neg-02", ExpectedSensitive: false, SensitiveType: "phone", LayerConfidences: map[int]float64{1: 0.90, 5: 0.05}},
			{ID: "syn-neg-03", ExpectedSensitive: false, SensitiveType: "id_card", LayerConfidences: map[int]float64{1: 0.93, 5: 0.15}},
			{ID: "syn-neg-04", ExpectedSensitive: false, SensitiveType: "phone", LayerConfidences: map[int]float64{1: 0.70, 5: 0.10}},
			{ID: "syn-neg-05", ExpectedSensitive: false, SensitiveType: "id_card", LayerConfidences: map[int]float64{1: 0.95, 2: 0.20, 5: 0.08}},
		},
	}
}

// SyntheticCalibrationOptions 返回适配合成开发集的搜索配置。
//
// 候选权重刻意不包含能把 Layer 1 单独压到阈值以下的极端值，从而要求搜索
// 同时调整 Layer 1 与 Layer 5 才能达标，用以验证坐标下降的多层协同能力。
func SyntheticCalibrationOptions() CalibrationOptions {
	options := DefaultCalibrationOptions()
	options.DecisionThreshold = SyntheticDecisionThreshold
	options.WeightCandidates = map[int][]float64{
		1: {0.4, 0.5, 0.6, 0.7, 0.8},
		2: {0.05, 0.1, 0.15, 0.2},
		4: {0.05, 0.1, 0.15, 0.2},
		5: {0.1, 0.2, 0.3, 0.4, 0.5},
	}
	return options
}
