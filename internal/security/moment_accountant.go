package security

import (
	"fmt"
	"math"
)

// ============================================================================
// 矩会计（Moments Accountant）
// ============================================================================
//
// 实现策划书 2.4.2 节"后端数学加噪"中的隐私预算跟踪机制：
//
//	"由于智慧养老长期记忆系统具有连续写入和连续检索特征，单次隐私预算
//	 控制不足以描述长期风险。系统需要跟踪多次查询和多次更新带来的隐私
//	 损失累积，因此引入矩会计机制，通过追踪隐私损失随机变量的对数矩
//	 获得更紧致的组合边界。"
//
// 数学基础
// --------
// 设单步机制的隐私损失随机变量为 Z，其对数量矩为
//
//	α(λ) = log E[e^{λZ}]            （λ 为矩阶）
//
// 独立机制的复合满足对数矩可加：α_total(λ) = Σ_j α_j(λ)。
// 由 Markov/Chernoff 不等式可得隐私损失超过 ε 的概率上界
//
//	Pr[Z_total ≥ ε] ≤ exp( α_total(λ) − λ·ε )
//
// 令该概率等于 δ 并解出 ε，即得组合后的 (ε, δ)-DP 参数：
//
//	ε(δ) = min_λ  ( α_total(λ) + ln(1/δ) ) / λ
//
// 对 λ 取最小值使边界最紧——这正是"矩会计"相对基础组合定理
// （ε_total = Σε_j，线性累加）更紧致的原因：它允许我们在
// "矩阶"这一自由维度上做优化，而不是被动接受线性累加。
//
// 拉普拉斯机制的精确对数矩
// ------------------------
// 对敏感度 Δ、尺度 b = Δ/ε 的拉普拉斯机制，隐私损失 Z 取值于 [−ε, ε]。
// 设 X ~ Lap(0, b) 与 Y ~ Lap(Δ, b)，则 Z = log(f_X/f_Y)。直接积分可得
// 其矩母函数（与 Mironov《Rényi Differential Privacy》中拉普拉斯机制的
// 精确 RDP 公式 α(λ)=(1/(λ-1))·log[ (λ e^{(λ-1)ε} + (λ-1)e^{-λε})/(2λ-1) ]
// 完全一致，此处以矩阶 λ 表示为等价形式）：
//
//	MGF(λ) = [ (λ+1)·e^{λε} + λ·e^{-(λ+1)ε} ] / (1 + 2λ)
//
// 代入 α(λ) = log MGF(λ) 即得单步对数矩。
//
// 子采样放大（Subsampling）
// ------------------------
// 当一次查询只触及采样率为 q 的子集时，隐私损失以概率 (1−q) 退化为 0，
// 于是矩母函数变为混合形式：
//
//	MGF_sub(λ) = (1−q) + q·MGF(λ)
//
// 该式对 q=1 自动退化为无放大的情形；对 q→0 则给出 α(λ) → q·(MGF(λ)−1)，
// 即风险随采样率同阶下降。
//
// 边界声明（如实标注）
// ------------------
//   - 本实现给出的是**有效的上界**，不是精确的最优解；子采样情形采用
//     上述混合矩公式，未采用"子采样 RDP"中 q² 阶的更强放大界，
//     因此在极小采样率 + 极大步数的最优区间内会略逊于解析矩会计。
//   - 矩会计相对**基础组合定理（线性累加）**的紧致性有严格保证，
//     这也是本实现回归测试所固化的性质；相对**高级组合定理**的优劣
//     取决于 ε 与步数所处的区间，不应一概而论。

// defaultMomentOrders 默认矩阶网格。
//
// 覆盖数量级：小矩阶在"步数多、单步 ε 小"时占优，大矩阶在
// "步数少、单步 ε 大"时占优。网格在对数尺度上近似均匀，
// 使得 4 个数量级的步数变化都能落在最优矩阶附近。
var defaultMomentOrders = []float64{
	0.125, 0.25, 0.5, 1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10,
	12, 16, 20, 24, 32, 48, 64, 96, 128,
}

// DefaultMomentOrders 返回默认矩阶网格的副本。
func DefaultMomentOrders() []float64 {
	out := make([]float64, len(defaultMomentOrders))
	copy(out, defaultMomentOrders)
	return out
}

// PrivacyStep 一次差分隐私操作。
type PrivacyStep struct {
	// Epsilon 该步机制的隐私预算（敏感度 Δ 与尺度 b 之比 Δ/b）。
	Epsilon float64 `json:"epsilon"`
	// SamplingRate 子采样率 q ∈ (0, 1]。1 表示不采样（每次查询都触及全量数据）。
	SamplingRate float64 `json:"sampling_rate"`
	// Label 可选的操作标签，便于审计时定位隐私消耗来源。
	Label string `json:"label,omitempty"`
}

// OrderBound 单个矩阶上的边界分解，用于审计"为什么取到该 ε"。
type OrderBound struct {
	Order     float64 `json:"order"`      // 矩阶 λ
	LogMoment float64 `json:"log_moment"` // α_total(λ)
	Epsilon   float64 `json:"epsilon"`    // (α(λ) + ln(1/δ)) / λ
}

// MomentAccountant 矩会计累加器。
//
// 非并发安全：调用方应在单一线程内累加，或在外部加锁。
type MomentAccountant struct {
	orders     []float64
	logMoments map[float64]float64 // 每个矩阶上的累计对数矩 α_total(λ)
	basicEps   float64             // 基础组合（线性累加）对照值
	steps      int
}

// NewMomentAccountant 使用给定矩阶创建矩会计。
//
// 非法矩阶（≤ 0 或非有限值）会被丢弃；若过滤后为空则回退到默认网格，
// 以避免出现"无矩阶可用"导致 ε 恒为 0 的静默失效。
func NewMomentAccountant(orders []float64) *MomentAccountant {
	clean := make([]float64, 0, len(orders))
	seen := make(map[float64]struct{}, len(orders))
	for _, order := range orders {
		if order <= 0 || math.IsNaN(order) || math.IsInf(order, 0) {
			continue
		}
		if _, dup := seen[order]; dup {
			continue
		}
		seen[order] = struct{}{}
		clean = append(clean, order)
	}
	if len(clean) == 0 {
		clean = DefaultMomentOrders()
	}
	return &MomentAccountant{
		orders:     clean,
		logMoments: make(map[float64]float64, len(clean)),
	}
}

// NewDefaultMomentAccountant 使用默认矩阶网格创建矩会计。
func NewDefaultMomentAccountant() *MomentAccountant {
	return NewMomentAccountant(nil)
}

// Accumulate 累加一次隐私操作。
//
// 允许 Epsilon == 0（无损操作，例如命中缓存的重复查询）：它不增加
// 隐私消耗，但仍计入 Steps，使调用方能如实统计"操作次数"与
// "真正产生隐私消耗的次数"的差异。
func (ma *MomentAccountant) Accumulate(step PrivacyStep) error {
	if step.Epsilon < 0 || math.IsNaN(step.Epsilon) || math.IsInf(step.Epsilon, 0) {
		return fmt.Errorf("moment_accountant: invalid epsilon %v", step.Epsilon)
	}
	samplingRate := step.SamplingRate
	if samplingRate == 0 {
		samplingRate = 1.0 // 未指定即视为全量采样。
	}
	if samplingRate < 0 || samplingRate > 1 {
		return fmt.Errorf("moment_accountant: sampling rate %.4f out of range (0, 1]", samplingRate)
	}

	for _, order := range ma.orders {
		ma.logMoments[order] += SubsampledLaplaceLogMoment(order, step.Epsilon, samplingRate)
	}
	ma.basicEps += step.Epsilon
	ma.steps++
	return nil
}

// Steps 返回已累加的操作次数（含零消耗操作）。
func (ma *MomentAccountant) Steps() int { return ma.steps }

// BasicEpsilon 返回基础组合定理给出的 ε（线性累加），用作对照基线。
func (ma *MomentAccountant) BasicEpsilon() float64 { return ma.basicEps }

// LogMoment 返回指定矩阶上的累计对数矩 α_total(λ)。
// 若该矩阶不在网格内，返回 0。
func (ma *MomentAccountant) LogMoment(order float64) float64 {
	return ma.logMoments[order]
}

// Epsilon 计算组合后的隐私预算 ε（给定目标失败概率 δ）。
//
// δ 必须落在 (0, 1) 开区间：δ=0 对应纯 DP 保证，无法由矩会计给出；
// δ≥1 使 ln(1/δ) ≤ 0，边界失去意义。
// 无任何操作时返回 0。
func (ma *MomentAccountant) Epsilon(delta float64) (float64, error) {
	if !(delta > 0 && delta < 1) {
		return 0, fmt.Errorf("moment_accountant: delta %.6g out of range (0, 1)", delta)
	}
	if ma.steps == 0 {
		return 0, nil
	}
	// 全部操作都是零消耗（如全部命中缓存）时，隐私损失恒为 0，
	// 真实 ε 应为 0。若不加此短路，尾界 δ = e^{-λε} 在有限矩阶下会把
	// ε 推到 ln(1/δ)/λ > 0，凭空制造出并不存在的隐私消耗。
	if ma.basicEps == 0 {
		return 0, nil
	}

	logInverseDelta := math.Log(1 / delta)
	best := math.Inf(1)
	for _, order := range ma.orders {
		candidate := (ma.logMoments[order] + logInverseDelta) / order
		if candidate < best {
			best = candidate
		}
	}
	if best < 0 {
		best = 0 // 数值噪声可能产生极小负值，ε 不可为负。
	}
	return best, nil
}

// BestEpsilon 返回两种有效边界中更紧的一个。
//
// 两种边界分别是：
//  1. 矩会计边界（Epsilon）；
//  2. 基础组合定理的线性累加（BasicEpsilon）。
//
// 二者都是合法上界——基础组合给出 (Σε, 0)-DP，而 (Σε, 0)-DP 蕴含
// (Σε, δ)-DP。因此在同一 δ 下取较小者恒为合法且更优。
//
// 为什么需要它：矩会计边界在"步数少、单步 ε 大"的区间会劣于线性累加
// （ln(1/δ)/λ 项尚未被步数摊薄），此时正确的做法是退回基础组合，
// 而不是硬报一个更松的数字。这是真实的数学性质，不应被掩盖。
func (ma *MomentAccountant) BestEpsilon(delta float64) (float64, error) {
	moment, err := ma.Epsilon(delta)
	if err != nil {
		return 0, err
	}
	if ma.basicEps < moment {
		return ma.basicEps, nil
	}
	return moment, nil
}

// OrderBounds 返回各矩阶的边界分解，供审计与调参使用。
// 结果按矩阶升序排列。
func (ma *MomentAccountant) OrderBounds(delta float64) ([]OrderBound, error) {
	if !(delta > 0 && delta < 1) {
		return nil, fmt.Errorf("moment_accountant: delta %.6g out of range (0, 1)", delta)
	}
	logInverseDelta := math.Log(1 / delta)

	out := make([]OrderBound, 0, len(ma.orders))
	for _, order := range ma.orders {
		alpha := ma.logMoments[order]
		out = append(out, OrderBound{
			Order:     order,
			LogMoment: alpha,
			Epsilon:   (alpha + logInverseDelta) / order,
		})
	}
	return out, nil
}

// Reset 清空累加状态，用于新的隐私预算周期。
func (ma *MomentAccountant) Reset() {
	ma.logMoments = make(map[float64]float64, len(ma.orders))
	ma.basicEps = 0
	ma.steps = 0
}

// ============================================================================
// 对数矩计算
// ============================================================================

// LaplaceLogMoment 返回拉普拉斯机制在矩阶 λ、隐私参数 ε 下的精确对数矩
//
//	α(λ) = log [ (λ+1)·e^{λε} + λ·e^{-(λ+1)ε} ] − log(1 + 2λ)
//
// 该式在 λ=1 时即 α(1) = log[ (2e^ε + e^{-2ε}) / 3 ]，与精确 RDP
// D_2 = log[(2e^ε + e^{-2ε})/3] 一致，可直接用 RDP 公式交叉验证。
//
// 大 λε 时用 log-sum-exp 计算，避免 e^{λε} 溢出为 +Inf。
func LaplaceLogMoment(order, epsilon float64) float64 {
	// λ=0 时矩恒为 1，对数矩为 0（单独处理以免 0/0）。
	if order == 0 {
		return 0
	}
	// ε=0 表示该步无隐私损失，矩恒为 1。
	if epsilon == 0 {
		return 0
	}

	term1 := math.Log(order+1) + order*epsilon   // log( (λ+1)e^{λε} )
	term2 := math.Log(order) - (order+1)*epsilon // log( λ e^{-(λ+1)ε} )

	return logSumExp(term1, term2) - math.Log(1+2*order)
}

// SubsampledLaplaceLogMoment 返回 q-子采样的拉普拉斯机制的对数矩
//
//	α_sub(λ) = log [ (1−q) + q·e^{α(λ)} ]
//
// q=1 时退化为 LaplaceLogMoment；q=0 时恒为 0。
func SubsampledLaplaceLogMoment(order, epsilon, samplingRate float64) float64 {
	if samplingRate >= 1 {
		return LaplaceLogMoment(order, epsilon)
	}
	if samplingRate <= 0 {
		return 0
	}

	logMoment := LaplaceLogMoment(order, epsilon)

	// log(1−q) 与 log(q)+α 的 log-sum-exp。
	// q=1 已在上方短路，故此处 log(1−q) 必为有限值。
	return logSumExp(math.Log(1-samplingRate), math.Log(samplingRate)+logMoment)
}

// ComposeMomentBound 便捷函数：对一组操作直接给出矩会计组合后的 ε。
func ComposeMomentBound(steps []PrivacyStep, delta float64) (float64, error) {
	accountant := NewDefaultMomentAccountant()
	for i, step := range steps {
		if err := accountant.Accumulate(step); err != nil {
			return 0, fmt.Errorf("step %d: %w", i, err)
		}
	}
	return accountant.Epsilon(delta)
}

// logSumExp 数值稳定的 log(e^a + e^b)。
// 两个入参同时为 -Inf 时返回 -Inf（表示概率为 0 的退化情形）。
func logSumExp(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	if a < b {
		a, b = b, a
	}
	return a + math.Log1p(math.Exp(b-a))
}
