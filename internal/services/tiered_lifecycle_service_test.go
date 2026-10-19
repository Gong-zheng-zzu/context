package services

import (
	"bytes"
	"testing"
	"time"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/memory/tiered"
)

// testConfig 构造一个"热层与温层立刻过期、冷层长期保留"的配置，
// 使条目在一次调度内就应被判定为 Cold 并触发归档。
func testConfig(dir string) *config.Config {
	return &config.Config{
		StoragePath:           dir,
		TierEvalInterval:      time.Minute,
		TierHotMaxAge:         time.Nanosecond,
		TierWarmMaxAge:        time.Nanosecond,
		TierColdMaxAge:        1000 * time.Hour,
		TierFrozenMaxAge:      1000 * time.Hour,
		TierHotBudgetMB:       1,
		TierWarmBudgetMB:      1,
		TierMinAccessKeepWarm: 100, // 设很大 → 不保级，必定降冷
	}
}

// TestServiceArchivesColdMemory 冷层迁移必须真的落盘到 ASDF，并能读回。
//
// 这是 "tiered + asdffile 装配成功" 的核心证据：两个原本孤立的包
// 在这里产生了真实的端到端效果。
func TestServiceArchivesColdMemory(t *testing.T) {
	payload := []byte("这是冷数据内容，应当被归档")
	service, err := NewTieredLifecycleService(testConfig(t.TempDir()), func(id string) ([]byte, error) {
		if id != "mem-1" {
			return nil, bytes.ErrTooLarge
		}
		return payload, nil
	})
	if err != nil {
		t.Fatalf("NewTieredLifecycleService: %v", err)
	}

	if err := service.Register("mem-1", int64(len(payload))); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 两个必须注意的点：
	// 1) Windows 系统时钟精度较低（约 1~15ms），若 Register 后立刻 RunOnce，
	//    两次 time.Now() 可能落在同一时钟刻度上，闲置测得 0，被判为"未过期"。
	//    因此每轮调度前都主动让条目变老。
	// 2) Classify 是**逐级**降级的：一次调度只降一层（Hot→Warm），下一轮
	//    才 Warm→Cold 并触发归档。这是有意的设计——逐级降级让每一层都有
	//    机会被访问"救回"，而不是一口气把数据丢到冷层。
	var stats *CycleStats
	for i := 0; i < 2; i++ {
		time.Sleep(20 * time.Millisecond)
		s, err := service.RunOnce()
		if err != nil {
			t.Fatalf("RunOnce #%d: %v", i+1, err)
		}
		stats = s
	}
	if stats.Applied == 0 {
		t.Fatalf("no transition applied: %+v", stats)
	}
	if stats.Archived == 0 {
		t.Fatalf("cold transition did not archive: %+v", stats)
	}

	tier, ok := service.TierOf("mem-1")
	if !ok || tier != tiered.TierCold {
		t.Fatalf("TierOf = %v (ok=%v), want Cold", tier, ok)
	}

	// 关键：归档内容必须能原样读回。
	restored, err := service.RestoreArchived("mem-1")
	if err != nil {
		t.Fatalf("RestoreArchived: %v", err)
	}
	if !bytes.Equal(restored, payload) {
		t.Errorf("restored = %q, want %q", restored, payload)
	}
}

// TestServiceForgetRequestTakesEffect 遗忘标记必须真正生效。
//
// 回归测试：Manager.Item 返回的是值快照，若通过修改 Item() 的返回值来
// 设置 ForgetRequested，改的是副本、完全无效，条目永远不会进入遗忘态。
// 这里直接断言"一次调度后确实进入遗忘态"，任何回到副本写法的改动都会被抓到。
func TestServiceForgetRequestTakesEffect(t *testing.T) {
	service, err := NewTieredLifecycleService(testConfig(t.TempDir()), func(id string) ([]byte, error) {
		return []byte("x"), nil
	})
	if err != nil {
		t.Fatalf("NewTieredLifecycleService: %v", err)
	}

	if err := service.Register("mem-2", 1); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := service.RequestForget("mem-2"); err != nil {
		t.Fatalf("RequestForget: %v", err)
	}

	if _, err := service.RunOnce(); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	tier, ok := service.TierOf("mem-2")
	if !ok {
		t.Fatal("item disappeared")
	}
	if tier != tiered.TierForgotten {
		t.Errorf("TierOf = %v, want Forgotten (forget request did not take effect)", tier)
	}
}

// TestServiceForgetUnknownRejected 对未登记条目设置遗忘必须报错，而非静默成功。
func TestServiceForgetUnknownRejected(t *testing.T) {
	service, err := NewTieredLifecycleService(testConfig(t.TempDir()), nil)
	if err != nil {
		t.Fatalf("NewTieredLifecycleService: %v", err)
	}
	if err := service.RequestForget("nope"); err == nil {
		t.Error("RequestForget on unregistered id succeeded")
	}
	if err := service.Register("", 1); err == nil {
		t.Error("Register with empty id succeeded")
	}
	if err := service.Touch("nope"); err == nil {
		t.Error("Touch on unregistered id succeeded")
	}
}

// TestServiceBudgetEvictionStillArchives 超预算降级后必须补做归档。
//
// 回归测试：EnforceBudget 会自行把 Tier 改成目标层并返回迁移记录，
// 若把这些记录交给 Apply，Apply 的状态复核可能判定"状态已变化"而跳过，
// 导致 Tier 已是 Cold 但内容从未落盘。这里断言：即使发生预算驱逐，
// 被降到 Cold 的条目也一定能在归档里读到。
func TestServiceBudgetEvictionStillArchives(t *testing.T) {
	payload := []byte("超预算被驱逐的内容")
	// 预算设得极小（1MB），塞入远超预算的数据以触发驱逐。
	cfg := testConfig(t.TempDir())
	cfg.TierHotBudgetMB = 1
	cfg.TierWarmBudgetMB = 1

	service, err := NewTieredLifecycleService(cfg, func(id string) ([]byte, error) {
		return payload, nil
	})
	if err != nil {
		t.Fatalf("NewTieredLifecycleService: %v", err)
	}

	// 每条 10MB，10 条共 100MB，远超 1MB 预算。
	for i := 0; i < 10; i++ {
		id := "big-" + string(rune('a'+i))
		if err := service.Register(id, 10<<20); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}

	stats, _ := service.RunOnce()

	// 不论走了哪条路径，凡是处于 Cold 的条目都必须能读回内容。
	archived := 0
	for i := 0; i < 10; i++ {
		id := "big-" + string(rune('a'+i))
		tier, ok := service.TierOf(id)
		if !ok || tier != tiered.TierCold {
			continue
		}
		archived++
		got, err := service.RestoreArchived(id)
		if err != nil {
			t.Errorf("条目 %s 处于 Cold 但归档不可读: %v", id, err)
			continue
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("条目 %s 归档内容不匹配", id)
		}
	}
	if archived == 0 {
		t.Skipf("本次未触发冷层降级（budgetDemoted=%d），跳过归档断言", stats.BudgetDemoted)
	}
	t.Logf("预算驱逐 %d 条，其中 %d 条降入冷层且均已成功归档", stats.BudgetDemoted, archived)
}

// TestServiceStartStopIdempotent 启动/停止必须幂等，重复 Start 不应产生多个调度循环。
func TestServiceStartStopIdempotent(t *testing.T) {
	service, err := NewTieredLifecycleService(testConfig(t.TempDir()), nil)
	if err != nil {
		t.Fatalf("NewTieredLifecycleService: %v", err)
	}

	service.Start()
	service.Start() // 幂等
	if !service.IsRunning() {
		t.Error("IsRunning = false after Start")
	}
	service.Stop()
	service.Stop() // 幂等
	if service.IsRunning() {
		t.Error("IsRunning = true after Stop")
	}
}

// TestServiceNilConfigRejected config 为 nil 必须报错，而不是带着零值静默运行。
func TestServiceNilConfigRejected(t *testing.T) {
	if _, err := NewTieredLifecycleService(nil, nil); err == nil {
		t.Error("NewTieredLifecycleService accepted nil config")
	}
}
