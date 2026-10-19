package services

import (
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/contextkeeper/service/internal/config"
	"github.com/contextkeeper/service/internal/memory/tiered"
	"github.com/contextkeeper/service/internal/storage/asdffile"
)

// ============================================================================
// 冷热分层生命周期服务
//
// 本文件把两个此前各自孤立的能力**装配**成一个可运行的服务：
//   - internal/memory/tiered    冷热五阶段判定与调度
//   - internal/storage/asdffile 冷数据归档（ASDF + LZ4 + mmap）
//
// 为什么不直接改 ContextService：那是 258KB 的生产核心文件，且记忆写入
// 路径分散在多处。为把接入风险控制在可审计范围内，这里做成**独立服务 +
// 回调注入**：不改动任何既有记忆读写逻辑，上层只需在合适的时机调用
// Register / Touch，并显式 Start。
//
// 默认关闭（TIERED_LIFECYCLE_ENABLED=false）：分层调度会改变记忆的存储
// 位置，属于行为变更，必须由部署方显式开启。
// ============================================================================

// PayloadFunc 按记忆 ID 取回原始字节，用于冷层归档。
//
// 用函数类型而非接口，是为了让上层能用闭包直接注入取数逻辑，
// 不必为了实现一个接口而新增类型。
type PayloadFunc func(id string) ([]byte, error)

// payloadAdapter 把 PayloadFunc 适配成 tiered.PayloadProvider。
type payloadAdapter struct {
	fn PayloadFunc
}

// Payload 实现 tiered.PayloadProvider。
func (a *payloadAdapter) Payload(id string) ([]byte, error) {
	return a.fn(id)
}

// CycleStats 一个调度周期的统计，供日志与审计使用。
type CycleStats struct {
	EvaluatedAt   time.Time `json:"evaluated_at"`
	Evaluated     int       `json:"evaluated"`      // 判定出的迁移数
	BudgetDemoted int       `json:"budget_demoted"` // 因超预算被降级的条数
	Applied       int       `json:"applied"`        // 成功执行数
	Failed        int       `json:"failed"`         // 失败数
	Archived      int       `json:"archived"`       // 本周期落盘归档数
	Deleted       int       `json:"deleted"`        // 本周期删除归档数
	FirstError    string    `json:"first_error,omitempty"`
}

// TieredLifecycleService 冷热分层生命周期服务。并发安全。
type TieredLifecycleService struct {
	manager    *tiered.Manager
	archiver   *asdffile.ArchiveStore
	provider   tiered.PayloadProvider
	interval   time.Duration
	archiveDir string

	mu           sync.Mutex
	running      bool
	stopChan     chan struct{}
	ticker       *time.Ticker
	lastReport   *tiered.Report
	lastStats    *CycleStats
	totalApplied int64
	totalFailed  int64
}

// NewTieredLifecycleService 构造冷热分层服务。
//
// payloadFn 用于冷层归档时取回记忆内容；传 nil 时冷层迁移只改元数据、
// 不落盘（适用于只想观察判定结果的场景）。
func NewTieredLifecycleService(cfg *config.Config, payloadFn PayloadFunc) (*TieredLifecycleService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("tiered lifecycle: config 为 nil")
	}

	policy := tiered.TierPolicy{
		HotMaxAge:                cfg.TierHotMaxAge,
		WarmMaxAge:               cfg.TierWarmMaxAge,
		ColdMaxAge:               cfg.TierColdMaxAge,
		FrozenMaxAge:             cfg.TierFrozenMaxAge,
		HotBudgetBytes:           int64(cfg.TierHotBudgetMB) << 20,
		WarmBudgetBytes:          int64(cfg.TierWarmBudgetMB) << 20,
		MinAccessCountToKeepWarm: cfg.TierMinAccessKeepWarm,
	}

	archiveDir := filepath.Join(cfg.StoragePath, "cold-archive")
	archiver, err := asdffile.NewArchiveStore(archiveDir)
	if err != nil {
		return nil, fmt.Errorf("tiered lifecycle: 创建归档存储失败: %w", err)
	}

	var provider tiered.PayloadProvider
	if payloadFn != nil {
		provider = &payloadAdapter{fn: payloadFn}
	}

	interval := cfg.TierEvalInterval
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	return &TieredLifecycleService{
		manager:    tiered.NewManager(policy, archiver, provider),
		archiver:   archiver,
		provider:   provider,
		interval:   interval,
		archiveDir: archiveDir,
	}, nil
}

// Start 启动周期调度。重复调用是安全的（幂等）。
func (s *TieredLifecycleService) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.ticker = time.NewTicker(s.interval)
	ticker := s.ticker
	stop := s.stopChan
	s.mu.Unlock()

	go func() {
		for {
			select {
			case <-ticker.C:
				// 单个周期的失败不应终止整个调度循环，只记录日志。
				if _, err := s.RunOnce(); err != nil {
					log.Printf("[tiered] 生命周期调度失败: %v", err)
				}
			case <-stop:
				ticker.Stop()
				return
			}
		}
	}()
	log.Printf("[tiered] 冷热分层调度已启动: 周期=%v 归档目录=%s", s.interval, s.archiveDir)
}

// Stop 停止周期调度。可重复调用（幂等）。
func (s *TieredLifecycleService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopChan)
	s.ticker.Stop()
	log.Printf("[tiered] 冷热分层调度已停止")
}

// IsRunning 报告调度是否在运行。
func (s *TieredLifecycleService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Register 登记（或更新）一条受管理的记忆。
//
// sizeBytes 用内容的字节长度估算即可；它只参与内存预算计算，
// 不要求精确。
func (s *TieredLifecycleService) Register(id string, sizeBytes int64) error {
	if id == "" {
		return fmt.Errorf("tiered lifecycle: 空的记忆 ID")
	}
	now := time.Now()
	item := &tiered.MemoryItem{
		ID:             id,
		SizeBytes:      sizeBytes,
		CreatedAt:      now,
		LastAccessedAt: now,
		AccessCount:    1,
	}
	// 已存在时 Add 会返回错误，此时退化为 Touch（刷新访问时间）。
	if err := s.manager.Add(item); err != nil {
		return s.manager.Touch(id)
	}
	return nil
}

// Touch 刷新一条记忆的访问时间（检索命中时调用）。
//
// 条目未登记时返回错误——这是有意的：静默忽略会让调用方误以为
// 访问被计入，实际却影响了分层判定。
func (s *TieredLifecycleService) Touch(id string) error {
	return s.manager.Touch(id)
}

// RequestForget 标记一条记忆进入机器遗忘流程。
func (s *TieredLifecycleService) RequestForget(id string) error {
	return s.RequestForgetFor(id, true)
}

// RequestForgetFor 设置指定条目的遗忘标记。
//
// 单独暴露带参数的版本，是为了让"取消遗忘标记"也成为显式操作，
// 而不是只能单向设置。
func (s *TieredLifecycleService) RequestForgetFor(id string, forget bool) error {
	// 必须走 Manager 的修改方法：Item 返回的是值快照，
	// 在其上改 ForgetRequested 改的是副本，不会生效。
	return s.manager.SetForgetRequested(id, forget)
}

// RunOnce 执行一次完整的调度周期：判定 → 执行 → 预算驱逐 → 补归档。
//
// 这是本服务的核心，也是最容易出错的地方，流程顺序有讲究：
//
//  1. Evaluate 只做判定，不产生副作用；
//  2. Apply 执行迁移（冷层归档、遗忘删除），单条失败不中断；
//  3. EnforceBudget 处理超预算的强制降级。
//
// **关键陷阱**：EnforceBudget 会**自行**把 item.Tier 改成目标层并返回
// 迁移记录——它已经执行了。若把这些记录再交给 Apply，Apply 的"状态复核"
// （Classify 结果必须等于目标层）可能判定为"状态已变化"而跳过，
// 于是出现**Tier 已是 Cold、但归档从未执行**的数据风险。
//
// 因此这里对 EnforceBudget 产生的 Cold 迁移**显式补做归档**，
// 而不是把它塞回 Apply。归档是幂等操作（覆盖写），重复执行无害。
func (s *TieredLifecycleService) RunOnce() (*CycleStats, error) {
	stats := &CycleStats{EvaluatedAt: time.Now()}

	report := s.manager.Evaluate()
	stats.Evaluated = len(report.Transitions)

	results := s.manager.Apply(report)
	for _, result := range results {
		if result.Applied {
			stats.Applied++
			if result.Transition.To == tiered.TierForgotten {
				stats.Deleted++
			}
			if result.Transition.To == tiered.TierCold {
				stats.Archived++
			}
		} else if result.Error != "" {
			stats.Failed++
			if stats.FirstError == "" {
				stats.FirstError = result.Error
			}
		}
	}

	// 预算驱逐：EnforceBudget 已自行改过 Tier，这里只补落盘。
	demoted := s.manager.EnforceBudget()
	stats.BudgetDemoted = len(demoted)
	for _, transition := range demoted {
		if transition.To != tiered.TierCold {
			continue
		}
		payload, err := s.payloadOf(transition.ItemID)
		if err != nil {
			stats.Failed++
			if stats.FirstError == "" {
				stats.FirstError = fmt.Sprintf("超预算降级后归档失败: %v", err)
			}
			continue
		}
		if err := s.archiver.Archive(transition.ItemID, payload); err != nil {
			stats.Failed++
			if stats.FirstError == "" {
				stats.FirstError = fmt.Sprintf("超预算降级后归档失败: %v", err)
			}
			continue
		}
		stats.Archived++
	}

	s.mu.Lock()
	s.lastReport = report
	s.lastStats = stats
	s.totalApplied += int64(stats.Applied)
	s.totalFailed += int64(stats.Failed)
	s.mu.Unlock()

	if stats.Failed > 0 {
		return stats, fmt.Errorf("%d 条迁移失败，首条原因: %s", stats.Failed, stats.FirstError)
	}
	return stats, nil
}

// payloadOf 取回条目内容用于归档。
//
// 未能取到时返回错误，由调用方决定是否中止——静默跳过会造成
// "标记为冷层但内容没落盘"的不一致状态。
func (s *TieredLifecycleService) payloadOf(id string) ([]byte, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("未配置 PayloadProvider，无法归档 %q", id)
	}
	return s.provider.Payload(id)
}

// LastReport 返回最近一次评估报告（首次调度前为 nil）。
func (s *TieredLifecycleService) LastReport() *tiered.Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastReport
}

// LastStats 返回最近一次调度统计。
func (s *TieredLifecycleService) LastStats() *CycleStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStats
}

// Totals 返回累计的执行/失败计数。
func (s *TieredLifecycleService) Totals() (applied, failed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalApplied, s.totalFailed
}

// TierOf 查询一条记忆当前所处的阶段。
func (s *TieredLifecycleService) TierOf(id string) (tiered.Tier, bool) {
	item, ok := s.manager.Item(id)
	if !ok {
		return 0, false
	}
	return item.Tier, true
}

// ArchiveDir 返回归档目录路径（便于运维与排障）。
func (s *TieredLifecycleService) ArchiveDir() string {
	return s.archiveDir
}

// RestoreArchived 读回已归档的冷数据。
func (s *TieredLifecycleService) RestoreArchived(id string) ([]byte, error) {
	return s.archiver.Restore(id)
}
