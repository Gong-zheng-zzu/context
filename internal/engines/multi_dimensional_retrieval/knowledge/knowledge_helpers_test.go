package knowledge

import (
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestBuildKnowledgeQueryScopesEveryModeAndRequiresSourceEvidence(t *testing.T) {
	engine := &Neo4jEngine{}
	baseQuery := KnowledgeQuery{
		StartConcepts: []string{"Redis"},
		EndConcepts:   []string{"PostgreSQL"},
		SearchText:    "缓存故障",
		Keywords:      []string{"缓存"},
		Categories:    []string{"database"},
		UserID:        "user-1",
		SessionID:     "session-1",
		WorkspaceID:   "workspace-1",
		Limit:         10,
	}

	for _, queryType := range []string{"expand", "path", "similarity", "search"} {
		query := baseQuery
		query.QueryType = queryType
		cypherQuery, parameters := engine.buildKnowledgeQuery(&query)

		for _, placeholder := range []string{"$user_id", "$session_id", "$workspace_id", "doc_ids"} {
			if !strings.Contains(cypherQuery, placeholder) {
				t.Fatalf("%s query is missing %q:\n%s", queryType, placeholder, cypherQuery)
			}
		}
		if parameters["user_id"] != query.UserID || parameters["session_id"] != query.SessionID || parameters["workspace_id"] != query.WorkspaceID {
			t.Fatalf("%s query did not propagate scope: %#v", queryType, parameters)
		}
	}
}

func TestSearchQueryUsesOriginalAndParsedTermsWithScopedRelationships(t *testing.T) {
	engine := &Neo4jEngine{}
	query := &KnowledgeQuery{
		QueryType:   "search",
		SearchText:  "patient recent health changes",
		Keywords:    []string{"patient", "health", "patient"},
		UserID:      "user-1",
		SessionID:   "session-1",
		WorkspaceID: "workspace-1",
		Limit:       5,
	}

	cypherQuery, parameters := engine.buildKnowledgeQuery(query)
	if !strings.Contains(cypherQuery, "$search_terms") {
		t.Fatalf("search query must use separate original and parsed terms:\n%s", cypherQuery)
	}
	terms, ok := parameters["search_terms"].([]string)
	if !ok || len(terms) != 3 || terms[0] != query.SearchText || terms[1] != "patient" || terms[2] != "health" {
		t.Fatalf("search_terms = %#v, want original query plus distinct parsed terms", parameters["search_terms"])
	}

	query.QueryType = "expand"
	query.StartConcepts = []string{"patient"}
	cypherQuery, _ = engine.buildKnowledgeQuery(query)
	for _, relationshipScope := range []string{"r.user_id = $user_id", "r.session_id = $session_id", "r.workspace = $workspace_id"} {
		if !strings.Contains(cypherQuery, relationshipScope) {
			t.Fatalf("expand query must scope relationships by %q:\n%s", relationshipScope, cypherQuery)
		}
	}
}

func TestParseNodePropagatesDocIDs(t *testing.T) {
	node := neo4j.Node{
		ElementId: "node-1",
		Labels:    []string{"Entity"},
		Props: map[string]interface{}{
			"name":    "Redis",
			"doc_ids": []string{"doc-1", "doc-2"},
		},
	}

	parsed := (&Neo4jEngine{}).parseNode(node)
	if len(parsed.DocIDs) != 2 || parsed.DocIDs[0] != "doc-1" || parsed.DocIDs[1] != "doc-2" {
		t.Fatalf("DocIDs = %v, want [doc-1 doc-2]", parsed.DocIDs)
	}
}

func TestGenerateEntityUUID(t *testing.T) {
	// 测试确定性UUID生成
	uuid1 := GenerateEntityUUID("Redis", "database", "/workspace1")
	uuid2 := GenerateEntityUUID("Redis", "database", "/workspace1")
	uuid3 := GenerateEntityUUID("redis", "database", "/workspace1") // 大小写不敏感

	// 相同输入应生成相同UUID
	if uuid1 != uuid2 {
		t.Errorf("相同输入应生成相同UUID: got %s vs %s", uuid1, uuid2)
	}

	// 大小写不敏感
	if uuid1 != uuid3 {
		t.Errorf("大小写不敏感测试失败: got %s vs %s", uuid1, uuid3)
	}

	// 验证UUID格式
	if !ValidateUUID(uuid1) {
		t.Errorf("生成的UUID格式无效: %s", uuid1)
	}

	// 不同实体应生成不同UUID
	uuid4 := GenerateEntityUUID("MySQL", "database", "/workspace1")
	if uuid1 == uuid4 {
		t.Errorf("不同输入应生成不同UUID: got same %s", uuid1)
	}
}

func TestGenerateEventUUID(t *testing.T) {
	uuid1 := GenerateEventUUID("内存溢出", "issue", "/workspace1")
	uuid2 := GenerateEventUUID("内存溢出", "issue", "/workspace1")

	if uuid1 != uuid2 {
		t.Errorf("相同输入应生成相同UUID: got %s vs %s", uuid1, uuid2)
	}

	if !ValidateUUID(uuid1) {
		t.Errorf("生成的UUID格式无效: %s", uuid1)
	}
}

func TestGenerateSolutionUUID(t *testing.T) {
	uuid1 := GenerateSolutionUUID("增加连接池", "optimization", "/workspace1")
	uuid2 := GenerateSolutionUUID("增加连接池", "optimization", "/workspace1")

	if uuid1 != uuid2 {
		t.Errorf("相同输入应生成相同UUID: got %s vs %s", uuid1, uuid2)
	}

	if !ValidateUUID(uuid1) {
		t.Errorf("生成的UUID格式无效: %s", uuid1)
	}
}

func TestExtractKnowledgeNodeIDs(t *testing.T) {
	analysisData := map[string]interface{}{
		"entities": []interface{}{
			map[string]interface{}{"name": "Redis", "type": "database"},
			map[string]interface{}{"name": "MySQL", "type": "database"},
		},
		"events": []interface{}{
			map[string]interface{}{"name": "连接超时", "type": "issue"},
		},
		"solutions": []interface{}{
			map[string]interface{}{"name": "增加连接池", "type": "optimization"},
		},
	}

	result := ExtractKnowledgeNodeIDs(analysisData, "/workspace1")

	if len(result.EntityIDs) != 2 {
		t.Errorf("期望2个EntityID, 实际: %d", len(result.EntityIDs))
	}

	if len(result.EventIDs) != 1 {
		t.Errorf("期望1个EventID, 实际: %d", len(result.EventIDs))
	}

	if len(result.SolutionIDs) != 1 {
		t.Errorf("期望1个SolutionID, 实际: %d", len(result.SolutionIDs))
	}

	// 验证生成的UUID格式
	for _, id := range result.EntityIDs {
		if !ValidateUUID(id) {
			t.Errorf("EntityID格式无效: %s", id)
		}
	}
}

func TestBuildEntitiesFromAnalysis(t *testing.T) {
	analysisData := map[string]interface{}{
		"entities": []interface{}{
			map[string]interface{}{
				"name":        "Redis",
				"type":        "database",
				"description": "高性能内存数据库",
			},
		},
	}

	entities := BuildEntitiesFromAnalysis(analysisData, "/workspace1")

	if len(entities) != 1 {
		t.Errorf("期望1个Entity, 实际: %d", len(entities))
	}

	if entities[0].Name != "Redis" {
		t.Errorf("Entity名称错误: %s", entities[0].Name)
	}

	if entities[0].Type != "database" {
		t.Errorf("Entity类型错误: %s", entities[0].Type)
	}

	if entities[0].Workspace != "/workspace1" {
		t.Errorf("Workspace错误: %s", entities[0].Workspace)
	}
}

func TestMergeKnowledgeNodeIDs(t *testing.T) {
	a := &KnowledgeNodeIDs{
		EntityIDs: []string{"e1", "e2"},
		EventIDs:  []string{"ev1"},
	}
	b := &KnowledgeNodeIDs{
		EntityIDs: []string{"e2", "e3"}, // e2重复
		EventIDs:  []string{"ev2"},
	}

	merged := MergeKnowledgeNodeIDs(a, b)

	if len(merged.EntityIDs) != 3 {
		t.Errorf("期望3个EntityID (去重后), 实际: %d", len(merged.EntityIDs))
	}

	if len(merged.EventIDs) != 2 {
		t.Errorf("期望2个EventID, 实际: %d", len(merged.EventIDs))
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		uuid     string
		expected bool
	}{
		{"123e4567-e89b-12d3-a456-426614174000", true},
		{"123e4567-e89b-12d3-a456-42661417400", false},   // 太短
		{"123e4567-e89b-12d3-a456-4266141740001", false}, // 太长
		{"not-a-uuid", false},
		{"", false},
	}

	for _, test := range tests {
		result := ValidateUUID(test.uuid)
		if result != test.expected {
			t.Errorf("ValidateUUID(%s) = %v, 期望 %v", test.uuid, result, test.expected)
		}
	}
}
