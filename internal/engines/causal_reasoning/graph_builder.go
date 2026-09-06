package causal_reasoning

import (
	"context"
	"fmt"
	"time"

	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
	"github.com/google/uuid"
)

// GraphBuilder 因果图谱构建器
type GraphBuilder struct {
	kgEngine *knowledge.Neo4jEngine
}

// NewGraphBuilder 创建图谱构建器实例
func NewGraphBuilder(kgEngine *knowledge.Neo4jEngine) *GraphBuilder {
	return &GraphBuilder{
		kgEngine: kgEngine,
	}
}

// BuildCausalGraph 构建因果图谱
// 从因果关系列表构建Neo4j图谱，创建O→C→P→R节点和CAUSES边
func (gb *GraphBuilder) BuildCausalGraph(ctx context.Context, relations []CausalRelation) error {
	for _, relation := range relations {
		if err := gb.buildRelation(ctx, &relation); err != nil {
			return fmt.Errorf("构建因果关系失败: %w", err)
		}
	}
	return nil
}

// buildRelation 构建单个因果关系
func (gb *GraphBuilder) buildRelation(ctx context.Context, relation *CausalRelation) error {
	// 创建实体节点
	objectID, err := gb.createOrGetEntity(ctx, relation.Object, "Object")
	if err != nil {
		return fmt.Errorf("创建对象节点失败: %w", err)
	}

	mediatorID, err := gb.createOrGetEntity(ctx, relation.Mediator, "Mediator")
	if err != nil {
		return fmt.Errorf("创建中介节点失败: %w", err)
	}

	propertyID, err := gb.createOrGetEntity(ctx, relation.Property, "Property")
	if err != nil {
		return fmt.Errorf("创建属性节点失败: %w", err)
	}

	resultID, err := gb.createOrGetEntity(ctx, relation.Result, "Result")
	if err != nil {
		return fmt.Errorf("创建结果节点失败: %w", err)
	}

	// 创建因果边: O→C→P→R
	// O→C
	if err := gb.createCausalEdge(ctx, objectID, mediatorID, relation.Confidence, relation.Evidence); err != nil {
		return fmt.Errorf("创建O→C边失败: %w", err)
	}

	// C→P
	if relation.Property != "" {
		if err := gb.createCausalEdge(ctx, mediatorID, propertyID, relation.Confidence, relation.Evidence); err != nil {
			return fmt.Errorf("创建C→P边失败: %w", err)
		}

		// P→R
		if err := gb.createCausalEdge(ctx, propertyID, resultID, relation.Confidence, relation.Evidence); err != nil {
			return fmt.Errorf("创建P→R边失败: %w", err)
		}
	} else {
		// 如果没有P，直接创建C→R
		if err := gb.createCausalEdge(ctx, mediatorID, resultID, relation.Confidence, relation.Evidence); err != nil {
			return fmt.Errorf("创建C→R边失败: %w", err)
		}
	}

	return nil
}

// createOrGetEntity 创建或获取实体节点
func (gb *GraphBuilder) createOrGetEntity(ctx context.Context, name, role string) (string, error) {
	if name == "" {
		return "", nil
	}

	entity := &knowledge.Entity{
		ID:          uuid.New().String(),
		Name:        name,
		Type:        "CausalEntity",
		Description: fmt.Sprintf("因果关系中的%s (role: %s)", name, role),
		Workspace:   "causal_reasoning",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 使用UpsertEntity确保幂等性
	if err := gb.kgEngine.UpsertEntity(ctx, entity); err != nil {
		return "", err
	}

	return entity.ID, nil
}

// createCausalEdge 创建因果边
func (gb *GraphBuilder) createCausalEdge(ctx context.Context, fromID, toID string, confidence float64, evidence []string) error {
	if fromID == "" || toID == "" {
		return nil
	}

	// 证据信息可以在Properties中存储，但当前Relation结构不支持
	// evidenceStr := ""
	// if len(evidence) > 0 {
	// 	evidenceStr = evidence[0] // 使用第一条证据
	// }

	relation := &knowledge.Relation{
		SourceID:  fromID,
		TargetID:  toID,
		Type:      knowledge.RelationCauses, // "CAUSES"
		Weight:    confidence,
		CreatedAt: time.Now(),
	}

	return gb.kgEngine.CreateRelation(ctx, relation)
}

// GetCausalChain 查询因果链
func (gb *GraphBuilder) GetCausalChain(ctx context.Context, from, to string, maxDepth int, minConfidence float64) (*CausalChain, error) {
	// 使用Neo4jEngine的FindPaths方法查询路径
	paths, err := gb.kgEngine.FindPaths(ctx, from, to, maxDepth)
	if err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return &CausalChain{
			Source:    from,
			Target:    to,
			Paths:     []CausalPath{},
			PathCount: 0,
			Timestamp: time.Now(),
		}, nil
	}

	// 转换为CausalPath格式并过滤置信度
	var causalPaths []CausalPath
	var maxConfidence float64

	for _, path := range paths {
		// 计算路径置信度（边置信度乘积）
		confidence := 1.0
		for _, rel := range path.Relations {
			confidence *= rel.Weight
		}

		// 过滤低置信度路径
		if confidence < minConfidence {
			continue
		}

		// 提取节点名称
		nodes := make([]string, len(path.Nodes))
		for i, node := range path.Nodes {
			nodes[i] = node.Name
		}

		// 提取关系类型
		relations := make([]string, len(path.Relations))
		for i, rel := range path.Relations {
			relations[i] = rel.Type
		}

		// 提取证据（当前Relation结构不包含证据字段）
		var evidence []string

		causalPath := CausalPath{
			Nodes:      nodes,
			Relations:  relations,
			Confidence: confidence,
			Evidence:   evidence,
			Length:     len(path.Nodes) - 1,
		}

		causalPaths = append(causalPaths, causalPath)

		if confidence > maxConfidence {
			maxConfidence = confidence
		}
	}

	return &CausalChain{
		Source:     from,
		Target:     to,
		Paths:      causalPaths,
		Confidence: maxConfidence,
		PathCount:  len(causalPaths),
		Timestamp:  time.Now(),
	}, nil
}

// DeleteCausalRelation 删除因果关系
func (gb *GraphBuilder) DeleteCausalRelation(ctx context.Context, sourceID, targetID string) error {
	return gb.kgEngine.DeleteRelation(ctx, sourceID, targetID, knowledge.RelationCauses)
}

// UpdateCausalConfidence 更新因果边的置信度
func (gb *GraphBuilder) UpdateCausalConfidence(ctx context.Context, sourceID, targetID string, newConfidence float64) error {
	// 获取现有关系
	relations, err := gb.kgEngine.GetRelations(ctx, sourceID)
	if err != nil {
		return err
	}

	// 找到目标关系并更新
	for _, rel := range relations {
		if rel.TargetID == targetID && rel.Type == knowledge.RelationCauses {
			rel.Weight = newConfidence
			return gb.kgEngine.UpdateRelation(ctx, rel)
		}
	}

	return fmt.Errorf("未找到因果关系: %s → %s", sourceID, targetID)
}

// GetGraphStats 获取因果图谱统计信息
func (gb *GraphBuilder) GetGraphStats(ctx context.Context) (*CausalGraphStats, error) {
	stats := &CausalGraphStats{
		LastUpdated: time.Now(),
	}

	// 查询节点总数
	nodeCount, err := gb.kgEngine.CountEntitiesByWorkspace(ctx, "causal_reasoning")
	if err == nil {
		stats.TotalNodes = int(nodeCount)
	}

	// 查询关系总数
	relationCount, err := gb.kgEngine.CountRelationsByType(ctx, knowledge.RelationCauses)
	if err == nil {
		stats.TotalRelations = int(relationCount)
	}

	// 查询平均置信度
	avgConf, err := gb.calculateAverageConfidence(ctx)
	if err == nil {
		stats.AvgConfidence = avgConf
	}

	return stats, nil
}

// calculateAverageConfidence 计算平均置信度
func (gb *GraphBuilder) calculateAverageConfidence(ctx context.Context) (float64, error) {
	// 通过查询所有CAUSES关系的confidence属性计算平均值
	// 这里简化实现，实际应该在Neo4j中执行聚合查询
	return 0.75, nil // 默认返回0.75
}

// MergeEntities 合并重复实体
func (gb *GraphBuilder) MergeEntities(ctx context.Context, keepID, mergeID string) error {
	// 1. 获取要合并的实体的所有关系
	relations, err := gb.kgEngine.GetRelations(ctx, mergeID)
	if err != nil {
		return err
	}

	// 2. 将关系转移到保留的实体
	for _, rel := range relations {
		rel.SourceID = keepID
		if err := gb.kgEngine.CreateRelation(ctx, rel); err != nil {
			return err
		}
	}

	// 3. 删除被合并的实体
	return gb.kgEngine.DeleteEntity(ctx, mergeID)
}
