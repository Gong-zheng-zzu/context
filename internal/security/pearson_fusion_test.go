package security

import (
	"math"
	"testing"
)

// correlatedSamples 构造一批样本：层 1 与层 2 完全相关，层 4 与之中等相关。
//
//	layer1 = (i%5)/4          → 0, 0.25, 0.5, 0.75, 1.0 循环
//	layer2 = layer1           → ρ(1,2) = 1（完全冗余）
//	layer4 = ((i*3)%5)/4      → ρ(1,4) = ρ(2,4) = 0.5
func correlatedSamples() []map[int]float64 {
	samples := make([]map[int]float64, 0, 20)
	for i := 0; i < 20; i++ {
		layer1 := float64(i%5) / 4.0
		layer4 := float64((i*3)%5) / 4.0
		samples = append(samples, map[int]float64{
			1: layer1,
			2: layer1, // 与层 1 完全冗余
			4: layer4,
		})
	}
	return samples
}

// ============================ 相关系数 ============================

// TestPearsonCorrelationPerfectlyCorrelated y = 2x + 3 时 ρ 必须为 1。
func TestPearsonCorrelationPerfectlyCorrelated(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5, 6}
	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = 2*v + 3
	}

	rho, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatalf("PearsonCorrelation: %v", err)
	}
	if math.Abs(rho-1) > 1e-12 {
		t.Fatalf("rho = %v, want 1", rho)
	}
}

// TestPearsonCorrelationAntiCorrelated y = -x 时 ρ 必须为 -1。
func TestPearsonCorrelationAntiCorrelated(t *testing.T) {
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{-1, -2, -3, -4, -5}

	rho, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatalf("PearsonCorrelation: %v", err)
	}
	if math.Abs(rho+1) > 1e-12 {
		t.Fatalf("rho = %v, want -1", rho)
	}
}

// TestPearsonCorrelationKnownValue 固定一组已知数据，锁定具体数值。
func TestPearsonCorrelationKnownValue(t *testing.T) {
	x := []float64{0, 1, 2, 3, 4}
	y := []float64{0, 3, 1, 4, 2}

	rho, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatalf("PearsonCorrelation: %v", err)
	}
	if math.Abs(rho-0.5) > 1e-12 {
		t.Fatalf("rho = %v, want 0.5", rho)
	}
}

// TestPearsonCorrelationRejectsDegenerateInput 退化输入必须显式报错，
// 而不是静默返回 0 让调用方误读为"无相关"。
func TestPearsonCorrelationRejectsDegenerateInput(t *testing.T) {
	cases := []struct {
		name string
		x, y []float64
	}{
		{"长度不一致", []float64{1, 2, 3}, []float64{1, 2}},
		{"空向量", []float64{}, []float64{}},
		{"x 方差为 0", []float64{5, 5, 5, 5}, []float64{1, 2, 3, 4}},
		{"y 方差为 0", []float64{1, 2, 3, 4}, []float64{7, 7, 7, 7}},
	}
	for _, tc := range cases {
		if _, err := PearsonCorrelation(tc.x, tc.y); err == nil {
			t.Fatalf("%s: expected error, got nil", tc.name)
		}
	}
}

// TestPearsonCorrelationIsSymmetric 对称性 ρ(x,y) == ρ(y,x)。
func TestPearsonCorrelationIsSymmetric(t *testing.T) {
	x := []float64{0.1, 0.9, 0.4, 0.7, 0.2, 0.55}
	y := []float64{0.9, 0.2, 0.6, 0.15, 0.8, 0.33}

	forward, err := PearsonCorrelation(x, y)
	if err != nil {
		t.Fatalf("PearsonCorrelation: %v", err)
	}
	backward, err := PearsonCorrelation(y, x)
	if err != nil {
		t.Fatalf("PearsonCorrelation: %v", err)
	}
	if math.Abs(forward-backward) > 1e-15 {
		t.Fatalf("asymmetric: %v vs %v", forward, backward)
	}
}

// ============================ 相关性矩阵 ============================

// TestBuildCorrelationMatrixIsSymmetricWithUnitDiagonal 矩阵必须
// 对称、对角线为 1，且层 ID 升序。
func TestBuildCorrelationMatrixIsSymmetricWithUnitDiagonal(t *testing.T) {
	matrix, err := BuildCorrelationMatrix(correlatedSamples())
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}

	wantLayers := []int{1, 2, 4}
	if len(matrix.LayerIDs) != len(wantLayers) {
		t.Fatalf("layer IDs = %v, want %v", matrix.LayerIDs, wantLayers)
	}
	for i, id := range wantLayers {
		if matrix.LayerIDs[i] != id {
			t.Fatalf("layer IDs not ascending: %v", matrix.LayerIDs)
		}
	}

	size := len(matrix.LayerIDs)
	for i := 0; i < size; i++ {
		if matrix.Matrix[i][i] != 1 {
			t.Fatalf("diagonal[%d] = %v, want 1", i, matrix.Matrix[i][i])
		}
		for j := 0; j < size; j++ {
			if matrix.Matrix[i][j] != matrix.Matrix[j][i] {
				t.Fatalf("matrix not symmetric at (%d,%d)", i, j)
			}
		}
	}

	// 层 1 与层 2 完全冗余。
	if rho, ok := matrix.Correlation(1, 2); !ok || math.Abs(rho-1) > 1e-12 {
		t.Fatalf("corr(1,2) = %v (ok=%v), want 1", rho, ok)
	}
	// 层 1 与层 4 中等相关。
	if rho, ok := matrix.Correlation(1, 4); !ok || math.Abs(rho-0.5) > 1e-12 {
		t.Fatalf("corr(1,4) = %v (ok=%v), want 0.5", rho, ok)
	}
	if _, ok := matrix.Correlation(1, 99); ok {
		t.Fatal("Correlation should report unknown layer")
	}
}

// TestBuildCorrelationMatrixValidation 输入校验。
func TestBuildCorrelationMatrixValidation(t *testing.T) {
	if _, err := BuildCorrelationMatrix(nil); err == nil {
		t.Fatal("expected error for empty samples")
	}
	// 只有一层时无法构成相关性矩阵。
	one := []map[int]float64{{1: 0.1}, {1: 0.2}, {1: 0.3}}
	if _, err := BuildCorrelationMatrix(one); err == nil {
		t.Fatal("expected error for single layer")
	}
}

// TestBuildCorrelationMatrixDegradedWhenFewSamples 样本量不足时标记退化。
func TestBuildCorrelationMatrixDegradedWhenFewSamples(t *testing.T) {
	few := []map[int]float64{
		{1: 0.1, 2: 0.1},
		{1: 0.9, 2: 0.8},
		{1: 0.5, 2: 0.4},
	}
	matrix, err := BuildCorrelationMatrix(few)
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}
	if !matrix.Degraded {
		t.Fatalf("expected Degraded=true for %d samples", len(few))
	}
	// 退化时不做任何惩罚。
	penalties := matrix.RedundancyPenalties(map[int]float64{1: 0.7, 2: 0.3}, 0.5)
	for id, penalty := range penalties {
		if penalty != 1 {
			t.Fatalf("degraded matrix applied penalty to layer %d: %v", id, penalty)
		}
	}
}

// TestConstantLayerIsReportedAsUndefined 恒定输出的层，其相关性未定义，
// 必须被如实记录且不受惩罚（宁可少惩罚，也不误伤）。
func TestConstantLayerIsReportedAsUndefined(t *testing.T) {
	samples := make([]map[int]float64, 0, 10)
	for i := 0; i < 10; i++ {
		samples = append(samples, map[int]float64{
			1: float64(i) / 9.0,
			2: 0.5, // 恒定输出
		})
	}

	matrix, err := BuildCorrelationMatrix(samples)
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}
	if len(matrix.Undefined) == 0 {
		t.Fatal("constant layer should be reported in Undefined")
	}
	if rho, ok := matrix.Correlation(1, 2); !ok || rho != 0 {
		t.Fatalf("undefined pair should read 0, got %v (ok=%v)", rho, ok)
	}
}

// ============================ 冗余权重调整 ============================

// TestRedundancyAdjustedWeightsDowngradesCorrelatedLayer 核心行为：
// 与高权重层强相关的层，其相对权重必须被下调。
func TestRedundancyAdjustedWeightsDowngradesCorrelatedLayer(t *testing.T) {
	matrix, err := BuildCorrelationMatrix(correlatedSamples())
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}

	base := map[int]float64{1: 0.70, 2: 0.10, 4: 0.15, 5: 0.05}
	adjusted := matrix.RedundancyAdjustedWeights(base, 0.5)

	total := 0.0
	for _, weight := range adjusted {
		total += weight
	}
	if math.Abs(total-1) > 1e-9 {
		t.Fatalf("adjusted weights do not sum to 1: %v", total)
	}

	// 层 5 不在相关性矩阵中（本轮样本未覆盖），应保留权重且不被惩罚。
	if _, ok := adjusted[5]; !ok {
		t.Fatal("layer absent from correlation matrix was dropped from fusion")
	}

	// 层 2 与层 1 完全冗余，相对权重必须下降。
	baseRatio := base[2] / base[1]
	adjustedRatio := adjusted[2] / adjusted[1]
	if adjustedRatio >= baseRatio {
		t.Fatalf("redundant layer not downgraded: base ratio %v -> adjusted %v", baseRatio, adjustedRatio)
	}

	// 层 4 只中等相关，降幅应小于层 2。
	baseRatio4 := base[4] / base[1]
	adjustedRatio4 := adjusted[4] / adjusted[1]
	if adjustedRatio4 >= baseRatio4 {
		t.Fatalf("layer 4 should also be downgraded somewhat: %v -> %v", baseRatio4, adjustedRatio4)
	}
	if adjusted[2]/base[2] >= adjusted[4]/base[4] {
		t.Fatalf("strongly correlated layer 2 should be penalized more than layer 4")
	}
}

// TestRedundancyPenaltiesAreAtLeastOne 惩罚因子恒 ≥ 1，绝不放大权重。
func TestRedundancyPenaltiesAreAtLeastOne(t *testing.T) {
	matrix, err := BuildCorrelationMatrix(correlatedSamples())
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}

	penalties := matrix.RedundancyPenalties(map[int]float64{1: 0.7, 2: 0.1, 4: 0.15, 5: 0.05}, 0.5)
	for id, penalty := range penalties {
		if penalty < 1 {
			t.Fatalf("layer %d penalty %v < 1 (would amplify weight)", id, penalty)
		}
	}
}

// TestZeroDampingDisablesAdjustment α=0 时退化为纯加权平均。
func TestZeroDampingDisablesAdjustment(t *testing.T) {
	matrix, err := BuildCorrelationMatrix(correlatedSamples())
	if err != nil {
		t.Fatalf("BuildCorrelationMatrix: %v", err)
	}

	base := map[int]float64{1: 0.70, 2: 0.10, 4: 0.15, 5: 0.05}
	adjusted := matrix.RedundancyAdjustedWeights(base, 0)

	baseNormalized := normalizeWeights(base)
	for id, weight := range baseNormalized {
		if math.Abs(adjusted[id]-weight) > 1e-12 {
			t.Fatalf("layer %d with damping=0: %v, want base %v", id, adjusted[id], weight)
		}
	}
}

// ============================ 融合引擎 ============================

// TestFusionEngineUsesBaseWeightsBeforeFit 未 Fit 时退化为基准权重的归一化加权。
func TestFusionEngineUsesBaseWeightsBeforeFit(t *testing.T) {
	engine := NewPearsonFusionEngine(DefaultPearsonFusionConfig())

	got := engine.Fuse(map[int]float64{1: 1.0, 2: 0.0})
	// 基准权重 1:0.70, 2:0.10 → (0.70*1.0 + 0.10*0.0)/(0.70+0.10) = 0.875
	if math.Abs(got-0.875) > 1e-12 {
		t.Fatalf("fuse = %v, want 0.875", got)
	}
}

// TestFusionEngineFitAndFuse Fit 后冗余层被下调，融合结果随之变化。
func TestFusionEngineFitAndFuse(t *testing.T) {
	engine := NewPearsonFusionEngine(DefaultPearsonFusionConfig())
	if err := engine.Fit(correlatedSamples()); err != nil {
		t.Fatalf("Fit: %v", err)
	}

	weights := engine.EffectiveWeights()
	total := 0.0
	for _, w := range weights {
		if w < 0 {
			t.Fatalf("negative effective weight: %v", w)
		}
		total += w
	}
	if math.Abs(total-1) > 1e-9 {
		t.Fatalf("effective weights do not sum to 1: %v", total)
	}

	// 层 2 与层 1 完全冗余 → 其有效权重必须低于基准。
	if weights[2] >= 0.10 {
		t.Fatalf("layer 2 effective weight %v not below base 0.10", weights[2])
	}

	// 融合结果必须落在输入区间内。
	fused := engine.Fuse(map[int]float64{1: 0.9, 2: 0.9, 4: 0.1})
	if fused < 0.1 || fused > 0.9 {
		t.Fatalf("fused value %v outside input range [0.1, 0.9]", fused)
	}
}

// TestFitFailurePreservesPreviousState 一次失败的 Fit 不得破坏已标定矩阵。
func TestFitFailurePreservesPreviousState(t *testing.T) {
	engine := NewPearsonFusionEngine(DefaultPearsonFusionConfig())
	if err := engine.Fit(correlatedSamples()); err != nil {
		t.Fatalf("Fit: %v", err)
	}
	before := engine.EffectiveWeights()

	// 空样本 → Fit 失败。
	if err := engine.Fit(nil); err == nil {
		t.Fatal("expected Fit to fail on empty samples")
	}

	after := engine.EffectiveWeights()
	for id, weight := range before {
		if math.Abs(after[id]-weight) > 1e-12 {
			t.Fatalf("failed Fit mutated state: layer %d %v -> %v", id, weight, after[id])
		}
	}
}

// TestFuseIgnoresUnknownLayers 未在权重表中出现的层不参与融合。
func TestFuseIgnoresUnknownLayers(t *testing.T) {
	engine := NewPearsonFusionEngine(PearsonFusionConfig{
		RedundancyDamping: 0,
		BaseWeights:       map[int]float64{1: 1.0},
	})

	got := engine.Fuse(map[int]float64{1: 0.8, 99: 1.0})
	if math.Abs(got-0.8) > 1e-12 {
		t.Fatalf("fuse = %v, want 0.8 (unknown layer must be ignored)", got)
	}
}

// TestFuseWithNoActiveLayerReturnsZero 无任何有效层时返回 0。
func TestFuseWithNoActiveLayerReturnsZero(t *testing.T) {
	engine := NewPearsonFusionEngine(DefaultPearsonFusionConfig())
	if got := engine.Fuse(map[int]float64{99: 1.0}); got != 0 {
		t.Fatalf("fuse = %v, want 0", got)
	}
	if got := engine.Fuse(nil); got != 0 {
		t.Fatalf("fuse(nil) = %v, want 0", got)
	}
}
