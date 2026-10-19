package multi_dimensional_retrieval

import (
	"fmt"
	"math"
	"sort"
)

// ============================================================================
// 风险感知的多源联合检索综合评分
// ============================================================================
//
// 策划书 2.3 节给出的多源联合检索综合评分公式：
//
//	Score = α·语义 + β·时序 + γ·图谱 + δ·结构化 − η·风险惩罚
//
// 与既有 RRF 融合的关系
// --------------------
// `internal/services/rrf_fusion.go` 提供的是**排名融合**（rank-based）：
// sum(weight/(K+rank))，优点是尺度无关、不需要各来源分数可比。
//
// 本文件提供的是**分数融合**（score-based）：要求四个维度的得分都已
// 归一化到 [0,1]，在此基础上做加权求和，并**减去风险惩罚项**。
//
// 二者互补而非替代：RRF 适合"各来源分数不可比"的场景；本评分器适合
// "分数已归一化、且需要把风险显式计入排序"的场景。风险惩罚是 RRF
// 天然无法表达的——RRF 只看到名次，看不到"这条结果有多敏感"。
//
// 风险惩罚的构成
// --------------
// 惩罚项 R 由四类信号合成，各自归一化到 [0,1]：
//
//	R = w_ctx·上下文风险 + w_sen·敏感密度 + w_prv·隐私成本 + w_src·来源不可信
//
// 之所以拆成四类而不是用一个笼统的"风险分"，是因为它们的**来源与
// 治理手段完全不同**：上下文风险来自 CASIA 判定、敏感密度来自内容
// 检测、隐私成本来自预算账本、来源不可信来自数据治理。混成一个数会
// 让"为什么这条被压分"无法解释。
//
// 边界声明
// --------
// 权重是**策略参数**，不是从数据中学到的最优解。默认取值只是让四路
// 证据都有发言权；若要声称"最优"，必须做门禁对照实验标定。这一点
// 与既有 defaultRRFSourceWeights 的注释口径保持一致。

// CompositeWeights 综合评分权重。
type CompositeWeights struct {
	// Alpha 语义（向量）维度权重。
	Alpha float64 `json:"alpha"`
	// Beta 时序维度权重。
	Beta float64 `json:"beta"`
	// Gamma 图谱维度权重。
	Gamma float64 `json:"gamma"`
	// Delta 结构化维度权重。
	Delta float64 `json:"delta"`
	// Eta 风险惩罚系数 η。越大表示对风险的容忍度越低。
	Eta float64 `json:"eta"`
}

// DefaultCompositeWeights 返回默认权重。
//
// 取值口径：四路证据权重之和为 1.0（语义 0.40 / 时序 0.25 / 图谱 0.20 /
// 结构化 0.15），使"无任何风险"的理想条目综合分为 1.0；η 取 0.30，
// 意味着最坏情况（R=1）会把一条满分条目压到 0.70——足以改变排序，
// 但不至于让高风险条目被完全隐藏（那会掩盖问题，不利于审计）。
func DefaultCompositeWeights() CompositeWeights {
	return CompositeWeights{
		Alpha: 0.40,
		Beta:  0.25,
		Gamma: 0.20,
		Delta: 0.15,
		Eta:   0.30,
	}
}

// RiskWeights 风险惩罚项内部各信号的权重。
type RiskWeights struct {
	// Context 上下文风险（CASIA 判定）。
	Context float64 `json:"context"`
	// Sensitive 敏感信息密度（内容检测）。
	Sensitive float64 `json:"sensitive"`
	// PrivacyCost 隐私预算消耗（预算账本）。
	PrivacyCost float64 `json:"privacy_cost"`
	// SourceUntrust 来源不可信度（数据治理标签）。
	SourceUntrust float64 `json:"source_untrust"`
}

// DefaultRiskWeights 返回默认风险权重。
//
// 四类信号权重之和为 1.0，因此 R ∈ [0,1]，惩罚幅度受 η 单点控制，
// 便于做"风险敏感度"的单一旋钮调节。
func DefaultRiskWeights() RiskWeights {
	return RiskWeights{
		Context:       0.35,
		Sensitive:     0.30,
		PrivacyCost:   0.20,
		SourceUntrust: 0.15,
	}
}

// Validate 校验权重合法性：非负、有限，且正项之和 > 0。
//
// 负权重会把"证据"变成"反证据"，通常源于配置笔误；NaN 会像传染病
// 一样污染所有候选的最终分数，必须在入口拦掉。
func (w CompositeWeights) Validate() error {
	values := map[string]float64{
		"alpha": w.Alpha, "beta": w.Beta, "gamma": w.Gamma,
		"delta": w.Delta, "eta": w.Eta,
	}
	for name, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("risk_fusion: weight %s is not finite (%v)", name, value)
		}
		if value < 0 {
			return fmt.Errorf("risk_fusion: weight %s is negative (%v)", name, value)
		}
	}
	if w.Alpha+w.Beta+w.Gamma+w.Delta <= 0 {
		return fmt.Errorf("risk_fusion: all evidence weights are zero")
	}
	return nil
}

// Validate 校验风险权重。
func (w RiskWeights) Validate() error {
	values := map[string]float64{
		"context": w.Context, "sensitive": w.Sensitive,
		"privacy_cost": w.PrivacyCost, "source_untrust": w.SourceUntrust,
	}
	total := 0.0
	for name, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("risk_fusion: risk weight %s is not finite (%v)", name, value)
		}
		if value < 0 {
			return fmt.Errorf("risk_fusion: risk weight %s is negative (%v)", name, value)
		}
		total += value
	}
	if total <= 0 {
		return fmt.Errorf("risk_fusion: all risk weights are zero")
	}
	return nil
}

// Normalized 把四路证据权重归一化到和为 1，η 保持不变。
//
// 归一化的意义：使"零风险满分条目"的综合分恒为 1.0，
// 从而让 η 的语义保持"最坏情况下扣多少分"，与权重缩放解耦。
func (w CompositeWeights) Normalized() CompositeWeights {
	total := w.Alpha + w.Beta + w.Gamma + w.Delta
	if total <= 0 {
		return w
	}
	return CompositeWeights{
		Alpha: w.Alpha / total,
		Beta:  w.Beta / total,
		Gamma: w.Gamma / total,
		Delta: w.Delta / total,
		Eta:   w.Eta,
	}
}

// DimensionScores 四个检索维度的得分，均应归一化到 [0,1]。
type DimensionScores struct {
	Semantic   float64 `json:"semantic"`
	Temporal   float64 `json:"temporal"`
	Graph      float64 `json:"graph"`
	Structured float64 `json:"structured"`
}

// RiskSignals 四类风险信号，均应归一化到 [0,1]。
type RiskSignals struct {
	Context       float64 `json:"context"`
	Sensitive     float64 `json:"sensitive"`
	PrivacyCost   float64 `json:"privacy_cost"`
	SourceUntrust float64 `json:"source_untrust"`
}

// ScoreBreakdown 综合评分的逐项分解，用于审计"为什么是这个分"。
type ScoreBreakdown struct {
	// Composite 最终综合评分（已施加风险惩罚，下限截为 0）。
	Composite float64 `json:"composite"`
	// EvidenceScore 惩罚前的加权证据分 α·s + β·t + γ·g + δ·d。
	EvidenceScore float64 `json:"evidence_score"`
	// RiskValue 合成的风险值 R ∈ [0,1]。
	RiskValue float64 `json:"risk_value"`
	// RiskPenalty 实际扣除的分 η·R。
	RiskPenalty float64 `json:"risk_penalty"`
	// Contributions 四个维度各自的加权贡献，顺序为[语义,时序,图谱,结构化]。
	Contributions [4]float64 `json:"contributions"`
	// ClampedAtZero 综合分是否被下限截断（说明风险压过了全部证据）。
	ClampedAtZero bool `json:"clamped_at_zero"`
}

// Candidate 参与综合评分的候选条目。
type Candidate struct {
	// ID 唯一标识。
	ID string `json:"id"`
	// Scores 四维度得分。
	Scores DimensionScores `json:"scores"`
	// Risk 四类风险信号。
	Risk RiskSignals `json:"risk"`
}

// RankedCandidate 评分后的候选（含分解明细）。
type RankedCandidate struct {
	Candidate `json:"candidate"`
	Breakdown ScoreBreakdown `json:"breakdown"`
	Rank      int            `json:"rank"`
}

// Composite 计算单条候选的综合评分。
//
// 输入会被夹取到 [0,1]：来自不同检索引擎的原始分数量纲各异，
// 若不夹取，某个维度的异常大值会淹没其余三路与风险项。
func Composite(candidate Candidate, weights CompositeWeights, riskWeights RiskWeights) (ScoreBreakdown, error) {
	if err := weights.Validate(); err != nil {
		return ScoreBreakdown{}, err
	}
	if err := riskWeights.Validate(); err != nil {
		return ScoreBreakdown{}, err
	}

	semantic := clampUnit(candidate.Scores.Semantic)
	temporal := clampUnit(candidate.Scores.Temporal)
	graph := clampUnit(candidate.Scores.Graph)
	structured := clampUnit(candidate.Scores.Structured)

	contributions := [4]float64{
		weights.Alpha * semantic,
		weights.Beta * temporal,
		weights.Gamma * graph,
		weights.Delta * structured,
	}
	evidence := contributions[0] + contributions[1] + contributions[2] + contributions[3]

	riskInput := RiskSignals{
		Context:       clampUnit(candidate.Risk.Context),
		Sensitive:     clampUnit(candidate.Risk.Sensitive),
		PrivacyCost:   clampUnit(candidate.Risk.PrivacyCost),
		SourceUntrust: clampUnit(candidate.Risk.SourceUntrust),
	}
	riskTotal := riskWeights.Context + riskWeights.Sensitive +
		riskWeights.PrivacyCost + riskWeights.SourceUntrust
	if riskTotal <= 0 {
		return ScoreBreakdown{}, fmt.Errorf("risk_fusion: risk weights sum to zero")
	}
	// 风险权重归一，使 R ∈ [0,1]，从而 η 的语义稳定。
	riskValue := (riskWeights.Context*riskInput.Context +
		riskWeights.Sensitive*riskInput.Sensitive +
		riskWeights.PrivacyCost*riskInput.PrivacyCost +
		riskWeights.SourceUntrust*riskInput.SourceUntrust) / riskTotal

	penalty := weights.Eta * riskValue
	composite := evidence - penalty

	breakdown := ScoreBreakdown{
		EvidenceScore: evidence,
		RiskValue:     riskValue,
		RiskPenalty:   penalty,
		Contributions: contributions,
	}
	if composite < 0 {
		composite = 0
		breakdown.ClampedAtZero = true
	}
	breakdown.Composite = composite
	return breakdown, nil
}

// RankCandidates 对候选集做风险感知排序。
//
// 排序键：综合分降序；并列时按 ID 升序，保证结果**确定性**——
// 检索结果的顺序若随机波动，会带来无法复现的评测噪声。
//
// 单条评分失败的候选会被跳过并记录在 errors 中，不中断整体排序。
func RankCandidates(candidates []Candidate, weights CompositeWeights, riskWeights RiskWeights) ([]RankedCandidate, []error) {
	normalized := weights.Normalized()
	ranked := make([]RankedCandidate, 0, len(candidates))
	var errs []error

	for _, candidate := range candidates {
		breakdown, err := Composite(candidate, normalized, riskWeights)
		if err != nil {
			errs = append(errs, fmt.Errorf("candidate %q: %w", candidate.ID, err))
			continue
		}
		ranked = append(ranked, RankedCandidate{
			Candidate: candidate,
			Breakdown: breakdown,
		})
	}

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Breakdown.Composite != ranked[j].Breakdown.Composite {
			return ranked[i].Breakdown.Composite > ranked[j].Breakdown.Composite
		}
		return ranked[i].ID < ranked[j].ID
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
	}
	return ranked, errs
}

// ============================================================================
// 风险信号构造辅助
// ============================================================================

// SensitiveScorer 评估文本内容的敏感密度，返回 [0,1]。
//
// 之所以做成可注入的函数而不是在本包内直接调用检测器：
// 检测器位于 internal/security，直接引用会让检索引擎包与隐私包
// 形成耦合（且存在循环依赖风险）。注入式依赖让本包可独立测试，
// 也让"敏感密度的定义"可以由部署方替换。
type SensitiveScorer func(content string) float64

// RiskAssessor 把原始内容转换为风险信号的评估器。
type RiskAssessor struct {
	// Weights 综合评分权重。
	Weights CompositeWeights
	// RiskWeights 风险项内部权重。
	RiskWeights RiskWeights
	// SensitiveScorer 可选的敏感密度评估器。
	// 为 nil 时敏感密度恒为 0，并在 Assess 的结果中如实标注，
	// 避免"没配检测器"被误读为"内容不敏感"。
	SensitiveScorer SensitiveScorer
}

// NewRiskAssessor 创建风险评估器，权重取默认值。
func NewRiskAssessor(scorer SensitiveScorer) *RiskAssessor {
	return &RiskAssessor{
		Weights:         DefaultCompositeWeights(),
		RiskWeights:     DefaultRiskWeights(),
		SensitiveScorer: scorer,
	}
}

// Assess 对单条内容做风险感知评分。
//
// privacyCost 与 sourceUntrust 由调用方按账本/治理标签传入，
// 本函数不臆造它们。
func (a *RiskAssessor) Assess(id, content string, scores DimensionScores, contextRisk, privacyCost, sourceUntrust float64) (ScoreBreakdown, error) {
	sensitive := 0.0
	if a.SensitiveScorer != nil {
		sensitive = clampUnit(a.SensitiveScorer(content))
	}
	return Composite(Candidate{
		ID:     id,
		Scores: scores,
		Risk: RiskSignals{
			Context:       contextRisk,
			Sensitive:     sensitive,
			PrivacyCost:   privacyCost,
			SourceUntrust: sourceUntrust,
		},
	}, a.Weights, a.RiskWeights)
}

// SensitiveScorerConfigured 敏感密度评估器是否就位。
//
// 暴露这个标志是为了让上层在"未配置检测器"时给出明确提示，
// 而不是让风险惩罚在静默中失效。
func (a *RiskAssessor) SensitiveScorerConfigured() bool {
	return a.SensitiveScorer != nil
}

// clampUnit 把数值夹取到 [0,1]；NaN 归零。
//
// NaN 归零而非透传：NaN 参与比较会让排序结果不可预测，
// 且会像传染病一样污染后续所有聚合。
func clampUnit(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
