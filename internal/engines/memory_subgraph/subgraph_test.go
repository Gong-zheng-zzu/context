package memory_subgraph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================ 测试夹具 ============================

// buildTwoCliqueGraph 构造一张"两个稠密团 + 一条弱连接"的图，
// 用于验证标签传播能否正确切分出两个社区。
//
//	A 团：a1-a2-a3 全连接（权重 1）
//	B 团：b1-b2-b3 全连接（权重 1）
//	桥边：a1-b1（权重 0.1，刻意很弱）
func buildTwoCliqueGraph(t *testing.T) *MemorySubgraph {
	t.Helper()
	g := NewMemorySubgraph(DefaultConfig())

	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	if err := g.AddEpisode(&EpisodicNode{
		ID: "ep:1", OwnerID: "elder-1", Timestamp: base,
		Source: SourceVoice, Content: "李爷爷今天血压偏高，服用了降压药",
	}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	entities := []struct {
		id   string
		name string
		kind EntityKind
	}{
		{"a1", "李爷爷", EntityElder},
		{"a2", "高血压", EntityDisease},
		{"a3", "头晕", EntitySymptom},
		{"b1", "降压药", EntityDrug},
		{"b2", "体位性低血压", EntitySymptom},
		{"b3", "跌倒", EntitySymptom},
	}
	for _, e := range entities {
		if err := g.AddEntity(&SemanticEntity{
			ID: e.id, OwnerID: "elder-1", Name: e.name, Kind: e.kind,
			Episodes: []string{"ep:1"}, Confidence: 0.9,
		}); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.id, err)
		}
	}

	edges := []struct {
		id       string
		source   string
		target   string
		relation string
		weight   float64
	}{
		{"f1", "a1", "a2", "患有", 1.0},
		{"f2", "a1", "a3", "主诉", 1.0},
		{"f3", "a2", "a3", "伴随", 1.0},
		{"f4", "b1", "b2", "导致", 1.0},
		{"f5", "b1", "b3", "可能引发", 1.0},
		{"f6", "b2", "b3", "表现为", 1.0},
		{"f7", "a1", "b1", "服用", 0.1},
	}
	for _, e := range edges {
		if err := g.AddFactEdge(&FactEdge{
			ID: e.id, OwnerID: "elder-1", Source: e.source, Target: e.target,
			Relation: e.relation, Weight: e.weight,
		}); err != nil {
			t.Fatalf("AddFactEdge(%s): %v", e.id, err)
		}
	}
	return g
}

// ============================ Ge/Gs 写入校验 ============================

// TestEntityWithoutProvenanceIsRejected 固化"语义结论必须可回溯"的不变量：
// 没有情景事件溯源的实体不得进入语义子图。
func TestEntityWithoutProvenanceIsRejected(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())

	err := g.AddEntity(&SemanticEntity{ID: "x", OwnerID: "elder-1", Name: "无依据实体"})
	if err == nil {
		t.Fatal("expected error for entity without provenance, got nil")
	}
	if !errors.Is(err, ErrEntityWithoutProvenance) {
		t.Fatalf("expected ErrEntityWithoutProvenance, got %v", err)
	}
}

// TestEntityReferencingUnknownEpisodeIsRejected 杜绝"引用不存在的事实"。
func TestEntityReferencingUnknownEpisodeIsRejected(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())

	err := g.AddEntity(&SemanticEntity{
		ID: "x", OwnerID: "elder-1", Name: "幽灵实体", Episodes: []string{"ep:does-not-exist"},
	})
	if !errors.Is(err, ErrEntityWithoutProvenance) {
		t.Fatalf("expected ErrEntityWithoutProvenance, got %v", err)
	}
}

// TestOwnerMismatchIsRejected 固化"一人一库"边界：实体与其溯源事件
// 必须同属一位老人。
func TestOwnerMismatchIsRejected(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())
	if err := g.AddEpisode(&EpisodicNode{ID: "ep:1", OwnerID: "elder-1"}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := g.AddEpisode(&EpisodicNode{ID: "ep:2", OwnerID: "elder-2"}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	err := g.AddEntity(&SemanticEntity{
		ID: "x", OwnerID: "elder-2", Name: "越界实体", Episodes: []string{"ep:1"},
	})
	if !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("expected ErrOwnerMismatch, got %v", err)
	}
}

// TestFactEdgeAcrossOwnersIsRejected 跨老人事实边必须被拒绝。
func TestFactEdgeAcrossOwnersIsRejected(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())
	for _, owner := range []string{"elder-1", "elder-2"} {
		if err := g.AddEpisode(&EpisodicNode{ID: "ep:" + owner, OwnerID: owner}); err != nil {
			t.Fatalf("AddEpisode: %v", err)
		}
		if err := g.AddEntity(&SemanticEntity{
			ID: "e:" + owner, OwnerID: owner, Name: owner, Episodes: []string{"ep:" + owner},
		}); err != nil {
			t.Fatalf("AddEntity: %v", err)
		}
	}

	err := g.AddFactEdge(&FactEdge{ID: "f", Source: "e:elder-1", Target: "e:elder-2"})
	if !errors.Is(err, ErrOwnerMismatch) {
		t.Fatalf("expected ErrOwnerMismatch, got %v", err)
	}
}

// ============================ 标签传播 ============================

// TestLabelPropagationSplitsTwoCliques 验证算法能把"两个稠密团 + 一条弱桥"
// 正确切成两个社区。
func TestLabelPropagationSplitsTwoCliques(t *testing.T) {
	g := buildTwoCliqueGraph(t)

	communities, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}
	if len(communities) != 2 {
		t.Fatalf("expected 2 communities, got %d (%v)", len(communities), communityMembers(communities))
	}

	// 每个社区恰好 3 个成员，且分属两个团。
	sizes := []int{communities[0].Size, communities[1].Size}
	if sizes[0] != 3 || sizes[1] != 3 {
		t.Fatalf("expected community sizes [3 3], got %v — %v", sizes, communityMembers(communities))
	}

	first := memberSet(communities[0])
	if _, ok := first["a1"]; ok {
		if _, ok := first["b1"]; ok {
			t.Fatalf("cliques were merged into one community: %v", communityMembers(communities))
		}
	}
}

// TestBuildCommunitiesIsDeterministic 固化确定性：同一张图重复构建，
// 分组、摘要与内聚度必须逐位一致。这是项目"可复现评测"的硬要求
// ——LPA 本身是随机算法，本项目通过固定遍历顺序与平局规则消除随机性。
func TestBuildCommunitiesIsDeterministic(t *testing.T) {
	g := buildTwoCliqueGraph(t)

	first, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}

	// 重建 20 次，逐字节比对结果。
	for i := 0; i < 20; i++ {
		again, err := g.BuildCommunities()
		if err != nil {
			t.Fatalf("BuildCommunities run %d: %v", i, err)
		}
		if len(again) != len(first) {
			t.Fatalf("run %d: community count %d != %d", i, len(again), len(first))
		}
		for j := range first {
			if again[j].ID != first[j].ID {
				t.Fatalf("run %d: community %d ID %q != %q", i, j, again[j].ID, first[j].ID)
			}
			if fmt.Sprint(again[j].Members) != fmt.Sprint(first[j].Members) {
				t.Fatalf("run %d: community %s members %v != %v",
					i, first[j].ID, again[j].Members, first[j].Members)
			}
			if again[j].Summary != first[j].Summary {
				t.Fatalf("run %d: community %s summary differs", i, first[j].ID)
			}
			if again[j].Cohesion != first[j].Cohesion {
				t.Fatalf("run %d: community %s cohesion %v != %v",
					i, first[j].ID, again[j].Cohesion, first[j].Cohesion)
			}
		}
	}
}

// TestModularityIsPositive 模块度为正，说明社区划分优于随机划分。
func TestModularityIsPositive(t *testing.T) {
	g := buildTwoCliqueGraph(t)
	if _, err := g.BuildCommunities(); err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}

	modularity := g.Modularity()
	if modularity <= 0 {
		t.Fatalf("expected positive modularity, got %v", modularity)
	}
	// 该夹具的解析解约 0.4836；给出宽容区间以防标定微调。
	if modularity < 0.40 || modularity > 0.55 {
		t.Fatalf("modularity %v outside expected range [0.40, 0.55]", modularity)
	}
}

// TestStrictOwnerPartitionPreventsCrossOwnerPropagation 固化"一人一库"：
// 即使两位老人的实体在名称上高度关联，也不得被聚进同一社区。
func TestStrictOwnerPartitionPreventsCrossOwnerPropagation(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())

	// 两位老人，各有一个实体；另有连接两实体的边会被拒绝，
	// 因此这里直接验证分区结果按 owner 隔离。
	for _, owner := range []string{"elder-1", "elder-2"} {
		if err := g.AddEpisode(&EpisodicNode{ID: "ep:" + owner, OwnerID: owner}); err != nil {
			t.Fatalf("AddEpisode: %v", err)
		}
	}
	if err := g.AddEntity(&SemanticEntity{ID: "x1", OwnerID: "elder-1", Name: "李爷爷", Kind: EntityElder, Episodes: []string{"ep:elder-1"}}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if err := g.AddEntity(&SemanticEntity{ID: "x2", OwnerID: "elder-2", Name: "李爷爷", Kind: EntityElder, Episodes: []string{"ep:elder-2"}}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}

	communities, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}
	if len(communities) != 2 {
		t.Fatalf("expected 2 owner-scoped communities, got %d", len(communities))
	}
	for _, c := range communities {
		if len(c.Members) != 1 {
			t.Fatalf("community %s mixed owners: %v", c.ID, c.Members)
		}
	}
}

// ============================ 三层溯源 ============================

// TestCommunityTracesBackToEpisodicFacts 验证 Gc → Gs → Ge 的完整回溯链，
// 这是"社区摘要不得凭空编造"的技术保证。
func TestCommunityTracesBackToEpisodicFacts(t *testing.T) {
	g := NewMemorySubgraph(DefaultConfig())
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	if err := g.AddEpisode(&EpisodicNode{
		ID: "ep:1", OwnerID: "elder-1", Timestamp: base,
		Source: SourceVoice, Content: "晚上没睡好，早上头晕",
	}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}
	if err := g.AddEpisode(&EpisodicNode{
		ID: "ep:2", OwnerID: "elder-1", Timestamp: base.Add(24 * time.Hour),
		Source: SourceSensor, Content: "收缩压 148", Numbers: map[string]float64{"systolic": 148},
	}); err != nil {
		t.Fatalf("AddEpisode: %v", err)
	}

	if err := g.AddEntity(&SemanticEntity{
		ID: "e1", OwnerID: "elder-1", Name: "头晕", Kind: EntitySymptom, Episodes: []string{"ep:1"},
	}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if err := g.AddEntity(&SemanticEntity{
		ID: "e2", OwnerID: "elder-1", Name: "高血压", Kind: EntityDisease, Episodes: []string{"ep:2"},
	}); err != nil {
		t.Fatalf("AddEntity: %v", err)
	}
	if err := g.AddFactEdge(&FactEdge{ID: "f1", Source: "e2", Target: "e1", Relation: "导致", Weight: 0.8}); err != nil {
		t.Fatalf("AddFactEdge: %v", err)
	}

	communities, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}
	if len(communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(communities))
	}

	community := communities[0]
	if community.Size != 2 {
		t.Fatalf("expected 2 members, got %d", community.Size)
	}
	if len(community.Episodes) != 2 {
		t.Fatalf("expected 2 traced episodes, got %v", community.Episodes)
	}

	// 回溯出的原始事实必须与写入内容一致，且按时间升序。
	episodes := g.EpisodesOfCommunity(community.ID)
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodic nodes, got %d", len(episodes))
	}
	if episodes[0].ID != "ep:1" || episodes[1].ID != "ep:2" {
		t.Fatalf("episodes not in chronological order: %s, %s", episodes[0].ID, episodes[1].ID)
	}
	if episodes[1].Numbers["systolic"] != 148 {
		t.Fatalf("episodic structured numbers lost in trace: %v", episodes[1].Numbers)
	}

	// 类别直方图应如实反映成员构成。
	if community.KindHistogram[EntitySymptom] != 1 || community.KindHistogram[EntityDisease] != 1 {
		t.Fatalf("unexpected kind histogram: %v", community.KindHistogram)
	}
}

// TestCommunitySummaryMentionsMembersAndProvenance 摘要必须同时包含
// 成员信息与溯源条数，便于审计核对。
func TestCommunitySummaryMentionsMembersAndProvenance(t *testing.T) {
	g := buildTwoCliqueGraph(t)
	communities, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}

	for _, c := range communities {
		if c.Summary == "" {
			t.Fatalf("community %s has empty summary", c.ID)
		}
		if !containsAll(c.Summary, "阶段性健康总览", "溯源证据") {
			t.Fatalf("summary missing required sections: %q", c.Summary)
		}
	}
}

// ============================ 可选 LLM 摘要增强 ============================

type stubSummarizer struct {
	failOn string
}

func (s stubSummarizer) SummarizeCommunity(_ context.Context, c *Community, _ []*SemanticEntity) (string, error) {
	if s.failOn == c.ID {
		return "", fmt.Errorf("stub failure for %s", c.ID)
	}
	return "LLM摘要:" + c.ID, nil
}

// TestEnrichSummariesKeepsDeterministicBaselineOnFailure LLM 增强失败时
// 必须降级为确定性摘要，而不是让整个社区失去摘要。
func TestEnrichSummariesKeepsDeterministicBaselineOnFailure(t *testing.T) {
	g := buildTwoCliqueGraph(t)
	communities, err := g.BuildCommunities()
	if err != nil {
		t.Fatalf("BuildCommunities: %v", err)
	}
	baseline := communities[0].Summary

	g.SetSummarizer(stubSummarizer{failOn: communities[0].ID})
	enriched, errs := g.EnrichSummaries(context.Background())

	if enriched != len(communities)-1 {
		t.Fatalf("expected %d enriched, got %d", len(communities)-1, enriched)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 aggregated error, got %d (%v)", len(errs), errs)
	}

	after := g.Communities()
	for _, c := range after {
		if c.ID == communities[0].ID {
			if c.Summary != baseline {
				t.Fatalf("failed community lost its deterministic baseline summary")
			}
		} else if c.Summary != "LLM摘要:"+c.ID {
			t.Fatalf("community %s not enriched: %q", c.ID, c.Summary)
		}
	}
}

// ============================ 辅助 ============================

func communityMembers(communities []*Community) [][]string {
	out := make([][]string, 0, len(communities))
	for _, c := range communities {
		out = append(out, c.Members)
	}
	return out
}

func memberSet(c *Community) map[string]struct{} {
	set := make(map[string]struct{}, len(c.Members))
	for _, m := range c.Members {
		set[m] = struct{}{}
	}
	return set
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
