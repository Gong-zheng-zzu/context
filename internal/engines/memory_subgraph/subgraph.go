package memory_subgraph

import (
	"fmt"
	"sort"
	"sync"
)

// MemorySubgraph 三层解耦层级子图。
//
// 读写并发安全。所有对外返回的实体/边/社区均为深拷贝，
// 调用方无法通过返回值反向修改内部状态。
type MemorySubgraph struct {
	mu     sync.RWMutex
	config Config

	// Ge 情景子图：episodeID -> 原始事实
	episodic map[string]*EpisodicNode
	// Gs 语义实体子图
	entities map[string]*SemanticEntity
	edges    map[string]*FactEdge
	// 邻接表：entityID -> {邻居 entityID -> 累计边权}
	adjacency map[string]map[string]float64

	// Gc 社区子图：communityID -> 社区
	communities map[string]*Community
	// 实体到社区的归属：entityID -> communityID
	entityCommunity map[string]string

	// summarizer 可选的社区摘要增强器（通常为本地 LLM）。为 nil 时
	// 社区仅携带确定性模板摘要。
	summarizer Summarizer
}

// NewMemorySubgraph 创建三层子图。
func NewMemorySubgraph(config Config) *MemorySubgraph {
	return &MemorySubgraph{
		config:          config.normalize(),
		episodic:        make(map[string]*EpisodicNode),
		entities:        make(map[string]*SemanticEntity),
		edges:           make(map[string]*FactEdge),
		adjacency:       make(map[string]map[string]float64),
		communities:     make(map[string]*Community),
		entityCommunity: make(map[string]string),
	}
}

// ============================ 写入（Ge） ============================

// AddEpisode 写入一条情景事实。ID 为空时返回错误。
func (m *MemorySubgraph) AddEpisode(ep *EpisodicNode) error {
	if ep == nil || ep.ID == "" {
		return fmt.Errorf("memory_subgraph: episode requires a non-empty ID")
	}
	if ep.OwnerID == "" {
		return fmt.Errorf("memory_subgraph: episode %q requires an owner ID", ep.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	stored := ep.Clone()
	if existing, ok := m.episodic[stored.ID]; ok {
		stored.Timestamp = existing.Timestamp
	}
	m.episodic[stored.ID] = stored
	return nil
}

// ============================ 写入（Gs） ============================

// AddEntity 写入一个语义实体。
//
// 强制校验溯源：实体必须至少引用一个已存在的情景事件，否则返回
// ErrEntityWithoutProvenance。这条约束把"语义结论必须有原始事实支撑"
// 从文档约定变成了代码级不变量。
func (m *MemorySubgraph) AddEntity(e *SemanticEntity) error {
	if e == nil || e.ID == "" {
		return fmt.Errorf("memory_subgraph: entity requires a non-empty ID")
	}
	if e.OwnerID == "" {
		return fmt.Errorf("memory_subgraph: entity %q requires an owner ID", e.ID)
	}
	if len(e.Episodes) == 0 {
		return fmt.Errorf("%w: entity %q", ErrEntityWithoutProvenance, e.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, episodeID := range e.Episodes {
		ep, ok := m.episodic[episodeID]
		if !ok {
			return fmt.Errorf("%w: entity %q references unknown episode %q",
				ErrEntityWithoutProvenance, e.ID, episodeID)
		}
		if ep.OwnerID != e.OwnerID {
			return fmt.Errorf("%w: entity %q owner %q vs episode %q owner %q",
				ErrOwnerMismatch, e.ID, e.OwnerID, episodeID, ep.OwnerID)
		}
	}

	stored := e.Clone()
	stored.Episodes = dedupSorted(stored.Episodes)
	if stored.Confidence <= 0 {
		stored.Confidence = 1.0
	}
	m.entities[stored.ID] = stored
	return nil
}

// AddFactEdge 写入一条事实边。两端实体必须已存在且同属一个 owner。
//
// 自环（Source == Target）被静默忽略：它不携带任何社区结构信息，
// 保留只会污染内聚度计算。
func (m *MemorySubgraph) AddFactEdge(e *FactEdge) error {
	if e == nil || e.ID == "" {
		return fmt.Errorf("memory_subgraph: fact edge requires a non-empty ID")
	}
	if e.Source == e.Target {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	src, ok := m.entities[e.Source]
	if !ok {
		return fmt.Errorf("memory_subgraph: fact edge %q references unknown source entity %q", e.ID, e.Source)
	}
	dst, ok := m.entities[e.Target]
	if !ok {
		return fmt.Errorf("memory_subgraph: fact edge %q references unknown target entity %q", e.ID, e.Target)
	}
	if src.OwnerID != dst.OwnerID {
		return fmt.Errorf("%w: fact edge %q spans owner %q and %q",
			ErrOwnerMismatch, e.ID, src.OwnerID, dst.OwnerID)
	}

	weight := e.Weight
	if weight <= 0 {
		weight = 1.0
	}
	if weight > 1 {
		weight = 1
	}

	stored := e.Clone()
	stored.OwnerID = src.OwnerID
	stored.Weight = weight
	stored.Episodes = dedupSorted(stored.Episodes)
	m.edges[stored.ID] = stored

	if m.adjacency[stored.Source] == nil {
		m.adjacency[stored.Source] = make(map[string]float64)
	}
	if m.adjacency[stored.Target] == nil {
		m.adjacency[stored.Target] = make(map[string]float64)
	}
	// 社区发现按无向加权图处理：两个方向的邻接都累加。
	m.adjacency[stored.Source][stored.Target] += weight
	m.adjacency[stored.Target][stored.Source] += weight

	return nil
}

// ============================ 社区构建（Gc） ============================

// BuildCommunities 在语义实体子图上执行 LabelPropagation，重建社区子图。
//
// 返回按 ID 排序的社区列表。该方法是确定性的：同一张图重复调用产出
// 完全一致的分组、摘要与内聚度。
func (m *MemorySubgraph) BuildCommunities() ([]*Community, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	partitions := m.partitionByOwner()
	communities := make([]*Community, 0, len(partitions))

	// 按 owner 字典序遍历，保证社区 ID 分配顺序稳定。
	owners := make([]string, 0, len(partitions))
	for owner := range partitions {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	for _, owner := range owners {
		members := partitions[owner]
		groups := labelPropagation(members, m.adjacency, m.config.MaxIterations)

		// 组内按社区规模降序、最小成员 ID 升序排列，令社区 ID 稳定。
		sort.Slice(groups, func(i, j int) bool {
			if len(groups[i]) != len(groups[j]) {
				return len(groups[i]) > len(groups[j])
			}
			return groups[i][0] < groups[j][0]
		})

		for idx, group := range groups {
			community := m.buildCommunity(owner, idx, group)
			communities = append(communities, community)
		}
	}

	// 原子替换社区子图。
	m.communities = make(map[string]*Community, len(communities))
	m.entityCommunity = make(map[string]string, len(m.entities))
	for _, c := range communities {
		m.communities[c.ID] = c
		for _, member := range c.Members {
			m.entityCommunity[member] = c.ID
		}
	}

	out := make([]*Community, 0, len(communities))
	for _, c := range communities {
		out = append(out, c.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// partitionByOwner 按 owner 分组实体 ID。
//
// StrictOwnerPartition 为 false 时所有实体归入同一分区（允许跨老人共现分析）；
// 为 true 时每个 owner 独立成区，标签传播不会跨越"一人一库"边界。
func (m *MemorySubgraph) partitionByOwner() map[string][]string {
	partitions := make(map[string][]string)
	for _, id := range sortedKeys(m.entities) {
		owner := m.entities[id].OwnerID
		if !m.config.StrictOwnerPartition {
			owner = "__global__"
		}
		partitions[owner] = append(partitions[owner], id)
	}
	for owner := range partitions {
		sort.Strings(partitions[owner])
	}
	return partitions
}

// buildCommunity 把一组实体 ID 组装为社区对象（含内聚度、摘要、溯源锚点）。
// 调用方必须已持有写锁。
func (m *MemorySubgraph) buildCommunity(owner string, index int, members []string) *Community {
	sort.Strings(members)

	kindHistogram := make(map[EntityKind]int, 8)
	episodeSet := make(map[string]struct{})
	entityList := make([]*SemanticEntity, 0, len(members))

	for _, id := range members {
		entity := m.entities[id]
		if entity == nil {
			continue
		}
		kindHistogram[entity.Kind]++
		entityList = append(entityList, entity)
		for _, episodeID := range entity.Episodes {
			episodeSet[episodeID] = struct{}{}
		}
	}

	memberSet := make(map[string]struct{}, len(members))
	for _, id := range members {
		memberSet[id] = struct{}{}
	}

	// 内聚度 = 社区内部边权之和 / 社区总度（含指向社区外的边权）。
	// 取值 (0,1]：1 表示社区与外界完全无连接。
	internalWeight := 0.0
	totalWeight := 0.0
	for _, id := range members {
		for neighbor, weight := range m.adjacency[id] {
			totalWeight += weight
			if _, inside := memberSet[neighbor]; inside {
				internalWeight += weight
			}
		}
	}
	cohesion := 0.0
	if totalWeight > 0 {
		cohesion = internalWeight / totalWeight
	}

	episodes := make([]string, 0, len(episodeSet))
	for id := range episodeSet {
		episodes = append(episodes, id)
	}
	sort.Strings(episodes)

	community := &Community{
		ID:            fmt.Sprintf("gc:%s:%03d", owner, index),
		OwnerID:       owner,
		Members:       members,
		Size:          len(members),
		Cohesion:      cohesion,
		KindHistogram: kindHistogram,
		Episodes:      episodes,
	}
	community.Summary = summarizeCommunity(community, entityList)
	return community
}

// ============================ 查询 ============================

// Communities 返回社区子图快照（按 ID 排序）。
func (m *MemorySubgraph) Communities() []*Community {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Community, 0, len(m.communities))
	for _, c := range m.communities {
		out = append(out, c.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// EntitiesOfCommunity 返回社区成员实体（按 ID 排序）。
func (m *MemorySubgraph) EntitiesOfCommunity(communityID string) []*SemanticEntity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	community, ok := m.communities[communityID]
	if !ok {
		return nil
	}
	out := make([]*SemanticEntity, 0, len(community.Members))
	for _, id := range community.Members {
		if entity, ok := m.entities[id]; ok {
			out = append(out, entity.Clone())
		}
	}
	return out
}

// EpisodesOfEntity 沿 Gs → Ge 回溯实体的原始事实（按时间升序）。
func (m *MemorySubgraph) EpisodesOfEntity(entityID string) []*EpisodicNode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.episodesOf(entityIDsToSlice(entityID))
}

// EpisodesOfCommunity 沿 Gc → Gs → Ge 回溯社区的全部原始事实（按时间升序）。
func (m *MemorySubgraph) EpisodesOfCommunity(communityID string) []*EpisodicNode {
	m.mu.RLock()
	defer m.mu.RUnlock()

	community, ok := m.communities[communityID]
	if !ok {
		return nil
	}
	return m.episodesOf(community.Members)
}

// episodesOf 汇总一组实体的情景事件并去重排序。调用方必须已持锁。
func (m *MemorySubgraph) episodesOf(entityIDs []string) []*EpisodicNode {
	seen := make(map[string]struct{})
	out := make([]*EpisodicNode, 0)

	for _, entityID := range entityIDs {
		entity, ok := m.entities[entityID]
		if !ok {
			continue
		}
		for _, episodeID := range entity.Episodes {
			if _, dup := seen[episodeID]; dup {
				continue
			}
			if episode, ok := m.episodic[episodeID]; ok {
				seen[episodeID] = struct{}{}
				out = append(out, episode.Clone())
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			return out[i].ID < out[j].ID
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}

// Entity 返回实体快照。
func (m *MemorySubgraph) Entity(entityID string) (*SemanticEntity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entity, ok := m.entities[entityID]
	if !ok {
		return nil, false
	}
	return entity.Clone(), true
}

// Episode 返回情景事实快照。
func (m *MemorySubgraph) Episode(episodeID string) (*EpisodicNode, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	episode, ok := m.episodic[episodeID]
	if !ok {
		return nil, false
	}
	return episode.Clone(), true
}

// Edges 返回事实边快照（按 ID 排序）。
func (m *MemorySubgraph) Edges() []*FactEdge {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*FactEdge, 0, len(m.edges))
	for _, e := range m.edges {
		out = append(out, e.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Stats 子图规模统计。
type Stats struct {
	Episodes    int `json:"episodes"`
	Entities    int `json:"entities"`
	Edges       int `json:"edges"`
	Communities int `json:"communities"`
}

// Stats 返回当前规模统计。
func (m *MemorySubgraph) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Stats{
		Episodes:    len(m.episodic),
		Entities:    len(m.entities),
		Edges:       len(m.edges),
		Communities: len(m.communities),
	}
}

func entityIDsToSlice(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}
