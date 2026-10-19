package memory_subgraph

import (
	"math"
	"testing"
	"time"
)

// TestEphemeralDecaysToNearZeroWithinTwoWeeks 固化策划书"偶发性日常事件
// 设为高漂移率，权重两周内迅速衰减至趋近于 0"。
func TestEphemeralDecaysToNearZeroWithinTwoWeeks(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		days float64
		max  float64
	}{
		{0, 1.0},
		{1, 0.85},
		{7, 0.25},
		{14, 0.06},
		{30, 0.01},
	}
	for _, tc := range cases {
		got := d.Weight(DecayEphemeral, written, written.Add(time.Duration(tc.days*24*float64(time.Hour))))
		if got > tc.max {
			t.Fatalf("ephemeral weight at %.0f days = %.4f, want <= %.2f", tc.days, got, tc.max)
		}
		if got < 0 {
			t.Fatalf("ephemeral weight must stay non-negative, got %v", got)
		}
	}
}

// TestPersistentNodesNeverDecay 固化"具备长程医学因果共振的节点（确诊史、
// 长期医嘱）… 权重不受时间衰减影响，在全生命周期内保持高可信度唤醒"。
//
// 这是安全性关键断言：确诊史绝不能被时间衰减掉。
func TestPersistentNodesNeverDecay(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, days := range []float64{1, 365, 3650, 36500} {
		got := d.Weight(DecayPersistent, written, written.Add(time.Duration(days*24*float64(time.Hour))))
		if got != 1.0 {
			t.Fatalf("persistent weight at %.0f days = %v, want exactly 1.0", days, got)
		}
	}
}

// TestPersistentGammaCannotBeOverriddenByConfig γ=0 是硬不变量，
// 不允许被配置覆盖——否则会退化出"确诊史被衰减"的医学安全隐患。
func TestPersistentGammaCannotBeOverriddenByConfig(t *testing.T) {
	d := NewTemporalSliderDecayMatrixWithPolicies(map[DecayClass]DecayPolicy{
		// 调用方试图给长程节点配一个高漂移率。
		DecayPersistent: {Class: DecayPersistent, Gamma: 0.9},
	})

	policy := d.Policy(DecayPersistent)
	if policy.Gamma != 0 {
		t.Fatalf("persistent gamma = %v, want forced 0", policy.Gamma)
	}
	if !policy.Pinned {
		t.Fatalf("persistent policy must be pinned")
	}

	written := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := d.Weight(DecayPersistent, written, written.Add(1000*24*time.Hour)); got != 1.0 {
		t.Fatalf("persistent weight = %v, want 1.0 despite configured gamma", got)
	}
}

// TestClassifyByAttributes 验证分级自适应标定：先判长程医学属性，
// 再判偶发属性，未知默认中等时效。
func TestClassifyByAttributes(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  DecayClass
	}{
		{"确诊史", map[string]string{"chronic": "true"}, DecayPersistent},
		{"长期医嘱", map[string]string{"medication_order": "long_term"}, DecayPersistent},
		{"疾病类别", map[string]string{"category": "disease"}, DecayPersistent},
		{"过敏史", map[string]string{"category": "allergy"}, DecayPersistent},
		{"散步", map[string]string{"category": "activity"}, DecayEphemeral},
		{"日常寒暄", map[string]string{"category": "chat"}, DecayEphemeral},
		{"未标注", map[string]string{}, DecayTransient},
		{"未知类别", map[string]string{"category": "unknown-thing"}, DecayTransient},
		// 医学属性优先于偶发属性。
		{"既标活动又标慢病", map[string]string{"category": "activity", "chronic": "true"}, DecayPersistent},
	}
	for _, tc := range cases {
		if got := ClassifyByAttributes(tc.attrs); got != tc.want {
			t.Fatalf("%s: ClassifyByAttributes(%v) = %q, want %q", tc.name, tc.attrs, got, tc.want)
		}
	}
}

// TestBuildMatrixShapeAndValues 验证矩阵形状与逐格取值。
func TestBuildMatrixShapeAndValues(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	items := []DecayItem{
		{MemoryID: "m-chronic", Class: DecayPersistent, WrittenAt: written},
		{MemoryID: "m-walk", Class: DecayEphemeral, WrittenAt: written},
	}
	ticks := []time.Time{
		written,
		written.Add(7 * 24 * time.Hour),
		written.Add(30 * 24 * time.Hour),
	}

	matrix := d.BuildMatrix(items, ticks)
	if len(matrix.Weights) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(matrix.Weights))
	}
	if len(matrix.Weights[0]) != 3 {
		t.Fatalf("expected 3 ticks per row, got %d", len(matrix.Weights[0]))
	}

	// 行按 MemoryID 排序：m-chronic 在前。
	if matrix.MemoryIDs[0] != "m-chronic" || matrix.MemoryIDs[1] != "m-walk" {
		t.Fatalf("rows not sorted by memory ID: %v", matrix.MemoryIDs)
	}
	for j := 0; j < 3; j++ {
		if got := matrix.WeightOf(0, j); got != 1.0 {
			t.Fatalf("persistent row tick %d = %v, want 1.0", j, got)
		}
	}
	if got := matrix.WeightOf(1, 0); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("ephemeral at t0 = %v, want 1.0", got)
	}
	if got := matrix.WeightOf(1, 2); got > 0.01 {
		t.Fatalf("ephemeral at 30d = %v, want near 0", got)
	}
	// 越界访问返回 0，不 panic。
	if got := matrix.WeightOf(99, 0); got != 0 {
		t.Fatalf("out-of-range row = %v, want 0", got)
	}
	if got := matrix.WeightOf(0, 99); got != 0 {
		t.Fatalf("out-of-range tick = %v, want 0", got)
	}
}

// TestBuildMatrixIsOrderIndependent 输入顺序不影响输出——
// 可复现评测要求矩阵逐位可比。
func TestBuildMatrixIsOrderIndependent(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	forward := []DecayItem{
		{MemoryID: "a", Class: DecayEphemeral, WrittenAt: written},
		{MemoryID: "b", Class: DecayPersistent, WrittenAt: written},
		{MemoryID: "c", Class: DecayTransient, WrittenAt: written},
	}
	reversed := []DecayItem{forward[2], forward[1], forward[0]}
	ticks := []time.Time{written.Add(72 * time.Hour), written}

	m1 := d.BuildMatrix(forward, ticks)
	m2 := d.BuildMatrix(reversed, ticks)

	if len(m1.Weights) != len(m2.Weights) {
		t.Fatal("row count mismatch")
	}
	for i := range m1.Weights {
		if m1.MemoryIDs[i] != m2.MemoryIDs[i] {
			t.Fatalf("row %d id mismatch: %s vs %s", i, m1.MemoryIDs[i], m2.MemoryIDs[i])
		}
		for j := range m1.Weights[i] {
			if m1.Weights[i][j] != m2.Weights[i][j] {
				t.Fatalf("cell [%d][%d] mismatch: %v vs %v", i, j, m1.Weights[i][j], m2.Weights[i][j])
			}
		}
	}
}

// TestClockSkewDoesNotAmplify 时钟回拨（写入时间晚于查询时间）时
// 返回 1 而非 >1，避免异常放大。
func TestClockSkewDoesNotAmplify(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // 早于写入时间

	if got := d.Weight(DecayEphemeral, written, now); got != 1.0 {
		t.Fatalf("clock skew weight = %v, want 1.0 (no amplification)", got)
	}
}

// TestHalfLifeDerivation 半衰期与 γ 的一致性：γ=ln2/T ⇔ 一个半衰期后权重 0.5。
func TestHalfLifeDerivation(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()

	for _, class := range []DecayClass{DecayEphemeral, DecayTransient} {
		policy := d.Policy(class)
		halfLife := policy.HalfLifeDays()
		if halfLife <= 0 || math.IsInf(halfLife, 0) {
			t.Fatalf("%s: unexpected half life %v", class, halfLife)
		}
		got := d.WeightByAgeDays(class, halfLife)
		if math.Abs(got-0.5) > 1e-6 {
			t.Fatalf("%s: weight at half life = %v, want 0.5", class, got)
		}
	}

	if !math.IsInf(d.Policy(DecayPersistent).HalfLifeDays(), 1) {
		t.Fatal("persistent half life should be +Inf (never decays)")
	}
}

// TestUnknownClassDefaultsToTransient 未标定类别不得退化为"永久保留"。
func TestUnknownClassDefaultsToTransient(t *testing.T) {
	d := NewTemporalSliderDecayMatrix()
	written := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := d.Weight(DecayClass("never-heard-of"), written, written.Add(365*24*time.Hour))
	if got >= 0.01 {
		t.Fatalf("unknown class weight after 1 year = %v, want decayed (< 0.01)", got)
	}
}
