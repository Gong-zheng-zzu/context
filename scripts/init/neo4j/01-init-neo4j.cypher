// Context-Keeper Neo4j 初始化脚本
// 用于知识图谱存储和关系分析

// ============================================
// 1. 创建约束（Constraints）
// ============================================

// 用户节点唯一性约束
CREATE CONSTRAINT user_id_unique IF NOT EXISTS
FOR (u:User) REQUIRE u.userId IS UNIQUE;

// 会话节点唯一性约束
CREATE CONSTRAINT session_id_unique IF NOT EXISTS
FOR (s:Session) REQUIRE s.sessionId IS UNIQUE;

// 工作空间节点唯一性约束
CREATE CONSTRAINT workspace_id_unique IF NOT EXISTS
FOR (w:Workspace) REQUIRE w.workspaceId IS UNIQUE;

// 概念节点唯一性约束
CREATE CONSTRAINT concept_id_unique IF NOT EXISTS
FOR (c:Concept) REQUIRE c.conceptId IS UNIQUE;

// 实体节点唯一性约束
CREATE CONSTRAINT entity_id_unique IF NOT EXISTS
FOR (e:Entity) REQUIRE e.entityId IS UNIQUE;

// 代码实体节点唯一性约束
CREATE CONSTRAINT code_entity_id_unique IF NOT EXISTS
FOR (ce:CodeEntity) REQUIRE ce.entityId IS UNIQUE;

// 决策节点唯一性约束
CREATE CONSTRAINT decision_id_unique IF NOT EXISTS
FOR (d:Decision) REQUIRE d.decisionId IS UNIQUE;

// 主题节点唯一性约束
CREATE CONSTRAINT topic_id_unique IF NOT EXISTS
FOR (t:Topic) REQUIRE t.topicId IS UNIQUE;

// ============================================
// 2. 创建索引（Indexes）
// ============================================

// 用户相关索引
CREATE INDEX user_created_at IF NOT EXISTS
FOR (u:User) ON (u.createdAt);

// 会话相关索引
CREATE INDEX session_created_at IF NOT EXISTS
FOR (s:Session) ON (s.createdAt);

CREATE INDEX session_updated_at IF NOT EXISTS
FOR (s:Session) ON (s.updatedAt);

CREATE INDEX session_user_id IF NOT EXISTS
FOR (s:Session) ON (s.userId);

// 工作空间相关索引
CREATE INDEX workspace_name IF NOT EXISTS
FOR (w:Workspace) ON (w.name);

CREATE INDEX workspace_created_at IF NOT EXISTS
FOR (w:Workspace) ON (w.createdAt);

// 概念相关索引
CREATE INDEX concept_name IF NOT EXISTS
FOR (c:Concept) ON (c.name);

CREATE INDEX concept_category IF NOT EXISTS
FOR (c:Concept) ON (c.category);

CREATE INDEX concept_importance IF NOT EXISTS
FOR (c:Concept) ON (c.importance);

// 实体相关索引
CREATE INDEX entity_type IF NOT EXISTS
FOR (e:Entity) ON (e.type);

CREATE INDEX entity_name IF NOT EXISTS
FOR (e:Entity) ON (e.name);

CREATE INDEX entity_created_at IF NOT EXISTS
FOR (e:Entity) ON (e.createdAt);

// 代码实体相关索引
CREATE INDEX code_entity_type IF NOT EXISTS
FOR (ce:CodeEntity) ON (ce.type);

CREATE INDEX code_entity_name IF NOT EXISTS
FOR (ce:CodeEntity) ON (ce.name);

CREATE INDEX code_entity_file_path IF NOT EXISTS
FOR (ce:CodeEntity) ON (ce.filePath);

// 决策相关索引
CREATE INDEX decision_created_at IF NOT EXISTS
FOR (d:Decision) ON (d.createdAt);

CREATE INDEX decision_importance IF NOT EXISTS
FOR (d:Decision) ON (d.importance);

// 主题相关索引
CREATE INDEX topic_name IF NOT EXISTS
FOR (t:Topic) ON (t.name);

CREATE INDEX topic_created_at IF NOT EXISTS
FOR (t:Topic) ON (t.createdAt);

CREATE INDEX topic_importance IF NOT EXISTS
FOR (t:Topic) ON (t.importance);

// ============================================
// 3. 创建全文搜索索引
// ============================================

// 概念全文搜索
CREATE FULLTEXT INDEX concept_fulltext IF NOT EXISTS
FOR (c:Concept) ON EACH [c.name, c.description];

// 实体全文搜索
CREATE FULLTEXT INDEX entity_fulltext IF NOT EXISTS
FOR (e:Entity) ON EACH [e.name, e.description];

// 代码实体全文搜索
CREATE FULLTEXT INDEX code_entity_fulltext IF NOT EXISTS
FOR (ce:CodeEntity) ON EACH [ce.name, ce.description, ce.filePath];

// 决策全文搜索
CREATE FULLTEXT INDEX decision_fulltext IF NOT EXISTS
FOR (d:Decision) ON EACH [d.title, d.description, d.rationale];

// 主题全文搜索
CREATE FULLTEXT INDEX topic_fulltext IF NOT EXISTS
FOR (t:Topic) ON EACH [t.name, t.description];

// ============================================
// 4. 创建示例数据（可选，用于测试）
// ============================================

// 创建系统用户
MERGE (u:User {userId: 'system'})
SET u.username = 'System',
    u.email = 'system@context-keeper.local',
    u.createdAt = datetime(),
    u.updatedAt = datetime();

// 创建默认工作空间
MERGE (w:Workspace {workspaceId: 'default'})
SET w.name = 'Default Workspace',
    w.description = 'Default workspace for Context-Keeper',
    w.createdAt = datetime(),
    w.updatedAt = datetime();

// 关联系统用户和默认工作空间
MATCH (u:User {userId: 'system'})
MATCH (w:Workspace {workspaceId: 'default'})
MERGE (u)-[r:OWNS]->(w)
SET r.createdAt = datetime();

// ============================================
// 5. 输出初始化信息
// ============================================

RETURN 'Neo4j initialization completed successfully!' AS status,
       'Created constraints, indexes, and sample data' AS details;
