package tiered

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// testPolicy 短时限策略，避免测试依赖真实时间流逝。
func testPolicy() TierPolicy {
	return TierPolicy{
		HotMaxAge:                1 * time.Hour,
		WarmMaxAge:               2 * time.Hour,
		ColdMaxAge:               3 * time.Hour,
		FrozenMaxAge:             4 * time.Hour,
		HotBudgetBytes:           1000,
		WarmBudgetBytes:          2000,
		MinAccessCountToKeepWarm: 3,
	}
}

// ============================================================================
// 阶段判定
// ============================================================================

// TestClassifyByAge 各阶段按闲置时长逐级降级。
func TestClassifyByAge(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)

	cases := []struct {
		name string
		item *MemoryItem
		want Tier
	}{
		{
			name: "热层未超时",
			item: &MemoryItem{ID: "a", Tier: TierHot, LastAccessedAt: now.Add(-30 * time.Minute)},
			want: TierHot,
		},
		{
			name: "热层超时",
			item: &MemoryItem{ID: "b", Tier: TierHot, LastAccessedAt: now.Add(-2 * time.Hour)},
			want: TierWarm,
		},
		{
			name: "温层未超时",
			item: &MemoryItem{ID: "c", Tier: TierWarm, LastAccessedAt: now.Add(-90 * time.Minute)},
			want: TierWarm,
		},
		{
			name: "温层超时且低频访问",
			item: &MemoryItem{ID: "d", Tier: TierWarm, LastAccessedAt: now.Add(-3 * time.Hour), AccessCount: 0},
			want: TierCold,
		},
		{
			name: "冷层超时",
			item: &MemoryItem{ID: "e", Tier: TierCold, LastAccessedAt: now.Add(-4 * time.Hour)},
			want: TierFrozen,
		},
		{
			name: "冻结层超时",
			item: &MemoryItem{ID: "f", Tier: TierFrozen, LastAccessedAt: now.Add(-5 * time.Hour)},
			want: TierForgotten,
		},
		{
			name: "冷层未超时",
			item: &MemoryItem{ID: "g", Tier: TierCold, LastAccessedAt: now.Add(-90 * time.Minute)},
			want: TierCold,
		},
	}

	for _, tc := range cases {
		if got := manager.Classify(tc.item, now); got != tc.want {
			t.Errorf("%s: Classify = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestClassifyKeepsFrequentlyAccessedWarm 高频访问的温层数据不因时长被降冷。
//
// 对应策划书"长程医学节点锁定 γ≈0"的思想在存储层的体现：
// 高价值记忆不应被时间盲目驱逐。
func TestClassifyKeepsFrequentlyAccessedWarm(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)

	often := &MemoryItem{ID: "often", Tier: TierWarm, LastAccessedAt: now.Add(-10 * time.Hour), AccessCount: 5}
	if got := manager.Classify(often, now); got != TierWarm {
		t.Fatalf("frequently accessed warm item demoted to %s, want 温层保级", got)
	}

	rarely := &MemoryItem{ID: "rarely", Tier: TierWarm, LastAccessedAt: now.Add(-10 * time.Hour), AccessCount: 1}
	if got := manager.Classify(rarely, now); got != TierCold {
		t.Fatalf("rarely accessed warm item kept at %s, want 冷层", got)
	}
}

// TestClassifyForgetRequestWins 机器遗忘请求优先级最高，
// 时间策略不得覆盖用户/合规的删除意图。
func TestClassifyForgetRequestWins(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)

	item := &MemoryItem{
		ID: "victim", Tier: TierHot,
		LastAccessedAt:  now,
		ForgetRequested: true,
	}
	if got := manager.Classify(item, now); got != TierForgotten {
		t.Fatalf("forget request ignored: got %s, want 遗忘", got)
	}
}

// TestClassifyForgottenIsTerminal 遗忘是终态，不得"复活"。
func TestClassifyForgottenIsTerminal(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)

	item := &MemoryItem{ID: "gone", Tier: TierForgotten, LastAccessedAt: now}
	if got := manager.Classify(item, now); got != TierForgotten {
		t.Fatalf("forgotten item resurrected to %s", got)
	}
}

// ============================================================================
// 登记与访问
// ============================================================================

// TestAddValidation 非法登记必须被拒绝。
func TestAddValidation(t *testing.T) {
	manager := NewManager(testPolicy(), nil, nil)

	if err := manager.Add(nil); err == nil {
		t.Fatal("expected error for nil item")
	}
	if err := manager.Add(&MemoryItem{}); err == nil {
		t.Fatal("expected error for empty ID")
	}
	if err := manager.Add(&MemoryItem{ID: "x", SizeBytes: -1}); err == nil {
		t.Fatal("expected error for negative size")
	}
	if err := manager.Add(&MemoryItem{ID: "dup"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := manager.Add(&MemoryItem{ID: "dup"}); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

// TestTouchReheatsItem 访问数据会回温到热层并累加计数。
func TestTouchReheatsItem(t *testing.T) {
	manager := NewManager(testPolicy(), nil, nil)
	if err := manager.Add(&MemoryItem{ID: "m1", Tier: TierCold}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := manager.Touch("m1"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	item, _ := manager.Item("m1")
	if item.Tier != TierHot {
		t.Fatalf("after Touch tier = %s, want 热层", item.Tier)
	}
	if item.AccessCount != 1 {
		t.Fatalf("AccessCount = %d, want 1", item.AccessCount)
	}

	if err := manager.Touch("unknown"); err == nil {
		t.Fatal("expected error for unknown item")
	}
}

// TestTouchDoesNotResurrectForgotten 访问不得让已遗忘数据复活。
func TestTouchDoesNotResurrectForgotten(t *testing.T) {
	manager := NewManager(testPolicy(), nil, nil)
	if err := manager.Add(&MemoryItem{ID: "gone", Tier: TierForgotten}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := manager.Touch("gone"); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	item, _ := manager.Item("gone")
	if item.Tier != TierForgotten {
		t.Fatalf("forgotten item resurrected to %s", item.Tier)
	}
}

// ============================================================================
// 评估与执行
// ============================================================================

// TestEvaluateReportsDueTransitions 评估产出应发生的迁移，且不改变状态。
func TestEvaluateReportsDueTransitions(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)

	if err := manager.Add(&MemoryItem{ID: "stale", Tier: TierHot, LastAccessedAt: now.Add(-5 * time.Hour)}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := manager.Add(&MemoryItem{ID: "fresh", Tier: TierHot, LastAccessedAt: now}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	report := manager.Evaluate()
	if len(report.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d (%+v)", len(report.Transitions), report.Transitions)
	}
	if report.Transitions[0].ItemID != "stale" {
		t.Fatalf("wrong item transitioned: %s", report.Transitions[0].ItemID)
	}
	if report.Transitions[0].To != TierWarm {
		t.Fatalf("expected demotion to 温层, got %s", report.Transitions[0].To)
	}
	if report.Transitions[0].Reason == "" {
		t.Fatal("transition reason must not be empty")
	}

	// 评估是无副作用的：状态不应变化。
	item, _ := manager.Item("stale")
	if item.Tier != TierHot {
		t.Fatalf("Evaluate mutated state: tier = %s", item.Tier)
	}

	// 分层计数应反映当前状态。
	if report.ByTier[TierHot] != 2 {
		t.Fatalf("ByTier[热层] = %d, want 2", report.ByTier[TierHot])
	}
}

// TestApplyExecutesTransitions 执行迁移才真正改变状态。
func TestApplyExecutesTransitions(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)
	if err := manager.Add(&MemoryItem{ID: "stale", Tier: TierHot, LastAccessedAt: now.Add(-5 * time.Hour)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results := manager.Apply(manager.Evaluate())
	if len(results) != 1 || !results[0].Applied {
		t.Fatalf("transition not applied: %+v", results)
	}

	item, _ := manager.Item("stale")
	if item.Tier != TierWarm {
		t.Fatalf("after Apply tier = %s, want 温层", item.Tier)
	}
}

// TestApplyDetectsStaleTransition 评估后若状态又被访问改变，
// 执行时必须拒绝而非盲目覆盖——避免把刚被访问的数据降级。
func TestApplyDetectsStaleTransition(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)
	if err := manager.Add(&MemoryItem{ID: "m", Tier: TierHot, LastAccessedAt: now.Add(-5 * time.Hour)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	report := manager.Evaluate()
	// 评估之后该条被访问，回温到热层。
	if err := manager.Touch("m"); err != nil {
		t.Fatalf("Touch: %v", err)
	}

	results := manager.Apply(report)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Applied {
		t.Fatal("stale transition should be rejected, not applied")
	}
	if results[0].Error == "" {
		t.Fatal("rejected transition must carry an error reason")
	}

	item, _ := manager.Item("m")
	if item.Tier != TierHot {
		t.Fatalf("item should stay 热层 after Touch, got %s", item.Tier)
	}
}

// TestApplyNilReportIsNoop nil 报告不得 panic。
func TestApplyNilReportIsNoop(t *testing.T) {
	manager := NewManager(testPolicy(), nil, nil)
	if results := manager.Apply(nil); results != nil {
		t.Fatalf("Apply(nil) = %+v, want nil", results)
	}
}

// ============================================================================
// 归档副作用
// ============================================================================

// memArchiver 内存实现的归档后端，用于验证归档/删除副作用。
type memArchiver struct {
	mu       sync.Mutex
	archived map[string][]byte
	failOn   string // 指定 ID 时强制失败
}

func newMemArchiver() *memArchiver {
	return &memArchiver{archived: make(map[string][]byte)}
}

func (a *memArchiver) Archive(id string, payload []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == a.failOn {
		return fmt.Errorf("simulated archive failure")
	}
	a.archived[id] = append([]byte(nil), payload...)
	return nil
}

func (a *memArchiver) Restore(id string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	payload, ok := a.archived[id]
	if !ok {
		return nil, fmt.Errorf("not archived: %s", id)
	}
	return payload, nil
}

func (a *memArchiver) Delete(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.archived, id)
	return nil
}

// memProvider 提供待归档的载荷。
type memProvider struct{ payload []byte }

func (p *memProvider) Payload(id string) ([]byte, error) {
	return append([]byte(nil), p.payload...), nil
}

// TestArchivesOnColdTransition 降冷时数据必须真正落盘：
// 只改状态而不归档会导致数据丢失，是生命周期调度最严重的缺陷。
func TestArchivesOnColdTransition(t *testing.T) {
	now := time.Now()
	archiver := newMemArchiver()
	provider := &memProvider{payload: []byte("memory-payload")}
	manager := NewManager(testPolicy(), archiver, provider)

	if err := manager.Add(&MemoryItem{ID: "cold", Tier: TierWarm, LastAccessedAt: now.Add(-5 * time.Hour), AccessCount: 0}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results := manager.Apply(manager.Evaluate())
	if len(results) != 1 || !results[0].Applied {
		t.Fatalf("cold transition not applied: %+v", results)
	}

	payload, err := archiver.Restore("cold")
	if err != nil {
		t.Fatalf("data was not archived: %v", err)
	}
	if string(payload) != "memory-payload" {
		t.Fatalf("archived payload = %q", payload)
	}

	item, _ := manager.Item("cold")
	if item.Tier != TierCold {
		t.Fatalf("tier = %s, want 冷层", item.Tier)
	}
}

// TestDeletesArchiveOnForget 进入遗忘态时必须删除归档数据，
// 否则"遗忘"只是逻辑标记，数据仍在磁盘上——这是合规硬伤。
func TestDeletesArchiveOnForget(t *testing.T) {
	archiver := newMemArchiver()
	manager := NewManager(testPolicy(), archiver, &memProvider{payload: []byte("x")})

	archiver.Archive("victim", []byte("sensitive"))
	if err := manager.Add(&MemoryItem{ID: "victim", Tier: TierCold, ForgetRequested: true}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results := manager.Apply(manager.Evaluate())
	if len(results) != 1 || !results[0].Applied {
		t.Fatalf("forget transition not applied: %+v", results)
	}
	if _, err := archiver.Restore("victim"); err == nil {
		t.Fatal("archived data survived forgetting")
	}

	item, _ := manager.Item("victim")
	if item.Tier != TierForgotten {
		t.Fatalf("tier = %s, want 遗忘", item.Tier)
	}
}

// TestArchiveFailureIsReportedNotPanic 归档失败必须被如实报告，
// 且不得把状态改成"已归档"——否则数据就真的丢了。
func TestArchiveFailureIsReportedNotPanic(t *testing.T) {
	now := time.Now()
	archiver := newMemArchiver()
	archiver.failOn = "broken"
	manager := NewManager(testPolicy(), archiver, &memProvider{payload: []byte("x")})

	if err := manager.Add(&MemoryItem{ID: "broken", Tier: TierWarm, LastAccessedAt: now.Add(-5 * time.Hour)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	results := manager.Apply(manager.Evaluate())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Applied {
		t.Fatal("transition should not be applied when archiving fails")
	}
	if results[0].Error == "" {
		t.Fatal("failure must be reported")
	}

	// 状态必须保持不变（数据还在热/温层，没有丢）。
	item, _ := manager.Item("broken")
	if item.Tier != TierWarm {
		t.Fatalf("tier changed to %s despite archive failure", item.Tier)
	}
}

// TestManagerWorksWithoutArchiver 无归档后端时仍可评估（纯判定场景）。
func TestManagerWorksWithoutArchiver(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)
	if err := manager.Add(&MemoryItem{ID: "m", Tier: TierWarm, LastAccessedAt: now.Add(-5 * time.Hour)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 无 archiver 时降冷不应失败——只是不落盘。
	results := manager.Apply(manager.Evaluate())
	if len(results) != 1 || !results[0].Applied {
		t.Fatalf("transition should still apply without archiver: %+v", results)
	}
}

// ============================================================================
// 内存预算
// ============================================================================

// TestEnforceBudgetDemotesColdestFirst 超预算时按冷度驱逐最冷的条目。
func TestEnforceBudgetDemotesColdestFirst(t *testing.T) {
	now := time.Now()
	policy := testPolicy()
	policy.HotBudgetBytes = 100
	manager := NewManager(policy, nil, nil)

	// 三条 50 字节的热数据，冷度依次 3 / 2 / 1。
	manager.Add(&MemoryItem{ID: "b-coldest", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-3 * time.Hour)})
	manager.Add(&MemoryItem{ID: "c-mid", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-2 * time.Hour)})
	manager.Add(&MemoryItem{ID: "a-warmest", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-1 * time.Hour)})

	demoted := manager.EnforceBudget()
	if len(demoted) != 1 {
		t.Fatalf("expected 1 demotion (150 -> 100), got %d: %+v", len(demoted), demoted)
	}
	if demoted[0].ItemID != "b-coldest" {
		t.Fatalf("expected coldest item demoted, got %s", demoted[0].ItemID)
	}
	if demoted[0].To != TierWarm {
		t.Fatalf("expected demotion to 温层, got %s", demoted[0].To)
	}

	// 回到预算内。
	report := manager.Evaluate()
	if report.BudgetPressure.HotBytes > policy.HotBudgetBytes {
		t.Fatalf("hot tier still over budget: %d > %d", report.BudgetPressure.HotBytes, policy.HotBudgetBytes)
	}
}

// TestEnforceBudgetKeepsFrequentlyAccessed 冷度考虑访问频率：
// 高频访问的条目应比"更久但更常用"的条目更晚被驱逐。
func TestEnforceBudgetKeepsFrequentlyAccessed(t *testing.T) {
	now := time.Now()
	policy := testPolicy()
	policy.HotBudgetBytes = 100
	manager := NewManager(policy, nil, nil)

	// idle=4h, count=0 → coldness 4；idle=1h, count=10 → coldness 1/11≈0.09
	manager.Add(&MemoryItem{ID: "old-rare", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-4 * time.Hour), AccessCount: 0})
	manager.Add(&MemoryItem{ID: "new-popular", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-1 * time.Hour), AccessCount: 10})
	manager.Add(&MemoryItem{ID: "mid", Tier: TierHot, SizeBytes: 50, LastAccessedAt: now.Add(-2 * time.Hour), AccessCount: 5})

	demoted := manager.EnforceBudget()
	for _, d := range demoted {
		if d.ItemID == "new-popular" {
			t.Fatalf("frequently accessed item was evicted first: %+v", demoted)
		}
	}
	if len(demoted) == 0 {
		t.Fatal("expected at least one demotion when over budget")
	}
	if demoted[0].ItemID != "old-rare" {
		t.Fatalf("expected coldest (old-rare) evicted first, got %s", demoted[0].ItemID)
	}
}

// TestEnforceBudgetIsNoOpWhenWithinBudget 预算内时不应产生任何迁移。
func TestEnforceBudgetIsNoOpWhenWithinBudget(t *testing.T) {
	now := time.Now()
	manager := NewManager(testPolicy(), nil, nil)
	manager.Add(&MemoryItem{ID: "small", Tier: TierHot, SizeBytes: 10, LastAccessedAt: now})

	if demoted := manager.EnforceBudget(); len(demoted) != 0 {
		t.Fatalf("unexpected demotions within budget: %+v", demoted)
	}
}

// TestEnforceBudgetCascadesHotToWarmToCold 热层超预算会先降到温层，
// 若温层随之超限再降到冷层——预算约束是级联生效的。
func TestEnforceBudgetCascadesHotToWarmToCold(t *testing.T) {
	now := time.Now()
	policy := testPolicy()
	policy.HotBudgetBytes = 100
	policy.WarmBudgetBytes = 100
	manager := NewManager(policy, nil, nil)

	for i := 0; i < 4; i++ {
		manager.Add(&MemoryItem{
			ID: fmt.Sprintf("item%d", i), Tier: TierHot, SizeBytes: 50,
			LastAccessedAt: now.Add(-time.Duration(i+1) * time.Hour),
		})
	}

	demoted := manager.EnforceBudget()
	if len(demoted) < 2 {
		t.Fatalf("expected cascading demotions, got %d: %+v", len(demoted), demoted)
	}

	// 最终热层与温层都应在预算内。
	hot, warm := int64(0), int64(0)
	for _, id := range []string{"item0", "item1", "item2", "item3"} {
		item, _ := manager.Item(id)
		switch item.Tier {
		case TierHot:
			hot += item.SizeBytes
		case TierWarm:
			warm += item.SizeBytes
		}
	}
	if hot > policy.HotBudgetBytes {
		t.Fatalf("hot tier over budget: %d > %d", hot, policy.HotBudgetBytes)
	}
	if warm > policy.WarmBudgetBytes {
		t.Fatalf("warm tier over budget: %d > %d", warm, policy.WarmBudgetBytes)
	}
}

// ============================================================================
// 策略与元数据
// ============================================================================

// TestZeroPolicyFallsBackToDefaults 零值时限会被兜底修正，
// 否则"所有数据立刻过期"这类配置事故会静默发生。
func TestZeroPolicyFallsBackToDefaults(t *testing.T) {
	manager := NewManager(TierPolicy{}, nil, nil)
	defaults := DefaultTierPolicy()

	item := &MemoryItem{ID: "x", Tier: TierHot, LastAccessedAt: time.Now()}
	// 零值策略下不应立刻降冷：热层时限已被兜底为 48h。
	if got := manager.Classify(item, time.Now()); got != TierHot {
		t.Fatalf("zero policy caused immediate demotion to %s", got)
	}

	if err := manager.Add(&MemoryItem{ID: "y", SizeBytes: -5}); err == nil {
		t.Fatal("negative size must still be rejected")
	}
	_ = defaults
}

// TestDefaultPolicyMatchesPlan 默认策略必须匹配策划书：热 48h、温 15d、冷 90d。
func TestDefaultPolicyMatchesPlan(t *testing.T) {
	policy := DefaultTierPolicy()
	if policy.HotMaxAge != 48*time.Hour {
		t.Errorf("HotMaxAge = %v, want 48h", policy.HotMaxAge)
	}
	if policy.WarmMaxAge != 15*24*time.Hour {
		t.Errorf("WarmMaxAge = %v, want 15d", policy.WarmMaxAge)
	}
	if policy.ColdMaxAge != 90*24*time.Hour {
		t.Errorf("ColdMaxAge = %v, want 90d", policy.ColdMaxAge)
	}
}

// TestTierStringAndResidency 阶段名称与"是否驻留内存"的语义。
func TestTierStringAndResidency(t *testing.T) {
	if TierHot.String() != "热层" {
		t.Errorf("TierHot.String() = %q", TierHot.String())
	}
	if !TierHot.IsResident() || !TierWarm.IsResident() {
		t.Error("hot and warm tiers must be resident")
	}
	if TierCold.IsResident() || TierFrozen.IsResident() || TierForgotten.IsResident() {
		t.Error("cold/frozen/forgotten must not be resident")
	}
	if Tier(99).String() == "" {
		t.Error("unknown tier should still render a string")
	}
}
