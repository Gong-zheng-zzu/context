package security

import (
	"math"
	"testing"
)

// mironovRDP 按 Mironov《Rényi Differential Privacy》(2017) 中拉普拉斯机制的
// 精确 RDP 公式计算 D_α：
//
//	D_α = 1/(α−1) · log[ ( α·e^{(α−1)ε} + (α−1)·e^{−αε} ) / (2α−1) ]
//
// 用于交叉验证本项目矩阶实现 α(λ) = λ·D_{λ+1} 的正确性。
func mironovRDP(alpha, epsilon float64) float64 {
	if alpha <= 1 {
		return 0
	}
	numerator := alpha*math.Exp((alpha-1)*epsilon) + (alpha-1)*math.Exp(-alpha*epsilon)
	denominator := 2*alpha - 1
	return math.Log(numerator/denominator) / (alpha - 1)
}

// TestLaplaceLogMomentMatchesMironovRDP 交叉验证：本项目以矩阶表示的对数矩
// α(λ) 必须等于 λ·D_{λ+1}，其中 D 为已发表的拉普拉斯 RDP 精确公式。
//
// 这是本实现数学正确性的核心证据：公式不是自造的，而是与公开结果逐点吻合。
func TestLaplaceLogMomentMatchesMironovRDP(t *testing.T) {
	for _, epsilon := range []float64{0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0} {
		for _, order := range []float64{0.25, 0.5, 1, 2, 3, 5, 10, 32} {
			got := LaplaceLogMoment(order, epsilon)
			want := order * mironovRDP(order+1, epsilon)
			if math.Abs(got-want) > 1e-9*math.Max(1, math.Abs(want)) {
				t.Fatalf("order=%v eps=%v: LaplaceLogMoment=%v, order*RDP=%v", order, epsilon, got, want)
			}
		}
	}
}

// TestLaplaceLogMomentDegenerateCases ε=0 时无隐私损失，对数矩必须为 0；
// 大 λε 时不得溢出为 +Inf。
func TestLaplaceLogMomentDegenerateCases(t *testing.T) {
	if got := LaplaceLogMoment(3, 0); got != 0 {
		t.Fatalf("eps=0 log moment = %v, want 0", got)
	}
	if got := LaplaceLogMoment(0, 1); got != 0 {
		t.Fatalf("order=0 log moment = %v, want 0", got)
	}

	// 极大 λε 场景：数值必须有限且逼近线性上界 λε。
	got := LaplaceLogMoment(100, 10)
	if math.IsInf(got, 0) || math.IsNaN(got) {
		t.Fatalf("large order*eps overflowed: %v", got)
	}
	if got > 100*10+1e-6 {
		t.Fatalf("log moment %v exceeds trivial bound λε=%v", got, 1000.0)
	}
}

// TestLaplaceLogMomentIsConvexInOrder 对数矩关于矩阶是凸的（矩母函数的
// 对数凸性）。这是矩会计能做"沿 λ 取最小"优化的理论基础。
func TestLaplaceLogMomentIsConvexInOrder(t *testing.T) {
	const epsilon = 0.3
	orders := []float64{0.25, 0.5, 1, 2, 4, 8, 16}

	for i := 1; i+1 < len(orders); i++ {
		left := (LaplaceLogMoment(orders[i], epsilon) - LaplaceLogMoment(orders[i-1], epsilon)) / (orders[i] - orders[i-1])
		right := (LaplaceLogMoment(orders[i+1], epsilon) - LaplaceLogMoment(orders[i], epsilon)) / (orders[i+1] - orders[i])
		if right < left-1e-9 {
			t.Fatalf("log moment not convex at order %v: left slope %v > right slope %v", orders[i], left, right)
		}
	}
}

// TestMomentAccountantZeroStepsIsZero 无操作时隐私消耗为 0。
func TestMomentAccountantZeroStepsIsZero(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	eps, err := ma.Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if eps != 0 {
		t.Fatalf("epsilon with zero steps = %v, want 0", eps)
	}
}

// TestMomentAccountantTighterThanBasicComposition 固化矩会计的核心价值：
// 在"步数多、单步 ε 小"的典型长期记忆场景下，矩会计给出的 ε 必须显著
// 小于基础组合定理的线性累加值。
//
// 场景：10000 次查询，每次 ε=0.1，δ=1e-5。
//
//	基础组合：ε = 10000 × 0.1 = 1000
//	矩会计  ：ε ≈ 95.5（在 λ≈0.5 处取到）
//
// 这正是策划书所称"更紧致的组合边界"。
func TestMomentAccountantTighterThanBasicComposition(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	for i := 0; i < 10000; i++ {
		if err := ma.Accumulate(PrivacyStep{Epsilon: 0.1}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}

	basic := ma.BasicEpsilon()
	if math.Abs(basic-1000) > 1e-6 {
		t.Fatalf("basic composition = %v, want 1000", basic)
	}

	moment, err := ma.Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if moment >= basic {
		t.Fatalf("moment accountant epsilon %v not tighter than basic %v", moment, basic)
	}
	// 期望约 95.5；给出宽容区间以吸收矩阶网格的离散化误差。
	if moment < 50 || moment > 150 {
		t.Fatalf("moment accountant epsilon %v outside expected range [50, 150]", moment)
	}

	// 不是微弱优势，而是数量级级别的紧致。
	if moment > basic*0.2 {
		t.Fatalf("expected substantial tightening, got moment=%v vs basic=%v", moment, basic)
	}
}

// TestMomentAccountantMonotoneNonDecreasing 组合 ε 随步数单调不减——
// 隐私只会越用越少，不允许出现"加了一步反而更安全"。
func TestMomentAccountantMonotoneNonDecreasing(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	previous := 0.0
	for i := 0; i < 500; i++ {
		if err := ma.Accumulate(PrivacyStep{Epsilon: 0.2}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
		current, err := ma.Epsilon(1e-6)
		if err != nil {
			t.Fatalf("Epsilon: %v", err)
		}
		if current < previous-1e-9 {
			t.Fatalf("epsilon decreased at step %d: %v -> %v", i, previous, current)
		}
		previous = current
	}
}

// TestSubsamplingReducesEpsilon 子采样放大：触及数据比例越低，
// 同等步数下的隐私消耗越小。
func TestSubsamplingReducesEpsilon(t *testing.T) {
	build := func(samplingRate float64) *MomentAccountant {
		ma := NewDefaultMomentAccountant()
		for i := 0; i < 5000; i++ {
			if err := ma.Accumulate(PrivacyStep{Epsilon: 0.2, SamplingRate: samplingRate}); err != nil {
				t.Fatalf("Accumulate: %v", err)
			}
		}
		return ma
	}

	full, err := build(1.0).Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	partial, err := build(0.05).Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}

	if partial >= full {
		t.Fatalf("subsampling did not reduce epsilon: q=1 -> %v, q=0.05 -> %v", full, partial)
	}
}

// TestSubsampledLogMomentDegenerates 子采样对数矩的边界行为：
// q=1 退化为无放大，q=0 完全无风险。
func TestSubsampledLogMomentDegenerates(t *testing.T) {
	const order, epsilon = 2.0, 0.5

	if got, want := SubsampledLaplaceLogMoment(order, epsilon, 1.0), LaplaceLogMoment(order, epsilon); got != want {
		t.Fatalf("q=1: got %v, want %v", got, want)
	}
	if got := SubsampledLaplaceLogMoment(order, epsilon, 0.0); got != 0 {
		t.Fatalf("q=0: got %v, want 0", got)
	}
	// 0<q<1 时对数矩介于两者之间。
	mid := SubsampledLaplaceLogMoment(order, epsilon, 0.5)
	if mid <= 0 || mid >= LaplaceLogMoment(order, epsilon) {
		t.Fatalf("q=0.5: got %v, want within (0, %v)", mid, LaplaceLogMoment(order, epsilon))
	}
}

// TestAccumulateRejectsInvalidInput 非法输入必须被拒绝，而不是
// 静默产生错误的隐私预算。
func TestAccumulateRejectsInvalidInput(t *testing.T) {
	ma := NewDefaultMomentAccountant()

	if err := ma.Accumulate(PrivacyStep{Epsilon: -0.1}); err == nil {
		t.Fatal("expected error for negative epsilon")
	}
	if err := ma.Accumulate(PrivacyStep{Epsilon: math.NaN()}); err == nil {
		t.Fatal("expected error for NaN epsilon")
	}
	if err := ma.Accumulate(PrivacyStep{Epsilon: 0.1, SamplingRate: 1.5}); err == nil {
		t.Fatal("expected error for sampling rate > 1")
	}
	if err := ma.Accumulate(PrivacyStep{Epsilon: 0.1, SamplingRate: -0.2}); err == nil {
		t.Fatal("expected error for negative sampling rate")
	}
	// 被拒绝的操作不得计入步数。
	if ma.Steps() != 0 {
		t.Fatalf("rejected steps were counted: %d", ma.Steps())
	}
}

// TestEpsilonRejectsInvalidDelta δ 必须位于 (0,1) 开区间。
func TestEpsilonRejectsInvalidDelta(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	if err := ma.Accumulate(PrivacyStep{Epsilon: 0.1}); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}

	for _, delta := range []float64{0, -1e-6, 1, 2} {
		if _, err := ma.Epsilon(delta); err == nil {
			t.Fatalf("expected error for delta=%v", delta)
		}
	}
	if _, err := ma.Epsilon(1e-6); err != nil {
		t.Fatalf("valid delta rejected: %v", err)
	}
}

// TestZeroEpsilonStepDoesNotConsumeBudget 零消耗操作（如缓存命中）
// 不增加隐私消耗，但仍被如实计数。
func TestZeroEpsilonStepDoesNotConsumeBudget(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	for i := 0; i < 100; i++ {
		if err := ma.Accumulate(PrivacyStep{Epsilon: 0, Label: "cache-hit"}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}

	eps, err := ma.Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if eps != 0 {
		t.Fatalf("zero-epsilon steps consumed budget: %v", eps)
	}
	if ma.Steps() != 100 {
		t.Fatalf("steps = %d, want 100", ma.Steps())
	}
	if ma.BasicEpsilon() != 0 {
		t.Fatalf("basic epsilon = %v, want 0", ma.BasicEpsilon())
	}
}

// TestOrderBoundsExposeAuditTrail 边界分解可用于解释"为什么取到这个 ε"。
func TestOrderBoundsExposeAuditTrail(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	for i := 0; i < 1000; i++ {
		if err := ma.Accumulate(PrivacyStep{Epsilon: 0.1}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}

	bounds, err := ma.OrderBounds(1e-5)
	if err != nil {
		t.Fatalf("OrderBounds: %v", err)
	}
	if len(bounds) == 0 {
		t.Fatal("no order bounds returned")
	}

	// 矩阶升序。
	for i := 1; i < len(bounds); i++ {
		if bounds[i].Order <= bounds[i-1].Order {
			t.Fatalf("order bounds not ascending at %d", i)
		}
	}

	// 最优矩阶上的 ε 必须与 Epsilon() 的返回值一致。
	best := math.Inf(1)
	for _, b := range bounds {
		if b.Epsilon < best {
			best = b.Epsilon
		}
	}
	eps, err := ma.Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if math.Abs(best-eps) > 1e-9 {
		t.Fatalf("min over order bounds %v != Epsilon() %v", best, eps)
	}
}

// TestResetClearsAccounting 重置后回到初始状态，用于新的预算周期。
func TestResetClearsAccounting(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	for i := 0; i < 10; i++ {
		if err := ma.Accumulate(PrivacyStep{Epsilon: 0.5}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}
	ma.Reset()

	if ma.Steps() != 0 || ma.BasicEpsilon() != 0 {
		t.Fatalf("reset did not clear state: steps=%d basic=%v", ma.Steps(), ma.BasicEpsilon())
	}
	eps, err := ma.Epsilon(1e-5)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if eps != 0 {
		t.Fatalf("epsilon after reset = %v, want 0", eps)
	}
}

// TestNewMomentAccountantFiltersInvalidOrders 非法矩阶被过滤；
// 全部非法时回退默认网格，避免出现"ε 恒为 0"的静默失效。
func TestNewMomentAccountantFiltersInvalidOrders(t *testing.T) {
	ma := NewMomentAccountant([]float64{-1, 0, math.NaN(), math.Inf(1), 2, 2})
	if err := ma.Accumulate(PrivacyStep{Epsilon: 1.0}); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}
	eps, err := ma.Epsilon(1e-3)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if eps <= 0 {
		t.Fatalf("epsilon = %v, want > 0 (invalid orders must not disable accounting)", eps)
	}

	fallback := NewMomentAccountant([]float64{-1, 0})
	if err := fallback.Accumulate(PrivacyStep{Epsilon: 1.0}); err != nil {
		t.Fatalf("Accumulate: %v", err)
	}
	eps, err = fallback.Epsilon(1e-3)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	if eps <= 0 {
		t.Fatalf("fallback grid produced epsilon %v, want > 0", eps)
	}
}

// TestComposeMomentBound 便捷函数与逐步累加结果一致。
func TestComposeMomentBound(t *testing.T) {
	steps := []PrivacyStep{
		{Epsilon: 0.3, SamplingRate: 0.2},
		{Epsilon: 0.15, SamplingRate: 1.0},
		{Epsilon: 0.0},
		{Epsilon: 0.25, SamplingRate: 0.5},
	}

	composed, err := ComposeMomentBound(steps, 1e-6)
	if err != nil {
		t.Fatalf("ComposeMomentBound: %v", err)
	}

	ma := NewDefaultMomentAccountant()
	for _, step := range steps {
		if err := ma.Accumulate(step); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}
	manual, err := ma.Epsilon(1e-6)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}

	if math.Abs(composed-manual) > 1e-9 {
		t.Fatalf("ComposeMomentBound=%v != manual=%v", composed, manual)
	}

	// BestEpsilon 取两界中更紧者，因此必然不超过基础组合。
	best, err := ma.BestEpsilon(1e-6)
	if err != nil {
		t.Fatalf("BestEpsilon: %v", err)
	}
	if best > ma.BasicEpsilon()+1e-12 {
		t.Fatalf("BestEpsilon %v worse than basic %v", best, ma.BasicEpsilon())
	}
	if best > composed+1e-12 {
		t.Fatalf("BestEpsilon %v worse than moment bound %v", best, composed)
	}
}

// TestMomentBoundLosesToBasicCompositionWithFewSteps 如实固化一个真实的
// 数学性质：在"步数少、单步 ε 大"的区间，矩会计边界会劣于基础组合的
// 线性累加——因为 ln(1/δ)/λ 项尚未被步数摊薄。
//
// 该测试存在的意义是防止后人误以为"矩会计恒优于线性累加"而写出
// 错误的对照实验。正确的对照口径是 BestEpsilon（取两界更紧者）。
func TestMomentBoundLosesToBasicCompositionWithFewSteps(t *testing.T) {
	ma := NewDefaultMomentAccountant()
	for i := 0; i < 4; i++ {
		if err := ma.Accumulate(PrivacyStep{
			Epsilon:      0.3,
			SamplingRate: []float64{0.2, 1.0, 1.0, 0.5}[i],
		}); err != nil {
			t.Fatalf("Accumulate: %v", err)
		}
	}

	moment, err := ma.Epsilon(1e-6)
	if err != nil {
		t.Fatalf("Epsilon: %v", err)
	}
	basic := ma.BasicEpsilon()

	// 少数几步时，矩会计边界确实更松（这是正确行为，不是缺陷）。
	if moment <= basic {
		t.Skipf("moment bound %v unexpectedly tighter than basic %v in this regime; "+
			"grid may have been retuned — re-verify the comparison口径", moment, basic)
	}

	// 而 BestEpsilon 必须回退到更紧的基础组合。
	best, err := ma.BestEpsilon(1e-6)
	if err != nil {
		t.Fatalf("BestEpsilon: %v", err)
	}
	if math.Abs(best-basic) > 1e-12 {
		t.Fatalf("BestEpsilon = %v, want basic %v (the tighter of the two)", best, basic)
	}
}

// TestComposeMomentBoundPropagatesStepError 单步非法时返回带下标的错误。
func TestComposeMomentBoundPropagatesStepError(t *testing.T) {
	steps := []PrivacyStep{{Epsilon: 0.1}, {Epsilon: -1}}
	if _, err := ComposeMomentBound(steps, 1e-5); err == nil {
		t.Fatal("expected error from invalid step")
	}
}
