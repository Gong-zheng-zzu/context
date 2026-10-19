package security

import (
	"fmt"
	"math"
	"sort"
	"sync"
)

// ============================================================================
// 基于 Pearson 积矩相关的置信度融合
// ============================================================================
//
// 实现策划书 2.3.2 节"写入级前置免疫"中描述的融合决策方式：
//
//	"系统并行调度五层检测矩阵…… 各层输出其独立的风险判决概率 C_j(x)，
//	 最后由基于 Pearson 积矩相关的 PCCM 置信度融合决策模型进行多维特征
//	 相关性矩阵计算，得出综合风险评分。"
//
// 动机
// ----
// 简单的加权平均隐含假设"各层贡献相互独立"。但在真实系统中，多层检测器
// 高度相关（例如正则层与字典层在身份证号上几乎同时命中），此时多个高相关
// 层的加权和会把"同一份证据"重复计算，导致风险评分虚高。
//
// 本引擎先用一批样本估计**层间相关性矩阵**，再据此对权重做冗余惩罚：
//
//	λ_i   = 1 + α · Σ_{j≠i} |ρ_ij| · ŵ_j        （层 i 的冗余惩罚因子）
//	w'_i  = w_i / λ_i                            （惩罚后的有效权重）
//
// 其中 ŵ_j 是归一化后的基准权重，α ∈ [0,1] 为惩罚强度（默认 0.5）。
// 与高权重层强相关的层被下调——它提供的边际独立信息最少。
//
// 与 PCCM-S（ProgressiveConfidenceModel）的关系
// -------------------------------------------
// 两者是**互补**而非替代：
//   - PCCM-S 负责"归一化加权 + 多层协同增强"，回答"多层同时命中时该不该加信"；
//   - 本引擎负责"层间相关性校正"，回答"这些层是不是在重复同一份证据"。
//
// 因此本引擎输出的权重矩阵可直接作为 PCCM-S 的 LayerWeights 输入，
// 但不会修改 PCCM-S 既有的加权与增强逻辑。
//
// 边界声明：相关性矩阵由**离线样本**估计，属统计量而非先验真理；
// 样本量不足或某层恒定输出时，相关值不可靠，此时引擎自动退化为
// 使用基准权重（不做任何惩罚），并在 CorrelationMatrix 中标记退化项。

// minSamplesForCorrelation 估计相关系数所需的最小样本量。
// 少于该值时相关性估计噪声过大，退化为不做惩罚。
const minSamplesForCorrelation = 5

// PearsonCorrelation 计算两个等长向量的皮尔逊积矩相关系数 ρ ∈ [-1, 1]。
//
// 使用两遍算法（先求均值再求离差），相比朴素单遍公式数值更稳定。
//
// 退化情形显式返回错误而非静默给出 0，避免调用方把"无法计算"
// 误读为"无相关性"：
//   - 长度不一致或样本数为 0；
//   - 任一向量方差为 0（恒定输出，相关系数在数学上未定义）。
func PearsonCorrelation(x, y []float64) (float64, error) {
	if len(x) != len(y) {
		return 0, fmt.Errorf("pearson: length mismatch %d vs %d", len(x), len(y))
	}
	n := len(x)
	if n == 0 {
		return 0, fmt.Errorf("pearson: empty vectors")
	}

	var sumX, sumY float64
	for i := 0; i < n; i++ {
		sumX += x[i]
		sumY += y[i]
	}
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)

	var covariance, varianceX, varianceY float64
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		covariance += dx * dy
		varianceX += dx * dx
		varianceY += dy * dy
	}

	// 方差为 0 ⇒ 该层在所有样本上输出恒定，相关性未定义。
	// 用相对阈值判断，避免浮点误差把"几乎恒定"误判为有方差。
	const varianceEpsilon = 1e-12
	if varianceX <= varianceEpsilon || varianceY <= varianceEpsilon {
		return 0, fmt.Errorf("pearson: zero variance (layer output is constant)")
	}

	rho := covariance / math.Sqrt(varianceX*varianceY)
	// 浮点误差可能让结果略微越界，夹回 [-1, 1]。
	if rho > 1 {
		rho = 1
	} else if rho < -1 {
		rho = -1
	}
	return rho, nil
}

// CorrelationMatrix 多维特征（检测层）相关性矩阵。
type CorrelationMatrix struct {
	// LayerIDs 参与计算的层 ID，升序排列。
	LayerIDs []int `json:"layer_ids"`
	// Matrix[i][j] 是 LayerIDs[i] 与 LayerIDs[j] 的皮尔逊相关系数。
	// 对角线恒为 1；对称。
	Matrix [][]float64 `json:"matrix"`
	// SampleCount 估计相关性所用的样本数。
	SampleCount int `json:"sample_count"`
	// Undefined 记录无法计算相关性的层对（如某层输出恒定）。
	Undefined []LayerPair `json:"undefined,omitempty"`
	// Degraded 为 true 表示样本量不足，矩阵仅供诊断、不应用于权重调整。
	Degraded bool `json:"degraded"`
}

// LayerPair 一对检测层。
type LayerPair struct {
	LayerA int `json:"layer_a"`
	LayerB int `json:"layer_b"`
}

// Correlation 返回指定两层之间的相关系数；不存在时返回 0 与 false。
func (cm *CorrelationMatrix) Correlation(layerA, layerB int) (float64, bool) {
	if cm == nil {
		return 0, false
	}
	indexA, indexB := -1, -1
	for i, id := range cm.LayerIDs {
		if id == layerA {
			indexA = i
		}
		if id == layerB {
			indexB = i
		}
	}
	if indexA < 0 || indexB < 0 {
		return 0, false
	}
	return cm.Matrix[indexA][indexB], true
}

// BuildCorrelationMatrix 从逐样本的各层判决概率估计相关性矩阵。
//
// samples 中每个元素是一条样本的「层 ID → 判决概率」。某层在该样本中
// 缺失时按 0 处理——语义是"该层未输出风险判决"，而非"该层缺席"。
func BuildCorrelationMatrix(samples []map[int]float64) (*CorrelationMatrix, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("pearson: no samples provided")
	}

	// 收集所有出现过的层 ID。
	layerSet := make(map[int]struct{})
	for _, sample := range samples {
		for layerID := range sample {
			layerSet[layerID] = struct{}{}
		}
	}
	if len(layerSet) < 2 {
		return nil, fmt.Errorf("pearson: need at least 2 layers, got %d", len(layerSet))
	}

	layerIDs := make([]int, 0, len(layerSet))
	for layerID := range layerSet {
		layerIDs = append(layerIDs, layerID)
	}
	sort.Ints(layerIDs)

	// 展开为每层一条观测序列。
	series := make(map[int][]float64, len(layerIDs))
	for _, layerID := range layerIDs {
		values := make([]float64, len(samples))
		for i, sample := range samples {
			values[i] = sample[layerID] // 缺失即为 0
		}
		series[layerID] = values
	}

	size := len(layerIDs)
	matrix := make([][]float64, size)
	for i := range matrix {
		matrix[i] = make([]float64, size)
		matrix[i][i] = 1
	}

	var undefined []LayerPair
	for i := 0; i < size; i++ {
		for j := i + 1; j < size; j++ {
			rho, err := PearsonCorrelation(series[layerIDs[i]], series[layerIDs[j]])
			if err != nil {
				// 记下退化层对，相关值按 0 处理（等价于"无冗余"），
				// 使该层对不受到惩罚——宁可少惩罚也不误伤。
				undefined = append(undefined, LayerPair{LayerA: layerIDs[i], LayerB: layerIDs[j]})
				rho = 0
			}
			matrix[i][j] = rho
			matrix[j][i] = rho
		}
	}

	return &CorrelationMatrix{
		LayerIDs:    layerIDs,
		Matrix:      matrix,
		SampleCount: len(samples),
		Undefined:   undefined,
		Degraded:    len(samples) < minSamplesForCorrelation,
	}, nil
}

// RedundancyPenalties 计算各层的冗余惩罚因子 λ_i = 1 + α·Σ_{j≠i}|ρ_ij|·ŵ_j。
//
// baseWeights 是基准权重（如 PCCM-S 的 LayerWeights）；只有出现在
// 相关性矩阵中的层才会被计算。矩阵退化（样本不足）时全部返回 1，
// 即不做任何调整。
func (cm *CorrelationMatrix) RedundancyPenalties(baseWeights map[int]float64, damping float64) map[int]float64 {
	// 参与惩罚计算的层 = 相关性矩阵中的层 ∪ 基准权重中的层。
	// 后者中不在矩阵里的层（如本轮样本未覆盖）保持 λ=1，即不调整——
	// 不能因为"没估计到"就把它从融合中剔除，那会静默改变融合语义。
	layerSet := make(map[int]struct{}, len(cm.LayerIDs)+len(baseWeights))
	for _, id := range cm.LayerIDs {
		layerSet[id] = struct{}{}
	}
	for id := range baseWeights {
		layerSet[id] = struct{}{}
	}

	penalties := make(map[int]float64, len(layerSet))
	for id := range layerSet {
		penalties[id] = 1.0
	}

	if cm.Degraded || damping <= 0 || len(cm.LayerIDs) == 0 {
		return penalties
	}
	if damping > 1 {
		damping = 1
	}

	// 归一化基准权重，使惩罚强度与权重量纲无关。
	total := 0.0
	for id := range layerSet {
		if w := baseWeights[id]; w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return penalties
	}

	for i, layerID := range cm.LayerIDs {
		sum := 0.0
		for j, otherID := range cm.LayerIDs {
			if i == j {
				continue
			}
			weight := baseWeights[otherID]
			if weight <= 0 {
				continue
			}
			sum += math.Abs(cm.Matrix[i][j]) * (weight / total)
		}
		penalties[layerID] = 1 + damping*sum
	}
	return penalties
}

// RedundancyAdjustedWeights 依据相关性矩阵给出冗余校正后的权重。
//
// 返回的权重已重新归一化（各层之和为 1），便于直接作为 PCCM-S 的
// LayerWeights 使用。仅归一化**输入权重中为正**的层。
func (cm *CorrelationMatrix) RedundancyAdjustedWeights(baseWeights map[int]float64, damping float64) map[int]float64 {
	penalties := cm.RedundancyPenalties(baseWeights, damping)

	adjusted := make(map[int]float64, len(baseWeights))
	total := 0.0
	for layerID, base := range baseWeights {
		if base <= 0 {
			continue
		}
		value := base / penalties[layerID]
		adjusted[layerID] = value
		total += value
	}
	if total <= 0 {
		return map[int]float64{}
	}
	for layerID := range adjusted {
		adjusted[layerID] /= total
	}
	return adjusted
}

// ============================================================================
// 融合引擎
// ============================================================================

// PearsonFusionConfig 融合引擎配置。
type PearsonFusionConfig struct {
	// RedundancyDamping 冗余惩罚强度 α ∈ [0, 1]，默认 0.5。
	// 0 表示关闭惩罚（退化为纯加权平均）。
	RedundancyDamping float64
	// BaseWeights 基准层权重；为空时使用 PCCM-S 的默认权重。
	BaseWeights map[int]float64
}

// DefaultPearsonFusionConfig 返回默认配置。
func DefaultPearsonFusionConfig() PearsonFusionConfig {
	return PearsonFusionConfig{
		RedundancyDamping: 0.5,
		BaseWeights:       map[int]float64{1: 0.70, 2: 0.10, 4: 0.15, 5: 0.05},
	}
}

// PearsonFusionEngine 基于 Pearson 积矩相关的置信度融合引擎。
//
// 并发安全。先用 Fit 在样本上估计层间相关性，随后 Fuse 即可在
// 相关性校正后的权重上做加权融合。
type PearsonFusionEngine struct {
	mu          sync.RWMutex
	config      PearsonFusionConfig
	correlation *CorrelationMatrix
}

// NewPearsonFusionEngine 创建融合引擎。
func NewPearsonFusionEngine(config PearsonFusionConfig) *PearsonFusionEngine {
	if config.RedundancyDamping <= 0 && config.BaseWeights == nil {
		config = DefaultPearsonFusionConfig()
	}
	if config.BaseWeights == nil {
		config.BaseWeights = DefaultPearsonFusionConfig().BaseWeights
	}
	return &PearsonFusionEngine{config: config}
}

// Fit 用一批样本估计层间相关性矩阵。
//
// 失败时不改变引擎既有状态——避免一次坏样本把已经标定好的矩阵打坏。
func (e *PearsonFusionEngine) Fit(samples []map[int]float64) error {
	matrix, err := BuildCorrelationMatrix(samples)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.correlation = matrix
	return nil
}

// CorrelationMatrix 返回当前相关性矩阵快照。
func (e *PearsonFusionEngine) CorrelationMatrix() *CorrelationMatrix {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.correlation
}

// EffectiveWeights 返回当前生效的（冗余校正后）层权重。
// 尚未 Fit 时返回基准权重的归一化结果。
func (e *PearsonFusionEngine) EffectiveWeights() map[int]float64 {
	e.mu.RLock()
	matrix := e.correlation
	config := e.config
	e.mu.RUnlock()

	if matrix == nil {
		return normalizeWeights(config.BaseWeights)
	}
	adjusted := matrix.RedundancyAdjustedWeights(config.BaseWeights, config.RedundancyDamping)
	if len(adjusted) == 0 {
		return normalizeWeights(config.BaseWeights)
	}
	return adjusted
}

// Fuse 融合单条样本的各层判决概率。
//
// 融合方式与 PCCM-S 保持一致的语义（在"有输出"的层上归一化加权），
// 区别仅在于权重经过相关性校正：与高权重层强相关的层被下调。
func (e *PearsonFusionEngine) Fuse(layerConfidences map[int]float64) float64 {
	weights := e.EffectiveWeights()

	weightedSum := 0.0
	activeWeight := 0.0
	for layerID, confidence := range layerConfidences {
		weight, ok := weights[layerID]
		if !ok || weight <= 0 {
			continue
		}
		weightedSum += weight * confidence
		activeWeight += weight
	}
	if activeWeight <= 0 {
		return 0
	}
	return weightedSum / activeWeight
}

// normalizeWeights 归一化权重到和为 1；全为非正值时返回空表。
func normalizeWeights(weights map[int]float64) map[int]float64 {
	total := 0.0
	for _, weight := range weights {
		if weight > 0 {
			total += weight
		}
	}
	if total <= 0 {
		return map[int]float64{}
	}
	out := make(map[int]float64, len(weights))
	for layerID, weight := range weights {
		if weight > 0 {
			out[layerID] = weight / total
		}
	}
	return out
}
