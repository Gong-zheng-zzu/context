package knowledge

import (
	"testing"
	"time"
)

func TestEntityCreation(t *testing.T) {
	entity := &Entity{
		ID:          "test-entity-id",
		Name:        "Redis",
		Type:        EntityTypeTechnology, // 使用实际定义的常量
		Description: "高性能内存数据库",
		Workspace:   "/workspace",
		MemoryIDs:   []string{"mem1", "mem2"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if entity.ID != "test-entity-id" {
		t.Errorf("Entity ID错误: %s", entity.ID)
	}

	if entity.Name != "Redis" {
		t.Errorf("Entity Name错误: %s", entity.Name)
	}

	if entity.Type != EntityTypeTechnology {
		t.Errorf("Entity Type错误: %s", entity.Type)
	}

	if len(entity.MemoryIDs) != 2 {
		t.Errorf("MemoryIDs数量错误: %d", len(entity.MemoryIDs))
	}
}

func TestEventCreation(t *testing.T) {
	event := &Event{
		ID:          "test-event-id",
		Name:        "连接超时",
		Type:        EventTypeIssue,
		Description: "Redis连接超时问题",
		Workspace:   "/workspace",
		MemoryIDs:   []string{"mem1"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if event.ID != "test-event-id" {
		t.Errorf("Event ID错误: %s", event.ID)
	}

	if event.Type != EventTypeIssue {
		t.Errorf("Event Type错误: %s", event.Type)
	}
}

func TestSolutionCreation(t *testing.T) {
	solution := &Solution{
		ID:          "test-solution-id",
		Name:        "增加连接池大小",
		Type:        SolutionTypeStrategy, // 使用实际定义的常量
		Description: "将连接池从10增加到50",
		Workspace:   "/workspace",
		MemoryIDs:   []string{"mem1"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if solution.ID != "test-solution-id" {
		t.Errorf("Solution ID错误: %s", solution.ID)
	}

	if solution.Type != SolutionTypeStrategy {
		t.Errorf("Solution Type错误: %s", solution.Type)
	}
}

func TestRelationCreation(t *testing.T) {
	relation := &Relation{
		SourceID:  "entity1",
		TargetID:  "entity2",
		Type:      RelationRelatesTo,
		Weight:    0.8,
		CreatedAt: time.Now(),
	}

	if relation.SourceID != "entity1" {
		t.Errorf("Relation SourceID错误: %s", relation.SourceID)
	}

	if relation.TargetID != "entity2" {
		t.Errorf("Relation TargetID错误: %s", relation.TargetID)
	}

	if relation.Type != RelationRelatesTo {
		t.Errorf("Relation Type错误: %s", relation.Type)
	}

	if relation.Weight != 0.8 {
		t.Errorf("Relation Weight错误: %f", relation.Weight)
	}
}

func TestEntityTypeConstants(t *testing.T) {
	// 验证Entity类型常量 - 使用实际定义的常量
	types := []string{
		EntityTypePerson,
		EntityTypeTeam,
		EntityTypeSystem,
		EntityTypeService,
		EntityTypeTechnology,
		EntityTypeComponent,
		EntityTypeConcept,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("Entity类型常量不应为空")
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	// 使用实际定义的Event类型常量
	types := []string{
		EventTypeIssue,
		EventTypeDecision,
		EventTypeTask,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("Event类型常量不应为空")
		}
	}
}

func TestSolutionTypeConstants(t *testing.T) {
	// 使用实际定义的Solution类型常量
	types := []string{
		SolutionTypeCombination,
		SolutionTypeMethod,
		SolutionTypeStrategy,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("Solution类型常量不应为空")
		}
	}
}

func TestRelationTypeConstants(t *testing.T) {
	// 使用实际定义的Relation类型常量
	types := []string{
		RelationMentions,
		RelationRelatesTo,
		RelationCauses,
		RelationSolves,
		RelationPrevents,
		RelationUses,
		RelationHasFeature,
		RelationBelongsTo,
		RelationAssignedTo,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("Relation类型常量不应为空")
		}
	}
}

func TestEntityValidate(t *testing.T) {
	// 有效Entity
	entity := &Entity{
		ID:        "test-id-123",
		Name:      "Redis",
		Type:      EntityTypeTechnology,
		Workspace: "/workspace",
	}
	if err := entity.Validate(); err != nil {
		t.Errorf("有效Entity应该通过验证: %v", err)
	}

	// 无效 - 空ID
	invalidEntity0 := &Entity{Name: "Redis", Type: EntityTypeTechnology, Workspace: "/workspace"}
	if err := invalidEntity0.Validate(); err == nil {
		t.Error("空ID Entity应该验证失败")
	}

	// 无效 - 空名称
	invalidEntity := &Entity{ID: "id1", Type: EntityTypeTechnology, Workspace: "/workspace"}
	if err := invalidEntity.Validate(); err == nil {
		t.Error("空名称Entity应该验证失败")
	}

	// 无效 - 空类型
	invalidEntity2 := &Entity{ID: "id1", Name: "Redis", Workspace: "/workspace"}
	if err := invalidEntity2.Validate(); err == nil {
		t.Error("空类型Entity应该验证失败")
	}

	// 无效类型
	invalidEntity3 := &Entity{ID: "id1", Name: "Redis", Type: "InvalidType", Workspace: "/workspace"}
	if err := invalidEntity3.Validate(); err == nil {
		t.Error("无效类型Entity应该验证失败")
	}
}

func TestEventValidate(t *testing.T) {
	// 有效Event
	event := &Event{
		ID:        "test-event-id",
		Name:      "问题A",
		Type:      EventTypeIssue,
		Workspace: "/workspace",
	}
	if err := event.Validate(); err != nil {
		t.Errorf("有效Event应该通过验证: %v", err)
	}

	// 无效 - 空名称
	invalidEvent := &Event{ID: "id1", Type: EventTypeIssue}
	if err := invalidEvent.Validate(); err == nil {
		t.Error("空名称Event应该验证失败")
	}
}

func TestSolutionValidate(t *testing.T) {
	// 有效Solution
	solution := &Solution{
		ID:        "test-solution-id",
		Name:      "方案A",
		Type:      SolutionTypeMethod,
		Workspace: "/workspace",
	}
	if err := solution.Validate(); err != nil {
		t.Errorf("有效Solution应该通过验证: %v", err)
	}

	// 无效 - 空名称
	invalidSolution := &Solution{ID: "id1", Type: SolutionTypeMethod}
	if err := invalidSolution.Validate(); err == nil {
		t.Error("空名称Solution应该验证失败")
	}
}

func TestRelationValidate(t *testing.T) {
	// 有效Relation
	relation := &Relation{
		SourceID: "source1",
		TargetID: "target1",
		Type:     RelationRelatesTo,
		Weight:   0.5,
	}
	if err := relation.Validate(); err != nil {
		t.Errorf("有效Relation应该通过验证: %v", err)
	}

	// 无效 - 权重超范围
	invalidRelation := &Relation{
		SourceID: "source1",
		TargetID: "target1",
		Type:     RelationRelatesTo,
		Weight:   1.5, // 超出0-1范围
	}
	if err := invalidRelation.Validate(); err == nil {
		t.Error("权重超范围Relation应该验证失败")
	}
}

func TestFeatureValidate(t *testing.T) {
	// 有效Feature
	feature := &Feature{
		ID:        "test-feature-id",
		Name:      "特性A",
		Workspace: "/workspace",
	}
	if err := feature.Validate(); err != nil {
		t.Errorf("有效Feature应该通过验证: %v", err)
	}

	// 无效 - 空名称
	invalidFeature := &Feature{ID: "id1", Workspace: "/workspace"}
	if err := invalidFeature.Validate(); err == nil {
		t.Error("空名称Feature应该验证失败")
	}
}

func TestGetRelationshipDescription(t *testing.T) {
	// 测试已知关系类型
	desc := GetRelationshipDescription(RelationshipRelatedTo)
	if desc != "相关" {
		t.Errorf("期望'相关', got: %s", desc)
	}

	desc = GetRelationshipDescription(RelationMentions)
	if desc != "提及" {
		t.Errorf("期望'提及', got: %s", desc)
	}

	// 测试未知关系类型 - 根据实际实现，返回"未知关系"
	desc = GetRelationshipDescription("UNKNOWN_TYPE")
	if desc != "未知关系" {
		t.Errorf("未知类型应该返回'未知关系', got: %s", desc)
	}
}

func TestConceptValidate(t *testing.T) {
	// 有效Concept
	concept := &Concept{
		Name:        "测试概念",
		Description: "描述",
		Category:    "技术",
	}
	if err := concept.Validate(); err != nil {
		t.Errorf("有效Concept应该通过验证: %v", err)
	}

	// 无效 - 空名称
	invalidConcept := &Concept{Description: "描述"}
	if err := invalidConcept.Validate(); err == nil {
		t.Error("空名称Concept应该验证失败")
	}
}

func TestTechnologyValidate(t *testing.T) {
	// 有效Technology
	tech := &Technology{
		Name:    "Go",
		Type:    "编程语言",
		Version: "1.21",
	}
	if err := tech.Validate(); err != nil {
		t.Errorf("有效Technology应该通过验证: %v", err)
	}

	// 无效 - 空名称
	invalidTech := &Technology{Type: "编程语言"}
	if err := invalidTech.Validate(); err == nil {
		t.Error("空名称Technology应该验证失败")
	}
}

func TestRelationshipValidate(t *testing.T) {
	// 有效Relationship
	rel := &Relationship{
		FromName: "A",
		ToName:   "B",
		Type:     RelationshipRelatedTo,
		Strength: 0.8,
	}
	if err := rel.Validate(); err != nil {
		t.Errorf("有效Relationship应该通过验证: %v", err)
	}

	// 无效 - 空FromName
	invalidRel := &Relationship{ToName: "B", Type: RelationshipRelatedTo}
	if err := invalidRel.Validate(); err == nil {
		t.Error("空FromName Relationship应该验证失败")
	}

	// 无效 - 空Type
	invalidRel2 := &Relationship{FromName: "A", ToName: "B"}
	if err := invalidRel2.Validate(); err == nil {
		t.Error("空Type Relationship应该验证失败")
	}

	// 无效 - 强度超范围
	invalidRel3 := &Relationship{FromName: "A", ToName: "B", Type: RelationshipRelatedTo, Strength: 1.5}
	if err := invalidRel3.Validate(); err == nil {
		t.Error("强度超范围Relationship应该验证失败")
	}
}

func TestKnowledgeQueryValidate(t *testing.T) {
	// 有效KnowledgeQuery
	query := &KnowledgeQuery{
		SearchText: "测试查询",
		Limit:      10,
	}
	if err := query.Validate(); err != nil {
		t.Errorf("有效KnowledgeQuery应该通过验证: %v", err)
	}

	// 无效 - 空SearchText
	invalidQuery := &KnowledgeQuery{Limit: 10}
	if err := invalidQuery.Validate(); err == nil {
		t.Error("空SearchText KnowledgeQuery应该验证失败")
	}
}

func TestEventValidateInvalidType(t *testing.T) {
	// 无效Event类型
	event := &Event{
		ID:   "id1",
		Name: "测试",
		Type: "InvalidType",
	}
	if err := event.Validate(); err == nil {
		t.Error("无效类型Event应该验证失败")
	}
}

func TestSolutionValidateInvalidType(t *testing.T) {
	// 无效Solution类型
	solution := &Solution{
		ID:   "id1",
		Name: "测试",
		Type: "InvalidType",
	}
	if err := solution.Validate(); err == nil {
		t.Error("无效类型Solution应该验证失败")
	}
}

func TestRelationValidateEmptyFields(t *testing.T) {
	// 空SourceID
	rel1 := &Relation{TargetID: "t1", Type: RelationRelatesTo, Weight: 0.5}
	if err := rel1.Validate(); err == nil {
		t.Error("空SourceID Relation应该验证失败")
	}

	// 空TargetID
	rel2 := &Relation{SourceID: "s1", Type: RelationRelatesTo, Weight: 0.5}
	if err := rel2.Validate(); err == nil {
		t.Error("空TargetID Relation应该验证失败")
	}

	// 空Type
	rel3 := &Relation{SourceID: "s1", TargetID: "t1", Weight: 0.5}
	if err := rel3.Validate(); err == nil {
		t.Error("空Type Relation应该验证失败")
	}

	// 负权重
	rel4 := &Relation{SourceID: "s1", TargetID: "t1", Type: RelationRelatesTo, Weight: -0.1}
	if err := rel4.Validate(); err == nil {
		t.Error("负权重Relation应该验证失败")
	}
}
