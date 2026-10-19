package memory_subgraph

import (
	"math"
	"sort"
	"time"
)

// ============================ 时序滑动衰减矩阵 ============================
//
// 实现策划书 2.2.2 节"时序滑动衰减矩阵（Temporal Slider Decay Matrix）"：
//
//	w(t) = exp(-γ · (t_now - t_write))
//
// 其中 γ 为该特征的漂移率（Drift Rate）。系统按因果模型抽取的属性做
// **分级自适应标定**：
//
//   - 偶发性日常事件（散步、一般寒暄）→ 高漂移率，两周内衰减至趋近 0，
//     防范陈旧信息干扰当前判断；
//   - 长程医学因果共振节点（确诊史、长期医嘱）→ γ 强制规约为 0，
//     权重不随时间衰减，在全生命周期内保持高可信度唤醒。
//
// 命名说明：矩阵体现在 BuildMatrix —— 对一组记忆 × 一组时间刻度
// 批量求解权重，形成 |memories| × |ticks| 的权重矩阵；单点查询则是
// 取矩阵中的一格。这样"矩阵"不是修辞，而是可被检索层整体消费的数据结构。

// DecayClass 记忆的衰减类别，决定漂移率 γ 的取值。
type DecayClass string

const (
	// DecayEphemeral 偶发性日常事件：高漂移，两周内趋近于 0。
	DecayEphemeral DecayClass = "ephemeral"
	// DecayTransient 中等时效信息：一个月量级的衰减。
	DecayTransient DecayClass = "transient"
	// DecayPersistent 长程医学因果共振节点：γ = 0，永久保持权重 1。
	DecayPersistent DecayClass = "persistent"
)

// ephemeralGamma 由"两周内衰减至约 5%"反解：γ = ln(20)/14 ≈ 0.214 /天。
// 取 0.21 使 14 天后权重 ≈ 0.053，符合"趋近于 0"的表述。
const ephemeralGamma = 0.21

// transientGamma 30 天半衰期：γ = ln(2)/30 ≈ 0.0231 /天。
const transientGamma = 0.0231

// DecayPolicy 单个衰减类别的标定策略。
type DecayPolicy struct {
	Class  DecayClass `json:"class"`
	Gamma  float64    `json:"gamma"`  // 漂移率（单位：1/天）
	Pinned bool       `json:"pinned"` // true 表示长程节点，权重恒为 1
}

// HalfLifeDays 返回半衰期（天）。γ=0（pinned）时返回 +Inf。
func (p DecayPolicy) HalfLifeDays() float64 {
	if p.Gamma <= 0 {
		return math.Inf(1)
	}
	return math.Ln2 / p.Gamma
}

// TemporalSliderDecayMatrix 时序滑动衰减矩阵求解器。
//
// 零值不可用，请使用 NewTemporalSliderDecayMatrix 构造。
type TemporalSliderDecayMatrix struct {
	policies map[DecayClass]DecayPolicy
}

// NewTemporalSliderDecayMatrix 使用默认分级标定创建衰减矩阵求解器。
func NewTemporalSliderDecayMatrix() *TemporalSliderDecayMatrix {
	return NewTemporalSliderDecayMatrixWithPolicies(map[DecayClass]DecayPolicy{
		DecayEphemeral:  {Class: DecayEphemeral, Gamma: ephemeralGamma},
		DecayTransient:  {Class: DecayTransient, Gamma: transientGamma},
		DecayPersistent: {Class: DecayPersistent, Gamma: 0, Pinned: true},
	})
}

// NewTemporalSliderDecayMatrixWithPolicies 使用自定义标定创建求解器。
//
// 传入的 persistent 策略会被强制规约为 γ=0：策划书明确要求长程医学
// 节点"强行规约至 0"，该不变量不允许被配置覆盖，否则会退化出
// "确诊史被时间衰减掉"这类医学安全隐患。
func NewTemporalSliderDecayMatrixWithPolicies(policies map[DecayClass]DecayPolicy) *TemporalSliderDecayMatrix {
	normalized := make(map[DecayClass]DecayPolicy, len(policies)+3)
	for class, policy := range policies {
		policy.Class = class
		if policy.Gamma < 0 {
			policy.Gamma = 0
		}
		normalized[class] = policy
	}
	if _, ok := normalized[DecayPersistent]; !ok {
		normalized[DecayPersistent] = DecayPolicy{Class: DecayPersistent, Pinned: true}
	}
	// 硬约束：persistent 永远不衰减。
	persistent := normalized[DecayPersistent]
	persistent.Gamma = 0
	persistent.Pinned = true
	normalized[DecayPersistent] = persistent

	if _, ok := normalized[DecayEphemeral]; !ok {
		normalized[DecayEphemeral] = DecayPolicy{Class: DecayEphemeral, Gamma: ephemeralGamma}
	}
	if _, ok := normalized[DecayTransient]; !ok {
		normalized[DecayTransient] = DecayPolicy{Class: DecayTransient, Gamma: transientGamma}
	}

	return &TemporalSliderDecayMatrix{policies: normalized}
}

// Policy 返回指定类别的标定策略。
func (d *TemporalSliderDecayMatrix) Policy(class DecayClass) DecayPolicy {
	if policy, ok := d.policies[class]; ok {
		return policy
	}
	// 未知类别按中等时效处理，避免"未标定即永久保留"导致陈旧信息污染。
	return d.policies[DecayTransient]
}

// Policies 返回全部标定策略（按类别名排序，便于确定性输出与审计）。
func (d *TemporalSliderDecayMatrix) Policies() []DecayPolicy {
	classes := make([]string, 0, len(d.policies))
	for class := range d.policies {
		classes = append(classes, string(class))
	}
	sort.Strings(classes)

	out := make([]DecayPolicy, 0, len(classes))
	for _, class := range classes {
		out = append(out, d.policies[DecayClass(class)])
	}
	return out
}

// Weight 计算单条记忆在 now 时刻的衰减权重，取值 (0, 1]。
//
// 早于写入时间的查询（时钟回拨、乱序写入）返回 1：宁可不衰减，
// 也不给出 >1 的放大权重。
func (d *TemporalSliderDecayMatrix) Weight(class DecayClass, writtenAt, now time.Time) float64 {
	policy := d.Policy(class)
	if policy.Pinned || policy.Gamma == 0 {
		return 1.0
	}
	ageDays := now.Sub(writtenAt).Hours() / 24.0
	if ageDays <= 0 {
		return 1.0
	}
	return math.Exp(-policy.Gamma * ageDays)
}

// WeightByAgeDays 直接按"已过天数"计算权重，便于测试与离线分析。
func (d *TemporalSliderDecayMatrix) WeightByAgeDays(class DecayClass, ageDays float64) float64 {
	policy := d.Policy(class)
	if policy.Pinned || policy.Gamma == 0 || ageDays <= 0 {
		return 1.0
	}
	return math.Exp(-policy.Gamma * ageDays)
}

// ============================ 分级自适应标定 ============================

// attributeRule 把因果模型抽取的属性映射到衰减类别。
type attributeRule struct {
	key   string
	value string
	class DecayClass
}

// attributeRules 分级自适应标定规则表。
//
// 顺序敏感：**先匹配长程医学属性，再匹配偶发属性**。这样当一条记录
// 同时带 "chronic=true" 与 "activity=walk" 时，医学共振属性优先，
// 避免把确诊史误判为可衰减的日常事件。
var attributeRules = []attributeRule{
	// —— 长程医学因果共振节点：γ 规约为 0 ——
	{key: "chronic", value: "true", class: DecayPersistent},
	{key: "long_term", value: "true", class: DecayPersistent},
	{key: "is_diagnosis", value: "true", class: DecayPersistent},
	{key: "diagnosis", value: "true", class: DecayPersistent},
	{key: "medication_order", value: "long_term", class: DecayPersistent},
	{key: "category", value: "disease", class: DecayPersistent},
	{key: "category", value: "medication", class: DecayPersistent},
	{key: "category", value: "allergy", class: DecayPersistent},

	// —— 偶发性日常事件：高漂移 ——
	{key: "category", value: "activity", class: DecayEphemeral},
	{key: "category", value: "daily_care", class: DecayEphemeral},
	{key: "category", value: "chat", class: DecayEphemeral},
	{key: "ephemeral", value: "true", class: DecayEphemeral},
}

// ClassifyByAttributes 依据因果模型抽取的属性判定衰减类别。
//
// 未命中任何规则时返回 DecayTransient（中等时效），这是有意选择的
// 保守默认：既不会让未知信息永久驻留，也不会让它过快消失。
func ClassifyByAttributes(attributes map[string]string) DecayClass {
	for _, rule := range attributeRules {
		if attributes[rule.key] == rule.value {
			return rule.class
		}
	}
	return DecayTransient
}

// ============================ 矩阵批量求解 ============================

// DecayItem 参与批量求解的一条记忆。
type DecayItem struct {
	MemoryID  string     `json:"memory_id"`
	Class     DecayClass `json:"class"`
	WrittenAt time.Time  `json:"written_at"`
}

// DecayMatrix |memories| × |ticks| 的衰减权重矩阵。
type DecayMatrix struct {
	MemoryIDs []string     `json:"memory_ids"`
	Classes   []DecayClass `json:"classes"`
	Ticks     []time.Time  `json:"ticks"`
	// Weights[i][j] 是第 i 条记忆在第 j 个时间刻度上的权重。
	Weights [][]float64 `json:"weights"`
}

// WeightOf 取矩阵中一格；越界返回 0。
func (dm *DecayMatrix) WeightOf(memoryIndex, tickIndex int) float64 {
	if memoryIndex < 0 || memoryIndex >= len(dm.Weights) {
		return 0
	}
	row := dm.Weights[memoryIndex]
	if tickIndex < 0 || tickIndex >= len(row) {
		return 0
	}
	return row[tickIndex]
}

// BuildMatrix 对一组记忆在一组时间刻度上批量求解衰减权重。
//
// 输入切片会被排序（按 MemoryID、按时间），因此同一批输入无论以何种
// 顺序传入，产出的矩阵完全一致——这是可复现评测的前提。
func (d *TemporalSliderDecayMatrix) BuildMatrix(items []DecayItem, ticks []time.Time) *DecayMatrix {
	ordered := append([]DecayItem(nil), items...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].MemoryID != ordered[j].MemoryID {
			return ordered[i].MemoryID < ordered[j].MemoryID
		}
		return ordered[i].WrittenAt.Before(ordered[j].WrittenAt)
	})

	orderedTicks := append([]time.Time(nil), ticks...)
	sort.Slice(orderedTicks, func(i, j int) bool { return orderedTicks[i].Before(orderedTicks[j]) })

	matrix := &DecayMatrix{
		MemoryIDs: make([]string, 0, len(ordered)),
		Classes:   make([]DecayClass, 0, len(ordered)),
		Ticks:     orderedTicks,
		Weights:   make([][]float64, 0, len(ordered)),
	}

	for _, item := range ordered {
		row := make([]float64, len(orderedTicks))
		for j, tick := range orderedTicks {
			row[j] = d.Weight(item.Class, item.WrittenAt, tick)
		}
		matrix.MemoryIDs = append(matrix.MemoryIDs, item.MemoryID)
		matrix.Classes = append(matrix.Classes, item.Class)
		matrix.Weights = append(matrix.Weights, row)
	}
	return matrix
}

// ApplyDecay 把衰减权重作用到检索分数上。
//
// 与 reranker 中已有的指数衰减（e^(-age/30)）的区别：这里按类别分级——
// 长程医学节点的权重恒为 1，不会被"时间久"误伤。检索层可同时使用两者：
// 通用新鲜度由 reranker 负责，医学语义优先级由本矩阵负责。
func (d *TemporalSliderDecayMatrix) ApplyDecay(score float64, class DecayClass, writtenAt, now time.Time) float64 {
	return score * d.Weight(class, writtenAt, now)
}
