// Package memory_subgraph 实现策划书 2.2.1 节描述的"三层解耦层级子图安全底盘"：
//
//	Ge 情景子图（Episodic Subgraph）   —— 原始对话文本/传感器数值/物理时间戳，作为"底盘事实"
//	Gs 语义实体子图（Semantic Entity） —— 从情景子图提炼的实体与事实边（有向逻辑关联）
//	Gc 社区子图（Community Subgraph）  —— LabelPropagation 聚类形成的"阶段性健康总览"
//
// 三层之间以 ID 溯源（provenance）串联：Gc 成员 → Gs 实体 → Ge 原始事件，
// 保证任何一条"阶段性健康总览"都可回溯到原始事实，杜绝无依据的摘要编造。
//
// 设计约束（与项目既有约定一致）：
//   - 全部计算在本地完成，不产生任何外网调用；
//   - 社区发现是确定性算法（节点按 ID 排序、平局按标签字典序），
//     同一输入在任何机器上产出同一分组，便于审计与复现；
//   - 跨 owner（老人）不做标签传播，与"一人一库"物理隔离口径一致。
package memory_subgraph

import (
	"fmt"
	"sort"
	"time"
)

// ============================ Ge 情景子图 ============================

// EpisodeSource 情景事件来源模态。
type EpisodeSource string

const (
	// SourceVoice 语音交班转写
	SourceVoice EpisodeSource = "voice"
	// SourceText 文本录入
	SourceText EpisodeSource = "text"
	// SourceSensor 传感器/设备数值
	SourceSensor EpisodeSource = "sensor"
	// SourceImage 图像描述（创面照片等的文字化结果）
	SourceImage EpisodeSource = "image"
)

// EpisodicNode 情景子图节点：一条不可再分的原始事实。
//
// 情景子图刻意保持"哑存储"：它不做语义加工，只负责保真地记住
// 「谁在什么时候、通过什么模态、说了/测到了什么」。上游所有语义结论
// 都必须能通过 ID 回溯到这一层，这是抗幻觉的证据底线。
type EpisodicNode struct {
	ID        string                 `json:"id"`
	OwnerID   string                 `json:"owner_id"` // 老人 ID，用于"一人一库"分区
	Timestamp time.Time              `json:"timestamp"`
	Source    EpisodeSource          `json:"source"`
	Content   string                 `json:"content"`
	Numbers   map[string]float64     `json:"numbers,omitempty"` // 传感器结构化数值（收缩压/舒张压/体温…）
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Clone 深拷贝，避免调用方持有内部引用。
func (e *EpisodicNode) Clone() *EpisodicNode {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Numbers = cloneFloatMap(e.Numbers)
	cp.Metadata = cloneAnyMap(e.Metadata)
	return &cp
}

// ============================ Gs 语义实体子图 ============================

// EntityKind 实体类别，对应策划书"老人、科室、病变创面"等实体类型。
type EntityKind string

const (
	// EntityElder 老人本人
	EntityElder EntityKind = "elder"
	// EntityDepartment 科室
	EntityDepartment EntityKind = "department"
	// EntityLesion 病变创面
	EntityLesion EntityKind = "lesion"
	// EntityDrug 药物
	EntityDrug EntityKind = "drug"
	// EntitySymptom 症状
	EntitySymptom EntityKind = "symptom"
	// EntityDisease 疾病/诊断
	EntityDisease EntityKind = "disease"
	// EntityCareAction 护理行为
	EntityCareAction EntityKind = "care_action"
	// EntityOther 其他
	EntityOther EntityKind = "other"
)

// SemanticEntity 语义实体子图节点。每个实体必须携带来源情景事件 ID 集合，
// 否则视为"无依据实体"并被拒绝写入。
type SemanticEntity struct {
	ID         string            `json:"id"`
	OwnerID    string            `json:"owner_id"`
	Name       string            `json:"name"`
	Kind       EntityKind        `json:"kind"`
	Attributes map[string]string `json:"attributes,omitempty"`
	// Episodes 是该实体的溯源锚点：证据文本来自哪些情景事件。
	Episodes   []string  `json:"episodes"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Clone 深拷贝。
func (e *SemanticEntity) Clone() *SemanticEntity {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Attributes = cloneStringMap(e.Attributes)
	cp.Episodes = append([]string(nil), e.Episodes...)
	return &cp
}

// FactEdge 事实边：实体之间的有向逻辑关联（如"李爷爷 →服用→ 降压药"）。
//
// Edge 采用有向语义存储，但社区发现阶段按无向加权图处理——
// 社区刻画的是"关联紧密程度"，而非因果关系方向；因果方向由
// PCCM 因果引擎单独负责，两者不混淆。
type FactEdge struct {
	ID         string    `json:"id"`
	OwnerID    string    `json:"owner_id"`
	Source     string    `json:"source"` // 源实体 ID
	Target     string    `json:"target"` // 目标实体 ID
	Relation   string    `json:"relation"`
	Weight     float64   `json:"weight"` // 关联强度（0,1]
	Episodes   []string  `json:"episodes"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// Clone 深拷贝。
func (e *FactEdge) Clone() *FactEdge {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Episodes = append([]string(nil), e.Episodes...)
	return &cp
}

// ============================ Gc 社区子图 ============================

// Community 社区：对语义实体做标签传播聚类后形成的"阶段性健康总览"。
type Community struct {
	ID       string   `json:"id"`
	OwnerID  string   `json:"owner_id"`
	Members  []string `json:"members"` // 成员实体 ID（已排序）
	Size     int      `json:"size"`
	Summary  string   `json:"summary"`
	Cohesion float64  `json:"cohesion"` // 社区内聚度（内部边权 / 总度）
	// KindHistogram 成员按实体类别统计，供摘要与检索路由使用。
	KindHistogram map[EntityKind]int `json:"kind_histogram"`
	// Episodes 是社区的溯源锚点：所有成员实体的情景事件并集（去重、已排序）。
	Episodes  []string  `json:"episodes"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Clone 深拷贝。
func (c *Community) Clone() *Community {
	if c == nil {
		return nil
	}
	cp := *c
	cp.Members = append([]string(nil), c.Members...)
	cp.Episodes = append([]string(nil), c.Episodes...)
	if c.KindHistogram != nil {
		cp.KindHistogram = make(map[EntityKind]int, len(c.KindHistogram))
		for k, v := range c.KindHistogram {
			cp.KindHistogram[k] = v
		}
	}
	return &cp
}

// ============================ 错误与选项 ============================

// ErrEntityWithoutProvenance 表示实体缺少情景事件溯源锚点。
// 项目约定：任何语义结论都必须可回溯到原始事实，因此该情况一律拒绝写入。
var ErrEntityWithoutProvenance = fmt.Errorf("memory_subgraph: entity has no episodic provenance")

// ErrOwnerMismatch 表示实体与事实边分属不同 owner。
var ErrOwnerMismatch = fmt.Errorf("memory_subgraph: owner mismatch")

// Config 控制子图构建策略。
type Config struct {
	// MaxIterations 标签传播最大迭代轮数。0 表示使用默认值 100。
	MaxIterations int
	// MinCommunitySize 小于该规模的社区会被并入"未归类"（Size 仍如实记录）。
	// 0 表示使用默认值 2。
	MinCommunitySize int
	// StrictOwnerPartition 为 true 时禁止跨 owner 的标签传播（默认 true）。
	// 与"一人一库"物理隔离口径一致；置 false 仅用于跨老人共现分析等特殊场景。
	StrictOwnerPartition bool
}

// DefaultConfig 返回默认配置。
func DefaultConfig() Config {
	return Config{
		MaxIterations:        100,
		MinCommunitySize:     2,
		StrictOwnerPartition: true,
	}
}

func (c Config) normalize() Config {
	if c.MaxIterations <= 0 {
		c.MaxIterations = 100
	}
	if c.MinCommunitySize <= 0 {
		c.MinCommunitySize = 2
	}
	return c
}

// ============================ 小工具 ============================

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if in == nil {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// sortedKeys 返回排序后的键，用于确定性遍历。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dedupSorted 去重并排序，保证溯源锚点集合的确定性表示。
func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
