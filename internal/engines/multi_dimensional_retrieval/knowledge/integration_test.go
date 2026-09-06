package knowledge

import (
	"context"
	"testing"
	"time"
)

// TestMemoryEntityMapping 测试Memory-Entity双向映射
func TestMemoryEntityMapping(t *testing.T) {
	// 模拟LLM分析结果
	analysisData := map[string]interface{}{
		"entities": []interface{}{
			map[string]interface{}{"name": "Redis", "type": "Technology", "description": "缓存数据库"},
			map[string]interface{}{"name": "MySQL", "type": "Technology", "description": "关系数据库"},
		},
		"events": []interface{}{
			map[string]interface{}{"name": "连接超时", "type": "Issue", "description": "Redis连接超时"},
		},
		"solutions": []interface{}{
			map[string]interface{}{"name": "增加连接池", "type": "method", "description": "优化连接池配置"},
		},
	}

	workspace := "/test/workspace"
	memoryID := "memory-123-456"

	// 1. 从分析结果提取知识节点IDs
	nodeIDs := ExtractKnowledgeNodeIDs(analysisData, workspace)

	if len(nodeIDs.EntityIDs) != 2 {
		t.Errorf("期望2个Entity ID, 实际: %d", len(nodeIDs.EntityIDs))
	}

	if len(nodeIDs.EventIDs) != 1 {
		t.Errorf("期望1个Event ID, 实际: %d", len(nodeIDs.EventIDs))
	}

	if len(nodeIDs.SolutionIDs) != 1 {
		t.Errorf("期望1个Solution ID, 实际: %d", len(nodeIDs.SolutionIDs))
	}

	// 2. 构建Entity对象
	entities := BuildEntitiesFromAnalysis(analysisData, workspace)

	for _, entity := range entities {
		// 验证UUID格式
		if !ValidateUUID(entity.ID) {
			t.Errorf("Entity UUID格式无效: %s", entity.ID)
		}

		// 添加MemoryID关联
		entity.MemoryIDs = append(entity.MemoryIDs, memoryID)

		// 验证Entity可以找到关联的MemoryID
		found := false
		for _, mid := range entity.MemoryIDs {
			if mid == memoryID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Entity未正确关联MemoryID: %s", entity.Name)
		}
	}

	// 3. 验证同一实体的UUID是确定性的
	uuid1 := GenerateEntityUUID("Redis", "Technology", workspace)
	uuid2 := GenerateEntityUUID("Redis", "Technology", workspace)

	if uuid1 != uuid2 {
		t.Error("相同实体应该生成相同的UUID")
	}

	// 验证entities中的Redis UUID与单独生成的一致
	for _, entity := range entities {
		if entity.Name == "Redis" {
			expectedID := GenerateEntityUUID("redis", entity.Type, workspace)
			if entity.ID != expectedID {
				t.Errorf("Entity ID与预期不符: got %s, want %s", entity.ID, expectedID)
			}
		}
	}
}

// TestMultiDimensionalRetrieverCreation 测试多维检索器创建
func TestMultiDimensionalRetrieverCreation(t *testing.T) {
	// 创建无Neo4j的检索器
	retriever := NewMultiDimensionalRetriever(nil)

	if retriever == nil {
		t.Error("检索器不应为nil")
	}
}

// TestRetrievalStrategySelection 测试检索策略选择集成
func TestRetrievalStrategySelection(t *testing.T) {
	testCases := []struct {
		query            string
		expectedStrategy RetrievalStrategy
		description      string
	}{
		{
			query:            "昨天讨论的Redis问题是什么？",
			expectedStrategy: StrategyTimeRecall,
			description:      "时间引用查询应选择time_recall策略",
		},
		{
			query:            "数据库的架构设计方案",
			expectedStrategy: StrategyGraphPriority,
			description:      "实体引用查询应选择graph_priority策略",
		},
		{
			query:            "上周关于API接口的设计讨论",
			expectedStrategy: StrategyTimeRecall, // 时间引用优先
			description:      "复杂查询（时间+实体）应选择time_recall策略",
		},
		{
			query:            "hello world",
			expectedStrategy: StrategyVectorOnly,
			description:      "简单查询应选择vector_only策略",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			analysis := AnalyzeQuery(tc.query)
			strategy := SelectRetrievalStrategy(tc.query, analysis)

			if strategy != tc.expectedStrategy {
				t.Errorf("查询 '%s': 期望策略 %s, 实际 %s",
					tc.query, tc.expectedStrategy, strategy)
			}
		})
	}
}

// TestRRFFusionIntegration 测试RRF融合集成
func TestRRFFusionIntegration(t *testing.T) {
	// 模拟三路检索结果
	vectorResults := []RetrievalResult{
		{MemoryID: "mem1", Score: 0.95, Source: "vector", Content: "Redis性能优化"},
		{MemoryID: "mem2", Score: 0.85, Source: "vector", Content: "MySQL索引优化"},
		{MemoryID: "mem3", Score: 0.75, Source: "vector", Content: "缓存策略"},
	}

	graphResults := []RetrievalResult{
		{MemoryID: "mem1", Score: 0.90, Source: "graph", Content: "Redis相关讨论"},
		{MemoryID: "mem4", Score: 0.80, Source: "graph", Content: "数据库架构"},
	}

	timeResults := []RetrievalResult{
		{MemoryID: "mem2", Score: 0.88, Source: "time", Content: "昨天的MySQL讨论"},
		{MemoryID: "mem5", Score: 0.70, Source: "time", Content: "上周的会议记录"},
	}

	// 使用自定义RRF配置
	config := &RRFConfig{
		K: 60.0,
		SourceWeight: map[string]float64{
			"vector": 1.0,
			"graph":  1.2, // 图谱结果权重更高
			"time":   0.8,
		},
	}

	// 执行融合
	resultSets := [][]RetrievalResult{vectorResults, graphResults, timeResults}
	fused := FuseResultsWithRRF(resultSets, config)

	// 验证结果
	if len(fused) != 5 {
		t.Errorf("期望5个融合结果, 实际: %d", len(fused))
	}

	// mem1出现在vector和graph中，应该排在前面
	if len(fused) > 0 && fused[0].MemoryID != "mem1" {
		t.Logf("警告: mem1应该是第一名（出现在多个结果集中），实际第一名是: %s", fused[0].MemoryID)
	}

	// 验证所有结果都有正的分数
	for _, result := range fused {
		if result.Score <= 0 {
			t.Errorf("结果分数应该为正: %s score=%f", result.MemoryID, result.Score)
		}
	}
}

// TestConfigIntegration 测试配置集成
func TestConfigIntegration(t *testing.T) {
	// 加载默认配置
	config := DefaultKnowledgeGraphConfig()

	// 验证配置完整性
	if config.RRFConfig == nil {
		t.Error("RRFConfig不应为nil")
	}

	// 验证功能开关状态
	if !config.IsGraphRetrievalEnabled() {
		t.Error("默认应启用图谱检索")
	}

	if !config.IsGraphStorageEnabled() {
		t.Error("默认应启用图谱存储")
	}

	if !config.IsRRFFusionEnabled() {
		t.Error("默认应启用RRF融合")
	}

	// 禁用总开关
	config.Enabled = false

	if config.IsGraphRetrievalEnabled() {
		t.Error("总开关禁用后，图谱检索应禁用")
	}
}

// TestMultiDimensionalRetrieverRetrieve 测试多维检索器的Retrieve方法
func TestMultiDimensionalRetrieverRetrieve(t *testing.T) {
	// 创建检索器（无真实引擎）
	retriever := NewMultiDimensionalRetriever(nil)

	ctx := context.Background()
	req := &RetrievalRequest{
		Query:     "测试查询",
		SessionID: "session-123",
		UserID:    "user-456",
		Workspace: "/workspace",
		TopK:      10,
		MinScore:  0.5,
	}

	// 执行检索
	results, err := retriever.Retrieve(ctx, req)
	if err != nil {
		t.Errorf("检索不应出错: %v", err)
	}

	// 无真实引擎时，结果应为空
	if len(results) != 0 {
		t.Errorf("无真实引擎时，结果应为空, 实际: %d", len(results))
	}
}

// TestBuildRelationsFromAnalysis 测试从分析结果构建关系
func TestBuildRelationsFromAnalysis(t *testing.T) {
	analysisData := map[string]interface{}{
		"relations": []interface{}{
			map[string]interface{}{
				"source": "Redis",
				"target": "MySQL",
				"type":   "RELATES_TO",
				"weight": 0.7,
			},
		},
	}

	// 创建entityMap
	entityMap := map[string]string{
		"Redis": "redis-uuid-123",
		"MySQL": "mysql-uuid-456",
	}

	relations := BuildRelationsFromAnalysis(analysisData, entityMap)

	if len(relations) != 1 {
		t.Errorf("期望1个关系, 实际: %d", len(relations))
	}

	if len(relations) > 0 {
		rel := relations[0]
		if rel.SourceID != "redis-uuid-123" {
			t.Errorf("SourceID错误: %s", rel.SourceID)
		}
		if rel.TargetID != "mysql-uuid-456" {
			t.Errorf("TargetID错误: %s", rel.TargetID)
		}
		if rel.Weight != 0.7 {
			t.Errorf("Weight错误: %f", rel.Weight)
		}
	}
}

// TestKnowledgeNodeIDsEmpty 测试空分析结果
func TestKnowledgeNodeIDsEmpty(t *testing.T) {
	emptyAnalysis := map[string]interface{}{}
	nodeIDs := ExtractKnowledgeNodeIDs(emptyAnalysis, "/workspace")

	if len(nodeIDs.EntityIDs) != 0 {
		t.Errorf("空分析应返回空EntityIDs, 实际: %d", len(nodeIDs.EntityIDs))
	}

	if len(nodeIDs.EventIDs) != 0 {
		t.Errorf("空分析应返回空EventIDs, 实际: %d", len(nodeIDs.EventIDs))
	}

	if len(nodeIDs.SolutionIDs) != 0 {
		t.Errorf("空分析应返回空SolutionIDs, 实际: %d", len(nodeIDs.SolutionIDs))
	}
}

// TestBuildEventsFromAnalysis 测试从分析结果构建Event
func TestBuildEventsFromAnalysis(t *testing.T) {
	analysisData := map[string]interface{}{
		"events": []interface{}{
			map[string]interface{}{
				"name":        "连接超时",
				"type":        "Issue",
				"description": "数据库连接超时问题",
			},
		},
	}

	events := BuildEventsFromAnalysis(analysisData, "/workspace")

	if len(events) != 1 {
		t.Errorf("期望1个Event, 实际: %d", len(events))
	}

	if len(events) > 0 {
		event := events[0]
		if event.Name != "连接超时" {
			t.Errorf("Event名称错误: %s", event.Name)
		}
		if !ValidateUUID(event.ID) {
			t.Errorf("Event UUID格式无效: %s", event.ID)
		}
	}
}

// TestBuildSolutionsFromAnalysis 测试从分析结果构建Solution
func TestBuildSolutionsFromAnalysis(t *testing.T) {
	analysisData := map[string]interface{}{
		"solutions": []interface{}{
			map[string]interface{}{
				"name":        "增加连接池",
				"type":        "method",
				"description": "扩大连接池容量",
			},
		},
	}

	solutions := BuildSolutionsFromAnalysis(analysisData, "/workspace")

	if len(solutions) != 1 {
		t.Errorf("期望1个Solution, 实际: %d", len(solutions))
	}

	if len(solutions) > 0 {
		solution := solutions[0]
		if solution.Name != "增加连接池" {
			t.Errorf("Solution名称错误: %s", solution.Name)
		}
		if !ValidateUUID(solution.ID) {
			t.Errorf("Solution UUID格式无效: %s", solution.ID)
		}
	}
}

// TestRetrievalRequestWithStrategy 测试带策略的检索请求
func TestRetrievalRequestWithStrategy(t *testing.T) {
	retriever := NewMultiDimensionalRetriever(nil)
	ctx := context.Background()

	// 测试指定策略
	strategies := []RetrievalStrategy{
		StrategyTimeRecall,
		StrategyGraphPriority,
		StrategyTimeContentHybrid,
		StrategyVectorOnly,
	}

	for _, strategy := range strategies {
		req := &RetrievalRequest{
			Query:     "测试查询",
			SessionID: "session-123",
			UserID:    "user-456",
			Workspace: "/workspace",
			Strategy:  strategy,
			TopK:      10,
		}

		_, err := retriever.Retrieve(ctx, req)
		if err != nil {
			t.Errorf("策略 %s 检索失败: %v", strategy, err)
		}
	}
}

// TestTimeRange 测试时间范围
func TestTimeRange(t *testing.T) {
	now := time.Now()
	timeRange := &TimeRange{
		Start: now.Add(-24 * time.Hour),
		End:   now,
	}

	if timeRange.End.Before(timeRange.Start) {
		t.Error("结束时间不应早于开始时间")
	}

	duration := timeRange.End.Sub(timeRange.Start)
	if duration < 23*time.Hour || duration > 25*time.Hour {
		t.Errorf("时间范围应约为24小时, 实际: %v", duration)
	}
}

