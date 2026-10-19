package memory_subgraph

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// kindOrder 定义摘要中实体类别的固定展示顺序。
// 固定顺序（而非按 map 迭代）是摘要可复现的前提。
var kindOrder = []EntityKind{
	EntityElder,
	EntityDisease,
	EntitySymptom,
	EntityDrug,
	EntityLesion,
	EntityCareAction,
	EntityDepartment,
	EntityOther,
}

// kindLabel 实体类别的中文名，用于生成人类可读的社区摘要。
var kindLabel = map[EntityKind]string{
	EntityElder:      "老人",
	EntityDisease:    "疾病",
	EntitySymptom:    "症状",
	EntityDrug:       "药物",
	EntityLesion:     "病变创面",
	EntityCareAction: "护理行为",
	EntityDepartment: "科室",
	EntityOther:      "其他",
}

// maxSummaryEntities 摘要中列举的核心实体上限，避免长摘要淹没关键信息。
const maxSummaryEntities = 6

// summarizeCommunity 生成确定性的社区摘要（CommunitySummaries）。
//
// 该摘要是纯函数的产物：给定同一组实体，任何时刻、任何机器产出同一段文本。
// 它刻意不含 LLM 生成内容——LLM 增强走 EnrichSummaries，属于可选的第二遍处理，
// 其结果不会污染确定性基线，也便于在审计中区分"事实摘要"与"生成摘要"。
func summarizeCommunity(community *Community, entities []*SemanticEntity) string {
	if community == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "阶段性健康总览（%s）：共 %d 个语义实体", community.ID, community.Size)

	// 类别分布，按固定顺序枚举，跳过计数为 0 的类别。
	parts := make([]string, 0, len(kindOrder))
	for _, kind := range kindOrder {
		if count := community.KindHistogram[kind]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", kindLabel[kind], count))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, "（%s）", strings.Join(parts, "、"))
	}
	b.WriteString("。")

	// 核心实体：按类别固定顺序 + 名称字典序，最多列举 maxSummaryEntities 个。
	ordered := append([]*SemanticEntity(nil), entities...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Kind != ordered[j].Kind {
			return kindRank(ordered[i].Kind) < kindRank(ordered[j].Kind)
		}
		return ordered[i].Name < ordered[j].Name
	})

	names := make([]string, 0, maxSummaryEntities)
	for _, entity := range ordered {
		if len(names) >= maxSummaryEntities {
			break
		}
		if entity.Name != "" {
			names = append(names, entity.Name)
		}
	}
	if len(names) > 0 {
		fmt.Fprintf(&b, "核心实体：%s", strings.Join(names, "、"))
		if len(ordered) > len(names) {
			fmt.Fprintf(&b, " 等 %d 项", len(ordered))
		}
		b.WriteString("。")
	}

	fmt.Fprintf(&b, "溯源证据 %d 条原始记录。", len(community.Episodes))
	return b.String()
}

// kindRank 返回类别在固定顺序中的位次；未知类别排在最后。
func kindRank(kind EntityKind) int {
	for i, k := range kindOrder {
		if k == kind {
			return i
		}
	}
	return len(kindOrder)
}

// ============================ 可选：LLM 摘要增强 ============================

// Summarizer 是社区摘要的可选增强器（通常由本地 LLM 实现）。
//
// 实现方必须把 members 作为**唯一**事实来源：不得引入外部知识或推测，
// 否则会破坏"摘要可回溯到原始事实"的安全不变量。
type Summarizer interface {
	SummarizeCommunity(ctx context.Context, community *Community, members []*SemanticEntity) (string, error)
}

// SetSummarizer 注入摘要增强器。传 nil 可移除。
func (m *MemorySubgraph) SetSummarizer(s Summarizer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.summarizer = s
}

// EnrichSummaries 用注入的 Summarizer 重新生成社区摘要。
//
// 与 BuildCommunities 分离的原因：LLM 调用是慢速 I/O，不应在持有写锁、
// 阻塞其它读写的前提下执行。本方法先取快照、在锁外调用、再回写。
//
// 单个社区失败不会中断整批：失败者保留确定性摘要，并把错误聚合返回，
// 使"LLM 不可用"降级为"摘要质量下降"而非"系统不可用"。
func (m *MemorySubgraph) EnrichSummaries(ctx context.Context) (enriched int, errs []error) {
	m.mu.RLock()
	summarizer := m.summarizer
	snapshot := make([]*Community, 0, len(m.communities))
	membersOf := make(map[string][]*SemanticEntity, len(m.communities))
	for _, community := range m.communities {
		snapshot = append(snapshot, community.Clone())
		members := make([]*SemanticEntity, 0, len(community.Members))
		for _, id := range community.Members {
			if entity, ok := m.entities[id]; ok {
				members = append(members, entity.Clone())
			}
		}
		membersOf[community.ID] = members
	}
	m.mu.RUnlock()

	if summarizer == nil {
		return 0, nil
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].ID < snapshot[j].ID })

	results := make(map[string]string)
	for _, community := range snapshot {
		if ctx != nil && ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		summary, err := summarizer.SummarizeCommunity(ctx, community, membersOf[community.ID])
		if err != nil {
			errs = append(errs, fmt.Errorf("community %s: %w", community.ID, err))
			continue
		}
		if strings.TrimSpace(summary) == "" {
			errs = append(errs, fmt.Errorf("community %s: summarizer returned empty summary", community.ID))
			continue
		}
		results[community.ID] = summary
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for id, summary := range results {
		if community, ok := m.communities[id]; ok {
			community.Summary = summary
			enriched++
		}
	}
	return enriched, errs
}
