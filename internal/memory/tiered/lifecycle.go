// Package tiered 实现记忆数据的冷热五阶段生命周期调度。
//
// 策划书 2.5 节描述的存储生命周期：
//
//	热层（<48h，全量内存，可即时检索）
//	  → 温层（<15d，压缩存储）
//	  → 冷层（归档，ASDF 文件 + LZ4 压缩，mmap 按需加载）
//	  → 冻结层（仅保留元数据索引，需解冻才能访问）
//	  → 遗忘（机器遗忘执行后或超出保留期的终态）
//
// 设计原则
// --------
//  1. **判定与执行分离**：`Evaluate` 只产出"应该发生什么"的报告，
//     `Apply` 才真正执行副作用（归档/删除）。这样同一份报告可以先
//     审计、再执行，也便于测试只验证判定逻辑。
//  2. **存储解耦**：冷层/冻结层需要落到具体存储后端（ASDF 文件、
//     LZ4、mmap），本包通过 `Archiver` 接口依赖它，不直接引用存储包——
//     因此本包可以独立测试，也能适配不同后端。
//  3. **内存预算是硬约束**：热层与温层受内存预算限制，超限时按
//     "冷度"排序强制降级，而不是任由内存增长。
package tiered

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Tier 生命周期阶段。数值越大越"冷"。
type Tier int

const (
	// TierHot 热层：全量在内存，可即时检索。
	TierHot Tier = iota
	// TierWarm 温层：压缩存储，检索前需解压。
	TierWarm
	// TierCold 冷层：已归档到存储后端，按需加载（mmap）。
	TierCold
	// TierFrozen 冻结层：仅保留元数据索引，访问前需解冻。
	TierFrozen
	// TierForgotten 遗忘：终态，数据已不可恢复。
	TierForgotten
)

// tierName 各阶段名称。
var tierName = map[Tier]string{
	TierHot:       "热层",
	TierWarm:      "温层",
	TierCold:      "冷层",
	TierFrozen:    "冻结层",
	TierForgotten: "遗忘",
}

// String 返回阶段的可读名称。
func (t Tier) String() string {
	if name, ok := tierName[t]; ok {
		return name
	}
	return fmt.Sprintf("未知阶段(%d)", int(t))
}

// IsResident 该阶段的数据是否驻留在内存中（受内存预算约束）。
func (t Tier) IsResident() bool {
	return t == TierHot || t == TierWarm
}

// TierPolicy 分层策略：各层的驻留时限与内存预算。
type TierPolicy struct {
	// HotMaxAge 热层最长驻留时间（默认 48h）。
	HotMaxAge time.Duration
	// WarmMaxAge 温层最长驻留时间（默认 15d）。
	WarmMaxAge time.Duration
	// ColdMaxAge 冷层最长驻留时间（默认 90d），超时进入冻结。
	ColdMaxAge time.Duration
	// FrozenMaxAge 冻结层最长保留时间（默认 365d），超时进入遗忘。
	FrozenMaxAge time.Duration

	// HotBudgetBytes 热层内存预算（字节）。
	HotBudgetBytes int64
	// WarmBudgetBytes 温层内存预算（字节）。
	WarmBudgetBytes int64

	// MinAccessCountToKeepWarm 温层保级所需的最低访问次数。
	// 访问频繁的"温"数据不因时长被降冷——这是策划书"长程医学节点
	// 锁定"思想在存储层的对应：高价值记忆不应被时间盲目驱逐。
	MinAccessCountToKeepWarm int
}

// DefaultTierPolicy 返回策划书默认策略：热 48h、温 15d、冷 90d、冻结 365d。
func DefaultTierPolicy() TierPolicy {
	return TierPolicy{
		HotMaxAge:                48 * time.Hour,
		WarmMaxAge:               15 * 24 * time.Hour,
		ColdMaxAge:               90 * 24 * time.Hour,
		FrozenMaxAge:             365 * 24 * time.Hour,
		HotBudgetBytes:           256 << 20, // 256 MiB
		WarmBudgetBytes:          512 << 20, // 512 MiB
		MinAccessCountToKeepWarm: 3,
	}
}

// validated 返回归一化后的策略：把非正值的时限补齐为默认值。
//
// 零值时限会导致"所有数据立刻过期"，是典型的配置事故，这里兜底修正。
func (p TierPolicy) validated() TierPolicy {
	defaults := DefaultTierPolicy()
	if p.HotMaxAge <= 0 {
		p.HotMaxAge = defaults.HotMaxAge
	}
	if p.WarmMaxAge <= 0 {
		p.WarmMaxAge = defaults.WarmMaxAge
	}
	if p.ColdMaxAge <= 0 {
		p.ColdMaxAge = defaults.ColdMaxAge
	}
	if p.FrozenMaxAge <= 0 {
		p.FrozenMaxAge = defaults.FrozenMaxAge
	}
	if p.HotBudgetBytes < 0 {
		p.HotBudgetBytes = 0
	}
	if p.WarmBudgetBytes < 0 {
		p.WarmBudgetBytes = 0
	}
	if p.MinAccessCountToKeepWarm < 0 {
		p.MinAccessCountToKeepWarm = 0
	}
	return p
}

// MemoryItem 一条受生命周期管理的记忆数据。
type MemoryItem struct {
	// ID 唯一标识。
	ID string
	// SizeBytes 该条数据在内存中的占用。
	SizeBytes int64
	// CreatedAt 创建时间。
	CreatedAt time.Time
	// LastAccessedAt 最近一次访问时间。
	LastAccessedAt time.Time
	// AccessCount 累计访问次数。
	AccessCount int
	// Tier 当前所处阶段。
	Tier Tier
	// ForgetRequested 是否已请求机器遗忘（一旦为 true，直接进遗忘态）。
	ForgetRequested bool
}

// Archiver 冷层/冻结层所需的存储后端。
//
// 由存储层（ASDF 文件 + LZ4 + mmap）实现。本包只依赖接口，
// 便于独立测试，也便于替换后端。
type Archiver interface {
	// Archive 把数据写入归档存储。
	Archive(id string, payload []byte) error
	// Restore 从归档存储读回数据。
	Restore(id string) ([]byte, error)
	// Delete 删除归档数据。
	Delete(id string) error
}

// PayloadProvider 在需要归档时提供原始字节。
//
// 单独抽象出来的原因：生命周期管理器本身不持有数据内容（只持有元数据），
// 真正落盘时才向数据源索要字节。
type PayloadProvider interface {
	// Payload 返回指定条目的字节内容；条目不存在返回错误。
	Payload(id string) ([]byte, error)
}

// Transition 一次阶段迁移（判定结果或执行结果）。
type Transition struct {
	ItemID string `json:"item_id"`
	From   Tier   `json:"from"`
	To     Tier   `json:"to"`
	Reason string `json:"reason"`
}

// BudgetPressure 内存预算压力统计。
type BudgetPressure struct {
	HotBytes            int64 `json:"hot_bytes"`
	WarmBytes           int64 `json:"warm_bytes"`
	HotBudgetBytes      int64 `json:"hot_budget_bytes"`
	WarmBudgetBytes     int64 `json:"warm_budget_bytes"`
	HotOverBudget       bool  `json:"hot_over_budget"`
	WarmOverBudget      bool  `json:"warm_over_budget"`
	BytesReleasedToWarm int64 `json:"bytes_released_to_warm"`
	BytesReleasedToCold int64 `json:"bytes_released_to_cold"`
}

// Report 一次生命周期评估的结果。
type Report struct {
	EvaluatedAt    time.Time      `json:"evaluated_at"`
	Transitions    []Transition   `json:"transitions"`
	BudgetPressure BudgetPressure `json:"budget_pressure"`
	ByTier         map[Tier]int   `json:"by_tier"`
}

// Manager 冷热生命周期管理器。并发安全。
type Manager struct {
	mu       sync.RWMutex
	policy   TierPolicy
	items    map[string]*MemoryItem
	archiver Archiver
	provider PayloadProvider
}

// NewManager 创建生命周期管理器。
//
// archiver 与 provider 可为 nil：nil 时冷层迁移只改元数据、不落盘，
// 适用于"只想看判定结果"的评估场景与单元测试。
func NewManager(policy TierPolicy, archiver Archiver, provider PayloadProvider) *Manager {
	return &Manager{
		policy:   policy.validated(),
		items:    make(map[string]*MemoryItem),
		archiver: archiver,
		provider: provider,
	}
}

// Add 登记一条记忆数据。ID 为空或重复时返回错误。
func (m *Manager) Add(item *MemoryItem) error {
	if item == nil || item.ID == "" {
		return fmt.Errorf("tiered: invalid item")
	}
	if item.SizeBytes < 0 {
		return fmt.Errorf("tiered: negative size for %q", item.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.items[item.ID]; exists {
		return fmt.Errorf("tiered: duplicate item %q", item.ID)
	}
	copied := *item
	m.items[item.ID] = &copied
	return nil
}

// Touch 记录一次访问：更新最近访问时间与计数，并把数据"回温"到热层。
//
// 回温是检索系统的自然语义——被访问的数据理应重新变得易取。
func (m *Manager) Touch(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, ok := m.items[id]
	if !ok {
		return fmt.Errorf("tiered: unknown item %q", id)
	}
	item.LastAccessedAt = time.Now()
	item.AccessCount++
	if item.Tier != TierForgotten {
		item.Tier = TierHot
	}
	return nil
}

// Item 返回指定条目的快照。
func (m *Manager) Item(id string) (MemoryItem, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[id]
	if !ok {
		return MemoryItem{}, false
	}
	return *item, true
}

// SetForgetRequested 设置条目的机器遗忘标记。
//
// 为什么必须提供这个方法，而不是让调用方改 Item 返回的副本：
// Item 返回的是**值快照**，调用方在其上修改 ForgetRequested 只会改到副本，
// 对管理器内部的条目毫无影响——这类 bug 静默无声，调用方会以为遗忘已生效。
// 因此修改类操作必须由管理器自己、在锁内完成。
func (m *Manager) SetForgetRequested(id string, forget bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok {
		return fmt.Errorf("未知条目 %q", id)
	}
	item.ForgetRequested = forget
	return nil
}

// Classify 依据策略判定一条数据**应当**处于哪个阶段。
//
// 判定优先级：
//  1. 已请求遗忘 → 遗忘（最高优先级，机器遗忘请求不可被时间策略覆盖）
//  2. 已处于遗忘 → 保持遗忘（终态，不可复活）
//  3. 未访问时长超过各层时限 → 降级
//  4. 温层数据若访问频繁，允许"超时不降级"（保级）
func (m *Manager) Classify(item *MemoryItem, now time.Time) Tier {
	if item.ForgetRequested || item.Tier == TierForgotten {
		return TierForgotten
	}

	idle := now.Sub(item.LastAccessedAt)
	policy := m.policy

	switch item.Tier {
	case TierHot:
		if idle >= policy.HotMaxAge {
			return TierWarm
		}
		return TierHot
	case TierWarm:
		if idle < policy.WarmMaxAge {
			return TierWarm
		}
		// 访问频繁的数据不因时长被降冷：保留在温层。
		if item.AccessCount >= policy.MinAccessCountToKeepWarm {
			return TierWarm
		}
		return TierCold
	case TierCold:
		if idle >= policy.ColdMaxAge {
			return TierFrozen
		}
		return TierCold
	case TierFrozen:
		if idle >= policy.FrozenMaxAge {
			return TierForgotten
		}
		return TierFrozen
	default:
		return item.Tier
	}
}

// Evaluate 评估全部条目，产出"应发生哪些迁移"的报告，不改变任何状态。
func (m *Manager) Evaluate() *Report {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	report := &Report{
		EvaluatedAt: now,
		ByTier:      make(map[Tier]int, len(tierName)),
	}

	for _, item := range sortedItems(m.items) {
		target := m.Classify(item, now)
		report.ByTier[item.Tier]++
		if target != item.Tier {
			report.Transitions = append(report.Transitions, Transition{
				ItemID: item.ID,
				From:   item.Tier,
				To:     target,
				Reason: m.transitionReason(item, target, now),
			})
		}
	}

	report.BudgetPressure = m.budgetPressureLocked()
	return report
}

// EnforceBudget 施加内存预算约束：热层/温层超限时，按"冷度"从高到低
// 把数据降级，直到回到预算内。
//
// 冷度排序：最近访问时间越早越冷；访问次数越少越冷。这样既保证
// 预算是硬约束，又尽量保留高频访问的数据在内存里。
func (m *Manager) EnforceBudget() []Transition {
	m.mu.Lock()
	defer m.mu.Unlock()

	var demoted []Transition

	for _, target := range []Tier{TierHot, TierWarm} {
		budget := m.policy.HotBudgetBytes
		nextTier := TierWarm
		if target == TierWarm {
			budget = m.policy.WarmBudgetBytes
			nextTier = TierCold
		}

		resident := m.residentItemsLocked(target)
		if len(resident) == 0 {
			continue
		}

		// 按冷度降序排一次，之后顺序驱逐——避免在循环里反复排序。
		sort.Slice(resident, func(i, j int) bool {
			return coldness(resident[i]) > coldness(resident[j])
		})

		used := int64(0)
		for _, item := range resident {
			used += item.SizeBytes
		}
		initialUsed := used
		if used <= budget {
			continue
		}

		released := int64(0)
		for _, item := range resident {
			if used <= budget {
				break
			}
			item.Tier = nextTier
			used -= item.SizeBytes
			released += item.SizeBytes
			demoted = append(demoted, Transition{
				ItemID: item.ID,
				From:   target,
				To:     nextTier,
				Reason: fmt.Sprintf("内存预算超限（%s 已用 %d 字节 > 预算 %d 字节），按冷度驱逐，本条释放 %d 字节",
					target, initialUsed, budget, released),
			})
		}
	}
	return demoted
}

// Apply 执行评估报告中的迁移，产生真实副作用（归档 / 删除）。
//
// 返回每条迁移的执行结果：失败项会在 Result.Error 中说明原因，
// 且不中断后续迁移——批量调度不应因单条失败而整体中止。
func (m *Manager) Apply(report *Report) []TransitionResult {
	if report == nil {
		return nil
	}

	results := make([]TransitionResult, 0, len(report.Transitions))
	for _, transition := range report.Transitions {
		results = append(results, m.applyOne(transition))
	}
	return results
}

// TransitionResult 单条迁移的执行结果。
type TransitionResult struct {
	Transition Transition `json:"transition"`
	Applied    bool       `json:"applied"`
	Error      string     `json:"error,omitempty"`
}

// applyOne 执行单条迁移。
func (m *Manager) applyOne(transition Transition) TransitionResult {
	result := TransitionResult{Transition: transition}

	m.mu.Lock()
	item, ok := m.items[transition.ItemID]
	if !ok {
		m.mu.Unlock()
		result.Error = fmt.Sprintf("未知条目 %q", transition.ItemID)
		return result
	}
	// 复核：条目状态可能在 Evaluate 之后又被 Touch 修改过。
	current := m.Classify(item, time.Now())
	if current != transition.To {
		m.mu.Unlock()
		result.Error = fmt.Sprintf("状态已变化，期望 %s 实际 %s", transition.To, current)
		return result
	}
	m.mu.Unlock()

	// 迁出到冷层/冻结层时需要落盘归档。
	if transition.To == TierCold && m.archiver != nil && m.provider != nil {
		payload, err := m.provider.Payload(transition.ItemID)
		if err != nil {
			result.Error = fmt.Sprintf("读取载荷失败: %v", err)
			return result
		}
		if err := m.archiver.Archive(transition.ItemID, payload); err != nil {
			result.Error = fmt.Sprintf("归档失败: %v", err)
			return result
		}
	}

	// 进入遗忘态时删除归档数据。
	if transition.To == TierForgotten && m.archiver != nil {
		if err := m.archiver.Delete(transition.ItemID); err != nil {
			result.Error = fmt.Sprintf("删除归档失败: %v", err)
			return result
		}
	}

	m.mu.Lock()
	item.Tier = transition.To
	m.mu.Unlock()

	result.Applied = true
	return result
}

// ============================================================================
// 内部工具
// ============================================================================

// budgetPressureLocked 统计当前内存预算压力。调用方须持锁。
func (m *Manager) budgetPressureLocked() BudgetPressure {
	pressure := BudgetPressure{
		HotBudgetBytes:  m.policy.HotBudgetBytes,
		WarmBudgetBytes: m.policy.WarmBudgetBytes,
	}
	for _, item := range m.items {
		switch item.Tier {
		case TierHot:
			pressure.HotBytes += item.SizeBytes
		case TierWarm:
			pressure.WarmBytes += item.SizeBytes
		}
	}
	pressure.HotOverBudget = pressure.HotBytes > pressure.HotBudgetBytes
	pressure.WarmOverBudget = pressure.WarmBytes > pressure.WarmBudgetBytes
	return pressure
}

// residentItemsLocked 返回驻留在指定阶段的条目指针切片。调用方须持锁。
func (m *Manager) residentItemsLocked(tier Tier) []*MemoryItem {
	out := make([]*MemoryItem, 0, len(m.items))
	for _, item := range m.items {
		if item.Tier == tier {
			out = append(out, item)
		}
	}
	return out
}

// coldness 冷度分数：越大越应被驱逐。
//
// 用"闲置时长（小时）"除以"访问次数+1"，兼顾新旧与冷热：
// 长期未访问且很少被访问的数据最该被降级。
func coldness(item *MemoryItem) float64 {
	idle := time.Since(item.LastAccessedAt).Hours()
	if idle < 0 {
		idle = 0
	}
	return idle / float64(item.AccessCount+1)
}

// sortedItems 按 ID 排序返回稳定顺序（避免报告条目顺序随机）。
func sortedItems(items map[string]*MemoryItem) []*MemoryItem {
	out := make([]*MemoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// transitionReason 生成人类可读的迁移原因。
//
// 时限一律取自当前策略对象，而非硬编码常量——否则更换策略后
// 审计日志里的原因文案会与实际判定依据不符。
func (m *Manager) transitionReason(item *MemoryItem, target Tier, now time.Time) string {
	if item.ForgetRequested {
		return "已请求机器遗忘"
	}
	idle := now.Sub(item.LastAccessedAt)
	switch target {
	case TierWarm:
		return fmt.Sprintf("闲置 %s 超过热层时限 %s", formatDuration(idle), formatDuration(m.policy.HotMaxAge))
	case TierCold:
		return fmt.Sprintf("闲置 %s 超过温层时限 %s 且访问次数 %d 未达保级阈值 %d",
			formatDuration(idle), formatDuration(m.policy.WarmMaxAge),
			item.AccessCount, m.policy.MinAccessCountToKeepWarm)
	case TierFrozen:
		return fmt.Sprintf("闲置 %s 超过冷层时限 %s", formatDuration(idle), formatDuration(m.policy.ColdMaxAge))
	case TierForgotten:
		return fmt.Sprintf("闲置 %s 超过冻结层保留期 %s", formatDuration(idle), formatDuration(m.policy.FrozenMaxAge))
	default:
		return "策略判定"
	}
}

// formatDuration 按量级选择合适的单位格式化时长。
//
// 不能用 Duration.Round(time.Minute)：亚分钟级的时长会被舍成 "0s"，
// 于是审计日志出现"闲置 0s 超过热层时限 400ms"这种自相矛盾的文案，
// 排查时会直接把人带偏（本函数正是被 demo 里这一行暴露出来的）。
// 也不用 Duration.String()，它会把 48 小时打印成冗长的 "48h0m0s"。
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%.1f分", d.Minutes())
	case d < 24*time.Hour:
		return fmt.Sprintf("%.1f小时", d.Hours())
	default:
		return fmt.Sprintf("%.1f天", d.Hours()/24)
	}
}
