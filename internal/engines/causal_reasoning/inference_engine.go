package causal_reasoning

import (
	"context"
	"fmt"
	"time"

	"github.com/contextkeeper/service/internal/engines/multi_dimensional_retrieval/knowledge"
)

// InferenceEngine 因果推理查询引擎
type InferenceEngine struct {
	kgEngine     *knowledge.Neo4jEngine
	graphBuilder *GraphBuilder
}

// NewInferenceEngine 创建推理引擎实例
func NewInferenceEngine(kgEngine *knowledge.Neo4jEngine) *InferenceEngine {
	return &InferenceEngine{
		kgEngine:     kgEngine,
		graphBuilder: NewGraphBuilder(kgEngine),
	}
}

// InferForward 正向推理：查询给定原因可能导致的结果
// 实现策划书中的Cypher查询：MATCH path = (start)-[:CAUSES*1..4]->(end)
func (ie *InferenceEngine) InferForward(ctx context.Context, cause string, maxDepth int, minConfidence float64, limit int) ([]CausalChain, error) {
	if maxDepth <= 0 {
		maxDepth = 4 // 默认最大深度4跳
	}
	if limit <= 0 {
		limit = 10 // 默认返回10条结果
	}

	// 获取起始实体
	entities, err := ie.kgEngine.SearchEntitiesByName(ctx, cause, "causal_reasoning", 1)
	if err != nil || len(entities) == 0 {
		return []CausalChain{}, fmt.Errorf("未找到实体: %s", cause)
	}

	startEntity := entities[0]

	// 查询所有出边关系（正向）
	chains := make([]CausalChain, 0)
	visited := make(map[string]bool)

	// 递归查找所有可达路径
	ie.findForwardPaths(ctx, startEntity, maxDepth, minConfidence, &chains, visited, []string{}, 1.0, []string{})

	// 按置信度排序
	sortChainsByConfidence(chains)

	// 限制返回数量
	if len(chains) > limit {
		chains = chains[:limit]
	}

	return chains, nil
}

// InferBackward 反向追溯：查询给定结果的可能原因
// 实现策划书中的Cypher查询：MATCH path = (start)<-[:CAUSES*1..4]-(end)
func (ie *InferenceEngine) InferBackward(ctx context.Context, effect string, maxDepth int, minConfidence float64, limit int) ([]CausalChain, error) {
	if maxDepth <= 0 {
		maxDepth = 4
	}
	if limit <= 0 {
		limit = 10
	}

	// 获取结果实体
	entities, err := ie.kgEngine.SearchEntitiesByName(ctx, effect, "causal_reasoning", 1)
	if err != nil || len(entities) == 0 {
		return []CausalChain{}, fmt.Errorf("未找到实体: %s", effect)
	}

	endEntity := entities[0]

	// 查询所有入边关系（反向）
	chains := make([]CausalChain, 0)
	visited := make(map[string]bool)

	// 递归查找所有可达路径
	ie.findBackwardPaths(ctx, endEntity, maxDepth, minConfidence, &chains, visited, []string{}, 1.0, []string{})

	// 按置信度排序
	sortChainsByConfidence(chains)

	// 限制返回数量
	if len(chains) > limit {
		chains = chains[:limit]
	}

	return chains, nil
}

// QueryCausalChain 查询两个实体之间的因果链
func (ie *InferenceEngine) QueryCausalChain(ctx context.Context, from, to string, maxDepth int, minConfidence float64, limit int) (*CausalChain, error) {
	if maxDepth <= 0 {
		maxDepth = 4
	}

	return ie.graphBuilder.GetCausalChain(ctx, from, to, maxDepth, minConfidence)
}

// findForwardPaths 递归查找正向路径
func (ie *InferenceEngine) findForwardPaths(
	ctx context.Context,
	current *knowledge.Entity,
	remainingDepth int,
	minConfidence float64,
	chains *[]CausalChain,
	visited map[string]bool,
	currentPath []string,
	currentConfidence float64,
	currentEvidence []string,
) {
	// 深度限制
	if remainingDepth <= 0 {
		return
	}

	// 标记访问
	visited[current.ID] = true
	currentPath = append(currentPath, current.Name)

	// 获取所有出边关系
	relations, err := ie.kgEngine.GetRelations(ctx, current.ID)
	if err != nil || len(relations) == 0 {
		// 如果路径长度>1，记录为一条因果链
		if len(currentPath) > 1 {
			chain := CausalChain{
				Source: currentPath[0],
				Target: currentPath[len(currentPath)-1],
				Paths: []CausalPath{
					{
						Nodes:      append([]string{}, currentPath...),
						Confidence: currentConfidence,
						Evidence:   append([]string{}, currentEvidence...),
						Length:     len(currentPath) - 1,
					},
				},
				Confidence: currentConfidence,
				PathCount:  1,
				Timestamp:  time.Now(),
			}
			*chains = append(*chains, chain)
		}
		delete(visited, current.ID)
		return
	}

	// 遍历所有CAUSES关系
	for _, rel := range relations {
		if rel.Type != knowledge.RelationCauses {
			continue
		}

		// 获取置信度（使用Weight字段）
		confidence := rel.Weight
		if confidence == 0 {
			confidence = 1.0
		}

		// 置信度过滤
		newConfidence := currentConfidence * confidence
		if newConfidence < minConfidence {
			continue
		}

		// 避免循环
		if visited[rel.TargetID] {
			continue
		}

		// 获取目标实体
		targetEntity, err := ie.kgEngine.GetEntityByID(ctx, rel.TargetID)
		if err != nil || targetEntity == nil {
			continue
		}

		// 提取证据（当前Relation结构不包含证据字段）
		evidence := append([]string{}, currentEvidence...)

		// 递归查找
		ie.findForwardPaths(ctx, targetEntity, remainingDepth-1, minConfidence, chains, visited, currentPath, newConfidence, evidence)
	}

	// 回溯
	delete(visited, current.ID)
}

// findBackwardPaths 递归查找反向路径
func (ie *InferenceEngine) findBackwardPaths(
	ctx context.Context,
	current *knowledge.Entity,
	remainingDepth int,
	minConfidence float64,
	chains *[]CausalChain,
	visited map[string]bool,
	currentPath []string,
	currentConfidence float64,
	currentEvidence []string,
) {
	// 深度限制
	if remainingDepth <= 0 {
		return
	}

	// 标记访问
	visited[current.ID] = true
	currentPath = append([]string{current.Name}, currentPath...) // 前插

	// 获取所有指向当前节点的关系（入边）
	incomingRels, err := ie.kgEngine.GetIncomingRelations(ctx, current.ID, knowledge.RelationCauses)
	if err != nil || len(incomingRels) == 0 {
		// 如果路径长度>1，记录为一条因果链
		if len(currentPath) > 1 {
			chain := CausalChain{
				Source: currentPath[0],
				Target: currentPath[len(currentPath)-1],
				Paths: []CausalPath{
					{
						Nodes:      append([]string{}, currentPath...),
						Confidence: currentConfidence,
						Evidence:   append([]string{}, currentEvidence...),
						Length:     len(currentPath) - 1,
					},
				},
				Confidence: currentConfidence,
				PathCount:  1,
				Timestamp:  time.Now(),
			}
			*chains = append(*chains, chain)
		}
		delete(visited, current.ID)
		return
	}

	// 遍历所有入边CAUSES关系
	for _, rel := range incomingRels {
		// 获取置信度（使用Weight字段）
		confidence := rel.Weight
		if confidence == 0 {
			confidence = 1.0
		}

		// 置信度过滤
		newConfidence := currentConfidence * confidence
		if newConfidence < minConfidence {
			continue
		}

		// 避免循环
		if visited[rel.SourceID] {
			continue
		}

		// 获取源实体
		sourceEntity, err := ie.kgEngine.GetEntityByID(ctx, rel.SourceID)
		if err != nil || sourceEntity == nil {
			continue
		}

		// 提取证据（当前Relation结构不包含证据字段）
		evidence := append([]string{}, currentEvidence...)

		// 递归查找
		ie.findBackwardPaths(ctx, sourceEntity, remainingDepth-1, minConfidence, chains, visited, currentPath, newConfidence, evidence)
	}

	// 回溯
	delete(visited, current.ID)
}

// GetRelatedCauses 获取与给定实体相关的所有原因
func (ie *InferenceEngine) GetRelatedCauses(ctx context.Context, entityName string, limit int) ([]*knowledge.Entity, error) {
	entities, err := ie.kgEngine.SearchEntitiesByName(ctx, entityName, "causal_reasoning", 1)
	if err != nil || len(entities) == 0 {
		return []*knowledge.Entity{}, fmt.Errorf("未找到实体: %s", entityName)
	}

	entity := entities[0]
	return ie.kgEngine.GetRelatedEntities(ctx, entity.ID, limit)
}

// GetRelatedEffects 获取与给定实体相关的所有结果
func (ie *InferenceEngine) GetRelatedEffects(ctx context.Context, entityName string, limit int) ([]*knowledge.Entity, error) {
	entities, err := ie.kgEngine.SearchEntitiesByName(ctx, entityName, "causal_reasoning", 1)
	if err != nil || len(entities) == 0 {
		return []*knowledge.Entity{}, fmt.Errorf("未找到实体: %s", entityName)
	}

	entity := entities[0]
	return ie.kgEngine.GetRelatedEntities(ctx, entity.ID, limit)
}

// sortChainsByConfidence 按置信度降序排序
func sortChainsByConfidence(chains []CausalChain) {
	for i := 0; i < len(chains)-1; i++ {
		for j := i + 1; j < len(chains); j++ {
			if chains[j].Confidence > chains[i].Confidence {
				chains[i], chains[j] = chains[j], chains[i]
			}
		}
	}
}
