// Package calibration 提供安全侧 PCCM-S 的「离线开发集权重校准」能力。
//
// 边界声明（重要）：
//   - 这里的“校准”指在带标注的离线开发集上，对固定搜索空间做坐标下降/网格搜索，
//     产出一个可版本化的权重配置产物；
//   - 它不是运行时自适应训练，也不是在线学习，权重不会在服务运行过程中被修改；
//   - 默认配置始终是 security.DefaultPCCMSecurityConfig()（CalibrationSource =
//     "fixed_unvalidated"）；校准产物只有在调用方显式加载并注入检测器时才生效；
//   - 任何指标数字都必须在真实评测环境用同配置、同数据集跑出，本包不预置也不声称成绩。
//
// 复用 security 包已导出的 PCCMSecurityConfig / ProgressiveConfidenceModel 作为
// 唯一实现源，不复制第二套公式。
package calibration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/contextkeeper/service/internal/security"
)

const (
	// CalibrationSourceDevSetV1 标识权重来自 v1 离线开发集校准产物。
	CalibrationSourceDevSetV1 = "devset_calibrated_v1"
	// CalibratedConfigVersion 是校准产物中配置的版本号前缀。
	CalibratedConfigVersion = "pccm-s-devset-v1"

	// ObjectiveF1 以 F1 为目标函数（越大越好）。
	ObjectiveF1 = "f1"
	// ObjectiveErrorWeighted 以加权误报率+漏报率的负值为目标函数（越大越好）。
	ObjectiveErrorWeighted = "error_weighted"

	// DevSetSourceSynthetic 标注该开发集为合成数据（仅用于流水线自测）。
	DevSetSourceSynthetic = "synthetic"
	// DevSetSourceRepository 标注该开发集来自仓库既有数据。
	DevSetSourceRepository = "repository"

	searchMethodCoordinateDescent = "coordinate_descent"
	searchScoreEpsilon            = 1e-12
	toolIdentifier                = "internal/security/calibration"
)

// LabeledSample 是校准流水线的一条标注样本。
//
// LayerConfidences 是调用方按 PCCM-S 层 ID 预先计算好的单层置信度
// （1=正则, 2=词典, 4=上下文规则, 5=LLM）。把分层结果与标注一起作为输入，
// 使校准可离线复现，无需在校准过程中启动完整检测栈。
type LabeledSample struct {
	ID                string          `json:"id"`
	Text              string          `json:"text,omitempty"`
	SensitiveType     string          `json:"sensitive_type,omitempty"`
	ExpectedSensitive bool            `json:"expected_sensitive"`
	LayerConfidences  map[int]float64 `json:"layer_confidences"`
}

// DevSet 是带标注的开发集。Source 必须如实标注数据来源（合成/仓库/自建）。
type DevSet struct {
	Name    string          `json:"name"`
	Source  string          `json:"source"`
	Notes   string          `json:"notes,omitempty"`
	Samples []LabeledSample `json:"samples"`
}

// Hash 返回开发集内容的稳定哈希（按样本 ID 排序后对样本做 SHA-256）。
// 用于校准指纹，保证产物可追溯到确切的输入数据集。
func (d DevSet) Hash() string {
	ordered := make([]LabeledSample, len(d.Samples))
	copy(ordered, d.Samples)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	payload, err := json.Marshal(ordered)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Metrics 是二分类（敏感/非敏感）评测指标快照。
type Metrics struct {
	Total             int     `json:"total"`
	TruePositives     int     `json:"true_positives"`
	FalsePositives    int     `json:"false_positives"`
	TrueNegatives     int     `json:"true_negatives"`
	FalseNegatives    int     `json:"false_negatives"`
	Precision         float64 `json:"precision"`
	Recall            float64 `json:"recall"`
	F1                float64 `json:"f1"`
	FalsePositiveRate float64 `json:"false_positive_rate"`
	FalseNegativeRate float64 `json:"false_negative_rate"`
	Accuracy          float64 `json:"accuracy"`
}

// Evaluate 用给定 PCCM-S 配置与决策阈值在开发集上计算指标。
//
// decisionThreshold <= 0 时回退到配置自身的 DecisionThreshold。
func Evaluate(config security.PCCMSecurityConfig, samples []LabeledSample, decisionThreshold float64) Metrics {
	if decisionThreshold <= 0 {
		decisionThreshold = config.DecisionThreshold
	}
	model := security.NewProgressiveConfidenceModelWithConfig(config)

	metrics := Metrics{Total: len(samples)}
	for _, sample := range samples {
		final := model.CalculateFinalConfidence(sample.LayerConfidences)
		predicted := final >= decisionThreshold
		switch {
		case sample.ExpectedSensitive && predicted:
			metrics.TruePositives++
		case !sample.ExpectedSensitive && predicted:
			metrics.FalsePositives++
		case !sample.ExpectedSensitive && !predicted:
			metrics.TrueNegatives++
		case sample.ExpectedSensitive && !predicted:
			metrics.FalseNegatives++
		}
	}

	metrics.Precision = ratio(metrics.TruePositives, metrics.TruePositives+metrics.FalsePositives)
	metrics.Recall = ratio(metrics.TruePositives, metrics.TruePositives+metrics.FalseNegatives)
	if metrics.Precision+metrics.Recall > 0 {
		metrics.F1 = 2 * metrics.Precision * metrics.Recall / (metrics.Precision + metrics.Recall)
	}
	metrics.FalsePositiveRate = ratio(metrics.FalsePositives, metrics.FalsePositives+metrics.TrueNegatives)
	metrics.FalseNegativeRate = ratio(metrics.FalseNegatives, metrics.FalseNegatives+metrics.TruePositives)
	metrics.Accuracy = ratio(metrics.TruePositives+metrics.TrueNegatives, metrics.Total)
	return metrics
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// objectiveScore 把指标映射为「越大越好」的标量。
func objectiveScore(objective string, metrics Metrics, fpPenalty, fnPenalty float64) float64 {
	if objective == ObjectiveErrorWeighted {
		return -(fpPenalty*metrics.FalsePositiveRate + fnPenalty*metrics.FalseNegativeRate)
	}
	return metrics.F1
}

// SearchStep 记录一次成功的坐标下降改进。
type SearchStep struct {
	Round           int     `json:"round"`
	LayerID         int     `json:"layer_id"`
	PreviousWeight  float64 `json:"previous_weight"`
	CandidateWeight float64 `json:"candidate_weight"`
	Score           float64 `json:"score"`
}

// SearchSummary 描述本次搜索的空间与过程，作为校准指纹的一部分。
type SearchSummary struct {
	Method                   string       `json:"method"`
	Rounds                   int          `json:"rounds"`
	Evaluations              int          `json:"evaluations"`
	ImprovedSteps            []SearchStep `json:"improved_steps"`
	SearchSpace              string       `json:"search_space"`
	Objective                string       `json:"objective"`
	FalsePositivePenalty     float64      `json:"false_positive_penalty"`
	FalseNegativePenalty     float64      `json:"false_negative_penalty"`
	DecisionThreshold        float64      `json:"decision_threshold"`
	LayerActivationThreshold float64      `json:"layer_activation_threshold"`
	EnhancementPerExtraLayer float64      `json:"enhancement_per_extra_layer"`
}

// CalibrationOptions 控制离线校准的搜索空间与目标函数。
type CalibrationOptions struct {
	// Objective 为目标函数名：ObjectiveF1 或 ObjectiveErrorWeighted。
	Objective string `json:"objective"`
	// FalsePositivePenalty / FalseNegativePenalty 仅在 ObjectiveErrorWeighted 下生效。
	FalsePositivePenalty float64 `json:"false_positive_penalty"`
	FalseNegativePenalty float64 `json:"false_negative_penalty"`
	// LayerIDs 参与搜索的层；未列出的层保持 BaseConfig 权重。
	LayerIDs []int `json:"layer_ids"`
	// WeightCandidates 每层的候选权重集合（网格）。
	WeightCandidates map[int][]float64 `json:"weight_candidates"`
	// MaxRounds 坐标下降的最大轮数。
	MaxRounds int `json:"max_rounds"`
	// DecisionThreshold / LayerActivationThreshold / EnhancementPerExtraLayer
	// 在校准期间固定，不参与搜索（避免同时搜索过多维度）。
	DecisionThreshold        float64 `json:"decision_threshold"`
	LayerActivationThreshold float64 `json:"layer_activation_threshold"`
	EnhancementPerExtraLayer float64 `json:"enhancement_per_extra_layer"`
	// BaseConfig 是搜索起点，也是未校准时的对照配置。
	BaseConfig security.PCCMSecurityConfig `json:"base_config"`
}

// DefaultCalibrationOptions 返回一套保守的默认搜索空间（覆盖 4 个 PCCM-S 层）。
func DefaultCalibrationOptions() CalibrationOptions {
	base := security.DefaultPCCMSecurityConfig()
	return CalibrationOptions{
		Objective:            ObjectiveF1,
		FalsePositivePenalty: 1.0,
		FalseNegativePenalty: 1.0,
		LayerIDs:             []int{1, 2, 4, 5},
		WeightCandidates: map[int][]float64{
			1: {0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9},
			2: {0.0, 0.05, 0.1, 0.15, 0.2, 0.3},
			4: {0.05, 0.1, 0.15, 0.2, 0.25, 0.3},
			5: {0.0, 0.05, 0.1, 0.15, 0.2},
		},
		MaxRounds:                4,
		DecisionThreshold:        base.DecisionThreshold,
		LayerActivationThreshold: base.LayerActivationThreshold,
		EnhancementPerExtraLayer: base.EnhancementPerExtraLayer,
		BaseConfig:               base,
	}
}

// withDefaults 填充零值字段，保证调用方可以只覆盖关心的部分。
func (o CalibrationOptions) withDefaults() CalibrationOptions {
	def := DefaultCalibrationOptions()
	if o.Objective == "" {
		o.Objective = def.Objective
	}
	if o.FalsePositivePenalty <= 0 {
		o.FalsePositivePenalty = def.FalsePositivePenalty
	}
	if o.FalseNegativePenalty <= 0 {
		o.FalseNegativePenalty = def.FalseNegativePenalty
	}
	if len(o.LayerIDs) == 0 {
		o.LayerIDs = def.LayerIDs
	}
	if len(o.WeightCandidates) == 0 {
		o.WeightCandidates = def.WeightCandidates
	}
	if o.MaxRounds <= 0 {
		o.MaxRounds = def.MaxRounds
	}
	if o.BaseConfig.LayerWeights == nil {
		o.BaseConfig = def.BaseConfig
	}
	if o.DecisionThreshold <= 0 {
		o.DecisionThreshold = o.BaseConfig.DecisionThreshold
	}
	if o.DecisionThreshold <= 0 {
		o.DecisionThreshold = def.DecisionThreshold
	}
	if o.LayerActivationThreshold <= 0 {
		o.LayerActivationThreshold = def.LayerActivationThreshold
	}
	if o.EnhancementPerExtraLayer <= 0 {
		o.EnhancementPerExtraLayer = def.EnhancementPerExtraLayer
	}
	return o
}

func (o CalibrationOptions) validate() error {
	if o.Objective != ObjectiveF1 && o.Objective != ObjectiveErrorWeighted {
		return fmt.Errorf("不支持的目标函数 %q（可选: %s, %s）", o.Objective, ObjectiveF1, ObjectiveErrorWeighted)
	}
	for layerID, candidates := range o.WeightCandidates {
		for _, candidate := range candidates {
			if math.IsNaN(candidate) || math.IsInf(candidate, 0) || candidate < 0 {
				return fmt.Errorf("层 %d 的候选权重非法: %v", layerID, candidate)
			}
		}
	}
	return nil
}

// SearchWeights 在开发集上用坐标下降搜索一组 PCCM-S 层权重。
//
// 返回：最优配置、搜索摘要、最优指标。搜索保证目标函数单调非降，
// 且在无改进时提前收敛（最多 MaxRounds 轮）。
func SearchWeights(devSet DevSet, options CalibrationOptions) (security.PCCMSecurityConfig, SearchSummary, Metrics, error) {
	options = options.withDefaults()
	if err := options.validate(); err != nil {
		return security.PCCMSecurityConfig{}, SearchSummary{}, Metrics{}, err
	}
	if len(devSet.Samples) == 0 {
		return security.PCCMSecurityConfig{}, SearchSummary{}, Metrics{}, fmt.Errorf("开发集为空，无法校准")
	}

	weights := startingWeights(options)
	bestConfig := buildConfig(options, weights)
	bestMetrics := Evaluate(bestConfig, devSet.Samples, options.DecisionThreshold)
	bestScore := objectiveScore(options.Objective, bestMetrics, options.FalsePositivePenalty, options.FalseNegativePenalty)

	summary := SearchSummary{
		Method:                   searchMethodCoordinateDescent,
		SearchSpace:              describeSearchSpace(options),
		Objective:                options.Objective,
		FalsePositivePenalty:     options.FalsePositivePenalty,
		FalseNegativePenalty:     options.FalseNegativePenalty,
		DecisionThreshold:        options.DecisionThreshold,
		LayerActivationThreshold: options.LayerActivationThreshold,
		EnhancementPerExtraLayer: options.EnhancementPerExtraLayer,
	}

	rounds := 0
	for round := 0; round < options.MaxRounds; round++ {
		improvedThisRound := 0
		for _, layerID := range options.LayerIDs {
			candidates := options.WeightCandidates[layerID]
			if len(candidates) == 0 {
				continue
			}
			previousWeight := weights[layerID]
			bestLocalWeight := previousWeight
			bestLocalScore := bestScore
			bestLocalMetrics := bestMetrics

			for _, candidate := range candidates {
				if candidate == previousWeight {
					continue
				}
				trial := cloneWeights(weights)
				trial[layerID] = candidate
				trialConfig := buildConfig(options, trial)
				trialMetrics := Evaluate(trialConfig, devSet.Samples, options.DecisionThreshold)
				summary.Evaluations++
				score := objectiveScore(options.Objective, trialMetrics, options.FalsePositivePenalty, options.FalseNegativePenalty)
				if score > bestLocalScore+searchScoreEpsilon {
					bestLocalScore = score
					bestLocalWeight = candidate
					bestLocalMetrics = trialMetrics
				}
			}

			if bestLocalWeight != previousWeight {
				weights[layerID] = bestLocalWeight
				bestScore = bestLocalScore
				bestMetrics = bestLocalMetrics
				summary.ImprovedSteps = append(summary.ImprovedSteps, SearchStep{
					Round:           round + 1,
					LayerID:         layerID,
					PreviousWeight:  previousWeight,
					CandidateWeight: bestLocalWeight,
					Score:           bestLocalScore,
				})
				improvedThisRound++
			}
		}
		rounds = round + 1
		if improvedThisRound == 0 {
			break
		}
	}
	summary.Rounds = rounds

	bestConfig = buildConfig(options, weights)
	bestMetrics = Evaluate(bestConfig, devSet.Samples, options.DecisionThreshold)
	return bestConfig, summary, bestMetrics, nil
}

// startingWeights 以 BaseConfig 权重为搜索起点，缺失层回退到内置默认权重。
func startingWeights(options CalibrationOptions) map[int]float64 {
	fallback := security.DefaultPCCMSecurityConfig()
	weights := make(map[int]float64, len(options.LayerIDs))
	for _, layerID := range options.LayerIDs {
		weight, ok := options.BaseConfig.LayerWeights[layerID]
		if !ok {
			weight = fallback.LayerWeights[layerID]
		}
		weights[layerID] = weight
	}
	return weights
}

// buildConfig 用搜索结果权重构造校准后的 PCCM-S 配置（含校准来源标记）。
func buildConfig(options CalibrationOptions, weights map[int]float64) security.PCCMSecurityConfig {
	config := options.BaseConfig
	config.Version = CalibratedConfigVersion
	config.CalibrationSource = CalibrationSourceDevSetV1
	config.LayerWeights = cloneWeights(weights)
	config.EnhancementPerExtraLayer = options.EnhancementPerExtraLayer
	config.DecisionThreshold = options.DecisionThreshold
	config.LayerActivationThreshold = options.LayerActivationThreshold
	return config
}

func cloneWeights(source map[int]float64) map[int]float64 {
	cloned := make(map[int]float64, len(source))
	for layerID, weight := range source {
		cloned[layerID] = weight
	}
	return cloned
}

// describeSearchSpace 生成人类可读的搜索空间描述，写入校准指纹。
func describeSearchSpace(options CalibrationOptions) string {
	layerIDs := make([]int, len(options.LayerIDs))
	copy(layerIDs, options.LayerIDs)
	sort.Ints(layerIDs)
	parts := make([]string, 0, len(layerIDs))
	for _, layerID := range layerIDs {
		candidates := append([]float64(nil), options.WeightCandidates[layerID]...)
		sort.Float64s(candidates)
		parts = append(parts, fmt.Sprintf("layer%d=%v", layerID, candidates))
	}
	return fmt.Sprintf("coordinate_descent[%s];max_rounds=%d", joinStrings(parts, ", "), options.MaxRounds)
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += sep
		}
		result += part
	}
	return result
}

// Run 执行完整离线校准流程并返回带指纹的产物。
//
// generatedAt 由调用方传入，便于测试固定时间戳、保证产物可复现。
func Run(devSet DevSet, options CalibrationOptions, generatedAt time.Time) (Artifact, error) {
	options = options.withDefaults()
	before := Evaluate(options.BaseConfig, devSet.Samples, options.DecisionThreshold)

	bestConfig, summary, after, err := SearchWeights(devSet, options)
	if err != nil {
		return Artifact{}, err
	}

	return Artifact{
		SchemaVersion: SchemaVersion,
		Fingerprint: Fingerprint{
			CalibrationSource: CalibrationSourceDevSetV1,
			ArtifactSchema:    SchemaVersion,
			DatasetName:       devSet.Name,
			DatasetSource:     devSet.Source,
			DatasetHash:       devSet.Hash(),
			SampleCount:       len(devSet.Samples),
			GeneratedAt:       generatedAt.UTC().Format(time.RFC3339),
			Objective:         options.Objective,
			SearchSpace:       summary.SearchSpace,
			Evaluations:       summary.Evaluations,
			Tool:              toolIdentifier,
		},
		Config:        bestConfig,
		Search:        summary,
		MetricsBefore: before,
		MetricsAfter:  after,
		Notes:         "离线开发集校准产物：默认配置不变，需调用方显式加载方生效；不代表运行时自适应训练。",
	}, nil
}
