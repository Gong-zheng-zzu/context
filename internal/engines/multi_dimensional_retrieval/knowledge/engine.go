package knowledge

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Neo4jEngine Neo4j知识图谱检索引擎
type Neo4jEngine struct {
	driver neo4j.DriverWithContext
	config *Neo4jConfig
}

// Neo4jConfig Neo4j配置
type Neo4jConfig struct {
	URI      string `json:"uri"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`

	// 连接池配置
	MaxConnectionPoolSize   int           `json:"max_connection_pool_size"`
	ConnectionTimeout       time.Duration `json:"connection_timeout"`
	MaxTransactionRetryTime time.Duration `json:"max_transaction_retry_time"`
}

// NewNeo4jEngine 创建Neo4j引擎
func NewNeo4jEngine(config *Neo4jConfig) (*Neo4jEngine, error) {
	if config == nil {
		return nil, fmt.Errorf("Neo4j配置不能为空，请使用统一配置管理器加载配置")
	}

	// 创建驱动
	driver, err := neo4j.NewDriverWithContext(
		config.URI,
		neo4j.BasicAuth(config.Username, config.Password, ""),
		func(c *neo4j.Config) {
			c.MaxConnectionPoolSize = config.MaxConnectionPoolSize
			c.ConnectionAcquisitionTimeout = config.ConnectionTimeout
			c.MaxTransactionRetryTime = config.MaxTransactionRetryTime
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建Neo4j驱动失败: %w", err)
	}

	engine := &Neo4jEngine{
		driver: driver,
		config: config,
	}

	// 验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := engine.verifyConnection(ctx); err != nil {
		return nil, fmt.Errorf("Neo4j连接验证失败: %w", err)
	}

	// 初始化图谱结构
	if err := engine.initializeGraph(ctx); err != nil {
		return nil, fmt.Errorf("初始化图谱结构失败: %w", err)
	}

	log.Printf("✅ Neo4j引擎初始化成功 - 数据库: %s", config.Database)
	return engine, nil
}

// verifyConnection 验证连接
func (engine *Neo4jEngine) verifyConnection(ctx context.Context) error {
	return engine.driver.VerifyConnectivity(ctx)
}

// initializeGraph 初始化图谱结构
func (engine *Neo4jEngine) initializeGraph(ctx context.Context) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	// 创建约束和索引
	constraints := []string{
		// 概念节点唯一性约束
		"CREATE CONSTRAINT concept_name_unique IF NOT EXISTS FOR (c:Concept) REQUIRE c.name IS UNIQUE",

		// 技术节点唯一性约束
		"CREATE CONSTRAINT technology_name_unique IF NOT EXISTS FOR (t:Technology) REQUIRE t.name IS UNIQUE",

		// 项目节点唯一性约束
		"CREATE CONSTRAINT project_name_unique IF NOT EXISTS FOR (p:Project) REQUIRE p.name IS UNIQUE",

		// 用户节点唯一性约束
		"CREATE CONSTRAINT user_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE",

		// 🆕 新增：Entity节点UUID唯一性约束
		"CREATE CONSTRAINT entity_id_unique IF NOT EXISTS FOR (e:Entity) REQUIRE e.id IS UNIQUE",

		// 🆕 新增：Event节点UUID唯一性约束
		"CREATE CONSTRAINT event_id_unique IF NOT EXISTS FOR (ev:Event) REQUIRE ev.id IS UNIQUE",

		// 🆕 新增：Solution节点UUID唯一性约束
		"CREATE CONSTRAINT solution_id_unique IF NOT EXISTS FOR (s:Solution) REQUIRE s.id IS UNIQUE",

		// 🆕 新增：Feature节点UUID唯一性约束
		"CREATE CONSTRAINT feature_id_unique IF NOT EXISTS FOR (f:Feature) REQUIRE f.id IS UNIQUE",
	}

	for _, constraint := range constraints {
		_, err := session.Run(ctx, constraint, nil)
		if err != nil {
			log.Printf("⚠️ 创建约束失败 (可能已存在): %v", err)
		}
	}

	// 创建索引
	indexes := []string{
		"CREATE INDEX concept_category_idx IF NOT EXISTS FOR (c:Concept) ON (c.category)",
		"CREATE INDEX technology_type_idx IF NOT EXISTS FOR (t:Technology) ON (t.type)",
		"CREATE INDEX project_domain_idx IF NOT EXISTS FOR (p:Project) ON (p.domain)",
		"CREATE FULLTEXT INDEX concept_search_idx IF NOT EXISTS FOR (c:Concept) ON EACH [c.name, c.description, c.keywords]",
		"CREATE FULLTEXT INDEX technology_search_idx IF NOT EXISTS FOR (t:Technology) ON EACH [t.name, t.description, t.keywords]",

		// 🆕 新增：Entity索引（按类型、工作空间查询）
		"CREATE INDEX entity_type_idx IF NOT EXISTS FOR (e:Entity) ON (e.type)",
		"CREATE INDEX entity_workspace_idx IF NOT EXISTS FOR (e:Entity) ON (e.workspace)",
		"CREATE INDEX entity_name_idx IF NOT EXISTS FOR (e:Entity) ON (e.name)",

		// 🆕 新增：Event索引（按类型、工作空间查询）
		"CREATE INDEX event_type_idx IF NOT EXISTS FOR (ev:Event) ON (ev.type)",
		"CREATE INDEX event_workspace_idx IF NOT EXISTS FOR (ev:Event) ON (ev.workspace)",

		// 🆕 新增：Solution索引（按类型、工作空间查询）
		"CREATE INDEX solution_type_idx IF NOT EXISTS FOR (s:Solution) ON (s.type)",
		"CREATE INDEX solution_workspace_idx IF NOT EXISTS FOR (s:Solution) ON (s.workspace)",

		// 🆕 新增：Feature索引
		"CREATE INDEX feature_workspace_idx IF NOT EXISTS FOR (f:Feature) ON (f.workspace)",

		// 🆕 新增：全文搜索索引
		"CREATE FULLTEXT INDEX entity_search_idx IF NOT EXISTS FOR (e:Entity) ON EACH [e.name, e.description]",
		"CREATE FULLTEXT INDEX event_search_idx IF NOT EXISTS FOR (ev:Event) ON EACH [ev.name, ev.description]",
		"CREATE FULLTEXT INDEX solution_search_idx IF NOT EXISTS FOR (s:Solution) ON EACH [s.name, s.description]",
	}

	for _, index := range indexes {
		_, err := session.Run(ctx, index, nil)
		if err != nil {
			log.Printf("⚠️ 创建索引失败 (可能已存在): %v", err)
		}
	}

	log.Printf("✅ Neo4j图谱结构初始化完成")
	return nil
}

// CreateConcept 创建概念节点
func (engine *Neo4jEngine) CreateConcept(ctx context.Context, concept *Concept) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (c:Concept {name: $name})
		SET c.description = $description,
		    c.category = $category,
		    c.keywords = $keywords,
		    c.importance = $importance,
		    c.created_at = datetime(),
		    c.updated_at = datetime()
		RETURN c.name as name`

	parameters := map[string]interface{}{
		"name":        concept.Name,
		"description": concept.Description,
		"category":    concept.Category,
		"keywords":    concept.Keywords,
		"importance":  concept.Importance,
	}

	result, err := session.Run(ctx, query, parameters)
	if err != nil {
		return fmt.Errorf("创建概念节点失败: %w", err)
	}

	if result.Next(ctx) {
		name, _ := result.Record().Get("name")
		log.Printf("✅ 创建概念节点: %s", name)
	}

	return result.Err()
}

// CreateTechnology 创建技术节点
func (engine *Neo4jEngine) CreateTechnology(ctx context.Context, tech *Technology) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (t:Technology {name: $name})
		SET t.description = $description,
		    t.type = $type,
		    t.version = $version,
		    t.keywords = $keywords,
		    t.popularity = $popularity,
		    t.created_at = datetime(),
		    t.updated_at = datetime()
		RETURN t.name as name`

	parameters := map[string]interface{}{
		"name":        tech.Name,
		"description": tech.Description,
		"type":        tech.Type,
		"version":     tech.Version,
		"keywords":    tech.Keywords,
		"popularity":  tech.Popularity,
	}

	result, err := session.Run(ctx, query, parameters)
	if err != nil {
		return fmt.Errorf("创建技术节点失败: %w", err)
	}

	if result.Next(ctx) {
		name, _ := result.Record().Get("name")
		log.Printf("✅ 创建技术节点: %s", name)
	}

	return result.Err()
}

// CreateRelationship 创建关系
func (engine *Neo4jEngine) CreateRelationship(ctx context.Context, rel *Relationship) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
		MATCH (from {name: $from_name})
		MATCH (to {name: $to_name})
		MERGE (from)-[r:%s]->(to)
		SET r.strength = $strength,
		    r.description = $description,
		    r.created_at = datetime(),
		    r.updated_at = datetime()
		RETURN type(r) as relationship_type`, rel.Type)

	parameters := map[string]interface{}{
		"from_name":   rel.FromName,
		"to_name":     rel.ToName,
		"strength":    rel.Strength,
		"description": rel.Description,
	}

	result, err := session.Run(ctx, query, parameters)
	if err != nil {
		return fmt.Errorf("创建关系失败: %w", err)
	}

	if result.Next(ctx) {
		relType, _ := result.Record().Get("relationship_type")
		log.Printf("✅ 创建关系: %s -[%s]-> %s", rel.FromName, relType, rel.ToName)
	}

	return result.Err()
}

// ExpandKnowledge 知识图谱扩展检索
func (engine *Neo4jEngine) ExpandKnowledge(ctx context.Context, query *KnowledgeQuery) (*KnowledgeResult, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	startTime := time.Now()

	// 构建Cypher查询
	cypherQuery, parameters := engine.buildKnowledgeQuery(query)

	log.Printf("🔍 执行知识图谱查询: %s", cypherQuery)

	// 执行查询
	result, err := session.Run(ctx, cypherQuery, parameters)
	if err != nil {
		return nil, fmt.Errorf("执行知识图谱查询失败: %w", err)
	}

	// 解析结果
	nodes := []KnowledgeNode{}
	relationships := []KnowledgeRelationship{}

	for result.Next(ctx) {
		record := result.Record()

		// 解析节点
		if nodeValue, found := record.Get("node"); found {
			if node, ok := nodeValue.(neo4j.Node); ok {
				knowledgeNode := engine.parseNode(node)
				nodes = append(nodes, knowledgeNode)
			}
		}

		// 解析关系
		if relValue, found := record.Get("relationship"); found {
			if rel, ok := relValue.(neo4j.Relationship); ok {
				knowledgeRel := engine.parseRelationship(rel)
				relationships = append(relationships, knowledgeRel)
			}
		}
	}

	if err = result.Err(); err != nil {
		return nil, fmt.Errorf("解析查询结果失败: %w", err)
	}

	duration := time.Since(startTime)

	return &KnowledgeResult{
		Nodes:         nodes,
		Relationships: relationships,
		Total:         len(nodes),
		Duration:      duration,
		Query:         query,
	}, nil
}

// buildKnowledgeQuery 构建知识图谱查询
func (engine *Neo4jEngine) buildKnowledgeQuery(query *KnowledgeQuery) (string, map[string]interface{}) {
	var cypherQuery string
	parameters := make(map[string]interface{})

	switch query.QueryType {
	case "expand":
		// 扩展查询：从给定概念开始，扩展相关概念
		cypherQuery = `
			MATCH (start {name: $start_concept})
			MATCH (start)-[r]-(related)
			WHERE r.strength >= $min_strength
			  AND start.user_id = $user_id
			  AND start.session_id = $session_id
			  AND start.workspace = $workspace_id
			  AND related.user_id = $user_id
			  AND related.session_id = $session_id
			  AND related.workspace = $workspace_id
			  AND r.user_id = $user_id
			  AND r.session_id = $session_id
			  AND r.workspace = $workspace_id
			  AND any(docID IN coalesce(related.doc_ids, []) WHERE trim(docID) <> '')
			RETURN DISTINCT related as node, r as relationship
			ORDER BY r.strength DESC
			LIMIT $limit`

		parameters["start_concept"] = query.StartConcepts[0]
		parameters["min_strength"] = query.MinStrength
		parameters["limit"] = query.Limit
		parameters["user_id"] = query.UserID
		parameters["session_id"] = query.SessionID
		parameters["workspace_id"] = query.WorkspaceID

	case "path":
		// 路径查询：查找两个概念之间的路径
		cypherQuery = `
			MATCH path = shortestPath((start {name: $start_concept})-[*..4]-(end {name: $end_concept}))
			WHERE all(node IN nodes(path) WHERE
				node.user_id = $user_id
				AND node.session_id = $session_id
				AND node.workspace = $workspace_id
				AND any(docID IN coalesce(node.doc_ids, []) WHERE trim(docID) <> '')
			)
			AND all(rel IN relationships(path) WHERE
				rel.user_id = $user_id
				AND rel.session_id = $session_id
				AND rel.workspace = $workspace_id
			)
			UNWIND nodes(path) as node
			UNWIND relationships(path) as relationship
			RETURN DISTINCT node, relationship
			LIMIT $limit`

		parameters["start_concept"] = query.StartConcepts[0]
		parameters["end_concept"] = query.EndConcepts[0]
		parameters["limit"] = query.Limit
		parameters["user_id"] = query.UserID
		parameters["session_id"] = query.SessionID
		parameters["workspace_id"] = query.WorkspaceID

	case "similarity":
		// 相似性查询：查找相似的概念
		cypherQuery = `
			MATCH (concept:Concept)
			WHERE concept.category IN $categories
			AND any(keyword IN $keywords WHERE keyword IN concept.keywords)
			AND concept.user_id = $user_id
			AND concept.session_id = $session_id
			AND concept.workspace = $workspace_id
			AND any(docID IN coalesce(concept.doc_ids, []) WHERE trim(docID) <> '')
			RETURN concept as node, null as relationship
			ORDER BY concept.importance DESC
			LIMIT $limit`

		parameters["categories"] = query.Categories
		parameters["keywords"] = query.Keywords
		parameters["limit"] = query.Limit
		parameters["user_id"] = query.UserID
		parameters["session_id"] = query.SessionID
		parameters["workspace_id"] = query.WorkspaceID

	default:
		// 默认全文搜索 - 使用entity_search_idx（Entity标签是实际存储的节点类型）
		cypherQuery = `
			MATCH (node)
			WHERE node.user_id = $user_id
			  AND node.session_id = $session_id
			  AND node.workspace = $workspace_id
			  AND any(docID IN coalesce(node.doc_ids, []) WHERE trim(docID) <> '')
			  AND any(term IN $search_terms WHERE
				toLower(coalesce(node.name, '')) CONTAINS toLower(term)
				OR toLower(coalesce(node.description, '')) CONTAINS toLower(term)
				OR any(keyword IN coalesce(node.keywords, []) WHERE toLower(keyword) CONTAINS toLower(term))
			)
			RETURN node, null as relationship, 1.0 as score
			ORDER BY node.importance DESC, node.updated_at DESC
			LIMIT $limit`

		parameters["search_terms"] = graphSearchTerms(query.SearchText, query.Keywords)
		parameters["limit"] = query.Limit
		parameters["user_id"] = query.UserID
		parameters["session_id"] = query.SessionID
		parameters["workspace_id"] = query.WorkspaceID
	}

	return cypherQuery, parameters
}

func graphSearchTerms(searchText string, keywords []string) []string {
	terms := make([]string, 0, len(keywords)+1)
	seen := make(map[string]struct{}, len(keywords)+1)
	for _, candidate := range append([]string{searchText}, keywords...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		key := strings.ToLower(candidate)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, candidate)
	}
	for _, fragment := range graphChineseSearchFragments(searchText) {
		key := strings.ToLower(fragment)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		terms = append(terms, fragment)
	}
	return terms
}

func graphChineseSearchFragments(text string) []string {
	fragments := make([]string, 0, 4)
	for _, run := range strings.FieldsFunc(text, func(r rune) bool {
		return r < '\u4e00' || r > '\u9fff'
	}) {
		runes := []rune(run)
		if len(runes) < 2 {
			continue
		}
		fragments = append(fragments, string(runes[:2]))
		if len(runes) >= 3 {
			fragments = append(fragments, string(runes[:3]))
		}
		if len(fragments) >= 4 {
			break
		}
	}
	return fragments
}

// parseNode 解析节点
func (engine *Neo4jEngine) parseNode(node neo4j.Node) KnowledgeNode {
	props := node.Props

	return KnowledgeNode{
		ID:          node.ElementId,
		Labels:      node.Labels,
		Name:        getStringProp(props, "name"),
		Description: getStringProp(props, "description"),
		Category:    getStringProp(props, "category"),
		Keywords:    getStringArrayProp(props, "keywords"),
		DocIDs:      getStringArrayProp(props, "doc_ids"),
		Properties:  props,
	}
}

// parseRelationship 解析关系
func (engine *Neo4jEngine) parseRelationship(rel neo4j.Relationship) KnowledgeRelationship {
	props := rel.Props

	return KnowledgeRelationship{
		ID:          rel.ElementId,
		Type:        rel.Type,
		StartNodeID: rel.StartElementId,
		EndNodeID:   rel.EndElementId,
		Strength:    getFloatProp(props, "strength"),
		Description: getStringProp(props, "description"),
		Properties:  props,
	}
}

// 辅助函数
func getStringProp(props map[string]interface{}, key string) string {
	if val, ok := props[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getStringArrayProp(props map[string]interface{}, key string) []string {
	if val, ok := props[key]; ok {
		switch arr := val.(type) {
		case []string:
			return append([]string(nil), arr...)
		case []interface{}:
			result := make([]string, len(arr))
			for i, v := range arr {
				if str, ok := v.(string); ok {
					result[i] = str
				}
			}
			return result
		}
	}
	return []string{}
}

func getFloatProp(props map[string]interface{}, key string) float64 {
	if val, ok := props[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
		if i, ok := val.(int64); ok {
			return float64(i)
		}
	}
	return 0.0
}

// HealthCheck 健康检查
func (engine *Neo4jEngine) HealthCheck(ctx context.Context) error {
	return engine.driver.VerifyConnectivity(ctx)
}

// CountSessionReplicaRecords returns session-scoped nodes and relationships for one owner.
func (engine *Neo4jEngine) CountSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: engine.config.Database})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		CALL {
			MATCH (n {user_id: $user_id, session_id: $session_id})
			RETURN count(n) AS nodes
		}
		CALL {
			MATCH ()-[r {user_id: $user_id, session_id: $session_id}]-()
			RETURN count(r) AS relationships
		}
		RETURN nodes + relationships AS total`, map[string]interface{}{
		"user_id": userID, "session_id": sessionID,
	})
	if err != nil {
		return 0, fmt.Errorf("count Neo4j session records: %w", err)
	}
	if !result.Next(ctx) {
		if err := result.Err(); err != nil {
			return 0, fmt.Errorf("read Neo4j session count: %w", err)
		}
		return 0, fmt.Errorf("read Neo4j session count: no result")
	}
	value, ok := result.Record().Get("total")
	if !ok {
		return 0, fmt.Errorf("read Neo4j session count: missing total")
	}
	count, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("read Neo4j session count: invalid total type %T", value)
	}
	return count, nil
}

// CountSessionDocumentRecords returns the read-only number of nodes and
// relationships that carry one document provenance value within an owner and
// session. The property checks avoid treating another tenant's shared concept
// node as evidence for this experiment.
func (engine *Neo4jEngine) CountSessionDocumentRecords(ctx context.Context, userID, sessionID, docID string) (int64, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: engine.config.Database})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		CALL {
			MATCH (n {user_id: $user_id, session_id: $session_id})
			WHERE $doc_id IN coalesce(n.doc_ids, [])
			RETURN count(n) AS nodes
		}
		CALL {
			MATCH ()-[r {user_id: $user_id, session_id: $session_id}]-()
			WHERE $doc_id IN coalesce(r.doc_ids, [])
			RETURN count(r) AS relationships
		}
		RETURN nodes + relationships AS total`, map[string]interface{}{
		"user_id": userID, "session_id": sessionID, "doc_id": docID,
	})
	if err != nil {
		return 0, fmt.Errorf("count Neo4j document records: %w", err)
	}
	if !result.Next(ctx) {
		if err := result.Err(); err != nil {
			return 0, fmt.Errorf("read Neo4j document count: %w", err)
		}
		return 0, fmt.Errorf("read Neo4j document count: no result")
	}
	value, ok := result.Record().Get("total")
	if !ok {
		return 0, fmt.Errorf("read Neo4j document count: missing total")
	}
	count, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("read Neo4j document count: invalid total type %T", value)
	}
	return count, nil
}

// DeleteSessionReplicaRecords removes only relationships and nodes scoped to this owner/session.
// It deliberately refuses to delete nodes still connected to another session's relationship.
func (engine *Neo4jEngine) DeleteSessionReplicaRecords(ctx context.Context, userID, sessionID string) (int64, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: engine.config.Database})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		OPTIONAL MATCH ()-[r {user_id: $user_id, session_id: $session_id}]-()
		WITH collect(r) AS relationships
		FOREACH (r IN relationships | DELETE r)
		WITH size(relationships) AS relationship_count
		OPTIONAL MATCH (n {user_id: $user_id, session_id: $session_id})
		WITH relationship_count, collect(n) AS nodes
		FOREACH (n IN nodes | DELETE n)
		RETURN relationship_count + size(nodes) AS deleted`, map[string]interface{}{
		"user_id": userID, "session_id": sessionID,
	})
	if err != nil {
		return 0, fmt.Errorf("delete Neo4j session records: %w", err)
	}
	if !result.Next(ctx) {
		if err := result.Err(); err != nil {
			return 0, fmt.Errorf("read Neo4j deleted count: %w", err)
		}
		return 0, fmt.Errorf("read Neo4j deleted count: no result")
	}
	value, ok := result.Record().Get("deleted")
	if !ok {
		return 0, fmt.Errorf("read Neo4j deleted count: missing deleted")
	}
	deleted, ok := value.(int64)
	if !ok {
		return 0, fmt.Errorf("read Neo4j deleted count: invalid deleted type %T", value)
	}
	return deleted, nil
}

// Close 关闭连接
func (engine *Neo4jEngine) Close(ctx context.Context) error {
	return engine.driver.Close(ctx)
}

func firstString(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ==================== 🆕 新增：知识节点CRUD方法 ====================

// UpsertEntity 创建或更新Entity节点（基于UUID MERGE）
func (engine *Neo4jEngine) UpsertEntity(ctx context.Context, entity *Entity) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (e:Entity {id: $id})
		ON CREATE SET e.created_at = datetime(), e.memory_ids = $memory_ids,
		    e.doc_ids = reduce(ids = [], docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		ON MATCH SET e.memory_ids =
		    CASE WHEN $memory_id IN coalesce(e.memory_ids, []) THEN e.memory_ids
		         ELSE coalesce(e.memory_ids, []) + $memory_id END,
		    e.doc_ids = reduce(ids = coalesce(e.doc_ids, []), docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		SET e.name = $name,
		    e.type = $type,
		    e.description = $description,
		    e.workspace = $workspace,
		    e.user_id = $user_id,
		    e.session_id = $session_id,
		    e.updated_at = datetime()
		RETURN e.id as id`

	memoryID := ""
	if len(entity.MemoryIDs) > 0 {
		memoryID = entity.MemoryIDs[0]
	}

	parameters := map[string]interface{}{
		"id":          entity.ID,
		"name":        entity.Name,
		"type":        entity.Type,
		"description": entity.Description,
		"workspace":   entity.Workspace,
		"memory_ids":  entity.MemoryIDs,
		"memory_id":   memoryID,
		"doc_ids":     entity.DocIDs,
		"doc_id":      firstString(entity.DocIDs),
		"user_id":     entity.UserID,
		"session_id":  entity.SessionID,
	}

	_, err := session.Run(ctx, query, parameters)
	return err
}

// UpsertEvent 创建或更新Event节点
func (engine *Neo4jEngine) UpsertEvent(ctx context.Context, event *Event) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (ev:Event {id: $id})
		ON CREATE SET ev.created_at = datetime(), ev.memory_ids = $memory_ids,
		    ev.doc_ids = reduce(ids = [], docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		ON MATCH SET ev.memory_ids =
		    CASE WHEN $memory_id IN coalesce(ev.memory_ids, []) THEN ev.memory_ids
		         ELSE coalesce(ev.memory_ids, []) + $memory_id END,
		    ev.doc_ids = reduce(ids = coalesce(ev.doc_ids, []), docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		SET ev.name = $name,
		    ev.type = $type,
		    ev.description = $description,
		    ev.workspace = $workspace,
		    ev.user_id = $user_id,
		    ev.session_id = $session_id,
		    ev.updated_at = datetime()
		RETURN ev.id as id`

	memoryID := ""
	if len(event.MemoryIDs) > 0 {
		memoryID = event.MemoryIDs[0]
	}

	parameters := map[string]interface{}{
		"id":          event.ID,
		"name":        event.Name,
		"type":        event.Type,
		"description": event.Description,
		"workspace":   event.Workspace,
		"memory_ids":  event.MemoryIDs,
		"memory_id":   memoryID,
		"doc_ids":     event.DocIDs,
		"doc_id":      firstString(event.DocIDs),
		"user_id":     event.UserID,
		"session_id":  event.SessionID,
	}

	_, err := session.Run(ctx, query, parameters)
	return err
}

// UpsertSolution 创建或更新Solution节点
func (engine *Neo4jEngine) UpsertSolution(ctx context.Context, solution *Solution) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (s:Solution {id: $id})
		ON CREATE SET s.created_at = datetime(), s.memory_ids = $memory_ids,
		    s.doc_ids = reduce(ids = [], docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		ON MATCH SET s.memory_ids =
		    CASE WHEN $memory_id IN coalesce(s.memory_ids, []) THEN s.memory_ids
		         ELSE coalesce(s.memory_ids, []) + $memory_id END,
		    s.doc_ids = reduce(ids = coalesce(s.doc_ids, []), docID IN coalesce($doc_ids, []) |
		        CASE WHEN trim(docID) = '' OR docID IN ids THEN ids ELSE ids + docID END)
		SET s.name = $name,
		    s.type = $type,
		    s.description = $description,
		    s.workspace = $workspace,
		    s.user_id = $user_id,
		    s.session_id = $session_id,
		    s.updated_at = datetime()
		RETURN s.id as id`

	memoryID := ""
	if len(solution.MemoryIDs) > 0 {
		memoryID = solution.MemoryIDs[0]
	}

	parameters := map[string]interface{}{
		"id":          solution.ID,
		"name":        solution.Name,
		"type":        solution.Type,
		"description": solution.Description,
		"workspace":   solution.Workspace,
		"memory_ids":  solution.MemoryIDs,
		"memory_id":   memoryID,
		"doc_ids":     solution.DocIDs,
		"doc_id":      firstString(solution.DocIDs),
		"user_id":     solution.UserID,
		"session_id":  solution.SessionID,
	}

	_, err := session.Run(ctx, query, parameters)
	return err
}

// CreateRelation 创建关系
func (engine *Neo4jEngine) CreateRelation(ctx context.Context, relation *Relation) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	// 使用动态关系类型
	query := fmt.Sprintf(`
		MATCH (source {id: $source_id})
		MATCH (target {id: $target_id})
		MERGE (source)-[r:%s]->(target)
		SET r.weight = $weight,
		    r.doc_ids = CASE WHEN $doc_id IN coalesce(r.doc_ids, []) THEN r.doc_ids ELSE coalesce(r.doc_ids, []) + $doc_id END,
		    r.user_id = $user_id,
		    r.session_id = $session_id,
		    r.workspace = $workspace,
		    r.created_at = datetime()
		RETURN type(r) as rel_type`, relation.Type)

	parameters := map[string]interface{}{
		"source_id":  relation.SourceID,
		"target_id":  relation.TargetID,
		"weight":     relation.Weight,
		"doc_id":     firstString(relation.DocIDs),
		"user_id":    relation.UserID,
		"session_id": relation.SessionID,
		"workspace":  relation.Workspace,
	}

	_, err := session.Run(ctx, query, parameters)
	return err
}

// AppendMemoryIDToEntity 追加MemoryID到Entity
func (engine *Neo4jEngine) AppendMemoryIDToEntity(ctx context.Context, entityID, memoryID string) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity {id: $entity_id})
		SET e.memory_ids =
		    CASE WHEN $memory_id IN e.memory_ids THEN e.memory_ids
		         ELSE e.memory_ids + $memory_id END,
		    e.updated_at = datetime()
		RETURN e.id as id`

	parameters := map[string]interface{}{
		"entity_id": entityID,
		"memory_id": memoryID,
	}

	_, err := session.Run(ctx, query, parameters)
	return err
}

// GetEntityByID 根据UUID获取Entity
func (engine *Neo4jEngine) GetEntityByID(ctx context.Context, entityID string) (*Entity, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity {id: $id})
		RETURN e.id as id, e.name as name, e.type as type,
		       e.description as description, e.workspace as workspace,
		       e.memory_ids as memory_ids`

	result, err := session.Run(ctx, query, map[string]interface{}{"id": entityID})
	if err != nil {
		return nil, err
	}

	if result.Next(ctx) {
		record := result.Record()
		entity := &Entity{
			ID:          getStringProp(record.AsMap(), "id"),
			Name:        getStringProp(record.AsMap(), "name"),
			Type:        getStringProp(record.AsMap(), "type"),
			Description: getStringProp(record.AsMap(), "description"),
			Workspace:   getStringProp(record.AsMap(), "workspace"),
			MemoryIDs:   getStringArrayProp(record.AsMap(), "memory_ids"),
		}
		return entity, nil
	}
	return nil, nil
}

// GetRelatedEntities 获取与指定Entity相关的实体
func (engine *Neo4jEngine) GetRelatedEntities(ctx context.Context, entityID string, depth int) ([]*Entity, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	query := fmt.Sprintf(`
		MATCH (start:Entity {id: $id})-[*1..%d]-(related:Entity)
		WHERE related.id <> $id
		RETURN DISTINCT related.id as id, related.name as name,
		       related.type as type, related.description as description,
		       related.workspace as workspace, related.memory_ids as memory_ids
		LIMIT 50`, depth)

	result, err := session.Run(ctx, query, map[string]interface{}{"id": entityID})
	if err != nil {
		return nil, err
	}

	var entities []*Entity
	for result.Next(ctx) {
		record := result.Record()
		entity := &Entity{
			ID:          getStringProp(record.AsMap(), "id"),
			Name:        getStringProp(record.AsMap(), "name"),
			Type:        getStringProp(record.AsMap(), "type"),
			Description: getStringProp(record.AsMap(), "description"),
			Workspace:   getStringProp(record.AsMap(), "workspace"),
			MemoryIDs:   getStringArrayProp(record.AsMap(), "memory_ids"),
		}
		entities = append(entities, entity)
	}
	return entities, result.Err()
}

// GetMemoryIDsByEntityIDs 根据Entity UUID列表获取关联的MemoryID列表
func (engine *Neo4jEngine) GetMemoryIDsByEntityIDs(ctx context.Context, entityIDs []string) ([]string, error) {
	if len(entityIDs) == 0 {
		return []string{}, nil
	}

	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity)
		WHERE e.id IN $entity_ids
		UNWIND e.memory_ids as memory_id
		RETURN DISTINCT memory_id`

	result, err := session.Run(ctx, query, map[string]interface{}{"entity_ids": entityIDs})
	if err != nil {
		return nil, err
	}

	var memoryIDs []string
	for result.Next(ctx) {
		if memoryID, ok := result.Record().Get("memory_id"); ok {
			if str, ok := memoryID.(string); ok {
				memoryIDs = append(memoryIDs, str)
			}
		}
	}
	return memoryIDs, result.Err()
}

// SearchEntitiesByName 根据名称搜索Entity
func (engine *Neo4jEngine) SearchEntitiesByName(ctx context.Context, name, workspace string, limit int) ([]*Entity, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	if limit <= 0 {
		limit = 20
	}

	query := `
		CALL db.index.fulltext.queryNodes('entity_search_idx', $search_text)
		YIELD node, score
		WHERE node.workspace = $workspace
		RETURN node.id as id, node.name as name, node.type as type,
		       node.description as description, node.workspace as workspace,
		       node.memory_ids as memory_ids, score
		ORDER BY score DESC
		LIMIT $limit`

	parameters := map[string]interface{}{
		"search_text": name,
		"workspace":   workspace,
		"limit":       limit,
	}

	result, err := session.Run(ctx, query, parameters)
	if err != nil {
		return nil, err
	}

	var entities []*Entity
	for result.Next(ctx) {
		record := result.Record()
		entity := &Entity{
			ID:          getStringProp(record.AsMap(), "id"),
			Name:        getStringProp(record.AsMap(), "name"),
			Type:        getStringProp(record.AsMap(), "type"),
			Description: getStringProp(record.AsMap(), "description"),
			Workspace:   getStringProp(record.AsMap(), "workspace"),
			MemoryIDs:   getStringArrayProp(record.AsMap(), "memory_ids"),
		}
		entities = append(entities, entity)
	}
	return entities, result.Err()
}

// BatchUpsertEntities 批量Upsert Entity
func (engine *Neo4jEngine) BatchUpsertEntities(ctx context.Context, entities []*Entity, memoryID string) error {
	if len(entities) == 0 {
		return nil
	}

	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	for _, entity := range entities {
		entity.MemoryIDs = []string{memoryID}
		if err := engine.UpsertEntity(ctx, entity); err != nil {
			log.Printf("⚠️ Upsert Entity失败: %s, error: %v", entity.Name, err)
			// 继续处理其他Entity
		}
	}
	return nil
}

// BatchUpsertEvents 批量Upsert Event
func (engine *Neo4jEngine) BatchUpsertEvents(ctx context.Context, events []*Event, memoryID string) error {
	if len(events) == 0 {
		return nil
	}

	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	for _, event := range events {
		event.MemoryIDs = []string{memoryID}
		if err := engine.UpsertEvent(ctx, event); err != nil {
			log.Printf("⚠️ Upsert Event失败: %s, error: %v", event.Name, err)
		}
	}
	return nil
}

// BatchUpsertSolutions 批量Upsert Solution
func (engine *Neo4jEngine) BatchUpsertSolutions(ctx context.Context, solutions []*Solution, memoryID string) error {
	if len(solutions) == 0 {
		return nil
	}

	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	for _, solution := range solutions {
		solution.MemoryIDs = []string{memoryID}
		if err := engine.UpsertSolution(ctx, solution); err != nil {
			log.Printf("⚠️ Upsert Solution失败: %s, error: %v", solution.Name, err)
		}
	}
	return nil
}

// ==================== 🆕 新增：AppendMemoryID方法（Event/Solution） ====================

// AppendMemoryIDToEvent 追加MemoryID到Event节点（MERGE by UUID，原子操作）
func (engine *Neo4jEngine) AppendMemoryIDToEvent(ctx context.Context, event *Event, memoryID string) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (ev:Event {id: $id})
		ON CREATE SET
			ev.name = $name,
			ev.type = $type,
			ev.description = $description,
			ev.workspace = $workspace,
			ev.memory_ids = [$memoryID],
			ev.created_at = datetime(),
			ev.updated_at = datetime()
		ON MATCH SET ev.memory_ids = CASE
			WHEN $memoryID IN ev.memory_ids THEN ev.memory_ids
			ELSE ev.memory_ids + [$memoryID]
		END,
		ev.updated_at = datetime()
	`
	_, err := session.Run(ctx, query, map[string]interface{}{
		"id":          event.ID,
		"name":        event.Name,
		"type":        event.Type,
		"description": event.Description,
		"workspace":   event.Workspace,
		"memoryID":    memoryID,
	})
	if err != nil {
		return fmt.Errorf("append memoryID to event failed: %w", err)
	}
	log.Printf("✅ AppendMemoryIDToEvent成功: %s (ID: %s, MemoryID: %s)", event.Name, event.ID, memoryID)
	return nil
}

// AppendMemoryIDToSolution 追加MemoryID到Solution节点（MERGE by UUID，原子操作）
func (engine *Neo4jEngine) AppendMemoryIDToSolution(ctx context.Context, solution *Solution, memoryID string) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MERGE (s:Solution {id: $id})
		ON CREATE SET
			s.name = $name,
			s.type = $type,
			s.description = $description,
			s.workspace = $workspace,
			s.memory_ids = [$memoryID],
			s.created_at = datetime(),
			s.updated_at = datetime()
		ON MATCH SET s.memory_ids = CASE
			WHEN $memoryID IN s.memory_ids THEN s.memory_ids
			ELSE s.memory_ids + [$memoryID]
		END,
		s.updated_at = datetime()
	`
	_, err := session.Run(ctx, query, map[string]interface{}{
		"id":          solution.ID,
		"name":        solution.Name,
		"type":        solution.Type,
		"description": solution.Description,
		"workspace":   solution.Workspace,
		"memoryID":    memoryID,
	})
	if err != nil {
		return fmt.Errorf("append memoryID to solution failed: %w", err)
	}
	log.Printf("✅ AppendMemoryIDToSolution成功: %s (ID: %s, MemoryID: %s)", solution.Name, solution.ID, memoryID)
	return nil
}

// ==================== 🆕 新增：GetRelated方法（Events/Solutions） ====================

// GetRelatedEvents 获取与指定Event相关的事件（支持多跳遍历）
func (engine *Neo4jEngine) GetRelatedEvents(ctx context.Context, eventID string, depth int) ([]*Event, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	if depth <= 0 {
		depth = 1
	}
	if depth > 2 {
		depth = 2 // 限制最大深度为2，防止性能问题
	}

	query := fmt.Sprintf(`
		MATCH (start:Event {id: $event_id})
		MATCH path = (start)-[*1..%d]-(related:Event)
		WHERE related <> start
		RETURN DISTINCT related.id AS id, related.name AS name, related.type AS type,
			   related.description AS description, related.workspace AS workspace,
			   related.memory_ids AS memory_ids
	`, depth)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"event_id": eventID,
	})
	if err != nil {
		return nil, fmt.Errorf("get related events failed: %w", err)
	}

	var events []*Event
	for result.Next(ctx) {
		record := result.Record()
		event := &Event{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Type:        getStringFromRecord(record, "type"),
			Description: getStringFromRecord(record, "description"),
			Workspace:   getStringFromRecord(record, "workspace"),
			MemoryIDs:   getStringSliceFromRecord(record, "memory_ids"),
		}
		events = append(events, event)
	}

	log.Printf("📊 GetRelatedEvents完成: 从%s出发，深度%d，找到%d个相关Event", eventID, depth, len(events))
	return events, nil
}

// GetRelatedSolutions 获取与指定问题相关的解决方案
func (engine *Neo4jEngine) GetRelatedSolutions(ctx context.Context, eventID string) ([]*Solution, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	// 查找SOLVES或PREVENTS关系指向该Event的Solution
	query := `
		MATCH (s:Solution)-[:SOLVES|PREVENTS]->(ev:Event {id: $event_id})
		RETURN s.id AS id, s.name AS name, s.type AS type,
			   s.description AS description, s.workspace AS workspace,
			   s.memory_ids AS memory_ids
	`

	result, err := session.Run(ctx, query, map[string]interface{}{
		"event_id": eventID,
	})
	if err != nil {
		return nil, fmt.Errorf("get related solutions failed: %w", err)
	}

	var solutions []*Solution
	for result.Next(ctx) {
		record := result.Record()
		solution := &Solution{
			ID:          getStringFromRecord(record, "id"),
			Name:        getStringFromRecord(record, "name"),
			Type:        getStringFromRecord(record, "type"),
			Description: getStringFromRecord(record, "description"),
			Workspace:   getStringFromRecord(record, "workspace"),
			MemoryIDs:   getStringSliceFromRecord(record, "memory_ids"),
		}
		solutions = append(solutions, solution)
	}

	log.Printf("📊 GetRelatedSolutions完成: Event %s 有%d个相关Solution", eventID, len(solutions))
	return solutions, nil
}

// ==================== 辅助函数 ====================

// getStringFromRecord 从记录中安全获取字符串
func getStringFromRecord(record *neo4j.Record, key string) string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// getStringSliceFromRecord 从记录中安全获取字符串切片
func getStringSliceFromRecord(record *neo4j.Record, key string) []string {
	val, ok := record.Get(key)
	if !ok || val == nil {
		return []string{}
	}
	if slice, ok := val.([]interface{}); ok {
		result := make([]string, 0, len(slice))
		for _, v := range slice {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

// ==================== 🆕 新增：因果推理所需方法 ====================

// GetRelations 获取指定节点的所有出边关系
func (engine *Neo4jEngine) GetRelations(ctx context.Context, entityID string) ([]*Relation, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (source {id: $entity_id})-[r]->(target)
		RETURN r, target.id as target_id, type(r) as rel_type
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"entity_id": entityID})
	if err != nil {
		return nil, fmt.Errorf("get relations failed: %w", err)
	}

	var relations []*Relation
	for result.Next(ctx) {
		record := result.Record()
		targetID := getStringFromRecord(record, "target_id")
		relType := getStringFromRecord(record, "rel_type")

		// 获取关系的weight属性
		weight := 1.0
		if relValue, ok := record.Get("r"); ok {
			if rel, ok := relValue.(neo4j.Relationship); ok {
				if w, exists := rel.Props["weight"]; exists {
					if wFloat, ok := w.(float64); ok {
						weight = wFloat
					}
				}
			}
		}

		relation := &Relation{
			SourceID:  entityID,
			TargetID:  targetID,
			Type:      relType,
			Weight:    weight,
			CreatedAt: time.Now(),
		}
		relations = append(relations, relation)
	}

	return relations, result.Err()
}

// GetIncomingRelations 获取指定节点的所有入边关系（指定类型）
func (engine *Neo4jEngine) GetIncomingRelations(ctx context.Context, entityID, relationType string) ([]*Relation, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
		MATCH (source)-[r:%s]->(target {id: $entity_id})
		RETURN source.id as source_id, type(r) as rel_type, r
	`, relationType)

	result, err := session.Run(ctx, query, map[string]interface{}{"entity_id": entityID})
	if err != nil {
		return nil, fmt.Errorf("get incoming relations failed: %w", err)
	}

	var relations []*Relation
	for result.Next(ctx) {
		record := result.Record()
		sourceID := getStringFromRecord(record, "source_id")
		relType := getStringFromRecord(record, "rel_type")

		// 获取关系的weight属性
		weight := 1.0
		if relValue, ok := record.Get("r"); ok {
			if rel, ok := relValue.(neo4j.Relationship); ok {
				if w, exists := rel.Props["weight"]; exists {
					if wFloat, ok := w.(float64); ok {
						weight = wFloat
					}
				}
			}
		}

		relation := &Relation{
			SourceID:  sourceID,
			TargetID:  entityID,
			Type:      relType,
			Weight:    weight,
			CreatedAt: time.Now(),
		}
		relations = append(relations, relation)
	}

	return relations, result.Err()
}

// Path 表示图中的路径
type Path struct {
	Nodes     []*Entity
	Relations []*Relation
}

// FindPaths 查找两个节点之间的路径（最多maxDepth跳）
func (engine *Neo4jEngine) FindPaths(ctx context.Context, fromName, toName string, maxDepth int) ([]Path, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	if maxDepth <= 0 {
		maxDepth = 4
	}

	query := fmt.Sprintf(`
		MATCH (start {name: $from_name})
		MATCH (end {name: $to_name})
		MATCH path = (start)-[*1..%d]->(end)
		RETURN path
		LIMIT 10
	`, maxDepth)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"from_name": fromName,
		"to_name":   toName,
	})
	if err != nil {
		return nil, fmt.Errorf("find paths failed: %w", err)
	}

	var paths []Path
	for result.Next(ctx) {
		record := result.Record()
		if pathValue, ok := record.Get("path"); ok {
			if neo4jPath, ok := pathValue.(neo4j.Path); ok {
				path := engine.parsePath(neo4jPath)
				paths = append(paths, path)
			}
		}
	}

	return paths, result.Err()
}

// parsePath 解析Neo4j路径为Path结构
func (engine *Neo4jEngine) parsePath(neo4jPath neo4j.Path) Path {
	path := Path{
		Nodes:     make([]*Entity, 0),
		Relations: make([]*Relation, 0),
	}

	// 解析节点
	for _, node := range neo4jPath.Nodes {
		entity := &Entity{
			ID:          getStringProp(node.Props, "id"),
			Name:        getStringProp(node.Props, "name"),
			Type:        getStringProp(node.Props, "type"),
			Description: getStringProp(node.Props, "description"),
			Workspace:   getStringProp(node.Props, "workspace"),
		}
		path.Nodes = append(path.Nodes, entity)
	}

	// 解析关系
	for _, rel := range neo4jPath.Relationships {
		relation := &Relation{
			SourceID: rel.StartElementId,
			TargetID: rel.EndElementId,
			Type:     rel.Type,
			Weight:   getFloatProp(rel.Props, "weight"),
		}
		path.Relations = append(path.Relations, relation)
	}

	return path
}

// DeleteRelation 删除指定类型的关系
func (engine *Neo4jEngine) DeleteRelation(ctx context.Context, sourceID, targetID, relationType string) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
		MATCH (source {id: $source_id})-[r:%s]->(target {id: $target_id})
		DELETE r
		RETURN count(r) as deleted_count
	`, relationType)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"source_id": sourceID,
		"target_id": targetID,
	})
	if err != nil {
		return fmt.Errorf("delete relation failed: %w", err)
	}

	if result.Next(ctx) {
		deletedCount := int64(0)
		if val, ok := result.Record().Get("deleted_count"); ok {
			if count, ok := val.(int64); ok {
				deletedCount = count
			}
		}
		log.Printf("✅ DeleteRelation完成: 删除了%d个关系", deletedCount)
	}

	return result.Err()
}

// UpdateRelation 更新关系属性
func (engine *Neo4jEngine) UpdateRelation(ctx context.Context, relation *Relation) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
		MATCH (source {id: $source_id})-[r:%s]->(target {id: $target_id})
		SET r.weight = $weight,
		    r.updated_at = datetime()
		RETURN count(r) as updated_count
	`, relation.Type)

	result, err := session.Run(ctx, query, map[string]interface{}{
		"source_id": relation.SourceID,
		"target_id": relation.TargetID,
		"weight":    relation.Weight,
	})
	if err != nil {
		return fmt.Errorf("update relation failed: %w", err)
	}

	if result.Next(ctx) {
		updatedCount := int64(0)
		if val, ok := result.Record().Get("updated_count"); ok {
			if count, ok := val.(int64); ok {
				updatedCount = count
			}
		}
		log.Printf("✅ UpdateRelation完成: 更新了%d个关系", updatedCount)
	}

	return result.Err()
}

// DeleteEntity 删除实体节点
func (engine *Neo4jEngine) DeleteEntity(ctx context.Context, entityID string) error {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (e {id: $entity_id})
		DETACH DELETE e
		RETURN count(e) as deleted_count
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"entity_id": entityID})
	if err != nil {
		return fmt.Errorf("delete entity failed: %w", err)
	}

	if result.Next(ctx) {
		deletedCount := int64(0)
		if val, ok := result.Record().Get("deleted_count"); ok {
			if count, ok := val.(int64); ok {
				deletedCount = count
			}
		}
		log.Printf("✅ DeleteEntity完成: 删除了%d个节点", deletedCount)
	}

	return result.Err()
}

// CountEntitiesByWorkspace 统计指定工作空间的实体数量
func (engine *Neo4jEngine) CountEntitiesByWorkspace(ctx context.Context, workspace string) (int64, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := `
		MATCH (e:Entity {workspace: $workspace})
		RETURN count(e) as entity_count
	`

	result, err := session.Run(ctx, query, map[string]interface{}{"workspace": workspace})
	if err != nil {
		return 0, fmt.Errorf("count entities failed: %w", err)
	}

	if result.Next(ctx) {
		if val, ok := result.Record().Get("entity_count"); ok {
			if count, ok := val.(int64); ok {
				return count, nil
			}
		}
	}

	return 0, result.Err()
}

// CountRelationsByType 统计指定类型的关系数量
func (engine *Neo4jEngine) CountRelationsByType(ctx context.Context, relationType string) (int64, error) {
	session := engine.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: engine.config.Database,
	})
	defer session.Close(ctx)

	query := fmt.Sprintf(`
		MATCH ()-[r:%s]->()
		RETURN count(r) as relation_count
	`, relationType)

	result, err := session.Run(ctx, query, nil)
	if err != nil {
		return 0, fmt.Errorf("count relations failed: %w", err)
	}

	if result.Next(ctx) {
		if val, ok := result.Record().Get("relation_count"); ok {
			if count, ok := val.(int64); ok {
				return count, nil
			}
		}
	}

	return 0, result.Err()
}
