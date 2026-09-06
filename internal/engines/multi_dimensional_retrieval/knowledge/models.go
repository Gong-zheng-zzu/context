package knowledge

import (
	"fmt"
	"time"
)

// ==================== 🆕 新增知识模型 v3.1 ====================

// Entity - 通用实体节点
// 用于表示人员、团队、系统、服务、技术、组件、概念等
type Entity struct {
	ID          string    `json:"id"`          // UUID，唯一主键
	Name        string    `json:"name"`        // 实体名称
	Type        string    `json:"type"`        // Person|Team|System|Service|Technology|Component|Concept
	Description string    `json:"description"` // 实体描述
	Workspace   string    `json:"workspace"`   // 工作空间隔离
	MemoryIDs   []string  `json:"memory_ids"`  // 关联的Memory列表
	DocIDs      []string  `json:"doc_ids"`     // 源文档列表，用于可追溯检索
	UserID      string    `json:"user_id"`
	SessionID   string    `json:"session_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EntityType 枚举
const (
	EntityTypePerson     = "Person"     // 人员
	EntityTypeTeam       = "Team"       // 团队
	EntityTypeSystem     = "System"     // 系统
	EntityTypeService    = "Service"    // 服务
	EntityTypeTechnology = "Technology" // 技术
	EntityTypeComponent  = "Component"  // 组件
	EntityTypeConcept    = "Concept"    // 概念
)

// Event - 事件/问题节点
type Event struct {
	ID          string    `json:"id"`          // UUID，唯一主键
	Name        string    `json:"name"`        // 事件名称
	Type        string    `json:"type"`        // Issue|Decision|Task
	Description string    `json:"description"` // 事件描述
	Workspace   string    `json:"workspace"`   // 工作空间隔离
	MemoryIDs   []string  `json:"memory_ids"`  // 关联的Memory列表
	DocIDs      []string  `json:"doc_ids"`
	UserID      string    `json:"user_id"`
	SessionID   string    `json:"session_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// EventType 枚举
const (
	EventTypeIssue    = "Issue"    // 问题
	EventTypeDecision = "Decision" // 决策
	EventTypeTask     = "Task"     // 任务
)

// Solution - 解决方案节点
type Solution struct {
	ID          string    `json:"id"`          // UUID，唯一主键
	Name        string    `json:"name"`        // 方案名称
	Type        string    `json:"type"`        // combination|method|strategy
	Description string    `json:"description"` // 方案描述
	Workspace   string    `json:"workspace"`   // 工作空间隔离
	MemoryIDs   []string  `json:"memory_ids"`  // 关联的Memory列表
	DocIDs      []string  `json:"doc_ids"`
	UserID      string    `json:"user_id"`
	SessionID   string    `json:"session_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SolutionType 枚举
const (
	SolutionTypeCombination = "combination" // 组合方案
	SolutionTypeMethod      = "method"      // 方法
	SolutionTypeStrategy    = "strategy"    // 策略
)

// Feature - 功能特性节点
type Feature struct {
	ID          string    `json:"id"`          // UUID，唯一主键
	Name        string    `json:"name"`        // 特性名称
	Description string    `json:"description"` // 特性描述
	Workspace   string    `json:"workspace"`   // 工作空间隔离
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Relation - 关系（基于UUID关联）
type Relation struct {
	SourceID  string    `json:"source_id"` // 源节点UUID
	TargetID  string    `json:"target_id"` // 目标节点UUID
	Type      string    `json:"type"`      // 关系类型
	Weight    float64   `json:"weight"`    // 关系权重 0-1
	DocIDs    []string  `json:"doc_ids"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
}

// 🆕 新增关系类型常量
const (
	RelationMentions   = "MENTIONS"    // Memory提及知识节点
	RelationRelatesTo  = "RELATES_TO"  // 实体关联
	RelationCauses     = "CAUSES"      // 导致（Event->Event）
	RelationSolves     = "SOLVES"      // 解决（Solution->Event）
	RelationPrevents   = "PREVENTS"    // 预防（Solution->Event）
	RelationUses       = "USES"        // 使用（Solution->Entity）
	RelationHasFeature = "HAS_FEATURE" // 拥有特性（Entity->Feature）
	RelationBelongsTo  = "BELONGS_TO"  // 归属（Entity->Entity）
	RelationAssignedTo = "ASSIGNED_TO" // 分配给（Event->Entity:Person）
)

// ==================== 原有模型保留 ====================

// Concept 概念节点（保留向后兼容）
type Concept struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"` // "技术概念", "业务概念", "架构模式"等
	Keywords    []string  `json:"keywords"`
	Importance  float64   `json:"importance"` // 重要性评分 0-1
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Technology 技术节点
type Technology struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"` // "数据库", "框架", "工具"等
	Version     string    `json:"version"`
	Keywords    []string  `json:"keywords"`
	Popularity  float64   `json:"popularity"` // 流行度评分 0-1
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Project 项目节点
type Project struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Domain      string    `json:"domain"` // "电商", "支付", "物流"等
	TechStack   []string  `json:"tech_stack"`
	Status      string    `json:"status"` // "开发中", "已上线", "维护中"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// User 用户节点
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"` // "开发者", "架构师", "产品经理"
	Skills    []string  `json:"skills"`
	Interests []string  `json:"interests"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Relationship 关系
type Relationship struct {
	FromName    string    `json:"from_name"`
	ToName      string    `json:"to_name"`
	Type        string    `json:"type"`     // 关系类型
	Strength    float64   `json:"strength"` // 关系强度 0-1
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// 关系类型常量
const (
	// 概念关系
	RelationshipRelatedTo  = "RELATED_TO"  // 相关
	RelationshipDependsOn  = "DEPENDS_ON"  // 依赖
	RelationshipImplements = "IMPLEMENTS"  // 实现
	RelationshipExtends    = "EXTENDS"     // 扩展
	RelationshipComposedOf = "COMPOSED_OF" // 组成

	// 技术关系
	RelationshipUsedWith       = "USED_WITH"       // 配合使用
	RelationshipReplacedBy     = "REPLACED_BY"     // 被替代
	RelationshipBasedOn        = "BASED_ON"        // 基于
	RelationshipIntegratesWith = "INTEGRATES_WITH" // 集成

	// 项目关系
	RelationshipUsedIn    = "USED_IN"    // 用于项目
	RelationshipAppliedTo = "APPLIED_TO" // 应用于
	RelationshipSolves    = "SOLVES"     // 解决问题

	// 用户关系
	RelationshipExpertIn     = "EXPERT_IN"     // 专家
	RelationshipInterestedIn = "INTERESTED_IN" // 感兴趣
	RelationshipWorksOn      = "WORKS_ON"      // 工作于
)

// KnowledgeQuery 知识图谱查询
type KnowledgeQuery struct {
	// 查询类型
	QueryType string `json:"query_type"` // "expand", "path", "similarity", "search"

	// 起始概念
	StartConcepts []string `json:"start_concepts"`
	EndConcepts   []string `json:"end_concepts"`

	// 搜索条件
	SearchText string   `json:"search_text"`
	Keywords   []string `json:"keywords"`
	Categories []string `json:"categories"`

	// 过滤条件
	MinStrength float64 `json:"min_strength"` // 最小关系强度
	MinScore    float64 `json:"min_score"`    // 最小搜索得分
	MaxDepth    int     `json:"max_depth"`    // 最大扩展深度

	// 分页
	Limit  int `json:"limit"`
	Offset int `json:"offset"`

	// 用户上下文
	UserID      string `json:"user_id"`
	SessionID   string `json:"session_id"`
	WorkspaceID string `json:"workspace_id"`
}

// KnowledgeResult 知识图谱查询结果
type KnowledgeResult struct {
	Nodes         []KnowledgeNode         `json:"nodes"`
	Relationships []KnowledgeRelationship `json:"relationships"`
	Total         int                     `json:"total"`
	Duration      time.Duration           `json:"duration"`
	Query         *KnowledgeQuery         `json:"query"`

	// 扩展信息
	Paths       []KnowledgePath    `json:"paths,omitempty"`
	Clusters    []KnowledgeCluster `json:"clusters,omitempty"`
	Suggestions []string           `json:"suggestions,omitempty"`
}

// KnowledgeNode 知识节点
type KnowledgeNode struct {
	ID          string                 `json:"id"`
	Labels      []string               `json:"labels"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Keywords    []string               `json:"keywords"`
	DocIDs      []string               `json:"doc_ids"`
	Score       float64                `json:"score,omitempty"`
	Properties  map[string]interface{} `json:"properties"`
}

// KnowledgeRelationship 知识关系
type KnowledgeRelationship struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	StartNodeID string                 `json:"start_node_id"`
	EndNodeID   string                 `json:"end_node_id"`
	Strength    float64                `json:"strength"`
	Description string                 `json:"description"`
	Properties  map[string]interface{} `json:"properties"`
}

// KnowledgePath 知识路径
type KnowledgePath struct {
	Nodes         []KnowledgeNode         `json:"nodes"`
	Relationships []KnowledgeRelationship `json:"relationships"`
	Length        int                     `json:"length"`
	Score         float64                 `json:"score"`
}

// KnowledgeCluster 知识聚类
type KnowledgeCluster struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Nodes      []KnowledgeNode `json:"nodes"`
	Centrality float64         `json:"centrality"`
	Keywords   []string        `json:"keywords"`
}

// CreateConceptRequest 创建概念请求
type CreateConceptRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Keywords    []string `json:"keywords"`
	Importance  float64  `json:"importance"`
}

// CreateTechnologyRequest 创建技术请求
type CreateTechnologyRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Version     string   `json:"version"`
	Keywords    []string `json:"keywords"`
	Popularity  float64  `json:"popularity"`
}

// CreateRelationshipRequest 创建关系请求
type CreateRelationshipRequest struct {
	FromName    string  `json:"from_name"`
	ToName      string  `json:"to_name"`
	Type        string  `json:"type"`
	Strength    float64 `json:"strength"`
	Description string  `json:"description"`
}

// KnowledgeGraphStats 知识图谱统计
type KnowledgeGraphStats struct {
	TotalNodes          int            `json:"total_nodes"`
	TotalRelationships  int            `json:"total_relationships"`
	NodesByLabel        map[string]int `json:"nodes_by_label"`
	RelationshipsByType map[string]int `json:"relationships_by_type"`
	TopConcepts         []string       `json:"top_concepts"`
	TopTechnologies     []string       `json:"top_technologies"`
	Density             float64        `json:"density"`
	AveragePathLength   float64        `json:"average_path_length"`
}

// Validate 验证概念
func (c *Concept) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("概念名称不能为空")
	}
	if c.Category == "" {
		return fmt.Errorf("概念类别不能为空")
	}
	if c.Importance < 0 || c.Importance > 1 {
		return fmt.Errorf("重要性评分必须在0-1之间")
	}
	return nil
}

// Validate 验证技术
func (t *Technology) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("技术名称不能为空")
	}
	if t.Type == "" {
		return fmt.Errorf("技术类型不能为空")
	}
	if t.Popularity < 0 || t.Popularity > 1 {
		return fmt.Errorf("流行度评分必须在0-1之间")
	}
	return nil
}

// Validate 验证关系
func (r *Relationship) Validate() error {
	if r.FromName == "" || r.ToName == "" {
		return fmt.Errorf("关系的起始和结束节点不能为空")
	}
	if r.Type == "" {
		return fmt.Errorf("关系类型不能为空")
	}
	if r.Strength < 0 || r.Strength > 1 {
		return fmt.Errorf("关系强度必须在0-1之间")
	}
	return nil
}

// Validate 验证查询
func (q *KnowledgeQuery) Validate() error {
	if q.QueryType == "" {
		q.QueryType = "search" // 默认搜索
	}

	if q.Limit <= 0 {
		q.Limit = 20 // 默认限制
	}
	if q.Limit > 1000 {
		q.Limit = 1000 // 最大限制
	}

	if q.MaxDepth <= 0 {
		q.MaxDepth = 3 // 默认深度
	}
	if q.MaxDepth > 5 {
		q.MaxDepth = 5 // 最大深度
	}

	// 验证查询类型特定的参数
	switch q.QueryType {
	case "expand":
		if len(q.StartConcepts) == 0 {
			return fmt.Errorf("扩展查询需要指定起始概念")
		}
	case "path":
		if len(q.StartConcepts) == 0 || len(q.EndConcepts) == 0 {
			return fmt.Errorf("路径查询需要指定起始和结束概念")
		}
	case "search":
		if q.SearchText == "" && len(q.Keywords) == 0 {
			return fmt.Errorf("搜索查询需要指定搜索文本或关键词")
		}
	}

	return nil
}

// GetRelationshipDescription 获取关系描述
func GetRelationshipDescription(relType string) string {
	descriptions := map[string]string{
		RelationshipRelatedTo:      "相关",
		RelationshipDependsOn:      "依赖",
		RelationshipImplements:     "实现",
		RelationshipExtends:        "扩展",
		RelationshipComposedOf:     "组成",
		RelationshipUsedWith:       "配合使用",
		RelationshipReplacedBy:     "被替代",
		RelationshipBasedOn:        "基于",
		RelationshipIntegratesWith: "集成",
		RelationshipUsedIn:         "用于项目",
		RelationshipAppliedTo:      "应用于",
		RelationshipSolves:         "解决问题",
		RelationshipExpertIn:       "专家",
		RelationshipInterestedIn:   "感兴趣",
		RelationshipWorksOn:        "工作于",
		// 🆕 新增关系类型描述
		RelationMentions:   "提及",
		RelationRelatesTo:  "关联",
		RelationCauses:     "导致",
		RelationPrevents:   "预防",
		RelationUses:       "使用",
		RelationHasFeature: "具有特性",
		RelationBelongsTo:  "归属于",
		RelationAssignedTo: "分配给",
	}

	if desc, exists := descriptions[relType]; exists {
		return desc
	}
	return "未知关系"
}

// ==================== 🆕 新增模型验证方法 ====================

// Validate 验证Entity
func (e *Entity) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("Entity ID不能为空")
	}
	if e.Name == "" {
		return fmt.Errorf("Entity名称不能为空")
	}
	if e.Type == "" {
		return fmt.Errorf("Entity类型不能为空")
	}
	// 验证类型是否有效
	validTypes := map[string]bool{
		EntityTypePerson:     true,
		EntityTypeTeam:       true,
		EntityTypeSystem:     true,
		EntityTypeService:    true,
		EntityTypeTechnology: true,
		EntityTypeComponent:  true,
		EntityTypeConcept:    true,
	}
	if !validTypes[e.Type] {
		return fmt.Errorf("无效的Entity类型: %s", e.Type)
	}
	return nil
}

// Validate 验证Event
func (ev *Event) Validate() error {
	if ev.ID == "" {
		return fmt.Errorf("Event ID不能为空")
	}
	if ev.Name == "" {
		return fmt.Errorf("Event名称不能为空")
	}
	if ev.Type == "" {
		return fmt.Errorf("Event类型不能为空")
	}
	// 验证类型是否有效
	validTypes := map[string]bool{
		EventTypeIssue:    true,
		EventTypeDecision: true,
		EventTypeTask:     true,
	}
	if !validTypes[ev.Type] {
		return fmt.Errorf("无效的Event类型: %s", ev.Type)
	}
	return nil
}

// Validate 验证Solution
func (s *Solution) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("Solution ID不能为空")
	}
	if s.Name == "" {
		return fmt.Errorf("Solution名称不能为空")
	}
	if s.Type == "" {
		return fmt.Errorf("Solution类型不能为空")
	}
	// 验证类型是否有效
	validTypes := map[string]bool{
		SolutionTypeCombination: true,
		SolutionTypeMethod:      true,
		SolutionTypeStrategy:    true,
	}
	if !validTypes[s.Type] {
		return fmt.Errorf("无效的Solution类型: %s", s.Type)
	}
	return nil
}

// Validate 验证Feature
func (f *Feature) Validate() error {
	if f.ID == "" {
		return fmt.Errorf("Feature ID不能为空")
	}
	if f.Name == "" {
		return fmt.Errorf("Feature名称不能为空")
	}
	return nil
}

// Validate 验证Relation
func (r *Relation) Validate() error {
	if r.SourceID == "" {
		return fmt.Errorf("Relation源节点ID不能为空")
	}
	if r.TargetID == "" {
		return fmt.Errorf("Relation目标节点ID不能为空")
	}
	if r.Type == "" {
		return fmt.Errorf("Relation类型不能为空")
	}
	if r.Weight < 0 || r.Weight > 1 {
		return fmt.Errorf("Relation权重必须在0-1之间")
	}
	return nil
}
