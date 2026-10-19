package multi_dimensional_retrieval

import (
	"math"
	"testing"
)

// uniform 构造四维同分的候选。
func uniform(id string, score, risk float64) Candidate {
	return Candidate{
		ID: id,
		Scores: DimensionScores{
			Semantic: score, Temporal: score, Graph: score, Structured: score,
		},
		Risk: RiskSignals{
			Context: risk, Sensitive: risk, PrivacyCost: risk, SourceUntrust: risk,
		},
	}
}

// ============================================================================
// 评分公式
// ============================================================================

// TestCompositeMatchesHandCalculation 逐项核对公式：
// Score = α·语义 + β·时序 + γ·图谱 + δ·结构化 − η·R
func TestCompositeMatchesHandCalculation(t *testing.T) {
	weights := DefaultCompositeWeights()
	riskWeights := DefaultRiskWeights()

	breakdown, err := Composite(uniform("full", 1.0, 0.0), weights, riskWeights)
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}
	// 零风险满分条目综合分应为 1.0（四路权重已归一）。
	if math.Abs(breakdown.EvidenceScore-1.0) > 1e-9 {
		t.Fatalf("EvidenceScore = %v, want 1.0", breakdown.EvidenceScore)
	}
	if math.Abs(breakdown.Composite-1.0) > 1e-9 {
		t.Fatalf("Composite = %v, want 1.0", breakdown.Composite)
	}
	if breakdown.RiskValue != 0 || breakdown.RiskPenalty != 0 {
		t.Fatalf("zero risk should yield zero penalty, got R=%v penalty=%v", breakdown.RiskValue, breakdown.RiskPenalty)
	}

	// 满分证据 + 最大风险：扣 η = 0.30。
	breakdown, err = Composite(uniform("risky", 1.0, 1.0), weights, riskWeights)
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}
	if math.Abs(breakdown.RiskValue-1.0) > 1e-9 {
		t.Fatalf("RiskValue = %v, want 1.0", breakdown.RiskValue)
	}
	if math.Abs(breakdown.RiskPenalty-0.30) > 1e-9 {
		t.Fatalf("RiskPenalty = %v, want 0.30 (η)", breakdown.RiskPenalty)
	}
	if math.Abs(breakdown.Composite-0.70) > 1e-9 {
		t.Fatalf("Composite = %v, want 0.70", breakdown.Composite)
	}
}

// TestCompositeDimensionContributions 各维度贡献必须与权重一一对应，
// 避免"权重配了但不生效"这类静默错误。
func TestCompositeDimensionContributions(t *testing.T) {
	weights := CompositeWeights{Alpha: 0.4, Beta: 0.3, Gamma: 0.2, Delta: 0.1, Eta: 0}

	breakdown, err := Composite(Candidate{
		ID: "x",
		Scores: DimensionScores{
			Semantic: 0.5, Temporal: 0.4, Graph: 0.25, Structured: 1.0,
		},
	}, weights, DefaultRiskWeights())
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}

	want := [4]float64{0.4 * 0.5, 0.3 * 0.4, 0.2 * 0.25, 0.1 * 1.0}
	for i := range want {
		if math.Abs(breakdown.Contributions[i]-want[i]) > 1e-9 {
			t.Errorf("contribution[%d] = %v, want %v", i, breakdown.Contributions[i], want[i])
		}
	}
	expectTotal := 0.2 + 0.12 + 0.05 + 0.1
	if math.Abs(breakdown.EvidenceScore-expectTotal) > 1e-9 {
		t.Fatalf("EvidenceScore = %v, want %v", breakdown.EvidenceScore, expectTotal)
	}
}

// TestCompositeClampsOutOfRangeInputs 越界输入必须被夹取到 [0,1]。
// 不同检索引擎的量纲各异，不夹取会让单一维度淹没其余三路。
func TestCompositeClampsOutOfRangeInputs(t *testing.T) {
	weights := DefaultCompositeWeights()
	riskWeights := DefaultRiskWeights()

	breakdown, err := Composite(Candidate{
		ID: "over",
		Scores: DimensionScores{
			Semantic: 99, Temporal: -5, Graph: 0.5, Structured: 0.5,
		},
		Risk: RiskSignals{Context: 50, Sensitive: -1, PrivacyCost: 0, SourceUntrust: 0},
	}, weights, riskWeights)
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}

	if breakdown.Contributions[0] > weights.Alpha+1e-9 {
		t.Fatalf("semantic contribution not clamped: %v", breakdown.Contributions[0])
	}
	if breakdown.Contributions[1] < 0 {
		t.Fatalf("temporal contribution not clamped: %v", breakdown.Contributions[1])
	}
	if breakdown.RiskValue < 0 || breakdown.RiskValue > 1 {
		t.Fatalf("RiskValue out of [0,1]: %v", breakdown.RiskValue)
	}
}

// TestCompositeTreatsNaNAsZero NaN 归零而非透传——
// NaN 参与比较会让排序结果不可预测。
func TestCompositeTreatsNaNAsZero(t *testing.T) {
	breakdown, err := Composite(Candidate{
		ID: "nan",
		Scores: DimensionScores{
			Semantic: math.NaN(), Temporal: 0.5, Graph: 0.5, Structured: 0.5,
		},
		Risk: RiskSignals{Context: math.NaN()},
	}, DefaultCompositeWeights(), DefaultRiskWeights())
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}
	if math.IsNaN(breakdown.Composite) {
		t.Fatal("Composite is NaN")
	}
	if breakdown.Contributions[0] != 0 {
		t.Fatalf("NaN semantic should contribute 0, got %v", breakdown.Contributions[0])
	}
}

// TestCompositeFloorsAtZero 风险压过证据时综合分截为 0，
// 并如实标注 ClampedAtZero（避免"负分"被误当作排序依据）。
func TestCompositeFloorsAtZero(t *testing.T) {
	breakdown, err := Composite(uniform("tiny", 0.1, 1.0), DefaultCompositeWeights(), DefaultRiskWeights())
	if err != nil {
		t.Fatalf("Composite: %v", err)
	}
	if breakdown.Composite != 0 {
		t.Fatalf("Composite = %v, want 0", breakdown.Composite)
	}
	if !breakdown.ClampedAtZero {
		t.Fatal("ClampedAtZero must be true when the score is floored")
	}
}

// ============================================================================
// 权重校验与归一化
// ============================================================================

// TestWeightsValidationRejectsBadConfig 非法权重必须在入口被拒绝。
func TestWeightsValidationRejectsBadConfig(t *testing.T) {
	bad := []CompositeWeights{
		{Alpha: -0.1, Beta: 0.5, Gamma: 0.3, Delta: 0.2},
		{Alpha: math.NaN(), Beta: 0.5, Gamma: 0.3, Delta: 0.2},
		{Alpha: math.Inf(1), Beta: 0.5, Gamma: 0.3, Delta: 0.2},
		{Alpha: 0, Beta: 0, Gamma: 0, Delta: 0},
	}
	for _, weights := range bad {
		if err := weights.Validate(); err == nil {
			t.Fatalf("expected error for weights %+v", weights)
		}
	}

	badRisk := []RiskWeights{
		{Context: -1, Sensitive: 0, PrivacyCost: 0, SourceUntrust: 0},
		{Context: math.NaN()},
		{Context: 0, Sensitive: 0, PrivacyCost: 0, SourceUntrust: 0},
	}
	for _, rw := range badRisk {
		if err := rw.Validate(); err == nil {
			t.Fatalf("expected error for risk weights %+v", rw)
		}
	}

	if err := DefaultCompositeWeights().Validate(); err != nil {
		t.Fatalf("default weights rejected: %v", err)
	}
	if err := DefaultRiskWeights().Validate(); err != nil {
		t.Fatalf("default risk weights rejected: %v", err)
	}
}

// TestNormalizedMakesEvidenceSumToOne 归一化后四路权重和为 1，
// 使"零风险满分条目 = 1.0"这一语义与权重缩放解耦。
func TestNormalizedMakesEvidenceSumToOne(t *testing.T) {
	weights := CompositeWeights{Alpha: 2, Beta: 2, Gamma: 1, Delta: 1, Eta: 0.3}
	normalized := weights.Normalized()

	sum := normalized.Alpha + normalized.Beta + normalized.Gamma + normalized.Delta
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("normalized weights sum = %v, want 1.0", sum)
	}
	// 相对比例保持不变。
	if math.Abs(normalized.Alpha-0.3333333) > 1e-6 {
		t.Fatalf("normalized alpha = %v, want ~1/3", normalized.Alpha)
	}
	// η 不受归一化影响。
	if normalized.Eta != 0.3 {
		t.Fatalf("eta changed by normalization: %v", normalized.Eta)
	}

	// 全零权重时归一化不得除零 panic。
	zero := CompositeWeights{}.Normalized()
	if zero.Alpha != 0 {
		t.Fatal("normalizing zero weights should be a no-op")
	}
}

// ============================================================================
// 排序
// ============================================================================

// TestRiskPenaltyChangesRanking 风险惩罚必须能改变排序：
// 证据分更高但高风险的条目，应排在证据分略低但无风险的条目之后。
//
// 这是风险惩罚项存在的意义——RRF 只看名次，做不到这一点。
func TestRiskPenaltyChangesRanking(t *testing.T) {
	candidates := []Candidate{
		uniform("high-evidence-high-risk", 1.0, 1.0), // 综合 1.0 - 0.30 = 0.70
		uniform("lower-evidence-no-risk", 0.9, 0.0),  // 综合 0.90
	}

	ranked, errs := RankCandidates(candidates, DefaultCompositeWeights(), DefaultRiskWeights())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked candidates, got %d", len(ranked))
	}

	if ranked[0].ID != "lower-evidence-no-risk" {
		t.Fatalf("risk penalty failed to reorder: winner is %s (composite %v)",
			ranked[0].ID, ranked[0].Breakdown.Composite)
	}
	if ranked[0].Rank != 1 || ranked[1].Rank != 2 {
		t.Fatalf("ranks not assigned: %d, %d", ranked[0].Rank, ranked[1].Rank)
	}
}

// TestRankingIsDeterministicOnTies 并列时按 ID 升序，
// 保证结果可复现（顺序随机波动会带来无法复现的评测噪声）。
func TestRankingIsDeterministicOnTies(t *testing.T) {
	candidates := []Candidate{
		uniform("zulu", 0.5, 0.0),
		uniform("alpha", 0.5, 0.0),
		uniform("mike", 0.5, 0.0),
	}

	for i := 0; i < 3; i++ {
		ranked, _ := RankCandidates(candidates, DefaultCompositeWeights(), DefaultRiskWeights())
		if ranked[0].ID != "alpha" || ranked[1].ID != "mike" || ranked[2].ID != "zulu" {
			t.Fatalf("non-deterministic ordering: %s, %s, %s",
				ranked[0].ID, ranked[1].ID, ranked[2].ID)
		}
	}
}

// TestRankingSkipsInvalidWithoutAborting 单条评分失败不应中断整体排序。
func TestRankingSkipsInvalidWithoutAborting(t *testing.T) {
	candidates := []Candidate{
		uniform("good", 0.8, 0.0),
		uniform("also-good", 0.6, 0.0),
	}

	ranked, errs := RankCandidates(candidates, CompositeWeights{Alpha: -1}, DefaultRiskWeights())
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid weights")
	}
	if len(ranked) != 0 {
		t.Fatalf("invalid weights should yield no ranking, got %d", len(ranked))
	}
}

// TestRankingHandlesEmptyInput 空输入不得 panic。
func TestRankingHandlesEmptyInput(t *testing.T) {
	ranked, errs := RankCandidates(nil, DefaultCompositeWeights(), DefaultRiskWeights())
	if len(ranked) != 0 || len(errs) != 0 {
		t.Fatalf("expected empty result, got %d ranked / %d errors", len(ranked), len(errs))
	}
}

// ============================================================================
// 风险评估器
// ============================================================================

// TestRiskAssessorUsesInjectedScorer 注入的敏感密度评估器必须生效。
func TestRiskAssessorUsesInjectedScorer(t *testing.T) {
	assessor := NewRiskAssessor(func(content string) float64 { return 1.0 })
	if !assessor.SensitiveScorerConfigured() {
		t.Fatal("scorer should be reported as configured")
	}

	breakdown, err := assessor.Assess("id", "含身份证的文本", uniform("x", 1, 0).Scores, 0, 0, 0)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if breakdown.RiskValue <= 0 {
		t.Fatalf("injected sensitive scorer had no effect: R = %v", breakdown.RiskValue)
	}
}

// TestRiskAssessorWithoutScorerIsHonest 未配置检测器时敏感密度为 0，
// 但必须能通过 SensitiveScorerConfigured 被上层察觉——
// 不能让"没配检测器"被误读为"内容不敏感"。
func TestRiskAssessorWithoutScorerIsHonest(t *testing.T) {
	assessor := NewRiskAssessor(nil)
	if assessor.SensitiveScorerConfigured() {
		t.Fatal("scorer should be reported as unconfigured")
	}

	breakdown, err := assessor.Assess("id", "含身份证的文本", uniform("x", 1, 0).Scores, 0, 0, 0)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if breakdown.RiskValue != 0 {
		t.Fatalf("without scorer, risk should be 0, got %v", breakdown.RiskValue)
	}
	if breakdown.Composite != 1.0 {
		t.Fatalf("Composite = %v, want 1.0", breakdown.Composite)
	}
}

// TestRiskAssessorSeparatesRiskSources 四类风险信号来源不同、权重不同，
// 因此"哪一项风险在起作用"必须能从分解中看出来，而不是混成一个黑箱数。
func TestRiskAssessorSeparatesRiskSources(t *testing.T) {
	assessor := NewRiskAssessor(nil)
	scores := uniform("x", 1, 0).Scores

	// 仅上下文风险拉满。
	contextOnly, _ := assessor.Assess("a", "", scores, 1.0, 0, 0)
	// 仅隐私成本拉满。
	privacyOnly, _ := assessor.Assess("b", "", scores, 0, 1.0, 0)

	// 上下文权重 0.35 > 隐私成本权重 0.20，故前者惩罚更重。
	if contextOnly.RiskPenalty <= privacyOnly.RiskPenalty {
		t.Fatalf("context risk (weight 0.35) should penalize more than privacy cost (0.20): %v vs %v",
			contextOnly.RiskPenalty, privacyOnly.RiskPenalty)
	}
	contextWant := 0.30 * 0.35
	if math.Abs(contextOnly.RiskPenalty-contextWant) > 1e-9 {
		t.Fatalf("context penalty = %v, want %v", contextOnly.RiskPenalty, contextWant)
	}
}

// TestDefaultWeightsAreSane 默认权重满足"四路都有发言权、η 不过激"。
func TestDefaultWeightsAreSane(t *testing.T) {
	w := DefaultCompositeWeights()
	if w.Alpha <= 0 || w.Beta <= 0 || w.Gamma <= 0 || w.Delta <= 0 {
		t.Fatalf("every evidence dimension must have non-zero weight: %+v", w)
	}
	if math.Abs(w.Alpha+w.Beta+w.Gamma+w.Delta-1.0) > 1e-9 {
		t.Fatalf("default evidence weights should sum to 1, got %v", w.Alpha+w.Beta+w.Gamma+w.Delta)
	}
	// η 必须小于 1：否则最坏情况下会把任何条目压成 0，
	// 高风险内容被完全隐藏，反而不利于审计。
	if w.Eta <= 0 || w.Eta >= 1 {
		t.Fatalf("eta should be in (0,1), got %v", w.Eta)
	}

	rw := DefaultRiskWeights()
	if math.Abs(rw.Context+rw.Sensitive+rw.PrivacyCost+rw.SourceUntrust-1.0) > 1e-9 {
		t.Fatal("default risk weights should sum to 1 so that R ∈ [0,1]")
	}
}

// TestClampUnit 夹取函数的边界行为。
func TestClampUnit(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0.5, 0.5}, {-1, 0}, {2, 1}, {math.NaN(), 0},
		{math.Inf(1), 1}, {math.Inf(-1), 0},
	}
	for _, c := range cases {
		if got := clampUnit(c.in); got != c.want {
			t.Errorf("clampUnit(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
